package selectionretirement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/selectionreconciliation"
	"github.com/yersonargotev/dots/internal/state"
)

func TestBuildRequiresExplicitReplaceForExactSymlinkSourceSwitchAddition(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	previousSource := "configs/app/base"
	currentSource := "configs/app/carbonfox"
	previousPath := writeSymlinkTransitionSource(t, sourceRoot, previousSource)
	writeSymlinkTransitionSource(t, sourceRoot, currentSource)
	target := filepath.Join(home, ".config", "app")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(previousPath, target); err != nil {
		t.Fatal(err)
	}

	record := exactSymlinkTransitionRecord(target, previousSource)
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	forward := plan.Plan{Actions: []plan.Action{{
		Source: currentSource, Target: target, Strategy: "symlink", Ownership: "whole", Status: plan.StatusConflict,
		PreviousRecordFingerprint: fingerprint, Contributions: []plan.Contribution{{Source: currentSource}},
	}}}
	report := symlinkTransitionReport(selectionreconciliation.OutcomeUpdate, target, previousSource, currentSource, false)

	meta := state.Metadata{Entries: []state.Record{record}}
	retirement, err := Build(report, meta, Options{Home: home, SourceRoot: sourceRoot, ForwardPlan: &forward})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(retirement.Actions) != 0 {
		t.Fatalf("Build() actions = %#v, want forward-only source switch", retirement.Actions)
	}
	if err := retirement.AuthorizeSymlinkTransitions(meta, &forward, map[string]install.ConflictDecision{target: install.DecisionReplace}); err != nil {
		t.Fatalf("AuthorizeSymlinkTransitions(Replace) error = %v", err)
	}
	for _, decisions := range []map[string]install.ConflictDecision{nil, {}, {target: install.DecisionSkip}, {target: install.DecisionAdopt}} {
		if err := retirement.AuthorizeSymlinkTransitions(meta, &forward, decisions); err == nil || !strings.Contains(err.Error(), "requires Conflict Resolution Replace") {
			t.Fatalf("AuthorizeSymlinkTransitions(%v) error = %v, want actionable rejection", decisions, err)
		}
	}
}

func TestBuildRejectsSymlinkSourceSwitchWithoutExactPriorAuthority(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	previousSource := "configs/app/base"
	currentSource := "configs/app/carbonfox"
	previousPath := writeSymlinkTransitionSource(t, sourceRoot, previousSource)
	writeSymlinkTransitionSource(t, sourceRoot, currentSource)
	target := filepath.Join(home, ".app")
	if err := os.Symlink(previousPath, target); err != nil {
		t.Fatal(err)
	}
	record := exactSymlinkTransitionRecord(target, previousSource)
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	baseForward := plan.Action{
		Source: currentSource, Target: target, Strategy: "symlink", Ownership: "whole", Status: plan.StatusConflict,
		PreviousRecordFingerprint: fingerprint, Contributions: []plan.Contribution{{Source: currentSource}},
	}
	report := symlinkTransitionReport(selectionreconciliation.OutcomeUpdate, target, previousSource, currentSource, true)

	for _, test := range []struct {
		name string
		edit func(*state.Record, *plan.Action)
	}{
		{name: "legacy record", edit: func(record *state.Record, _ *plan.Action) { record.Contributions = nil }},
		{name: "contradictory record", edit: func(record *state.Record, _ *plan.Action) { record.Contributions[0].Source = currentSource }},
		{name: "stale fingerprint", edit: func(_ *state.Record, forward *plan.Action) { forward.PreviousRecordFingerprint = "stale" }},
		{name: "copy strategy", edit: func(_ *state.Record, forward *plan.Action) { forward.Strategy = "copy" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidateRecord := record.Clone()
			candidateForward := baseForward
			test.edit(&candidateRecord, &candidateForward)
			_, err := Build(report, state.Metadata{Entries: []state.Record{candidateRecord}}, Options{
				Home: home, SourceRoot: sourceRoot, ForwardPlan: &plan.Plan{Actions: []plan.Action{candidateForward}},
			})
			if err == nil {
				t.Fatal("Build() error = nil, want exact-authority rejection")
			}
		})
	}

	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(sourceRoot, "external"), target); err != nil {
		t.Fatal(err)
	}
	_, err = Build(report, state.Metadata{Entries: []state.Record{record}}, Options{
		Home: home, SourceRoot: sourceRoot, ForwardPlan: &plan.Plan{Actions: []plan.Action{baseForward}},
	})
	if err == nil || !strings.Contains(err.Error(), "no longer the exact previous link") {
		t.Fatalf("Build(retargeted) error = %v, want exact-link rejection", err)
	}
}

