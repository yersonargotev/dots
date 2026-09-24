package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepositoryTerminalCockpitInstallsCompleteSandboxedFlow is the high-level
// acceptance seam for the terminal cockpit. Lower-level tests exercise real
// widgets, popup commands, prompt rendering, and configuration parsing; this
// test proves their complete atomic selection installs together and provisions
// only the pinned external plugins inside an isolated home.
func TestRepositoryTerminalCockpitInstallsCompleteSandboxedFlow(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	stubDir := t.TempDir()
	externalLog := filepath.Join(home, "external.log")

	oldOS, oldArch := installHostOS, installHostArch
	installHostOS, installHostArch = "darwin", "arm64"
	t.Cleanup(func() {
		installHostOS, installHostArch = oldOS, oldArch
	})
	t.Setenv("HOME", fakeRealHome)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("EXTERNAL_LOG", externalLog)

	writeHerdrIntegrationStub(t, filepath.Join(stubDir, "herdr"), `#!/bin/sh
printf 'herdr:%s\n' "$*" >> "$EXTERNAL_LOG"
`)
	writeHerdrIntegrationStub(t, filepath.Join(stubDir, "zsh"), `#!/bin/sh
printf 'zsh:%s\n' "$*" >> "$EXTERNAL_LOG"
mkdir -p "$HOME/.zim"
printf 'sandbox zim runtime\n' > "$HOME/.zim/zimfw.zsh"
printf 'sandbox zim init\n' > "$HOME/.zim/init.zsh"
`)
	for _, command := range []string{
		"atuin", "bat", "cargo", "curl", "eza", "fd", "fnm", "fzf", "git",
		"lazygit", "node", "open", "pbcopy", "python3", "rustc", "rustup",
		"starship", "tar", "zoxide",
	} {
		writeHerdrIntegrationStub(t, filepath.Join(stubDir, command), "#!/bin/sh\nexit 0\n")
	}
	// Any regression that reaches a package manager fails without touching the
	// host and leaves evidence in the sandbox.
	writeHerdrIntegrationStub(t, filepath.Join(stubDir, "brew"), `#!/bin/sh
printf 'brew:%s\n' "$*" >> "$EXTERNAL_LOG"
exit 97
`)

	args := []string{
		"--file", filepath.Join(repositoryRoot, "dots.yaml"),
		"--home", home,
		"--source-root", repositoryRoot,
		"--state-root", stateRoot,
		"--tag", "zsh",
		"--tag", "zimfw",
		"--tag", "starship",
		"--tag", "atuin",
		"--tag", "zoxide",
		"--tag", "herdr",
	}

	dryRun := NewRootCommand()
	var dryRunOutput bytes.Buffer
	dryRun.SetOut(&dryRunOutput)
	dryRun.SetErr(&dryRunOutput)
	dryRun.SetArgs(append([]string{"install", "--dry-run"}, args...))
	if err := dryRun.Execute(); err != nil {
		t.Fatalf("terminal cockpit dry-run failed: %v\noutput:\n%s", err, dryRunOutput.String())
	}
	for _, want := range []string{
		".config/herdr/config.toml",
		".config/starship.toml",
		".config/atuin/config.toml",
		"rmarganti/herdr-pluck",
		"d1eacb80956c3a23ab6f7428a9e83961fb86ba28",
	} {
		if !strings.Contains(dryRunOutput.String(), want) {
			t.Errorf("terminal cockpit dry-run omitted %q:\n%s", want, dryRunOutput.String())
		}
	}
	if _, err := os.Stat(externalLog); !os.IsNotExist(err) {
		t.Fatalf("terminal cockpit dry-run executed an external command: %v", err)
	}

	install := NewRootCommand()
	var installOutput bytes.Buffer
	install.SetOut(&installOutput)
	install.SetErr(&installOutput)
	install.SetArgs(append([]string{"install", "--yes"}, args...))
	if err := install.Execute(); err != nil {
		t.Fatalf("terminal cockpit sandbox install failed: %v\noutput:\n%s", err, installOutput.String())
	}

	assertInstalledContains(t, filepath.Join(home, ".config", "herdr", "config.toml"), []string{
		`key = "prefix+t"`,
		`rmarganti.herdr-pluck.pluck`,
		`key = "prefix+alt+f"`,
		`exec "$@" "$selection"`,
	})
	assertInstalledContains(t, filepath.Join(home, ".config", "starship.toml"), []string{
		`$git_status`,
		`success_symbol = "[❯](bold fg:green)"`,
		`error_symbol = "[❯](bold fg:red)"`,
	})
	assertInstalledContains(t, filepath.Join(home, ".config", "atuin", "config.toml"), []string{
		`workspaces = true`,
		`filter_mode = "workspace"`,
		`filter_mode_shell_up_key_binding = "session"`,
		`enter_accept = true`,
	})
	installedZsh := filepath.Join(home, ".config", "dots", "zsh", "zshrc")
	assertInstalledContains(t, installedZsh, []string{`rc.d/post/*.zsh`})
	resolvedZsh, err := filepath.EvalSymlinks(installedZsh)
	if err != nil {
		t.Fatalf("resolve installed Zsh Source of Truth: %v", err)
	}
	assertInstalledContains(t, filepath.Join(filepath.Dir(resolvedZsh), "rc.d", "post", "40-tools.zsh"), []string{
		`FZF_CTRL_R_COMMAND= eval`,
		`bindkey '^X^E' edit-command-line`,
		`zoxide init zsh`,
		`atuin init zsh`,
	})

	logBytes, err := os.ReadFile(externalLog)
	if err != nil {
		t.Fatalf("read sandbox external-command log: %v", err)
	}
	log := string(logBytes)
	for _, want := range []string{
		"szrenwei/herdr-space-tab-metadata",
		"yersonargotev/herdr-tab-git",
		"yersonargotev/tabby",
		"rmarganti/herdr-pluck",
		"zsh:-c",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("sandbox provisioning log omitted %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "brew:") {
		t.Fatalf("sandbox install reached a package manager:\n%s", log)
	}
	if entries, err := os.ReadDir(fakeRealHome); err != nil || len(entries) != 0 {
		t.Fatalf("terminal cockpit install touched inherited HOME %q: entries=%v err=%v", fakeRealHome, entries, err)
	}
}

func assertInstalledContains(t *testing.T, path string, wants []string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read installed terminal cockpit file %s: %v", path, err)
	}
	for _, want := range wants {
		if !strings.Contains(string(content), want) {
			t.Errorf("installed file %s omitted %q", path, want)
		}
	}
}
