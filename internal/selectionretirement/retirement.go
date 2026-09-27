// Package selectionretirement validates explicit selection transitions and
// applies the filesystem and Installation Metadata effects of reductions. It
// deliberately handles only whole Managed Entry retirement; shared-ownership
// subtraction and still-selected target updates belong to the forward
// ownership-specific reconciliation flow.
package selectionretirement

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/ownershipevidence"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/selectionreconciliation"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/textblock"
)

// Options contains the resolved roots needed to validate and apply retirement.
type Options struct {
	SourceRoot string
	Home       string
	StateRoot  string
	// ForwardPlan owns every target that remains selected. Build requires a
	// matching safe action before delegating contribution reconciliation to it.
	ForwardPlan *plan.Plan
}

// Action is one validated retirement effect.
type Action struct {
	Target  string                          `json:"target"`
	Outcome selectionreconciliation.Outcome `json:"outcome"`
}

// Plan is a validated, deterministic sequence of retirement effects.
type Plan struct {
	Actions                    []Action `json:"actions"`
	records                    map[string]state.Record
	symlinkTransitions         []symlinkTransitionAuthority
	expectedInstalledSelection *state.InstalledSelection
	home                       string
}

type symlinkTransitionAuthority struct {
	target                  string
	previousSources         []string
	currentSources          []string
	recordFingerprint       string
	expectedLinkDestination string
	forwardAction           plan.Action
	requiresReplace         bool
}

// Result distinguishes targets deleted by dots from targets preserved while
// dots released its ownership record. Both slices are always non-nil.
type Result struct {
	Removed  []string `json:"removed"`
	Retained []string `json:"retained"`
}

