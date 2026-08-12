package catalog

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

func TestBuildHidesLegacyAndSortsDeclaredCatalog(t *testing.T) {
	m := fixtureManifest()
	got, err := Build(m, Options{OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	if got.MetadataOrigin != MetadataDeclared {
		t.Fatalf("MetadataOrigin = %q", got.MetadataOrigin)
	}
	if got.Hidden != (Hidden{Profiles: 1, Tags: 1}) {
		t.Fatalf("Hidden = %#v", got.Hidden)
	}
	if names := profileNames(got.Profiles); !reflect.DeepEqual(names, []string{"core", "desktop"}) {
		t.Fatalf("profiles = %#v", names)
	}
	if names := tagNames(got.Tags); !reflect.DeepEqual(names, []string{"agents", "cleanup", "core", "theme"}) {
		t.Fatalf("tags = %#v", names)
	}
}

func TestTagDetailIsPortableSafeAndDescribesExclusions(t *testing.T) {
	m := fixtureManifest()
	got, err := Tag(m, "theme", Options{OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	detail := got.Tag
	if detail == nil {
		t.Fatal("Tag detail = nil")
	}
	if len(detail.SourceOverrides) != 1 || detail.SourceOverrides[0].Source != "adaptive" || detail.SourceOverrides[0].Applicable {
		t.Fatalf("SourceOverrides = %#v", detail.SourceOverrides)
	}
	if len(detail.Excluded) != 3 {
		t.Fatalf("Excluded = %#v", detail.Excluded)
	}
	all, err := Tag(m, "theme", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Tag.Provisioners) != 1 {
		t.Fatalf("Provisioners = %#v", all.Tag.Provisioners)
	}
	p := all.Tag.Provisioners[0]
	if !reflect.DeepEqual(p.EnvironmentNames, []string{"PUBLIC", "SECRET"}) {
		t.Fatalf("EnvironmentNames = %#v", p.EnvironmentNames)
	}
	if reflect.DeepEqual(p.Command, []string{"hidden"}) {
		t.Fatal("unexpected command")
	}
	if p.Identity != "demo" || p.Operation != "mcp" {
		t.Fatalf("Provisioner = %#v", p)
	}
}

func TestDetailUsesSelectedSurfaceForResolvedEntriesAndActiveOverrides(t *testing.T) {
	m := manifest.Manifest{
		Profiles: map[string]manifest.Profile{
			"theme": {Tags: []string{"core", "theme"}},
		},
		Dependencies: []manifest.DependencySet{{
			Tags:         []string{"core"},
			Dependencies: []manifest.Dependency{{Name: "set-tool"}},
		}},
		Entries: []manifest.Entry{
			{Source: "base", Target: "selected", Strategy: "copy", Tags: []string{"core"}, SourceOverrides: map[string]string{"theme": "selected-theme"}, Dependencies: []manifest.Dependency{{Name: "entry-tool"}}},
			{Source: "other", Target: "unselected", Strategy: "copy", Tags: []string{"other"}, SourceOverrides: map[string]string{"theme": "unselected-theme"}},
		},
		Provisioners: []manifest.Provisioner{{
			Tool:         "theme-tool",
			Tags:         []string{"theme"},
			Dependencies: []manifest.Dependency{{Name: "provisioner-tool"}},
		}},
	}

	got, err := Profile(m, "theme", Options{OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	detail := got.Profile
	if detail == nil {
		t.Fatal("Profile detail = nil")
	}
	if len(detail.Entries) != 1 || detail.Entries[0].Source != "selected-theme" {
		t.Fatalf("Entries = %#v", detail.Entries)
	}
	if got := detail.Dependencies; !reflect.DeepEqual(dependencyNames(got), []string{"set-tool", "entry-tool", "provisioner-tool"}) {
		t.Fatalf("Dependency origins = %#v", got)
	}
	if got := detail.SourceOverrides; !reflect.DeepEqual(overrideSources(got), []string{"selected-theme", "unselected-theme"}) || !allApplicable(got) {
		t.Fatalf("active SourceOverrides = %#v", got)
	}
}

func TestAllOSDetailCombinesSelectedSurfacesInManifestOrder(t *testing.T) {
	m := manifest.Manifest{
		Profiles: map[string]manifest.Profile{"core": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{Source: "linux", Target: "linux", Strategy: "copy", Tags: []string{"core"}, OS: []string{"linux"}},
			{Source: "portable", Target: "portable", Strategy: "copy", Tags: []string{"core"}},
			{Source: "darwin", Target: "darwin", Strategy: "copy", Tags: []string{"core"}, OS: []string{"darwin"}},
		},
	}

	got, err := Profile(m, "core", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if targets := entryTargets(got.Profile.Entries); !reflect.DeepEqual(targets, []string{"linux", "portable", "darwin"}) {
		t.Fatalf("Entries = %#v", targets)
	}
}

func TestRegistrylessCatalogDerivesSurfaceTags(t *testing.T) {
	m := fixtureManifest()
	m.Tags = nil
	got, err := Build(m, Options{OS: "all", IncludeLegacy: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.MetadataOrigin != MetadataDerived {
		t.Fatalf("MetadataOrigin = %q", got.MetadataOrigin)
	}
	for _, tag := range got.Tags {
		if tag.Origin != MetadataDerived || tag.Kind != "surface" || tag.Status != "current" {
			t.Fatalf("derived tag = %#v", tag)
		}
	}
}

func TestProfileDetailIncludesOriginWithoutBuiltInTagBehavior(t *testing.T) {
	got, err := Profile(fixtureManifest(), "core", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	detail := got.Profile
	if detail == nil || !reflect.DeepEqual(detail.ResolvedTags, []string{"core", "agents"}) {
		t.Fatalf("Detail = %#v", detail)
	}
	if len(detail.ProfileDependencies) != 0 {
		t.Fatalf("profile dependencies = %#v, want empty compatibility field", detail.ProfileDependencies)
	}
	if len(detail.Behaviors) != 0 {
		t.Fatalf("behaviors = %#v, want Tags to remain declarative selection only", detail.Behaviors)
	}
}

func TestCompareProfilesDescribesPortableSurfaceDelta(t *testing.T) {
	got, err := CompareProfiles(fixtureManifest(), "core", "desktop", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	comparison := got.Comparison
	if comparison == nil || comparison.From != "core" || comparison.To != "desktop" {
		t.Fatalf("Comparison = %#v", comparison)
	}
	if !reflect.DeepEqual(comparison.Added.ResolvedTags, []string{"theme"}) {
		t.Fatalf("added tags = %#v", comparison.Added.ResolvedTags)
	}
	if len(comparison.Added.Entries) != 1 || comparison.Added.Entries[0].Source != "adaptive" {
		t.Fatalf("added entries = %#v", comparison.Added.Entries)
	}
	if len(comparison.Added.Provisioners) != 1 || comparison.Added.Provisioners[0].Identity != "demo" {
		t.Fatalf("added provisioners = %#v", comparison.Added.Provisioners)
	}
	if len(comparison.Removed.ResolvedTags) != 0 || comparison.Shared.ResolvedTags != 2 {
		t.Fatalf("comparison tags = added %#v removed %#v shared %d", comparison.Added.ResolvedTags, comparison.Removed.ResolvedTags, comparison.Shared.ResolvedTags)
	}
	if comparison.Shared.Dependencies != 1 {
		t.Fatalf("shared dependencies = %d, want set-tool", comparison.Shared.Dependencies)
	}
	if got.Profile != nil {
		t.Fatalf("Profile = %#v, want nil in comparison report", got.Profile)
	}
}

func TestCompareProfilesRejectsUnknownProfile(t *testing.T) {
	_, err := CompareProfiles(fixtureManifest(), "core", "missing", Options{OS: "all"})
	if err == nil || err.Error() != `profile "missing" not found` {
		t.Fatalf("error = %v", err)
	}
}

func TestMapProfileCountsTagSurfacesAndDeduplicatesTotalDependencies(t *testing.T) {
	m := fixtureManifest()
	m.Dependencies = append(m.Dependencies, manifest.DependencySet{
		Tags:         []string{"agents"},
		Dependencies: []manifest.Dependency{{Name: "set-tool"}, {Name: "agent-tool"}},
	})

	got, err := MapProfile(m, "desktop", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != nil || got.Map == nil {
		t.Fatalf("report = %#v", got)
	}
	if got.Map.Profile != "desktop" || got.Map.Status != "current" {
		t.Fatalf("map identity = %#v", got.Map)
	}
	wantTags := []TagSurface{
		{Name: "core", Description: "Core", Surface: SurfaceCount{Dependencies: 1}},
		{Name: "agents", Description: "Agents", Surface: SurfaceCount{Dependencies: 2}},
		{Name: "theme", Description: "Theme", Surface: SurfaceCount{Entries: 1, Dependencies: 2, Provisioners: 1}},
	}
	if !reflect.DeepEqual(got.Map.Tags, wantTags) {
		t.Fatalf("tag surfaces = %#v, want %#v", got.Map.Tags, wantTags)
	}
	wantTotal := SurfaceCount{Entries: 1, Dependencies: 4, Provisioners: 1}
	if got.Map.Total != wantTotal {
		t.Fatalf("total = %#v, want %#v", got.Map.Total, wantTotal)
	}
}

func TestMapProfileRejectsUnknownProfile(t *testing.T) {
	_, err := MapProfile(fixtureManifest(), "missing", Options{OS: "all"})
	if err == nil || err.Error() != `profile "missing" not found` {
		t.Fatalf("error = %v", err)
	}
}

func TestExplainProfileItemPreservesDependencyOrigins(t *testing.T) {
	m := manifest.Manifest{
		Profiles: map[string]manifest.Profile{"workstation": {Tags: []string{"core", "agents"}}},
		Dependencies: []manifest.DependencySet{{
			Tags:         []string{"core"},
			Dependencies: []manifest.Dependency{{Name: "shared-tool"}},
		}},
		Entries: []manifest.Entry{{
			Source: "base", Target: "~/.shared", Strategy: "copy", Tags: []string{"agents"},
			Dependencies: []manifest.Dependency{{Name: "shared-tool", Requirement: manifest.DependencyRequirementRequired}},
		}},
	}

	got, err := ExplainProfileItem(m, "workstation", "shared-tool", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile != nil || got.Why == nil {
		t.Fatalf("report = %#v", got)
	}
	if !reflect.DeepEqual(got.Why.ResolvedTags, []string{"core", "agents"}) {
		t.Fatalf("resolved tags = %#v", got.Why.ResolvedTags)
	}
	if len(got.Why.Matches) != 1 {
		t.Fatalf("matches = %#v", got.Why.Matches)
	}
	match := got.Why.Matches[0]
	if match.Type != "dependency" || !reflect.DeepEqual(match.ContributingTags, []string{"core", "agents"}) {
		t.Fatalf("match = %#v", match)
	}
	if match.Dependency == nil || len(match.Dependency.Declarations) != 2 {
		t.Fatalf("dependency = %#v", match.Dependency)
	}
	if got := match.Dependency.Declarations[1].Origin; got.Type != "entry" || got.Name != "~/.shared" {
		t.Fatalf("second origin = %#v", got)
	}
}

func TestExplainProfileItemReportsEntryOverrideAndProvisioners(t *testing.T) {
	m := fixtureManifest()

	entryReport, err := ExplainProfileItem(m, "desktop", "adaptive", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	entry := entryReport.Why.Matches[0]
	if entry.Type != "entry" || entry.Identity != "~/.example" || entry.Entry == nil {
		t.Fatalf("entry match = %#v", entry)
	}
	if entry.Entry.SourceOverrideTag != "theme" || !reflect.DeepEqual(entry.ContributingTags, []string{"theme"}) {
		t.Fatalf("entry explanation = %#v", entry)
	}

	provisionerReport, err := ExplainProfileItem(m, "desktop", "codex", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	provisioner := provisionerReport.Why.Matches[0]
	if provisioner.Type != "provisioner" || provisioner.Identity != "demo" || provisioner.Provisioner == nil {
		t.Fatalf("provisioner match = %#v", provisioner)
	}
	if provisioner.Provisioner.Operation != "mcp" || !reflect.DeepEqual(provisioner.ContributingTags, []string{"theme"}) {
		t.Fatalf("provisioner explanation = %#v", provisioner)
	}
}

func TestExplainProfileItemHonorsOSAndRejectsUnknownQueries(t *testing.T) {
	m := fixtureManifest()
	_, err := ExplainProfileItem(m, "desktop", "adaptive", Options{OS: "linux"})
	if err == nil || err.Error() != `profile "desktop" does not select an item matching "adaptive" for OS linux` {
		t.Fatalf("error = %v", err)
	}
	_, err = ExplainProfileItem(m, "missing", "adaptive", Options{OS: "all"})
	if err == nil || err.Error() != `profile "missing" not found` {
		t.Fatalf("unknown profile error = %v", err)
	}
}

func fixtureManifest() manifest.Manifest {
	return manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"core":    {Description: "Core", Kind: "surface", Status: "current"},
			"agents":  {Description: "Agents", Kind: "surface", Status: "current"},
			"theme":   {Description: "Theme", Kind: "surface", Status: "current"},
			"cleanup": {Description: "Cleanup", Kind: "cleanup", Status: "current"},
			"old":     {Description: "Old", Kind: "compatibility", Status: "legacy", ReplacedBy: "core"},
		},
		Profiles: map[string]manifest.Profile{
			"core":    {Description: "Core profile", Tags: []string{"core", "agents"}},
			"desktop": {Description: "Desktop profile", Tags: []string{"core", "agents", "theme"}},
			"old":     {Description: "Old profile", Status: "legacy", Tags: []string{"old"}},
		},
		Dependencies: []manifest.DependencySet{{Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "set-tool"}}}},
		Entries:      []manifest.Entry{{Source: "base", SourceOverrides: map[string]string{"theme": "adaptive"}, Target: "~/.example", Strategy: "copy", Tags: []string{"theme"}, OS: []string{"darwin"}, Dependencies: []manifest.Dependency{{Name: "entry-tool"}}}},
		Provisioners: []manifest.Provisioner{{Tool: "codex", Tags: []string{"theme"}, OS: []string{"darwin"}, Spec: manifest.ProvisionerSpec{MCP: "demo", Command: []string{"demo", "serve"}, Env: map[string]string{"SECRET": "must-not-leak", "PUBLIC": "1"}}, Dependencies: []manifest.Dependency{{Name: "provisioner-tool"}}}},
	}
}

func profileNames(items []ProfileSummary) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Name)
	}
	return result
}
func tagNames(items []TagSummary) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Name)
	}
	return result
}

func dependencyNames(items []Dependency) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Name)
	}
	return result
}

func overrideSources(items []SourceOverride) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Source)
	}
	return result
}

func entryTargets(items []Entry) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Target)
	}
	return result
}

func allApplicable(items []SourceOverride) bool {
	for _, item := range items {
		if !item.Applicable {
			return false
		}
	}
	return true
}
