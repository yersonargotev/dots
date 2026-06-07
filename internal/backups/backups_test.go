package backups

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathReturnsCentralizedBackupMetadataPath(t *testing.T) {
	stateRoot := filepath.Join("state", "dots")
	want := filepath.Join(stateRoot, "backups", "metadata.json")
	if got := Path(stateRoot); got != want {
		t.Fatalf("Path(%q) = %q, want %q", stateRoot, got, want)
	}
}

func TestLoadReturnsEmptyMetadataWhenBackupMetadataIsAbsent(t *testing.T) {
	meta, err := Load(Path(t.TempDir()))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(meta.Sets) != 0 {
		t.Fatalf("Load() sets = %v, want empty", meta.Sets)
	}
}

func TestLoadReturnsEmptyMetadataWhenBackupMetadataFileIsEmpty(t *testing.T) {
	stateRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Path(stateRoot)), 0o755); err != nil {
		t.Fatalf("create Backup Metadata directory: %v", err)
	}
	if err := os.WriteFile(Path(stateRoot), []byte("\n\t "), 0o600); err != nil {
		t.Fatalf("write empty Backup Metadata: %v", err)
	}

	meta, err := Load(Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(meta.Sets) != 0 {
		t.Fatalf("Load() sets = %v, want empty", meta.Sets)
	}
}

func TestLoadReadsBackupSets(t *testing.T) {
	stateRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(Path(stateRoot)), 0o755); err != nil {
		t.Fatalf("create Backup Metadata directory: %v", err)
	}
	content := []byte(`{
  "version": 1,
  "sets": [
    {
      "id": "backup-001",
      "createdAt": "2026-01-02T03:04:05Z",
      "reason": "pre-install conflict protection",
      "targets": ["/home/user/.zshrc"]
    }
  ]
}
`)
	if err := os.WriteFile(Path(stateRoot), content, 0o600); err != nil {
		t.Fatalf("write Backup Metadata: %v", err)
	}

	meta, err := Load(Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.Version != 1 {
		t.Fatalf("Version = %d, want 1", meta.Version)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("sets len = %d, want 1", len(meta.Sets))
	}
	set := meta.Sets[0]
	if set.ID != "backup-001" || set.CreatedAt != "2026-01-02T03:04:05Z" || set.Reason != "pre-install conflict protection" {
		t.Fatalf("Backup Set = %+v, want fixture fields", set)
	}
	if len(set.Targets) != 1 || set.Targets[0] != "/home/user/.zshrc" {
		t.Fatalf("targets = %v, want [/home/user/.zshrc]", set.Targets)
	}
}