// Build validates every Managed Entry retirement and any explicit Replace
// requirement for an owned symlink source switch before returning a plan. It
// performs no mutation.
func Build(report selectionreconciliation.Report, meta state.Metadata, opts Options) (Plan, error) {
	home, err := cleanAbs(opts.Home)
	if err != nil {
		return Plan{}, fmt.Errorf("build selection retirement: resolve home: %w", err)
	}
	if opts.Home == "" {
		return Plan{}, fmt.Errorf("build selection retirement: home is required")
	}
	if opts.SourceRoot == "" {
		return Plan{}, fmt.Errorf("build selection retirement: source root is required")
	}

	result := Plan{
		Actions:                    make([]Action, 0),
		records:                    make(map[string]state.Record),
		symlinkTransitions:         make([]symlinkTransitionAuthority, 0),
		expectedInstalledSelection: cloneInstalledSelection(meta.InstalledSelection),
		home:                       home,
	}
	if report.RequestedIntent.Authority != selectionreconciliation.AuthorityExplicitRequest {
		return result, nil
	}
	selectionReduction := hasSelectionReduction(report.Actions)
	seen := make(map[string]bool)
	seenTransition := make(map[string]bool)
	for _, action := range report.Actions {
		if action.Scope != selectionreconciliation.ScopeManagedEntry || !retiresSource(action.PreviousSources, action.CurrentSources) {
			continue
		}
		if action.Reason == selectionreconciliation.ReasonManifestEvolution {
			continue
		}
		// Addition-only source changes normally belong entirely to the forward
		// installer. The one exception is a whole-owned symlink conflict: its
		// existing Replace path needs explicit authorization before the broader
		// selection can be committed.
		if !selectionReduction && !forwardSymlinkTransition(action.ResolvedTarget, opts.ForwardPlan) {
			continue
		}
		rec, recorded := meta.FindByTarget(action.ResolvedTarget)
		if len(action.CurrentSources) != 0 {
			switch action.Outcome {
			case selectionreconciliation.OutcomeCreate,
				selectionreconciliation.OutcomeUpdate,
				selectionreconciliation.OutcomePreserve,
				selectionreconciliation.OutcomeReconcile:
				if !recorded {
					return Plan{}, fmt.Errorf("build selection retirement for %s: installation metadata record is required for partial retirement", action.ResolvedTarget)
				}
				transition, err := validateForwardReconciliation(action, rec, opts.ForwardPlan, opts.SourceRoot)
				if err != nil {
					return Plan{}, fmt.Errorf("build selection retirement for %s: %w", action.ResolvedTarget, err)
				}
				if transition != nil && !seenTransition[action.ResolvedTarget] {
					if err := validateTarget(action.ResolvedTarget, home); err != nil {
						return Plan{}, fmt.Errorf("build selection retirement for %s: %w", action.ResolvedTarget, err)
					}
					result.symlinkTransitions = append(result.symlinkTransitions, *transition)
					seenTransition[action.ResolvedTarget] = true
				}
				// The forward Managed Entry plan owns still-selected targets. Its
				// ownership-specific update also replaces contribution evidence at
				// the terminal metadata commit, so retirement must only validate the
				// reconciliation report and avoid applying the target twice.
				continue
			default:
				return Plan{}, fmt.Errorf("build selection retirement for %s: partial retirement outcome %q is unsafe: %s", action.ResolvedTarget, action.Outcome, action.Reason)
			}
		}
		if !selectionReduction {
			continue
		}
		if action.ResolvedTarget == "" {
			return Plan{}, fmt.Errorf("build selection retirement: managed entry target is required")
		}
		if seen[action.ResolvedTarget] {
			return Plan{}, fmt.Errorf("build selection retirement: duplicate target %s", action.ResolvedTarget)
		}
		seen[action.ResolvedTarget] = true
		if err := validateTarget(action.ResolvedTarget, home); err != nil {
			return Plan{}, fmt.Errorf("build selection retirement for %s: %w", action.ResolvedTarget, err)
		}
		if action.Outcome != selectionreconciliation.OutcomeRemove && action.Outcome != selectionreconciliation.OutcomeRetain {
			return Plan{}, fmt.Errorf("build selection retirement for %s: outcome %q is unsupported", action.ResolvedTarget, action.Outcome)
		}

		if !recorded {
			if action.Outcome == selectionreconciliation.OutcomeRemove {
				return Plan{}, fmt.Errorf("build selection retirement for %s: installation metadata record is required", action.ResolvedTarget)
			}
			result.Actions = append(result.Actions, Action{Target: action.ResolvedTarget, Outcome: action.Outcome})
			continue
		}

		if action.Outcome == selectionreconciliation.OutcomeRemove {
			removable, classifyErr := entireTargetRemovable(rec, opts.SourceRoot, home)
			if classifyErr != nil {
				return Plan{}, fmt.Errorf("build selection retirement for %s: %w", action.ResolvedTarget, classifyErr)
			}
			if !removable {
				return Plan{}, fmt.Errorf("build selection retirement for %s: target is not proven entirely removable", action.ResolvedTarget)
			}
		}
		result.Actions = append(result.Actions, Action{Target: action.ResolvedTarget, Outcome: action.Outcome})
		result.records[action.ResolvedTarget] = rec.Clone()
	}
	return result, nil
}

func forwardSymlinkTransition(target string, forward *plan.Plan) bool {
	if forward == nil {
		return false
	}
	count := 0
	transition := false
	for _, action := range forward.Actions {
		if action.Target != target {
			continue
		}
		count++
		transition = action.Strategy == "symlink" && normalizedOwnership(action.Ownership) == "whole"
	}
	return count == 1 && transition
}

