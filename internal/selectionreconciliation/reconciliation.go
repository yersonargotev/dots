// Package selectionreconciliation builds a deterministic, read-only Selection
// Reconciliation Plan from already resolved selection and inspection evidence.
// It never reads or writes the filesystem or mutates Installation Metadata.
package selectionreconciliation

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/seededstate"
	"github.com/yersonargotev/dots/internal/selectedsurface"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/textblock"
)

// Authority identifies whether the requested intent may authorize retirement.
type Authority string

const (
	AuthorityRecorded          Authority = "recorded"
	AuthorityExplicitRequest   Authority = "explicit-request"
	AuthorityManifestEvolution Authority = "manifest-evolution"
)

var errMissingSourceEvidence = errors.New("source evidence is missing")

// Intent is a detached selection intent and its resolved Tag snapshot.
type Intent struct {
	Authority    Authority `json:"authority"`
	Profiles     []string  `json:"profiles"`
	ExtraTags    []string  `json:"extra_tags"`
	ResolvedTags []string  `json:"resolved_tags"`
}

// Outcome is the stable result of one reconciliation action.
type Outcome string

const (
	OutcomeCreate                Outcome = "create"
	OutcomeUpdate                Outcome = "update"
	OutcomePreserve              Outcome = "preserve"
	OutcomeReconcile             Outcome = "reconcile"
	OutcomeRemove                Outcome = "remove"
	OutcomeRetain                Outcome = "retain"
	OutcomeBlocked               Outcome = "blocked"
	OutcomeRetainedExternalState Outcome = "retained-external-state"
)

// Scope identifies the kind of selected surface item described by an action.
type Scope string

const (
	ScopeSelection    Scope = "selection"
	ScopeManagedEntry Scope = "managed-entry"
	ScopeDependency   Scope = "dependency"
	ScopeProvisioner  Scope = "provisioner"
)

const (
	ReasonWholeTargetDrift          = "whole-target-drift"
	ReasonLostOwnership             = "lost-ownership"
	ReasonAmbiguousPartialOwnership = "ambiguous-partial-ownership"
	ReasonMissingSource             = "missing-source"
	ReasonManifestEvolution         = "manifest-evolution-report-only"
	ReasonLocalEvolution            = "seeded-local-evolution"
)

// TargetKind is the inspected filesystem object kind. The adapter supplies it;
// Build does not perform any filesystem inspection.
type TargetKind string

const (
	TargetKindRegular TargetKind = "regular"
	TargetKindSymlink TargetKind = "symlink"
	TargetKindOther   TargetKind = "other"
)

// ForwardStatus is the adapter's portable projection of the existing Install
// Plan classification. Supplying it lets Build reuse template rendering and
// ordinary forward-install decisions instead of reproducing them.
type ForwardStatus string

const (
	ForwardCreate        ForwardStatus = "create"
	ForwardUpdate        ForwardStatus = "update"
	ForwardUnchanged     ForwardStatus = "unchanged"
	ForwardConflict      ForwardStatus = "conflict"
	ForwardMissingSource ForwardStatus = "missing-source"
)

// TargetEvidence is an immutable snapshot prepared by the filesystem adapter.
// DeclarativeTarget joins it to Selected Surface order. ResolvedTarget joins it
// to Installation Metadata and is also carried into the resulting action.
type TargetEvidence struct {
	DeclarativeTarget string
	ResolvedTarget    string
	// RetirementAuthority overrides the requested intent for contributions
	// reconstructed only from Installation Metadata after manifest evolution.
	RetirementAuthority Authority
	Exists              bool
	Kind                TargetKind
	Content             []byte
	LinkDestination     string
	ForwardStatus       ForwardStatus
	ForwardReason       string
}

// SourceEvidence is an immutable snapshot of the selected source contribution.
// Content is the final desired contribution (rendered already, when needed).
type SourceEvidence struct {
	DeclarativeTarget string
	Source            string
	ResolvedSource    string
	Exists            bool
	Content           []byte
}

// Evidence contains complete, ordered inspection snapshots. Slices are treated
// as immutable and result values never alias their byte storage.
type Evidence struct {
	Targets              []TargetEvidence
	Sources              []SourceEvidence
	PreviousProvisioners []ProvisionerEvidence
	CurrentProvisioners  []ProvisionerEvidence
}