func TestBuildAllowsExactPostReplacePrecommitRerunOnlyWithPreviousRecord(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	previousSource := "configs/app/base"
	currentSource := "configs/app/carbonfox"
	writeSymlinkTransitionSource(t, sourceRoot, previousSource)
	currentPath := writeSymlinkTransitionSource(t, sourceRoot, currentSource)
	target := filepath.Join(home, ".app")
	if err := os.Symlink(currentPath, target); err != nil {
		t.Fatal(err)
	}
	record := exactSymlinkTransitionRecord(target, previousSource)
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	forward := plan.Plan{Actions: []plan.Action{{
		Source: currentSource, Target: target, Strategy: "symlink", Ownership: "whole", Status: plan.StatusUnchanged,
		PreviousRecordFingerprint: fingerprint, Contributions: []plan.Contribution{{Source: currentSource}},
	}}}
	report := symlinkTransitionReport(selectionreconciliation.OutcomePreserve, target, previousSource, currentSource, false)

	meta := state.Metadata{Entries: []state.Record{record}}
	retirement, err := Build(report, meta, Options{Home: home, SourceRoot: sourceRoot, ForwardPlan: &forward})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if err := retirement.AuthorizeSymlinkTransitions(meta, &forward, nil); err != nil {
		t.Fatalf("post-replace rerun unexpectedly requires Replace: %v", err)
	}

	contradictory := exactSymlinkTransitionRecord(target, currentSource)
	contradictoryFingerprint, fingerprintErr := state.RecordEvidenceFingerprint(contradictory)
	if fingerprintErr != nil {
		t.Fatal(fingerprintErr)
	}
	forward.Actions[0].PreviousRecordFingerprint = contradictoryFingerprint
	_, err = Build(report, state.Metadata{Entries: []state.Record{contradictory}}, Options{Home: home, SourceRoot: sourceRoot, ForwardPlan: &forward})
	if err == nil || !strings.Contains(err.Error(), "recorded sources") {
		t.Fatalf("Build(contradictory current record) error = %v, want fail-closed rejection", err)
	}
}

