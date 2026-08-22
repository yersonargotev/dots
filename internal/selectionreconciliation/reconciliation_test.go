package selectionreconciliation

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
	"github.com/yersonargotev/dots/internal/state"
)

func TestBuildReusesForwardClassification(t *testing.T) {
	tests := []struct {
		name       string
		ownership  string
		status     ForwardStatus
		want       Outcome
		wantReason string
	}{
		{name: "create", status: ForwardCreate, want: OutcomeCreate},
		{name: "whole update", status: ForwardUpdate, want: OutcomeUpdate},
		{name: "partial reconcile", ownership: "json-subset", status: ForwardUpdate, want: OutcomeReconcile},
		{name: "preserve", status: ForwardUnchanged, want: OutcomePreserve},
		{name: "conflict", status: ForwardConflict, want: OutcomeBlocked, wantReason: ReasonLostOwnership},
		{name: "missing source", status: ForwardMissingSource, want: OutcomeBlocked, wantReason: ReasonMissingSource},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry := selectedEntry("source", "~/.target", "copy", test.ownership)
			report, err := Build(baseInput(nil, []selectedsurface.SelectedEntry{entry}, TargetEvidence{
				DeclarativeTarget: "~/.target",
				ResolvedTarget:    "/home/test/.target",
				ForwardStatus:     test.status,
			}))
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if len(report.Actions) != 1 {
				t.Fatalf("Actions = %#v, want one", report.Actions)
			}
			if got := report.Actions[0]; got.Outcome != test.want || got.Reason != test.wantReason {
				t.Fatalf("Action = %#v, want outcome %q reason %q", got, test.want, test.wantReason)
			}
			if got, want := report.HasFindings(), test.want == OutcomeBlocked; got != want {
				t.Fatalf("HasFindings() = %v, want %v", got, want)
			}
		})
	}
}

func TestBuildClassifiesWholeTargetReplacementFromRecordedEvidence(t *testing.T) {
	tests := []struct {
		name       string
		live       []byte
		want       Outcome
		wantReason string
	}{
		{name: "owned previous bytes update", live: []byte("old\n"), want: OutcomeUpdate},
		{name: "drift blocks", live: []byte("external\n"), want: OutcomeBlocked, wantReason: ReasonWholeTargetDrift},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previous := selectedEntry("old", "~/.target", "copy", "whole")
			current := selectedEntry("new", "~/.target", "copy", "whole")
			input := baseInput([]selectedsurface.SelectedEntry{previous}, []selectedsurface.SelectedEntry{current}, TargetEvidence{
				DeclarativeTarget: "~/.target",
				ResolvedTarget:    "/home/test/.target",
				Exists:            true,
				Kind:              TargetKindRegular,
				Content:           test.live,
			})
			input.Metadata.Entries = []state.Record{{
				Target: "/home/test/.target",
				Source: "old",
				Hash:   state.HashBytes([]byte("old\n")),
			}}
			input.Evidence.Sources = []SourceEvidence{{
				DeclarativeTarget: "~/.target", Source: "new", Exists: true, Content: []byte("new\n"),
			}}
			report, err := Build(input)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			got := report.Actions[0]
			if got.Outcome != test.want || got.Reason != test.wantReason {
				t.Fatalf("Action = %#v, want outcome %q reason %q", got, test.want, test.wantReason)
			}
		})
	}
}

