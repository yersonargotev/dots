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

// newPreservedSet writes target with content, captures it into a Backup Set
// under a state root kept outside home, and returns the created set. The state
// root is deliberately separate from home so the test exercises restore without
// the in-home symlink validation that explicit external state roots skip.
func newPreservedSet(t *testing.T, home, stateRoot, name, content, machine string) backups.BackupSet {
	t.Helper()
	target := filepath.Join(home, name)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	set, err := backups.CreateSet(stateRoot, []string{target}, backups.CreateOptions{
		Reason:  "pre-install conflict protection",
		Machine: machine,
		Repo:    "/src/dots",
	})
	if err != nil {
		t.Fatalf("CreateSet: %v", err)
	}
	return set
}

func runRestore(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"backups", "restore"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestBackupsRestoreReturnsTargetToPreservedContent(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	set := newPreservedSet(t, home, stateRoot, ".zshrc", "original\n", "")

	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("drifted\n"), 0o600); err != nil {
		t.Fatalf("drift target: %v", err)
	}

	out, err := runRestore(t, set.ID, "--home", home, "--state-root", stateRoot)
	if err != nil {
		t.Fatalf("restore Execute() error = %v\n%s", err, out)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(got) != "original\n" {
		t.Fatalf("restored content = %q, want original", got)
	}
	if !strings.Contains(out, "Restored 1 target from Backup Set "+set.ID) {
		t.Fatalf("missing restore summary in output:\n%s", out)
	}
}

func TestBackupsRestoreDryRunChangesNothing(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	set := newPreservedSet(t, home, stateRoot, ".zshrc", "original\n", "")

	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("drifted\n"), 0o600); err != nil {
		t.Fatalf("drift target: %v", err)
	}

	out, err := runRestore(t, set.ID, "--home", home, "--state-root", stateRoot, "--dry-run")
	if err != nil {
		t.Fatalf("restore --dry-run Execute() error = %v\n%s", err, out)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "drifted\n" {
		t.Fatalf("dry run changed target content = %q, want drifted", got)
	}
	if !strings.Contains(out, "Dry run: no files changed.") {
		t.Fatalf("missing dry run notice in output:\n%s", out)
	}
	if !strings.Contains(out, "overwrite") || !strings.Contains(out, target) {
		t.Fatalf("dry run did not report planned overwrite:\n%s", out)
	}

	// A dry run must not record a safety Backup Set.
	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("dry run recorded extra Backup Sets: %d", len(meta.Sets))
	}
}

func TestBackupsRestoreRefusesForeignMachineWithoutForce(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	set := newPreservedSet(t, home, stateRoot, ".zshrc", "original\n", "some-other-machine")

	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("drifted\n"), 0o600); err != nil {
		t.Fatalf("drift target: %v", err)
	}

	out, err := runRestore(t, set.ID, "--home", home, "--state-root", stateRoot)
	if err == nil {
		t.Fatalf("restore Execute() error = nil, want foreign-machine refusal\n%s", out)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error %q does not mention --force", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "drifted\n" {
		t.Fatalf("refused restore still changed target = %q", got)
	}
}

func TestBackupsRestoreForceOverridesForeignMachine(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	set := newPreservedSet(t, home, stateRoot, ".zshrc", "original\n", "some-other-machine")

	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("drifted\n"), 0o600); err != nil {
		t.Fatalf("drift target: %v", err)
	}

	if _, err := runRestore(t, set.ID, "--home", home, "--state-root", stateRoot, "--force"); err != nil {
		t.Fatalf("restore --force Execute() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(got) != "original\n" {
		t.Fatalf("forced restore content = %q, want original", got)
	}
}

func TestBackupsRestoreBacksUpOverwrittenTargetFirst(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	set := newPreservedSet(t, home, stateRoot, ".zshrc", "original\n", "")

	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("drifted\n"), 0o600); err != nil {
		t.Fatalf("drift target: %v", err)
	}

	out, err := runRestore(t, set.ID, "--home", home, "--state-root", stateRoot)
	if err != nil {
		t.Fatalf("restore Execute() error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "pre-restore") && !strings.Contains(out, "before restoring") {
		t.Fatalf("missing safety backup notice in output:\n%s", out)
	}

	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(meta.Sets) != 2 {
		t.Fatalf("expected original + safety Backup Set, got %d", len(meta.Sets))
	}

	var safety backups.BackupSet
	for _, s := range meta.Sets {
		if s.ID != set.ID {
			safety = s
		}
	}
	if safety.Reason != "pre-restore safety backup" {
		t.Fatalf("safety set reason = %q, want pre-restore safety backup", safety.Reason)
	}
	preserved, err := os.ReadFile(backups.FilePath(stateRoot, safety.ID, 1, target))
	if err != nil {
		t.Fatalf("read safety preserved file: %v", err)
	}
	if string(preserved) != "drifted\n" {
		t.Fatalf("safety backup preserved %q, want the drifted content", preserved)
	}
}

func TestBackupsRestoreRejectsTargetOutsideHome(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir() // explicit external state root, outside home
	outside := t.TempDir()
	victim := filepath.Join(outside, ".victim")
	if err := os.WriteFile(victim, []byte("do not touch\n"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}

	// Hand-crafted metadata whose target escapes the home sandbox. Trusting the
	// external state root's location must not extend to trusting this target.
	if err := os.MkdirAll(filepath.Join(stateRoot, "backups", "backup-evil", "files"), 0o755); err != nil {
		t.Fatalf("mkdir set: %v", err)
	}
	preserved := backups.FilePath(stateRoot, "backup-evil", 1, victim)
	if err := os.WriteFile(preserved, []byte("payload\n"), 0o600); err != nil {
		t.Fatalf("write preserved: %v", err)
	}
	meta := backups.Metadata{Version: 1, Sets: []backups.BackupSet{{
		ID: "backup-evil", CreatedAt: "2026-06-08T10:00:00Z", Reason: "x", Targets: []string{victim},
	}}}
	if err := backups.Save(backups.Path(stateRoot), meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	out, err := runRestore(t, "backup-evil", "--home", home, "--state-root", stateRoot)
	if err == nil {
		t.Fatalf("restore Execute() error = nil, want out-of-home rejection\n%s", out)
	}
	if !strings.Contains(err.Error(), "escapes home") && !strings.Contains(err.Error(), "restore target") {
		t.Fatalf("error %q does not report the out-of-home target", err)
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}
	if string(got) != "do not touch\n" {
		t.Fatalf("out-of-home target was modified: %q", got)
	}
}

func TestBackupsRestoreUnknownSetReports(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()

	out, err := runRestore(t, "backup-missing", "--home", home, "--state-root", stateRoot)
	if err == nil {
		t.Fatalf("restore Execute() error = nil, want unknown-set error\n%s", out)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error %q does not report missing set", err)
	}
}
