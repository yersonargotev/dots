package backups

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilePathUsesIndexedBaseNameUnderSetDir(t *testing.T) {
	stateRoot := filepath.Join("state", "dots")
	got := FilePath(stateRoot, "backup-001", 2, "/home/user/.zshrc")
	want := filepath.Join(stateRoot, "backups", "backup-001", "files", "000002-.zshrc")
	if got != want {
		t.Fatalf("FilePath() = %q, want %q", got, want)
	}
}

func TestCreateSetPreservesFileAndRecordsProvenance(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("local\n"), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}

	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{
		Reason:  "pre-restore safety backup",
		Machine: "workstation-1",
		Repo:    "/src/dots",
	})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}
	if set.Machine != "workstation-1" || set.Repo != "/src/dots" || set.Reason != "pre-restore safety backup" {
		t.Fatalf("provenance not recorded: %+v", set)
	}

	preserved, err := os.ReadFile(FilePath(stateRoot, set.ID, 1, target))
	if err != nil {
		t.Fatalf("read preserved file: %v", err)
	}
	if string(preserved) != "local\n" {
		t.Fatalf("preserved content = %q, want local", preserved)
	}

	meta, err := Load(Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.Version != metadataVersion {
		t.Fatalf("Version = %d, want %d", meta.Version, metadataVersion)
	}
	if _, ok := meta.FindSet(set.ID); !ok {
		t.Fatalf("created set %s not found in metadata", set.ID)
	}
}

func TestPlanRestoreClassifiesTargetsAgainstCurrentFilesystem(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	existing := filepath.Join(home, ".existing")
	absent := filepath.Join(home, "nested", ".absent")
	if err := os.WriteFile(existing, []byte("v1\n"), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(absent), 0o755); err != nil {
		t.Fatalf("mkdir absent parent: %v", err)
	}
	if err := os.WriteFile(absent, []byte("v1\n"), 0o600); err != nil {
		t.Fatalf("write absent (for backup): %v", err)
	}

	set, err := CreateSet(stateRoot, []string{existing, absent}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}
	// Remove one target so PlanRestore sees it as a create, keep the other.
	if err := os.Remove(absent); err != nil {
		t.Fatalf("remove absent: %v", err)
	}

	items, err := PlanRestore(stateRoot, set)
	if err != nil {
		t.Fatalf("PlanRestore() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Target != existing || items[0].Action != RestoreOverwrite {
		t.Fatalf("item[0] = %+v, want overwrite of existing", items[0])
	}
	if items[1].Target != absent || items[1].Action != RestoreCreate {
		t.Fatalf("item[1] = %+v, want create of absent", items[1])
	}
}

func TestPlanRestoreFailsWhenPreservedFileMissing(t *testing.T) {
	stateRoot := t.TempDir()
	set := BackupSet{ID: "backup-broken", Targets: []string{"/home/user/.zshrc"}}
	if _, err := PlanRestore(stateRoot, set); err == nil {
		t.Fatal("PlanRestore() error = nil, want missing preserved file error")
	}
}

func TestApplyRestoreReturnsTargetsToPreservedContent(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, "nested", ".zshrc")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}
	if err := os.WriteFile(target, []byte("drifted\n"), 0o600); err != nil {
		t.Fatalf("drift target: %v", err)
	}

	items, err := PlanRestore(stateRoot, set)
	if err != nil {
		t.Fatalf("PlanRestore() error = %v", err)
	}
	if err := ApplyRestore(items); err != nil {
		t.Fatalf("ApplyRestore() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(got) != "original\n" {
		t.Fatalf("restored content = %q, want original", got)
	}
}

func TestApplyRestoreRefusesDirectoryTarget(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, ".config")
	if err := os.WriteFile(target, []byte("was a file\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}

	// The target is now a non-empty directory; restoring a preserved file must
	// never recursively delete it.
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove file target: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(target, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir dir target: %v", err)
	}
	canary := filepath.Join(target, "nested", "keep")
	if err := os.WriteFile(canary, []byte("important\n"), 0o600); err != nil {
		t.Fatalf("write canary: %v", err)
	}

	items, err := PlanRestore(stateRoot, set)
	if err != nil {
		t.Fatalf("PlanRestore() error = %v", err)
	}
	err = ApplyRestore(items)
	if err == nil {
		t.Fatal("ApplyRestore() error = nil, want directory refusal")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("error %q does not mention directory", err)
	}
	if _, statErr := os.Stat(canary); statErr != nil {
		t.Fatalf("directory tree was destroyed: %v", statErr)
	}
}

func TestApplyRestoreRecreatesSymlinkTarget(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, ".link")
	if err := os.Symlink("/dev/null", target); err != nil {
		t.Fatalf("symlink target: %v", err)
	}

	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove target: %v", err)
	}

	items, err := PlanRestore(stateRoot, set)
	if err != nil {
		t.Fatalf("PlanRestore() error = %v", err)
	}
	if err := ApplyRestore(items); err != nil {
		t.Fatalf("ApplyRestore() error = %v", err)
	}

	dest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink restored target: %v", err)
	}
	if dest != "/dev/null" {
		t.Fatalf("restored symlink dest = %q, want /dev/null", dest)
	}
}