// ProvisionerEvidence identifies one external Provisioner effect by its exact
// rendered command while keeping command details out of the public report.
type ProvisionerEvidence struct {
	Identity string
	Name     string
}

// NewProvisionerEvidence builds a stable identity from the exact command that
// a manifest declaration or Installation Metadata receipt represents.
func NewProvisionerEvidence(tool, executable string, args []string) ProvisionerEvidence {
	parts := append([]string{tool, executable}, args...)
	return ProvisionerEvidence{Identity: strings.Join(parts, "\x00"), Name: tool}
}

// Input is the complete pure seam for building a reconciliation report.
type Input struct {
	PreviousIntent  Intent
	RequestedIntent Intent
	PreviousSurface selectedsurface.Surface
	CurrentSurface  selectedsurface.Surface
	Metadata        state.Metadata
	Evidence        Evidence
}

// Action describes one semantic outcome. Arrays are always non-nil so JSON is
// stable. Managed Entry actions carry target and source fields; selection and
// external-state actions carry Names.
type Action struct {
	Scope             Scope    `json:"scope"`
	Outcome           Outcome  `json:"outcome"`
	Reason            string   `json:"reason,omitempty"`
	DeclarativeTarget string   `json:"declarative_target,omitempty"`
	ResolvedTarget    string   `json:"resolved_target,omitempty"`
	PreviousSources   []string `json:"previous_sources"`
	CurrentSources    []string `json:"current_sources"`
	Names             []string `json:"names"`
	Identity          string   `json:"identity,omitempty"`
}

// Report is a deterministic Selection Reconciliation Plan.
type Report struct {
	PreviousIntent  Intent   `json:"previous_intent"`
	RequestedIntent Intent   `json:"requested_intent"`
	Actions         []Action `json:"actions"`
}

// HasFindings reports whether the plan contains a blocked action or unsafe
// whole-target evidence that a read-only caller must surface as divergence.
// Retained External State and manifest-evolution retention stay informational.
func (r Report) HasFindings() bool {
	for _, action := range r.Actions {
		if action.Outcome == OutcomeBlocked {
			return true
		}
		if action.Scope == ScopeManagedEntry && action.Outcome == OutcomeRetain &&
			(action.Reason == ReasonWholeTargetDrift || action.Reason == ReasonLostOwnership || action.Reason == ReasonAmbiguousPartialOwnership) {
			return true
		}
	}
	return false
}

type targetGroup struct {
	target  string
	entries []selectedsurface.SelectedEntry
}

// Build computes a Selection Reconciliation Plan without mutating any input.
func Build(input Input) (Report, error) {
	if input.RequestedIntent.Authority != AuthorityExplicitRequest && input.RequestedIntent.Authority != AuthorityManifestEvolution {
		return Report{}, fmt.Errorf("build selection reconciliation: requested authority %q is invalid", input.RequestedIntent.Authority)
	}
	if input.PreviousIntent.Authority != AuthorityRecorded {
		return Report{}, fmt.Errorf("build selection reconciliation: previous authority %q is invalid", input.PreviousIntent.Authority)
	}

	report := Report{
		PreviousIntent:  cloneIntent(input.PreviousIntent),
		RequestedIntent: cloneIntent(input.RequestedIntent),
		Actions:         make([]Action, 0),
	}
	report.Actions = append(report.Actions, selectionActions(input.PreviousIntent, input.RequestedIntent)...)

	previousGroups, err := groupTargets(input.PreviousSurface.Entries)
	if err != nil {
		return Report{}, fmt.Errorf("build selection reconciliation: group previous surface: %w", err)
	}
	currentGroups, err := groupTargets(input.CurrentSurface.Entries)
	if err != nil {
		return Report{}, fmt.Errorf("build selection reconciliation: group current surface: %w", err)
	}

	previousByTarget := make(map[string]targetGroup, len(previousGroups))
	currentByTarget := make(map[string]targetGroup, len(currentGroups))
	for _, group := range previousGroups {
		previousByTarget[group.target] = group
	}
	for _, group := range currentGroups {
		currentByTarget[group.target] = group
	}
	for _, current := range currentGroups {
		action, buildErr := buildTargetAction(previousByTarget[current.target], current, input)
		if buildErr != nil {
			return Report{}, fmt.Errorf("build selection reconciliation for target %s: %w", current.target, buildErr)
		}
		report.Actions = append(report.Actions, action)
	}
	for _, previous := range previousGroups {
		if _, retained := currentByTarget[previous.target]; retained {
			continue
		}
		action, buildErr := buildTargetAction(previous, targetGroup{}, input)
		if buildErr != nil {
			return Report{}, fmt.Errorf("build selection reconciliation for target %s: %w", previous.target, buildErr)
		}
		report.Actions = append(report.Actions, action)
	}

	report.Actions = append(report.Actions, externalActions(input.PreviousSurface, input.CurrentSurface, input.Evidence)...)
	return report, nil
}

