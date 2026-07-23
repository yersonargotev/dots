package selection_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
)

func TestResolvePreservesExplicitSelectionIntent(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"core":   {Tags: []string{"core", "shared"}},
		"agents": {Tags: []string{"agents", "shared"}},
	}}
	got, err := selection.Resolve(m, []string{"core", "agents", "core"}, []string{"shared", "web", "shared"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := state.InstalledSelection{
		Profiles:     []string{"core", "agents"},
		ExtraTags:    []string{"shared", "web"},
		ResolvedTags: []string{"core", "shared", "agents", "web"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %+v, want %+v", got, want)
	}
}

func TestRecordReloadsLatestInventoryBeforePersistingSelection(t *testing.T) {
	path := state.Path(t.TempDir())
	latest := state.Metadata{
		Version:      2,
		Entries:      []state.Record{{Target: "/home/user/.zshrc"}},
		Provisioners: []state.ProvisionerRecord{{Tool: "gentle-ai"}},
		InstalledSelection: &state.InstalledSelection{
			Profiles: []string{"old"},
		},
	}
	if err := state.Save(path, latest); err != nil {
		t.Fatalf("save latest metadata: %v", err)
	}
	want := state.InstalledSelection{
		Profiles:     []string{"core"},
		ResolvedTags: []string{"core"},
		Provenance:   state.Provenance{RecordedAt: "2026-07-23T12:00:00Z"},
	}

	if err := selection.Record(path, want); err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	got, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Version != state.CurrentVersion {
		t.Fatalf("metadata version = %d, want %d", got.Version, state.CurrentVersion)
	}
	if !reflect.DeepEqual(got.Entries, latest.Entries) {
		t.Fatalf("entries = %+v, want %+v", got.Entries, latest.Entries)
	}
	if !reflect.DeepEqual(got.Provisioners, latest.Provisioners) {
		t.Fatalf("provisioners = %+v, want %+v", got.Provisioners, latest.Provisioners)
	}
	if got.InstalledSelection == nil || !reflect.DeepEqual(*got.InstalledSelection, want) {
		t.Fatalf("installed selection = %+v, want %+v", got.InstalledSelection, want)
	}
}
