package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestMetadataCommitRevalidatesCapturedSourceIdentity(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "theme.conf")
	target := filepath.Join(home, ".config", "theme.conf")
	if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{
		Source: "theme.conf", Target: target, Strategy: "copy", Status: plan.StatusCreate,
	}}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := ReleaseCapturedSources(captures); err != nil {
			t.Errorf("ReleaseCapturedSources() error = %v", err)
		}
	})
	opts.CapturedSources = captures
	commit, err := ApplyManagedEntries(p, opts)
	if err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed := state.InstalledSelection{ExtraTags: []string{"theme-carbonfox"}, ResolvedTags: []string{"theme-carbonfox"}}
	if err := commit.Commit(&installed); err == nil || !strings.Contains(err.Error(), "changed identity") {
		t.Fatalf("Commit() error = %v, want captured source identity rejection", err)
	}
	metadata, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.InstalledSelection != nil {
		t.Fatalf("InstalledSelection = %+v, want no terminal selection commit", metadata.InstalledSelection)
	}
}
