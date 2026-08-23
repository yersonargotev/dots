package install

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/ownershipevidence"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/textblock"
	"github.com/yersonargotev/dots/internal/version"
	"golang.org/x/sys/unix"
)

// cleanup operations are variables so tests can inject deterministic failures
// without exhausting process descriptors. Tests that replace them must run
// serially and restore the original operation.
var (
	closeRootOperation    = func(root *os.Root) error { return root.Close() }
	closeFileOperation    = func(file *os.File) error { return file.Close() }
	closeFDOperation      = unix.Close
	newFileOperation      = os.NewFile
	writeFileOperation    = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
	writeAllFileOperation = func(file *os.File, data []byte) error { return writeAll(file, data) }
)

type cleanupFailure struct {
	err error
}

func (e *cleanupFailure) Error() string { return e.err.Error() }
func (e *cleanupFailure) Unwrap() error { return e.err }
func (e *cleanupFailure) CleanupFailure() bool {
	return true
}

type completedActionCleanupFailure struct {
	err error
}

func (e *completedActionCleanupFailure) Error() string { return e.err.Error() }
func (e *completedActionCleanupFailure) Unwrap() error { return e.err }

type errorWithPreservedMessage struct {
	message string
	err     error
}

func (e *errorWithPreservedMessage) Error() string { return e.message }
func (e *errorWithPreservedMessage) Unwrap() error { return e.err }

func classifyCleanup(err error) error {
	if err == nil || isCleanupFailure(err) {
		return err
	}
	return &cleanupFailure{err: err}
}

func completedActionCleanup(err error) error {
	if err == nil {
		return nil
	}
	return &completedActionCleanupFailure{err: classifyCleanup(err)}
}

func isCleanupFailure(err error) bool {
	var classified interface{ CleanupFailure() bool }
	return errors.As(err, &classified) && classified.CleanupFailure()
}

func isCompletedActionCleanup(err error) bool {
	var completed *completedActionCleanupFailure
	return errors.As(err, &completed)
}

func withoutCompletedActionMarker(err error) error {
	if err == nil {
		return nil
	}
	var stripped []error
	var collect func(error)
	collect = func(current error) {
		switch typed := current.(type) {
		case *completedActionCleanupFailure:
			collect(typed.err)
		case interface{ Unwrap() []error }:
			for _, nested := range typed.Unwrap() {
				collect(nested)
			}
		case interface{ Unwrap() error }:
			if isCompletedActionCleanup(current) {
				strippedNested := withoutCompletedActionMarker(typed.Unwrap())
				if strippedNested != nil {
					stripped = append(stripped, &errorWithPreservedMessage{
						message: current.Error(),
						err:     strippedNested,
					})
				}
				return
			}
			stripped = append(stripped, current)
		default:
			stripped = append(stripped, current)
		}
	}
	collect(err)
	return errors.Join(stripped...)
}

func closeRootOnce(root *os.Root, path string) error {
	// Never retry a genuine close: the descriptor number may already have been
	// reused even when close reports an error.
	if err := closeRootOperation(root); err != nil {
		return classifyCleanup(fmt.Errorf("close filesystem root %s: %w", path, err))
	}
	return nil
}

func closeFileOnce(file *os.File, path string) error {
	// Never retry a genuine close: the descriptor number may already have been
	// reused even when close reports an error.
	if err := closeFileOperation(file); err != nil {
		return classifyCleanup(fmt.Errorf("close file %s: %w", path, err))
	}
	return nil
}

func closeFDOnce(fd int, target string) error {
	// unix.Close is deliberately attempted exactly once because retrying can
	// close an unrelated descriptor after fd reuse.
	if err := closeFDOperation(fd); err != nil {
		return classifyCleanup(fmt.Errorf("close descriptor for %s: %w", target, err))
	}
	return nil
}

// ConflictDecision describes the explicit per-target action selected for a
// conflict. The zero value is intentionally equivalent to skip so unattended
// installs preserve existing workstation files.
type ConflictDecision string

const (
	DecisionSkip    ConflictDecision = "skip"
	DecisionReplace ConflictDecision = "replace"
	DecisionAdopt   ConflictDecision = "adopt"
)

// Options carries resolved inputs needed to apply an Install Plan.
type Options struct {
	SourceRoot string
	Home       string
	// StateRoot is the directory where Installation Metadata is recorded so
	// dots status can later detect Drift. When empty, metadata is not written.
	StateRoot string
	// ConflictDecisions contains explicit per-target decisions. Missing conflict
	// targets default to skip; there is deliberately no global adopt policy.
	ConflictDecisions map[string]ConflictDecision
	// CapturedSources binds selector-driven actions to the exact confined Source
	// of Truth objects reviewed before dependency and provisioner effects.
	// Explicit installs may leave this nil to retain their historical behavior.
	CapturedSources map[SourceCaptureKey]CapturedSource
	// AdoptSnapshots binds explicit adopt decisions to post-conflict snapshots.
	AdoptSnapshots map[string]AdoptSnapshot
}

// SourceCaptureKey identifies one Source of Truth input for one target. Target
// is included because the same source can legitimately participate in actions
// with different selector authority.
type SourceCaptureKey struct {
	Target string
	Source string
}

// CapturedSource is an immutable snapshot produced by CaptureManagedSources.
// Explicit presence flags preserve empty content and zero-valued metadata.
type CapturedSource struct {
	Content             []byte
	ContentPresent      bool
	Mode                os.FileMode
	ModePresent         bool
	IdentityFingerprint string
	IdentityPresent     bool
	identity            *capturedFileIdentity
}

type capturedFileIdentity struct {
	mu   sync.Mutex
	file *os.File
}

func (i *capturedFileIdentity) stat() (os.FileInfo, error) {
	if i == nil {
		return nil, fmt.Errorf("captured identity is unavailable")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.file == nil {
		return nil, fmt.Errorf("captured identity was released")
	}
	info, err := i.file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat captured identity: %w", err)
	}
	return info, nil
}

func (i *capturedFileIdentity) release() error {
	if i == nil {
		return nil
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.file == nil {
		return nil
	}
	file := i.file
	i.file = nil
	if err := closeFileOnce(file, file.Name()); err != nil {
		return fmt.Errorf("close captured identity for %s: %w", file.Name(), err)
	}
	return nil
}

// SourceCaptureAuthority is the stable, serializable projection of one opaque
// captured Source of Truth input. It is suitable for semantic digests; apply
// authority remains the process-local CapturedSource identity.
type SourceCaptureAuthority struct {
	Target              string `json:"target"`
	Source              string `json:"source"`
	Content             []byte `json:"content"`
	ContentPresent      bool   `json:"content_present"`
	Mode                uint32 `json:"mode"`
	ModePresent         bool   `json:"mode_present"`
	IdentityFingerprint string `json:"identity_fingerprint"`
	IdentityPresent     bool   `json:"identity_present"`
}

// SourceCaptureAuthorities returns a detached, deterministic projection of
// captures without exposing their opaque filesystem identities.
func SourceCaptureAuthorities(captures map[SourceCaptureKey]CapturedSource) []SourceCaptureAuthority {
	keys := make([]SourceCaptureKey, 0, len(captures))
	for key := range captures {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Target != keys[j].Target {
			return keys[i].Target < keys[j].Target
		}
		return keys[i].Source < keys[j].Source
	})
	result := make([]SourceCaptureAuthority, 0, len(keys))
	for _, key := range keys {
		capture := captures[key]
		result = append(result, SourceCaptureAuthority{
			Target: key.Target, Source: key.Source, Content: append([]byte(nil), capture.Content...), ContentPresent: capture.ContentPresent,
			Mode: uint32(capture.Mode), ModePresent: capture.ModePresent,
			IdentityFingerprint: capture.IdentityFingerprint, IdentityPresent: capture.IdentityPresent,
		})
	}
	return result
}

