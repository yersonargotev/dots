package selectionretirement

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/selectionreconciliation"
	"github.com/yersonargotev/dots/internal/state"
)

func TestBuildSelectsOnlySupportedRetirements(t *testing.T) {
	home := t.TempDir()
	removed := writeTarget(t, home, ".remove", "owned")
	retained := writeTarget(t, home, ".retain", "local")
	meta := state.Metadata{Entries: []state.Record{
		wholeRecord(removed, "owned"),
		{Target: retained, Source: "retain", Strategy: "copy", Ownership: "seeded"},
	}}
	report := explicitReport(
		selectionreconciliation.Action{Scope: selectionreconciliation.ScopeSelection, Outcome: selectionreconciliation.OutcomeRemove},
		retirementAction(removed, selectionreconciliation.OutcomeRemove),
		retirementAction(retained, selectionreconciliation.OutcomeRetain),
		selectionreconciliation.Action{Scope: selectionreconciliation.ScopeManagedEntry, Outcome: selectionreconciliation.OutcomePreserve, ResolvedTarget: filepath.Join(home, ".active"), CurrentSources: []string{"active"}},
	)

	got, err := Build(report, meta, Options{Home: home, SourceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := []Action{
		{Target: removed, Outcome: selectionreconciliation.OutcomeRemove},
		{Target: retained, Outcome: selectionreconciliation.OutcomeRetain},
	}
	if !reflect.DeepEqual(got.Actions, want) {
		t.Fatalf("Build() actions = %#v, want %#v", got.Actions, want)
	}
}

func TestBuildKeepsManifestEvolutionRetirementReportOnly(t *testing.T) {
	home := t.TempDir()
	target := writeTarget(t, home, ".retain", "owned")
	meta := state.Metadata{Entries: []state.Record{wholeRecord(target, "owned")}}
	report := selectionreconciliation.Report{
		RequestedIntent: selectionreconciliation.Intent{Authority: selectionreconciliation.AuthorityManifestEvolution},
		Actions: []selectionreconciliation.Action{
			retirementAction(target, selectionreconciliation.OutcomeRetain),
		},
	}

	got, err := Build(report, meta, Options{Home: home, SourceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(got.Actions) != 0 {
		t.Fatalf("Build() actions = %#v, want manifest evolution to remain report-only", got.Actions)
	}
	if _, ok := meta.FindByTarget(target); !ok {
		t.Fatal("Build mutated Installation Metadata")
	}
}

func TestBuildKeepsSupplementedManifestEvolutionReportOnlyDuringExplicitReduction(t *testing.T) {
	home := t.TempDir()
	target := writeTarget(t, home, ".retain", "owned")
	meta := state.Metadata{Entries: []state.Record{wholeRecord(target, "owned")}}
	action := retirementAction(target, selectionreconciliation.OutcomeRetain)
	action.Reason = selectionreconciliation.ReasonManifestEvolution
	report := explicitReport(action)

	got, err := Build(report, meta, Options{Home: home, SourceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(got.Actions) != 0 {
		t.Fatalf("Build() actions = %#v, want supplemented manifest evolution to remain report-only", got.Actions)
	}
}

func TestBuildLeavesAdditionOnlySourceChangesToForwardInstall(t *testing.T) {
	home := t.TempDir()
	target := writeTarget(t, home, ".config", "old")
	report := explicitReport(
		selectionreconciliation.Action{Scope: selectionreconciliation.ScopeSelection, Outcome: selectionreconciliation.OutcomeCreate, Names: []string{"added"}},
		selectionreconciliation.Action{
			Scope: selectionreconciliation.ScopeManagedEntry, Outcome: selectionreconciliation.OutcomeUpdate,
			ResolvedTarget: target, PreviousSources: []string{"base"}, CurrentSources: []string{"override"},
		},
	)

	forward := plan.Plan{Actions: []plan.Action{{Target: target, Status: plan.StatusUpdate}}}
	got, err := Build(report, state.Metadata{}, Options{Home: home, SourceRoot: t.TempDir(), ForwardPlan: &forward})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(got.Actions) != 0 {
		t.Fatalf("Build() actions = %#v, want addition-only change left to forward install", got.Actions)
	}
}

func writeTarget(t *testing.T, home, name, content string) string {
	t.Helper()
	target := filepath.Join(home, name)
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	return target
}

func wholeRecord(target, content string) state.Record {
	return state.Record{Target: target, Source: filepath.Base(target), Strategy: "copy", Ownership: "whole", Hash: state.HashBytes([]byte(content))}
}

func retirementAction(target string, outcome selectionreconciliation.Outcome) selectionreconciliation.Action {
	return selectionreconciliation.Action{
		Scope:           selectionreconciliation.ScopeManagedEntry,
		Outcome:         outcome,
		ResolvedTarget:  target,
		PreviousSources: []string{filepath.Base(target)},
		CurrentSources:  []string{},
	}
}

func explicitReport(actions ...selectionreconciliation.Action) selectionreconciliation.Report {
	hasSelectionAction := false
	for _, action := range actions {
		if action.Scope == selectionreconciliation.ScopeSelection {
			hasSelectionAction = true
			break
		}
	}
	if !hasSelectionAction {
		actions = append([]selectionreconciliation.Action{{
			Scope: selectionreconciliation.ScopeSelection, Outcome: selectionreconciliation.OutcomeRemove,
		}}, actions...)
	}
	return selectionreconciliation.Report{
		RequestedIntent: selectionreconciliation.Intent{Authority: selectionreconciliation.AuthorityExplicitRequest},
		Actions:         actions,
	}
}

func TestBuildRejectsBlockedAndUnsupportedRetirementsBeforeMutation(t *testing.T) {
	home := t.TempDir()
	target := writeTarget(t, home, ".owned", "owned")
	meta := state.Metadata{Entries: []state.Record{wholeRecord(target, "owned")}}

	tests := []struct {
		name   string
		action selectionreconciliation.Action
	}{
		{name: "blocked", action: retirementAction(target, selectionreconciliation.OutcomeBlocked)},
		{name: "unsupported outcome", action: retirementAction(target, selectionreconciliation.OutcomePreserve)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Build(explicitReport(tt.action), meta, Options{Home: home, SourceRoot: t.TempDir()})
			if err == nil {
				t.Fatal("Build() error = nil, want rejection")
			}
			if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "owned" {
				t.Fatalf("target changed during Build: content=%q err=%v", got, readErr)
			}
		})
	}
}

func TestBuildAllowsEntirePartialOwnershipTargetToRetire(t *testing.T) {
	home := t.TempDir()
	removed := writeTarget(t, home, ".remove.json", `{"owned":true}`)
	retained := writeTarget(t, home, ".retain.json", `{"owned":true,"external":true}`)
	record := func(target string) state.Record {
		return state.Record{
			Target: target, Source: filepath.Base(target), Strategy: "copy", Ownership: "json-subset",
			OwnedContent: []byte(`{"owned":true}`),
			Contributions: []state.Contribution{{
				Source: filepath.Base(target), Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: []byte(`{"owned":true}`),
			}},
		}
	}
	meta := state.Metadata{Entries: []state.Record{record(removed), record(retained)}}

	got, err := Build(explicitReport(
		retirementAction(removed, selectionreconciliation.OutcomeRemove),
		retirementAction(retained, selectionreconciliation.OutcomeRetain),
	), meta, Options{Home: home, SourceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := []Action{
		{Target: removed, Outcome: selectionreconciliation.OutcomeRemove},
		{Target: retained, Outcome: selectionreconciliation.OutcomeRetain},
	}
	if !reflect.DeepEqual(got.Actions, want) {
		t.Fatalf("Build() actions = %#v, want %#v", got.Actions, want)
	}
}

func TestBuildDelegatesSafePartialRetirementToForwardPlan(t *testing.T) {
	home := t.TempDir()
	target := writeTarget(t, home, ".shared.json", `{}`)
	report := explicitReport(selectionreconciliation.Action{
		Scope:           selectionreconciliation.ScopeManagedEntry,
		Outcome:         selectionreconciliation.OutcomeReconcile,
		ResolvedTarget:  target,
		PreviousSources: []string{"removed.json", "kept.json"},
		CurrentSources:  []string{"kept.json"},
	})

	forward := plan.Plan{Actions: []plan.Action{{Target: target, Status: plan.StatusUpdate}}}
	got, err := Build(report, state.Metadata{}, Options{Home: home, SourceRoot: t.TempDir(), ForwardPlan: &forward})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(got.Actions) != 0 {
		t.Fatalf("Build() actions = %#v, want forward plan to own still-selected target", got.Actions)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != `{}` {
		t.Fatalf("target changed during Build: content=%q err=%v", got, readErr)
	}
}

func TestBuildRejectsUnsafePartialRetirementBeforeMutation(t *testing.T) {
	home := t.TempDir()
	target := writeTarget(t, home, ".shared.json", `{}`)
	report := explicitReport(selectionreconciliation.Action{
		Scope:           selectionreconciliation.ScopeManagedEntry,
		Outcome:         selectionreconciliation.OutcomeBlocked,
		Reason:          selectionreconciliation.ReasonAmbiguousPartialOwnership,
		ResolvedTarget:  target,
		PreviousSources: []string{"removed.json", "kept.json"},
		CurrentSources:  []string{"kept.json"},
	})

	_, err := Build(report, state.Metadata{}, Options{Home: home, SourceRoot: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "partial retirement outcome \"blocked\" is unsafe") {
		t.Fatalf("Build() error = %v, want unsafe partial-retirement rejection", err)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != `{}` {
		t.Fatalf("target changed during Build: content=%q err=%v", got, readErr)
	}
}

func TestBuildRejectsTargetOutsideHome(t *testing.T) {
	home := t.TempDir()
	outside := writeTarget(t, t.TempDir(), ".outside", "owned")
	meta := state.Metadata{Entries: []state.Record{wholeRecord(outside, "owned")}}

	_, err := Build(
		explicitReport(retirementAction(outside, selectionreconciliation.OutcomeRemove)),
		meta,
		Options{Home: home, SourceRoot: t.TempDir()},
	)
	if err == nil {
		t.Fatal("Build() error = nil, want home-confinement rejection")
	}
	if got, readErr := os.ReadFile(outside); readErr != nil || string(got) != "owned" {
		t.Fatalf("outside target changed: content=%q err=%v", got, readErr)
	}
}

func TestBuildAllowsRetainWithoutInventory(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".stale-selection")
	action := retirementAction(target, selectionreconciliation.OutcomeRetain)

	got, err := Build(
		explicitReport(action),
		state.Metadata{},
		Options{Home: home, SourceRoot: t.TempDir()},
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !reflect.DeepEqual(got.Actions, []Action{{Target: target, Outcome: selectionreconciliation.OutcomeRetain}}) {
		t.Fatalf("Build() actions = %#v", got.Actions)
	}
}

func TestBuildAllowsRetainForLostLegacyOwnership(t *testing.T) {
	home := t.TempDir()
	target := writeTarget(t, home, ".legacy", "external")
	action := retirementAction(target, selectionreconciliation.OutcomeRetain)
	meta := state.Metadata{Entries: []state.Record{{Target: target, Source: "legacy", Strategy: "copy"}}}

	got, err := Build(explicitReport(action), meta, Options{Home: home, SourceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !reflect.DeepEqual(got.Actions, []Action{{Target: target, Outcome: selectionreconciliation.OutcomeRetain}}) {
		t.Fatalf("Build() actions = %#v", got.Actions)
	}
}

func TestApplyRemovesOrRetainsAndPreservesIndependentMetadata(t *testing.T) {
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".state")
	removed := writeTarget(t, home, ".remove", "owned")
	retained := writeTarget(t, home, ".retain", "local")
	selection := &state.InstalledSelection{Profiles: []string{"previous"}, ResolvedTags: []string{"previous"}}
	provisioners := []state.ProvisionerRecord{{Tool: "tool", Executable: "tool", Status: "ok"}}
	meta := state.Metadata{
		Version:            state.CurrentVersion,
		Provenance:         state.Provenance{SourceRoot: "/source"},
		Entries:            []state.Record{wholeRecord(removed, "owned"), {Target: retained, Source: "retain", Strategy: "copy", Ownership: "seeded"}},
		Provisioners:       provisioners,
		InstalledSelection: selection,
	}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	report := explicitReport(
		retirementAction(removed, selectionreconciliation.OutcomeRemove),
		retirementAction(retained, selectionreconciliation.OutcomeRetain),
	)
	opts := Options{Home: home, SourceRoot: t.TempDir(), StateRoot: stateRoot}
	p, err := Build(report, meta, opts)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	result, err := Apply(p, opts)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !reflect.DeepEqual(result, Result{Removed: []string{removed}, Retained: []string{retained}}) {
		t.Fatalf("Apply() result = %#v", result)
	}
	if _, err := os.Lstat(removed); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed target still exists: %v", err)
	}
	if got, err := os.ReadFile(retained); err != nil || string(got) != "local" {
		t.Fatalf("retained target = %q, %v", got, err)
	}
	got, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load pruned metadata: %v", err)
	}
	if len(got.Entries) != 0 || !reflect.DeepEqual(got.Provisioners, provisioners) || !reflect.DeepEqual(got.InstalledSelection, selection) || got.Provenance != meta.Provenance {
		t.Fatalf("pruned metadata = %#v, want independent fields preserved", got)
	}
	if _, err := os.Stat(state.Path(stateRoot)); err != nil {
		t.Fatalf("empty metadata was not persisted: %v", err)
	}
}

func TestApplyPreservesDriftedPlannedRemovalAndReleasesMetadata(t *testing.T) {
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".state")
	target := writeTarget(t, home, ".owned", "owned")
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{wholeRecord(target, "owned")}}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatal(err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Home: home, SourceRoot: t.TempDir(), StateRoot: stateRoot}
	p, err := Build(explicitReport(retirementAction(target, selectionreconciliation.OutcomeRemove)), meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("drift"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Apply(p, opts)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !reflect.DeepEqual(result, Result{Removed: []string{}, Retained: []string{target}}) {
		t.Fatalf("Apply() result = %#v", result)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "drift" {
		t.Fatalf("drifted target = %q, %v", got, err)
	}
	got, err := state.Load(state.Path(stateRoot))
	if err != nil || len(got.Entries) != 0 {
		t.Fatalf("metadata after release = %#v, %v", got, err)
	}
}

func TestApplyPreservesSubsetTargetExtendedAfterBuild(t *testing.T) {
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".state")
	target := writeTarget(t, home, ".owned.json", `{"owned":true}`)
	rec := state.Record{
		Target: target, Source: "owned.json", Strategy: "copy", Ownership: "json-subset",
		OwnedContent: []byte(`{"owned":true}`),
		Contributions: []state.Contribution{{
			Source: "owned.json", Ownership: "json-subset", EvidenceRecorded: true, OwnedContent: []byte(`{"owned":true}`),
		}},
	}
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{rec}}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatal(err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Home: home, SourceRoot: t.TempDir(), StateRoot: stateRoot}
	p, err := Build(explicitReport(retirementAction(target, selectionreconciliation.OutcomeRemove)), meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"owned":true,"external":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Apply(p, opts)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !reflect.DeepEqual(result, Result{Removed: []string{}, Retained: []string{target}}) {
		t.Fatalf("Apply() result = %#v", result)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != `{"owned":true,"external":true}` {
		t.Fatalf("extended subset target = %q, %v", got, err)
	}
	got, err := state.Load(state.Path(stateRoot))
	if err != nil || len(got.Entries) != 0 {
		t.Fatalf("metadata after retained release = %#v, %v", got, err)
	}
}

func TestApplyRejectsChangedMetadataBeforeFilesystemMutation(t *testing.T) {
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".state")
	target := writeTarget(t, home, ".owned", "owned")
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{wholeRecord(target, "owned")}}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatal(err)
	}
	opts := Options{Home: home, SourceRoot: t.TempDir(), StateRoot: stateRoot}
	p, err := Build(explicitReport(retirementAction(target, selectionreconciliation.OutcomeRemove)), meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	changed := meta
	changed.Entries = []state.Record{wholeRecord(target, "replacement")}
	if err := state.Save(state.Path(stateRoot), changed); err != nil {
		t.Fatal(err)
	}

	result, err := Apply(p, opts)
	if err == nil {
		t.Fatalf("Apply() = (%#v, nil), want stale metadata error", result)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "owned" {
		t.Fatalf("target after metadata change = %q, %v", got, err)
	}
	got, err := state.Load(state.Path(stateRoot))
	if err != nil || !reflect.DeepEqual(got.Entries, changed.Entries) {
		t.Fatalf("metadata after rejection = %#v, %v", got, err)
	}
}

func TestApplyPrevalidatesAllMetadataBeforeFirstRemoval(t *testing.T) {
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".state")
	first := writeTarget(t, home, ".first", "first")
	second := writeTarget(t, home, ".second", "second")
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{wholeRecord(first, "first"), wholeRecord(second, "second")}}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatal(err)
	}
	opts := Options{Home: home, SourceRoot: t.TempDir(), StateRoot: stateRoot}
	report := explicitReport(
		retirementAction(first, selectionreconciliation.OutcomeRemove),
		retirementAction(second, selectionreconciliation.OutcomeRemove),
	)
	p, err := Build(report, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	changed := meta
	changed.Entries = append([]state.Record(nil), meta.Entries...)
	changed.Entries[1].Hash = state.HashBytes([]byte("replacement"))
	if err := state.Save(state.Path(stateRoot), changed); err != nil {
		t.Fatal(err)
	}

	if _, err := Apply(p, opts); err == nil {
		t.Fatal("Apply() error = nil, want stale metadata rejection")
	}
	for _, target := range []string{first, second} {
		if _, err := os.Stat(target); err != nil {
			t.Fatalf("target %s mutated before metadata prevalidation: %v", target, err)
		}
	}
}

func TestApplyMergesConcurrentIndependentMetadataBeforePruning(t *testing.T) {
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".state")
	target := writeTarget(t, home, ".owned", "owned")
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{wholeRecord(target, "owned")}}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatal(err)
	}
	opts := Options{Home: home, SourceRoot: t.TempDir(), StateRoot: stateRoot}
	p, err := Build(explicitReport(retirementAction(target, selectionreconciliation.OutcomeRemove)), meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	receipt := state.ProvisionerRecord{Tool: "concurrent", Executable: "tool", Status: "provisioned"}
	updateStarted := make(chan struct{})
	updateDone := make(chan error, 1)

	result, err := applyWithRemove(p, opts, func(target string) error {
		if err := os.Remove(target); err != nil {
			return err
		}
		go func() {
			close(updateStarted)
			updateDone <- state.Update(state.Path(stateRoot), func(latest *state.Metadata) error {
				latest.Provisioners = append(latest.Provisioners, receipt)
				return nil
			})
		}()
		<-updateStarted
		return nil
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := <-updateDone; err != nil {
		t.Fatalf("concurrent metadata update: %v", err)
	}
	if !reflect.DeepEqual(result.Removed, []string{target}) {
		t.Fatalf("Apply() result = %#v", result)
	}
	got, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 0 || !reflect.DeepEqual(got.Provisioners, []state.ProvisionerRecord{receipt}) {
		t.Fatalf("metadata after prune = %#v, want concurrent receipt preserved", got)
	}
}

func TestApplyRemovalFailureLeavesMetadataRepairableAndRerunConverges(t *testing.T) {
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".state")
	first := writeTarget(t, home, ".first", "first")
	second := writeTarget(t, home, ".second", "second")
	selection := &state.InstalledSelection{Profiles: []string{"previous"}, ResolvedTags: []string{"previous"}}
	meta := state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{wholeRecord(first, "first"), wholeRecord(second, "second")}, InstalledSelection: selection}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatal(err)
	}
	opts := Options{Home: home, SourceRoot: t.TempDir(), StateRoot: stateRoot}
	report := explicitReport(
		retirementAction(first, selectionreconciliation.OutcomeRemove),
		retirementAction(second, selectionreconciliation.OutcomeRemove),
	)
	p, err := Build(report, meta, opts)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	result, err := applyWithRemove(p, opts, func(target string) error {
		calls++
		if calls == 2 {
			return errors.New("injected removal failure")
		}
		return os.Remove(target)
	})
	if err == nil || !reflect.DeepEqual(result.Removed, []string{first}) {
		t.Fatalf("first Apply() = (%#v, %v)", result, err)
	}
	got, loadErr := state.Load(state.Path(stateRoot))
	if loadErr != nil || len(got.Entries) != 2 || !reflect.DeepEqual(got.InstalledSelection, selection) {
		t.Fatalf("metadata after failure = %#v, %v", got, loadErr)
	}

	result, err = Apply(p, opts)
	if err != nil {
		t.Fatalf("rerun Apply() error = %v", err)
	}
	if !reflect.DeepEqual(result, Result{Removed: []string{second}, Retained: []string{first}}) {
		t.Fatalf("rerun Apply() = %#v", result)
	}
	got, loadErr = state.Load(state.Path(stateRoot))
	if loadErr != nil || len(got.Entries) != 0 || !reflect.DeepEqual(got.InstalledSelection, selection) {
		t.Fatalf("metadata after rerun = %#v, %v", got, loadErr)
	}
}