func TestBuildReconcilesSharedJSONTargetInSelectedSurfaceOrder(t *testing.T) {
	previous := []selectedsurface.SelectedEntry{
		selectedEntry("a.json", "~/.shared.json", "copy", "json-subset"),
		selectedEntry("b.json", "~/.shared.json", "copy", "json-subset"),
		selectedEntry("old", "~/.old", "copy", "whole"),
	}
	current := []selectedsurface.SelectedEntry{
		selectedEntry("second", "~/.second", "copy", "whole"),
		selectedEntry("b.json", "~/.shared.json", "copy", "json-subset"),
	}
	input := baseInput(previous, current,
		TargetEvidence{DeclarativeTarget: "~/.second", ResolvedTarget: "/home/test/.second", ForwardStatus: ForwardCreate},
		TargetEvidence{DeclarativeTarget: "~/.shared.json", ResolvedTarget: "/home/test/.shared.json", Exists: true, Kind: TargetKindRegular, Content: []byte(`{"a":1,"b":2,"external":true}`)},
		TargetEvidence{DeclarativeTarget: "~/.old", ResolvedTarget: "/home/test/.old", Exists: true, Kind: TargetKindRegular, Content: []byte("old")},
	)
	input.Evidence.Sources = []SourceEvidence{{
		DeclarativeTarget: "~/.shared.json", Source: "b.json", Exists: true, Content: []byte(`{"b":2}`),
	}}
	input.Metadata.Entries = []state.Record{
		{
			Target: "/home/test/.shared.json",
			Contributions: []state.Contribution{
				{Source: "a.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"a":1}`)},
				{Source: "b.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"b":2}`)},
			},
		},
		{Target: "/home/test/.old", Source: "old", Hash: state.HashBytes([]byte("old"))},
	}

	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got, want := managedTargets(report.Actions), []string{"~/.second", "~/.shared.json", "~/.old"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Managed Entry order = %#v, want %#v", got, want)
	}
	if got := report.Actions[1]; got.Outcome != OutcomeReconcile || !reflect.DeepEqual(got.PreviousSources, []string{"a.json", "b.json"}) || !reflect.DeepEqual(got.CurrentSources, []string{"b.json"}) {
		t.Fatalf("shared target action = %#v", got)
	}
	if got := report.Actions[2].Outcome; got != OutcomeRemove {
		t.Fatalf("retired whole target outcome = %q, want remove", got)
	}
}

func TestBuildNeverUsesLegacyTargetWideProjectionForPartialRetirement(t *testing.T) {
	previous := selectedEntry("a.json", "~/.shared.json", "copy", "json-subset")
	input := baseInput([]selectedsurface.SelectedEntry{previous}, nil, TargetEvidence{
		DeclarativeTarget: "~/.shared.json",
		ResolvedTarget:    "/home/test/.shared.json",
		Exists:            true,
		Kind:              TargetKindRegular,
		Content:           []byte(`{"a":1,"external":true}`),
	})
	input.Metadata.Entries = []state.Record{{
		Target:       "/home/test/.shared.json",
		Source:       "a.json",
		Ownership:    "json-subset",
		OwnedContent: json.RawMessage(`{"a":1}`),
	}}

	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := report.Actions[0]; got.Outcome != OutcomeBlocked || got.Reason != ReasonAmbiguousPartialOwnership {
		t.Fatalf("Action = %#v, want ambiguous partial ownership block", got)
	}
}

func TestBuildClassifiesExactPartialRetirement(t *testing.T) {
	tests := []struct {
		name string
		live []byte
		want Outcome
	}{
		{name: "remove empty target", live: []byte(`{"a":1}`), want: OutcomeRemove},
		{name: "retain external target bytes", live: []byte(`{"a":1,"external":true}`), want: OutcomeRetain},
		{name: "changed owned value blocks", live: []byte(`{"a":2}`), want: OutcomeBlocked},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previous := selectedEntry("a.json", "~/.shared.json", "copy", "json-subset")
			input := baseInput([]selectedsurface.SelectedEntry{previous}, nil, TargetEvidence{
				DeclarativeTarget: "~/.shared.json", ResolvedTarget: "/home/test/.shared.json",
				Exists: true, Kind: TargetKindRegular, Content: test.live,
			})
			input.Metadata.Entries = []state.Record{{
				Target: "/home/test/.shared.json",
				Contributions: []state.Contribution{{
					Source: "a.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"a":1}`),
				}},
			}}
			report, err := Build(input)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got := report.Actions[0].Outcome; got != test.want {
				t.Fatalf("Outcome = %q, want %q (action %#v)", got, test.want, report.Actions[0])
			}
			if test.want == OutcomeBlocked && report.Actions[0].Reason != ReasonAmbiguousPartialOwnership {
				t.Fatalf("Reason = %q, want ambiguous partial ownership", report.Actions[0].Reason)
			}
		})
	}
}