// ReleaseCapturedSources releases the process-local descriptors that pin
// reviewed Source of Truth identities. It is safe to call more than once and
// must be deferred immediately after a successful CaptureManagedSources call.
func ReleaseCapturedSources(captures map[SourceCaptureKey]CapturedSource) error {
	identities := make(map[*capturedFileIdentity]struct{}, len(captures))
	for _, capture := range captures {
		if capture.identity != nil {
			identities[capture.identity] = struct{}{}
		}
	}
	var errs []error
	for identity := range identities {
		if err := identity.release(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// AdoptSnapshot is an immutable post-conflict snapshot produced by
// CaptureAdoptSnapshots. Its fields are opaque so only confined capture can
// grant authority to overwrite the Source of Truth.
type AdoptSnapshot struct {
	target CapturedSource
	source CapturedSource
}

// ReleaseAdoptSnapshots releases the descriptors that pin reviewed adopt
// target and Source of Truth identities. It is safe to call more than once and
// must be deferred immediately after CaptureAdoptSnapshots succeeds.
func ReleaseAdoptSnapshots(snapshots map[string]AdoptSnapshot) error {
	identities := make(map[*capturedFileIdentity]struct{}, len(snapshots)*2)
	for _, snapshot := range snapshots {
		if snapshot.target.identity != nil {
			identities[snapshot.target.identity] = struct{}{}
		}
		if snapshot.source.identity != nil {
			identities[snapshot.source.identity] = struct{}{}
		}
	}
	var errs []error
	for identity := range identities {
		if err := identity.release(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// MetadataCommit finalizes exact contribution evidence after every terminal
// install step has succeeded. Its fields are intentionally opaque so callers
// cannot commit evidence for a plan that was not applied and validated here.
type MetadataCommit struct {
	profiles []string
	tags     []string
	opts     Options
	actions  []metadataAction
}

type metadataAction struct {
	action          plan.Action
	resolvedSources []string
	stagedRecord    state.Record
	pending         *state.ReconciliationReceipt
	recordsEvidence bool
}

// Apply performs safe filesystem changes described by an Install Plan and
// immediately commits their metadata. Command workflows with later terminal
// steps use ApplyManagedEntries and commit only after those steps succeed.
func Apply(p plan.Plan, opts Options) error {
	commit, err := ApplyManagedEntries(p, opts)
	if err != nil {
		return err
	}
	return commit.Commit(nil)
}

// ApplyManagedEntries applies and validates Managed Entries while persisting
// only compatibility inventory and recovery receipts. Exact current
// per-contribution evidence is kept out of Installation Metadata until the
// returned commit reaches terminal success.
func ApplyManagedEntries(p plan.Plan, opts Options) (MetadataCommit, error) {
	return applyManagedEntriesWithApply(p, opts, applyManagedAction)
}

type managedActionResult struct {
	PreviousTargetContent []byte
	TargetContent         []byte
	ExactTargetContent    bool
}

type managedActionApply func(plan.Action, string, Options) (managedActionResult, error)

func applyManagedEntriesWithApply(p plan.Plan, opts Options, applyAction managedActionApply) (MetadataCommit, error) {
	resolvedSources, err := validatePlan(p, opts)
	if err != nil {
		return MetadataCommit{}, err
	}

	appliedActions := append([]plan.Action(nil), p.Actions...)
	appliedReceipts := make([]*state.ReconciliationReceipt, len(p.Actions))
	for i, action := range p.Actions {
		action, err = prepareCapturedAction(action, resolvedSources[i], opts)
		if err != nil {
			if isCleanupFailure(err) {
				err = errors.Join(err, persistCompletedPrefix(p, appliedActions, appliedReceipts, resolvedSources, opts, i))
			}
			return MetadataCommit{}, err
		}
		source := resolvedSources[i][0]
		prepared, receipt, err := applyActionWithReconciliationReceipt(action, source, resolvedSources[i], opts, applyAction)
		if err != nil {
			if isCleanupFailure(err) {
				completed := i
				if isCompletedActionCleanup(err) {
					appliedActions[i] = prepared
					appliedReceipts[i] = receipt
					completed++
				}
				err = errors.Join(err, persistCompletedPrefix(p, appliedActions, appliedReceipts, resolvedSources, opts, completed))
			}
			return MetadataCommit{}, err
		}
		appliedActions[i] = prepared
		appliedReceipts[i] = receipt
	}

	p.Actions = appliedActions
	commit := newMetadataCommit(p, resolvedSources, opts)
	for i := range commit.actions {
		commit.actions[i].pending = appliedReceipts[i]
	}
	if opts.StateRoot != "" {
		if err := commit.captureStagedEvidence(); err != nil {
			return MetadataCommit{}, err
		}
	}
	if err := commit.recordPartialInventory(); err != nil {
		return MetadataCommit{}, err
	}
	return commit, nil
}

func persistCompletedPrefix(p plan.Plan, actions []plan.Action, receipts []*state.ReconciliationReceipt, resolvedSources [][]string, opts Options, completed int) error {
	if completed == 0 {
		return nil
	}
	p.Actions = append([]plan.Action(nil), actions[:completed]...)
	commit := newMetadataCommit(p, resolvedSources[:completed], opts)
	for i := range commit.actions {
		commit.actions[i].pending = receipts[i]
	}
	if opts.StateRoot != "" {
		if err := commit.captureStagedEvidence(); err != nil {
			return fmt.Errorf("capture completed install prefix evidence: %w", err)
		}
	}
	if err := commit.recordPartialInventory(); err != nil {
		return fmt.Errorf("persist completed install prefix: %w", err)
	}
	return nil
}

func applyManagedAction(action plan.Action, source string, opts Options) (managedActionResult, error) {
	switch action.Status {
	case plan.StatusUnchanged:
		return managedActionResult{}, nil
	case plan.StatusCreate:
		return managedActionResult{}, applyCreate(action, source, opts)
	case plan.StatusUpdate:
		return applyUpdate(action, source, opts)
	case plan.StatusMigrate:
		return managedActionResult{}, applyMigration(action, source, opts)
	case plan.StatusConflict:
		switch conflictDecision(action, opts) {
		case DecisionSkip:
			// Safe default for unresolved conflicts is skip: do not mutate the
			// existing workstation target, but continue applying safe actions.
			return managedActionResult{}, nil
		case DecisionReplace:
			return managedActionResult{}, applyReplace(action, source, opts)
		case DecisionAdopt:
			return managedActionResult{}, applyAdopt(action, source, opts)
		}
	}
	return managedActionResult{}, nil
}

func applyActionWithReconciliationReceipt(action plan.Action, source string, resolvedSources []string, opts Options, applyAction managedActionApply) (prepared plan.Action, receipt *state.ReconciliationReceipt, resultErr error) {
	prepared = action
	if !requiresReconciliationReceipt(action, opts) {
		_, err := applyAction(action, source, opts)
		return prepared, nil, err
	}

	locked, err := state.LockMetadata(state.Path(opts.StateRoot))
	if err != nil {
		return prepared, nil, err
	}
	actionCompleted := false
	defer func() {
		if closeErr := locked.Close(); closeErr != nil {
			closeErr = classifyCleanup(fmt.Errorf("close installation metadata lock for %s: %w", action.Target, closeErr))
			if actionCompleted {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	meta, err := locked.Load()
	if err != nil {
		return prepared, nil, err
	}
	if err := validateAuthorizingRecord(meta, action, action.PreviousReconciliationReceipt); err != nil {
		return prepared, nil, err
	}

	prepared, sourceContents, err := snapshotReconciliationSources(action, resolvedSources, opts.SourceRoot)
	if err != nil {
		return action, nil, err
	}
	result, applyErr := applyAction(prepared, source, opts)
	if applyErr != nil && !isCompletedActionCleanup(applyErr) {
		return prepared, nil, applyErr
	}
	if !result.ExactTargetContent {
		return prepared, nil, fmt.Errorf("record reconciliation receipt for %s: apply did not return exact target bytes", action.Target)
	}
	receipt = &state.ReconciliationReceipt{
		TargetHash:   state.HashBytes(result.TargetContent),
		Sources:      actionSourceList(action),
		SourceHashes: make([]string, len(sourceContents)),
		Strategy:     action.Strategy,
		Ownership:    ownershipevidence.For(action.Strategy, action.Ownership).Ownership(),
	}
	for i := range sourceContents {
		receipt.SourceHashes[i] = state.HashBytes(sourceContents[i])
	}
	meta.Version = state.CurrentVersion
	meta.Provenance = state.CaptureProvenance(opts.SourceRoot, version.Value)
	setPendingReconciliation(&meta, action.Target, receipt)
	if err := locked.Save(meta); err != nil {
		rollbackErr := restoreConfinedRegularFile(action.Target, opts.Home, result.TargetContent, result.PreviousTargetContent)
		return prepared, nil, errors.Join(fmt.Errorf("persist reconciliation receipt for %s: %w", action.Target, err), withoutCompletedActionMarker(applyErr), rollbackErr)
	}
	actionCompleted = true
	if applyErr != nil {
		return prepared, receipt, completedActionCleanup(applyErr)
	}
	return prepared, receipt, nil
}

func requiresReconciliationReceipt(action plan.Action, opts Options) bool {
	return opts.StateRoot != "" && action.Status == plan.StatusUpdate && action.Strategy == "copy" &&
		action.PreviousRecordFingerprint != "" && (len(action.PreviousContent) > 0 || action.PreviousHash != "")
}

func snapshotReconciliationSources(action plan.Action, resolvedSources []string, sourceRoot string) (plan.Action, [][]byte, error) {
	contents := make([][]byte, len(resolvedSources))
	for i, source := range resolvedSources {
		data, err := readConfinedRegularFile(source, sourceRoot)
		if err != nil {
			return action, nil, fmt.Errorf("snapshot reconciliation source %s: %w", source, err)
		}
		contents[i] = data
	}
	prepared := action
	if len(action.Sources) == 0 {
		prepared.Content = append([]byte{}, contents[0]...)
		return prepared, contents, nil
	}
	composed := append([]byte{}, contents[0]...)
	for i := 1; i < len(contents); i++ {
		merged, err := configsubset.MergeJSON(composed, contents[i])
		if err != nil {
			return action, nil, fmt.Errorf("compose snapshotted reconciliation source %s: %w", action.Sources[i], err)
		}
		composed = merged
	}
	if !bytes.Equal(composed, action.Content) {
		return action, nil, fmt.Errorf("install plan source content changed before reconciliation for %s", action.Target)
	}
	prepared.Content = composed
	return prepared, contents, nil
}

func readConfinedRegularFile(path, rootPath string) ([]byte, error) {
	captured, err := captureConfinedSource(path, rootPath, true)
	if err != nil {
		return nil, err
	}
	data := append([]byte(nil), captured.Content...)
	return data, captured.identity.release()
}

// CaptureManagedSources snapshots selector-driven Source of Truth inputs under
// os.Root confinement. Callers pass the result back in Options for both early
// validation and per-action apply revalidation.
func CaptureManagedSources(p plan.Plan, opts Options) (result map[SourceCaptureKey]CapturedSource, resultErr error) {
	captureOpts := opts
	captureOpts.CapturedSources = nil
	resolvedSources, err := validatePlan(p, captureOpts)
	if err != nil {
		return nil, err
	}
	captures := make(map[SourceCaptureKey]CapturedSource)
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, ReleaseCapturedSources(captures))
		}
	}()
	for i, action := range p.Actions {
		decision := conflictDecision(action, opts)
		if !recordsMetadata(action, decision) || decision == DecisionAdopt {
			continue
		}
		sourceNames := actionSourceList(action)
		for j, source := range resolvedSources[i] {
			captured, err := captureConfinedSource(source, opts.SourceRoot, action.Strategy == "copy")
			if err != nil {
				return nil, fmt.Errorf("capture source %s for %s: %w", sourceNames[j], action.Target, err)
			}
			key := SourceCaptureKey{Target: action.Target, Source: sourceNames[j]}
			if previous, ok := captures[key]; ok {
				if err := previous.identity.release(); err != nil {
					return nil, errors.Join(err, captured.identity.release())
				}
			}
			captures[key] = captured
		}
	}
	return captures, nil
}

// CaptureAdoptSnapshots captures the post-conflict target and Source of Truth
// authority for every explicit adopt decision. It must run after conflict
// selection and before any unrelated external effects.
func CaptureAdoptSnapshots(p plan.Plan, opts Options) (result map[string]AdoptSnapshot, resultErr error) {
	captureOpts := opts
	captureOpts.AdoptSnapshots = nil
	resolvedSources, err := validatePlan(p, captureOpts)
	if err != nil {
		return nil, err
	}
	snapshots := make(map[string]AdoptSnapshot)
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, ReleaseAdoptSnapshots(snapshots))
		}
	}()
	for i, action := range p.Actions {
		if action.Status != plan.StatusConflict || conflictDecision(action, opts) != DecisionAdopt {
			continue
		}
		target, err := captureConfinedSource(action.Target, opts.Home, true)
		if err != nil {
			return nil, fmt.Errorf("capture adopt target %s: %w", action.Target, err)
		}
		source, err := captureConfinedSource(resolvedSources[i][0], opts.SourceRoot, true)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("capture adopt source %s: %w", action.Source, err), target.identity.release())
		}
		if previous, ok := snapshots[action.Target]; ok {
			if err := errors.Join(previous.target.identity.release(), previous.source.identity.release()); err != nil {
				return nil, errors.Join(err, target.identity.release(), source.identity.release())
			}
		}
		snapshots[action.Target] = AdoptSnapshot{target: target, source: source}
	}
	return snapshots, nil
}

func captureConfinedSource(path, rootPath string, contentRequired bool) (CapturedSource, error) {
	rootAbs, err := cleanAbs(rootPath)
	if err != nil {
		return CapturedSource{}, fmt.Errorf("resolve source root %s: %w", rootPath, err)
	}
	pathAbs, err := cleanAbs(path)
	if err != nil {
		return CapturedSource{}, fmt.Errorf("resolve source %s: %w", path, err)
	}
	relative, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return CapturedSource{}, fmt.Errorf("confine source %s beneath source root %s", path, rootAbs)
	}
	root, err := os.OpenRoot(rootAbs)
	if err != nil {
		return CapturedSource{}, fmt.Errorf("open source root %s: %w", rootAbs, err)
	}
	observed, err := root.Stat(relative)
	if err != nil {
		return CapturedSource{}, errors.Join(fmt.Errorf("inspect confined source %s: %w", path, err), closeRootOnce(root, rootAbs))
	}
	if contentRequired && !observed.Mode().IsRegular() {
		return CapturedSource{}, errors.Join(fmt.Errorf("confined source %s does not resolve to a regular file", path), closeRootOnce(root, rootAbs))
	}
	file, err := root.OpenFile(relative, os.O_RDONLY, 0)
	if err != nil {
		return CapturedSource{}, errors.Join(fmt.Errorf("open confined source %s: %w", path, err), closeRootOnce(root, rootAbs))
	}
	info, err := file.Stat()
	if err != nil {
		return CapturedSource{}, errors.Join(fmt.Errorf("stat confined source %s: %w", path, err), closeFileOnce(file, path), closeRootOnce(root, rootAbs))
	}
	if (contentRequired && !info.Mode().IsRegular()) || !os.SameFile(observed, info) {
		return CapturedSource{}, errors.Join(fmt.Errorf("install plan is stale: source %s changed identity before snapshot", path), closeFileOnce(file, path), closeRootOnce(root, rootAbs))
	}
	captured := CapturedSource{
		Mode:                info.Mode(),
		ModePresent:         true,
		IdentityFingerprint: fmt.Sprintf("%s:%d:%d:%d", info.Name(), info.Size(), info.Mode(), info.ModTime().UnixNano()),
		IdentityPresent:     true,
		identity:            &capturedFileIdentity{file: file},
	}
	if contentRequired {
		data, err := io.ReadAll(file)
		if err != nil {
			return CapturedSource{}, errors.Join(fmt.Errorf("read confined source %s: %w", path, err), captured.identity.release(), closeRootOnce(root, rootAbs))
		}
		captured.Content = append([]byte{}, data...)
		captured.ContentPresent = true
	}
	if err := closeRootOnce(root, rootAbs); err != nil {
		// The root must be released before ownership of the captured descriptor
		// transfers to the caller. A root-close fault invalidates the capture.
		return CapturedSource{}, errors.Join(err, captured.identity.release())
	}
	return captured, nil
}

