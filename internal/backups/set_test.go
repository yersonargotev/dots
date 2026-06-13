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

func TestCreateSetPreservesDirectoryRecursively(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(filepath.Join(target, "plugin"), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "plugin", "local.lua"), []byte("-- local\n"), 0o600); err != nil {
		t.Fatalf("write nested file: %v", err)
	}

	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}

	preserved := filepath.Join(FilePath(stateRoot, set.ID, 1, target), "plugin", "local.lua")
	got, err := os.ReadFile(preserved)
	if err != nil {
		t.Fatalf("read preserved nested file: %v", err)
	}
	if string(got) != "-- local\n" {
		t.Fatalf("preserved nested file = %q, want local content", got)
	}
}

func TestCreateSetPreservesReadOnlyDirectoryModesAfterCopyingChildren(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	t.Cleanup(func() {
		makeTreeWritableForCleanup(stateRoot)
		makeTreeWritableForCleanup(home)
	})
	target := filepath.Join(home, ".config", "readonly")
	nested := filepath.Join(target, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "config.lua"), []byte("-- readonly\n"), 0o600); err != nil {
		t.Fatalf("write nested file: %v", err)
	}
	if err := os.Chmod(nested, 0o555); err != nil {
		t.Fatalf("chmod nested readonly: %v", err)
	}
	if err := os.Chmod(target, 0o555); err != nil {
		t.Fatalf("chmod target readonly: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(nested, 0o755)
		_ = os.Chmod(target, 0o755)
	})

	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}

	preservedDir := FilePath(stateRoot, set.ID, 1, target)
	preservedNested := filepath.Join(preservedDir, "nested")
	preservedFile := filepath.Join(preservedNested, "config.lua")
	got, err := os.ReadFile(preservedFile)
	if err != nil {
		t.Fatalf("read preserved nested file: %v", err)
	}
	if string(got) != "-- readonly\n" {
		t.Fatalf("preserved nested file = %q, want readonly content", got)
	}

	for _, tt := range []struct {
		name string
		path string
	}{
		{name: "root", path: preservedDir},
		{name: "nested", path: preservedNested},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info, err := os.Stat(tt.path)
			if err != nil {
				t.Fatalf("stat preserved dir: %v", err)
			}
			if got := info.Mode().Perm(); got != 0o555 {
				t.Fatalf("mode = %v, want 0555", got)
			}
		})
	}

	if err := os.Chmod(nested, 0o755); err != nil {
		t.Fatalf("chmod source nested writable before restore: %v", err)
	}
	if err := os.Chmod(target, 0o755); err != nil {
		t.Fatalf("chmod source target writable before restore: %v", err)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove source before restore: %v", err)
	}

	items, err := PlanRestore(stateRoot, set)
	if err != nil {
		t.Fatalf("PlanRestore() error = %v", err)
	}
	if err := applyRestore(items); err != nil {
		t.Fatalf("applyRestore() error = %v", err)
	}
	restoredFile := filepath.Join(nested, "config.lua")
	restored, err := os.ReadFile(restoredFile)
	if err != nil {
		t.Fatalf("read restored nested file: %v", err)
	}
	if string(restored) != "-- readonly\n" {
		t.Fatalf("restored nested file = %q, want readonly content", restored)
	}
	for _, tt := range []struct {
		name string
		path string
	}{
		{name: "root", path: target},
		{name: "nested", path: nested},
	} {
		t.Run("restore_"+tt.name, func(t *testing.T) {
			info, err := os.Stat(tt.path)
			if err != nil {
				t.Fatalf("stat restored dir: %v", err)
			}
			if got := info.Mode().Perm(); got != 0o555 {
				t.Fatalf("mode = %v, want 0555", got)
			}
		})
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
	if err := applyRestore(items); err != nil {
		t.Fatalf("applyRestore() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(got) != "original\n" {
		t.Fatalf("restored content = %q, want original", got)
	}
}

func TestApplyRestoreReturnsDirectoryToPreservedContent(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(filepath.Join(target, "plugin"), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	original := filepath.Join(target, "plugin", "local.lua")
	if err := os.WriteFile(original, []byte("-- original\n"), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}

	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove target dir: %v", err)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir drifted target dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, "drifted.lua"), []byte("-- drifted\n"), 0o600); err != nil {
		t.Fatalf("write drifted: %v", err)
	}

	items, err := PlanRestore(stateRoot, set)
	if err != nil {
		t.Fatalf("PlanRestore() error = %v", err)
	}
	if err := applyRestore(items); err != nil {
		t.Fatalf("applyRestore() error = %v", err)
	}

	got, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("read restored nested file: %v", err)
	}
	if string(got) != "-- original\n" {
		t.Fatalf("restored nested file = %q, want original", got)
	}
	if _, err := os.Lstat(filepath.Join(target, "drifted.lua")); !os.IsNotExist(err) {
		t.Fatalf("drifted file still exists after directory restore; lstat err = %v", err)
	}
}

