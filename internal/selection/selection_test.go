package selection_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
)

func TestResolveReadOnlyExplicitSelectionWinsCompletely(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"core": {Tags: []string{"core", "shared"}},
		"web":  {Tags: []string{"web"}},
	}}
	recorded := &state.InstalledSelection{
		Profiles:     []string{"core"},
		ExtraTags:    []string{"recorded-extra"},
		ResolvedTags: []string{"stale", "snapshot"},
	}

	got, err := selection.ResolveReadOnly(m, []string{"web", "web"}, []string{"explicit", "explicit"}, recorded)
	if err != nil {
		t.Fatalf("ResolveReadOnly() error = %v", err)
	}
	want := selection.Effective{
		Profiles:  []string{"web"},
		ExtraTags: []string{"explicit"},
		Selection: manifest.Selection{
			Profile:  "web",
			Profiles: []string{"web"},
			Tags:     []string{"web", "explicit"},
		},
		Report: selection.Report{
			Source:        selection.SourceExplicit,
			Profiles:      []string{"web"},
			ExtraTags:     []string{"explicit"},
			EffectiveTags: []string{"web", "explicit"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveReadOnly() = %#v, want %#v", got, want)
	}
}

func TestResolveReadOnlyTagOnlySelectionDoesNotInferDefaultProfile(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"default": {Tags: []string{"must-not-be-used"}},
	}}

	got, err := selection.ResolveReadOnly(m, nil, []string{"web", "web"}, nil)
	if err != nil {
		t.Fatalf("ResolveReadOnly() error = %v", err)
	}
	if len(got.Selection.Profiles) != 0 || got.Selection.Profile != "" {
		t.Fatalf("Selection Profiles = %#v, Profile = %q; want no inferred Profile", got.Selection.Profiles, got.Selection.Profile)
	}
	if want := []string{"web"}; !reflect.DeepEqual(got.Selection.Tags, want) {
		t.Fatalf("Selection Tags = %#v, want %#v", got.Selection.Tags, want)
	}
	if got.Report.Source != selection.SourceExplicit {
		t.Fatalf("Source = %q, want %q", got.Report.Source, selection.SourceExplicit)
	}
}

func TestResolveReadOnlyRecomputesRecordedSelection(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"core": {Tags: []string{"current", "shared"}},
	}}
	recorded := &state.InstalledSelection{
		Profiles:     []string{"core"},
		ExtraTags:    []string{"shared", "extra", "extra"},
		ResolvedTags: []string{"stale"},
	}

	got, err := selection.ResolveReadOnly(m, nil, nil, recorded)
	if err != nil {
		t.Fatalf("ResolveReadOnly() error = %v", err)
	}
	if got.Report.Source != selection.SourceRecorded {
		t.Fatalf("Source = %q, want %q", got.Report.Source, selection.SourceRecorded)
	}
	if want := []string{"current", "shared", "extra"}; !reflect.DeepEqual(got.Report.EffectiveTags, want) {
		t.Fatalf("EffectiveTags = %#v, want %#v", got.Report.EffectiveTags, want)
	}
	if want := []string{"shared", "extra"}; !reflect.DeepEqual(got.ExtraTags, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got.ExtraTags, want)
	}
}

func TestResolveReadOnlyRequiresInvocationOrRecordedSelection(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"default": {Tags: []string{"must-not-be-used"}},
	}}

	_, err := selection.ResolveReadOnly(m, nil, nil, nil)
	if !errors.Is(err, selection.ErrSelectionRequired) {
		t.Fatalf("ResolveReadOnly() error = %v, want ErrSelectionRequired", err)
	}
	if !strings.Contains(err.Error(), "--profile") || !strings.Contains(err.Error(), "dots install") {
		t.Fatalf("ResolveReadOnly() error = %q, want actionable guidance", err)
	}
}

func TestResolveReadOnlyValidatesSelectedSource(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"core": {Tags: []string{"core"}},
	}}
	tests := []struct {
		name     string
		profiles []string
		recorded *state.InstalledSelection
		source   selection.Source
	}{
		{name: "explicit", profiles: []string{"missing"}, source: selection.SourceExplicit},
		{name: "recorded", recorded: &state.InstalledSelection{Profiles: []string{"missing"}}, source: selection.SourceRecorded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := selection.ResolveReadOnly(m, tt.profiles, nil, tt.recorded)
			if err == nil {
				t.Fatal("ResolveReadOnly() error = nil")
			}
			if !strings.Contains(err.Error(), string(tt.source)+" selection") || !strings.Contains(err.Error(), `profile "missing" not found`) {
				t.Fatalf("ResolveReadOnly() error = %q, want source and validation context", err)
			}
		})
	}
}

func TestResolveReadOnlyRejectsEmptyIntentValues(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"core": {Tags: []string{"core"}},
	}}
	tests := []struct {
		name     string
		profiles []string
		tags     []string
		recorded *state.InstalledSelection
		want     string
	}{
		{name: "explicit empty profile", profiles: []string{""}, want: "explicit selection: profile names must not be empty"},
		{name: "explicit empty tag", tags: []string{""}, want: "explicit selection: tags must not be empty"},
		{name: "explicit whitespace profile", profiles: []string{" \t"}, want: "explicit selection: profile names must not be empty"},
		{name: "explicit whitespace tag", tags: []string{" \t"}, want: "explicit selection: tags must not be empty"},
		{name: "recorded empty", recorded: &state.InstalledSelection{}, want: "recorded selection: at least one Profile or extra Tag is required"},
		{name: "recorded whitespace tag", recorded: &state.InstalledSelection{ExtraTags: []string{" "}}, want: "recorded selection: tags must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := selection.ResolveReadOnly(m, tt.profiles, tt.tags, tt.recorded)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("ResolveReadOnly() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestResolveReadOnlyClonesSlices(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"core": {Tags: []string{"core"}},
	}}
	profiles := []string{"core"}
	tags := []string{"extra"}
	got, err := selection.ResolveReadOnly(m, profiles, tags, nil)
	if err != nil {
		t.Fatalf("ResolveReadOnly() error = %v", err)
	}

	profiles[0], tags[0] = "changed", "changed"
	got.Profiles[0], got.ExtraTags[0] = "effective-changed", "effective-changed"
	if got.Report.Profiles[0] != "core" || got.Report.ExtraTags[0] != "extra" {
		t.Fatalf("Report aliases input or Effective slices: %#v", got.Report)
	}
	if got.Selection.Profiles[0] != "core" || got.Selection.Tags[1] != "extra" {
		t.Fatalf("Selection aliases Effective slices: %#v", got.Selection)
	}
}

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
