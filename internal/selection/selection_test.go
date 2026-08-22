package selection_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
)

func TestCompareEvolutionRetainsHistoricalProfileDependenciesForComparison(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dots.yaml")
	previousSource := []byte(`version: 1
profiles:
  desktop:
    tags: [desktop]
    dependencies:
      - name: Desktop Nerd Font
        brew_cask: font-cascadia-code-nf
        font_match: "CascadiaCodeNF*"
entries:
  - source: configs/ghostty/config
    target: ~/.config/ghostty/config
    strategy: copy
    tags: [desktop]
`)
	if err := os.WriteFile(path, previousSource, 0o600); err != nil {
		t.Fatalf("write previous manifest: %v", err)
	}
	previousManifest, err := manifest.LoadPreviousFile(path)
	if err != nil {
		t.Fatalf("load previous manifest: %v", err)
	}
	currentManifest := manifest.Manifest{
		Profiles: map[string]manifest.Profile{"desktop": {Tags: []string{"desktop"}}},
		Dependencies: []manifest.DependencySet{{Tags: []string{"desktop"}, Dependencies: []manifest.Dependency{{
			Name: "Desktop Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*",
		}}}},
		Entries: []manifest.Entry{{Source: "configs/ghostty/config", Target: "~/.config/ghostty/config", Strategy: "copy", Tags: []string{"desktop"}}},
	}
	previous, err := selection.ResolveIntent(*previousManifest, selection.Intent{Source: selection.SourceRecorded, Profiles: []string{"desktop"}})
	if err != nil {
		t.Fatalf("resolve previous selection: %v", err)
	}
	evolved, err := selection.CompareEvolution(*previousManifest, currentManifest, previous, "darwin")
	if err != nil {
		t.Fatalf("compare evolution: %v", err)
	}
	if delta := evolved.Report.Delta; delta == nil || len(delta.Added.Dependencies) != 0 || len(delta.Removed.Dependencies) != 0 {
		t.Fatalf("dependency evolution = %#v, want unchanged historical font selection", delta)
	}
}

func TestCompareEvolutionReportsSelectedSurfaceChangesInManifestOrder(t *testing.T) {
	oldManifest := manifest.Manifest{
		Profiles: map[string]manifest.Profile{
			"core": {Tags: []string{"core"}},
		},
		Entries: []manifest.Entry{
			{Target: ".old", Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "entry-old"}}},
			{Target: ".shared", Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "shared"}}},
		},
		Provisioners: []manifest.Provisioner{{Tool: "old-tool", Tags: []string{"core"}}},
	}
	newManifest := manifest.Manifest{
		Profiles: map[string]manifest.Profile{
			"core": {Tags: []string{"core", "new"}},
		},
		Dependencies: []manifest.DependencySet{{
			Tags: []string{"new"}, Dependencies: []manifest.Dependency{{Name: "set-new"}, {Name: "shared"}},
		}},
		Entries: []manifest.Entry{
			{Target: ".shared", Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "shared"}}},
			{Target: ".new", Tags: []string{"new"}, Dependencies: []manifest.Dependency{{Name: "entry-new"}}},
		},
		Provisioners: []manifest.Provisioner{{
			Tool: "new-tool", Tags: []string{"new"}, Dependencies: []manifest.Dependency{{Name: "provisioner-new"}},
		}},
	}
	previous, err := selection.ResolveIntent(oldManifest, selection.Intent{
		Source: selection.SourceRecorded, Profiles: []string{"core"},
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}

	got, err := selection.CompareEvolution(oldManifest, newManifest, previous, "darwin")
	if err != nil {
		t.Fatalf("CompareEvolution() error = %v", err)
	}
	delta := got.Report.Delta
	if delta == nil {
		t.Fatal("SelectionDelta = nil")
	}
	if want := []string{"new"}; !reflect.DeepEqual(delta.Added.EffectiveTags, want) {
		t.Fatalf("added effective Tags = %#v, want %#v", delta.Added.EffectiveTags, want)
	}
	if want := []string{".new"}; !reflect.DeepEqual(delta.Added.ManagedEntries, want) {
		t.Fatalf("added Managed Entries = %#v, want %#v", delta.Added.ManagedEntries, want)
	}
	if want := []string{"set-new", "entry-new", "provisioner-new"}; !reflect.DeepEqual(delta.Added.Dependencies, want) {
		t.Fatalf("added Dependencies = %#v, want %#v", delta.Added.Dependencies, want)
	}
	if want := []string{"new-tool"}; !reflect.DeepEqual(delta.Added.Provisioners, want) {
		t.Fatalf("added Provisioners = %#v, want %#v", delta.Added.Provisioners, want)
	}
	if want := []string{".old"}; !reflect.DeepEqual(delta.Removed.ManagedEntries, want) {
		t.Fatalf("removed Managed Entries = %#v, want %#v", delta.Removed.ManagedEntries, want)
	}
	if want := []string{"entry-old"}; !reflect.DeepEqual(delta.Removed.Dependencies, want) {
		t.Fatalf("removed Dependencies = %#v, want %#v", delta.Removed.Dependencies, want)
	}
	if want := []string{"old-tool"}; !reflect.DeepEqual(delta.Removed.Provisioners, want) {
		t.Fatalf("removed Provisioners = %#v, want %#v", delta.Removed.Provisioners, want)
	}
}