func selectionActions(previous, current Intent) []Action {
	actions := make([]Action, 0, 2)
	removed := append(difference(previous.Profiles, current.Profiles), difference(previous.ExtraTags, current.ExtraTags)...)
	removed = append(removed, difference(previous.ResolvedTags, current.ResolvedTags)...)
	if len(removed) > 0 {
		actions = append(actions, newNamedAction(ScopeSelection, OutcomeRemove, orderedUnique(removed)))
	}
	added := append(difference(current.Profiles, previous.Profiles), difference(current.ExtraTags, previous.ExtraTags)...)
	added = append(added, difference(current.ResolvedTags, previous.ResolvedTags)...)
	if len(added) > 0 {
		actions = append(actions, newNamedAction(ScopeSelection, OutcomeCreate, orderedUnique(added)))
	}
	return actions
}

func externalActions(previous, current selectedsurface.Surface, evidence Evidence) []Action {
	actions := make([]Action, 0)
	previousDependencies := dependencyNames(previous.Dependencies)
	currentDependencies := dependencyNames(current.Dependencies)
	for _, name := range difference(currentDependencies, previousDependencies) {
		actions = append(actions, newNamedAction(ScopeDependency, OutcomeCreate, []string{name}))
	}
	for _, name := range difference(previousDependencies, currentDependencies) {
		actions = append(actions, newNamedAction(ScopeDependency, OutcomeRetainedExternalState, []string{name}))
	}
	previousProvisioners := evidence.PreviousProvisioners
	if previousProvisioners == nil {
		previousProvisioners = surfaceProvisionerEvidence(previous.Provisioners)
	}
	currentProvisioners := evidence.CurrentProvisioners
	if currentProvisioners == nil {
		currentProvisioners = surfaceProvisionerEvidence(current.Provisioners)
	}
	actions = append(actions, provisionerActions(previousProvisioners, currentProvisioners)...)
	return actions
}

func provisionerActions(previous, current []ProvisionerEvidence) []Action {
	previous = orderedUniqueProvisioners(previous)
	current = orderedUniqueProvisioners(current)
	previousIDs := make(map[string]bool, len(previous))
	currentIDs := make(map[string]bool, len(current))
	for _, item := range previous {
		previousIDs[item.Identity] = true
	}
	for _, item := range current {
		currentIDs[item.Identity] = true
	}
	actions := make([]Action, 0)
	for _, item := range current {
		if !previousIDs[item.Identity] {
			actions = append(actions, newProvisionerAction(OutcomeCreate, item))
		}
	}
	for _, item := range previous {
		if !currentIDs[item.Identity] {
			actions = append(actions, newProvisionerAction(OutcomeRetainedExternalState, item))
		}
	}
	return actions
}

func surfaceProvisionerEvidence(provisioners []manifest.Provisioner) []ProvisionerEvidence {
	result := make([]ProvisionerEvidence, 0, len(provisioners))
	for _, item := range provisioners {
		executable, args := provision.RenderCommand(item)
		result = append(result, NewProvisionerEvidence(item.Tool, executable, args))
	}
	return result
}

func orderedUniqueProvisioners(items []ProvisionerEvidence) []ProvisionerEvidence {
	result := make([]ProvisionerEvidence, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Identity == "" || seen[item.Identity] {
			continue
		}
		seen[item.Identity] = true
		result = append(result, item)
	}
	return result
}

func newProvisionerAction(outcome Outcome, item ProvisionerEvidence) Action {
	action := newNamedAction(ScopeProvisioner, outcome, []string{item.Name})
	digest := sha256.Sum256([]byte(item.Identity))
	action.Identity = fmt.Sprintf("sha256:%x", digest)
	return action
}

