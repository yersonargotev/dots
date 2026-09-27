package selectionreconciliation

import (
	"testing"

	"github.com/yersonargotev/dots/internal/selectedsurface"
	"github.com/yersonargotev/dots/internal/state"
)

func TestBuildClassifiesExactOwnedSymlinkSourceSwitch(t *testing.T) {
	const (
		target   = "~/.config/app"
		resolved = "/home/test/.config/app"
		previous = "configs/app/base"
		current  = "configs/app/carbonfox"
		oldPath  = "/repo/configs/app/base"
		newPath  = "/repo/configs/app/carbonfox"
	)
	previousEntry := selectedEntry(previous, target, "symlink", "whole")
	currentEntry := selectedEntry(current, target, "symlink", "whole")
	base := func() Input {
		input := baseInput([]selectedsurface.SelectedEntry{previousEntry}, []selectedsurface.SelectedEntry{currentEntry}, TargetEvidence{
			DeclarativeTarget: target,
			ResolvedTarget:    resolved,
			Exists:            true,
			Kind:              TargetKindSymlink,
			LinkDestination:   oldPath,
			ForwardStatus:     ForwardConflict,
		})
		input.Evidence.Sources = []SourceEvidence{
			{DeclarativeTarget: target, Source: previous, ResolvedSource: oldPath, Exists: true},
			{DeclarativeTarget: target, Source: current, ResolvedSource: newPath, Exists: true},
		}
		input.Metadata.Entries = []state.Record{{
			Target: resolved, Source: previous, Strategy: "symlink", Ownership: "whole",
			Contributions: []state.Contribution{{Source: previous, Ownership: "whole", EvidenceRecorded: true}},
		}}
		return input
	}

	t.Run("exact previous link is an explicit update", func(t *testing.T) {
		report, err := Build(base())
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		if got := report.Actions[0]; got.Outcome != OutcomeUpdate || got.Reason != "" {
			t.Fatalf("Action = %#v, want exact symlink update", got)
		}
	})

	for _, test := range []struct {
		name string
		edit func(*Input)
	}{
		{name: "missing target", edit: func(input *Input) {
			input.Evidence.Targets[0].Exists = false
			input.Evidence.Targets[0].Kind = ""
			input.Evidence.Targets[0].LinkDestination = ""
			input.Evidence.Targets[0].ForwardStatus = ForwardCreate
		}},
		{name: "retargeted link", edit: func(input *Input) { input.Evidence.Targets[0].LinkDestination = "/external" }},
		{name: "regular target", edit: func(input *Input) { input.Evidence.Targets[0].Kind = TargetKindRegular }},
		{name: "legacy record", edit: func(input *Input) { input.Metadata.Entries[0].Contributions = nil }},
		{name: "contradictory record", edit: func(input *Input) { input.Metadata.Entries[0].Contributions[0].Source = "other" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := base()
			test.edit(&input)
			report, err := Build(input)
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got := report.Actions[0]; got.Outcome != OutcomeBlocked || got.Reason != ReasonLostOwnership {
				t.Fatalf("Action = %#v, want lost-ownership block", got)
			}
		})
	}
}
