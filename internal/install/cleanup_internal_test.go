package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

type typedInjectedCleanupError struct {
	lifecycle string
}

func (e *typedInjectedCleanupError) Error() string {
	return "typed injected cleanup for " + e.lifecycle
}

type typedInjectedPrimaryError struct {
	lifecycle string
}

func (e *typedInjectedPrimaryError) Error() string {
	return "typed injected primary failure for " + e.lifecycle
}

func TestWithoutCompletedActionMarkerPreservesEveryJoinedCleanup(t *testing.T) {
	first := errors.New("first cleanup")
	second := errors.New("second cleanup")
	joined := errors.Join(
		completedActionCleanup(first),
		fmt.Errorf("wrapped cleanup: %w", completedActionCleanup(second)),
	)

	stripped := withoutCompletedActionMarker(joined)
	if !errors.Is(stripped, first) || !errors.Is(stripped, second) {
		t.Fatalf("stripped cleanup = %v, want both cleanup errors", stripped)
	}
	if !strings.Contains(stripped.Error(), "wrapped cleanup: second cleanup") {
		t.Fatalf("stripped cleanup = %q, want outer operation context preserved", stripped)
	}
	if isCompletedActionCleanup(stripped) {
		t.Fatalf("stripped cleanup = %v, completed-action marker remains", stripped)
	}
	if !isCleanupFailure(stripped) {
		t.Fatalf("stripped cleanup = %v, cleanup classification was lost", stripped)
	}
}

