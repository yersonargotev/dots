package ownershipevidence_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/ownershipevidence"
	"github.com/yersonargotev/dots/internal/state"
)

func TestCaptureAndDiscriminatorByMode(t *testing.T) {
	t.Parallel()

	marked := []byte("# >>> dots managed block >>>\nmanaged\n# <<< dots managed block <<<\n")
	tests := []struct {
		name           string
		strategy       string
		ownership      string
		content        []byte
		baseline       []byte
		wantEvidence   string
		wantOwned      []byte
		wantOwnedBytes []byte
		wantBaseline   []byte
		wantHash       bool
	}{
		{name: "implicit whole copy", strategy: "copy", content: []byte("copy\n"), wantEvidence: ownershipevidence.DiscriminatorSourceHash, wantHash: true},
		{name: "implicit whole symlink", strategy: "symlink", content: []byte("not read"), wantEvidence: ownershipevidence.DiscriminatorSourceIdentity},
		{name: "json subset", strategy: "copy", ownership: "json-subset", content: []byte(`{"owned":true}`), wantEvidence: ownershipevidence.DiscriminatorOwnedJSON, wantOwned: []byte(`{"owned":true}`), wantHash: true},
		{name: "jsonc canonical", strategy: "copy", ownership: "jsonc-subset", content: []byte("{\n  // note\n  \"owned\": true,\n}\n"), wantEvidence: ownershipevidence.DiscriminatorOwnedJSONC, wantOwned: []byte("{\n  \"owned\": true\n}\n"), wantHash: true},
		{name: "empty exact toml", strategy: "copy", ownership: "toml-subset", content: []byte{}, wantEvidence: ownershipevidence.DiscriminatorOwnedTOML, wantOwnedBytes: []byte{}, wantHash: true},
		{name: "marked block", strategy: "copy", ownership: "marked-block", content: marked, wantEvidence: ownershipevidence.DiscriminatorOwnedMarkedBlock, wantOwnedBytes: marked, wantHash: true},
		{name: "seeded recorded baseline", strategy: "copy", ownership: "seeded", content: []byte("current source\n"), baseline: []byte("recorded baseline\n"), wantEvidence: ownershipevidence.DiscriminatorSeededBaseline, wantBaseline: []byte("recorded baseline\n"), wantHash: true},
		{name: "seeded explicit empty baseline", strategy: "copy", ownership: "seeded", content: []byte("current source\n"), baseline: []byte{}, wantEvidence: ownershipevidence.DiscriminatorSeededBaseline, wantBaseline: []byte{}, wantHash: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "source")
			if tt.strategy != "symlink" {
				if err := os.WriteFile(path, tt.content, 0o600); err != nil {
					t.Fatalf("write source: %v", err)
				}
			}

			mode := ownershipevidence.For(tt.strategy, tt.ownership)
			got, err := mode.Capture("configs/source", path, []string{"core"}, tt.baseline)
			if err != nil {
				t.Fatalf("Capture() error = %v", err)
			}
			if got.Source != "configs/source" || !reflect.DeepEqual(got.SelectorTags, []string{"core"}) || got.Ownership != mode.Ownership() || !got.EvidenceRecorded {
				t.Fatalf("Capture() identity = %#v", got)
			}
			if (got.Hash != "") != tt.wantHash {
				t.Errorf("Capture().Hash presence = %v, want %v", got.Hash != "", tt.wantHash)
			}
			if !reflect.DeepEqual([]byte(got.OwnedContent), tt.wantOwned) {
				t.Errorf("Capture().OwnedContent = %q, want %q", got.OwnedContent, tt.wantOwned)
			}
			if !reflect.DeepEqual(got.OwnedBytes, tt.wantOwnedBytes) {
				t.Errorf("Capture().OwnedBytes = %q, want %q", got.OwnedBytes, tt.wantOwnedBytes)
			}
			if !reflect.DeepEqual(got.SeededBaseline, tt.wantBaseline) {
				t.Errorf("Capture().SeededBaseline = %#v, want %#v", got.SeededBaseline, tt.wantBaseline)
			}
			if got := mode.Discriminator(got); got != tt.wantEvidence {
				t.Errorf("Discriminator() = %q, want %q", got, tt.wantEvidence)
			}
		})
	}
}

