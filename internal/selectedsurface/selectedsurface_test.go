package selectedsurface_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestEvaluateFiltersByTagAndOSInManifestOrder(t *testing.T) {
	m := manifest.Manifest{
		Dependencies: []manifest.DependencySet{
			{Tags: []string{"linux"}, OS: []string{"linux"}, Dependencies: []manifest.Dependency{{Name: "linux-set"}}},
			{Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "core-set"}}},
			{Tags: []string{"core"}, OS: []string{"darwin"}, Dependencies: []manifest.Dependency{{Name: "darwin-set"}}},
		},
		Entries: []manifest.Entry{
			{Source: "skip", Target: "skip", Strategy: "copy", Tags: []string{"other"}},
			{Source: "core", Target: "core", Strategy: "copy", Tags: []string{"core"}},
			{Source: "linux", Target: "linux", Strategy: "copy", Tags: []string{"linux"}, OS: []string{"linux"}},
		},
		Provisioners: []manifest.Provisioner{
			{Tool: "core-tool", Tags: []string{"core"}},
			{Tool: "darwin-tool", Tags: []string{"core"}, OS: []string{"darwin"}},
		},
	}

	got := selectedsurface.Evaluate(m, []string{"core", "linux", "core"}, "linux")
	if want := []string{"core", "linux"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("Tags = %#v, want %#v", got.Tags, want)
	}
	if got.DependencySets[0].Dependencies[0].Name != "linux-set" || got.DependencySets[1].Dependencies[0].Name != "core-set" {
		t.Fatalf("DependencySets order = %#v", got.DependencySets)
	}
	if got.Entries[0].Entry.Target != "core" || got.Entries[1].Entry.Target != "linux" {
		t.Fatalf("Entries order = %#v", got.Entries)
	}
	if len(got.Provisioners) != 1 || got.Provisioners[0].Tool != "core-tool" {
		t.Fatalf("Provisioners = %#v", got.Provisioners)
	}
}

func TestEvaluateDeduplicatesOnlyExactSelectedDeclarations(t *testing.T) {
	set := manifest.DependencySet{Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "set"}}}
	entry := manifest.Entry{Source: "a", Target: "shared", Strategy: "copy", Tags: []string{"core"}}
	provisioner := manifest.Provisioner{Tool: "tool", Tags: []string{"core"}}
	m := manifest.Manifest{
		Dependencies: []manifest.DependencySet{set, set, {Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "different"}}}},
		Entries:      []manifest.Entry{entry, entry, {Source: "b", Target: "shared", Strategy: "copy", Tags: []string{"core"}}},
		Provisioners: []manifest.Provisioner{provisioner, provisioner, {Tool: "tool", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Scope: "user"}}},
	}
	got := selectedsurface.Evaluate(m, []string{"core"}, "linux")
	if len(got.DependencySets) != 2 || len(got.Entries) != 2 || len(got.Provisioners) != 2 {
		t.Fatalf("deduplicated surface = %#v", got)
	}
}

func TestEvaluateDependenciesPreservesOccurrencesAndPromotesRequired(t *testing.T) {
	m := manifest.Manifest{
		Dependencies: []manifest.DependencySet{{Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: " tool ", Requirement: "optional"}, {Name: "set-only"}}}},
		Entries:      []manifest.Entry{{Source: "entry", Target: "target", Strategy: "copy", Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "tool", Requirement: "required"}, {Name: "entry-only"}}}},
		Provisioners: []manifest.Provisioner{{Tool: "prov", Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "tool"}, {Name: "prov-only"}}}},
	}
	got := selectedsurface.Evaluate(m, []string{"core"}, "linux")
	if want := []string{"tool", "set-only", "entry-only", "prov-only"}; dependencyNames(got.Dependencies) == nil || !reflect.DeepEqual(dependencyNames(got.Dependencies), want) {
		t.Fatalf("Dependencies = %#v, want %#v", got.Dependencies, want)
	}
	if !got.Dependencies[0].IsRequired() {
		t.Fatalf("first dependency = %#v, want required after promotion", got.Dependencies[0])
	}
	if want := []string{"dependency_set", "dependency_set", "entry", "entry", "provisioner", "provisioner"}; !reflect.DeepEqual(originTypes(got.DependencyOrigins), want) {
		t.Fatalf("DependencyOrigins = %#v", got.DependencyOrigins)
	}
	if got.DependencyOrigins[0].Dependency.Name != "tool" || got.DependencyOrigins[2].Origin.Name != "target" || got.DependencyOrigins[4].Origin.Name != "prov" {
		t.Fatalf("DependencyOrigins do not retain declaration and origin: %#v", got.DependencyOrigins)
	}
}