// AuthorizeSymlinkTransitions revalidates the detached authority for every
// symlink source switch against fresh Installation Metadata, the current
// forward plan, the live target, and the operator's actual Conflict Resolution
// decisions. Callers run it after resolving conflicts and immediately before
// any dependency or filesystem effect.
func (p Plan) AuthorizeSymlinkTransitions(meta state.Metadata, forward *plan.Plan, decisions map[string]install.ConflictDecision) error {
	if len(p.symlinkTransitions) == 0 {
		return nil
	}
	if !reflect.DeepEqual(meta.InstalledSelection, p.expectedInstalledSelection) {
		return fmt.Errorf("authorize selection symlink transition: Installed Selection changed after planning")
	}
	for _, authority := range p.symlinkTransitions {
		if authority.requiresReplace && decisions[authority.target] != install.DecisionReplace {
			return fmt.Errorf("selection source switch for %s requires Conflict Resolution Replace; choose Replace interactively or rerun with --backup-and-replace", authority.target)
		}
		record, ok := meta.FindByTarget(authority.target)
		if !ok {
			return fmt.Errorf("authorize selection symlink transition for %s: installation metadata record disappeared", authority.target)
		}
		fingerprint, err := state.RecordEvidenceFingerprint(record)
		if err != nil || fingerprint != authority.recordFingerprint || !reflect.DeepEqual(record.SourceList(), authority.previousSources) {
			return fmt.Errorf("authorize selection symlink transition for %s: exact recorded contribution authority changed", authority.target)
		}
		matched, err := exactForwardAction(forward, authority.target)
		if err != nil {
			return fmt.Errorf("authorize selection symlink transition for %s: %w", authority.target, err)
		}
		if !reflect.DeepEqual(forwardActionSources(*matched), authority.currentSources) {
			return fmt.Errorf("authorize selection symlink transition for %s: ordered forward sources changed after planning", authority.target)
		}
		if authority.requiresReplace && (matched.Status != plan.StatusConflict || matched.Strategy != "symlink" || normalizedOwnership(matched.Ownership) != "whole" || matched.PreviousRecordFingerprint != authority.recordFingerprint) {
			return fmt.Errorf("authorize selection symlink transition for %s: forward conflict authority changed after planning", authority.target)
		}
		if !reflect.DeepEqual(clonePlanAction(*matched), authority.forwardAction) {
			return fmt.Errorf("authorize selection symlink transition for %s: forward action changed after planning", authority.target)
		}
		if err := validateTarget(authority.target, p.home); err != nil {
			return fmt.Errorf("authorize selection symlink transition for %s: %w", authority.target, err)
		}
		if err := validateLiveSymlink(authority.target, authority.expectedLinkDestination); err != nil {
			return fmt.Errorf("authorize selection symlink transition for %s: %w", authority.target, err)
		}
	}
	return nil
}

func validateForwardReconciliation(action selectionreconciliation.Action, record state.Record, forward *plan.Plan, sourceRoot string) (*symlinkTransitionAuthority, error) {
	if forward == nil {
		return nil, fmt.Errorf("forward plan is required for partial retirement")
	}
	want := plan.StatusUnchanged
	switch action.Outcome {
	case selectionreconciliation.OutcomeCreate:
		want = plan.StatusCreate
	case selectionreconciliation.OutcomeUpdate, selectionreconciliation.OutcomeReconcile:
		want = plan.StatusUpdate
	case selectionreconciliation.OutcomePreserve:
		want = plan.StatusUnchanged
	}
	matched, err := exactForwardAction(forward, action.ResolvedTarget)
	if err != nil {
		return nil, err
	}
	requiresReplace := action.Outcome == selectionreconciliation.OutcomeUpdate && matched.Status == plan.StatusConflict &&
		matched.Strategy == "symlink" && normalizedOwnership(matched.Ownership) == "whole"
	if matched.Status != want && !requiresReplace {
		return nil, fmt.Errorf("forward action status %q does not implement safe outcome %q", matched.Status, action.Outcome)
	}
	currentSources := []string{matched.Source}
	if len(matched.Sources) > 0 {
		currentSources = matched.Sources
	}
	if !reflect.DeepEqual(currentSources, action.CurrentSources) {
		return nil, fmt.Errorf("forward action sources %v do not match reconciliation sources %v", currentSources, action.CurrentSources)
	}
	if !reflect.DeepEqual(record.SourceList(), action.PreviousSources) {
		return nil, fmt.Errorf("recorded sources %v do not match reconciliation evidence %v", record.SourceList(), action.PreviousSources)
	}
	if matched.Strategy != record.Strategy || normalizedOwnership(matched.Ownership) != normalizedOwnership(record.Ownership) {
		return nil, fmt.Errorf("forward action mode %s/%s does not match recorded mode %s/%s", matched.Strategy, normalizedOwnership(matched.Ownership), record.Strategy, normalizedOwnership(record.Ownership))
	}
	if len(matched.Contributions) != len(action.CurrentSources) {
		return nil, fmt.Errorf("forward action has %d contribution identities for %d current sources", len(matched.Contributions), len(action.CurrentSources))
	}
	for i, contribution := range matched.Contributions {
		if contribution.Source != action.CurrentSources[i] {
			return nil, fmt.Errorf("forward contribution source %q does not match reconciliation source %q", contribution.Source, action.CurrentSources[i])
		}
	}
	recordFingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil || matched.PreviousRecordFingerprint == "" || matched.PreviousRecordFingerprint != recordFingerprint {
		return nil, fmt.Errorf("forward action is not bound to the exact recorded contribution authority")
	}
	var expectedLinkDestination string
	if requiresReplace {
		expectedLinkDestination, err = validateSymlinkReplacement(action, record, sourceRoot)
		if err != nil {
			return nil, err
		}
	} else if want == plan.StatusUpdate && !matchesPreviousEvidence(*matched, record, action.PreviousSources) {
		return nil, fmt.Errorf("forward update does not carry the exact recorded previous contribution evidence")
	} else if matched.Strategy == "symlink" && normalizedOwnership(matched.Ownership) == "whole" && action.Outcome == selectionreconciliation.OutcomePreserve {
		if err := validateExactSymlinkContribution(record, action.PreviousSources); err != nil {
			return nil, err
		}
		if len(action.CurrentSources) != 1 {
			return nil, fmt.Errorf("forward symlink transition requires exactly one current source")
		}
		expectedLinkDestination, err = plan.ResolveSource(action.CurrentSources[0], sourceRoot)
		if err != nil {
			return nil, fmt.Errorf("resolve current symlink source: %w", err)
		}
		if err := validateLiveSymlink(action.ResolvedTarget, expectedLinkDestination); err != nil {
			return nil, err
		}
	}
	if expectedLinkDestination == "" {
		return nil, nil
	}
	return &symlinkTransitionAuthority{
		target:                  action.ResolvedTarget,
		previousSources:         append([]string(nil), action.PreviousSources...),
		currentSources:          append([]string(nil), action.CurrentSources...),
		recordFingerprint:       recordFingerprint,
		expectedLinkDestination: expectedLinkDestination,
		forwardAction:           clonePlanAction(*matched),
		requiresReplace:         requiresReplace,
	}, nil
}

