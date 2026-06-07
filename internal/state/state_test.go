package state_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/state"
)

func TestLoadReturnsEmptyMetadataWhenFileAbsent(t *testing.T) {
	stateRoot := t.TempDir()

	got, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for absent file", err)
	}
	if len(got.Entries) != 0 {
		t.Fatalf("Load() entries = %d, want 0 for absent file", len(got.Entries))
	}
}

func TestSaveThenLoadRoundTripsRecords(t *testing.T) {
	stateRoot := t.TempDir()
	path := state.Path(stateRoot)

	want := state.Metadata{
		Version: 1,
		Entries: []state.Record{{
			Target:      "/home/user/.zshrc",
			Source:      "configs/zsh/zshrc",
			Strategy:    "symlink",
			Hash:        "abc123",
			InstalledAt: "2026-06-06T00:00:00Z",
		}},
	}

	if err := state.Save(path, want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Load() entries = %d, want 1", len(got.Entries))
	}
	if got.Entries[0] != want.Entries[0] {
		t.Fatalf("Load() record = %+v, want %+v", got.Entries[0], want.Entries[0])
	}
}

func TestFindByTargetReturnsRecordWhenPresent(t *testing.T) {
	meta := state.Metadata{Entries: []state.Record{
		{Target: "/home/user/.zshrc", Strategy: "symlink"},
		{Target: "/home/user/.gitconfig", Strategy: "copy"},
	}}

	got, ok := meta.FindByTarget("/home/user/.gitconfig")
	if !ok {
		t.Fatal("FindByTarget() ok = false, want true for present target")
	}
	if got.Strategy != "copy" {
		t.Fatalf("FindByTarget() strategy = %q, want copy", got.Strategy)
	}

	if _, ok := meta.FindByTarget("/home/user/.missing"); ok {
		t.Fatal("FindByTarget() ok = true, want false for absent target")
	}
}

func TestHashFileIsStableAndContentSensitive(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("same\n"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, []byte("same\n"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	ha, err := state.HashFile(a)
	if err != nil {
		t.Fatalf("HashFile(a) error = %v", err)
	}
	hb, err := state.HashFile(b)
	if err != nil {
		t.Fatalf("HashFile(b) error = %v", err)
	}
	if ha != hb {
		t.Fatalf("HashFile equal content mismatch: %q vs %q", ha, hb)
	}

	if err := os.WriteFile(b, []byte("different\n"), 0o600); err != nil {
		t.Fatalf("rewrite b: %v", err)
	}
	hb2, err := state.HashFile(b)
	if err != nil {
		t.Fatalf("HashFile(b) error = %v", err)
	}
	if ha == hb2 {
		t.Fatal("HashFile() returned same hash for different content")
	}
}