func buildTargetAction(previous, current targetGroup, input Input) (Action, error) {
	declarativeTarget := current.target
	if declarativeTarget == "" {
		declarativeTarget = previous.target
	}
	action := Action{
		Scope:             ScopeManagedEntry,
		DeclarativeTarget: declarativeTarget,
		PreviousSources:   groupSources(previous),
		CurrentSources:    groupSources(current),
		Names:             make([]string, 0),
	}
	target, ok := findTargetEvidence(input.Evidence.Targets, declarativeTarget)
	if !ok {
		return Action{}, fmt.Errorf("target evidence is required")
	}
	action.ResolvedTarget = target.ResolvedTarget

	active := current
	if active.target == "" {
		active = previous
	}
	strategy, ownership, err := groupMode(active)
	if err != nil {
		return Action{}, err
	}
	if previous.target != "" {
		previousStrategy, previousOwnership, previousErr := groupMode(previous)
		if previousErr != nil {
			return Action{}, previousErr
		}
		if current.target != "" && (previousStrategy != strategy || normalizedOwnership(previousOwnership) != normalizedOwnership(ownership)) {
			return blocked(action, ReasonAmbiguousPartialOwnership), nil
		}
	}
	ownership = normalizedOwnership(ownership)

	record, recorded := findRecord(input.Metadata.Entries, target.ResolvedTarget)
	retirementAuthority := input.RequestedIntent.Authority
	if target.RetirementAuthority != "" {
		retirementAuthority = target.RetirementAuthority
	}
	if current.target == "" && retirementAuthority == AuthorityManifestEvolution {
		action.Outcome = OutcomeRetain
		action.Reason = ReasonManifestEvolution
		return action, nil
	}
	if current.target != "" && hasRetiredSources(previous, current) && retirementAuthority == AuthorityManifestEvolution {
		action.Outcome = OutcomeRetain
		action.Reason = ReasonManifestEvolution
		return action, nil
	}
	if current.target != "" && hasRetiredSources(previous, current) {
		switch target.ForwardStatus {
		case ForwardConflict:
			if partialOwnership(ownership) {
				return blocked(action, ReasonAmbiguousPartialOwnership), nil
			}
			return classifyForward(action, ownership, target.ForwardStatus, target.ForwardReason), nil
		case ForwardMissingSource:
			return blocked(action, ReasonMissingSource), nil
		}
	}
	if current.target != "" && !hasRetiredSources(previous, current) && target.ForwardStatus != "" {
		return classifyForward(action, ownership, target.ForwardStatus, target.ForwardReason), nil
	}

	currentContent, currentSources, err := composeGroup(current, ownership, input.Evidence.Sources)
	if err != nil {
		if missingSource(err) {
			return blocked(action, ReasonMissingSource), nil
		}
		return Action{}, err
	}
	if current.target != "" && !target.Exists {
		action.Outcome = OutcomeCreate
		return action, nil
	}
	if !target.Exists {
		if current.target == "" {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonLostOwnership
			return action, nil
		}
		return blocked(action, ReasonLostOwnership), nil
	}

	if strategy == "symlink" {
		return classifySymlink(action, previous, current, target, currentSources, record, recorded, input.Evidence.Sources), nil
	}
	if target.Kind != TargetKindRegular {
		if current.target == "" {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonLostOwnership
			return action, nil
		}
		return blocked(action, ReasonLostOwnership), nil
	}
	if ownership == "whole" {
		return classifyWhole(action, previous, current, target.Content, currentContent, record, recorded), nil
	}
	if ownership == "seeded" {
		return classifySeeded(action, previous, current, target.Content, currentContent, record, recorded), nil
	}
	return classifyPartial(action, previous, current, ownership, target.Content, currentContent, currentSources, record, recorded)
}

func partialOwnership(ownership string) bool {
	switch normalizedOwnership(ownership) {
	case "json-subset", "jsonc-subset", "toml-subset", "marked-block":
		return true
	default:
		return false
	}
}

