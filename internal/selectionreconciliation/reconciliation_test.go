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

func TestBuildBlocksPartialRetirementWhenForwardPlanConflicts(t *testing.T) {
	previous := []selectedsurface.SelectedEntry{
		selectedEntry("a.json", "~/.shared.json", "copy", "json-subset"),
		selectedEntry("b.json", "~/.shared.json", "copy", "json-subset"),
	}
	current := []selectedsurface.SelectedEntry{
		selectedEntry("b.json", "~/.shared.json", "copy", "json-subset"),
	}
	input := baseInput(previous, current, TargetEvidence{
		DeclarativeTarget: "~/.shared.json",
		ResolvedTarget:    "/home/test/.shared.json",
		Exists:            true,
		Kind:              TargetKindRegular,
		Content:           []byte(`{"a":1,"b":2}`),
		ForwardStatus:     ForwardConflict,
	})
	input.Metadata.Entries = []state.Record{{
		Target:    "/home/test/.shared.json",
		Strategy:  "copy",
		Ownership: "json-subset",
		Contributions: []state.Contribution{
			{Source: "a.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"a":1}`)},
			{Source: "b.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"b":2}`)},
		},
		OwnedContent: json.RawMessage(`{"contradictory":true}`),
	}}

	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := report.Actions[0]; got.Outcome != OutcomeBlocked || got.Reason != ReasonAmbiguousPartialOwnership {
		t.Fatalf("Action = %#v, want ambiguous partial ownership block", got)
	}
	if !report.HasFindings() {
		t.Fatal("forward conflict must remain a read-only finding")
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
				Target:    "/home/test/.target",
				Source:    "old",
				Strategy:  "copy",
				Ownership: "whole",
				Hash:      state.HashBytes([]byte("old\n")),
				Contributions: []state.Contribution{{
					Source: "old", Ownership: "whole", EvidenceRecorded: true, Hash: state.HashBytes([]byte("old\n")),
				}},
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

func TestBuildRequiresExactWholeTargetContributionEvidence(t *testing.T) {
	previous := selectedEntry("old", "~/.target", "copy", "whole")
	tests := []struct {
		name   string
		record state.Record
	}{
		{
			name: "legacy target-wide hash",
			record: state.Record{
				Target: "/home/test/.target", Source: "old", Strategy: "copy", Ownership: "whole", Hash: state.HashBytes([]byte("old\n")),
			},
		},
		{
			name: "different attributed source",
			record: state.Record{
				Target: "/home/test/.target", Source: "other", Strategy: "copy", Ownership: "whole", Hash: state.HashBytes([]byte("old\n")),
				Contributions: []state.Contribution{{
					Source: "other", Ownership: "whole", EvidenceRecorded: true, Hash: state.HashBytes([]byte("old\n")),
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := baseInput([]selectedsurface.SelectedEntry{previous}, nil, TargetEvidence{
				DeclarativeTarget: "~/.target", ResolvedTarget: "/home/test/.target",
				Exists: true, Kind: TargetKindRegular, Content: []byte("old\n"),
			})
			input.Metadata.Entries = []state.Record{test.record}
			report, err := Build(input)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got := report.Actions[0]; got.Outcome != OutcomeRetain || got.Reason != ReasonLostOwnership {
				t.Fatalf("Action = %#v, want lost ownership retention", got)
			}
		})
	}
}

func TestBuildRetainsRetiredWholeTargetWhenLiveContentDrifted(t *testing.T) {
	previous := selectedEntry("old", "~/.target", "copy", "whole")
	input := baseInput([]selectedsurface.SelectedEntry{previous}, nil, TargetEvidence{
		DeclarativeTarget: "~/.target",
		ResolvedTarget:    "/home/test/.target",
		Exists:            true,
		Kind:              TargetKindRegular,
		Content:           []byte("external\n"),
	})
	input.Metadata.Entries = []state.Record{{
		Target:    "/home/test/.target",
		Source:    "old",
		Strategy:  "copy",
		Ownership: "whole",
		Contributions: []state.Contribution{{
			Source: "old", Ownership: "whole", EvidenceRecorded: true, Hash: state.HashBytes([]byte("old\n")),
		}},
	}}

	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := report.Actions[0]; got.Outcome != OutcomeRetain || got.Reason != ReasonWholeTargetDrift {
		t.Fatalf("Action = %#v, want whole-target Drift retention", got)
	}
	if !report.HasFindings() {
		t.Fatal("retired whole-target Drift must remain a read-only finding")
	}
}

func TestReportTreatsLostWholeTargetOwnershipAsFinding(t *testing.T) {
	report := Report{Actions: []Action{{
		Scope: ScopeManagedEntry, Outcome: OutcomeRetain, Reason: ReasonLostOwnership,
	}}}
	if !report.HasFindings() {
		t.Fatal("lost whole-target ownership must remain a read-only finding")
	}
}

func TestBuildBlocksMultipleWholeTargetContributions(t *testing.T) {
	previous := []selectedsurface.SelectedEntry{
		selectedEntry("first", "~/.target", "copy", "whole"),
		selectedEntry("second", "~/.target", "copy", "whole"),
	}
	input := baseInput(previous, nil, TargetEvidence{
		DeclarativeTarget: "~/.target", ResolvedTarget: "/home/test/.target",
		Exists: true, Kind: TargetKindRegular, Content: []byte("first\n"),
	})
	input.Metadata.Entries = []state.Record{{
		Target: "/home/test/.target", Strategy: "copy", Ownership: "whole",
		Contributions: []state.Contribution{
			{Source: "first", Ownership: "whole", EvidenceRecorded: true, Hash: state.HashBytes([]byte("first\n"))},
			{Source: "second", Ownership: "whole", EvidenceRecorded: true, Hash: state.HashBytes([]byte("second\n"))},
		},
	}}
	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := report.Actions[0]; got.Outcome != OutcomeRetain || got.Reason != ReasonLostOwnership {
		t.Fatalf("Action = %#v, want lost ownership retention", got)
	}
}

func TestBuildRequiresExactSymlinkContributionSet(t *testing.T) {
	previous := selectedEntry("source", "~/.target", "symlink", "whole")
	input := baseInput([]selectedsurface.SelectedEntry{previous}, nil, TargetEvidence{
		DeclarativeTarget: "~/.target", ResolvedTarget: "/home/test/.target",
		Exists: true, Kind: TargetKindSymlink, LinkDestination: "/repo/source",
	})
	input.Evidence.Sources = []SourceEvidence{{
		DeclarativeTarget: "~/.target", Source: "source", ResolvedSource: "/repo/source", Exists: true,
	}}
	input.Metadata.Entries = []state.Record{{
		Target: "/home/test/.target", Strategy: "symlink", Ownership: "whole",
		Contributions: []state.Contribution{
			{Source: "source", Ownership: "whole", EvidenceRecorded: true},
			{Source: "other", Ownership: "whole", EvidenceRecorded: true},
		},
	}}
	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := report.Actions[0]; got.Outcome != OutcomeRetain || got.Reason != ReasonLostOwnership {
		t.Fatalf("Action = %#v, want lost ownership retention", got)
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
			Target: "/home/test/.shared.json", Strategy: "copy", Ownership: "json-subset",
			Contributions: []state.Contribution{
				{Source: "a.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"a":1}`)},
				{Source: "b.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"b":2}`)},
			},
		},
		{
			Target: "/home/test/.old", Source: "old", Strategy: "copy", Ownership: "whole", Hash: state.HashBytes([]byte("old")),
			Contributions: []state.Contribution{{
				Source: "old", Ownership: "whole", EvidenceRecorded: true, Hash: state.HashBytes([]byte("old")),
			}},
		},
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

func TestBuildAcceptsAlreadyAppliedSharedReconciliationOnlyWithExactReceipt(t *testing.T) {
	previous := []selectedsurface.SelectedEntry{
		selectedEntry("a.json", "~/.shared.json", "copy", "json-subset"),
		selectedEntry("b.json", "~/.shared.json", "copy", "json-subset"),
	}
	current := []selectedsurface.SelectedEntry{selectedEntry("b.json", "~/.shared.json", "copy", "json-subset")}
	live := []byte(`{"b":2,"external":true}`)
	currentSource := []byte(`{"b":2}`)
	input := baseInput(previous, current, TargetEvidence{
		DeclarativeTarget: "~/.shared.json", ResolvedTarget: "/home/test/.shared.json", Exists: true, Kind: TargetKindRegular, Content: live,
	})
	input.Evidence.Sources = []SourceEvidence{{
		DeclarativeTarget: "~/.shared.json", Source: "b.json", Exists: true, Content: currentSource,
	}}
	input.Metadata.Entries = []state.Record{{
		Target: "/home/test/.shared.json", Strategy: "copy", Ownership: "json-subset",
		Contributions: []state.Contribution{
			{Source: "a.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"a":1}`)},
			{Source: "b.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(currentSource)},
		},
		PendingReconciliation: &state.ReconciliationReceipt{
			TargetHash: state.HashBytes(live), Sources: []string{"b.json"}, SourceHashes: []string{state.HashBytes(currentSource)},
			Strategy: "copy", Ownership: "json-subset",
		},
	}}

	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if got := report.Actions[0]; got.Outcome != OutcomePreserve || got.Reason != "" {
		t.Fatalf("receipt-backed Action = %#v, want preserve", got)
	}

	input.Evidence.Targets[0].Content = []byte(`{"b":2,"external":"changed"}`)
	report, err = Build(input)
	if err != nil {
		t.Fatalf("Build(drifted) error = %v", err)
	}
	if got := report.Actions[0]; got.Outcome != OutcomeBlocked || got.Reason != ReasonAmbiguousPartialOwnership {
		t.Fatalf("drifted receipt Action = %#v, want ambiguous block", got)
	}
}

func TestBuildRetainsPartialRetirementWithoutExactContributionEvidence(t *testing.T) {
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
	if got := report.Actions[0]; got.Outcome != OutcomeRetain || got.Reason != ReasonAmbiguousPartialOwnership {
		t.Fatalf("Action = %#v, want ambiguous partial ownership retention", got)
	}
	if !report.HasFindings() {
		t.Fatal("ambiguous partial ownership must remain a read-only finding")
	}
}

func TestBuildClassifiesExactPartialRetirement(t *testing.T) {
	tests := []struct {
		name       string
		live       []byte
		want       Outcome
		wantReason string
	}{
		{name: "remove empty target", live: []byte(`{"a":1}`), want: OutcomeRemove},
		{name: "retain external target bytes", live: []byte(`{"a":1,"external":true}`), want: OutcomeRetain},
		{name: "retain changed owned value", live: []byte(`{"a":2}`), want: OutcomeRetain, wantReason: ReasonAmbiguousPartialOwnership},
		{name: "retain missing owned value", live: []byte(`{"external":true}`), want: OutcomeRetain, wantReason: ReasonAmbiguousPartialOwnership},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			previous := selectedEntry("a.json", "~/.shared.json", "copy", "json-subset")
			input := baseInput([]selectedsurface.SelectedEntry{previous}, nil, TargetEvidence{
				DeclarativeTarget: "~/.shared.json", ResolvedTarget: "/home/test/.shared.json",
				Exists: true, Kind: TargetKindRegular, Content: test.live,
			})
			input.Metadata.Entries = []state.Record{{
				Target: "/home/test/.shared.json", Strategy: "copy", Ownership: "json-subset",
				Contributions: []state.Contribution{{
					Source: "a.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: json.RawMessage(`{"a":1}`),
				}},
			}}
			report, err := Build(input)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got := report.Actions[0]; got.Outcome != test.want || got.Reason != test.wantReason {
				t.Fatalf("Action = %#v, want outcome %q reason %q", got, test.want, test.wantReason)
			}
			if test.wantReason != "" && !report.HasFindings() {
				t.Fatal("unsafe retained partial target must remain a read-only finding")
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

func TestBuildDistinguishesProvisionerEffectsWithTheSameTool(t *testing.T) {
	removed := NewProvisionerEvidence("claude", "claude", []string{"plugin", "install", "removed"})
	retained := NewProvisionerEvidence("claude", "claude", []string{"mcp", "add", "retained"})
	input := baseInput(nil, nil)
	input.Evidence.PreviousProvisioners = []ProvisionerEvidence{removed, retained}
	input.Evidence.CurrentProvisioners = []ProvisionerEvidence{retained}

	report, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions = %#v, want one removed Provisioner effect", report.Actions)
	}
	got := report.Actions[0]
	if got.Scope != ScopeProvisioner || got.Outcome != OutcomeRetainedExternalState || !reflect.DeepEqual(got.Names, []string{"claude"}) || got.Identity == "" {
		t.Fatalf("Action = %#v, want one identified retained external Provisioner effect", got)
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
