package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestInstalledShellPoliciesPreserveNativeContent(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not available")
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sandbox := t.TempDir()
	home, source, stateRoot := filepath.Join(sandbox, "home"), filepath.Join(sandbox, "source"), filepath.Join(sandbox, "state")
	for _, dir := range []string{home, filepath.Join(source, "configs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"zsh", "git", "bat", "themes"} {
		if err := os.CopyFS(filepath.Join(source, "configs", name), os.DirFS(filepath.Join(repo, "configs", name))); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(repo, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(source, "dots.yaml")
	if err := os.WriteFile(file, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	operatorHome := t.TempDir()
	t.Setenv("HOME", operatorHome)
	t.Setenv("ZDOTDIR", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(operatorHome, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(operatorHome, ".local/state"))
	stubManifestProvisionerTools(t)
	bin := t.TempDir()
	for name, binary := range map[string]string{"zsh": zsh, "git": git} {
		if err := os.Symlink(binary, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the retained curl bootstrap with a local runtime fixture. No
	// downloaded code or real package manager is reachable from this install.
	writeExecStub(t, filepath.Join(bin, "curl"), `#!/bin/sh
test "$1" = -fsSL && test "$2" = -o || exit 91
test "$3" = "$HOME/.zim/zimfw.zsh" || exit 92
test "$4" = https://github.com/zimfw/zimfw/releases/latest/download/zimfw.zsh || exit 93
cat > "$3" <<'ZIM'
zmodule() { print -r -- "$*" >> "$HOME/modules-loaded"; }
source "${ZIM_CONFIG_FILE:-${ZDOTDIR:-${HOME}}/.zimrc}"
print 'export DOTS_ZIM_LOADED=1' > "$ZIM_HOME/init.zsh"
ZIM
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	common := []string{"--tag", "zsh", "--tag", "zimfw", "--tag", "git", "--file", file, "--source-root", source, "--home", home, "--state-root", stateRoot}
	run := func(args ...string) string {
		t.Helper()
		cmd := cli.NewRootCommand()
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		cmd.SetArgs(append(args, common...))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output.String())
		}
		return output.String()
	}
	run("install", "--yes", "--skip-deps", "--no-tui")
	native := map[string]string{
		".zshrc":  "\nexport PERSONAL_RC=preserved\n",
		".zshenv": "\nexport PERSONAL_ENV=preserved\n",
		".zimrc":  "\nzmodule personal/module\n",
	}
	for name, external := range native {
		path := filepath.Join(home, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("%s must be a regular marked loader: %v, %v", name, info, err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		native[name] = string(content) + external
		if err := os.WriteFile(path, []byte(native[name]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run("install", "--yes", "--skip-deps", "--no-tui")
	for name, want := range native {
		got, err := os.ReadFile(filepath.Join(home, name))
		if err != nil || string(got) != want {
			t.Fatalf("repeat install changed external %s: %q, %v", name, got, err)
		}
	}
	for _, dir := range []string{".foundry/bin", ".cargo", ".local/bin", "Library/pnpm/bin", ".bun/bin"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeCLISource(t, home, ".cargo/env", "export CARGO_LOADED=1\n")
	writeCLISource(t, home, ".local/bin/env", "export LOCAL_BIN_LOADED=1\n")
	startup := func() ([]byte, error) {
		cmd := exec.Command(zsh, "-i", "-c", `print -r -- "LOADED=$PERSONAL_ENV:$PERSONAL_RC:$CARGO_LOADED:$LOCAL_BIN_LOADED:$DOTS_ZIM_LOADED"; print -r -- "PATH=$PATH"`)
		cmd.Env = []string{"HOME=" + home, "ZDOTDIR=" + home, "PATH=" + bin + ":/usr/bin:/bin", "TERM=xterm-256color"}
		return cmd.CombinedOutput()
	}
	output, err := startup()
	if err != nil || !strings.Contains(string(output), "LOADED=preserved:preserved:1:1:1") {
		t.Fatalf("installed Zsh did not load configuration: %v\n%s", err, output)
	}
	if strings.Contains(string(output), filepath.Join(home, ".foundry/bin")) {
		t.Fatalf("managed configuration added Foundry: %s", output)
	}
	for _, dir := range []string{"Library/pnpm/bin", ".bun/bin"} {
		if !strings.Contains(string(output), filepath.Join(home, dir)) {
			t.Fatalf("lost %s PATH integration: %s", dir, output)
		}
	}
	modules, err := os.ReadFile(filepath.Join(home, "modules-loaded"))
	if err != nil || !strings.Contains(string(modules), "Aloxaf/fzf-tab\n") || !strings.Contains(string(modules), "personal/module\n") {
		t.Fatalf("Zim did not load portable and personal modules: %s, %v", modules, err)
	}
	getGit := func(want string) {
		t.Helper()
		cmd := exec.Command(git, "config", "--global", "--includes", "--get", "pull.rebase")
		cmd.Env = []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "XDG_CONFIG_HOME=" + filepath.Join(home, ".config")}
		got, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(got)) != want {
			t.Fatalf("effective pull.rebase = %q, %v; want %s", got, err, want)
		}
	}
	getGit("true")
	writeCLISource(t, home, ".gitconfig.local", "[pull]\n\trebase = false\n")
	getGit("false")
	gitNative := filepath.Join(home, ".gitconfig")
	gitBytes, err := os.ReadFile(gitNative)
	if err != nil {
		t.Fatal(err)
	}
	gitBytes = append(gitBytes, []byte("\n# personal native override\n[pull]\n\trebase = true\n")...)
	if err := os.WriteFile(gitNative, gitBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	// A personal Foundry integration remains outside dots' ownership.
	personalEnv := native[".zshenv"] + "export PATH=\"${PATH}:${HOME}/.foundry/bin\"\n"
	if err := os.WriteFile(filepath.Join(home, ".zshenv"), []byte(personalEnv), 0o600); err != nil {
		t.Fatal(err)
	}
	run("install", "--yes", "--skip-deps", "--no-tui")
	getGit("true")
	if got, err := os.ReadFile(gitNative); err != nil || !bytes.Equal(got, gitBytes) {
		t.Fatalf("repeat install changed native Git content: %q, %v", got, err)
	}
	output, err = startup()
	if err != nil || !strings.Contains(string(output), filepath.Join(home, ".foundry/bin")) {
		t.Fatalf("lost personal Foundry PATH: %s, %v", output, err)
	}
	// Updating the portable module list must also refresh Zim's compiled init,
	// even though its native marked loader did not change.
	portableZim := filepath.Join(source, "configs/zsh/zimrc")
	portableBytes, err := os.ReadFile(portableZim)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(portableZim, append(portableBytes, []byte("\nzmodule project/new-module\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for path, timestamp := range map[string]time.Time{
		filepath.Join(home, ".zimrc"):        now.Add(-2 * time.Hour),
		filepath.Join(home, ".zim/init.zsh"): now.Add(-time.Hour),
		portableZim:                          now,
	} {
		if err := os.Chtimes(path, timestamp, timestamp); err != nil {
			t.Fatal(err)
		}
	}
	output, err = startup()
	if err != nil {
		t.Fatalf("startup after module update: %s, %v", output, err)
	}
	modules, err = os.ReadFile(filepath.Join(home, "modules-loaded"))
	if err != nil || !strings.Contains(string(modules), "project/new-module\n") {
		t.Fatalf("portable module update did not refresh Zim: %s, %v", modules, err)
	}
	if err := os.RemoveAll(filepath.Join(home, ".zim")); err != nil {
		t.Fatal(err)
	}
	output, err = startup()
	if err != nil || !strings.Contains(string(output), "Zim not found") || !strings.Contains(string(output), "LOADED=preserved:preserved:1:1:") {
		t.Fatalf("missing Zim runtime broke shell startup: %s, %v", output, err)
	}
	if entries, err := os.ReadDir(operatorHome); err != nil || len(entries) != 0 {
		t.Fatalf("install touched inherited HOME: %v, %v", entries, err)
	}
}
