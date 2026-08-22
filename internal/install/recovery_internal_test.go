package install

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
	p := plan.Plan{Profiles: []string{"next"}, Tags: []string{"base", "later"}, Actions: []plan.Action{
		{
			Source: "base.json", Target: target, Strategy: "copy", Ownership: "json-subset", Status: plan.StatusUpdate,
			PreviousContent: previous, Contributions: []plan.Contribution{{Source: "base.json", SelectorTags: []string{"base"}}},
		},
		{Source: "later", Target: filepath.Join(home, ".later"), Strategy: "copy", Status: plan.StatusCreate, Contributions: []plan.Contribution{{Source: "later", SelectorTags: []string{"later"}}}},
	}}
	calls := 0
	_, err = applyManagedEntriesWithApply(p, Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}, func(action plan.Action, source string, opts Options) error {
		calls++
		if calls == 2 {
			return errors.New("injected later Managed Entry failure")
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
