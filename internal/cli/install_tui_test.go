package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/cli"
)

// TestInstallTUIDefaultReplacesConflict drives the default (Bubble Tea) conflict
// path through the real cobra command, feeding scripted keystrokes: 'r' selects
// replace and Enter applies. It proves the TUI is wired into install and still
// honors the backup safety guarantee.
func TestInstallTUIDefaultReplacesConflict(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "managed\n")
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
	}

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/git/gitconfig
    target: ~/.gitconfig
    strategy: copy
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// 'r' = replace the highlighted conflict, '\r' (Enter) = apply.
	cmd.SetIn(strings.NewReader("r\r"))
	cmd.SetArgs([]string{"install", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "managed\n" {
		t.Fatalf("target contents = %q, want managed source after replace", got)
	}
	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets = %d, want 1 (replace must back up the original)", len(meta.Sets))
	}
}

// TestInstallTUIDefaultCancelAppliesNothing proves ctrl+c in the TUI aborts the
// install without mutating the conflicting target.
func TestInstallTUIDefaultCancelAppliesNothing(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "managed\n")
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
	}

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/git/gitconfig
    target: ~/.gitconfig
    strategy: copy
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// 'r' selects replace, then ctrl+c (0x03) cancels before applying.
	cmd.SetIn(strings.NewReader("r\x03"))
	cmd.SetArgs([]string{"install", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "local\n" {
		t.Fatalf("target contents = %q, want untouched local content after cancel", got)
	}
	if !strings.Contains(out.String(), "canceled") {
		t.Fatalf("output missing cancel notice\noutput:\n%s", out.String())
	}
}
