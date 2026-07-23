package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestDepsInstallDeclineDoesNotInvokePackageManager(t *testing.T) {
	binDir := t.TempDir()
	marker := binDir + "/brew-was-run"
	brew := binDir + "/brew"
	if err := os.WriteFile(brew, []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-starship
        command: definitely-missing-starship-probe
        brew: starship
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetArgs([]string{"deps", "install", "--file", manifestPath, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("declined install invoked fake package manager; marker stat err = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`Dependency install preview for profile "default" (tags: core) (homebrew)`,
		"would-install definitely-missing-starship",
		"brew install starship",
		"Proceed with dependency installation? [y/N]",
		"Dependency installation cancelled.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("decline output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestDepsInstallRejectsUnexpectedTrailingToken(t *testing.T) {
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "install", "--dry-run", "--", "starship"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want unexpected trailing token error")
	}
	if !strings.Contains(err.Error(), "starship") {
		t.Fatalf("error = %q, want trailing token to be rejected", err.Error())
	}
}

func TestDepsInstallInvalidOrEmptyConfirmationCancelsDeterministically(t *testing.T) {
	for _, tt := range []struct {
		name       string
		stdin      string
		wantOutput string
	}{
		{name: "invalid", stdin: "maybe\n", wantOutput: `Response "maybe" is not yes/y; cancelling.`},
		{name: "empty", stdin: "\n", wantOutput: "Dependency installation cancelled."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			marker := binDir + "/brew-was-run"
			brew := binDir + "/brew"
			if err := os.WriteFile(brew, []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0o700); err != nil {
				t.Fatalf("write fake brew: %v", err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-starship
        command: definitely-missing-starship-probe
        brew: starship
`)

			cmd := cli.NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetIn(strings.NewReader(tt.stdin))
			cmd.SetArgs([]string{"deps", "install", "--file", manifestPath, "--tier", "homebrew"})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("confirmation %q invoked fake package manager; marker stat err = %v", tt.stdin, err)
			}
			if got := out.String(); !strings.Contains(got, tt.wantOutput) {
				t.Fatalf("output missing %q\noutput:\n%s", tt.wantOutput, got)
			}
		})
	}
}

func TestDepsInstallWithNoMissingActionsDoesNotPrompt(t *testing.T) {
	binDir := t.TempDir()
	present := binDir + "/present-tool"
	if err := os.WriteFile(present, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake present command: %v", err)
	}
	t.Setenv("PATH", binDir)

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: present-tool
        command: present-tool
        brew: present-tool
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"deps", "install", "--file", manifestPath, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	got := out.String()
	if strings.Contains(got, "Proceed with dependency installation?") {
		t.Fatalf("output prompted despite no missing actions:\n%s", got)
	}
	if !strings.Contains(got, "All declared dependencies are already installed.") {
		t.Fatalf("output missing all-installed message:\n%s", got)
	}
}

func TestDepsInstallManualOnlyPreviewDoesNotPromptOrError(t *testing.T) {
	binDir := t.TempDir()
	brew := binDir + "/brew"
	marker := binDir + "/brew-was-run"
	if err := os.WriteFile(brew, []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", binDir)

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-manual
        command: definitely-missing-manual-probe
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"deps", "install", "--file", manifestPath, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("manual-only preview invoked fake package manager; marker stat err = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`Dependency install preview for profile "default" (tags: core) (homebrew)`,
		`manual        definitely-missing-manual no homebrew package declared for "definitely-missing-manual"; install it manually`,
		"Summary: 0 installable, 1 manual",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("manual-only output missing %q\noutput:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{
		"Proceed with dependency installation?",
		"dependency to install",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("manual-only output contained %q\noutput:\n%s", unwanted, got)
		}
	}
}

func TestDepsInstallMixedManualAndInstallableExplainsManualBeforeConfirmation(t *testing.T) {
	binDir := t.TempDir()
	argsLog := binDir + "/brew-args"
	probe := binDir + "/definitely-missing-starship-probe"
	brew := binDir + "/brew"
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
cat > %q <<'PROBE'
#!/bin/sh
exit 0
PROBE
chmod +x %q
`, argsLog, probe, probe)
	if err := os.WriteFile(brew, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-manual
        command: definitely-missing-manual-probe
      - name: definitely-missing-starship
        command: definitely-missing-starship-probe
        brew: starship
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"deps", "install", "--file", manifestPath, "--tier", "homebrew"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want unresolved manual dependency error\noutput:\n%s", out.String())
	}
	if !strings.Contains(err.Error(), "unresolved required dependencies remain") {
		t.Fatalf("Execute() error = %v, want unresolved required dependencies remain", err)
	}

	args, readErr := os.ReadFile(argsLog)
	if readErr != nil {
		t.Fatalf("read fake brew args: %v", readErr)
	}
	if string(args) != "install\nstarship\n" {
		t.Fatalf("fake brew args = %q, want only installable dependency", string(args))
	}

	got := out.String()
	manualPreview := `manual        definitely-missing-manual no homebrew package declared for "definitely-missing-manual"; install it manually`
	for _, want := range []string{
		manualPreview,
		"Proceed with dependency installation? [y/N]",
		"installed  definitely-missing-starship",
		"manual     definitely-missing-manual",
		"Summary: 1 installed, 1 manual, 0 unresolved, 0 failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("mixed install output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestDepsInstallExplainsRustupToolchainWhenProbesRemainMissing(t *testing.T) {
	binDir := t.TempDir()
	runLog := binDir + "/rustup-args"
	rustup := binDir + "/rustup"
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' \"$@\" > %q\nexit 0\n", runLog)
	if err := os.WriteFile(rustup, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake rustup: %v", err)
	}
	t.Setenv("PATH", binDir)

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
dependencies:
  - tags: [core]
    dependencies:
      - name: Rust stable (rustup)
        commands: [rustup, rustc, cargo]
        toolchain: rust-stable-rustup
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "install", "--yes", "--file", manifestPath, "--tier", "generic"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want unresolved Rust dependency error\noutput:\n%s", out.String())
	}
	args, readErr := os.ReadFile(runLog)
	if readErr != nil {
		t.Fatalf("read rustup args: %v", readErr)
	}
	if string(args) != "default\nstable\n" {
		t.Fatalf("rustup args = %q, want default/stable", string(args))
	}
	got := out.String()
	for _, want := range []string{
		"unresolved Rust stable (rustup)",
		"repair",
		"rustc, cargo are not available on PATH",
		"~/.cargo/bin",
		"rustup which rustc",
		"rustup which cargo",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Rust unresolved output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestDepsInstallDryRunRendersPreview(t *testing.T) {
	binDir := t.TempDir()
	brew := binDir + "/brew"
	if err := os.WriteFile(brew, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-starship
        command: definitely-missing-starship-probe
        brew: starship
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"deps", "install", "--dry-run", "--file", manifestPath, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`Dependency install dry-run for profile "default" (tags: core) (homebrew)`,
		"would-install definitely-missing-starship",
		"brew install starship",
		"Summary: 1 installable, 0 manual",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q\noutput:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Proceed with dependency installation?") {
		t.Fatalf("dry-run prompted unexpectedly:\n%s", got)
	}
}

func TestDepsInstallConfirmYesExecutesFakePackageManagerAndRendersSummary(t *testing.T) {
	binDir := t.TempDir()
	argsLog := binDir + "/brew-args"
	probe := binDir + "/definitely-missing-starship-probe"
	brew := binDir + "/brew"
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
printf 'package stdout\n'
printf 'package stderr\n' >&2
cat > %q <<'PROBE'
#!/bin/sh
exit 0
PROBE
chmod +x %q
`, argsLog, probe, probe)
	if err := os.WriteFile(brew, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-starship
        command: definitely-missing-starship-probe
        brew: starship
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetArgs([]string{"deps", "install", "--file", manifestPath, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read fake brew args: %v", err)
	}
	if string(args) != "install\nstarship\n" {
		t.Fatalf("fake brew args = %q, want direct argv install/starship", string(args))
	}

	got := out.String()
	for _, want := range []string{
		`Dependency install preview for profile "default" (tags: core) (homebrew)`,
		"Proceed with dependency installation? [y/N]",
		"package stdout",
		"package stderr",
		`Dependency install for profile "default" (tags: core) (homebrew)`,
		"Summary: 1 installed, 0 manual, 0 unresolved, 0 failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("confirmed install output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestDepsInstallYesExecutesFakePackageManagerAndRendersSummary(t *testing.T) {
	binDir := t.TempDir()
	argsLog := binDir + "/brew-args"
	stdinLog := binDir + "/brew-stdin"
	probe := binDir + "/definitely-missing-starship-probe"
	brew := binDir + "/brew"
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q
cat > %q
printf 'package stdout\n'
printf 'package stderr\n' >&2
cat > %q <<'PROBE'
#!/bin/sh
exit 0
PROBE
chmod +x %q
`, argsLog, stdinLog, probe, probe)
	if err := os.WriteFile(brew, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-starship
        command: definitely-missing-starship-probe
        brew: starship
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("runner stdin\n"))
	cmd.SetArgs([]string{"deps", "install", "--yes", "--file", manifestPath, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatalf("read fake brew args: %v", err)
	}
	if string(args) != "install\nstarship\n" {
		t.Fatalf("fake brew args = %q, want direct argv install/starship", string(args))
	}
	stdin, err := os.ReadFile(stdinLog)
	if err != nil {
		t.Fatalf("read fake brew stdin: %v", err)
	}
	if string(stdin) != "runner stdin\n" {
		t.Fatalf("fake brew stdin = %q, want cobra stdin passthrough", string(stdin))
	}

	got := out.String()
	for _, want := range []string{
		"package stdout",
		"package stderr",
		`Dependency install for profile "default" (tags: core) (homebrew)`,
		"installed",
		"definitely-missing-starship",
		"Summary: 1 installed, 0 manual, 0 unresolved, 0 failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install output missing %q\noutput:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Proceed with dependency installation?") {
		t.Fatalf("--yes prompted unexpectedly:\n%s", got)
	}
}

func TestDepsInstallDryRunClassifiesMissingFNMToolchainLikePlan(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)
	manifestPath := writeDepsInstallManifest(t, fnmBootstrapDepsManifest)

	planCmd := cli.NewRootCommand()
	var planOut bytes.Buffer
	planCmd.SetOut(&planOut)
	planCmd.SetErr(&planOut)
	planCmd.SetArgs([]string{"deps", "plan", "--profile", "default", "--file", manifestPath, "--tier", "generic"})
	if err := planCmd.Execute(); err == nil {
		t.Fatalf("deps plan error = nil, want findings exit")
	}

	installCmd := cli.NewRootCommand()
	var installOut bytes.Buffer
	installCmd.SetOut(&installOut)
	installCmd.SetErr(&installOut)
	installCmd.SetArgs([]string{"deps", "install", "--dry-run", "--file", manifestPath, "--tier", "generic"})
	if err := installCmd.Execute(); err != nil {
		t.Fatalf("deps install --dry-run error = %v\noutput:\n%s", err, installOut.String())
	}

	if strings.Contains(planOut.String(), "would-install") || strings.Contains(planOut.String(), "fnm install --lts") {
		t.Fatalf("deps plan should not classify missing fnm bootstrap as executable:\n%s", planOut.String())
	}
	got := installOut.String()
	if !strings.Contains(got, "manual") || strings.Contains(got, "would-install Node LTS (fnm)") {
		t.Fatalf("deps install --dry-run should match deps plan manual classification:\n%s", got)
	}
}

func TestDepsInstallYesDoesNotRunFNMBootstrapWhenFNMIsMissing(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)
	manifestPath := writeDepsInstallManifest(t, fnmBootstrapDepsManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "install", "--yes", "--file", manifestPath, "--tier", "generic"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want missing fnm to remain a non-executed manual action\noutput:\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "manual") || strings.Contains(got, `exec: "fnm"`) {
		t.Fatalf("deps install --yes should not execute missing fnm bootstrap:\n%s", got)
	}
}

const fnmBootstrapDepsManifest = `version: 1
profiles:
  default:
    tags: [core]
dependencies:
  - tags: [core]
    dependencies:
      - name: Node LTS (fnm)
        commands: [fnm, node]
        brew: fnm
        linux_homebrew: true
        toolchain: node-lts-fnm
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`

func writeDepsInstallManifest(t *testing.T, content string) string {
	t.Helper()
	path := t.TempDir() + "/dots.yaml"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func TestDepsInstallDryRunDoesNotInvokePackageManager(t *testing.T) {
	binDir := t.TempDir()
	marker := binDir + "/brew-was-run"
	brew := binDir + "/brew"
	if err := os.WriteFile(brew, []byte("#!/bin/sh\ntouch \""+marker+"\"\n"), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-starship
        command: definitely-missing-starship-probe
        brew: starship
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "install", "--dry-run", "--file", manifestPath, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("dry-run invoked fake package manager; marker stat err = %v", err)
	}
}

func TestDepsInstallDryRunAcceptsSandboxHomeAndStateRoot(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	for _, dir := range []string{filepath.Join(home, "Library", "Fonts"), filepath.Join(home, ".local", "share", "fonts")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create sandbox font dir: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, "Library", "Fonts", "SandboxFont-Regular.ttf"), []byte("font"), 0o600); err != nil {
		t.Fatalf("write sandbox darwin font: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".local", "share", "fonts", "SandboxFont-Regular.ttf"), []byte("font"), 0o600); err != nil {
		t.Fatalf("write sandbox linux font: %v", err)
	}
	manifestPath := writeDepsInstallManifest(t, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: Sandbox Font
        brew_cask: font-sandbox
        font_match: SandboxFont*
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "install", "--dry-run", "--file", manifestPath, "--tier", "homebrew", "--home", home, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "dependencies.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run with sandbox state wrote dependency metadata; stat err = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "All declared dependencies are already installed.") {
		t.Fatalf("deps install did not use sandbox --home for font detection:\n%s", got)
	}
}
