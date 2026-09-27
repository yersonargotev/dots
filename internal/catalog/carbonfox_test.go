package catalog

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

func TestRepositoryCatalogExposesCarbonfoxAsGlobalPreferenceOnly(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	report, err := Build(*m, Options{OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	var carbonfox *TagSummary
	for index := range report.Tags {
		if report.Tags[index].Name == "theme-carbonfox" {
			carbonfox = &report.Tags[index]
			break
		}
	}
	if carbonfox == nil {
		t.Fatal("catalog omitted current theme-carbonfox Tag")
	}
	if carbonfox.Kind != "surface" || carbonfox.Status != "current" || carbonfox.Origin != MetadataDeclared {
		t.Fatalf("theme-carbonfox summary = %#v", carbonfox)
	}
	for name, profile := range m.Profiles {
		if catalogHasTag(profile.Tags, "theme-carbonfox") {
			t.Errorf("Profile %q implicitly selects theme-carbonfox", name)
		}
	}

	tagReport, err := Tag(*m, "theme-carbonfox", Options{OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	detail := tagReport.Tag
	if !reflect.DeepEqual(detail.ResolvedTags, []string{"theme-carbonfox"}) {
		t.Fatalf("resolved Tags = %#v", detail.ResolvedTags)
	}
	if len(detail.Entries) != 1 || detail.Entries[0].Source != "configs/dots/theme-carbonfox" || detail.Entries[0].Target != "~/.config/dots/theme-carbonfox" {
		t.Fatalf("theme-only entries = %#v, want only the shared preference marker", detail.Entries)
	}
	if len(detail.Dependencies) != 0 || len(detail.DependencySets) != 0 || len(detail.Provisioners) != 0 {
		t.Fatalf("theme-only selection pulled application capabilities: dependencies=%#v sets=%#v provisioners=%#v", detail.Dependencies, detail.DependencySets, detail.Provisioners)
	}
	if len(detail.SourceOverrides) == 0 {
		t.Fatal("theme detail omitted declarative application override relationships")
	}
}

func TestCatalogDerivesRequiredOnlyTagWithoutSelectingProvisioner(t *testing.T) {
	m := manifest.Manifest{Provisioners: []manifest.Provisioner{{
		Tool: "bat", Tags: []string{"bat"}, RequiredTags: []string{"theme-carbonfox"},
		Spec: manifest.ProvisionerSpec{Cache: "build"},
	}}}
	report, err := Build(m, Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if got := tagNames(report.Tags); !reflect.DeepEqual(got, []string{"bat", "theme-carbonfox"}) {
		t.Fatalf("derived Tags = %#v", got)
	}
	if report.Tags[1].Origin != MetadataDerived {
		t.Fatalf("required-only Tag origin = %q", report.Tags[1].Origin)
	}
	theme, err := Tag(m, "theme-carbonfox", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(theme.Tag.Provisioners) != 0 || len(theme.Tag.Dependencies) != 0 || len(theme.Tag.Entries) != 0 {
		t.Fatalf("required-only preference selected a surface: %#v", theme.Tag)
	}
}

func TestCatalogRequiredTagsUseAuthoritativeSelectedAndExcludedSurfaces(t *testing.T) {
	m := carbonfoxCatalogFixture()

	bareDarwin := mustCatalogProfile(t, m, "bare", Options{OS: "darwin"}).Profile
	if tools := catalogProvisionerTools(bareDarwin.Provisioners); !reflect.DeepEqual(tools, []string{"zimfw"}) {
		t.Fatalf("bare Darwin provisioners = %#v, want only unconditional provisioner", tools)
	}

	themedDarwin := mustCatalogProfile(t, m, "themed", Options{OS: "darwin"}).Profile
	if tools := catalogProvisionerTools(themedDarwin.Provisioners); !reflect.DeepEqual(tools, []string{"codex", "zimfw"}) {
		t.Fatalf("themed Darwin provisioners = %#v", tools)
	}
	conditional := themedDarwin.Provisioners[0]
	if !reflect.DeepEqual(conditional.RequiredTags, []string{"theme-carbonfox"}) {
		t.Fatalf("conditional required Tags = %#v", conditional.RequiredTags)
	}

	bareLinux := mustCatalogProfile(t, m, "bare", Options{OS: "linux"}).Profile
	if names := catalogExcludedProvisioners(bareLinux.Excluded); !reflect.DeepEqual(names, []string{"zimfw"}) {
		t.Fatalf("bare Linux excluded provisioners = %#v; unmet conditional provisioner must stay absent", names)
	}
	themedLinux := mustCatalogProfile(t, m, "themed", Options{OS: "linux"}).Profile
	if names := catalogExcludedProvisioners(themedLinux.Excluded); !reflect.DeepEqual(names, []string{"codex", "zimfw"}) {
		t.Fatalf("themed Linux excluded provisioners = %#v", names)
	}

	all := mustCatalogProfile(t, m, "themed", Options{OS: "all"}).Profile
	if tools := catalogProvisionerTools(all.Provisioners); !reflect.DeepEqual(tools, []string{"codex", "zimfw"}) || len(all.Excluded) != 0 {
		t.Fatalf("all-platform themed surface = provisioners %#v, excluded %#v", tools, all.Excluded)
	}

	conditionalJSON, err := json.Marshal(conditional)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conditionalJSON), `"required_tags":["theme-carbonfox"]`) {
		t.Fatalf("conditional provisioner JSON omitted required_tags: %s", conditionalJSON)
	}
	unconditionalJSON, err := json.Marshal(themedDarwin.Provisioners[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(unconditionalJSON), `"required_tags"`) {
		t.Fatalf("unconditional provisioner JSON emitted empty optional field: %s", unconditionalJSON)
	}

	themedDarwin.Provisioners[0].RequiredTags[0] = "mutated"
	if got := m.Provisioners[0].RequiredTags; !reflect.DeepEqual(got, []string{"theme-carbonfox"}) {
		t.Fatalf("catalog RequiredTags aliases manifest: %#v", got)
	}
}

func TestCatalogBatCacheIdentityAndExplanationIncludeRequiredPreference(t *testing.T) {
	m := manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"bat":             {Kind: "surface", Status: "current"},
			"theme-carbonfox": {Kind: "surface", Status: "current"},
		},
		Profiles: map[string]manifest.Profile{
			"bare":   {Tags: []string{"bat"}},
			"themed": {Tags: []string{"bat", "theme-carbonfox"}},
		},
		Provisioners: []manifest.Provisioner{{
			Tool: "bat", Tags: []string{"bat"}, RequiredTags: []string{"theme-carbonfox"},
			Spec: manifest.ProvisionerSpec{Cache: "build"},
		}},
	}
	if got := mustCatalogProfile(t, m, "bare", Options{OS: "all"}).Profile.Provisioners; len(got) != 0 {
		t.Fatalf("bare bat Profile selected conditional cache builder: %#v", got)
	}
	detail := mustCatalogProfile(t, m, "themed", Options{OS: "all"}).Profile
	if len(detail.Provisioners) != 1 {
		t.Fatalf("themed bat provisioners = %#v", detail.Provisioners)
	}
	got := detail.Provisioners[0]
	if got.Tool != "bat" || got.Operation != "cache" || got.Identity != "Carbonfox" || !reflect.DeepEqual(got.Command, []string{"bat", "cache", "--build"}) {
		t.Fatalf("bat catalog identity = %#v", got)
	}

	why, err := ExplainProfileItem(m, "themed", "Carbonfox", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(why.Why.Matches) != 1 || !reflect.DeepEqual(why.Why.Matches[0].ContributingTags, []string{"bat", "theme-carbonfox"}) {
		t.Fatalf("bat cache explanation = %#v", why.Why.Matches)
	}
}

func TestCatalogComparisonDistinguishesProvisionerRequiredTags(t *testing.T) {
	m := manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"app":     {Kind: "surface", Status: "current"},
			"theme-a": {Kind: "surface", Status: "current"},
			"theme-b": {Kind: "surface", Status: "current"},
		},
		Profiles: map[string]manifest.Profile{
			"a": {Tags: []string{"app", "theme-a"}},
			"b": {Tags: []string{"app", "theme-b"}},
		},
		Provisioners: []manifest.Provisioner{
			{Tool: "codex", Tags: []string{"app"}, RequiredTags: []string{"theme-a"}, Spec: manifest.ProvisionerSpec{MCP: "same", Command: []string{"same"}}},
			{Tool: "codex", Tags: []string{"app"}, RequiredTags: []string{"theme-b"}, Spec: manifest.ProvisionerSpec{MCP: "same", Command: []string{"same"}}},
		},
	}
	report, err := CompareProfiles(m, "a", "b", Options{OS: "all"})
	if err != nil {
		t.Fatal(err)
	}
	comparison := report.Comparison
	if comparison.Shared.Provisioners != 0 || len(comparison.Added.Provisioners) != 1 || len(comparison.Removed.Provisioners) != 1 {
		t.Fatalf("comparison collapsed distinct required Tags: %#v", comparison)
	}
	if got := comparison.Removed.Provisioners[0].RequiredTags; !reflect.DeepEqual(got, []string{"theme-a"}) {
		t.Fatalf("removed provisioner RequiredTags = %#v", got)
	}
	if got := comparison.Added.Provisioners[0].RequiredTags; !reflect.DeepEqual(got, []string{"theme-b"}) {
		t.Fatalf("added provisioner RequiredTags = %#v", got)
	}
}

func carbonfoxCatalogFixture() manifest.Manifest {
	return manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"app":             {Kind: "surface", Status: "current"},
			"theme-carbonfox": {Kind: "surface", Status: "current"},
		},
		Profiles: map[string]manifest.Profile{
			"bare":   {Tags: []string{"app"}},
			"themed": {Tags: []string{"app", "theme-carbonfox"}},
		},
		Provisioners: []manifest.Provisioner{
			{Tool: "codex", Tags: []string{"app"}, RequiredTags: []string{"theme-carbonfox"}, OS: []string{"darwin"}, Spec: manifest.ProvisionerSpec{MCP: "conditional", Command: []string{"conditional"}}},
			{Tool: "zimfw", Tags: []string{"app"}, OS: []string{"darwin"}},
		},
	}
}

func catalogHasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func catalogProvisionerTools(items []Provisioner) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Tool)
	}
	return result
}

func catalogExcludedProvisioners(items []ExcludedSurface) []string {
	result := []string{}
	for _, item := range items {
		if item.Type == "provisioner" {
			result = append(result, item.Name)
		}
	}
	return result
}