func classifySymlink(action Action, previous, current targetGroup, target TargetEvidence, currentSources []SourceEvidence, record state.Record, recorded bool, evidence []SourceEvidence) Action {
	if target.Kind != TargetKindSymlink {
		if current.target == "" {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonLostOwnership
			return action
		}
		return blocked(action, ReasonLostOwnership)
	}
	if current.target == "" {
		if !recorded || target.LinkDestination == "" || !recordHasExactContributions(record, action.PreviousSources, "symlink", "whole") {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonLostOwnership
			return action
		}
		if len(action.PreviousSources) != 1 ||
			target.LinkDestination != resolvedSource(evidence, previous.target, action.PreviousSources[0]) {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonLostOwnership
			return action
		}
		action.Outcome = OutcomeRemove
		return action
	}
	if len(currentSources) != 1 {
		return blocked(action, ReasonLostOwnership)
	}
	if target.LinkDestination == currentSources[0].ResolvedSource {
		action.Outcome = OutcomePreserve
		return action
	}
	if recorded && previous.target != "" && recordHasExactContributions(record, action.PreviousSources, "symlink", "whole") {
		if len(action.PreviousSources) != 1 || target.LinkDestination != resolvedSource(evidence, previous.target, action.PreviousSources[0]) {
			return blocked(action, ReasonLostOwnership)
		}
		action.Outcome = OutcomeUpdate
		return action
	}
	return blocked(action, ReasonLostOwnership)
}

func classifyWhole(action Action, previous, current targetGroup, live, desired []byte, record state.Record, recorded bool) Action {
	if current.target != "" && bytes.Equal(live, desired) {
		action.Outcome = OutcomePreserve
		return action
	}
	if !recorded || previous.target == "" || !recordHasExactContributions(record, action.PreviousSources, previous.entries[0].Entry.Strategy, "whole") {
		if current.target == "" {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonLostOwnership
			return action
		}
		return blocked(action, ReasonLostOwnership)
	}
	contribution, _ := exactContribution(record.Contributions, action.PreviousSources[0], "whole")
	if contribution.Hash == "" || state.HashBytes(live) != contribution.Hash {
		if current.target == "" {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonWholeTargetDrift
			return action
		}
		return blocked(action, ReasonWholeTargetDrift)
	}
	if current.target == "" {
		action.Outcome = OutcomeRemove
	} else {
		action.Outcome = OutcomeUpdate
	}
	return action
}

func classifySeeded(action Action, previous, current targetGroup, live, desired []byte, record state.Record, recorded bool) Action {
	if current.target == "" {
		action.Outcome = OutcomeRetain
		return action
	}
	if !recorded || previous.target == "" || !recordHasExactContributions(record, action.PreviousSources, previous.entries[0].Entry.Strategy, "seeded") {
		if bytes.Equal(live, desired) {
			action.Outcome = OutcomePreserve
			return action
		}
		return blocked(action, ReasonLostOwnership)
	}
	previousBaseline, ok := exactSeededBaseline(record, action.PreviousSources)
	if !ok {
		return blocked(action, ReasonAmbiguousPartialOwnership)
	}
	result := seededstate.Reconcile(live, previousBaseline, desired)
	switch result.Classification {
	case seededstate.AlignedCurrent:
		action.Outcome = OutcomePreserve
	case seededstate.AdvanceBaseline:
		action.Outcome = OutcomeUpdate
	default:
		action.Outcome = OutcomeRetain
		action.Reason = ReasonLocalEvolution
	}
	return action
}

func classifyPartial(action Action, previous, current targetGroup, ownership string, live, desired []byte, currentEvidence []SourceEvidence, record state.Record, recorded bool) (Action, error) {
	if previous.target == "" {
		outcome, err := analyzePartialAddition(ownership, live, desired)
		if err != nil {
			return Action{}, fmt.Errorf("analyze new partial contribution: %w", err)
		}
		if outcome == OutcomeBlocked {
			return blocked(action, ReasonAmbiguousPartialOwnership), nil
		}
		action.Outcome = outcome
		return action, nil
	}
	if !recorded || !recordHasExactContributions(record, action.PreviousSources, previous.entries[0].Entry.Strategy, ownership) {
		if current.target == "" {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonAmbiguousPartialOwnership
			return action, nil
		}
		return blocked(action, ReasonAmbiguousPartialOwnership), nil
	}
	currentContents := make([][]byte, len(currentEvidence))
	for i := range currentEvidence {
		currentContents[i] = currentEvidence[i].Content
	}
	if record.PendingReconciliationMatches(live, previous.entries[0].Entry.Strategy, ownership, action.CurrentSources, currentContents) {
		action.Outcome = OutcomePreserve
		return action, nil
	}
	previousOwned, ok, err := exactPreviousContent(previous, ownership, record)
	if err != nil {
		return Action{}, fmt.Errorf("compose exact previous contribution: %w", err)
	}
	if !ok {
		if current.target == "" {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonAmbiguousPartialOwnership
			return action, nil
		}
		return blocked(action, ReasonAmbiguousPartialOwnership), nil
	}

	if current.target == "" {
		changed, empty, compatible, removeErr := removePartial(ownership, live, previousOwned)
		if removeErr != nil {
			return Action{}, fmt.Errorf("analyze partial retirement: %w", removeErr)
		}
		if !compatible {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonAmbiguousPartialOwnership
			return action, nil
		}
		if !changed {
			action.Outcome = OutcomeRetain
			action.Reason = ReasonAmbiguousPartialOwnership
		} else if empty {
			action.Outcome = OutcomeRemove
		} else {
			action.Outcome = OutcomeRetain
		}
		return action, nil
	}

	changed, compatible, reconcileErr := reconcilePartial(ownership, live, previousOwned, desired)
	if reconcileErr != nil {
		return Action{}, fmt.Errorf("reconcile partial contribution: %w", reconcileErr)
	}
	if !compatible {
		return blocked(action, ReasonAmbiguousPartialOwnership), nil
	}
	if changed {
		action.Outcome = OutcomeReconcile
	} else {
		action.Outcome = OutcomePreserve
	}
	return action, nil
}