func prepareCapturedAction(action plan.Action, resolvedSources []string, opts Options) (plan.Action, error) {
	if len(opts.CapturedSources) == 0 {
		return action, nil
	}
	sourceNames := actionSourceList(action)
	hasTargetCapture := false
	for key := range opts.CapturedSources {
		if key.Target == action.Target {
			hasTargetCapture = true
			break
		}
	}
	if !hasTargetCapture {
		return action, nil
	}
	contents := make([][]byte, len(sourceNames))
	for i, sourceName := range sourceNames {
		captured, ok := opts.CapturedSources[SourceCaptureKey{Target: action.Target, Source: sourceName}]
		if !ok {
			return action, fmt.Errorf("captured source %q is missing for %s", sourceName, action.Target)
		}
		current, err := captureConfinedSource(resolvedSources[i], opts.SourceRoot, captured.ContentPresent)
		if err != nil {
			return action, fmt.Errorf("revalidate captured source %s for %s: %w", sourceName, action.Target, err)
		}
		label := fmt.Sprintf("captured source %s for %s", sourceName, action.Target)
		if err := validateCapturedSource(captured, current, label, func(component string) error {
			return fmt.Errorf("install plan is stale: captured source %s changed %s for %s", sourceName, component, action.Target)
		}); err != nil {
			return action, err
		}
		if captured.ContentPresent {
			contents[i] = append([]byte{}, captured.Content...)
		}
	}
	if action.Strategy != "copy" {
		return action, nil
	}
	prepared := action
	if len(contents) == 1 {
		prepared.Content = contents[0]
		return prepared, nil
	}
	composed := append([]byte{}, contents[0]...)
	for i := 1; i < len(contents); i++ {
		merged, err := configsubset.MergeJSON(composed, contents[i])
		if err != nil {
			return action, fmt.Errorf("compose captured source %s for %s: %w", sourceNames[i], action.Target, err)
		}
		composed = merged
	}
	prepared.Content = composed
	return prepared, nil
}

func validateCapturedFile(path, rootPath, label string, expected CapturedSource) error {
	current, err := captureConfinedSource(path, rootPath, true)
	if err != nil {
		return fmt.Errorf("validate %s: %w", label, err)
	}
	return validateCapturedSource(expected, current, label, func(component string) error {
		return fmt.Errorf("%s changed %s", label, component)
	})
}

func validateCapturedSource(expected, current CapturedSource, label string, changed func(string) error) (resultErr error) {
	defer func() {
		if err := current.identity.release(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release current %s identity: %w", label, err))
		}
	}()
	if expected.IdentityPresent {
		same, err := sameCapturedIdentity(expected, current)
		if err != nil {
			return fmt.Errorf("%s identity authority: %w", label, err)
		}
		if !same {
			return changed("identity")
		}
	}
	if expected.ModePresent && current.Mode.Perm() != expected.Mode.Perm() {
		return changed("mode")
	}
	if expected.ContentPresent && (!current.ContentPresent || !bytes.Equal(current.Content, expected.Content)) {
		return changed("content")
	}
	return nil
}

func sameCapturedIdentity(expected, current CapturedSource) (bool, error) {
	expectedInfo, err := expected.identity.stat()
	if err != nil {
		return false, err
	}
	currentInfo, err := current.identity.stat()
	if err != nil {
		return false, err
	}
	return os.SameFile(expectedInfo, currentInfo), nil
}

func actionSourceList(action plan.Action) []string {
	if len(action.Sources) > 0 {
		return append([]string(nil), action.Sources...)
	}
	return []string{action.Source}
}

// ValidateManagedEntries validates the complete forward Install Plan without
// mutating the filesystem or Installation Metadata. Action workflows call it
// before dependency or provisioner effects, then ApplyManagedEntries repeats
// validation against the latest filesystem state immediately before applying.
func ValidateManagedEntries(p plan.Plan, opts Options) error {
	_, err := validatePlan(p, opts)
	return err
}

// Commit revalidates sources and live targets, then atomically records exact
// contribution evidence and, when supplied, the authoritative selection.
func (c MetadataCommit) Commit(installed *state.InstalledSelection) error {
	return c.commitSelection(nil, installed, false)
}

// CommitTransition atomically records terminal evidence and replaces the
// Installed Selection only when its exact prior value still matches expected.
// In particular, nil authority is distinct from an explicitly empty selection.
func (c MetadataCommit) CommitTransition(expected, installed *state.InstalledSelection) error {
	return c.commitSelection(expected, installed, true)
}

func (c MetadataCommit) commitSelection(expected, installed *state.InstalledSelection, transition bool) error {
	if c.opts.StateRoot == "" {
		return nil
	}
	path := state.Path(c.opts.StateRoot)
	return state.Update(path, func(meta *state.Metadata) error {
		if transition && !reflect.DeepEqual(meta.InstalledSelection, expected) {
			return fmt.Errorf("installed selection changed concurrently")
		}
		if err := c.validateTerminalPaths(); err != nil {
			return err
		}
		meta.Version = state.CurrentVersion
		meta.Provenance = state.CaptureProvenance(c.opts.SourceRoot, version.Value)

		now := time.Now().UTC().Format(time.RFC3339)
		legacyTargets := map[string]struct{}{}
		for _, stagedAction := range c.actions {
			if !stagedAction.recordsEvidence {
				continue
			}
			action := stagedAction.action
			if action.PreviousRecordFingerprint != "" {
				expectedReceipt := action.PreviousReconciliationReceipt
				if stagedAction.pending != nil {
					expectedReceipt = stagedAction.pending
				}
				if err := validateAuthorizingRecord(*meta, action, expectedReceipt); err != nil {
					return err
				}
			}
			staged, err := stagedAction.evidence()
			if err != nil {
				return err
			}
			current, err := buildMetadataRecord(c.profiles, c.tags, action, stagedAction.resolvedSources, staged.InstalledAt)
			if err != nil {
				return err
			}
			if !sameRecordEvidence(staged, current) {
				return fmt.Errorf("source contribution evidence changed before terminal metadata commit for %s: %w", action.Target, ownershipevidence.ErrDrift)
			}
			if err := validateMetadataRecord(action, stagedAction.resolvedSources, staged); err != nil {
				return fmt.Errorf("validate staged contribution evidence for %s: %w", action.Target, err)
			}
			staged.InstalledAt = now
			upsertRecord(meta, staged)
			if action.Migration != nil && action.Migration.LegacyTarget != "" {
				legacyTargets[action.Migration.LegacyTarget] = struct{}{}
			}
		}
		for target := range legacyTargets {
			*meta = meta.Remove(target)
		}
		if transition {
			meta.InstalledSelection = cloneInstalledSelection(installed)
		} else if installed != nil {
			meta.InstalledSelection = cloneInstalledSelection(installed)
		}
		return nil
	})
}

func cloneInstalledSelection(installed *state.InstalledSelection) *state.InstalledSelection {
	if installed == nil {
		return nil
	}
	cloned := *installed
	cloned.Profiles = append([]string(nil), installed.Profiles...)
	cloned.ExtraTags = append([]string(nil), installed.ExtraTags...)
	cloned.ResolvedTags = append([]string(nil), installed.ResolvedTags...)
	return &cloned
}

func (c *MetadataCommit) captureStagedEvidence() error {
	now := time.Now().UTC().Format(time.RFC3339)
	for i := range c.actions {
		stagedAction := &c.actions[i]
		if !stagedAction.recordsEvidence {
			continue
		}
		record, err := buildMetadataRecord(c.profiles, c.tags, stagedAction.action, stagedAction.resolvedSources, now)
		if err != nil {
			return err
		}
		stagedAction.stagedRecord = record
		if stagedAction.pending != nil {
			if err := validateRecordAgainstReceipt(record, stagedAction.pending); err != nil {
				return fmt.Errorf("validate snapshotted reconciliation evidence for %s: %w", stagedAction.action.Target, err)
			}
		}
	}
	return nil
}

func validateRecordAgainstReceipt(record state.Record, receipt *state.ReconciliationReceipt) error {
	if receipt == nil || record.Strategy != receipt.Strategy || record.Ownership != receipt.Ownership ||
		!reflect.DeepEqual(record.SourceList(), receipt.Sources) || len(record.Contributions) != len(receipt.SourceHashes) {
		return fmt.Errorf("record does not match reconciliation receipt identity")
	}
	for i := range record.Contributions {
		if record.Contributions[i].Source != receipt.Sources[i] || record.Contributions[i].Hash != receipt.SourceHashes[i] {
			return fmt.Errorf("source %q changed after reconciliation", receipt.Sources[i])
		}
	}
	return nil
}

// recordPartialInventory retains the existing rerunnable failed-install
// inventory without replacing any previously committed exact evidence.
func (c MetadataCommit) recordPartialInventory() error {
	if c.opts.StateRoot == "" {
		return nil
	}
	path := state.Path(c.opts.StateRoot)
	return state.Update(path, func(meta *state.Metadata) error {
		meta.Version = state.CurrentVersion
		meta.Provenance = state.CaptureProvenance(c.opts.SourceRoot, version.Value)
		for _, stagedAction := range c.actions {
			if !stagedAction.recordsEvidence {
				continue
			}
			action := stagedAction.action
			if hasCommittedContributions(*meta, action.Target) {
				if action.PreviousRecordFingerprint != "" {
					expectedReceipt := action.PreviousReconciliationReceipt
					if stagedAction.pending != nil {
						expectedReceipt = stagedAction.pending
					}
					if err := validateAuthorizingRecord(*meta, action, expectedReceipt); err != nil {
						return err
					}
				}
				continue
			}
			record, err := stagedAction.evidence()
			if err != nil {
				return err
			}
			record.Contributions = nil
			upsertRecord(meta, record)
		}
		return nil
	})
}

func validateAuthorizingRecord(meta state.Metadata, action plan.Action, expectedReceipt *state.ReconciliationReceipt) error {
	record, ok := meta.FindByTarget(action.Target)
	if !ok {
		return fmt.Errorf("validate reconciliation receipt for %s: authorizing record disappeared", action.Target)
	}
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		return err
	}
	if action.PreviousRecordFingerprint == "" || fingerprint != action.PreviousRecordFingerprint {
		return fmt.Errorf("validate reconciliation receipt for %s: authorizing record changed concurrently", action.Target)
	}
	if !reflect.DeepEqual(record.PendingReconciliation, expectedReceipt) {
		return fmt.Errorf("validate reconciliation receipt for %s: receipt changed concurrently", action.Target)
	}
	return nil
}

func setPendingReconciliation(meta *state.Metadata, target string, receipt *state.ReconciliationReceipt) {
	for i := range meta.Entries {
		if meta.Entries[i].Target != target {
			continue
		}
		pending := *receipt
		pending.Sources = append([]string(nil), receipt.Sources...)
		pending.SourceHashes = append([]string(nil), receipt.SourceHashes...)
		meta.Entries[i].PendingReconciliation = &pending
		return
	}
}

func (a metadataAction) evidence() (state.Record, error) {
	if a.stagedRecord.Target == "" {
		return state.Record{}, fmt.Errorf("terminal metadata commit has no staged evidence for %s", a.action.Target)
	}
	return a.stagedRecord, nil
}

