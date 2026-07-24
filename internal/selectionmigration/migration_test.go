package selectionmigration

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
)

func TestAnalyzeRecordedProfileHighConfidence(t *testing.T) {
	m, meta, opts := fixture(t)
	meta.Entries[0].Profiles = []string{"core"}

	got, err := Analyze(m, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	want := &Candidate{
		Profiles:           []string{"core"},
		ExtraTags:          []string{},
		EffectiveTags:      []string{"core"},
		Confidence:         ConfidenceHigh,
		AmbiguityReasons:   []AmbiguityReason{},
		RecommendedCommand: "dots install --profile core",
	}
	if !got.Required || !reflect.DeepEqual(got.Candidate, want) {
		t.Fatalf("Analyze() = %#v, want candidate %#v", got, want)
	}
}

func TestAnalyzeInfersUniqueCompleteProfile(t *testing.T) {
	m, meta, opts := fixture(t)
	got, err := Analyze(m, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.Candidate.Confidence != ConfidenceMedium || !reflect.DeepEqual(got.Candidate.Profiles, []string{"core"}) || !got.Candidate.Unambiguous() {
		t.Fatalf("candidate = %#v", got.Candidate)
	}
}

func TestAnalyzeSourceOverrideBecomesExtraTag(t *testing.T) {
	m, meta, opts := fixture(t)
	override := filepath.Join(opts.SourceRoot, "configs", "work")
	if err := os.WriteFile(override, []byte("work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(meta.Entries[0].Target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(override, meta.Entries[0].Target); err != nil {
		t.Fatal(err)
	}
	m.Entries[0].SourceOverrides = map[string]string{"work": "configs/work"}
	meta.Entries[0].Source = "configs/work"
	meta.Entries[0].Profiles = []string{"core"}

	got, err := Analyze(m, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Candidate.ExtraTags, []string{"work"}) ||
		!reflect.DeepEqual(got.Candidate.EffectiveTags, []string{"core", "work"}) ||
		!got.Candidate.Unambiguous() {
		t.Fatalf("candidate = %#v", got.Candidate)
	}
}

func TestAnalyzeSourceOverrideEvidenceRequiresMatchingTarget(t *testing.T) {
	m, meta, opts := fixture(t)
	m.Entries = append(m.Entries, manifest.Entry{
		Source:          "configs/other",
		Target:          "~/.other",
		Strategy:        "symlink",
		Tags:            []string{"other"},
		SourceOverrides: map[string]string{"unrelated": "configs/x"},
	})
	meta.Entries[0].Profiles = []string{"core"}

	got, err := Analyze(m, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	if contains(got.Candidate.ExtraTags, "unrelated") {
		t.Fatalf("unrelated source override leaked into candidate: %#v", got.Candidate)
	}
}

func TestAnalyzeMixedMissingRecordedTagsIsAmbiguousAndRetainsInferredTag(t *testing.T) {
	m, meta, opts := fixture(t)
	workSource := filepath.Join(opts.SourceRoot, "configs", "work")
	if err := os.WriteFile(workSource, []byte("work\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	workTarget := filepath.Join(opts.Home, ".work")
	if err := os.Symlink(workSource, workTarget); err != nil {
		t.Fatal(err)
	}
	m.Entries = append(m.Entries, manifest.Entry{
		Source: "configs/work", Target: "~/.work", Strategy: "symlink", Tags: []string{"work"},
	})
	meta.Entries[0].Profiles = []string{"core"}
	meta.Entries = append(meta.Entries, state.Record{
		Source: "configs/work", Target: workTarget, Strategy: "symlink", Profiles: []string{"core"},
	})

	got, err := Analyze(m, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got.Candidate.AmbiguityReasons, ReasonMissingRecordedTags) ||
		!contains(got.Candidate.ExtraTags, "work") ||
		got.Candidate.Unambiguous() {
		t.Fatalf("candidate = %#v", got.Candidate)
	}
}

func TestAnalyzeAmbiguities(t *testing.T) {
	tests := []struct {
		name   string
		change func(*testing.T, *manifest.Manifest, *state.Metadata, Options)
		reason AmbiguityReason
	}{
		{"conflicting history", func(_ *testing.T, m *manifest.Manifest, meta *state.Metadata, _ Options) {
			m.Profiles["work"] = manifest.Profile{Tags: []string{"core"}}
			meta.Entries[0].Profiles = []string{"core"}
			meta.Provisioners = []state.ProvisionerRecord{{Profiles: []string{"work"}, Tool: "missing"}}
		}, "conflicting_recorded_profiles"},
		{"conflicting recorded tags", func(_ *testing.T, _ *manifest.Manifest, meta *state.Metadata, _ Options) {
			meta.Entries[0].Profiles = []string{"core"}
			meta.Provisioners = []state.ProvisionerRecord{{Profiles: []string{"core"}, Tags: []string{"historical"}, Tool: "missing"}}
		}, "conflicting_recorded_tags"},
		{"multiple complete profiles", func(_ *testing.T, m *manifest.Manifest, _ *state.Metadata, _ Options) {
			m.Profiles["also-core"] = manifest.Profile{Tags: []string{"core"}}
		}, "multiple_complete_profiles"},
		{"unmatched record", func(_ *testing.T, _ *manifest.Manifest, meta *state.Metadata, _ Options) {
			meta.Entries[0].Source = "configs/removed"
		}, "unmatched_historical_record"},
		{"target mismatch", func(t *testing.T, _ *manifest.Manifest, meta *state.Metadata, _ Options) {
			if err := os.Remove(meta.Entries[0].Target); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(meta.Entries[0].Target, []byte("different"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "target_source_mismatch"},
		{"missing source", func(t *testing.T, _ *manifest.Manifest, _ *state.Metadata, opts Options) {
			if err := os.Remove(filepath.Join(opts.SourceRoot, "configs", "x")); err != nil {
				t.Fatal(err)
			}
		}, "missing_target_source_evidence"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, meta, opts := fixture(t)
			tt.change(t, &m, &meta, opts)
			got, err := Analyze(m, meta, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !contains(got.Candidate.AmbiguityReasons, tt.reason) || got.Candidate.Confidence != ConfidenceLow || got.Candidate.RecommendedCommand != "" {
				t.Fatalf("candidate = %#v, want reason %q", got.Candidate, tt.reason)
			}
		})
	}
}

func TestAnalyzeNoEvidenceAndNonLegacy(t *testing.T) {
	m, _, opts := fixture(t)
	for _, meta := range []state.Metadata{{Version: 0}, {Version: 3}, {Version: 2, InstalledSelection: &state.InstalledSelection{}}} {
		got, err := Analyze(m, meta, opts)
		if err != nil || got.Required {
			t.Fatalf("Analyze(%#v) = %#v, %v", meta, got, err)
		}
	}
	got, err := Analyze(m, state.Metadata{Version: 2}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(got.Candidate.AmbiguityReasons, "no_historical_evidence") {
		t.Fatalf("candidate = %#v", got.Candidate)
	}
}

func TestAnalyzeOrderingIsDeterministic(t *testing.T) {
	m, meta, opts := fixture(t)
	m.Profiles["z"] = manifest.Profile{Tags: []string{"z", "a"}}
	meta.Entries[0].Profiles = []string{"z", "core", "z"}
	meta.Entries[0].Tags = []string{"z", "core", "a"}
	first, err := Analyze(m, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Analyze(m, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) ||
		!reflect.DeepEqual(first.Candidate.Profiles, []string{"core", "z"}) ||
		!reflect.DeepEqual(first.Candidate.ExtraTags, []string{}) {
		t.Fatalf("results = %#v / %#v", first, second)
	}
}

func fixture(t *testing.T) (manifest.Manifest, state.Metadata, Options) {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "configs"), 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceRoot, "configs", "x")
	target := filepath.Join(home, ".x")
	if err := os.WriteFile(source, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}
	m := manifest.Manifest{
		Profiles: map[string]manifest.Profile{"core": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
		}},
	}
	meta := state.Metadata{Version: 2, Entries: []state.Record{{
		Source: "configs/x", Target: target, Strategy: "symlink", Tags: []string{"core"},
	}}}
	return m, meta, Options{OS: "linux", Home: home, SourceRoot: sourceRoot, StatePath: filepath.Join(root, "installed.json")}
}

func contains[T ~string](values []T, value T) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