func analyzePartialAddition(ownership string, live, desired []byte) (Outcome, error) {
	switch ownership {
	case "json-subset":
		relation, err := configsubset.AnalyzeJSON(live, desired)
		return relationOutcome(relation), err
	case "jsonc-subset":
		relation, err := configsubset.AnalyzeJSONC(live, desired)
		return relationOutcome(relation), err
	case "toml-subset":
		relation, err := configsubset.AnalyzeTOML(live, desired)
		return relationOutcome(relation), err
	case "marked-block":
		reconciliation := textblock.ReconcileOwned(live, desired, desired, textblock.DotsManagedMarkers())
		if reconciliation.Compatible && !reconciliation.Changed {
			return OutcomePreserve, nil
		}
		return OutcomeBlocked, nil
	default:
		return "", fmt.Errorf("partial ownership %q is not supported", ownership)
	}
}

func relationOutcome(relation configsubset.JSONFileRelation) Outcome {
	if relation.Contains {
		return OutcomePreserve
	}
	if relation.Mergeable {
		return OutcomeReconcile
	}
	return OutcomeBlocked
}

func reconcilePartial(ownership string, live, previous, current []byte) (bool, bool, error) {
	switch ownership {
	case "json-subset":
		result, err := configsubset.ReconcileJSON(live, previous, current)
		return result.Changed, result.Compatible, err
	case "jsonc-subset":
		result, err := configsubset.ReconcileJSONC(live, previous, current)
		return result.Changed, result.Compatible, err
	case "toml-subset":
		result, err := configsubset.ReconcileTOML(live, previous, current)
		return result.Changed, result.Compatible, err
	case "marked-block":
		result := textblock.ReconcileOwned(live, previous, current, textblock.DotsManagedMarkers())
		return result.Changed, result.Compatible, nil
	default:
		return false, false, fmt.Errorf("partial ownership %q is not supported", ownership)
	}
}

func removePartial(ownership string, live, owned []byte) (bool, bool, bool, error) {
	switch ownership {
	case "json-subset":
		_, changed, empty, compatible, err := configsubset.RemoveJSON(live, owned)
		return changed, empty, compatible, err
	case "jsonc-subset":
		_, changed, empty, compatible, err := configsubset.RemoveJSONC(live, owned)
		return changed, empty, compatible, err
	case "toml-subset":
		_, changed, empty, compatible, err := configsubset.RemoveTOML(live, owned)
		return changed, empty, compatible, err
	case "marked-block":
		_, changed, empty, compatible := textblock.RemoveOwned(live, owned, textblock.DotsManagedMarkers())
		return changed, empty, compatible, nil
	default:
		return false, false, false, fmt.Errorf("partial ownership %q is not supported", ownership)
	}
}

func exactPreviousContent(group targetGroup, ownership string, record state.Record) ([]byte, bool, error) {
	contributions := make([][]byte, 0, len(group.entries))
	seen := make(map[string]bool)
	for _, entry := range group.entries {
		if seen[entry.Source] {
			continue
		}
		seen[entry.Source] = true
		contribution, ok := exactContribution(record.Contributions, entry.Source, ownership)
		if !ok {
			return nil, false, nil
		}
		switch ownership {
		case "json-subset", "jsonc-subset":
			contributions = append(contributions, cloneBytes(contribution.OwnedContent))
		case "toml-subset", "marked-block":
			contributions = append(contributions, cloneBytes(contribution.OwnedBytes))
		default:
			return nil, false, fmt.Errorf("partial ownership %q is not supported", ownership)
		}
	}
	return composeContents(ownership, contributions)
}