func TestRepositoryAgentsEvolutionReportsRetiredGentleAISurfaces(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "dots.yaml")
	oldManifest := manifest.Manifest{
		Profiles: map[string]manifest.Profile{"agents": {Tags: []string{"agents"}}},
		Provisioners: []manifest.Provisioner{
			{
				Tool: "gentle-ai", Tags: []string{"agents"},
				Dependencies: []manifest.Dependency{{Name: "gentle-ai"}, {Name: "engram"}},
			},
			{Tool: "skills", Tags: []string{"agents"}},
		},
	}
	newManifest, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("load current manifest: %v", err)
	}
	previous, err := selection.ResolveIntent(oldManifest, selection.Intent{
		Source: selection.SourceRecorded, Profiles: []string{"agents"},
	})
	if err != nil {
		t.Fatalf("resolve previous agents selection: %v", err)
	}
	evolved, err := selection.CompareEvolution(oldManifest, *newManifest, previous, "linux")
	if err != nil {
		t.Fatalf("compare agents evolution: %v", err)
	}
	if evolved.Report.Delta == nil {
		t.Fatal("agents evolution Delta = nil")
	}
	removed := evolved.Report.Delta.Removed
	for _, dependency := range []string{"gentle-ai", "engram"} {
		if !slices.Contains(removed.Dependencies, dependency) {
			t.Errorf("removed Dependencies = %#v, want %q", removed.Dependencies, dependency)
		}
	}
	for _, provisioner := range []string{"gentle-ai", "skills"} {
		if !slices.Contains(removed.Provisioners, provisioner) {
			t.Errorf("removed Provisioners = %#v, want %q", removed.Provisioners, provisioner)
		}
	}
}

