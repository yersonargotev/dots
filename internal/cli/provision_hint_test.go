package cli_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

const skippedHintManifest = `version: 1
profiles:
  default:
    tags: [core]
  desktop:
    tags: [core, desktop]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      persona: neutral
      agents: [codex]
    dependencies:
      - name: gentle-ai
      - name: engram
  - tool: claude
    tags: [desktop]
    os: [darwin, linux]
    spec:
      marketplace: ChromeDevTools/chrome-devtools-mcp
    dependencies:
      - name: claude
  - tool: codex
    tags: [desktop]
    os: [darwin, linux]
    spec:
      mcp: chrome-devtools
      command: [npx, -y, chrome-devtools-mcp@latest]
    dependencies:
      - name: codex
`

// TestInstallDryRunHintsSkippedDesktopProvisioners proves the default profile
// surfaces a one-line nudge naming the fuller profile that would include the
// desktop-only provisioners it silently skips.
func TestInstallDryRunHintsSkippedDesktopProvisioners(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, skippedHintManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--dry-run", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	want := `Note: profile "default" skips 2 provisioner(s); run with --profile desktop to include them.`
	if !strings.Contains(out.String(), want) {
		t.Fatalf("install --dry-run output missing skipped-provisioner hint %q\noutput:\n%s", want, out.String())
	}
}

// TestInstallDesktopProfileShowsNoSkippedHint proves the profile that already
// selects every provisioner stays quiet — no spurious nudge.
func TestInstallDesktopProfileShowsNoSkippedHint(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, skippedHintManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--dry-run", "--profile", "desktop", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if strings.Contains(out.String(), "skips") {
		t.Fatalf("desktop profile should not show a skipped-provisioner hint\noutput:\n%s", out.String())
	}
}

// TestUpdateDryRunHintsSkippedDesktopProvisioners proves the discoverability
// hint reaches the update path too, not just install.
func TestUpdateDryRunHintsSkippedDesktopProvisioners(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "managed\n",
		"dots.yaml":         skippedHintManifest,
	})

	out := runUpdate(t, "--dry-run", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot)

	want := `Note: profile "default" skips 2 provisioner(s); run with --profile desktop to include them.`
	if !strings.Contains(out, want) {
		t.Fatalf("update --dry-run output missing skipped-provisioner hint %q\noutput:\n%s", want, out)
	}
}