func composeGroup(group targetGroup, ownership string, evidence []SourceEvidence) ([]byte, []SourceEvidence, error) {
	if group.target == "" {
		return nil, make([]SourceEvidence, 0), nil
	}
	contents := make([][]byte, 0, len(group.entries))
	selected := make([]SourceEvidence, 0, len(group.entries))
	seen := make(map[string]bool)
	for _, entry := range group.entries {
		if seen[entry.Source] {
			continue
		}
		seen[entry.Source] = true
		source, ok := findSourceEvidence(evidence, group.target, entry.Source)
		if !ok || !source.Exists {
			return nil, nil, fmt.Errorf("source evidence for %s: %w", entry.Source, errMissingSourceEvidence)
		}
		selected = append(selected, cloneSourceEvidence(source))
		contents = append(contents, cloneBytes(source.Content))
	}
	if ownership == "whole" || ownership == "seeded" {
		if len(contents) != 1 {
			return nil, nil, fmt.Errorf("ownership %s requires exactly one source", ownership)
		}
		return contents[0], selected, nil
	}
	content, ok, err := composeContents(ownership, contents)
	if !ok {
		return nil, nil, fmt.Errorf("compose %s contribution: exact content is unavailable", ownership)
	}
	return content, selected, err
}

func composeContents(ownership string, contents [][]byte) ([]byte, bool, error) {
	if len(contents) == 0 {
		return nil, false, nil
	}
	if len(contents) == 1 {
		return cloneBytes(contents[0]), true, nil
	}
	if ownership != "json-subset" {
		return nil, false, fmt.Errorf("multiple contributions require json-subset ownership")
	}
	composed := cloneBytes(contents[0])
	for _, content := range contents[1:] {
		var err error
		composed, err = configsubset.MergeJSON(composed, content)
		if err != nil {
			return nil, false, fmt.Errorf("merge contribution: %w", err)
		}
	}
	return composed, true, nil
}

func groupTargets(entries []selectedsurface.SelectedEntry) ([]targetGroup, error) {
	groups := make([]targetGroup, 0)
	indexes := make(map[string]int)
	for _, entry := range entries {
		if entry.Entry.Target == "" {
			return nil, fmt.Errorf("managed entry target is empty")
		}
		if index, ok := indexes[entry.Entry.Target]; ok {
			groups[index].entries = append(groups[index].entries, entry)
			continue
		}
		indexes[entry.Entry.Target] = len(groups)
		groups = append(groups, targetGroup{target: entry.Entry.Target, entries: []selectedsurface.SelectedEntry{entry}})
	}
	return groups, nil
}

func groupMode(group targetGroup) (string, string, error) {
	if len(group.entries) == 0 {
		return "", "", fmt.Errorf("managed target has no selected entries")
	}
	strategy := group.entries[0].Entry.Strategy
	ownership := normalizedOwnership(group.entries[0].Entry.Ownership)
	for _, entry := range group.entries[1:] {
		if entry.Entry.Strategy != strategy || normalizedOwnership(entry.Entry.Ownership) != ownership {
			return "", "", fmt.Errorf("shared target has incompatible strategy or ownership")
		}
	}
	return strategy, ownership, nil
}

func groupSources(group targetGroup) []string {
	result := make([]string, 0, len(group.entries))
	seen := make(map[string]bool)
	for _, entry := range group.entries {
		if !seen[entry.Source] {
			seen[entry.Source] = true
			result = append(result, entry.Source)
		}
	}
	return result
}

func hasRetiredSources(previous, current targetGroup) bool {
	if previous.target == "" {
		return false
	}
	return len(difference(groupSources(previous), groupSources(current))) > 0
}

