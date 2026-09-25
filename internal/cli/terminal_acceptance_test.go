package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestTerminalInstalledZshStarshipIntegration(t *testing.T) {
	zsh, zshErr := exec.LookPath("zsh")
	starship, starshipErr := exec.LookPath("starship")
	if zshErr != nil || starshipErr != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("CI needs zsh and starship: zsh=%v, starship=%v", zshErr, starshipErr)
		}
		t.Skipf("terminal acceptance needs zsh and starship: zsh=%v, starship=%v", zshErr, starshipErr)
	}

	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sandbox := t.TempDir()
	sourceRoot := filepath.Join(sandbox, "source")
	home := filepath.Join(sandbox, "home")
	stateRoot := filepath.Join(sandbox, "state")
	operatorHome := filepath.Join(sandbox, "operator")
	for _, dir := range []string{sourceRoot, home, stateRoot, operatorHome, filepath.Join(sourceRoot, "configs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"zsh", "starship", "bat"} {
		if err := os.CopyFS(filepath.Join(sourceRoot, "configs", name), os.DirFS(filepath.Join(repositoryRoot, "configs", name))); err != nil {
			t.Fatalf("copy %s Source of Truth: %v", name, err)
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(repositoryRoot, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	surface := selectedsurface.EvaluateForPlatform(*m, []string{"zsh", "starship"}, runtime.GOOS, runtime.GOARCH)
	if len(surface.Provisioners) != 0 {
		t.Fatalf("terminal acceptance cannot run Provisioners: %v", surface.Provisioners)
	}

	t.Setenv("HOME", operatorHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(operatorHome, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(operatorHome, ".cache"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(operatorHome, ".local", "state"))
	stubManifestProvisionerTools(t)

	common := []string{
		"--tag", "zsh", "--tag", "starship", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}
	plan := cli.NewRootCommand()
	var planOutput bytes.Buffer
	plan.SetOut(&planOutput)
	plan.SetErr(&planOutput)
	plan.SetArgs(append([]string{"plan"}, common...))
	if err := plan.Execute(); err != nil {
		t.Fatalf("sandboxed terminal plan: %v\n%s", err, planOutput.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("read-only plan changed .zshrc: %v", err)
	}

	install := cli.NewRootCommand()
	var installOutput bytes.Buffer
	install.SetOut(&installOutput)
	install.SetErr(&installOutput)
	install.SetArgs(append([]string{"install", "--yes", "--skip-deps", "--no-tui"}, common...))
	if err := install.Execute(); err != nil {
		t.Fatalf("sandboxed terminal install: %v\n%s", err, installOutput.String())
	}
	loader, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil || !bytes.Contains(loader, []byte("source \"${HOME}/.config/dots/zsh/zshrc\"")) {
		t.Fatalf("installed .zshrc does not load portable config: %v\n%s", err, loader)
	}
	for target, source := range map[string]string{
		".zshenv":                "configs/zsh/zshenv",
		".config/dots/zsh/zshrc": "configs/zsh/zshrc",
		".config/starship.toml":  "configs/starship/starship.toml",
	} {
		got, err := os.Readlink(filepath.Join(home, target))
		if err != nil || got != filepath.Join(sourceRoot, source) {
			t.Fatalf("installed %s link = %q, %v; want %s", target, got, err, filepath.Join(sourceRoot, source))
		}
	}

	binDir := filepath.Join(sandbox, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, binary := range map[string]string{"zsh": zsh, "starship": starship} {
		if err := os.Symlink(binary, filepath.Join(binDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(zsh, "-i", "-c", `print -r -- "STARSHIP_HOOK=${precmd_functions[(r)prompt_starship_precmd]}"; print -P -- "$PROMPT"`)
	cmd.Dir = home
	cmd.Env = []string{
		"HOME=" + home,
		"ZDOTDIR=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"STARSHIP_CONFIG=" + filepath.Join(home, ".config", "starship.toml"),
		"TERM=xterm-256color",
		"PATH=" + binDir + string(os.PathListSeparator) + "/usr/bin" + string(os.PathListSeparator) + "/bin",
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("sandboxed Zsh startup: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "STARSHIP_HOOK=prompt_starship_precmd") || !strings.Contains(string(output), "❯") {
		t.Fatalf("installed Zsh and Starship configuration did not initialize and render the prompt:\n%s", output)
	}
}