func TestDecisionReplaceCompletedCreateCleanupRetainsBackupAndPrefix(t *testing.T) {
	home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
	source, laterSource := filepath.Join(sourceRoot, "replacement"), filepath.Join(sourceRoot, "later")
	target, laterTarget := filepath.Join(home, "target"), filepath.Join(home, "later")
	if err := os.WriteFile(source, []byte("replacement\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(laterSource, []byte("later\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: oldSelection}); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Profiles: []string{"new"}, Actions: []plan.Action{
		{Source: "replacement", Target: target, Strategy: "copy", Status: plan.StatusConflict},
		{Source: "later", Target: laterTarget, Strategy: "copy", Status: plan.StatusCreate},
	}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot, ConflictDecisions: map[string]ConflictDecision{target: DecisionReplace}}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures

	originalRoot, originalFile := closeRootOperation, closeFileOperation
	defer func() { closeRootOperation, closeFileOperation = originalRoot, originalFile }()
	fault := errors.New("injected replace create root close fault")
	targetClosed, calls := false, 0
	closeFileOperation = func(file *os.File) error {
		err := originalFile(file)
		if file.Name() == target {
			targetClosed = true
		}
		return err
	}
	closeRootOperation = func(root *os.Root) error {
		err := originalRoot(root)
		if root.Name() == home && targetClosed && calls == 0 {
			calls++
			return errors.Join(err, fault)
		}
		return err
	}
	_, err = ApplyManagedEntries(p, opts)
	if !errors.Is(err, fault) || calls != 1 || !strings.Contains(err.Error(), "close filesystem root "+home) {
		t.Fatalf("replace cleanup = %v, calls %d", err, calls)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "replacement\n" {
		t.Fatalf("replacement target = %q, %v", got, readErr)
	}
	if _, statErr := os.Lstat(laterTarget); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("later action ran: %v", statErr)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 1 {
		t.Fatalf("Backup Sets = %#v, %v", backupMeta.Sets, err)
	}
	backup, err := os.ReadFile(backups.FilePath(stateRoot, backupMeta.Sets[0].ID, 1, target))
	if err != nil || string(backup) != "original\n" {
		t.Fatalf("replacement backup = %q, %v", backup, err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if record, ok := meta.FindByTarget(target); !ok || record.Hash != state.HashBytes([]byte("replacement\n")) || len(record.Contributions) != 0 {
		t.Fatalf("replacement partial inventory = %#v", record)
	}
	if meta.InstalledSelection == nil || meta.InstalledSelection.Profiles[0] != "old" {
		t.Fatalf("InstalledSelection = %#v", meta.InstalledSelection)
	}

	closeRootOperation, closeFileOperation = originalRoot, originalFile
	commit, err := ApplyManagedEntries(p, opts)
	if err != nil {
		t.Fatalf("safe replace rerun = %v", err)
	}
	if err := commit.Commit(nil); err != nil {
		t.Fatal(err)
	}
	if got, readErr := os.ReadFile(laterTarget); readErr != nil || string(got) != "later\n" {
		t.Fatalf("safe replace rerun later target = %q, %v", got, readErr)
	}
}

func TestExternalEditDuringCompletedCreateCleanupIsPreservedAndUnclaimed(t *testing.T) {
	home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
	source, laterSource := filepath.Join(sourceRoot, "source"), filepath.Join(sourceRoot, "later")
	target, laterTarget := filepath.Join(home, "target"), filepath.Join(home, "later")
	if err := os.WriteFile(source, []byte("managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(laterSource, []byte("later\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: oldSelection}); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Profiles: []string{"new"}, Actions: []plan.Action{
		{Source: "source", Target: target, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "later", Target: laterTarget, Strategy: "copy", Status: plan.StatusCreate},
	}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures
	originalRoot := closeRootOperation
	defer func() { closeRootOperation = originalRoot }()
	fault := errors.New("injected create root close fault after external edit")
	calls := 0
	closeRootOperation = func(root *os.Root) error {
		err := originalRoot(root)
		if root.Name() == home && calls == 0 {
			if _, statErr := os.Lstat(target); statErr == nil {
				if editErr := os.WriteFile(target, []byte("external\n"), 0o600); editErr != nil {
					t.Fatalf("inject external edit: %v", editErr)
				}
				calls++
				return errors.Join(err, fault)
			}
		}
		return err
	}
	_, err = ApplyManagedEntries(p, opts)
	if !errors.Is(err, fault) || calls != 1 || !strings.Contains(err.Error(), "capture completed install prefix evidence") {
		t.Fatalf("ApplyManagedEntries() = %v, calls %d; want cleanup plus fail-closed evidence", err, calls)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "external\n" {
		t.Fatalf("external target = %q, %v; want preserved", got, readErr)
	}
	if _, statErr := os.Lstat(laterTarget); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("later action ran: %v", statErr)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.FindByTarget(target); ok {
		t.Fatal("cleanup recovery claimed externally edited bytes")
	}
	if meta.InstalledSelection == nil || meta.InstalledSelection.Profiles[0] != "old" {
		t.Fatalf("InstalledSelection = %#v", meta.InstalledSelection)
	}

	closeRootOperation = originalRoot
	_, rerunErr := ApplyManagedEntries(p, opts)
	if rerunErr == nil {
		t.Fatal("rerun accepted stale create plan over external target")
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "external\n" {
		t.Fatalf("rerun changed external target = %q, %v", got, readErr)
	}
	if _, statErr := os.Lstat(laterTarget); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rerun reached later action: %v", statErr)
	}
}

func TestLaterCreateParentRootCleanupPersistsEarlierPrefixAndReruns(t *testing.T) {
	home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		if err := os.WriteFile(filepath.Join(sourceRoot, name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	targetOne := filepath.Join(home, "one")
	targetTwo := filepath.Join(home, "nested", "two")
	targetThree := filepath.Join(home, "three")
	p := plan.Plan{Actions: []plan.Action{
		{Source: "one", Target: targetOne, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "two", Target: targetTwo, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "three", Target: targetThree, Strategy: "copy", Status: plan.StatusCreate},
	}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures
	originalRoot := closeRootOperation
	defer func() { closeRootOperation = originalRoot }()
	fault := errors.New("injected later create parent root close fault")
	homeCloses, faults := 0, 0
	closeRootOperation = func(root *os.Root) error {
		err := originalRoot(root)
		if root.Name() == home {
			homeCloses++
			if homeCloses == 3 {
				faults++
				return errors.Join(err, fault)
			}
		}
		return err
	}
	_, err = ApplyManagedEntries(p, opts)
	if !errors.Is(err, fault) || faults != 1 || !strings.Contains(err.Error(), "create parent directory for "+targetTwo) {
		t.Fatalf("ApplyManagedEntries() = %v, faults %d; want action-two parent cleanup", err, faults)
	}
	if got, readErr := os.ReadFile(targetOne); readErr != nil || string(got) != "one\n" {
		t.Fatalf("first target = %q, %v", got, readErr)
	}
	for _, target := range []string{targetTwo, targetThree} {
		if _, statErr := os.Lstat(target); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("uncompleted target %s exists: %v", target, statErr)
		}
	}
	if info, statErr := os.Stat(filepath.Dir(targetTwo)); statErr != nil || !info.IsDir() {
		t.Fatalf("non-owned empty parent may remain, got %v, %v", info, statErr)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.FindByTarget(targetOne); !ok {
		t.Fatal("partial inventory missing earlier completed action")
	}
	if _, ok := meta.FindByTarget(targetTwo); ok {
		t.Fatal("partial inventory includes current uncompleted action")
	}

	closeRootOperation = originalRoot
	rerun := p
	rerun.Actions = append([]plan.Action(nil), p.Actions...)
	rerun.Actions[0].Status = plan.StatusUnchanged
	commit, err := ApplyManagedEntries(rerun, opts)
	if err != nil {
		t.Fatalf("safe rerun = %v", err)
	}
	if err := commit.Commit(nil); err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]string{targetTwo: "two\n", targetThree: "three\n"} {
		if got, readErr := os.ReadFile(target); readErr != nil || string(got) != want {
			t.Fatalf("rerun target %s = %q, %v", target, got, readErr)
		}
	}
}

func TestCompletedCreateRootCleanupPersistsExactPrefix(t *testing.T) {
	home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
	sourceOne, sourceTwo := filepath.Join(sourceRoot, "one"), filepath.Join(sourceRoot, "two")
	targetOne, targetTwo := filepath.Join(home, "one"), filepath.Join(home, "two")
	if err := os.WriteFile(sourceOne, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceTwo, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}, ResolvedTags: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: oldSelection}); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Profiles: []string{"next"}, Actions: []plan.Action{
		{Source: "one", Target: targetOne, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "two", Target: targetTwo, Strategy: "copy", Status: plan.StatusCreate},
	}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures

	originalRoot, originalFile := closeRootOperation, closeFileOperation
	defer func() { closeRootOperation, closeFileOperation = originalRoot, originalFile }()
	cleanupFault := errors.New("injected completed create root close fault")
	targetClosed := false
	rootFaults := 0
	closeFileOperation = func(file *os.File) error {
		err := originalFile(file)
		if file.Name() == targetOne {
			targetClosed = true
		}
		return err
	}
	closeRootOperation = func(root *os.Root) error {
		err := originalRoot(root)
		if targetClosed && root.Name() == home && rootFaults == 0 {
			rootFaults++
			return errors.Join(err, cleanupFault)
		}
		return err
	}

	_, err = ApplyManagedEntries(p, opts)
	if !errors.Is(err, cleanupFault) || !isCompletedActionCleanup(err) {
		t.Fatalf("ApplyManagedEntries() error = %v, want completed cleanup", err)
	}
	if rootFaults != 1 {
		t.Fatalf("injected root cleanup count = %d, want one", rootFaults)
	}
	if got, readErr := os.ReadFile(targetOne); readErr != nil || string(got) != "one\n" {
		t.Fatalf("completed target = %q, %v", got, readErr)
	}
	if _, statErr := os.Lstat(targetTwo); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("later target exists or stat failed unexpectedly: %v", statErr)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	record, ok := meta.FindByTarget(targetOne)
	if !ok || len(record.Contributions) != 0 || record.Hash != state.HashBytes([]byte("one\n")) {
		t.Fatalf("partial inventory = %#v, want exact completed first action without terminal contributions", record)
	}
	if _, ok := meta.FindByTarget(targetTwo); ok {
		t.Fatalf("partial inventory includes unapplied later target %s", targetTwo)
	}
	if meta.InstalledSelection == nil || len(meta.InstalledSelection.Profiles) != 1 || meta.InstalledSelection.Profiles[0] != "old" {
		t.Fatalf("InstalledSelection = %#v, want previous selection", meta.InstalledSelection)
	}

	closeRootOperation, closeFileOperation = originalRoot, originalFile
	rerun := p
	rerun.Actions = append([]plan.Action(nil), p.Actions...)
	rerun.Actions[0].Status = plan.StatusUnchanged
	commit, err := ApplyManagedEntries(rerun, opts)
	if err != nil {
		t.Fatalf("safe rerun ApplyManagedEntries() error = %v", err)
	}
	if err := commit.Commit(nil); err != nil {
		t.Fatalf("safe rerun Commit() error = %v", err)
	}
	if got, readErr := os.ReadFile(targetTwo); readErr != nil || string(got) != "two\n" {
		t.Fatalf("safe rerun later target = %q, %v", got, readErr)
	}
}

func TestUpdateCleanupReturnsExactPostimageAndJoinsPrimary(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "target")
	if err := os.WriteFile(target, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fileFault := errors.New("injected update file close fault")
	rootFault := errors.New("injected update root close fault")
	originalRoot, originalFile := closeRootOperation, closeFileOperation
	defer func() { closeRootOperation, closeFileOperation = originalRoot, originalFile }()
	fileCalls, rootCalls := 0, 0
	closeFileOperation = func(file *os.File) error {
		err := originalFile(file)
		if file.Name() == target {
			fileCalls++
			return errors.Join(err, fileFault)
		}
		return err
	}
	closeRootOperation = func(root *os.Root) error {
		err := originalRoot(root)
		if root.Name() == home {
			rootCalls++
			return errors.Join(err, rootFault)
		}
		return err
	}
	result, err := updateConfinedRegularFile(target, home, func([]byte) ([]byte, bool, error) {
		return []byte("new\n"), true, nil
	})
	if !errors.Is(err, fileFault) || !errors.Is(err, rootFault) || !isCompletedActionCleanup(err) {
		t.Fatalf("update cleanup error = %v", err)
	}
	if !result.ExactTargetContent || string(result.PreviousTargetContent) != "old\n" || string(result.TargetContent) != "new\n" {
		t.Fatalf("update result = %#v, want exact preimage and postimage", result)
	}
	if fileCalls != 1 || rootCalls != 1 {
		t.Fatalf("update close calls = file %d, root %d; want one each", fileCalls, rootCalls)
	}

	closeFileOperation, closeRootOperation = originalFile, originalRoot
	primaryFault := errors.New("injected update transform fault")
	closeFileOperation = func(file *os.File) error { return errors.Join(originalFile(file), fileFault) }
	closeRootOperation = func(root *os.Root) error { return errors.Join(originalRoot(root), rootFault) }
	_, err = updateConfinedRegularFile(target, home, func([]byte) ([]byte, bool, error) {
		return nil, false, primaryFault
	})
	if !errors.Is(err, primaryFault) || !errors.Is(err, fileFault) || !errors.Is(err, rootFault) || isCompletedActionCleanup(err) {
		t.Fatalf("primary plus cleanup error = %v", err)
	}
}

func TestWriteNewFileCloseFailureCompensatesWithoutRetry(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "target")
	fault := errors.New("injected new file close fault")
	originalFile := closeFileOperation
	defer func() { closeFileOperation = originalFile }()
	calls := 0
	closeFileOperation = func(file *os.File) error {
		calls++
		return errors.Join(originalFile(file), fault)
	}
	err := writeNewFileWithMode(home, target, []byte("content\n"), 0o600)
	if !errors.Is(err, fault) || !strings.Contains(err.Error(), "close file "+target) {
		t.Fatalf("writeNewFileWithMode() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("new file close calls = %d, want exactly one", calls)
	}
	if _, statErr := os.Lstat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("target remains after file-close compensation: %v", statErr)
	}
}

func TestCompletedReconciliationCleanupPersistsReceiptAndPriorSelection(t *testing.T) {
	home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
	sourceName := "shared.json"
	source := filepath.Join(sourceRoot, sourceName)
	target := filepath.Join(home, "shared.json")
	previous := []byte(`{"old":true}`)
	current := []byte(`{"new":true}`)
	live := []byte(`{"old":true,"external":true}`)
	if err := os.WriteFile(source, current, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, live, 0o600); err != nil {
		t.Fatal(err)
	}
	record := recoveryAuthorityRecord(target, sourceName, previous)
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{record}, InstalledSelection: oldSelection}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	action := recoveryAuthorityAction(target, sourceName, previous, fingerprint)
	fault := errors.New("injected reconciliation file close fault")
	originalFile := closeFileOperation
	defer func() { closeFileOperation = originalFile }()
	calls := 0
	closeFileOperation = func(file *os.File) error {
		err := originalFile(file)
		if file.Name() == target {
			calls++
			return errors.Join(err, fault)
		}
		return err
	}
	_, err = ApplyManagedEntries(plan.Plan{Profiles: []string{"new"}, Actions: []plan.Action{action}}, Options{
		Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot,
	})
	if !errors.Is(err, fault) || !isCompletedActionCleanup(err) || calls != 1 {
		t.Fatalf("ApplyManagedEntries() = %v, close calls %d; want completed reconciliation cleanup once", err, calls)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := meta.FindByTarget(target)
	if !ok || updated.PendingReconciliation == nil || updated.PendingReconciliation.TargetHash != state.HashBytes(got) {
		t.Fatalf("reconciliation receipt = %#v, want exact postimage %q", updated.PendingReconciliation, got)
	}
	if len(updated.Contributions) != 1 || updated.Contributions[0].Hash != state.HashBytes(previous) {
		t.Fatalf("committed contribution evidence changed on cleanup failure: %#v", updated.Contributions)
	}
	if meta.InstalledSelection == nil || len(meta.InstalledSelection.Profiles) != 1 || meta.InstalledSelection.Profiles[0] != "old" {
		t.Fatalf("InstalledSelection = %#v, want prior selection", meta.InstalledSelection)
	}
}

func TestCompletedCapturedAdoptionCleanupPersistsRecoveryInventory(t *testing.T) {
	home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
	source := filepath.Join(sourceRoot, "adopt")
	target := filepath.Join(home, "adopt")
	later := filepath.Join(home, "later")
	if err := os.WriteFile(source, []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("adopted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "later"), []byte("later\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldSelection := &state.InstalledSelection{Profiles: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: oldSelection}); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Profiles: []string{"new"}, Actions: []plan.Action{
		{Source: "adopt", Target: target, Strategy: "copy", Status: plan.StatusConflict},
		{Source: "later", Target: later, Strategy: "copy", Status: plan.StatusCreate},
	}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot, ConflictDecisions: map[string]ConflictDecision{target: DecisionAdopt}}
	snapshots, err := CaptureAdoptSnapshots(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	opts.AdoptSnapshots = snapshots
	fault := errors.New("injected completed adoption source close fault")
	originalFile := closeFileOperation
	calls := 0
	closeFileOperation = func(file *os.File) error {
		err := originalFile(file)
		if file.Name() == source {
			content, readErr := os.ReadFile(source)
			if readErr == nil && string(content) == "adopted\n" {
				calls++
				return errors.Join(err, fault)
			}
		}
		return err
	}
	_, applyErr := ApplyManagedEntries(p, opts)
	closeFileOperation = originalFile
	if releaseErr := ReleaseAdoptSnapshots(snapshots); releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if !errors.Is(applyErr, fault) || !isCompletedActionCleanup(applyErr) || calls != 1 {
		t.Fatalf("ApplyManagedEntries() = %v, close calls %d; want completed adoption cleanup once", applyErr, calls)
	}
	if got, readErr := os.ReadFile(source); readErr != nil || string(got) != "adopted\n" {
		t.Fatalf("adopted source = %q, %v", got, readErr)
	}
	if _, statErr := os.Lstat(later); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("later action ran: %v", statErr)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	record, ok := meta.FindByTarget(target)
	if !ok || record.Hash != state.HashBytes([]byte("adopted\n")) || len(record.Contributions) != 0 {
		t.Fatalf("adoption recovery inventory = %#v", record)
	}
	if meta.InstalledSelection == nil || meta.InstalledSelection.Profiles[0] != "old" {
		t.Fatalf("InstalledSelection = %#v, want prior selection", meta.InstalledSelection)
	}
}

func TestLaterPrepareCaptureCleanupPersistsEarlierPrefix(t *testing.T) {
	home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		if err := os.WriteFile(filepath.Join(sourceRoot, name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := plan.Plan{Actions: []plan.Action{
		{Source: "one", Target: filepath.Join(home, "one"), Strategy: "copy", Status: plan.StatusCreate},
		{Source: "two", Target: filepath.Join(home, "two"), Strategy: "copy", Status: plan.StatusCreate},
		{Source: "three", Target: filepath.Join(home, "three"), Strategy: "copy", Status: plan.StatusCreate},
	}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures
	originalRoot := closeRootOperation
	defer func() { closeRootOperation = originalRoot }()
	fault := errors.New("injected later prepare capture root cleanup")
	faults := 0
	closeRootOperation = func(root *os.Root) error {
		err := originalRoot(root)
		if root.Name() == sourceRoot {
			if _, statErr := os.Lstat(filepath.Join(home, "one")); statErr == nil && faults == 0 {
				faults++
				return errors.Join(err, fault)
			}
		}
		return err
	}
	_, err = ApplyManagedEntries(p, opts)
	if !errors.Is(err, fault) || isCompletedActionCleanup(err) || faults != 1 {
		t.Fatalf("ApplyManagedEntries() = %v, faults %d; want later prepare cleanup", err, faults)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.FindByTarget(filepath.Join(home, "one")); !ok {
		t.Fatal("partial inventory missing completed first action")
	}
	for _, name := range []string{"two", "three"} {
		if _, statErr := os.Lstat(filepath.Join(home, name)); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("action %s ran after cleanup failure: %v", name, statErr)
		}
		if _, ok := meta.FindByTarget(filepath.Join(home, name)); ok {
			t.Fatalf("partial inventory includes unapplied action %s", name)
		}
	}
}

func TestCaptureManagedSourcesReportsRootAndIdentityCleanupExactlyOnce(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(sourceRoot, "source")
	target := filepath.Join(home, "target")
	if err := os.WriteFile(source, []byte("content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "source", Target: target, Strategy: "copy", Status: plan.StatusCreate}}}
	rootFault := errors.New("injected root close fault")
	identityFault := errors.New("injected identity close fault")

	t.Run("root close invalidates capture", func(t *testing.T) {
		originalRoot := closeRootOperation
		defer func() { closeRootOperation = originalRoot }()
		calls := 0
		closeRootOperation = func(root *os.Root) error {
			calls++
			return errors.Join(originalRoot(root), rootFault)
		}
		captures, err := CaptureManagedSources(p, Options{SourceRoot: sourceRoot, Home: home})
		if !errors.Is(err, rootFault) || captures != nil {
			t.Fatalf("CaptureManagedSources() = %#v, %v; want no live capture and root fault", captures, err)
		}
		if calls != 1 {
			t.Fatalf("root close calls = %d, want exactly one", calls)
		}
	})

	t.Run("identity release closes once", func(t *testing.T) {
		originalFile := closeFileOperation
		defer func() { closeFileOperation = originalFile }()
		calls := 0
		closeFileOperation = func(file *os.File) error {
			calls++
			return errors.Join(originalFile(file), identityFault)
		}
		captures, err := CaptureManagedSources(p, Options{SourceRoot: sourceRoot, Home: home})
		if err != nil {
			t.Fatal(err)
		}
		err = ReleaseCapturedSources(captures)
		if !errors.Is(err, identityFault) {
			t.Fatalf("ReleaseCapturedSources() error = %v, want identity fault", err)
		}
		if calls != 1 {
			t.Fatalf("identity close calls = %d, want exactly one", calls)
		}
		if err := ReleaseCapturedSources(captures); err != nil || calls != 1 {
			t.Fatalf("second release = %v, calls = %d; genuine close must never be retried after possible fd reuse", err, calls)
		}
	})

	t.Run("simultaneous root and identity cleanup", func(t *testing.T) {
		originalRoot, originalFile := closeRootOperation, closeFileOperation
		defer func() { closeRootOperation, closeFileOperation = originalRoot, originalFile }()
		rootCalls, fileCalls := 0, 0
		closeRootOperation = func(root *os.Root) error {
			rootCalls++
			return errors.Join(originalRoot(root), rootFault)
		}
		closeFileOperation = func(file *os.File) error {
			fileCalls++
			return errors.Join(originalFile(file), identityFault)
		}
		captures, err := CaptureManagedSources(p, Options{SourceRoot: sourceRoot, Home: home})
		if captures != nil || !errors.Is(err, rootFault) || !errors.Is(err, identityFault) {
			t.Fatalf("CaptureManagedSources() = %#v, %v; want both faults and no live capture", captures, err)
		}
		if rootCalls != 1 || fileCalls != 1 {
			t.Fatalf("close calls = root %d, identity %d; want one each", rootCalls, fileCalls)
		}
	})
}

func TestSymlinkDescriptorCleanupPreservesValidResults(t *testing.T) {
	rootPath := t.TempDir()
	resolvedPath := filepath.Join(rootPath, "source")
	if err := os.WriteFile(resolvedPath, []byte("content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	}()
	parentFault := errors.New("injected parent close fault")
	resolvedFault := errors.New("injected resolved close fault")

	originalFile := closeFileOperation
	defer func() { closeFileOperation = originalFile }()
	closeFileOperation = func(file *os.File) error {
		err := originalFile(file)
		return errors.Join(err, parentFault)
	}
	err = symlinkAtRoot(root, "link", "source")
	if !errors.Is(err, parentFault) || !isCompletedActionCleanup(err) {
		t.Fatalf("symlinkAtRoot() error = %v, want completed parent cleanup", err)
	}
	if destination, readErr := os.Readlink(filepath.Join(rootPath, "link")); readErr != nil || destination != "source" {
		t.Fatalf("created symlink = %q, %v", destination, readErr)
	}

	for _, test := range []struct {
		name         string
		failParent   bool
		failResolved bool
		wantParent   bool
		wantResolved bool
	}{
		{name: "parent", failParent: true, wantParent: true},
		{name: "resolved", failResolved: true, wantResolved: true},
		{name: "simultaneous", failParent: true, failResolved: true, wantParent: true, wantResolved: true},
	} {
		t.Run("stat "+test.name, func(t *testing.T) {
			closeFileOperation = func(file *os.File) error {
				err := originalFile(file)
				if test.failParent && file.Name() != "link" {
					return errors.Join(err, parentFault)
				}
				if test.failResolved && file.Name() == "link" {
					return errors.Join(err, resolvedFault)
				}
				return err
			}
			info, err := statSymlinkTargetAtRoot(root, "link")
			if info == nil || !info.Mode().IsRegular() {
				t.Fatalf("stat result = %#v, want valid regular result", info)
			}
			if errors.Is(err, parentFault) != test.wantParent || errors.Is(err, resolvedFault) != test.wantResolved {
				t.Fatalf("stat error = %v", err)
			}
			if err != nil && !strings.Contains(err.Error(), "close file") {
				t.Fatalf("stat error lacks operation context: %v", err)
			}
		})
	}

	closeFileOperation = func(file *os.File) error {
		err := originalFile(file)
		return errors.Join(err, parentFault)
	}
	destination, err := readlinkAtRoot(root, "link")
	if destination != "source" || !errors.Is(err, parentFault) || !strings.Contains(err.Error(), "close file") {
		t.Fatalf("readlinkAtRoot() = %q, %v; want valid destination plus contextual cleanup", destination, err)
	}
}

func TestStatSymlinkNewFileFailureClosesDescriptorOnce(t *testing.T) {
	rootPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootPath, "source"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("source", filepath.Join(rootPath, "link")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("close test root %s: %v", rootPath, err)
		}
	})
	originalNewFile, originalCloseFD := newFileOperation, closeFDOperation
	defer func() { newFileOperation, closeFDOperation = originalNewFile, originalCloseFD }()
	fdFault := errors.New("injected fd close fault")
	closeCalls := 0
	newFileOperation = func(uintptr, string) *os.File { return nil }
	closeFDOperation = func(fd int) error {
		closeCalls++
		return errors.Join(originalCloseFD(fd), fdFault)
	}
	_, err = statSymlinkTargetAtRoot(root, "link")
	if !errors.Is(err, fdFault) || !strings.Contains(err.Error(), "close descriptor for link") {
		t.Fatalf("statSymlinkTargetAtRoot() error = %v", err)
	}
	if closeCalls != 1 {
		t.Fatalf("unix close calls = %d, want exactly one because retry risks fd reuse", closeCalls)
	}
}

func TestNewFileLifecyclesPreserveJoinedPrimaryAndTypedCloseFailures(t *testing.T) {
	tests := []struct {
		name        string
		run         func(*testing.T, string) (string, error)
		wantPresent bool
		write       func(error)
		wantPrimary func(string) string
		wantContext func(string) string
	}{
		{
			name: "copyRegularFile",
			run: func(t *testing.T, dir string) (string, error) {
				source, target := filepath.Join(dir, "copy-source"), filepath.Join(dir, "copy-target")
				if err := os.WriteFile(source, []byte("copy\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return target, copyRegularFile(source, target)
			},
			write: func(primary error) {
				writeFileOperation = func(*os.File, []byte) (int, error) { return 0, primary }
			},
			wantPrimary: func(target string) string { return "write copy target " + target },
			wantContext: func(target string) string { return "close file " + target },
		},
		{
			name: "writeNewFileFromSourceMode",
			run: func(t *testing.T, dir string) (string, error) {
				source, target := filepath.Join(dir, "mode-source"), filepath.Join(dir, "mode-target")
				if err := os.WriteFile(source, []byte("source\n"), 0o640); err != nil {
					t.Fatal(err)
				}
				return target, writeNewFileFromSourceMode(source, target, []byte("written\n"))
			},
			write: func(primary error) {
				writeFileOperation = func(*os.File, []byte) (int, error) { return 0, primary }
			},
			wantPrimary: func(target string) string { return "write new target " + target },
			wantContext: func(target string) string { return "close file " + target },
		},
		{
			name: "restoreCapturedTargetIfAbsent",
			run: func(t *testing.T, dir string) (string, error) {
				root, err := os.OpenRoot(dir)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := root.Close(); err != nil {
						t.Errorf("close restore test root %s: %v", dir, err)
					}
				}()
				relative := "restored-target"
				return filepath.Join(dir, relative), restoreCapturedTargetIfAbsent(root, relative, CapturedSource{
					Content: []byte("restored\n"), Mode: 0o600,
				})
			},
			wantPresent: true,
			write: func(primary error) {
				writeAllFileOperation = func(*os.File, []byte) error { return primary }
			},
			wantPrimary: func(string) string { return "restore adopted target restored-target: write" },
			wantContext: func(string) string { return "close file restored-target" },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalFile, originalWrite, originalWriteAll := closeFileOperation, writeFileOperation, writeAllFileOperation
			defer func() {
				closeFileOperation, writeFileOperation, writeAllFileOperation = originalFile, originalWrite, originalWriteAll
			}()
			primaryFault := &typedInjectedPrimaryError{lifecycle: test.name}
			typedClose := &typedInjectedCleanupError{lifecycle: test.name}
			calls := 0
			test.write(primaryFault)
			closeFileOperation = func(file *os.File) error {
				calls++
				return errors.Join(originalFile(file), typedClose)
			}
			target, err := test.run(t, t.TempDir())
			if !errors.Is(err, primaryFault) || !isCleanupFailure(err) {
				t.Fatalf("%s error = %v, want joined primary and cleanup classification", test.name, err)
			}
			var gotPrimary *typedInjectedPrimaryError
			if !errors.As(err, &gotPrimary) || gotPrimary != primaryFault {
				t.Fatalf("%s error = %v, want errors.As typed primary failure", test.name, err)
			}
			var gotTyped *typedInjectedCleanupError
			if !errors.As(err, &gotTyped) || gotTyped != typedClose {
				t.Fatalf("%s error = %v, want errors.As typed cleanup", test.name, err)
			}
			for _, context := range []string{test.wantPrimary(target), test.wantContext(target)} {
				if !strings.Contains(err.Error(), context) {
					t.Fatalf("%s error = %v, want context %q", test.name, err, context)
				}
			}
			if calls != 1 {
				t.Fatalf("%s close calls = %d, want exactly one", test.name, calls)
			}
			_, statErr := os.Lstat(target)
			if test.wantPresent && statErr != nil {
				t.Fatalf("%s target missing after restoration: %v", test.name, statErr)
			}
			if !test.wantPresent && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("%s target remains after compensation: %v", test.name, statErr)
			}
		})
	}
}

func TestCaptureConfinedSourceJoinsPrimaryAndTypedRootCloseFailures(t *testing.T) {
	rootPath := t.TempDir()
	directorySource := filepath.Join(rootPath, "directory")
	if err := os.Mkdir(directorySource, 0o700); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		path        string
		wantPrimary string
		wantIs      error
	}{
		{name: "stat missing source", path: filepath.Join(rootPath, "missing"), wantPrimary: "inspect confined source", wantIs: os.ErrNotExist},
		{name: "reject nonregular source", path: directorySource, wantPrimary: "does not resolve to a regular file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalRoot := closeRootOperation
			defer func() { closeRootOperation = originalRoot }()
			typedClose := &typedInjectedCleanupError{lifecycle: test.name}
			closeSentinel := errors.New("injected capture root close failure")
			calls := 0
			closeRootOperation = func(root *os.Root) error {
				calls++
				return errors.Join(originalRoot(root), closeSentinel, typedClose)
			}
			capture, err := captureConfinedSource(test.path, rootPath, true)
			if capture.identity != nil {
				t.Fatalf("capture = %#v, want no transferred identity", capture)
			}
			if !errors.Is(err, closeSentinel) || !isCleanupFailure(err) || (test.wantIs != nil && !errors.Is(err, test.wantIs)) {
				t.Fatalf("captureConfinedSource() error = %v, want primary plus cleanup", err)
			}
			var gotTyped *typedInjectedCleanupError
			if !errors.As(err, &gotTyped) || gotTyped != typedClose {
				t.Fatalf("captureConfinedSource() error = %v, want errors.As typed cleanup", err)
			}
			for _, context := range []string{test.wantPrimary, "close filesystem root " + rootPath} {
				if !strings.Contains(err.Error(), context) {
					t.Fatalf("captureConfinedSource() error = %v, want context %q", err, context)
				}
			}
			if calls != 1 {
				t.Fatalf("root close calls = %d, want exactly one", calls)
			}
		})
	}
}