func TestBuildManifestEvolutionNeverAuthorizesRetirement(t *testing.T) {
	previous := []selectedsurface.SelectedEntry{
		selectedEntry("a.json", "~/.shared.json", "copy", "json-subset"),
		selectedEntry("b.json", "~/.shared.json", "copy", "json-subset"),
		selectedEntry("old", "~/.old", "copy", "whole"),
	}
	current := []selectedsurface.SelectedEntry{selectedEntry("b.json", "~/.shared.json", "copy", "json-subset")}
	input := baseInput(previous, current,
		TargetEvidence{DeclarativeTarget: "~/.shared.json", ResolvedTarget: "/home/test/.shared.json"},
		TargetEvidence{DeclarativeTarget: "~/.old", ResolvedTarget: "/home/test/.old"},
	)
	input.RequestedIntent.Authority = AuthorityManifestEvolution

	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, action := range report.Actions {
		if action.Scope != ScopeManagedEntry {
			continue
		}
		if action.Outcome != OutcomeRetain || action.Reason != ReasonManifestEvolution {
			t.Fatalf("evolution action = %#v, want report-only retain", action)
		}
	}
}

func TestBuildDependencyAndProvisionerOnlyReductionRetainsExternalState(t *testing.T) {
	input := baseInput(nil, nil)
	input.PreviousIntent = Intent{Authority: AuthorityRecorded, ExtraTags: []string{"tools"}, ResolvedTags: []string{"tools"}}
	input.RequestedIntent = Intent{Authority: AuthorityExplicitRequest, ExtraTags: []string{"core"}, ResolvedTags: []string{"core"}}
	input.PreviousSurface.Dependencies = []manifest.Dependency{{Name: "ripgrep"}}
	input.PreviousSurface.Provisioners = []manifest.Provisioner{{Tool: "codex"}}

	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got, want := actionOutcomes(report.Actions), []Outcome{OutcomeRemove, OutcomeCreate, OutcomeRetainedExternalState, OutcomeRetainedExternalState}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outcomes = %#v, want %#v", got, want)
	}
	for _, action := range report.Actions {
		if action.Scope == ScopeManagedEntry {
			t.Fatalf("unexpected Managed Entry action: %#v", action)
		}
	}
	if report.HasFindings() {
		t.Fatal("Retained External State must remain informational")
	}
}

func TestBuildReturnsDetachedStableArrays(t *testing.T) {
	input := baseInput(nil, nil)
	input.PreviousIntent.Profiles = []string{"old"}
	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.PreviousIntent.Profiles[0] = "mutated"
	if report.PreviousIntent.Profiles[0] != "old" {
		t.Fatalf("report aliases input: %#v", report.PreviousIntent.Profiles)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(data) == "" || report.Actions == nil || report.Actions[0].PreviousSources == nil || report.Actions[0].CurrentSources == nil || report.Actions[0].Names == nil {
		t.Fatalf("report arrays are not stable: %s", data)
	}
}

func baseInput(previous, current []selectedsurface.SelectedEntry, targets ...TargetEvidence) Input {
	return Input{
		PreviousIntent:  Intent{Authority: AuthorityRecorded},
		RequestedIntent: Intent{Authority: AuthorityExplicitRequest},
		PreviousSurface: selectedsurface.Surface{Entries: previous},
		CurrentSurface:  selectedsurface.Surface{Entries: current},
		Metadata:        state.Metadata{Version: state.CurrentVersion},
		Evidence:        Evidence{Targets: targets, Sources: make([]SourceEvidence, 0)},
	}
}

func selectedEntry(source, target, strategy, ownership string) selectedsurface.SelectedEntry {
	return selectedsurface.SelectedEntry{
		Entry:  manifest.Entry{Source: source, Target: target, Strategy: strategy, Ownership: ownership},
		Source: source,
	}
}

func managedTargets(actions []Action) []string {
	result := make([]string, 0)
	for _, action := range actions {
		if action.Scope == ScopeManagedEntry {
			result = append(result, action.DeclarativeTarget)
		}
	}
	return result
}

func actionOutcomes(actions []Action) []Outcome {
	result := make([]Outcome, 0, len(actions))
	for _, action := range actions {
		result = append(result, action.Outcome)
	}
	return result
}
