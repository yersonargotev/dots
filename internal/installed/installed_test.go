package installed_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	inst "github.com/yersonargotev/dots/internal/installed"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/state"
)

func TestBuildExposesEverySharedTargetContributor(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".config", "shared.json")
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"base", "mobile"}}},
		Entries: []manifest.Entry{
			{Source: "configs/base.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"base"}},
			{Source: "configs/mobile.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"mobile"}},
		},
	}
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{{
		Target: target, Source: "configs/base.json", Sources: []string{"configs/base.json", "configs/mobile.json"}, Strategy: "copy",
		Profiles: []string{"default"}, Tags: []string{"base", "mobile"},
	}}}

	report, err := inst.Build(m, meta, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	gotSources := make([]string, 0, len(report.ManagedEntries))
	for _, entry := range report.ManagedEntries {
		gotSources = append(gotSources, entry.Source)
		if !entry.ManifestMatched {
			t.Fatalf("ManagedEntry = %+v, want manifest match", entry)
		}
	}
	wantSources := []string{"configs/base.json", "configs/mobile.json"}
	if !reflect.DeepEqual(gotSources, wantSources) {
		t.Fatalf("managed sources = %#v, want %#v", gotSources, wantSources)
	}
	coverage := findProfile(t, report.Profiles, "default")
	if coverage.TotalEntries != 2 || coverage.CoveredEntries != 2 || coverage.State != inst.CoverageComplete {
		t.Fatalf("default coverage = %+v, want both contributors covered", coverage)
	}
}

func TestBuildExplainsAttributedAndLegacyUnattributedOwnership(t *testing.T) {
	home := t.TempDir()
	sharedTarget := filepath.Join(home, ".config", "shared.json")
	legacyTarget := filepath.Join(home, ".legacy")
	m := manifest.Manifest{
		Version: 1,
		Entries: []manifest.Entry{
			{Source: "configs/opencode.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"opencode"}},
			{Source: "configs/antigravity.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"antigravity"}},
			{Source: "configs/legacy", Target: "~/.legacy", Strategy: "copy", Tags: []string{"legacy"}},
		},
	}
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{
		{
			Target: sharedTarget, Source: "configs/opencode.json", Sources: []string{"configs/opencode.json", "configs/antigravity.json"}, Strategy: "copy", Ownership: "json-subset",
			Contributions: []state.Contribution{
				{Source: "configs/opencode.json", SelectorTags: []string{"opencode"}, Ownership: "json-subset", EvidenceRecorded: true, Hash: "one", OwnedContent: []byte(`{"opencode":true}`)},
				{Source: "configs/antigravity.json", SelectorTags: []string{"antigravity"}, Ownership: "json-subset", EvidenceRecorded: true, Hash: "two", OwnedContent: []byte(`{"antigravity":true}`)},
			},
		},
		{Target: legacyTarget, Source: "configs/legacy", Strategy: "copy", Ownership: "whole", Hash: "legacy-hash", Tags: []string{"legacy"}},
	}}

	report, err := inst.Build(m, meta, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if report.InstalledSelection != nil {
		t.Fatalf("InstalledSelection = %+v, want historical inventory to remain non-authoritative", report.InstalledSelection)
	}
	if len(report.ManagedEntries) != 3 {
		t.Fatalf("ManagedEntries = %+v, want two attributed contributors and one legacy record", report.ManagedEntries)
	}
	for i, wantTag := range []string{"opencode", "antigravity"} {
		entry := report.ManagedEntries[i]
		if entry.Attribution != "recorded-contribution" || entry.Ownership != "json-subset" || entry.OwnershipEvidence != "owned-json" ||
			!reflect.DeepEqual(entry.Tags, []string{wantTag}) || entry.TagsSource != "recorded-contribution" {
			t.Fatalf("ManagedEntries[%d] = %+v, want attributed %s JSON contribution", i, entry, wantTag)
		}
	}
	legacy := report.ManagedEntries[2]
	if legacy.Attribution != "legacy-unattributed" || legacy.Ownership != "whole" || legacy.OwnershipEvidence != "legacy-target-wide" {
		t.Fatalf("legacy Managed Entry = %+v, want explicit legacy-unattributed explanation", legacy)
	}
}

func TestBuildReportsRecordedEmptyOwnershipEvidence(t *testing.T) {
	home := t.TempDir()
	tests := []struct {
		ownership string
		evidence  string
	}{
		{ownership: "toml-subset", evidence: "owned-toml"},
		{ownership: "seeded", evidence: "seeded-baseline"},
	}
	for _, tt := range tests {
		t.Run(tt.ownership, func(t *testing.T) {
			target := filepath.Join(home, tt.ownership)
			m := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{
				Source: "configs/empty", Target: "~/" + tt.ownership, Strategy: "copy", Ownership: tt.ownership, Tags: []string{"empty"},
			}}}
			meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{{
				Target: target, Source: "configs/empty", Strategy: "copy", Ownership: tt.ownership,
				Contributions: []state.Contribution{{
					Source: "configs/empty", SelectorTags: []string{"empty"}, Ownership: tt.ownership, EvidenceRecorded: true,
				}},
			}}}
			report, err := inst.Build(m, meta, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if len(report.ManagedEntries) != 1 || report.ManagedEntries[0].OwnershipEvidence != tt.evidence {
				t.Fatalf("ManagedEntries = %+v, want empty %s evidence reported as %s", report.ManagedEntries, tt.ownership, tt.evidence)
			}
		})
	}
}

