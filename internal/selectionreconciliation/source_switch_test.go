package selectionreconciliation

import (
	"github.com/yersonargotev/dots/internal/selectedsurface"
	"github.com/yersonargotev/dots/internal/state"
	"testing"
)

func TestBuildTOMLSourceSwitchRequiresExactAttribution(t *testing.T) {
	for _, scenario := range []string{"old", "edited", "desired", "receipt", "legacy", "contradictory", "hash", "missing-record", "missing-bytes", "empty", "directory", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			old := []byte("width=38\n")
			next := []byte("width=26\n")
			live := old
			status := ForwardConflict
			if scenario == "edited" {
				live = []byte("width=99\n")
			}
			if scenario == "desired" || scenario == "receipt" {
				live = next
				status = ForwardUnchanged
			}
			if scenario == "empty" {
				old = []byte{}
			}
			input := baseInput([]selectedsurface.SelectedEntry{selectedEntry("old.toml", "~/.target", "copy", "toml-subset")}, []selectedsurface.SelectedEntry{selectedEntry("new.toml", "~/.target", "copy", "toml-subset")}, TargetEvidence{DeclarativeTarget: "~/.target", ResolvedTarget: "/home/test/.target", Exists: true, Kind: TargetKindRegular, Content: live, ForwardStatus: status})
			input.PreviousIntent.ExtraTags = []string{"old"}
			input.RequestedIntent.ExtraTags = []string{"new"}
			rec := state.Record{Target: "/home/test/.target", Source: "old.toml", Strategy: "copy", Ownership: "toml-subset", Hash: state.HashBytes(old), OwnedBytes: old, Contributions: []state.Contribution{{Source: "old.toml", Ownership: "toml-subset", EvidenceRecorded: true, Hash: state.HashBytes(old), OwnedBytes: old}}}
			if scenario == "receipt" {
				rec.PendingReconciliation = &state.ReconciliationReceipt{TargetHash: state.HashBytes(live), Sources: []string{"new.toml"}, SourceHashes: []string{state.HashBytes(next)}, Strategy: "copy", Ownership: "toml-subset"}
			}
			if scenario == "legacy" {
				rec.Contributions = nil
			}
			if scenario == "contradictory" {
				rec.OwnedBytes = []byte("width=44\n")
			}
			if scenario == "hash" {
				rec.Hash = "wrong"
				rec.Contributions[0].Hash = "wrong"
			}
			if scenario == "missing-bytes" {
				rec.Contributions[0].OwnedBytes = nil
				rec.OwnedBytes = nil
			}
			if scenario == "directory" {
				input.Evidence.Targets[0].Kind = TargetKindOther
			}
			if scenario == "symlink" {
				input.Evidence.Targets[0].Kind = TargetKindSymlink
			}
			input.Metadata.Entries = []state.Record{rec}
			if scenario == "missing-record" {
				input.Metadata.Entries = nil
			}
			input.Evidence.Sources = []SourceEvidence{{DeclarativeTarget: "~/.target", Source: "new.toml", Exists: true, Content: next}}
			report, err := Build(input)
			if err != nil {
				t.Fatal(err)
			}
			got := report.Actions[len(report.Actions)-1]
			want := OutcomeBlocked
			reason := ReasonAmbiguousPartialOwnership
			switch scenario {
			case "old", "edited", "desired":
				want = OutcomeReconcile
				reason = ReasonOwnedSourceConflict
			case "receipt":
				want = OutcomePreserve
				reason = ""
			case "directory", "symlink":
				reason = ReasonLostOwnership
			}
			if got.Outcome != want || got.Reason != reason {
				t.Fatalf("action=%+v want %s/%s", got, want, reason)
			}
			if report.HasFindings() != (want != OutcomePreserve) {
				t.Fatal("incorrect findings")
			}
		})
	}
}