func TestAuthorizeSymlinkTransitionsRejectsStaleAuthority(t *testing.T) {
	type fixture struct {
		retirement Plan
		metadata   state.Metadata
		forward    plan.Plan
		target     string
	}
	setup := func(t *testing.T) fixture {
		t.Helper()
		home := t.TempDir()
		sourceRoot := t.TempDir()
		previousSource := "configs/app/base"
		currentSource := "configs/app/carbonfox"
		previousPath := writeSymlinkTransitionSource(t, sourceRoot, previousSource)
		writeSymlinkTransitionSource(t, sourceRoot, currentSource)
		target := filepath.Join(home, ".app")
		if err := os.Symlink(previousPath, target); err != nil {
			t.Fatal(err)
		}
		record := exactSymlinkTransitionRecord(target, previousSource)
		metadata := state.Metadata{
			Entries: []state.Record{record},
			InstalledSelection: &state.InstalledSelection{
				ExtraTags: []string{"app"}, ResolvedTags: []string{"app"},
			},
		}
		fingerprint, err := state.RecordEvidenceFingerprint(record)
		if err != nil {
			t.Fatal(err)
		}
		forward := plan.Plan{Actions: []plan.Action{{
			Source: currentSource, ResolvedSource: filepath.Join(sourceRoot, filepath.FromSlash(currentSource)),
			Target: target, Strategy: "symlink", Ownership: "whole", Status: plan.StatusConflict,
			PreviousRecordFingerprint: fingerprint,
			Contributions:             []plan.Contribution{{Source: currentSource, SelectorTags: []string{"theme-carbonfox"}}},
		}}}
		retirement, err := Build(
			symlinkTransitionReport(selectionreconciliation.OutcomeUpdate, target, previousSource, currentSource, false),
			metadata,
			Options{Home: home, SourceRoot: sourceRoot, ForwardPlan: &forward},
		)
		if err != nil {
			t.Fatal(err)
		}
		return fixture{retirement: retirement, metadata: metadata, forward: forward, target: target}
	}
	decisions := func(target string) map[string]install.ConflictDecision {
		return map[string]install.ConflictDecision{target: install.DecisionReplace}
	}

	tests := []struct {
		name string
		edit func(*testing.T, *fixture)
		want string
	}{
		{
			name: "record fingerprint",
			edit: func(_ *testing.T, fixture *fixture) {
				fixture.metadata.Entries[0].Contributions[0].SelectorTags = []string{"changed"}
			},
			want: "recorded contribution authority changed",
		},
		{
			name: "Installed Selection",
			edit: func(_ *testing.T, fixture *fixture) {
				fixture.metadata.InstalledSelection.ExtraTags = append(fixture.metadata.InstalledSelection.ExtraTags, "changed")
			},
			want: "Installed Selection changed",
		},
		{
			name: "forward action",
			edit: func(_ *testing.T, fixture *fixture) {
				fixture.forward.Actions[0].ResolvedSource += "-changed"
			},
			want: "forward action changed",
		},
		{
			name: "duplicate forward target",
			edit: func(_ *testing.T, fixture *fixture) {
				fixture.forward.Actions = append(fixture.forward.Actions, clonePlanAction(fixture.forward.Actions[0]))
			},
			want: "duplicate target",
		},
		{
			name: "live link retargeted",
			edit: func(t *testing.T, fixture *fixture) {
				if err := os.Remove(fixture.target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(filepath.Dir(fixture.target), "external"), fixture.target); err != nil {
					t.Fatal(err)
				}
			},
			want: "no longer the exact previous link",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := setup(t)
			test.edit(t, &fixture)
			err := fixture.retirement.AuthorizeSymlinkTransitions(fixture.metadata, &fixture.forward, decisions(fixture.target))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("AuthorizeSymlinkTransitions() error = %v, want %q", err, test.want)
			}
		})
	}
}

func symlinkTransitionReport(outcome selectionreconciliation.Outcome, target, previous, current string, reduction bool) selectionreconciliation.Report {
	actions := make([]selectionreconciliation.Action, 0, 2)
	if reduction {
		actions = append(actions, selectionreconciliation.Action{Scope: selectionreconciliation.ScopeSelection, Outcome: selectionreconciliation.OutcomeRemove})
	} else {
		actions = append(actions, selectionreconciliation.Action{Scope: selectionreconciliation.ScopeSelection, Outcome: selectionreconciliation.OutcomeCreate})
	}
	actions = append(actions, selectionreconciliation.Action{
		Scope: selectionreconciliation.ScopeManagedEntry, Outcome: outcome, ResolvedTarget: target,
		PreviousSources: []string{previous}, CurrentSources: []string{current},
	})
	return selectionreconciliation.Report{
		RequestedIntent: selectionreconciliation.Intent{Authority: selectionreconciliation.AuthorityExplicitRequest},
		Actions:         actions,
	}
}

func exactSymlinkTransitionRecord(target, source string) state.Record {
	return state.Record{
		Target: target, Source: source, Strategy: "symlink", Ownership: "whole",
		Contributions: []state.Contribution{{Source: source, Ownership: "whole", EvidenceRecorded: true}},
	}
}

func writeSymlinkTransitionSource(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(name+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
