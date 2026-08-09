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
	if names := profileNames(got.Profiles); !reflect.DeepEqual(names, []string{"core"}) {
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

func TestProfileDetailIncludesOriginAndBehavior(t *testing.T) {
	got, err := Profile(fixtureManifest(), "core", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	detail := got.Profile
	if detail == nil || !reflect.DeepEqual(detail.ResolvedTags, []string{"core", "agents"}) {
		t.Fatalf("Detail = %#v", detail)
	}
	if len(detail.ProfileDependencies) != 1 || detail.ProfileDependencies[0].Origin.Type != "profile" {
		t.Fatalf("profile dependencies = %#v", detail.ProfileDependencies)
	}
	if len(detail.Behaviors) != 1 || detail.Behaviors[0].Action != "retire-gentle-ai-state" {
		t.Fatalf("behaviors = %#v", detail.Behaviors)
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
			"core": {Description: "Core profile", Tags: []string{"core", "agents"}, Dependencies: []manifest.Dependency{{Name: "profile-tool"}}},
			"old":  {Description: "Old profile", Status: "legacy", Tags: []string{"old"}},
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
