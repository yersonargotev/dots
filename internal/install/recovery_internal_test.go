package install

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestApplyManagedEntriesPersistsReconciliationReceiptBeforeLaterActionFails(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	baseSource := filepath.Join(sourceRoot, "base.json")
	retiredSource := filepath.Join(sourceRoot, "retired.json")
	laterSource := filepath.Join(sourceRoot, "later")
	base := []byte("{\"base\":true}\n")
	retired := []byte("{\"retired\":true}\n")
	for path, content := range map[string][]byte{baseSource: base, retiredSource: retired, laterSource: []byte("later\n")} {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	previous, err := configsubset.MergeJSON(base, retired)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, ".shared.json")
	live := []byte("{\"base\":true,\"retired\":true,\"external\":true}\n")
	if err := os.WriteFile(target, live, 0o600); err != nil {
		t.Fatal(err)
	}
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}, ResolvedTags: []string{"base", "retired"}}
	record := state.Record{
		Target: target, Source: "base.json", Sources: []string{"base.json", "retired.json"}, Strategy: "copy", Ownership: "json-subset",
		OwnedContent: previous, Hash: state.HashBytes(previous),
		Contributions: []state.Contribution{
			{Source: "base.json", Ownership: "json-subset", EvidenceRecorded: true, Hash: state.HashBytes(base), OwnedContent: base},
			{Source: "retired.json", Ownership: "json-subset", EvidenceRecorded: true, Hash: state.HashBytes(retired), OwnedContent: retired},
		},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{record}, InstalledSelection: oldSelection}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Profiles: []string{"next"}, Tags: []string{"base", "later"}, Actions: []plan.Action{
		{
			Source: "base.json", Target: target, Strategy: "copy", Ownership: "json-subset", Status: plan.StatusUpdate,
			PreviousContent: previous, PreviousRecordFingerprint: fingerprint,
			Contributions: []plan.Contribution{{Source: "base.json", SelectorTags: []string{"base"}}},
		},
		{Source: "later", Target: filepath.Join(home, ".later"), Strategy: "copy", Status: plan.StatusCreate, Contributions: []plan.Contribution{{Source: "later", SelectorTags: []string{"later"}}}},
	}}
	calls := 0
	_, err = applyManagedEntriesWithApply(p, Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}, func(action plan.Action, source string, opts Options) (managedActionResult, error) {
		calls++
		if calls == 2 {
			return managedActionResult{}, errors.New("injected later Managed Entry failure")
		}
		return applyManagedAction(action, source, opts)
	})
	if err == nil || err.Error() != "injected later Managed Entry failure" {
		t.Fatalf("applyManagedEntriesWithApply() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	failed, ok := meta.FindByTarget(target)
	if !ok || len(failed.Contributions) != 2 {
		t.Fatalf("failed metadata replaced committed contributions: %#v", failed)
	}
	if failed.PendingReconciliation == nil || failed.PendingReconciliation.TargetHash != state.HashBytes(got) ||
		!reflect.DeepEqual(failed.PendingReconciliation.Sources, []string{"base.json"}) {
		t.Fatalf("recovery receipt = %#v, want exact first-action result", failed.PendingReconciliation)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"old"}) {
		t.Fatalf("failed apply Installed Selection = %#v, want old", meta.InstalledSelection)
	}
}