func TestBuildKeepsRecordedContributionTagProvenanceWhenSelectorTagsAreMissing(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".config", "shared.json")
	m := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{
		Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"shared"},
	}}}
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{{
		Target: target, Source: "configs/shared.json", Strategy: "copy", Ownership: "json-subset",
		Contributions: []state.Contribution{{
			Source: "configs/shared.json", Ownership: "json-subset",
		}},
	}}}

	report, err := inst.Build(m, meta, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(report.ManagedEntries) != 1 {
		t.Fatalf("ManagedEntries = %+v, want one attributed entry", report.ManagedEntries)
	}
	entry := report.ManagedEntries[0]
	if entry.Attribution != "recorded-contribution" || entry.TagsSource != "recorded-contribution" || entry.OwnershipEvidence != "missing" || len(entry.Tags) != 0 {
		t.Fatalf("ManagedEntry = %+v, want incomplete recorded-contribution provenance", entry)
	}
	if got := strings.Join(report.Notes, "\n"); !strings.Contains(got, "do not record selector Tags") {
		t.Fatalf("Notes = %q, want missing selector Tags explanation", got)
	}
}

func TestBuildMatchesXDGStateTargetRoot(t *testing.T) {
	home := t.TempDir()
	xdgStateHome := filepath.Join(home, ".local", "state")
	target := filepath.Join(xdgStateHome, "nvim", "lazy-lock.json")
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"editor"}}},
		Entries: []manifest.Entry{{
			Source:     "configs/nvim/lazy-lock.json",
			Target:     "nvim/lazy-lock.json",
			TargetRoot: "xdg-state",
			Strategy:   "copy",
			Ownership:  "seeded",
			Tags:       []string{"editor"},
		}},
	}
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{{
		Target: target, Source: "configs/nvim/lazy-lock.json", Strategy: "copy",
		Profiles: []string{"default"}, Tags: []string{"editor"},
	}}}

	report, err := inst.Build(m, meta, inst.Options{
		StatePath:    filepath.Join(home, "installed.json"),
		Home:         home,
		XDGStateHome: xdgStateHome,
		OS:           "linux",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(report.ManagedEntries) != 1 || !report.ManagedEntries[0].ManifestMatched {
		t.Fatalf("ManagedEntries = %+v, want one manifest match", report.ManagedEntries)
	}
	coverage := findProfile(t, report.Profiles, "default")
	if coverage.TotalEntries != 1 || coverage.CoveredEntries != 1 || coverage.State != inst.CoverageComplete {
		t.Fatalf("default coverage = %+v, want complete 1/1 coverage", coverage)
	}
}

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

func TestBuildExposesAuthoritativeInstalledSelectionSeparatelyFromInventory(t *testing.T) {
	home := t.TempDir()
	selection := &state.InstalledSelection{
		Profiles:     []string{"core", "agents"},
		ExtraTags:    []string{"work"},
		ResolvedTags: []string{"core", "agents", "work"},
		Provenance: state.Provenance{
			SourceRoot:     "/src",
			SourceRevision: "abc123",
			DotsVersion:    "v0.test",
			RecordedAt:     "2026-07-23T12:00:00Z",
		},
	}
	meta := state.Metadata{
		Version:            3,
		InstalledSelection: selection,
		Entries: []state.Record{{
			Target:   filepath.Join(home, ".zshrc"),
			Source:   "configs/zsh/zshrc",
			Strategy: "symlink",
			Profiles: []string{"core"},
			Tags:     []string{"core"},
		}},
	}

	report, err := inst.Build(inventoryManifest(), meta, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if report.InstalledSelection != selection {
		t.Fatalf("InstalledSelection = %#v, want authoritative metadata selection %#v", report.InstalledSelection, selection)
	}
	if got, want := report.Tags, []string{"core"}; !sameStrings(got, want) {
		t.Fatalf("inventory Tags = %v, want %v; resolved Installed Selection Tags must remain separate", got, want)
	}
}

func TestBuildDoesNotPromoteLegacyInventoryToInstalledSelection(t *testing.T) {
	home := t.TempDir()
	meta := state.Metadata{Version: 2, Entries: []state.Record{{
		Target:   filepath.Join(home, ".zshrc"),
		Source:   "configs/zsh/zshrc",
		Strategy: "symlink",
		Profiles: []string{"core"},
		Tags:     []string{"core"},
	}}}

	report, err := inst.Build(inventoryManifest(), meta, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if report.InstalledSelection != nil {
		t.Fatalf("InstalledSelection = %#v, want nil for legacy metadata", report.InstalledSelection)
	}
}

func TestBuildUsesSelectedSurfaceForCurrentProfileCoverageWithoutPromotingInventory(t *testing.T) {
	home := t.TempDir()
	entry := manifest.Entry{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}, OS: []string{"linux"}}
	m := manifest.Manifest{
		Profiles: map[string]manifest.Profile{"core": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{entry, entry},
	}
	meta := state.Metadata{Version: 2, Entries: []state.Record{{
		Target: filepath.Join(home, ".zshrc"), Source: entry.Source, Strategy: entry.Strategy, Tags: []string{"core"},
	}}}
	report, err := inst.Build(m, meta, inst.Options{StatePath: filepath.Join(home, "installed.json"), Home: home, OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	coverage := findProfile(t, report.Profiles, "core")
	if coverage.TotalEntries != 1 || coverage.CoveredEntries != 1 || coverage.State != inst.CoverageComplete {
		t.Fatalf("core coverage = %+v, want de-duplicated current Selected Surface coverage", coverage)
	}
	if report.InstalledSelection != nil || !sameStrings(report.Tags, []string{"core"}) {
		t.Fatalf("historical inventory was promoted: selection=%+v tags=%v", report.InstalledSelection, report.Tags)
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
		Provisioners: []manifest.Provisioner{{Tool: "claude", Tags: []string{"agents"}, Spec: manifest.ProvisionerSpec{Marketplace: "example/tools"}}},
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