func validateSymlinkReplacement(action selectionreconciliation.Action, record state.Record, sourceRoot string) (string, error) {
	if err := validateExactSymlinkContribution(record, action.PreviousSources); err != nil {
		return "", err
	}
	expected, err := plan.ResolveSource(action.PreviousSources[0], sourceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve previous symlink source: %w", err)
	}
	if err := validateLiveSymlink(action.ResolvedTarget, expected); err != nil {
		return "", err
	}
	return expected, nil
}

func validateExactSymlinkContribution(record state.Record, previousSources []string) error {
	if len(previousSources) != 1 || record.Strategy != "symlink" || normalizedOwnership(record.Ownership) != "whole" || len(record.Contributions) != 1 {
		return fmt.Errorf("forward symlink replacement lacks exact previous contribution evidence")
	}
	contribution := record.Contributions[0]
	if contribution.Source != previousSources[0] || contribution.Ownership != "whole" || !contribution.EvidenceRecorded {
		return fmt.Errorf("forward symlink replacement lacks exact previous contribution evidence")
	}
	return nil
}

func validateLiveSymlink(target, expected string) error {
	info, err := os.Lstat(target)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("forward symlink replacement target is no longer the exact previous link")
	}
	destination, err := os.Readlink(target)
	if err != nil || filepath.Clean(destination) != filepath.Clean(expected) {
		return fmt.Errorf("forward symlink replacement target is no longer the exact previous link")
	}
	return nil
}

func exactForwardAction(forward *plan.Plan, target string) (*plan.Action, error) {
	if forward == nil {
		return nil, fmt.Errorf("forward plan is required for partial retirement")
	}
	var matched *plan.Action
	for i := range forward.Actions {
		candidate := &forward.Actions[i]
		if candidate.Target != target {
			continue
		}
		if matched != nil {
			return nil, fmt.Errorf("forward plan contains duplicate target %s", target)
		}
		matched = candidate
	}
	if matched == nil {
		return nil, fmt.Errorf("forward action is required for partial retirement")
	}
	return matched, nil
}