func TestApplyRestoreReplacesNonWritableDirectoryTreeWithPreservedDirectory(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, ".config", "nvim")
	original := filepath.Join(target, "plugin", "local.lua")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatalf("mkdir original dir: %v", err)
	}
	if err := os.WriteFile(original, []byte("-- original\n"), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}

	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("remove target dir: %v", err)
	}
	driftedDir := filepath.Join(target, "after", "locked")
	if err := os.MkdirAll(driftedDir, 0o755); err != nil {
		t.Fatalf("mkdir drifted dir: %v", err)
	}
	driftedFile := filepath.Join(driftedDir, "drifted.lua")
	if err := os.WriteFile(driftedFile, []byte("-- drifted\n"), 0o400); err != nil {
		t.Fatalf("write drifted file: %v", err)
	}
	if err := os.Chmod(driftedDir, 0o555); err != nil {
		t.Fatalf("chmod drifted dir: %v", err)
	}
	t.Cleanup(func() {
		makeTreeWritableForCleanup(stateRoot)
		makeTreeWritableForCleanup(home)
	})

	items, err := PlanRestore(stateRoot, set)
	if err != nil {
		t.Fatalf("PlanRestore() error = %v", err)
	}
	if err := applyRestore(items); err != nil {
		t.Fatalf("applyRestore() error = %v", err)
	}

	got, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("read restored nested file: %v", err)
	}
	if string(got) != "-- original\n" {
		t.Fatalf("restored nested file = %q, want original", got)
	}
	if _, err := os.Lstat(driftedFile); !os.IsNotExist(err) {
		t.Fatalf("drifted file still exists after directory restore; lstat err = %v", err)
	}
}

func makeTreeWritableForCleanup(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			_ = os.Chmod(path, 0o755)
		}
		return nil
	})
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
	err = applyRestore(items)
	if err == nil {
		t.Fatal("applyRestore() error = nil, want directory refusal")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("error %q does not mention directory", err)
	}
	if _, statErr := os.Stat(canary); statErr != nil {
		t.Fatalf("directory tree was destroyed: %v", statErr)
	}
}

func TestRestoreRecordsSafetyBackupBeforeOverwriting(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, ".zshrc")
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

	result, err := Restore(stateRoot, set, RestoreOptions{Machine: "m", Repo: "r"})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if result.SafetySet == nil {
		t.Fatal("Restore() recorded no safety Backup Set for an overwrite")
	}
	if result.SafetySet.Reason != RestoreSafetyReason {
		t.Fatalf("safety reason = %q, want %q", result.SafetySet.Reason, RestoreSafetyReason)
	}

	// Target is restored.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "original\n" {
		t.Fatalf("restored content = %q, want original", got)
	}
	// The safety set preserved the drifted content, so the restore is reversible.
	preserved, err := os.ReadFile(FilePath(stateRoot, result.SafetySet.ID, 1, target))
	if err != nil {
		t.Fatalf("read safety preserved: %v", err)
	}
	if string(preserved) != "drifted\n" {
		t.Fatalf("safety preserved %q, want drifted", preserved)
	}
}

func TestRestoreSkipsSafetyBackupWhenNothingOverwritten(t *testing.T) {
	stateRoot := t.TempDir()
	home := t.TempDir()
	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	set, err := CreateSet(stateRoot, []string{target}, CreateOptions{Reason: "x"})
	if err != nil {
		t.Fatalf("CreateSet() error = %v", err)
	}
	if err := os.Remove(target); err != nil { // absent -> create, not overwrite
		t.Fatalf("remove target: %v", err)
	}

	result, err := Restore(stateRoot, set, RestoreOptions{})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if result.SafetySet != nil {
		t.Fatalf("Restore() recorded an unnecessary safety Backup Set: %s", result.SafetySet.ID)
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
	if err := applyRestore(items); err != nil {
		t.Fatalf("applyRestore() error = %v", err)
	}

	dest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink restored target: %v", err)
	}
	if dest != "/dev/null" {
		t.Fatalf("restored symlink dest = %q, want /dev/null", dest)
	}
}
