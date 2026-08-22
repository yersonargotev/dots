package install

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/ownershipevidence"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/textblock"
	"github.com/yersonargotev/dots/internal/version"
)

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
		source := resolvedSources[i][0]
		prepared, receipt, err := applyActionWithReconciliationReceipt(action, source, resolvedSources[i], opts, applyAction)
		if err != nil {
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

func applyManagedAction(action plan.Action, source string, opts Options) (managedActionResult, error) {
	switch action.Status {
	case plan.StatusUnchanged:
		return managedActionResult{}, nil
	case plan.StatusCreate:
		return managedActionResult{}, applyCreate(action, source)
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
	defer func() { resultErr = errors.Join(resultErr, locked.Close()) }()
	meta, err := locked.Load()
	if err != nil {
		return prepared, nil, err
	}
	if err := validateAuthorizingRecord(meta, action, action.PreviousReconciliationReceipt); err != nil {
		return prepared, nil, err
	}

	prepared, sourceContents, err := snapshotReconciliationSources(action, resolvedSources)
	if err != nil {
		return action, nil, err
	}
	result, err := applyAction(prepared, source, opts)
	if err != nil {
		return prepared, nil, err
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
		rollbackErr := restoreConfinedRegularFile(action.Target, opts.Home, result.PreviousTargetContent)
		return prepared, nil, errors.Join(fmt.Errorf("persist reconciliation receipt for %s: %w", action.Target, err), rollbackErr)
	}
	return prepared, receipt, nil
}

func requiresReconciliationReceipt(action plan.Action, opts Options) bool {
	return opts.StateRoot != "" && action.Status == plan.StatusUpdate && action.Strategy == "copy" &&
		action.PreviousRecordFingerprint != "" && (len(action.PreviousContent) > 0 || action.PreviousHash != "")
}

func snapshotReconciliationSources(action plan.Action, resolvedSources []string) (plan.Action, [][]byte, error) {
	contents := make([][]byte, len(resolvedSources))
	for i, source := range resolvedSources {
		data, err := os.ReadFile(source)
		if err != nil {
			return action, nil, fmt.Errorf("snapshot reconciliation source %s: %w", source, err)
		}
		contents[i] = data
	}
	prepared := action
	if len(action.Sources) == 0 {
		prepared.Content = append([]byte(nil), contents[0]...)
		return prepared, contents, nil
	}
	composed := append([]byte(nil), contents[0]...)
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
	if c.opts.StateRoot == "" {
		return nil
	}
	path := state.Path(c.opts.StateRoot)
	return state.Update(path, func(meta *state.Metadata) error {
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
		if installed != nil {
			selection := *installed
			meta.InstalledSelection = &selection
		}
		return nil
	})
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

func applyCreate(action plan.Action, source string) error {
	if err := os.MkdirAll(filepath.Dir(action.Target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", action.Target, err)
	}

	switch action.Strategy {
	case "symlink":
		if err := os.Symlink(source, action.Target); err != nil {
			return fmt.Errorf("create symlink %s: %w", action.Target, err)
		}
		return nil
	case "copy":
		if len(action.Content) > 0 {
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

func applyReplace(action plan.Action, source string, opts Options) error {
	if err := createBackupSet(opts, action.Target); err != nil {
		return err
	}
	if err := removeConflictingTarget(action.Target); err != nil {
		return fmt.Errorf("remove conflicting target %s: %w", action.Target, err)
	}
	if err := applyCreate(action, source); err != nil {
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
				return managedActionResult{}, fmt.Errorf("reconcile JSON update for %s: %w", action.Target, err)
			}
			return result, nil
		}
		result, err := mergeJSONContentFile(action.Target, current, opts.Home)
		if err != nil {
			return managedActionResult{}, fmt.Errorf("merge JSON update for %s: %w", action.Target, err)
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
				return managedActionResult{}, fmt.Errorf("reconcile JSONC update for %s: %w", action.Target, err)
			}
			return result, nil
		}
		result, err := mergeJSONCContentFile(action.Target, current, opts.Home)
		if err != nil {
			return managedActionResult{}, fmt.Errorf("merge JSONC update for %s: %w", action.Target, err)
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
				return managedActionResult{}, fmt.Errorf("reconcile TOML update for %s: %w", action.Target, err)
			}
			return result, nil
		}
		result, err := mergeTOMLContentFile(action.Target, current, opts.Home)
		if err != nil {
			return managedActionResult{}, fmt.Errorf("merge TOML update for %s: %w", action.Target, err)
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
			return managedActionResult{}, fmt.Errorf("advance seeded target %s: %w", action.Target, err)
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
			return managedActionResult{}, fmt.Errorf("update marked block %s: %w", action.Target, err)
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

func updateConfinedRegularFile(target, home string, transform confinedTransform) (managedActionResult, error) {
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
	defer root.Close()
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
	defer file.Close()
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
	result := managedActionResult{
		PreviousTargetContent: append([]byte(nil), live...),
		TargetContent:         append([]byte(nil), updated...),
		ExactTargetContent:    true,
	}
	if !changed {
		result.TargetContent = append([]byte(nil), live...)
		return result, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return managedActionResult{}, fmt.Errorf("rewind confined target %s: %w", target, err)
	}
	if err := file.Truncate(0); err != nil {
		return managedActionResult{}, fmt.Errorf("truncate confined target %s: %w", target, err)
	}
	written, err := file.Write(updated)
	if err != nil {
		return managedActionResult{}, fmt.Errorf("write confined target %s: %w", target, err)
	}
	if written != len(updated) {
		return managedActionResult{}, fmt.Errorf("write confined target %s: wrote %d of %d bytes", target, written, len(updated))
	}
	if err := file.Sync(); err != nil {
		return managedActionResult{}, fmt.Errorf("sync confined target %s: %w", target, err)
	}
	return result, nil
}

func restoreConfinedRegularFile(target, home string, content []byte) error {
	_, err := updateConfinedRegularFile(target, home, func(live []byte) ([]byte, bool, error) {
		return content, !bytes.Equal(live, content), nil
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
	if err := applyCreate(action, source); err != nil {
		return err
	}
	return nil
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
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(target)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(target)
		return err
	}
	return os.Chmod(target, info.Mode().Perm())
}

func writeNewFileFromSourceMode(source, target string, data []byte) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(target)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(target)
		return err
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