func TestCompareInstalledNoRecordedSelectionOrEqualSelectionHasNoChange(t *testing.T) {
	m := replacementManifest()
	requested, err := selection.ResolveIntent(m, selection.Intent{
		Source: selection.SourceExplicit, Profiles: []string{"core"}, ExtraTags: []string{"extra"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := selection.CompareInstalled(m, requested, nil, "linux"); got.Report.Change != nil {
		t.Fatalf("nil recorded Change = %#v, want nil", got.Report.Change)
	}
	recorded := &state.InstalledSelection{
		Profiles: []string{"core"}, ExtraTags: []string{"extra"},
		ResolvedTags: []string{"core", "extra"},
	}
	if got := selection.CompareInstalled(m, requested, recorded, "linux"); got.Report.Change != nil {
		t.Fatalf("equal Change = %#v, want nil", got.Report.Change)
	}
	recorded.ResolvedTags = []string{"core"}
	if got := selection.CompareInstalled(m, requested, recorded, "linux"); got.Report.Change != nil {
		t.Fatalf("equal intent with stale audit snapshot Change = %#v, want nil", got.Report.Change)
	}
}

func TestCompareInstalledAdditiveChangeDoesNotRequireAcknowledgement(t *testing.T) {
	m := replacementManifest()
	requested, err := selection.ResolveIntent(m, selection.Intent{
		Source: selection.SourceExplicit, Profiles: []string{"core", "work"}, ExtraTags: []string{"extra"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := selection.CompareInstalled(m, requested, &state.InstalledSelection{
		Profiles: []string{"core"}, ResolvedTags: []string{"core"},
	}, "linux")
	change := got.Report.Change
	if change == nil {
		t.Fatal("Change = nil")
	}
	if change.AcknowledgementRequired || change.AcknowledgementAccepted {
		t.Fatalf("acknowledgement = required:%t accepted:%t, want false/false",
			change.AcknowledgementRequired, change.AcknowledgementAccepted)
	}
	if want := []string{"work"}; !reflect.DeepEqual(change.Delta.Added.Profiles, want) {
		t.Fatalf("added Profiles = %#v, want %#v", change.Delta.Added.Profiles, want)
	}
	if want := []string{"extra"}; !reflect.DeepEqual(change.Delta.Added.ExtraTags, want) {
		t.Fatalf("added extra Tags = %#v, want %#v", change.Delta.Added.ExtraTags, want)
	}
}

func TestCompareInstalledIntentReductionRequiresAcknowledgement(t *testing.T) {
	m := replacementManifest()
	requested, err := selection.ResolveIntent(m, selection.Intent{
		Source: selection.SourceExplicit, Profiles: []string{"core"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := selection.CompareInstalled(m, requested, &state.InstalledSelection{
		Profiles: []string{"core", "work"}, ExtraTags: []string{"extra"},
		ResolvedTags: []string{"core", "work", "extra"},
	}, "linux")
	change := got.Report.Change
	if change == nil || !change.AcknowledgementRequired || change.AcknowledgementAccepted {
		t.Fatalf("Change = %#v, want required and unaccepted", change)
	}
	if want := []string{"work"}; !reflect.DeepEqual(change.Delta.Removed.Profiles, want) {
		t.Fatalf("removed Profiles = %#v, want %#v", change.Delta.Removed.Profiles, want)
	}
	if want := []string{"extra"}; !reflect.DeepEqual(change.Delta.Removed.ExtraTags, want) {
		t.Fatalf("removed extra Tags = %#v, want %#v", change.Delta.Removed.ExtraTags, want)
	}
}

func TestCompareInstalledReportsRecordedAuditSurfaceDifferences(t *testing.T) {
	m := replacementManifest()
	requested, err := selection.ResolveIntent(m, selection.Intent{
		Source: selection.SourceExplicit, Profiles: []string{"work"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := selection.CompareInstalled(m, requested, &state.InstalledSelection{
		Profiles: []string{"core"}, ResolvedTags: []string{"core", "retired"},
	}, "linux")
	delta := got.Report.Change.Delta
	if want := []string{"work"}; !reflect.DeepEqual(delta.Current.Profiles, want) {
		t.Fatalf("current Profiles = %#v, want %#v", delta.Current.Profiles, want)
	}
	if want := []string{"core", "retired"}; !reflect.DeepEqual(delta.Previous.EffectiveTags, want) {
		t.Fatalf("previous effective Tags = %#v, want recorded audit snapshot %#v", delta.Previous.EffectiveTags, want)
	}
	if want := []string{".work"}; !reflect.DeepEqual(delta.Added.ManagedEntries, want) {
		t.Fatalf("added Managed Entries = %#v, want %#v", delta.Added.ManagedEntries, want)
	}
	if want := []string{".core", ".retired"}; !reflect.DeepEqual(delta.Removed.ManagedEntries, want) {
		t.Fatalf("removed Managed Entries = %#v, want %#v", delta.Removed.ManagedEntries, want)
	}
	if want := []string{"work-dep"}; !reflect.DeepEqual(delta.Added.Dependencies, want) {
		t.Fatalf("added Dependencies = %#v, want %#v", delta.Added.Dependencies, want)
	}
	if want := []string{"core-dep", "retired-dep"}; !reflect.DeepEqual(delta.Removed.Dependencies, want) {
		t.Fatalf("removed Dependencies = %#v, want %#v", delta.Removed.Dependencies, want)
	}
	if want := []string{"work-tool"}; !reflect.DeepEqual(delta.Added.Provisioners, want) {
		t.Fatalf("added Provisioners = %#v, want %#v", delta.Added.Provisioners, want)
	}
	if want := []string{"retired-tool"}; !reflect.DeepEqual(delta.Removed.Provisioners, want) {
		t.Fatalf("removed Provisioners = %#v, want %#v", delta.Removed.Provisioners, want)
	}
}

func TestCompareInstalledChangeJSONHasDeterministicArraysAndEvolutionPreservesChange(t *testing.T) {
	m := replacementManifest()
	requested, err := selection.ResolveIntent(m, selection.Intent{
		Source: selection.SourceExplicit, Profiles: []string{"work"},
	})
	if err != nil {
		t.Fatal(err)
	}
	compared := selection.CompareInstalled(m, requested, &state.InstalledSelection{
		Profiles: []string{"core"}, ResolvedTags: []string{"core"},
	}, "linux")
	data, err := json.Marshal(compared.Report.Change)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"extra_tags":[]`, `"provisioners":[]`,
		`"missing_profiles":[]`, `"stale_extra_tags":[]`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("JSON = %s, want %s", data, want)
		}
	}
	evolved, err := selection.CompareEvolution(m, m, compared, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if evolved.Report.Delta == nil || !reflect.DeepEqual(evolved.Report.Change, compared.Report.Change) {
		t.Fatalf("evolved Report = %#v, want evolution Delta and preserved Change", evolved.Report)
	}
}

func replacementManifest() manifest.Manifest {
	return manifest.Manifest{
		Profiles: map[string]manifest.Profile{
			"core": {Tags: []string{"core"}},
			"work": {Tags: []string{"work"}},
		},
		Entries: []manifest.Entry{
			{Target: ".core", Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "core-dep"}}},
			{Target: ".retired", Tags: []string{"retired"}, Dependencies: []manifest.Dependency{{Name: "retired-dep"}}},
			{Target: ".work", Tags: []string{"work"}, Dependencies: []manifest.Dependency{{Name: "work-dep"}}},
			{Target: ".extra", Tags: []string{"extra"}},
		},
		Provisioners: []manifest.Provisioner{
			{Tool: "retired-tool", Tags: []string{"retired"}},
			{Tool: "work-tool", Tags: []string{"work"}},
		},
	}
}

func TestCompareEvolutionRejectsMissingProfileWithStructuredDelta(t *testing.T) {
	oldManifest := manifest.Manifest{Profiles: map[string]manifest.Profile{"core": {Tags: []string{"core"}}}}
	previous, err := selection.ResolveIntent(oldManifest, selection.Intent{
		Source: selection.SourceRecorded, Profiles: []string{"core"},
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}

	_, err = selection.CompareEvolution(oldManifest, manifest.Manifest{}, previous, "linux")
	var evolutionErr *selection.EvolutionError
	if !errors.As(err, &evolutionErr) {
		t.Fatalf("CompareEvolution() error = %T %v, want *EvolutionError", err, err)
	}
	if want := []string{"core"}; !reflect.DeepEqual(evolutionErr.SelectionDelta.MissingProfiles, want) {
		t.Fatalf("missing Profiles = %#v, want %#v", evolutionErr.SelectionDelta.MissingProfiles, want)
	}
	if !strings.Contains(err.Error(), "update the selection") {
		t.Fatalf("error = %q, want actionable guidance", err)
	}
}

func TestCompareEvolutionRejectsStaleExplicitExtraTag(t *testing.T) {
	oldManifest := manifest.Manifest{
		Entries: []manifest.Entry{{Target: ".old", Tags: []string{"retired"}}},
	}
	previous, err := selection.ResolveIntent(oldManifest, selection.Intent{
		Source: selection.SourceExplicit, ExtraTags: []string{"retired"},
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}

	_, err = selection.CompareEvolution(oldManifest, manifest.Manifest{
		Entries: []manifest.Entry{{Target: ".new", Tags: []string{"current"}}},
	}, previous, "linux")
	var evolutionErr *selection.EvolutionError
	if !errors.As(err, &evolutionErr) {
		t.Fatalf("CompareEvolution() error = %T %v, want *EvolutionError", err, err)
	}
	if want := []string{"retired"}; !reflect.DeepEqual(evolutionErr.SelectionDelta.StaleExtraTags, want) {
		t.Fatalf("stale extra Tags = %#v, want %#v", evolutionErr.SelectionDelta.StaleExtraTags, want)
	}
	if want := []string{".old"}; !reflect.DeepEqual(evolutionErr.SelectionDelta.Removed.ManagedEntries, want) {
		t.Fatalf("removed Managed Entries = %#v, want %#v", evolutionErr.SelectionDelta.Removed.ManagedEntries, want)
	}
	if !strings.Contains(err.Error(), `explicit selection: extra Tag "retired" is no longer declared`) {
		t.Fatalf("error = %q, want source and actionable stale Tag", err)
	}
	data, marshalErr := json.Marshal(evolutionErr.JSONErrorData())
	if marshalErr != nil || !strings.Contains(string(data), `"selection_delta"`) {
		t.Fatalf("JSONErrorData() = %s, %v; want selection_delta", data, marshalErr)
	}
}

func TestRepositoryEvolutionReportsRemovedCodexDelegationProfileAndTag(t *testing.T) {
	oldManifest := manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"core":             {Kind: "surface", Status: "current"},
			"codex-delegation": {Kind: "surface", Status: "current"},
		},
		Profiles: map[string]manifest.Profile{
			"core":             {Tags: []string{"core"}},
			"codex-delegation": {Tags: []string{"codex-delegation"}},
		},
		Provisioners: []manifest.Provisioner{{Tool: "skills", Tags: []string{"codex-delegation"}}},
	}
	newManifest, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatalf("load current manifest: %v", err)
	}

	previous, err := selection.ResolveIntent(oldManifest, selection.Intent{
		Source:    selection.SourceExplicit,
		Profiles:  []string{"core", "codex-delegation"},
		ExtraTags: []string{"codex-delegation"},
	})
	if err != nil {
		t.Fatalf("resolve previous selection: %v", err)
	}

	_, err = selection.CompareEvolution(oldManifest, *newManifest, previous, "linux")
	var evolutionErr *selection.EvolutionError
	if !errors.As(err, &evolutionErr) {
		t.Fatalf("CompareEvolution() error = %T %v, want *EvolutionError", err, err)
	}
	if want := []string{"codex-delegation"}; !reflect.DeepEqual(evolutionErr.SelectionDelta.MissingProfiles, want) {
		t.Fatalf("missing Profiles = %#v, want %#v", evolutionErr.SelectionDelta.MissingProfiles, want)
	}
	if want := []string{"codex-delegation"}; !reflect.DeepEqual(evolutionErr.SelectionDelta.StaleExtraTags, want) {
		t.Fatalf("stale extra Tags = %#v, want %#v", evolutionErr.SelectionDelta.StaleExtraTags, want)
	}
}

func TestCompareEvolutionUsesTagRegistryForStaleExtraTags(t *testing.T) {
	m := manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"core":          {Kind: "surface", Status: "current"},
			"legacy-option": {Kind: "compatibility", Status: "legacy", ReplacedBy: manifest.ReplacementTags{"core"}},
		},
		Profiles: map[string]manifest.Profile{"core": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{{Target: ".core", Tags: []string{"core"}}},
	}
	previous, err := selection.ResolveIntent(m, selection.Intent{
		Source: selection.SourceExplicit, Profiles: []string{"core"}, ExtraTags: []string{"legacy-option"},
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}
	evolved, err := selection.CompareEvolution(m, m, previous, "linux")
	if err != nil {
		t.Fatalf("CompareEvolution() error = %v, want declared legacy tag to remain selectable", err)
	}
	if want := []manifest.TagReplacement{{LegacyTag: "legacy-option", ReplacementTags: []string{"core"}}}; !reflect.DeepEqual(evolved.Report.TagMigrations, want) {
		t.Fatalf("TagMigrations = %#v, want preserved evidence %#v", evolved.Report.TagMigrations, want)
	}

	_, err = selection.ResolveIntent(m, selection.Intent{
		Source: selection.SourceExplicit, Profiles: []string{"core"}, ExtraTags: []string{"unknown"},
	})
	if err == nil || !strings.Contains(err.Error(), `tag "unknown" is not declared`) {
		t.Fatalf("ResolveIntent() error = %v, want undeclared registry Tag rejection", err)
	}
}

func TestCompareEvolutionDeltaJSONUsesDeterministicEmptyArrays(t *testing.T) {
	m := manifest.Manifest{
		Profiles: map[string]manifest.Profile{"core": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{{Target: ".shared", Tags: []string{"core"}}},
	}
	previous, err := selection.ResolveIntent(m, selection.Intent{
		Source: selection.SourceRecorded, Profiles: []string{"core"},
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}
	got, err := selection.CompareEvolution(m, m, previous, "darwin")
	if err != nil {
		t.Fatalf("CompareEvolution() error = %v", err)
	}

	data, err := json.Marshal(got.Report)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, want := range []string{
		`"effective_tags":[]`, `"managed_entries":[]`, `"dependencies":[]`,
		`"provisioners":[]`, `"missing_profiles":[]`, `"stale_extra_tags":[]`,
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("JSON = %s, want %s", data, want)
		}
	}
}

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
	if want := []string{"web"}; !reflect.DeepEqual(got.Profiles, want) {
		t.Fatalf("Profiles = %#v, want %#v", got.Profiles, want)
	}
	if want := []string{"explicit"}; !reflect.DeepEqual(got.ExtraTags, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got.ExtraTags, want)
	}
	if want := []string{"web", "explicit"}; !reflect.DeepEqual(got.Selection.Tags, want) || !reflect.DeepEqual(got.Report.EffectiveTags, want) {
		t.Fatalf("effective Tags = selection %#v report %#v, want %#v", got.Selection.Tags, got.Report.EffectiveTags, want)
	}
	if got.Report.Source != selection.SourceExplicit {
		t.Fatalf("Source = %q, want %q", got.Report.Source, selection.SourceExplicit)
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

func TestResolveIntentNormalizesLegacyExtraTagsBeforeSelection(t *testing.T) {
	m := loadSelectionManifest(t, `version: 1
tags:
  core:
    kind: surface
    status: current
  shell:
    kind: surface
    status: current
  legacy-shell:
    kind: compatibility
    status: legacy
    replaced_by: [core, shell]
profiles:
  core:
    tags: [core]
entries:
  - source: shell
    target: .shell
    strategy: copy
    tags: [shell]
`)

	got, err := selection.ResolveIntent(m, selection.Intent{
		Source:    selection.SourceExplicit,
		Profiles:  []string{"core"},
		ExtraTags: []string{"legacy-shell", "shell", "legacy-shell", "core"},
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}
	if want := []string{"core", "shell"}; !reflect.DeepEqual(got.ExtraTags, want) {
		t.Fatalf("ExtraTags = %#v, want normalized current Tags %#v", got.ExtraTags, want)
	}
	if want := []string{"core", "shell"}; !reflect.DeepEqual(got.Selection.Tags, want) {
		t.Fatalf("Selection Tags = %#v, want normalized current Tags %#v", got.Selection.Tags, want)
	}
	if want := []string{"core", "shell"}; !reflect.DeepEqual(got.Report.EffectiveTags, want) {
		t.Fatalf("EffectiveTags = %#v, want normalized current Tags %#v", got.Report.EffectiveTags, want)
	}
	if want := []string{"legacy-shell", "shell", "legacy-shell", "core"}; !reflect.DeepEqual(got.Intent().ExtraTags, want) {
		t.Fatalf("Intent ExtraTags = %#v, want original ordered input %#v", got.Intent().ExtraTags, want)
	}

	installed := got.InstalledSelection(state.Provenance{})
	if want := []string{"core", "shell"}; !reflect.DeepEqual(installed.ExtraTags, want) {
		t.Fatalf("Installed ExtraTags = %#v, want normalized current Tags %#v", installed.ExtraTags, want)
	}
	if want := []string{"core", "shell"}; !reflect.DeepEqual(installed.ResolvedTags, want) {
		t.Fatalf("Installed ResolvedTags = %#v, want normalized current Tags %#v", installed.ResolvedTags, want)
	}

	reportJSON, err := json.Marshal(got.Report)
	if err != nil {
		t.Fatalf("json.Marshal(Report) error = %v", err)
	}
	wantEvidence := `"tag_migrations":[{"legacy_tag":"legacy-shell","replacement_tags":["core","shell"]}]`
	if !strings.Contains(string(reportJSON), wantEvidence) {
		t.Fatalf("Report JSON = %s, want deterministic evidence %s", reportJSON, wantEvidence)
	}
}

func TestResolveIntentNormalizesRecordedLegacyExtraTags(t *testing.T) {
	m := loadSelectionManifest(t, `version: 1
tags:
  current:
    kind: surface
    status: current
  legacy:
    kind: compatibility
    status: legacy
    replaced_by: [current]
profiles:
  current:
    tags: [current]
entries:
  - source: current
    target: .current
    strategy: copy
    tags: [current]
`)

	got, err := selection.ResolveIntent(m, selection.Intent{
		Source:    selection.SourceRecorded,
		ExtraTags: []string{"legacy", "current", "legacy"},
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}
	if got.Report.Source != selection.SourceRecorded {
		t.Fatalf("Source = %q, want %q", got.Report.Source, selection.SourceRecorded)
	}
	if want := []string{"current"}; !reflect.DeepEqual(got.ExtraTags, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got.ExtraTags, want)
	}
	reportJSON, err := json.Marshal(got.Report)
	if err != nil {
		t.Fatalf("json.Marshal(Report) error = %v", err)
	}
	if count := strings.Count(string(reportJSON), `"legacy_tag":"legacy"`); count != 1 {
		t.Fatalf("Report JSON = %s, want one deduplicated legacy migration", reportJSON)
	}
}

func loadSelectionManifest(t *testing.T, source string) manifest.Manifest {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dots.yaml")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	return *m
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

func TestResolveIntentAcceptsExplicitlyAllowedEmptySelection(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"default": {Tags: []string{"must-not-be-used"}},
	}}

	got, err := selection.ResolveIntent(m, selection.Intent{
		Source:     selection.SourceExplicit,
		AllowEmpty: true,
	})
	if err != nil {
		t.Fatalf("ResolveIntent() error = %v", err)
	}
	if len(got.Profiles) != 0 || len(got.ExtraTags) != 0 || len(got.Selection.Tags) != 0 || len(got.Report.EffectiveTags) != 0 {
		t.Fatalf("Effective = %#v, want empty selection", got)
	}
	if intent := got.Intent(); !intent.AllowEmpty {
		t.Fatalf("Intent() = %#v, want empty-selection authority preserved", intent)
	}

	evolved, err := selection.CompareEvolution(m, m, got, "linux")
	if err != nil {
		t.Fatalf("CompareEvolution() error = %v", err)
	}
	if !evolved.Intent().AllowEmpty || len(evolved.Selection.Tags) != 0 {
		t.Fatalf("evolved Effective = %#v, want authorized empty selection", evolved)
	}

	installed := evolved.InstalledSelection(state.Provenance{})
	if len(installed.Profiles) != 0 || len(installed.ExtraTags) != 0 || len(installed.ResolvedTags) != 0 {
		t.Fatalf("InstalledSelection() = %#v, want empty selection", installed)
	}
}

func TestResolveIntentRejectsUnapprovedEmptySelection(t *testing.T) {
	_, err := selection.ResolveIntent(manifest.Manifest{}, selection.Intent{Source: selection.SourceExplicit})
	if err == nil || err.Error() != "explicit selection: at least one Profile or extra Tag is required" {
		t.Fatalf("ResolveIntent() error = %v, want unapproved empty selection rejection", err)
	}
}

func TestResolveReadOnlyAcceptsRecordedEmptySelection(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"default": {Tags: []string{"must-not-be-used"}},
	}}

	got, err := selection.ResolveReadOnly(m, nil, nil, &state.InstalledSelection{})
	if err != nil {
		t.Fatalf("ResolveReadOnly() error = %v", err)
	}
	if got.Report.Source != selection.SourceRecorded {
		t.Fatalf("Source = %q, want %q", got.Report.Source, selection.SourceRecorded)
	}
	if len(got.Profiles) != 0 || len(got.ExtraTags) != 0 || len(got.Selection.Tags) != 0 || len(got.Report.EffectiveTags) != 0 {
		t.Fatalf("Effective = %#v, want empty recorded selection", got)
	}
	installed := got.InstalledSelection(state.Provenance{})
	if len(installed.Profiles) != 0 || len(installed.ExtraTags) != 0 || len(installed.ResolvedTags) != 0 {
		t.Fatalf("InstalledSelection() = %#v, want empty selection", installed)
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

func TestResolveIntentAcceptsConfirmedMigrationSource(t *testing.T) {
	m := manifest.Manifest{
		Profiles: map[string]manifest.Profile{
			"core": {Tags: []string{"core"}},
		},
	}

	effective, err := selection.ResolveIntent(m, selection.Intent{Source: selection.SourceMigration, Profiles: []string{"core"}, ExtraTags: []string{"extra"}})
	if err != nil {
		t.Fatalf("ResolveIntent: %v", err)
	}
	if got, want := effective.Report.Source, selection.SourceMigration; got != want {
		t.Fatalf("Source = %q, want %q", got, want)
	}
	if got, want := effective.Report.EffectiveTags, []string{"core", "extra"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("EffectiveTags = %#v, want %#v", got, want)
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