func buildMetadataRecord(profiles, tags []string, action plan.Action, resolvedSources []string, installedAt string) (state.Record, error) {
	mode := ownershipevidence.For(action.Strategy, action.Ownership)
	contributions, projectionInputs, err := captureContributions(mode, action, resolvedSources)
	if err != nil {
		return state.Record{}, err
	}
	targetWide, err := mode.Project(action.Target, projectionInputs)
	if err != nil {
		return state.Record{}, err
	}
	err = validateProjectedEvidence(mode, action, resolvedSources, targetWide)
	if err != nil {
		return state.Record{}, fmt.Errorf("validate terminal contribution evidence for %s: %w", action.Target, err)
	}
	return state.Record{
		Target:         action.Target,
		Source:         action.Source,
		Sources:        append([]string(nil), action.Sources...),
		Strategy:       action.Strategy,
		Ownership:      mode.Ownership(),
		OwnedContent:   append([]byte(nil), targetWide.OwnedContent...),
		OwnedBytes:     append([]byte(nil), targetWide.OwnedBytes...),
		SeededBaseline: append([]byte(nil), targetWide.SeededBaseline...),
		Contributions:  contributions,
		Hash:           targetWide.Hash,
		InstalledAt:    installedAt,
		Profiles:       append([]string(nil), profiles...),
		Tags:           append([]string(nil), tags...),
	}, nil
}

func validateMetadataRecord(action plan.Action, resolvedSources []string, record state.Record) error {
	mode := ownershipevidence.For(action.Strategy, record.Ownership)
	return validateProjectedEvidence(mode, action, resolvedSources, state.Contribution{
		Ownership:        record.Ownership,
		EvidenceRecorded: true,
		Hash:             record.Hash,
		OwnedContent:     append([]byte(nil), record.OwnedContent...),
		OwnedBytes:       append([]byte(nil), record.OwnedBytes...),
		SeededBaseline:   append([]byte(nil), record.SeededBaseline...),
	})
}

func validateProjectedEvidence(mode ownershipevidence.Mode, action plan.Action, resolvedSources []string, evidence state.Contribution) error {
	if action.Migration != nil && mode.Ownership() == "seeded" {
		return mode.Validate(action.Target, resolvedSources, evidence, action.Migration.FinalContent)
	}
	return mode.Validate(action.Target, resolvedSources, evidence)
}

func sameRecordEvidence(staged, current state.Record) bool {
	if staged.Ownership != current.Ownership || staged.Hash != current.Hash ||
		!bytes.Equal(staged.OwnedContent, current.OwnedContent) ||
		!bytes.Equal(staged.OwnedBytes, current.OwnedBytes) ||
		!bytes.Equal(staged.SeededBaseline, current.SeededBaseline) ||
		len(staged.Contributions) != len(current.Contributions) {
		return false
	}
	for i := range staged.Contributions {
		if !sameContributionEvidence(staged.Contributions[i], current.Contributions[i]) {
			return false
		}
	}
	return true
}

func sameContributionEvidence(staged, current state.Contribution) bool {
	if staged.Source != current.Source || staged.Ownership != current.Ownership ||
		staged.EvidenceRecorded != current.EvidenceRecorded || staged.Hash != current.Hash ||
		len(staged.SelectorTags) != len(current.SelectorTags) ||
		!bytes.Equal(staged.OwnedContent, current.OwnedContent) ||
		!bytes.Equal(staged.OwnedBytes, current.OwnedBytes) ||
		!bytes.Equal(staged.SeededBaseline, current.SeededBaseline) {
		return false
	}
	for i := range staged.SelectorTags {
		if staged.SelectorTags[i] != current.SelectorTags[i] {
			return false
		}
	}
	return true
}

func captureContributions(mode ownershipevidence.Mode, action plan.Action, resolvedSources []string) ([]state.Contribution, []state.Contribution, error) {
	planned := action.Contributions
	if len(planned) == 0 {
		if len(resolvedSources) != 1 {
			return nil, nil, fmt.Errorf("record contribution evidence for %s: no contribution identities for %d resolved sources", action.Target, len(resolvedSources))
		}
		recorded, err := mode.Capture(action.Source, resolvedSources[0], nil, seededBaseline(action))
		if err != nil {
			return nil, nil, err
		}
		return nil, []state.Contribution{recorded}, nil
	}
	if len(planned) != len(resolvedSources) {
		return nil, nil, fmt.Errorf("record contribution evidence for %s: %d contributions for %d resolved sources", action.Target, len(planned), len(resolvedSources))
	}

	contributions := make([]state.Contribution, 0, len(planned))
	for i, contribution := range planned {
		recorded, err := mode.Capture(contribution.Source, resolvedSources[i], contribution.SelectorTags, seededBaseline(action))
		if err != nil {
			return nil, nil, err
		}
		contributions = append(contributions, recorded)
	}
	return contributions, contributions, nil
}

func seededBaseline(action plan.Action) []byte {
	if action.Migration != nil && action.Migration.RecordedBaseline != nil {
		return action.Migration.RecordedBaseline
	}
	return nil
}

func hasCommittedContributions(meta state.Metadata, target string) bool {
	for _, record := range meta.Entries {
		if record.Target == target {
			return len(record.Contributions) > 0
		}
	}
	return false
}

func newMetadataCommit(p plan.Plan, resolvedSources [][]string, opts Options) MetadataCommit {
	commit := MetadataCommit{
		profiles: append([]string(nil), p.Profiles...),
		tags:     append([]string(nil), p.Tags...),
		opts:     cloneOptions(opts),
		actions:  make([]metadataAction, len(p.Actions)),
	}
	for i, action := range p.Actions {
		commit.actions[i] = metadataAction{
			action:          action,
			resolvedSources: append([]string(nil), resolvedSources[i]...),
			recordsEvidence: recordsContributionEvidence(action, conflictDecision(action, opts)),
		}
	}
	return commit
}

func cloneOptions(opts Options) Options {
	cloned := opts
	if opts.ConflictDecisions != nil {
		cloned.ConflictDecisions = make(map[string]ConflictDecision, len(opts.ConflictDecisions))
		for target, decision := range opts.ConflictDecisions {
			cloned.ConflictDecisions[target] = decision
		}
	}
	if opts.CapturedSources != nil {
		cloned.CapturedSources = make(map[SourceCaptureKey]CapturedSource, len(opts.CapturedSources))
		for key, capture := range opts.CapturedSources {
			capture.Content = append([]byte(nil), capture.Content...)
			cloned.CapturedSources[key] = capture
		}
	}
	if opts.AdoptSnapshots != nil {
		cloned.AdoptSnapshots = make(map[string]AdoptSnapshot, len(opts.AdoptSnapshots))
		for target, snapshot := range opts.AdoptSnapshots {
			snapshot.target.Content = append([]byte(nil), snapshot.target.Content...)
			snapshot.source.Content = append([]byte(nil), snapshot.source.Content...)
			cloned.AdoptSnapshots[target] = snapshot
		}
	}
	return cloned
}

