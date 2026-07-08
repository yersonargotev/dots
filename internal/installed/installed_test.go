package installed_test

import (
	"path/filepath"
	"testing"

	inst "github.com/yersonargotev/dots/internal/installed"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/state"
)

func TestBuildInfersLegacyMetadataAndPartialProfiles(t *testing.T) {
	home := t.TempDir()
	m := inventoryManifest()
	prov := m.Provisioners[0]
	executable, args := provision.RenderCommand(prov)
	meta := state.Metadata{
		Version: 1,
		Entries: []state.Record{{
			Target:   filepath.Join(home, ".zshrc"),
			Source:   "configs/zsh/zshrc",
			Strategy: "symlink",
		}},
		Provisioners: []state.ProvisionerRecord{{
			Profile:    "agents",
			Tool:       prov.Tool,
			Executable: executable,
			Args:       args,
			Status:     string(provision.RunStatusProvisioned),
		}},
	}

	report, err := inst.Build(m, meta, inst.Options{StatePath: filepath.Join(home, ".local/state/dots/installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if got, want := report.Tags, []string{"core", "agents"}; !sameStrings(got, want) {
		t.Fatalf("Tags = %v, want %v", got, want)
	}
	if len(report.ManagedEntries) != 1 || report.ManagedEntries[0].TagsSource != "inferred-from-manifest" || report.ManagedEntries[0].ProfilesSource != "unknown" {
		t.Fatalf("legacy entry inference = %+v", report.ManagedEntries)
	}
	if len(report.Provisioners) != 1 || report.Provisioners[0].ProfilesSource != "recorded" || report.Provisioners[0].TagsSource != "inferred-from-manifest" {
		t.Fatalf("provisioner inference = %+v", report.Provisioners)
	}

	workstation := findProfile(t, report.Profiles, "workstation")
	if workstation.Source != "inferred" || workstation.State != inst.CoveragePartial {
		t.Fatalf("workstation coverage = %+v, want inferred partial", workstation)
	}
	if got, want := workstation.CoveredTags, []string{"core", "agents"}; !sameStrings(got, want) {
		t.Fatalf("workstation covered tags = %v, want %v", got, want)
	}
	if got, want := workstation.MissingTags, []string{"desktop"}; !sameStrings(got, want) {
		t.Fatalf("workstation missing tags = %v, want %v", got, want)
	}

	agents := findProfile(t, report.Profiles, "agents")
	if agents.Source != "recorded+inferred" || agents.State != inst.CoverageComplete || agents.ProvisionedProvisioners != 1 {
		t.Fatalf("agents coverage = %+v, want recorded+inferred complete with provisioner", agents)
	}
	if len(report.Notes) == 0 {
		t.Fatal("legacy metadata should produce explanatory notes")
	}
}

func TestBuildUsesRecordedProfilesAndTags(t *testing.T) {
	home := t.TempDir()
	m := inventoryManifest()
	meta := state.Metadata{Version: 2, Provenance: state.Provenance{SourceRoot: "/src", SourceRevision: "abc123", DotsVersion: "v0.test"}, Entries: []state.Record{{
		Target:   filepath.Join(home, ".zshrc"),
		Source:   "configs/zsh/zshrc",
		Strategy: "symlink",
		Profiles: []string{"core"},
		Tags:     []string{"core"},
	}}}

	report, err := inst.Build(m, meta, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	core := findProfile(t, report.Profiles, "core")
	if core.Source != "recorded+inferred" || core.State != inst.CoverageComplete {
		t.Fatalf("core coverage = %+v, want recorded+inferred complete", core)
	}
	if report.Provenance.SourceRevision != "abc123" {
		t.Fatalf("provenance = %+v", report.Provenance)
	}
}

func TestBuildReturnsEmptyListsForEmptyMetadata(t *testing.T) {
	home := t.TempDir()
	report, err := inst.Build(inventoryManifest(), state.Metadata{}, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if report.ManagedEntries == nil {
		t.Fatal("ManagedEntries = nil, want empty list for stable JSON output")
	}
	if report.Tags == nil {
		t.Fatal("Tags = nil, want empty list for stable JSON output")
	}
	if report.Profiles == nil {
		t.Fatal("Profiles = nil, want empty list for stable JSON output")
	}
	if report.Provisioners == nil {
		t.Fatal("Provisioners = nil, want empty list for stable JSON output")
	}
}

func inventoryManifest() manifest.Manifest {
	return manifest.Manifest{
		Profiles: map[string]manifest.Profile{
			"core":        {Tags: []string{"core"}},
			"desktop":     {Tags: []string{"desktop"}},
			"agents":      {Tags: []string{"agents"}},
			"workstation": {Tags: []string{"core", "desktop", "agents"}},
		},
		Entries: []manifest.Entry{
			{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}},
			{Source: "configs/ghostty/config", Target: "~/.config/ghostty/config", Strategy: "symlink", Tags: []string{"desktop"}},
		},
		Provisioners: []manifest.Provisioner{{Tool: "gentle-ai", Tags: []string{"agents"}, Spec: manifest.ProvisionerSpec{Scope: "global", Agents: []string{"codex"}}}},
	}
}

func findProfile(t *testing.T, profiles []inst.ProfileCoverage, name string) inst.ProfileCoverage {
	t.Helper()
	for _, profile := range profiles {
		if profile.Name == name {
			return profile
		}
	}
	t.Fatalf("profile %q not found in %+v", name, profiles)
	return inst.ProfileCoverage{}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