func forwardActionSources(action plan.Action) []string {
	if len(action.Sources) > 0 {
		return append([]string(nil), action.Sources...)
	}
	return []string{action.Source}
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

func clonePlanAction(action plan.Action) plan.Action {
	cloned := action
	cloned.Sources = append([]string(nil), action.Sources...)
	cloned.ResolvedSources = append([]string(nil), action.ResolvedSources...)
	cloned.Contributions = append([]plan.Contribution(nil), action.Contributions...)
	for index := range cloned.Contributions {
		cloned.Contributions[index].SelectorTags = append([]string(nil), action.Contributions[index].SelectorTags...)
	}
	cloned.Content = append([]byte(nil), action.Content...)
	cloned.PreviousContent = append([]byte(nil), action.PreviousContent...)
	cloned.MatchingTags = append([]string(nil), action.MatchingTags...)
	cloned.PreviousReconciliationReceipt = action.PreviousReconciliationReceipt.Clone()
	if action.Migration != nil {
		migration := *action.Migration
		migration.CapturedContent = append([]byte(nil), action.Migration.CapturedContent...)
		migration.PreviousSourceContent = append([]byte(nil), action.Migration.PreviousSourceContent...)
		migration.ExpectedLinkContent = append([]byte(nil), action.Migration.ExpectedLinkContent...)
		migration.FinalContent = append([]byte(nil), action.Migration.FinalContent...)
		migration.RecordedBaseline = append([]byte(nil), action.Migration.RecordedBaseline...)
		cloned.Migration = &migration
	}
	return cloned
}

func normalizedOwnership(ownership string) string {
	if ownership == "" {
		return "whole"
	}
	return ownership
}

func matchesPreviousEvidence(action plan.Action, record state.Record, previousSources []string) bool {
	projected, err := ownershipevidence.ProjectRecord(record)
	if err != nil {
		return false
	}
	switch normalizedOwnership(record.Ownership) {
	case "json-subset", "jsonc-subset":
		return len(action.PreviousContent) > 0 && reflect.DeepEqual(action.PreviousContent, []byte(projected.OwnedContent))
	case "toml-subset", "marked-block":
		return len(action.PreviousContent) > 0 && reflect.DeepEqual(action.PreviousContent, projected.OwnedBytes)
	case "seeded":
		return action.PreviousContent != nil && reflect.DeepEqual(action.PreviousContent, projected.SeededBaseline)
	case "whole":
		if len(previousSources) != 1 || action.PreviousHash == "" {
			return false
		}
		for _, contribution := range record.Contributions {
			if contribution.Source == previousSources[0] && contribution.Ownership == "whole" {
				return action.PreviousHash == contribution.Hash
			}
		}
	}
	return false
}

func hasSelectionReduction(actions []selectionreconciliation.Action) bool {
	for _, action := range actions {
		if action.Scope == selectionreconciliation.ScopeSelection && action.Outcome == selectionreconciliation.OutcomeRemove {
			return true
		}
	}
	return false
}

func retiresSource(previous, current []string) bool {
	selected := make(map[string]bool, len(current))
	for _, source := range current {
		selected[source] = true
	}
	for _, source := range previous {
		if !selected[source] {
			return true
		}
	}
	return false
}

// Apply reloads Installation Metadata and revalidates every target immediately
// before acting. A stale remove is converted to retain-and-release.
func Apply(retirement Plan, opts Options) (Result, error) {
	return applyWithRemove(retirement, opts, nil)
}

func applyWithRemove(retirement Plan, opts Options, injectedRemove func(string) error) (Result, error) {
	result := Result{Removed: make([]string, 0), Retained: make([]string, 0)}
	home, err := cleanAbs(opts.Home)
	if err != nil {
		return result, fmt.Errorf("apply selection retirement: resolve home: %w", err)
	}
	if opts.Home == "" {
		return result, fmt.Errorf("apply selection retirement: home is required")
	}
	if opts.SourceRoot == "" {
		return result, fmt.Errorf("apply selection retirement: source root is required")
	}
	if opts.StateRoot == "" {
		return result, fmt.Errorf("apply selection retirement: state root is required")
	}
	if err := validateStateRoot(opts.StateRoot, home); err != nil {
		return result, fmt.Errorf("apply selection retirement: %w", err)
	}

	metadataPath := state.Path(opts.StateRoot)
	locked, err := state.LockMetadata(metadataPath)
	if err != nil {
		return result, fmt.Errorf("apply selection retirement: lock installation metadata: %w", err)
	}
	meta, err := locked.Load()
	if err != nil {
		return result, errors.Join(fmt.Errorf("apply selection retirement: load installation metadata: %w", err), locked.Close())
	}
	homeRoot, err := os.OpenRoot(home)
	if err != nil {
		return result, errors.Join(fmt.Errorf("apply selection retirement: open home root: %w", err), locked.Close())
	}
	result, applyErr := applyLoaded(retirement, opts, home, locked, meta, homeRoot, injectedRemove)
	if closeErr := homeRoot.Close(); closeErr != nil {
		applyErr = errors.Join(applyErr, fmt.Errorf("apply selection retirement: close home root: %w", closeErr))
	}
	if closeErr := locked.Close(); closeErr != nil {
		applyErr = errors.Join(applyErr, closeErr)
	}
	return result, applyErr
}

func applyLoaded(retirement Plan, opts Options, home string, metadata *state.LockedMetadata, meta state.Metadata, homeRoot *os.Root, injectedRemove func(string) error) (Result, error) {
	result := Result{Removed: make([]string, 0), Retained: make([]string, 0)}
	if err := prevalidateMetadata(retirement, meta); err != nil {
		return result, err
	}
	changed := false
	for _, action := range retirement.Actions {
		latest, err := metadata.Load()
		if err != nil {
			return result, fmt.Errorf("apply selection retirement: reload installation metadata: %w", err)
		}
		if err := prevalidateActionMetadata(action, retirement.records, latest); err != nil {
			return result, err
		}
		rec, ok := latest.FindByTarget(action.Target)
		if !ok {
			result.Retained = append(result.Retained, action.Target)
			continue
		}
		if err := validateTarget(rec.Target, home); err != nil {
			return result, fmt.Errorf("apply selection retirement for %s: %w", action.Target, err)
		}

		remove := false
		if action.Outcome == selectionreconciliation.OutcomeRemove {
			var classifyErr error
			remove, classifyErr = entireTargetRemovable(rec, opts.SourceRoot, home)
			if classifyErr != nil {
				return result, fmt.Errorf("apply selection retirement for %s: %w", action.Target, classifyErr)
			}
		} else if _, statErr := os.Lstat(rec.Target); statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return result, fmt.Errorf("apply selection retirement for %s: revalidate retained target: %w", action.Target, statErr)
		}
		if remove {
			if err := plan.ValidateBackupableTarget(rec.Target); err != nil {
				return result, fmt.Errorf("apply selection retirement for %s: %w", action.Target, err)
			}
			removeFn := injectedRemove
			if removeFn == nil {
				relative, relErr := filepath.Rel(home, rec.Target)
				if relErr != nil {
					return result, fmt.Errorf("apply selection retirement for %s: resolve target relative to home: %w", action.Target, relErr)
				}
				if !filepath.IsLocal(relative) {
					return result, fmt.Errorf("apply selection retirement for %s: target is not local to home", action.Target)
				}
				removeFn = func(string) error { return homeRoot.Remove(relative) }
			}
			if err := removeFn(rec.Target); err != nil {
				return result, fmt.Errorf("apply selection retirement for %s: remove target: %w", action.Target, err)
			}
			result.Removed = append(result.Removed, action.Target)
		} else {
			result.Retained = append(result.Retained, action.Target)
		}
		changed = true
	}

	if changed {
		latest, err := metadata.Load()
		if err != nil {
			return result, fmt.Errorf("apply selection retirement: reload installation metadata before prune: %w", err)
		}
		if err := prevalidateMetadata(retirement, latest); err != nil {
			return result, err
		}
		for _, action := range retirement.Actions {
			if _, planned := retirement.records[action.Target]; planned {
				latest = latest.Remove(action.Target)
			}
		}
		if err := metadata.Save(latest); err != nil {
			return result, fmt.Errorf("apply selection retirement: save pruned installation metadata: %w", err)
		}
	}
	return result, nil
}