// validateTerminalPaths repeats the security boundary checks that can become
// stale while Provisioners run. It intentionally does not repeat pre-apply
// target-status checks, because successfully created targets now exist.
func (c MetadataCommit) validateTerminalPaths() error {
	home, err := cleanAbs(c.opts.Home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	sourceRoot, err := cleanAbs(c.opts.SourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source root: %w", err)
	}
	if err := validateStateRoot(c.opts.StateRoot, home); err != nil {
		return err
	}

	seenTargets := map[string]struct{}{}
	for _, stagedAction := range c.actions {
		if !stagedAction.recordsEvidence {
			continue
		}
		action := stagedAction.action
		if err := plan.ValidateResolvedTarget(action.Target, home); err != nil {
			return err
		}
		targetKey, err := cleanAbs(action.Target)
		if err != nil {
			return fmt.Errorf("resolve target %s: %w", action.Target, err)
		}
		if _, exists := seenTargets[targetKey]; exists {
			return fmt.Errorf("terminal metadata commit contains duplicate target %s", targetKey)
		}
		seenTargets[targetKey] = struct{}{}
		if err := validateTargetParentInsideHome(action.Target, home); err != nil {
			return err
		}
		if action.Strategy == "copy" {
			if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(action.Target, home, "terminal managed target"); err != nil {
				return err
			}
		}

		resolvedSources, err := validateActionSources(action, sourceRoot, stagedAction.resolvedSources)
		if err != nil {
			return err
		}
		for _, source := range resolvedSources {
			if err := validateSource(action.Strategy, source, sourceRoot); err != nil {
				return err
			}
		}
		if len(action.Sources) > 0 {
			composed, err := configsubset.ComposeJSONFiles(resolvedSources)
			if err != nil {
				return fmt.Errorf("validate terminal composed target %s: %w", action.Target, err)
			}
			if !bytes.Equal(composed, action.Content) {
				return fmt.Errorf("install plan composed content changed before terminal metadata commit for %s", action.Target)
			}
		}
	}
	return nil
}

func upsertRecord(meta *state.Metadata, rec state.Record) {
	for i := range meta.Entries {
		if meta.Entries[i].Target == rec.Target {
			meta.Entries[i] = rec
			return
		}
	}
	meta.Entries = append(meta.Entries, rec)
}

func validatePlan(p plan.Plan, opts Options) ([][]string, error) {
	if opts.Home == "" {
		return nil, fmt.Errorf("install home is required")
	}
	if opts.SourceRoot == "" {
		return nil, fmt.Errorf("install source root is required")
	}
	home, err := cleanAbs(opts.Home)
	if err != nil {
		return nil, fmt.Errorf("resolve home: %w", err)
	}
	sourceRoot, err := cleanAbs(opts.SourceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve source root: %w", err)
	}
	if err := validateStateRoot(opts.StateRoot, home); err != nil {
		return nil, err
	}

	seenTargets := map[string]struct{}{}
	validatedLegacyRemovals := map[string]struct{}{}
	resolvedSources := make([][]string, len(p.Actions))
	for i, action := range p.Actions {
		if err := plan.ValidateResolvedTarget(action.Target, home); err != nil {
			return nil, err
		}
		targetKey, err := cleanAbs(action.Target)
		if err != nil {
			return nil, fmt.Errorf("resolve target %s: %w", action.Target, err)
		}
		if _, ok := seenTargets[targetKey]; ok {
			return nil, fmt.Errorf("install plan contains duplicate target %s", targetKey)
		}
		seenTargets[targetKey] = struct{}{}
		resolvedSources[i], err = validateActionSources(action, sourceRoot, nil)
		if err != nil {
			return nil, err
		}
		if _, err := prepareCapturedAction(action, resolvedSources[i], opts); err != nil {
			return nil, err
		}
		source := resolvedSources[i][0]
		if len(action.Sources) > 0 {
			for _, composedSource := range resolvedSources[i] {
				if err := validateSource(action.Strategy, composedSource, sourceRoot); err != nil {
					return nil, err
				}
			}
			composed, err := configsubset.ComposeJSONFiles(resolvedSources[i])
			if err != nil {
				return nil, fmt.Errorf("validate composed target %s: %w", action.Target, err)
			}
			if !bytes.Equal(composed, action.Content) {
				return nil, fmt.Errorf("install plan composed content is stale for %s", action.Target)
			}
		}

		switch action.Status {
		case plan.StatusCreate:
			if !supportedStrategy(action.Strategy) {
				return nil, fmt.Errorf("install strategy %q is not supported for %s", action.Strategy, action.Target)
			}
			if action.LegacyParent != "" {
				if _, ok := validatedLegacyRemovals[action.LegacyParent]; !ok {
					return nil, fmt.Errorf("create target %s requires an earlier validated migration of %s", action.Target, action.LegacyParent)
				}
				if !plan.InsideRoot(action.Target, action.LegacyParent) {
					return nil, fmt.Errorf("unsafe create target %s outside legacy parent %s", action.Target, action.LegacyParent)
				}
				if err := validateSource(action.Strategy, source, sourceRoot); err != nil {
					return nil, err
				}
			} else if err := validateCreate(action, source, sourceRoot, home); err != nil {
				return nil, err
			}
		case plan.StatusUnchanged:
			continue
		case plan.StatusUpdate:
			if opts.StateRoot == "" {
				return nil, fmt.Errorf("update for %s requires state root for Backup Set metadata", action.Target)
			}
			wholeOverride := (action.Ownership == "" || action.Ownership == "whole") && action.PreviousHash != ""
			if action.Strategy != "copy" || (!wholeOverride && action.Ownership != "json-subset" && action.Ownership != "jsonc-subset" && action.Ownership != "toml-subset" && action.Ownership != "marked-block" && action.Ownership != "seeded") {
				return nil, fmt.Errorf("update for %s requires copy strategy with reconcilable ownership", action.Target)
			}
			if err := validateBackupStateRoot(opts.StateRoot, home); err != nil {
				return nil, err
			}
			if err := validateTargetParentInsideHome(action.Target, home); err != nil {
				return nil, err
			}
			if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(action.Target, home, "update target"); err != nil {
				return nil, err
			}
			if err := plan.ValidateBackupableTarget(action.Target); err != nil {
				return nil, err
			}
			if err := validateSource(action.Strategy, source, sourceRoot); err != nil {
				return nil, err
			}
		case plan.StatusMigrate:
			if opts.StateRoot == "" {
				return nil, fmt.Errorf("migration for %s requires state root for Backup Set metadata", action.Target)
			}
			if action.Strategy != "copy" || action.Migration == nil {
				return nil, fmt.Errorf("migration for %s requires captured copy content", action.Target)
			}
			if err := validateBackupStateRoot(opts.StateRoot, home); err != nil {
				return nil, err
			}
			if err := validateTargetParentInsideHome(action.Target, home); err != nil {
				return nil, err
			}
			if action.Migration.LegacyTarget != "" {
				if err := plan.ValidateResolvedTarget(action.Migration.LegacyTarget, home); err != nil {
					return nil, err
				}
				if err := plan.ValidateTargetParentInsideHome(action.Migration.LegacyTarget, home); err != nil {
					return nil, err
				}
				if !plan.InsideRoot(action.Migration.LegacyContentTarget, action.Migration.LegacyTarget) {
					return nil, fmt.Errorf("unsafe legacy content target %s", action.Migration.LegacyContentTarget)
				}
			}
			if err := validateSource(action.Strategy, source, sourceRoot); err != nil {
				return nil, err
			}
			if err := validateMigrationTarget(action); err != nil {
				return nil, err
			}
			if action.Migration.LegacyTarget != "" {
				validatedLegacyRemovals[action.Migration.LegacyTarget] = struct{}{}
			}
		case plan.StatusConflict:
			switch conflictDecision(action, opts) {
			case DecisionSkip:
				continue
			case DecisionReplace:
				if opts.StateRoot == "" {
					return nil, fmt.Errorf("replace conflict for %s requires state root for Backup Set metadata", action.Target)
				}
				if err := validateBackupStateRoot(opts.StateRoot, home); err != nil {
					return nil, err
				}
				if !supportedStrategy(action.Strategy) {
					return nil, fmt.Errorf("install strategy %q is not supported for %s", action.Strategy, action.Target)
				}
				if err := validateTargetParentInsideHome(action.Target, home); err != nil {
					return nil, err
				}
				if err := plan.ValidateBackupableTarget(action.Target); err != nil {
					return nil, err
				}
				if err := validateSource(action.Strategy, source, sourceRoot); err != nil {
					return nil, err
				}
			case DecisionAdopt:
				if !supportedStrategy(action.Strategy) {
					return nil, fmt.Errorf("install strategy %q is not supported for %s", action.Strategy, action.Target)
				}
				if err := validateTargetParentInsideHome(action.Target, home); err != nil {
					return nil, err
				}
				if err := validateAdoptableTarget(action.Target, home); err != nil {
					return nil, err
				}
				if err := validateAdoptableSource(source, sourceRoot); err != nil {
					return nil, err
				}
				if snapshot, ok := opts.AdoptSnapshots[action.Target]; ok {
					if err := validateCapturedFile(action.Target, home, "adopt target", snapshot.target); err != nil {
						return nil, fmt.Errorf("install plan is stale: %w", err)
					}
					if err := validateCapturedFile(source, sourceRoot, "adopt source", snapshot.source); err != nil {
						return nil, fmt.Errorf("install plan is stale: %w", err)
					}
				}
				if action.Strategy == "symlink" {
					if opts.StateRoot == "" {
						return nil, fmt.Errorf("adopt symlink conflict for %s requires state root for Backup Set metadata", action.Target)
					}
					if err := validateBackupStateRoot(opts.StateRoot, home); err != nil {
						return nil, err
					}
				}
			default:
				return nil, fmt.Errorf("unsupported conflict decision %q for %s", conflictDecision(action, opts), action.Target)
			}
		case plan.StatusMissingSource:
			return nil, fmt.Errorf("install plan contains %s for %s", action.Status, action.Target)
		default:
			return nil, fmt.Errorf("install plan contains unsupported status %q for %s", action.Status, action.Target)
		}
	}
	return resolvedSources, nil
}

// validateActionSources resolves and validates an Action's ordered Source of
// Truth inputs. A non-nil expected slice also enforces the identity captured
// by an earlier validation.
func validateActionSources(action plan.Action, sourceRoot string, expected []string) ([]string, error) {
	sourceNames := []string{action.Source}
	declaredResolved := []string{action.ResolvedSource}
	if len(action.Sources) > 0 {
		sourceNames = action.Sources
		declaredResolved = action.ResolvedSources
		if action.Strategy != "copy" || action.Ownership != "json-subset" || len(sourceNames) < 2 {
			return nil, fmt.Errorf("composed target %s requires at least two copy/json-subset sources", action.Target)
		}
	}

	if expected != nil && len(sourceNames) != len(expected) {
		return nil, fmt.Errorf("terminal metadata commit for %s has %d sources, want %d", action.Target, len(sourceNames), len(expected))
	}
	if len(action.Contributions) > 0 && len(action.Contributions) != len(sourceNames) {
		if expected != nil {
			return nil, fmt.Errorf("terminal metadata commit has %d contributions for %d sources on %s", len(action.Contributions), len(sourceNames), action.Target)
		}
		return nil, fmt.Errorf("install plan has %d contributions for %d sources on %s", len(action.Contributions), len(sourceNames), action.Target)
	}
	for i, contribution := range action.Contributions {
		if contribution.Source != sourceNames[i] {
			return nil, fmt.Errorf("install plan contribution source %q does not match source %q on %s", contribution.Source, sourceNames[i], action.Target)
		}
	}

	resolvedSources := make([]string, len(sourceNames))
	for i, sourceName := range sourceNames {
		resolved, err := plan.ResolveSource(sourceName, sourceRoot)
		if err != nil {
			return nil, err
		}
		if expected != nil && resolved != expected[i] {
			return nil, fmt.Errorf("terminal source %q resolved to %q after applying from %q", sourceName, resolved, expected[i])
		}
		if i < len(declaredResolved) && declaredResolved[i] != "" && declaredResolved[i] != resolved {
			return nil, fmt.Errorf("install plan source %q resolved to %q, want %q", sourceName, declaredResolved[i], resolved)
		}
		resolvedSources[i] = resolved
	}
	return resolvedSources, nil
}

func conflictDecision(action plan.Action, opts Options) ConflictDecision {
	if action.Status != plan.StatusConflict || opts.ConflictDecisions == nil {
		return DecisionSkip
	}
	decision := opts.ConflictDecisions[action.Target]
	if decision == "" {
		return DecisionSkip
	}
	return decision
}

func recordsMetadata(action plan.Action, decision ConflictDecision) bool {
	if action.Status == plan.StatusCreate || action.Status == plan.StatusUpdate || action.Status == plan.StatusMigrate || action.Status == plan.StatusUnchanged {
		return true
	}
	return action.Status == plan.StatusConflict && (decision == DecisionReplace || decision == DecisionAdopt)
}

func recordsContributionEvidence(action plan.Action, decision ConflictDecision) bool {
	if !recordsMetadata(action, decision) {
		return false
	}
	// Preserve the baseline that originally seeded locally evolved state.
	// Replacing it with the new Source of Truth baseline would destroy the
	// evidence needed to recognize a later reset and advance safely.
	return action.Ownership != "seeded" || action.Reason != plan.ReasonSeededLocalEvolution
}

func validateStateRoot(stateRoot, home string) error {
	if stateRoot == "" {
		return nil
	}
	stateAbs, err := cleanAbs(stateRoot)
	if err != nil {
		return fmt.Errorf("resolve state root %s: %w", stateRoot, err)
	}
	homeAbs, err := cleanAbs(home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	// Explicit state roots outside home are trusted caller-controlled storage.
	// State roots inside home, including the CLI default, must not escape home
	// through symlinks because that would write default metadata outside the
	// sandbox selected by --home.
	if !plan.InsideRoot(stateAbs, homeAbs) {
		return nil
	}
	return validateStatePathInsideHome(stateAbs, homeAbs)
}

func validateStatePathInsideHome(stateAbs, homeAbs string) error {
	if err := plan.ValidatePathInsideHomeNoSymlinkEscape(stateAbs, homeAbs, "state root"); err != nil {
		return err
	}
	return plan.ValidateFilePathInsideHomeNoSymlinkEscape(state.Path(stateAbs), homeAbs, "installation metadata")
}

func validateBackupStateRoot(stateRoot, home string) error {
	stateAbs, err := cleanAbs(stateRoot)
	if err != nil {
		return fmt.Errorf("resolve state root %s: %w", stateRoot, err)
	}
	homeAbs, err := cleanAbs(home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	if !plan.InsideRoot(stateAbs, homeAbs) {
		return nil
	}
	if err := plan.ValidatePathInsideHomeNoSymlinkEscape(filepath.Join(stateAbs, "backups"), homeAbs, "Backup Set directory"); err != nil {
		return err
	}
	return plan.ValidateFilePathInsideHomeNoSymlinkEscape(backups.Path(stateAbs), homeAbs, "Backup Metadata")
}

func cleanAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func validateCreate(action plan.Action, source, sourceRoot, home string) error {
	if err := validateTargetStillAbsent(action.Target); err != nil {
		return err
	}
	if err := validateTargetParentInsideHome(action.Target, home); err != nil {
		return err
	}
	if err := validateSource(action.Strategy, source, sourceRoot); err != nil {
		return err
	}
	return nil
}

func validateTargetStillAbsent(target string) error {
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("install plan is stale: create target %s already exists", target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat target %s: %w", target, err)
	}
	return nil
}

func validateTargetParentInsideHome(target, home string) error {
	return plan.ValidateTargetParentInsideHome(target, home)
}

func validateSource(strategy, source, sourceRoot string) error {
	if err := plan.ValidateResolvedSource(source, sourceRoot); err != nil {
		return err
	}

	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat source %s: %w", source, err)
	}
	if strategy == "copy" && !info.Mode().IsRegular() {
		return fmt.Errorf("source %s is not a regular file", source)
	}
	return nil
}

func supportedStrategy(strategy string) bool {
	switch strategy {
	case "symlink", "copy":
		return true
	default:
		return false
	}
}

func applyCreate(action plan.Action, source string, opts Options) error {
	if err := createConfinedParent(opts.Home, action.Target); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", action.Target, err)
	}

	switch action.Strategy {
	case "symlink":
		if err := createCapturedSymlink(action, source, opts); err != nil {
			return fmt.Errorf("create symlink %s: %w", action.Target, err)
		}
		return nil
	case "copy":
		firstSource := actionSourceList(action)[0]
		if captured, ok := opts.CapturedSources[SourceCaptureKey{Target: action.Target, Source: firstSource}]; ok && captured.ContentPresent {
			if err := writeNewFileWithMode(opts.Home, action.Target, action.Content, captured.Mode.Perm()); err != nil {
				return fmt.Errorf("write captured source to %s: %w", action.Target, err)
			}
			return nil
		}
		if action.Content != nil {
			if err := writeNewFileFromSourceMode(source, action.Target, action.Content); err != nil {
				return fmt.Errorf("write composed JSON to %s: %w", action.Target, err)
			}
			return nil
		}
		if err := copyRegularFile(source, action.Target); err != nil {
			return fmt.Errorf("copy %s to %s: %w", source, action.Target, err)
		}
		return nil
	default:
		return fmt.Errorf("install strategy %q is not supported for %s", action.Strategy, action.Target)
	}
}

func createCapturedSymlink(action plan.Action, source string, opts Options) error {
	return createCapturedSymlinkWithHook(action, source, opts, nil)
}

func createCapturedSymlinkWithHook(action plan.Action, source string, opts Options, afterCreate func() error) (resultErr error) {
	sourceName := actionSourceList(action)[0]
	captured, hasCapture := opts.CapturedSources[SourceCaptureKey{Target: action.Target, Source: sourceName}]
	if hasCapture {
		if _, err := prepareCapturedAction(action, []string{source}, opts); err != nil {
			return err
		}
	}
	homeAbs, err := cleanAbs(opts.Home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	targetAbs, err := cleanAbs(action.Target)
	if err != nil {
		return fmt.Errorf("resolve target %s: %w", action.Target, err)
	}
	relative, err := filepath.Rel(homeAbs, targetAbs)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return fmt.Errorf("confine target %s beneath home %s", action.Target, homeAbs)
	}
	root, err := os.OpenRoot(homeAbs)
	if err != nil {
		return fmt.Errorf("open home root %s: %w", homeAbs, err)
	}
	actionCompleted := false
	defer func() {
		if closeErr := closeRootOnce(root, homeAbs); closeErr != nil {
			if actionCompleted {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	var creationCleanup error
	if err := symlinkAtRoot(root, relative, source); err != nil {
		if !isCompletedActionCleanup(err) {
			return err
		}
		creationCleanup = withoutCompletedActionMarker(err)
	}
	created, err := root.Lstat(relative)
	if err != nil {
		return errors.Join(fmt.Errorf("inspect created symlink %s: %w", action.Target, err), creationCleanup)
	}
	if afterCreate != nil {
		if err := afterCreate(); err != nil {
			return errors.Join(fmt.Errorf("after symlink creation for %s: %w", action.Target, err), creationCleanup, removeCreatedSymlinkIfMatching(root, relative, source, created))
		}
	}
	if !hasCapture {
		actionCompleted = true
		return completedActionCleanup(creationCleanup)
	}
	resolved, resolveErr := statSymlinkTargetAtRoot(root, relative)
	capturedInfo, capturedInfoErr := captured.identity.stat()
	if (resolveErr == nil || isCleanupFailure(resolveErr)) && capturedInfoErr == nil && resolved != nil && os.SameFile(resolved, capturedInfo) {
		// After terminal commit, a managed symlink intentionally follows later
		// Source of Truth checkout changes. This identity check only closes the
		// selector review-to-creation window.
		actionCompleted = true
		return completedActionCleanup(errors.Join(creationCleanup, resolveErr))
	}
	cleanupErr := removeCreatedSymlinkIfMatching(root, relative, source, created)
	if capturedInfoErr != nil {
		return errors.Join(fmt.Errorf("resolve captured source identity for %s: %w", action.Target, capturedInfoErr), creationCleanup, resolveErr, cleanupErr)
	}
	if resolveErr != nil {
		return errors.Join(fmt.Errorf("resolve created symlink %s: %w", action.Target, resolveErr), creationCleanup, cleanupErr)
	}
	return errors.Join(fmt.Errorf("created symlink %s resolved to a different source identity", action.Target), creationCleanup, cleanupErr)
}

func createConfinedParent(rootPath, target string) (resultErr error) {
	root, relative, err := openConfinedRoot(rootPath, target)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeRootOnce(root, rootPath)) }()
	parent := filepath.Dir(relative)
	if parent == "." {
		return nil
	}
	current := ""
	for _, component := range strings.Split(parent, string(os.PathSeparator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		if err := root.Mkdir(current, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create confined parent %s for %s: %w", current, target, err)
		}
		info, err := root.Stat(current)
		if err != nil {
			return fmt.Errorf("stat confined parent %s for %s: %w", current, target, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("parent %s is not a directory", current)
		}
	}
	return nil
}

func symlinkAtRoot(root *os.Root, relative, destination string) (resultErr error) {
	parent, base, err := openRootParent(root, relative)
	if err != nil {
		return fmt.Errorf("open symlink parent: %w", err)
	}
	created := false
	defer func() {
		if closeErr := closeFileOnce(parent, "parent of "+relative); closeErr != nil {
			if created {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	if err := unix.Symlinkat(destination, int(parent.Fd()), base); err != nil {
		return fmt.Errorf("create confined symlink %s to %s: %w", relative, destination, err)
	}
	created = true
	return nil
}

func openRootParent(root *os.Root, relative string) (*os.File, string, error) {
	parent, err := root.OpenFile(filepath.Dir(relative), os.O_RDONLY, 0)
	if err != nil {
		return nil, "", err
	}
	return parent, filepath.Base(relative), nil
}

func statSymlinkTargetAtRoot(root *os.Root, relative string) (result os.FileInfo, resultErr error) {
	parent, base, err := openRootParent(root, relative)
	if err != nil {
		return nil, fmt.Errorf("open parent of resolved symlink %s: %w", relative, err)
	}
	defer func() { resultErr = errors.Join(resultErr, closeFileOnce(parent, "parent of "+relative)) }()
	fd, err := unix.Openat(int(parent.Fd()), base, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open resolved symlink %s: %w", relative, err)
	}
	resolved := newFileOperation(uintptr(fd), relative)
	if resolved == nil {
		return nil, errors.Join(fmt.Errorf("open resolved symlink descriptor for %s", relative), closeFDOnce(fd, relative))
	}
	defer func() { resultErr = errors.Join(resultErr, closeFileOnce(resolved, relative)) }()
	result, err = resolved.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat resolved symlink %s: %w", relative, err)
	}
	return result, nil
}

func readlinkAtRoot(root *os.Root, relative string) (result string, resultErr error) {
	parent, base, err := openRootParent(root, relative)
	if err != nil {
		return "", fmt.Errorf("open parent of confined symlink %s: %w", relative, err)
	}
	defer func() { resultErr = errors.Join(resultErr, closeFileOnce(parent, "parent of "+relative)) }()
	for size := 256; size <= 1<<20; size *= 2 {
		buffer := make([]byte, size)
		n, err := unix.Readlinkat(int(parent.Fd()), base, buffer)
		if err != nil {
			return "", fmt.Errorf("read confined symlink %s: %w", relative, err)
		}
		if n < len(buffer) {
			return string(buffer[:n]), nil
		}
	}
	return "", fmt.Errorf("read confined symlink %s: destination exceeds supported length", relative)
}

func removeCreatedSymlinkIfMatching(root *os.Root, relative, destination string, created os.FileInfo) error {
	current, err := root.Lstat(relative)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect created symlink for compensation: %w", err)
	}
	if current.Mode()&os.ModeSymlink == 0 || !os.SameFile(current, created) {
		return fmt.Errorf("created symlink changed before compensation; refusing removal")
	}
	currentDestination, readErr := readlinkAtRoot(root, relative)
	if readErr != nil && !isCleanupFailure(readErr) {
		return fmt.Errorf("read created symlink %s for compensation: %w", relative, readErr)
	}
	if currentDestination != destination {
		return errors.Join(fmt.Errorf("created symlink %s destination changed before compensation; refusing removal", relative), readErr)
	}
	if err := root.Remove(relative); err != nil {
		return errors.Join(fmt.Errorf("remove created symlink %s during compensation: %w", relative, err), readErr)
	}
	return readErr
}

func applyReplace(action plan.Action, source string, opts Options) error {
	if err := createBackupSet(opts, action.Target); err != nil {
		return err
	}
	if err := removeConflictingTarget(action.Target); err != nil {
		return fmt.Errorf("remove conflicting target %s: %w", action.Target, err)
	}
	if err := applyCreate(action, source, opts); err != nil {
		return err
	}
	return nil
}

func applyUpdate(action plan.Action, source string, opts Options) (managedActionResult, error) {
	if err := createBackupSet(opts, action.Target); err != nil {
		return managedActionResult{}, err
	}
	switch action.Ownership {
	case "json-subset":
		current := action.Content
		if current == nil {
			var err error
			current, err = os.ReadFile(source)
			if err != nil {
				return managedActionResult{}, fmt.Errorf("read current owned JSON %s: %w", source, err)
			}
		}
		if len(action.PreviousContent) > 0 {
			result, err := reconcileJSONContentFile(action.Target, action.PreviousContent, current, opts.Home)
			if err != nil {
				return result, fmt.Errorf("reconcile JSON update for %s: %w", action.Target, err)
			}
			return result, nil
		}
		result, err := mergeJSONContentFile(action.Target, current, opts.Home)
		if err != nil {
			return result, fmt.Errorf("merge JSON update for %s: %w", action.Target, err)
		}
		return result, nil
	case "jsonc-subset":
		current := action.Content
		if current == nil {
			var err error
			current, err = os.ReadFile(source)
			if err != nil {
				return managedActionResult{}, fmt.Errorf("read current owned JSONC %s: %w", source, err)
			}
		}
		if len(action.PreviousContent) > 0 {
			result, err := reconcileJSONCContentFile(action.Target, action.PreviousContent, current, opts.Home)
			if err != nil {
				return result, fmt.Errorf("reconcile JSONC update for %s: %w", action.Target, err)
			}
			return result, nil
		}
		result, err := mergeJSONCContentFile(action.Target, current, opts.Home)
		if err != nil {
			return result, fmt.Errorf("merge JSONC update for %s: %w", action.Target, err)
		}
		return result, nil
	case "toml-subset":
		current := action.Content
		if current == nil {
			var err error
			current, err = os.ReadFile(source)
			if err != nil {
				return managedActionResult{}, fmt.Errorf("read current owned TOML %s: %w", source, err)
			}
		}
		if len(action.PreviousContent) > 0 {
			result, err := reconcileTOMLContentFile(action.Target, action.PreviousContent, current, opts.Home)
			if err != nil {
				return result, fmt.Errorf("reconcile TOML update for %s: %w", action.Target, err)
			}
			return result, nil
		}
		result, err := mergeTOMLContentFile(action.Target, current, opts.Home)
		if err != nil {
			return result, fmt.Errorf("merge TOML update for %s: %w", action.Target, err)
		}
		return result, nil
	case "seeded":
		current := action.Content
		if current == nil {
			var err error
			current, err = os.ReadFile(source)
			if err != nil {
				return managedActionResult{}, fmt.Errorf("read seeded baseline %s: %w", source, err)
			}
		}
		result, err := updateConfinedRegularFile(action.Target, opts.Home, func(live []byte) ([]byte, bool, error) {
			if !bytes.Equal(live, action.PreviousContent) {
				return nil, false, fmt.Errorf("install plan is stale: seeded target %s evolved before baseline advancement", action.Target)
			}
			return current, !bytes.Equal(live, current), nil
		})
		if err != nil {
			return result, fmt.Errorf("advance seeded target %s: %w", action.Target, err)
		}
		return result, nil
	case "marked-block":
		current := action.Content
		if current == nil {
			var err error
			current, err = os.ReadFile(source)
			if err != nil {
				return managedActionResult{}, fmt.Errorf("read marked-block source %s: %w", source, err)
			}
		}
		result, err := updateConfinedRegularFile(action.Target, opts.Home, func(live []byte) ([]byte, bool, error) {
			reconciliation := textblock.ReconcileOwned(live, action.PreviousContent, current, textblock.DotsManagedMarkers())
			if !reconciliation.Compatible {
				return nil, false, fmt.Errorf("install plan is stale: marked block %s changed before update", action.Target)
			}
			return reconciliation.Content, !bytes.Equal(reconciliation.Content, live), nil
		})
		if err != nil {
			return result, fmt.Errorf("update marked block %s: %w", action.Target, err)
		}
		return result, nil
	case "", "whole":
		result, err := updateWholeTargetWithResult(action, source, opts.Home)
		if err != nil {
			return managedActionResult{}, err
		}
		return result, nil
	default:
		return managedActionResult{}, fmt.Errorf("update ownership %q is not supported for %s", action.Ownership, action.Target)
	}
}

func updateWholeTarget(action plan.Action, source, home string) error {
	_, err := updateWholeTargetWithResult(action, source, home)
	return err
}

func updateWholeTargetWithResult(action plan.Action, source, home string) (managedActionResult, error) {
	if action.PreviousHash == "" {
		return managedActionResult{}, fmt.Errorf("previous whole-target evidence is required for %s", action.Target)
	}
	current := action.Content
	if current == nil {
		var err error
		current, err = os.ReadFile(source)
		if err != nil {
			return managedActionResult{}, fmt.Errorf("read current whole target source %s: %w", source, err)
		}
	}
	return updateConfinedRegularFile(action.Target, home, func(live []byte) ([]byte, bool, error) {
		if state.HashBytes(live) != action.PreviousHash {
			return nil, false, fmt.Errorf("install plan is stale: whole target %s changed before update", action.Target)
		}
		return current, !bytes.Equal(live, current), nil
	})
}

type confinedTransform func([]byte) ([]byte, bool, error)

func updateConfinedRegularFile(target, home string, transform confinedTransform) (result managedActionResult, resultErr error) {
	homeAbs, err := cleanAbs(home)
	if err != nil {
		return managedActionResult{}, fmt.Errorf("resolve home for target %s: %w", target, err)
	}
	targetAbs, err := cleanAbs(target)
	if err != nil {
		return managedActionResult{}, fmt.Errorf("resolve target %s: %w", target, err)
	}
	relative, err := filepath.Rel(homeAbs, targetAbs)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return managedActionResult{}, fmt.Errorf("confine target %s beneath home %s", target, homeAbs)
	}
	root, err := os.OpenRoot(homeAbs)
	if err != nil {
		return managedActionResult{}, fmt.Errorf("open home root %s: %w", homeAbs, err)
	}
	actionCompleted := false
	defer func() {
		if closeErr := closeRootOnce(root, homeAbs); closeErr != nil {
			if actionCompleted {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	observed, err := root.Lstat(relative)
	if err != nil {
		return managedActionResult{}, fmt.Errorf("inspect confined target %s: %w", target, err)
	}
	if observed.Mode()&os.ModeSymlink != 0 || !observed.Mode().IsRegular() {
		return managedActionResult{}, fmt.Errorf("confined target %s is not a non-symlink regular file", target)
	}
	file, err := root.OpenFile(relative, os.O_RDWR, 0)
	if err != nil {
		return managedActionResult{}, fmt.Errorf("open confined target %s: %w", target, err)
	}
	defer func() {
		if closeErr := closeFileOnce(file, target); closeErr != nil {
			if actionCompleted {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return managedActionResult{}, fmt.Errorf("stat confined target %s: %w", target, err)
	}
	if !info.Mode().IsRegular() || !os.SameFile(observed, info) {
		return managedActionResult{}, fmt.Errorf("install plan is stale: target %s changed identity before update", target)
	}
	live, err := io.ReadAll(file)
	if err != nil {
		return managedActionResult{}, fmt.Errorf("read confined target %s: %w", target, err)
	}
	updated, changed, err := transform(live)
	if err != nil {
		return managedActionResult{}, err
	}
	result = managedActionResult{
		PreviousTargetContent: append([]byte(nil), live...),
		TargetContent:         append([]byte(nil), updated...),
		ExactTargetContent:    true,
	}
	if !changed {
		result.TargetContent = append([]byte(nil), live...)
		actionCompleted = true
		return result, nil
	}
	if err := rewriteOpenedRegularFile(file, target, updated, live); err != nil {
		return managedActionResult{}, err
	}
	actionCompleted = true
	return result, nil
}

type openedRegularFile interface {
	io.Writer
	io.Seeker
	Truncate(int64) error
	Sync() error
}

func rewriteOpenedRegularFile(file openedRegularFile, target string, updated, previous []byte) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind confined target %s: %w", target, err)
	}
	if err := file.Truncate(0); err != nil {
		return errors.Join(
			fmt.Errorf("truncate confined target %s: %w", target, err),
			restoreOpenedRegularFile(file, target, previous),
		)
	}
	if err := writeAll(file, updated); err != nil {
		return errors.Join(
			fmt.Errorf("write confined target %s: %w", target, err),
			restoreOpenedRegularFile(file, target, previous),
		)
	}
	if err := file.Sync(); err != nil {
		return errors.Join(
			fmt.Errorf("sync confined target %s: %w", target, err),
			restoreOpenedRegularFile(file, target, previous),
		)
	}
	return nil
}

func restoreOpenedRegularFile(file openedRegularFile, target string, previous []byte) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("restore confined target %s: rewind: %w", target, err)
	}
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("restore confined target %s: truncate: %w", target, err)
	}
	if err := writeAll(file, previous); err != nil {
		return fmt.Errorf("restore confined target %s: write: %w", target, err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("restore confined target %s: sync: %w", target, err)
	}
	return nil
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func restoreConfinedRegularFile(target, home string, applied, previous []byte) error {
	_, err := updateConfinedRegularFile(target, home, func(live []byte) ([]byte, bool, error) {
		if !bytes.Equal(live, applied) {
			return nil, false, fmt.Errorf("target changed after reconciliation; refusing rollback")
		}
		return previous, !bytes.Equal(live, previous), nil
	})
	if err != nil {
		return fmt.Errorf("restore target %s after metadata failure: %w", target, err)
	}
	return nil
}

func validateMigrationTarget(action plan.Action) error {
	target := action.Target
	contentTarget := action.Target
	if action.Migration.LegacyTarget != "" {
		target = action.Migration.LegacyTarget
		contentTarget = action.Migration.LegacyContentTarget
	}
	info, err := os.Lstat(target)
	if err != nil {
		return fmt.Errorf("install plan is stale: migration target %s changed: %w", target, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("install plan is stale: migration target %s is no longer a symlink", target)
	}
	destination, err := os.Readlink(target)
	if err != nil {
		return fmt.Errorf("read migration target %s: %w", target, err)
	}
	if filepath.Clean(destination) != filepath.Clean(action.Migration.LinkDestination) {
		return fmt.Errorf("install plan is stale: migration target %s changed destination", target)
	}
	content, err := os.ReadFile(contentTarget)
	if err != nil {
		return fmt.Errorf("install plan is stale: read migration target %s: %w", contentTarget, err)
	}
	if !bytes.Equal(content, action.Migration.ExpectedLinkContent) {
		return fmt.Errorf("install plan is stale: migration target %s content changed", contentTarget)
	}
	return nil
}

func applyMigration(action plan.Action, source string, opts Options) error {
	if err := validateMigrationTarget(action); err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat migration source %s: %w", source, err)
	}
	backupTarget := action.Target
	removeTarget := action.Target
	if action.Migration.LegacyTarget != "" {
		backupTarget = action.Migration.LegacyContentTarget
		removeTarget = action.Migration.LegacyTarget
	}
	if _, err := backups.CreateContentSet(opts.StateRoot, backupTarget, action.Migration.CapturedContent, info.Mode(), backups.CreateOptions{
		Reason: "pre-install legacy target migration", Machine: backups.MachineName(), Repo: opts.SourceRoot,
	}); err != nil {
		return err
	}
	if err := os.Remove(removeTarget); err != nil {
		return fmt.Errorf("remove legacy symlink %s: %w", removeTarget, err)
	}
	if err := os.MkdirAll(filepath.Dir(action.Target), 0o755); err != nil {
		return fmt.Errorf("create migrated target parent %s: %w", filepath.Dir(action.Target), err)
	}
	if err := writeNewFileFromSourceMode(source, action.Target, action.Migration.FinalContent); err != nil {
		return fmt.Errorf("materialize migrated target %s: %w", action.Target, err)
	}
	return nil
}

func removeConflictingTarget(target string) error {
	info, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return backups.RemoveDirectoryTree(target)
	}
	return os.Remove(target)
}

func applyAdopt(action plan.Action, source string, opts Options) error {
	if snapshot, ok := opts.AdoptSnapshots[action.Target]; ok {
		return applyCapturedAdopt(action, source, opts, snapshot)
	}
	if err := copyAdoptedTargetToSource(action.Target, source); err != nil {
		return err
	}
	if action.Strategy != "symlink" {
		return nil
	}
	if err := createBackupSet(opts, action.Target); err != nil {
		return err
	}
	if err := os.Remove(action.Target); err != nil {
		return fmt.Errorf("remove adopted target %s: %w", action.Target, err)
	}
	if err := applyCreate(action, source, opts); err != nil {
		return err
	}
	return nil
}

func applyCapturedAdopt(action plan.Action, source string, opts Options, snapshot AdoptSnapshot) (resultErr error) {
	actionCompleted := false
	if err := validateCapturedFile(action.Target, opts.Home, "adopt target", snapshot.target); err != nil {
		return fmt.Errorf("install plan is stale: %w", err)
	}
	if err := validateCapturedFile(source, opts.SourceRoot, "adopt source", snapshot.source); err != nil {
		return fmt.Errorf("install plan is stale: %w", err)
	}

	sourceRoot, sourceRelative, err := openConfinedRoot(opts.SourceRoot, source)
	if err != nil {
		return fmt.Errorf("open adopt source root: %w", err)
	}
	defer func() {
		if closeErr := closeRootOnce(sourceRoot, opts.SourceRoot); closeErr != nil {
			if actionCompleted {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	sourceFile, err := sourceRoot.OpenFile(sourceRelative, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open adopt source %s: %w", source, err)
	}
	defer func() {
		if closeErr := closeFileOnce(sourceFile, source); closeErr != nil {
			if actionCompleted {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	sourceInfo, sourceData, err := readCapturedDescriptor(sourceFile, "adopt source", snapshot.source)
	if err != nil {
		return err
	}

	homeRoot, targetRelative, err := openConfinedRoot(opts.Home, action.Target)
	if err != nil {
		return fmt.Errorf("open adopt target root: %w", err)
	}
	defer func() {
		if closeErr := closeRootOnce(homeRoot, opts.Home); closeErr != nil {
			if actionCompleted {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	targetFile, err := homeRoot.OpenFile(targetRelative, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open adopt target %s: %w", action.Target, err)
	}
	_, targetData, targetReadErr := readCapturedDescriptor(targetFile, "adopt target", snapshot.target)
	targetCloseErr := closeFileOnce(targetFile, action.Target)
	if err := errors.Join(targetReadErr, targetCloseErr); err != nil {
		return fmt.Errorf("read captured adopt target %s: %w", action.Target, err)
	}

	if err := rewriteOpenedRegularFile(sourceFile, source, targetData, sourceData); err != nil {
		return fmt.Errorf("write captured adopt source %s: %w", source, err)
	}
	if action.Strategy != "symlink" {
		actionCompleted = true
		return nil
	}
	rollbackSource := func() error {
		if _, err := sourceFile.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("seek adopt source for rollback: %w", err)
		}
		current, err := io.ReadAll(sourceFile)
		if err != nil {
			return fmt.Errorf("read adopt source for rollback: %w", err)
		}
		if !bytes.Equal(current, targetData) {
			return fmt.Errorf("adopt source changed after write; refusing rollback")
		}
		return rewriteOpenedRegularFile(sourceFile, source, sourceData, targetData)
	}
	if _, err := backups.CreateContentSet(opts.StateRoot, action.Target, targetData, snapshot.target.Mode, backups.CreateOptions{
		Reason: "pre-install conflict protection", Machine: backups.MachineName(), Repo: opts.SourceRoot,
	}); err != nil {
		return errors.Join(err, rollbackSource())
	}
	if err := validateCapturedFile(action.Target, opts.Home, "adopt target", snapshot.target); err != nil {
		return errors.Join(fmt.Errorf("install plan is stale: %w", err), rollbackSource())
	}
	if err := homeRoot.Remove(targetRelative); err != nil {
		return errors.Join(fmt.Errorf("remove adopted target %s: %w", action.Target, err), rollbackSource())
	}

	adoptedSource := CapturedSource{
		Content: append([]byte{}, targetData...), ContentPresent: true,
		Mode: sourceInfo.Mode(), ModePresent: true,
		IdentityFingerprint: snapshot.source.IdentityFingerprint, IdentityPresent: true, identity: snapshot.source.identity,
	}
	createOpts := opts
	createOpts.CapturedSources = map[SourceCaptureKey]CapturedSource{
		{Target: action.Target, Source: action.Source}: adoptedSource,
	}
	createErr := applyCreate(action, source, createOpts)
	if createErr != nil && !isCompletedActionCleanup(createErr) {
		restoreErr := restoreCapturedTargetIfAbsent(homeRoot, targetRelative, snapshot.target)
		return errors.Join(createErr, restoreErr, rollbackSource())
	}
	actionCompleted = true
	return completedActionCleanup(createErr)
}

func openConfinedRoot(rootPath, path string) (*os.Root, string, error) {
	rootAbs, err := cleanAbs(rootPath)
	if err != nil {
		return nil, "", err
	}
	pathAbs, err := cleanAbs(path)
	if err != nil {
		return nil, "", err
	}
	relative, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil || !filepath.IsLocal(relative) || relative == "." {
		return nil, "", fmt.Errorf("confine %s beneath %s", path, rootAbs)
	}
	root, err := os.OpenRoot(rootAbs)
	if err != nil {
		return nil, "", err
	}
	return root, relative, nil
}

func readCapturedDescriptor(file *os.File, label string, expected CapturedSource) (os.FileInfo, []byte, error) {
	info, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("stat %s descriptor: %w", label, err)
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s descriptor is not a regular file", label)
	}
	if expected.IdentityPresent {
		expectedInfo, err := expected.identity.stat()
		if err != nil {
			return nil, nil, fmt.Errorf("install plan is stale: %s identity authority: %w", label, err)
		}
		if !os.SameFile(info, expectedInfo) {
			return nil, nil, fmt.Errorf("install plan is stale: %s changed identity", label)
		}
	}
	if expected.ModePresent && info.Mode().Perm() != expected.Mode.Perm() {
		return nil, nil, fmt.Errorf("install plan is stale: %s changed mode", label)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("seek %s descriptor: %w", label, err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s descriptor: %w", label, err)
	}
	if expected.ContentPresent && !bytes.Equal(data, expected.Content) {
		return nil, nil, fmt.Errorf("install plan is stale: %s changed content", label)
	}
	return info, data, nil
}

func restoreCapturedTargetIfAbsent(root *os.Root, relative string, snapshot CapturedSource) error {
	file, err := root.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, snapshot.Mode.Perm())
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("adopt target was replaced after removal; refusing compensation")
		}
		return fmt.Errorf("restore adopted target: %w", err)
	}
	writeErr := writeAllFileOperation(file, snapshot.Content)
	chmodErr := file.Chmod(snapshot.Mode.Perm())
	closeErr := closeFileOnce(file, relative)
	if writeErr != nil {
		writeErr = fmt.Errorf("restore adopted target %s: write: %w", relative, writeErr)
	}
	if chmodErr != nil {
		chmodErr = fmt.Errorf("restore adopted target %s: chmod: %w", relative, chmodErr)
	}
	return errors.Join(writeErr, chmodErr, closeErr)
}

func validateAdoptableTarget(target, home string) error {
	if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(target, home, "adopt target"); err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("stat adopt target %s: %w", target, err)
	}
	if info.IsDir() {
		return fmt.Errorf("adopting directory target %s is not supported; use replace to back it up and install the managed symlink", target)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("adopt target %s is not a regular file", target)
	}
	return nil
}

func validateAdoptableSource(source, sourceRoot string) error {
	if err := plan.ValidateResolvedSource(source, sourceRoot); err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat adopt source %s: %w", source, err)
	}
	if info.IsDir() {
		return fmt.Errorf("adopting directory source %s is not supported; use replace to back up the target and install the managed symlink", source)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("adopt source %s is not a regular file", source)
	}
	return nil
}

func copyAdoptedTargetToSource(target, source string) error {
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read adopt target %s: %w", target, err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat adopt source %s: %w", source, err)
	}
	if err := os.WriteFile(source, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write adopted source %s: %w", source, err)
	}
	return nil
}

func createBackupSet(opts Options, target string) error {
	_, err := backups.CreateSet(opts.StateRoot, []string{target}, backups.CreateOptions{
		Reason:  "pre-install conflict protection",
		Machine: backups.MachineName(),
		Repo:    opts.SourceRoot,
	})
	return err
}

func copyRegularFile(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat copy source %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read copy source %s: %w", source, err)
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("open copy target %s: %w", target, err)
	}
	if _, err := writeFileOperation(file, data); err != nil {
		return errors.Join(fmt.Errorf("write copy target %s: %w", target, err), closeFileOnce(file, target), removePathForCompensation(target))
	}
	if err := closeFileOnce(file, target); err != nil {
		return errors.Join(err, removePathForCompensation(target))
	}
	if err := os.Chmod(target, info.Mode().Perm()); err != nil {
		return fmt.Errorf("chmod copy target %s: %w", target, err)
	}
	return nil
}

func writeNewFileFromSourceMode(source, target string, data []byte) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat source mode %s: %w", source, err)
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("open new target %s: %w", target, err)
	}
	if _, err := writeFileOperation(file, data); err != nil {
		return errors.Join(fmt.Errorf("write new target %s: %w", target, err), closeFileOnce(file, target), removePathForCompensation(target))
	}
	if err := closeFileOnce(file, target); err != nil {
		return errors.Join(err, removePathForCompensation(target))
	}
	return nil
}

func removePathForCompensation(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove target %s during compensation: %w", path, err)
	}
	return nil
}

func writeNewFileWithMode(rootPath, target string, data []byte, mode os.FileMode) error {
	return writeNewFileWithModeAtRoot(rootPath, target, data, mode, nil)
}

func writeNewFileWithModeAtRoot(rootPath, target string, data []byte, mode os.FileMode, beforeOpen func() error) (resultErr error) {
	root, relative, err := openConfinedRoot(rootPath, target)
	if err != nil {
		return err
	}
	actionCompleted := false
	defer func() {
		if closeErr := closeRootOnce(root, rootPath); closeErr != nil {
			if actionCompleted {
				closeErr = completedActionCleanup(closeErr)
			}
			resultErr = errors.Join(resultErr, closeErr)
		}
	}()
	if beforeOpen != nil {
		if err := beforeOpen(); err != nil {
			return fmt.Errorf("before confined target create: %w", err)
		}
	}
	file, err := root.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return fmt.Errorf("open confined target %s: %w", target, err)
	}
	created, err := file.Stat()
	if err != nil {
		return errors.Join(fmt.Errorf("stat created target %s: %w", target, err), closeFileOnce(file, target))
	}
	if err := writeAll(file, data); err != nil {
		return errors.Join(fmt.Errorf("write confined target %s: %w", target, err), closeFileOnce(file, target), removeCreatedFileIfMatching(root, relative, created))
	}
	if err := file.Chmod(mode.Perm()); err != nil {
		return errors.Join(fmt.Errorf("chmod confined target %s: %w", target, err), closeFileOnce(file, target), removeCreatedFileIfMatching(root, relative, created))
	}
	if err := closeFileOnce(file, target); err != nil {
		return errors.Join(err, removeCreatedFileIfMatching(root, relative, created))
	}
	actionCompleted = true
	return nil
}

func removeCreatedFileIfMatching(root *os.Root, relative string, created os.FileInfo) error {
	current, err := root.Lstat(relative)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect created target for compensation: %w", err)
	}
	if !current.Mode().IsRegular() || !os.SameFile(current, created) {
		return fmt.Errorf("created target changed before compensation; refusing removal")
	}
	if err := root.Remove(relative); err != nil {
		return fmt.Errorf("remove created target during compensation: %w", err)
	}
	return nil
}

