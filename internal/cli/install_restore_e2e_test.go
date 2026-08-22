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

func TestInstallReplaceAndBackupRestoreEndToEndUsesSandbox(t *testing.T) {
	realHomeSentinel := t.TempDir()
	t.Setenv("HOME", realHomeSentinel)
	if err := os.WriteFile(filepath.Join(realHomeSentinel, ".zshrc"), []byte("real home must stay untouched\n"), 0o600); err != nil {
		t.Fatalf("write real-home sentinel: %v", err)
	}

	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	sourceRel := "configs/zsh/zshrc"
	sourcePath := filepath.Join(sourceRoot, sourceRel)
	target := filepath.Join(home, ".zshrc")

	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source parent: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("managed zshrc\n"), 0o600); err != nil {
		t.Fatalf("write managed source: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(target, []byte("original user zshrc\n"), 0o600); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	installOut, err := runRootCommand(t, "r\n",
		"install",
		"--profile", "default",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
		"--no-tui",
	)
	if err != nil {
		t.Fatalf("install Execute() error = %v\n%s", err, installOut)
	}

	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata after install: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets after install = %d, want 1", len(meta.Sets))
	}
	installSet := meta.Sets[0]
	if installSet.Reason != "pre-install conflict protection" {
		t.Fatalf("install Backup Set reason = %q", installSet.Reason)
	}
	if len(installSet.Targets) != 1 || installSet.Targets[0] != target {
		t.Fatalf("install Backup Set targets = %#v, want [%q]", installSet.Targets, target)
	}
	preserved, err := os.ReadFile(backups.FilePath(stateRoot, installSet.ID, 1, target))
	if err != nil {
		t.Fatalf("read preserved install backup: %v", err)
	}
	if string(preserved) != "original user zshrc\n" {
		t.Fatalf("install backup preserved %q, want original user content", preserved)
	}

	linkDest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("installed target is not a symlink: %v", err)
	}
	if linkDest != sourcePath {
		t.Fatalf("installed symlink points to %q, want %q", linkDest, sourcePath)
	}

	restoreOut, err := runRootCommand(t, "",
		"backups", "restore", installSet.ID,
		"--home", home,
		"--state-root", stateRoot,
	)
	if err != nil {
		t.Fatalf("restore Execute() error = %v\n%s", err, restoreOut)
	}

	restored, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(restored) != "original user zshrc\n" {
		t.Fatalf("restored target = %q, want original user content", restored)
	}

	meta, err = backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata after restore: %v", err)
	}
	if len(meta.Sets) != 2 {
		t.Fatalf("Backup Sets after restore = %d, want install set + restore safety set", len(meta.Sets))
	}
	safetySet := meta.Sets[1]
	if safetySet.ID == installSet.ID {
		t.Fatalf("restore did not create a distinct safety Backup Set")
	}
	if safetySet.Reason != backups.RestoreSafetyReason {
		t.Fatalf("restore safety reason = %q, want %q", safetySet.Reason, backups.RestoreSafetyReason)
	}
	safetyPreserved := backups.FilePath(stateRoot, safetySet.ID, 1, target)
	safetyDest, err := os.Readlink(safetyPreserved)
	if err != nil {
		t.Fatalf("restore safety backup did not preserve current symlink: %v", err)
	}
	if safetyDest != sourcePath {
		t.Fatalf("restore safety backup symlink points to %q, want %q", safetyDest, sourcePath)
	}

	if got, err := os.ReadFile(filepath.Join(realHomeSentinel, ".zshrc")); err != nil {
		t.Fatalf("read real-home sentinel: %v", err)
	} else if string(got) != "real home must stay untouched\n" {
		t.Fatalf("real HOME sentinel changed to %q", got)
	}
	if _, err := os.Stat(filepath.Join(realHomeSentinel, ".local", "state", "dots")); !os.IsNotExist(err) {
		t.Fatalf("real HOME state root was touched, stat err = %v", err)
	}
}

func runRootCommand(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}