func prevalidateActionMetadata(action Action, records map[string]state.Record, meta state.Metadata) error {
	current, exists := meta.FindByTarget(action.Target)
	if !exists {
		return nil
	}
	planned, plannedExists := records[action.Target]
	if !plannedExists || !reflect.DeepEqual(current, planned) {
		return fmt.Errorf("apply selection retirement for %s: installation metadata changed after build", action.Target)
	}
	return nil
}

func prevalidateMetadata(retirement Plan, meta state.Metadata) error {
	seen := make(map[string]bool)
	for _, action := range retirement.Actions {
		if seen[action.Target] {
			return fmt.Errorf("apply selection retirement: duplicate target %s", action.Target)
		}
		seen[action.Target] = true
		if action.Outcome != selectionreconciliation.OutcomeRemove && action.Outcome != selectionreconciliation.OutcomeRetain {
			return fmt.Errorf("apply selection retirement for %s: outcome %q is unsupported", action.Target, action.Outcome)
		}
		if err := prevalidateActionMetadata(action, retirement.records, meta); err != nil {
			return err
		}
	}
	return nil
}

func classify(rec state.Record, sourceRoot, home string) (plan.UninstallStatus, error) {
	p, err := plan.BuildUninstall(state.Metadata{Entries: []state.Record{rec}}, plan.UninstallOptions{SourceRoot: sourceRoot, Home: home})
	if err != nil {
		return "", fmt.Errorf("classify target: %w", err)
	}
	if len(p.Actions) != 1 {
		return "", fmt.Errorf("classify target: expected one uninstall action, got %d", len(p.Actions))
	}
	return p.Actions[0].Status, nil
}

