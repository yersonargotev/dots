package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

// fontManifest declares a single profile whose only dependency is a font
// detected by scanning the per-user font directories. The glob is unique enough
// that no system font directory satisfies it, so detection hinges entirely on
// which home roots the per-user font directories.
const fontManifest = `version: 1
profiles:
  default:
    tags: [core]
    dependencies:
      - name: Dots Home Flag Probe Font
        brew_cask: font-dots-home-flag-probe
        font_match: "DotsHomeFlagProbeNF*"
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`

const darwinAppManifest = `version: 1
profiles:
  default:
    tags: [desktop]
    dependencies:
      - name: Dots Sandbox App
        command: dots-sandbox-app
        darwin_app: DotsSandbox.app
        brew_cask: dots-sandbox-app
entries:
  - source: configs/ghostty/config.ghostty
    target: ~/.config/ghostty/config.ghostty
    strategy: symlink
    tags: [desktop]
`

// writeProbeFont drops a file matching the probe glob in every per-user font
// directory dots scans, so the test stays OS-agnostic across darwin and linux.
func writeProbeFont(t *testing.T, home string) {
	t.Helper()
	for _, dir := range []string{
		filepath.Join(home, "Library", "Fonts"),
		filepath.Join(home, ".local", "share", "fonts"),
		filepath.Join(home, ".fonts"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create font dir %q: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "DotsHomeFlagProbeNF-Regular.ttf"), []byte("font"), 0o600); err != nil {
			t.Fatalf("write probe font: %v", err)
		}
	}
}

func TestDepsCheckHomeFlagDetectsSandboxFont(t *testing.T) {
	// The real home stays empty; the font only exists under the sandbox home
	// passed via --home, so a "present" result proves detection honored the flag.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	sandbox := t.TempDir()
	writeProbeFont(t, sandbox)
	manifestPath := writeCLIManifest(t, t.TempDir(), fontManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "check", "--profile", "default", "--file", manifestPath, "--home", sandbox})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "present  Dots Home Flag Probe Font") {
		t.Fatalf("sandbox font not detected under --home\noutput:\n%s", got)
	}
	if !strings.Contains(got, "Summary: 1 present, 0 missing") {
		t.Fatalf("unexpected summary\noutput:\n%s", got)
	}
}

func TestDepsCheckHomeFlagDetectsSandboxDarwinApp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS application detection is Darwin-only")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	sandbox := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sandbox, "Applications", "DotsSandbox.app"), 0o755); err != nil {
		t.Fatalf("create sandbox app bundle: %v", err)
	}
	manifestPath := writeCLIManifest(t, t.TempDir(), darwinAppManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "check", "--profile", "default", "--file", manifestPath, "--home", sandbox})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "present  Dots Sandbox App") {
		t.Fatalf("sandbox app not detected under --home\noutput:\n%s", out.String())
	}
}

func TestDepsCheckHomeFlagIgnoresRealHomeFont(t *testing.T) {
	// The font lives only in the real home; --home points at an empty sandbox.
	// A "missing" result proves --home overrides the real home rather than
	// dishonestly reporting the operator's own installed font.
	realHome := t.TempDir()
	writeProbeFont(t, realHome)
	t.Setenv("HOME", realHome)
	t.Setenv("PATH", t.TempDir())

	sandbox := t.TempDir()
	manifestPath := writeCLIManifest(t, t.TempDir(), fontManifest)

	// A missing dependency is a finding, so deps check exits with code 2.
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"deps", "check", "--profile", "default", "--file", manifestPath, "--home", sandbox}, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (findings)\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}

	got := out.String()
	if !strings.Contains(got, "missing  Dots Home Flag Probe Font") {
		t.Fatalf("--home sandbox font detection leaked into real home\noutput:\n%s", got)
	}
	if !strings.Contains(got, "Summary: 0 present, 1 missing") {
		t.Fatalf("unexpected summary\noutput:\n%s", got)
	}
}

func TestDepsPlanHomeFlagDetectsSandboxFont(t *testing.T) {
	// plan must route --home into the same font detection as check: a font under
	// the sandbox home counts as installed and drops out of the plan.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	sandbox := t.TempDir()
	writeProbeFont(t, sandbox)
	manifestPath := writeCLIManifest(t, t.TempDir(), fontManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "plan", "--profile", "default", "--file", manifestPath, "--home", sandbox, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, "All declared dependencies are already installed.") {
		t.Fatalf("plan did not honor --home for sandbox font detection\noutput:\n%s", got)
	}
}

func TestDepsPlanHomeFlagDetectsSandboxDarwinApp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS application detection is Darwin-only")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	sandbox := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sandbox, "Applications", "DotsSandbox.app"), 0o755); err != nil {
		t.Fatalf("create sandbox app bundle: %v", err)
	}
	manifestPath := writeCLIManifest(t, t.TempDir(), darwinAppManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "plan", "--profile", "default", "--file", manifestPath, "--home", sandbox, "--tier", "homebrew"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "All declared dependencies are already installed.") {
		t.Fatalf("plan did not honor --home for sandbox app detection\noutput:\n%s", out.String())
	}
}
