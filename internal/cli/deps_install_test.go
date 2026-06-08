package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestDepsInstallWithoutDryRunOrYesErrorsClearly(t *testing.T) {
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "install"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want unsupported install error")
	}
	if !strings.Contains(err.Error(), "--dry-run") || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %q, want --dry-run or --yes guidance", err.Error())
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

func TestDepsInstallDryRunRendersPreview(t *testing.T) {
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

	got := out.String()
	for _, want := range []string{
		`Dependency install dry-run for profile "default" (homebrew)`,
		"would-install definitely-missing-starship",
		"brew install starship",
		"Summary: 1 dependency to install",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q\noutput:\n%s", want, got)
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
		`Dependency install for profile "default" (homebrew)`,
		"installed",
		"definitely-missing-starship",
		"Summary: 1 installed, 0 manual, 0 unresolved, 0 failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install output missing %q\noutput:\n%s", want, got)
		}
	}
}

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