func TestDiscriminatorReportsMissingEvidence(t *testing.T) {
	t.Parallel()

	mode := ownershipevidence.For("copy", "json-subset")
	if got := mode.Discriminator(state.Contribution{Ownership: "json-subset"}); got != ownershipevidence.DiscriminatorMissing {
		t.Fatalf("Discriminator(incomplete) = %q, want missing", got)
	}
	if got := mode.Discriminator(state.Contribution{Ownership: "whole", EvidenceRecorded: true}); got != ownershipevidence.DiscriminatorMissing {
		t.Fatalf("Discriminator(mismatched) = %q, want missing", got)
	}
}

func TestProjectComposesMultipleJSONContributions(t *testing.T) {
	t.Parallel()

	mode := ownershipevidence.For("copy", "json-subset")
	contributions := []state.Contribution{
		{Source: "base.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: []byte(`{"base":{"one":true}}`)},
		{Source: "mobile.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: []byte(`{"mobile":true,"base":{"two":true}}`)},
	}
	got, err := mode.Project("/home/test/shared.json", contributions)
	if err != nil {
		t.Fatalf("Project() error = %v", err)
	}
	want := "{\n  \"base\": {\n    \"one\": true,\n    \"two\": true\n  },\n  \"mobile\": true\n}\n"
	if string(got.OwnedContent) != want || got.Hash != state.HashBytes([]byte(want)) || !got.EvidenceRecorded || got.Ownership != "json-subset" {
		t.Fatalf("Project() = %#v, want composed JSON and its hash", got)
	}

	got.OwnedContent[0] = '['
	if contributions[0].OwnedContent[0] != '{' {
		t.Fatal("Project() aliased contribution evidence")
	}
}

func TestProjectRejectsInvalidComposition(t *testing.T) {
	t.Parallel()

	jsonMode := ownershipevidence.For("copy", "json-subset")
	tests := []struct {
		name          string
		mode          ownershipevidence.Mode
		contributions []state.Contribution
		want          string
	}{
		{name: "empty", mode: jsonMode, want: "no contributions"},
		{name: "incomplete", mode: jsonMode, contributions: []state.Contribution{{Ownership: "json-subset"}}, want: "incomplete evidence"},
		{name: "mismatched ownership", mode: jsonMode, contributions: []state.Contribution{{Ownership: "whole", EvidenceRecorded: true}}, want: "does not match"},
		{name: "multiple toml", mode: ownershipevidence.For("copy", "toml-subset"), contributions: []state.Contribution{{Ownership: "toml-subset", EvidenceRecorded: true}, {Ownership: "toml-subset", EvidenceRecorded: true}}, want: "require copy/json-subset"},
		{name: "incompatible json", mode: jsonMode, contributions: []state.Contribution{{Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: []byte(`{"value":1}`)}, {Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: []byte(`{"value":2}`)}}, want: "cannot be merged safely"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.mode.Project("/target", tt.contributions)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Project() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateConvergenceAndDriftByCopyMode(t *testing.T) {
	t.Parallel()

	marked := "# >>> dots managed block >>>\nmanaged\n# <<< dots managed block <<<\n"
	tests := []struct {
		name        string
		ownership   string
		source      string
		live        string
		drift       string
		baseline    []byte
		seededFinal []byte
	}{
		{name: "whole", source: "exact\n", live: "exact\n", drift: "changed\n"},
		{name: "json subset", ownership: "json-subset", source: `{"owned":true}`, live: `{"owned":true,"external":1}`, drift: `{"owned":false,"external":1}`},
		{name: "jsonc subset", ownership: "jsonc-subset", source: "{\n// owned\n\"owned\": true,\n}", live: "{\n// kept\n\"owned\": true,\n\"external\": 1,\n}", drift: `{"owned":false}`},
		{name: "toml subset", ownership: "toml-subset", source: "[owned]\nvalue = true\n", live: "external = 1\n[owned]\nvalue = true\n", drift: "[owned]\nvalue = false\n"},
		{name: "marked block", ownership: "marked-block", source: marked, live: "# external\n" + marked, drift: strings.Replace(marked, "managed", "changed", 1)},
		{name: "seeded baseline", ownership: "seeded", source: "baseline\n", live: "baseline\n", drift: "evolved\n"},
		{name: "seeded migration final", ownership: "seeded", source: "current\n", live: "final\n", drift: "other\n", baseline: []byte("baseline\n"), seededFinal: []byte("final\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			source := filepath.Join(dir, "source")
			target := filepath.Join(dir, "target")
			if err := os.WriteFile(source, []byte(tt.source), 0o600); err != nil {
				t.Fatalf("write source: %v", err)
			}
			if err := os.WriteFile(target, []byte(tt.live), 0o600); err != nil {
				t.Fatalf("write target: %v", err)
			}
			mode := ownershipevidence.For("copy", tt.ownership)
			evidence, err := mode.Capture("source", source, nil, tt.baseline)
			if err != nil {
				t.Fatalf("Capture() error = %v", err)
			}
			var validateErr error
			if tt.seededFinal != nil {
				validateErr = mode.Validate(target, []string{source}, evidence, tt.seededFinal)
			} else {
				validateErr = mode.Validate(target, []string{source}, evidence)
			}
			if validateErr != nil {
				t.Fatalf("Validate(converged) error = %v", validateErr)
			}

			if err := os.WriteFile(target, []byte(tt.drift), 0o600); err != nil {
				t.Fatalf("write drift: %v", err)
			}
			if tt.seededFinal != nil {
				validateErr = mode.Validate(target, []string{source}, evidence, tt.seededFinal)
			} else {
				validateErr = mode.Validate(target, []string{source}, evidence)
			}
			if !errors.Is(validateErr, ownershipevidence.ErrDrift) {
				t.Fatalf("Validate(drift) error = %v, want ErrDrift", validateErr)
			}
		})
	}
}

func TestValidateSeededExplicitEmptyFinal(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	mode := ownershipevidence.For("copy", "seeded")
	evidence, err := mode.Capture("source", source, nil, []byte("baseline"))
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if err := mode.Validate(target, []string{source}, evidence, []byte{}); err != nil {
		t.Fatalf("Validate(explicit empty final) error = %v", err)
	}
	if err := mode.Validate(target, []string{source}, evidence, []byte{}, []byte{}); err == nil || !strings.Contains(err.Error(), "at most one") {
		t.Fatalf("Validate(multiple final content) error = %v", err)
	}
}

func TestValidateSymlinkIdentityAndDestinationDrift(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	other := filepath.Join(dir, "other")
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(other, []byte("other"), 0o600); err != nil {
		t.Fatalf("write other: %v", err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	mode := ownershipevidence.For("symlink", "")
	evidence, err := mode.Capture("source", source, nil, nil)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if err := mode.Validate(target, []string{source}, evidence); err != nil {
		t.Fatalf("Validate(converged symlink) error = %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove symlink: %v", err)
	}
	if err := os.Symlink(other, target); err != nil {
		t.Fatalf("replace symlink: %v", err)
	}
	if err := mode.Validate(target, []string{source}, evidence); !errors.Is(err, ownershipevidence.ErrDrift) || !strings.Contains(err.Error(), "destination changed") {
		t.Fatalf("Validate(changed destination) error = %v, want destination ErrDrift", err)
	}
	if err := mode.Validate(target, []string{source, other}, evidence); !errors.Is(err, ownershipevidence.ErrDrift) {
		t.Fatalf("Validate(multiple sources) error = %v, want ErrDrift", err)
	}
}

func TestUnsupportedModesFailClosed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	for _, mode := range []ownershipevidence.Mode{
		ownershipevidence.For("template", "whole"),
		ownershipevidence.For("symlink", "json-subset"),
		ownershipevidence.For("copy", "future-mode"),
	} {
		if _, err := mode.Capture("source", source, nil, nil); err == nil {
			t.Fatalf("Capture() unsupported mode error = nil")
		}
		if got := mode.Discriminator(state.Contribution{Ownership: mode.Ownership(), EvidenceRecorded: true}); got != ownershipevidence.DiscriminatorMissing {
			t.Fatalf("Discriminator(unsupported) = %q, want missing", got)
		}
	}
}