func classifyForward(action Action, ownership string, status ForwardStatus, reason string) Action {
	switch status {
	case ForwardCreate:
		action.Outcome = OutcomeCreate
	case ForwardUpdate:
		if ownership == "json-subset" || ownership == "jsonc-subset" || ownership == "toml-subset" || ownership == "marked-block" {
			action.Outcome = OutcomeReconcile
		} else {
			action.Outcome = OutcomeUpdate
		}
	case ForwardUnchanged:
		action.Outcome = OutcomePreserve
	case ForwardConflict:
		switch reason {
		case ReasonWholeTargetDrift, ReasonLostOwnership, ReasonAmbiguousPartialOwnership:
			return blocked(action, reason)
		default:
			return blocked(action, ReasonLostOwnership)
		}
	case ForwardMissingSource:
		return blocked(action, ReasonMissingSource)
	default:
		return blocked(action, ReasonLostOwnership)
	}
	return action
}

func exactContribution(contributions []state.Contribution, source, ownership string) (state.Contribution, bool) {
	for _, contribution := range contributions {
		if contribution.Source == source && contribution.EvidenceRecorded && normalizedOwnership(contribution.Ownership) == ownership {
			return contribution, true
		}
	}
	return state.Contribution{}, false
}

func exactSeededBaseline(record state.Record, sources []string) ([]byte, bool) {
	if len(sources) != 1 {
		return nil, false
	}
	contribution, ok := exactContribution(record.Contributions, sources[0], "seeded")
	if !ok {
		return nil, false
	}
	return cloneBytes(contribution.SeededBaseline), true
}

func recordHasExactContributions(record state.Record, sources []string, strategy, ownership string) bool {
	if len(sources) == 0 || record.Strategy != strategy || record.Ownership != ownership || len(record.Contributions) != len(sources) {
		return false
	}
	if len(sources) > 1 && ownership != "json-subset" {
		return false
	}
	for index, source := range sources {
		contribution := record.Contributions[index]
		if contribution.Source != source || !contribution.EvidenceRecorded || contribution.Ownership != ownership {
			return false
		}
	}
	return true
}

func findTargetEvidence(evidence []TargetEvidence, target string) (TargetEvidence, bool) {
	for _, candidate := range evidence {
		if candidate.DeclarativeTarget == target {
			candidate.Content = cloneBytes(candidate.Content)
			return candidate, true
		}
	}
	return TargetEvidence{}, false
}

func findSourceEvidence(evidence []SourceEvidence, target, source string) (SourceEvidence, bool) {
	for _, candidate := range evidence {
		if candidate.DeclarativeTarget == target && candidate.Source == source {
			return candidate, true
		}
	}
	return SourceEvidence{}, false
}

func resolvedSource(evidence []SourceEvidence, target, source string) string {
	resolved, ok := findSourceEvidence(evidence, target, source)
	if !ok {
		return ""
	}
	return resolved.ResolvedSource
}

func findRecord(records []state.Record, target string) (state.Record, bool) {
	for _, record := range records {
		if record.Target == target {
			return record, true
		}
	}
	return state.Record{}, false
}

func dependencyNames(dependencies []manifest.Dependency) []string {
	result := make([]string, 0, len(dependencies))
	for _, dependency := range dependencies {
		result = append(result, dependency.Name)
	}
	return orderedUnique(result)
}

func difference(left, right []string) []string {
	excluded := make(map[string]bool, len(right))
	for _, value := range right {
		excluded[value] = true
	}
	result := make([]string, 0)
	for _, value := range left {
		if !excluded[value] {
			result = append(result, value)
		}
	}
	return result
}

func orderedUnique(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func newNamedAction(scope Scope, outcome Outcome, names []string) Action {
	return Action{
		Scope:           scope,
		Outcome:         outcome,
		PreviousSources: make([]string, 0),
		CurrentSources:  make([]string, 0),
		Names:           append(make([]string, 0, len(names)), names...),
	}
}

func blocked(action Action, reason string) Action {
	action.Outcome = OutcomeBlocked
	action.Reason = reason
	return action
}

func normalizedOwnership(ownership string) string {
	if ownership == "" {
		return "whole"
	}
	return ownership
}

func missingSource(err error) bool {
	return errors.Is(err, errMissingSourceEvidence)
}

func cloneIntent(intent Intent) Intent {
	intent.Profiles = append(make([]string, 0, len(intent.Profiles)), intent.Profiles...)
	intent.ExtraTags = append(make([]string, 0, len(intent.ExtraTags)), intent.ExtraTags...)
	intent.ResolvedTags = append(make([]string, 0, len(intent.ResolvedTags)), intent.ResolvedTags...)
	return intent
}

func cloneSourceEvidence(source SourceEvidence) SourceEvidence {
	source.Content = cloneBytes(source.Content)
	return source
}

func cloneBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