func TestReconciliationRejectsConcurrentAuthorityChangeBeforeMutation(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	target := filepath.Join(home, ".shared.json")
	source := filepath.Join(sourceRoot, "shared.json")
	previous := []byte(`{"old":true}`)
	current := []byte(`{"new":true}`)
	if err := os.WriteFile(target, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, current, 0o600); err != nil {
		t.Fatal(err)
	}
	record := recoveryAuthorityRecord(target, "shared.json", previous)
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{record}}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	action := recoveryAuthorityAction(target, "shared.json", previous, fingerprint)
	concurrentReceipt := &state.ReconciliationReceipt{
		TargetHash: "concurrent", Sources: []string{"shared.json"}, SourceHashes: []string{"concurrent"},
		Strategy: "copy", Ownership: "json-subset",
	}
	if err := state.Update(state.Path(stateRoot), func(meta *state.Metadata) error {
		meta.Entries[0].PendingReconciliation = concurrentReceipt.Clone()
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	mutated := false
	_, err = applyManagedEntriesWithApply(plan.Plan{Actions: []plan.Action{action}}, Options{
		SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot,
	}, func(action plan.Action, source string, opts Options) (managedActionResult, error) {
		mutated = true
		return applyManagedAction(action, source, opts)
	})
	if err == nil || !strings.Contains(err.Error(), "receipt changed concurrently") {
		t.Fatalf("applyManagedEntriesWithApply() error = %v, want authority CAS rejection", err)
	}
	if mutated {
		t.Fatal("reconciliation mutated the target before validating concurrent authority")
	}
	if got, readErr := os.ReadFile(target); readErr != nil || !bytes.Equal(got, previous) {
		t.Fatalf("target = %q, %v; want unchanged previous bytes", got, readErr)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := meta.FindByTarget(target)
	if !ok || !reflect.DeepEqual(got.PendingReconciliation, concurrentReceipt) {
		t.Fatalf("concurrent record = %#v, want concurrent receipt preserved", got)
	}
}

func TestReconciliationReceiptUsesExactAppliedSourceSnapshot(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	target := filepath.Join(home, ".shared.json")
	sourceName := "shared.json"
	source := filepath.Join(sourceRoot, sourceName)
	previous := []byte(`{"old":true}`)
	applied := []byte(`{"kept":true,"removed":true}`)
	concurrent := []byte(`{"kept":true}`)
	if err := os.WriteFile(target, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, applied, 0o600); err != nil {
		t.Fatal(err)
	}
	record := recoveryAuthorityRecord(target, sourceName, previous)
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}, ResolvedTags: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: state.CurrentVersion, Entries: []state.Record{record}, InstalledSelection: oldSelection,
	}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	action := recoveryAuthorityAction(target, sourceName, previous, fingerprint)

	_, err = applyManagedEntriesWithApply(plan.Plan{Actions: []plan.Action{action}}, Options{
		SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot,
	}, func(action plan.Action, source string, opts Options) (managedActionResult, error) {
		if !bytes.Equal(action.Content, applied) {
			t.Fatalf("prepared source snapshot = %q, want %q", action.Content, applied)
		}
		if err := os.WriteFile(source, concurrent, 0o600); err != nil {
			t.Fatal(err)
		}
		return applyManagedAction(action, source, opts)
	})
	if err == nil || !strings.Contains(err.Error(), "changed after reconciliation") {
		t.Fatalf("applyManagedEntriesWithApply() error = %v, want changed source rejection", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte(`"removed":true`)) {
		t.Fatalf("target = %q, want exact applied source snapshot", got)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	failed, ok := meta.FindByTarget(target)
	if !ok || failed.PendingReconciliation == nil || failed.PendingReconciliation.SourceHashes[0] != state.HashBytes(applied) {
		t.Fatalf("receipt = %#v, want exact applied source hash", failed.PendingReconciliation)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"old"}) {
		t.Fatalf("Installed Selection = %#v, want old", meta.InstalledSelection)
	}
}

func TestMetadataCommitRejectsConcurrentAuthorityChange(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	target := filepath.Join(home, ".shared.json")
	sourceName := "shared.json"
	source := filepath.Join(sourceRoot, sourceName)
	previous := []byte(`{"old":true}`)
	current := []byte(`{"new":true}`)
	if err := os.WriteFile(target, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, current, 0o600); err != nil {
		t.Fatal(err)
	}
	record := recoveryAuthorityRecord(target, sourceName, previous)
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}, ResolvedTags: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: state.CurrentVersion, Entries: []state.Record{record}, InstalledSelection: oldSelection,
	}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	action := recoveryAuthorityAction(target, sourceName, previous, fingerprint)
	commit, err := ApplyManagedEntries(plan.Plan{Actions: []plan.Action{action}}, Options{
		SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot,
	})
	if err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}
	if err := state.Update(state.Path(stateRoot), func(meta *state.Metadata) error {
		meta.Entries[0].Tags = []string{"concurrent"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	err = commit.Commit(&state.InstalledSelection{Profiles: []string{"new"}, ResolvedTags: []string{"new"}})
	if err == nil || !strings.Contains(err.Error(), "changed concurrently") {
		t.Fatalf("Commit() error = %v, want authority CAS rejection", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := meta.FindByTarget(target)
	if !ok || !reflect.DeepEqual(got.Tags, []string{"concurrent"}) || got.PendingReconciliation == nil {
		t.Fatalf("concurrent record = %#v, want preserved with original receipt", got)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"old"}) {
		t.Fatalf("Installed Selection = %#v, want old", meta.InstalledSelection)
	}
}

func TestReceiptBackedUnchangedCommitRejectsConcurrentReceiptChange(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	target := filepath.Join(home, ".shared.json")
	sourceName := "shared.json"
	source := filepath.Join(sourceRoot, sourceName)
	content := []byte(`{"current":true}`)
	if err := os.WriteFile(target, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, content, 0o600); err != nil {
		t.Fatal(err)
	}
	record := recoveryAuthorityRecord(target, sourceName, []byte(`{"retired":true}`))
	receipt := &state.ReconciliationReceipt{
		TargetHash: state.HashBytes(content), Sources: []string{sourceName},
		SourceHashes: []string{state.HashBytes(content)}, Strategy: "copy", Ownership: "json-subset",
	}
	record.PendingReconciliation = receipt.Clone()
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}, ResolvedTags: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: state.CurrentVersion, Entries: []state.Record{record}, InstalledSelection: oldSelection,
	}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	action := plan.Action{
		Source: sourceName, Target: target, Strategy: "copy", Ownership: "json-subset", Status: plan.StatusUnchanged,
		PreviousContent: record.OwnedContent, PreviousRecordFingerprint: fingerprint,
		PreviousReconciliationReceipt: receipt.Clone(), Contributions: []plan.Contribution{{Source: sourceName}},
	}
	commit, err := ApplyManagedEntries(plan.Plan{Actions: []plan.Action{action}}, Options{
		SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot,
	})
	if err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}
	if err := state.Update(state.Path(stateRoot), func(meta *state.Metadata) error {
		meta.Entries[0].PendingReconciliation.TargetHash = state.HashBytes([]byte("concurrent"))
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	err = commit.Commit(&state.InstalledSelection{Profiles: []string{"new"}, ResolvedTags: []string{"new"}})
	if err == nil || !strings.Contains(err.Error(), "receipt changed concurrently") {
		t.Fatalf("Commit() error = %v, want receipt CAS rejection", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"old"}) {
		t.Fatalf("Installed Selection = %#v, want old", meta.InstalledSelection)
	}
}

func recoveryAuthorityRecord(target, source string, previous []byte) state.Record {
	return state.Record{
		Target: target, Source: source, Strategy: "copy", Ownership: "json-subset",
		OwnedContent: previous, Hash: state.HashBytes(previous),
		Contributions: []state.Contribution{{
			Source: source, Ownership: "json-subset", EvidenceRecorded: true,
			Hash: state.HashBytes(previous), OwnedContent: previous,
		}},
	}
}

func recoveryAuthorityAction(target, source string, previous []byte, fingerprint string) plan.Action {
	return plan.Action{
		Source: source, Target: target, Strategy: "copy", Ownership: "json-subset", Status: plan.StatusUpdate,
		PreviousContent: previous, PreviousRecordFingerprint: fingerprint,
		Contributions: []plan.Contribution{{Source: source}},
	}
}