func entireTargetRemovable(rec state.Record, sourceRoot, home string) (bool, error) {
	if !supportedRetirementOwnership(rec.Ownership) || rec.Ownership == "seeded" {
		return false, fmt.Errorf("ownership %q cannot remove an entire target", rec.Ownership)
	}
	status, err := classify(rec, sourceRoot, home)
	if err != nil || status != plan.UninstallRemove {
		return false, err
	}
	if rec.Ownership == "whole" {
		return true, nil
	}
	live, err := os.ReadFile(rec.Target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("read partial target: %w", err)
	}
	var empty, compatible bool
	switch rec.Ownership {
	case "json-subset":
		_, _, empty, compatible, err = configsubset.RemoveJSON(live, rec.OwnedContent)
	case "jsonc-subset":
		_, _, empty, compatible, err = configsubset.RemoveJSONC(live, rec.OwnedContent)
	case "toml-subset":
		_, _, empty, compatible, err = configsubset.RemoveTOML(live, rec.OwnedBytes)
	case "marked-block":
		_, _, empty, compatible = textblock.RemoveOwned(live, rec.OwnedBytes, textblock.DotsManagedMarkers())
	}
	if err != nil {
		return false, fmt.Errorf("analyze entire partial target removal: %w", err)
	}
	return compatible && empty, nil
}

func supportedRetirementOwnership(ownership string) bool {
	switch ownership {
	case "whole", "seeded", "json-subset", "jsonc-subset", "toml-subset", "marked-block":
		return true
	default:
		return false
	}
}

func validateTarget(target, home string) error {
	if err := plan.ValidateResolvedTarget(target, home); err != nil {
		return err
	}
	if err := plan.ValidateTargetParentInsideHome(target, home); err != nil {
		return err
	}
	return nil
}

func validateStateRoot(stateRoot, home string) error {
	stateAbs, err := cleanAbs(stateRoot)
	if err != nil {
		return fmt.Errorf("resolve state root: %w", err)
	}
	if !plan.InsideRoot(stateAbs, home) {
		return nil
	}
	if err := plan.ValidatePathInsideHomeNoSymlinkEscape(stateAbs, home, "state root"); err != nil {
		return err
	}
	return plan.ValidateFilePathInsideHomeNoSymlinkEscape(state.Path(stateAbs), home, "installation metadata")
}

func cleanAbs(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