func mergeJSONContentFile(target string, sourceData []byte, home string) (managedActionResult, error) {
	return updateConfinedRegularFile(target, home, func(targetData []byte) ([]byte, bool, error) {
		merged, err := configsubset.MergeJSON(targetData, sourceData)
		return merged, err == nil && !bytes.Equal(merged, targetData), err
	})
}

func reconcileJSONContentFile(target string, previousData, currentData []byte, home string) (managedActionResult, error) {
	return updateConfinedRegularFile(target, home, func(targetData []byte) ([]byte, bool, error) {
		reconciliation, err := configsubset.ReconcileJSON(targetData, previousData, currentData)
		if err != nil {
			return nil, false, err
		}
		if !reconciliation.Compatible {
			return nil, false, fmt.Errorf("live target changed a previously owned JSON value")
		}
		return reconciliation.Content, reconciliation.Changed, nil
	})
}

func mergeJSONCContentFile(target string, sourceData []byte, home string) (managedActionResult, error) {
	return updateConfinedRegularFile(target, home, func(targetData []byte) ([]byte, bool, error) {
		merged, err := configsubset.MergeJSONC(targetData, sourceData)
		return merged, err == nil && !bytes.Equal(merged, targetData), err
	})
}

func reconcileJSONCContentFile(target string, previousData, currentData []byte, home string) (managedActionResult, error) {
	return updateConfinedRegularFile(target, home, func(targetData []byte) ([]byte, bool, error) {
		reconciliation, err := configsubset.ReconcileJSONC(targetData, previousData, currentData)
		if err != nil {
			return nil, false, err
		}
		if !reconciliation.Compatible {
			return nil, false, fmt.Errorf("live target changed a previously owned JSONC value")
		}
		return reconciliation.Content, reconciliation.Changed, nil
	})
}

func mergeTOMLContentFile(target string, sourceData []byte, home string) (managedActionResult, error) {
	return updateConfinedRegularFile(target, home, func(targetData []byte) ([]byte, bool, error) {
		merged, err := configsubset.MergeTOML(targetData, sourceData)
		return merged, err == nil && !bytes.Equal(merged, targetData), err
	})
}

func reconcileTOMLContentFile(target string, previousData, currentData []byte, home string) (managedActionResult, error) {
	return updateConfinedRegularFile(target, home, func(targetData []byte) ([]byte, bool, error) {
		reconciliation, err := configsubset.ReconcileTOML(targetData, previousData, currentData)
		if err != nil {
			return nil, false, err
		}
		if !reconciliation.Compatible {
			return nil, false, fmt.Errorf("live target changed a previously owned TOML value")
		}
		return reconciliation.Content, reconciliation.Changed, nil
	})
}
