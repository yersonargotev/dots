package state_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		Version:    2,
		Provenance: state.Provenance{SourceRoot: "/src/dots", SourceRevision: "abc123", DotsVersion: "v0.test", RecordedAt: "2026-06-06T00:00:00Z"},
		Entries: []state.Record{{
			Target:      "/home/user/.zshrc",
			Source:      "configs/zsh/zshrc",
			Strategy:    "symlink",
			Hash:        "abc123",
			InstalledAt: "2026-06-06T00:00:00Z",
			Profiles:    []string{"core"},
			Tags:        []string{"core"},
		}},
		Provisioners: []state.ProvisionerRecord{{
			Profile:    "core",
			Profiles:   []string{"core"},
			Tags:       []string{"core"},
			Tool:       "gentle-ai",
			Executable: "gentle-ai",
			Args:       []string{"install"},
			Status:     "provisioned",
			LastRunAt:  "2026-06-06T00:00:00Z",
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
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load() metadata = %+v, want %+v", got, want)
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

func TestHashFileRejectsDirectories(t *testing.T) {
	dir := t.TempDir()

	_, err := state.HashFile(dir)
	if err == nil {
		t.Fatal("HashFile() error = nil, want directory rejection")
	}
	if !strings.Contains(err.Error(), "directories are not supported") {
		t.Fatalf("HashFile() error = %q, want directory rejection", err)
	}
}