func TestEvaluateResolvesLastTagOverrideAndKeepsActiveOverrides(t *testing.T) {
	selected := manifest.Entry{Source: "base", Target: "selected", Strategy: "copy", Tags: []string{"core"}, SourceOverrides: map[string]string{"theme": "theme-source", "work": "work-source"}}
	unselected := manifest.Entry{Source: "other", Target: "unselected", Strategy: "copy", Tags: []string{"other"}, SourceOverrides: map[string]string{"theme": "other-theme"}}
	m := manifest.Manifest{Entries: []manifest.Entry{selected, unselected, {Source: "darwin", Target: "darwin", Strategy: "copy", Tags: []string{"core"}, OS: []string{"darwin"}, SourceOverrides: map[string]string{"theme": "skip"}}}}
	got := selectedsurface.Evaluate(m, []string{"core", "theme", "work"}, "linux")
	if len(got.Entries) != 1 || got.Entries[0].Source != "work-source" || got.Entries[0].OverrideTag != "work" {
		t.Fatalf("Entries = %#v", got.Entries)
	}
	if want := []string{"theme-source", "work-source", "other-theme"}; !reflect.DeepEqual(overrideSources(got.SourceOverrides), want) {
		t.Fatalf("SourceOverrides = %#v, want %#v", got.SourceOverrides, want)
	}
}

func TestEvaluateSameTagsHaveNoProvenanceInputOrOutput(t *testing.T) {
	m := manifest.Manifest{Entries: []manifest.Entry{{Source: "base", Target: "target", Strategy: "copy", Tags: []string{"core"}}}}
	first := selectedsurface.Evaluate(m, []string{"core"}, "linux")
	second := selectedsurface.Evaluate(m, []string{"core"}, "linux")
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same effective tags must have the same pure surface: %#v != %#v", first, second)
	}
}

func TestRepositoryProfilesMatchEquivalentExplicitTagsAcrossSupportedOS(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for name, profile := range m.Profiles {
		profileSelection, err := manifest.ResolveReadOnlySelection(*m, []string{name}, nil)
		if err != nil {
			t.Fatalf("resolve Profile %q: %v", name, err)
		}
		tagSelection, err := manifest.ResolveReadOnlySelection(*m, nil, profile.Tags)
		if err != nil {
			t.Fatalf("resolve explicit Tags for %q: %v", name, err)
		}
		for _, osName := range []string{"darwin", "linux"} {
			t.Run(name+"/"+osName, func(t *testing.T) {
				fromProfile := selectedsurface.Evaluate(*m, profileSelection.Tags, osName)
				fromTags := selectedsurface.Evaluate(*m, tagSelection.Tags, osName)
				if !reflect.DeepEqual(fromProfile, fromTags) {
					t.Fatalf("Profile and explicit Tag surfaces differ\nProfile: %#v\nTags: %#v", fromProfile, fromTags)
				}
			})
		}
	}
}

func dependencyNames(dependencies []manifest.Dependency) []string {
	names := make([]string, len(dependencies))
	for i, dependency := range dependencies {
		names[i] = dependency.Name
	}
	return names
}

func originTypes(origins []selectedsurface.DependencyOrigin) []string {
	types := make([]string, len(origins))
	for i, origin := range origins {
		types[i] = origin.Origin.Type
	}
	return types
}

func overrideSources(overrides []selectedsurface.SourceOverride) []string {
	sources := make([]string, len(overrides))
	for i, override := range overrides {
		sources[i] = override.Source
	}
	return sources
}
