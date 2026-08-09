package plan

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/seededstate"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/textblock"
)

// Status describes what installing an Action would do to its target.
type Status string

const (
	StatusCreate        Status = "create"
	StatusUpdate        Status = "update"
	StatusMigrate       Status = "migrate"
	StatusConflict      Status = "conflict"
	StatusUnchanged     Status = "unchanged"
	StatusMissingSource Status = "missing-source"
)

const ConflictReasonSourceOverrideNotSelected = "source-override-not-selected"

const ReasonSeededLocalEvolution = "seeded-local-evolution"

// Action is a single planned filesystem change for a Managed Entry.
type Action struct {
	Source string `json:"source"`
	// Sources lists every Source of Truth contribution when compatible
	// json-subset Managed Entries compose into one physical target operation.
	Sources []string `json:"sources,omitempty"`
	// ResolvedSource is an absolute, machine-local path and is therefore kept out
	// of the Agent Output Contract: it differs between machines and install
	// locations, which would break idempotent envelope comparisons. Agents key on
	// the portable, manifest-relative Source instead.
	ResolvedSource  string   `json:"-"`
	ResolvedSources []string `json:"-"`
	// Content is the conservatively composed baseline for a shared json-subset
	// target. Apply writes or merges it once instead of mutating per contributor.
	Content []byte `json:"-"`
	// PreviousContent is the last recorded dots-owned JSON contribution. It is
	// used only to revalidate a reversible update at apply time.
	PreviousContent []byte   `json:"-"`
	Target          string   `json:"target"`
	TargetRoot      string   `json:"target_root,omitempty"`
	Strategy        string   `json:"strategy"`
	Status          Status   `json:"status"`
	Reason          string   `json:"reason,omitempty"`
	MatchingTags    []string `json:"matching_tags,omitempty"`
	Ownership       string   `json:"-"`
	// Migration carries pre-refresh evidence for an ownership-changing legacy
	// symlink. It is intentionally excluded from machine output; status is the
	// portable contract and captured workstation content may be sensitive.
	Migration *LegacyMigration `json:"-"`
	// LegacyParent is a proven directory symlink removed by an earlier migrate
	// action in the same plan. It lets apply validate a later child create
	// without treating the pre-removal view as an unrelated conflict.
	LegacyParent string `json:"-"`
}

// LegacyMigration is provenance-backed content captured before the Installed
// Repository checkout changed. FinalContent is the exact regular-file content
// apply may materialize after last-moment symlink and content revalidation.
type LegacyMigration struct {
	LinkDestination string
	// LegacyTarget is set when the captured content lived beneath a proven
	// directory symlink that must be removed before new native targets can be
	// materialized. Empty retains the single-file migration behavior.
	LegacyTarget          string
	LegacyContentTarget   string
	CapturedContent       []byte
	PreviousSourceContent []byte
	ExpectedLinkContent   []byte
	FinalContent          []byte
	RecordedBaseline      []byte
}

// Plan is the preview of changes the installer would apply for a Profile.
type Plan struct {
	Profile   string            `json:"profile,omitempty"`
	Profiles  []string          `json:"profiles,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Selection *selection.Report `json:"selection,omitempty"`
	Actions   []Action          `json:"actions"`
}

// HasFindings reports whether the Install Plan contains an action the caller
// must act on before a clean apply: an unresolved Conflict, or a missing source
// the manifest declares but the Installed Repository does not provide.
func (p Plan) HasFindings() bool {
	for _, a := range p.Actions {
		switch a.Status {
		case StatusConflict, StatusMissingSource:
			return true
		}
	}
	return false
}

// Options carries the resolved inputs needed to compute a Plan.
type Options struct {
	Profile   string
	Profiles  []string
	ExtraTags []string
	Selection *manifest.Selection
	OS        string
	// SourceRoot is the canonical Installed Repository path recorded in Actions.
	SourceRoot string
	// SourceReadRoot optionally supplies a snapshot used only to inspect source
	// existence and content. Empty defaults to SourceRoot.
	SourceReadRoot string
	Home           string
	// XDGStateHome is the resolved application state root. It is distinct from
	// dots' Installation Metadata state root and is required only by entries
	// whose allowlisted target_root is xdg-state.
	XDGStateHome     string
	Metadata         state.Metadata
	LegacyMigrations map[string]LegacyMigration
}

// Build computes the Install Plan for the selected Profile without mutating
// the filesystem. It selects Managed Entries whose tags intersect the
// Profile's tags and that pass the OS filter, then determines each target's
// status against the current workstation state.
func Build(m manifest.Manifest, opts Options) (Plan, error) {
	resolved := opts.Selection
	if resolved == nil {
		selection, err := manifest.ResolveSelection(m, manifest.SelectedProfileNames(opts.Profile, opts.Profiles), opts.ExtraTags)
		if err != nil {
			return Plan{}, err
		}
		resolved = &selection
	}
	tags := resolved.Tags
	readRoot := opts.SourceReadRoot
	if readRoot == "" {
		readRoot = opts.SourceRoot
	}

	plan := Plan{Profile: resolved.Profile, Profiles: resolved.Profiles, Tags: resolved.Tags}
	actionByTarget := map[string]int{}
	readSourcesByTarget := map[string][]string{}
	scheduledLegacyParents := map[string]struct{}{}
	for _, entry := range m.Entries {
		if !manifest.SharesTag(entry.Tags, tags) {
			continue
		}
		if !manifest.MatchesOS(entry.OS, opts.OS) {
			continue
		}

		defaultSource := entry.Source
		source := manifest.EntrySource(entry, tags)
		target, err := ResolveEntryTarget(entry, opts.Home, opts.XDGStateHome)
		if err != nil {
			return Plan{}, err
		}
		sourceAbs, err := ResolveSource(source, opts.SourceRoot)
		if err != nil {
			return Plan{}, err
		}
		readSourceAbs, err := ResolveSource(source, readRoot)
		if err != nil {
			return Plan{}, err
		}
		entry.Source = source
		actionStatus, err := status(entry, target, readSourceAbs, readRoot, sourceAbs, opts.Metadata, defaultSource)
		if err != nil {
			return Plan{}, err
		}
		legacyParent := ""
		if actionStatus == StatusConflict {
			legacyParent = scheduledLegacyParent(target, scheduledLegacyParents)
			if legacyParent != "" {
				actionStatus = StatusCreate
			}
		}
		var reason string
		if actionStatus == StatusUnchanged && entry.Ownership == "seeded" {
			local, localErr := seededLocalEvolution(entry, target, readSourceAbs, opts.Metadata)
			if localErr != nil {
				return Plan{}, localErr
			}
			if local {
				reason = ReasonSeededLocalEvolution
			}
		}
		var matchingTags []string
		if actionStatus == StatusConflict {
			matchingTags, err = matchingUnselectedSourceOverrideTags(entry, tags, target, readRoot, opts.SourceRoot)
			if err != nil {
				return Plan{}, err
			}
			if len(matchingTags) > 0 {
				reason = ConflictReasonSourceOverrideNotSelected
			}
		}
		action := Action{
			Source:         source,
			ResolvedSource: sourceAbs,
			Target:         target,
			TargetRoot:     entry.TargetRoot,
			Strategy:       entry.Strategy,
			Status:         actionStatus,
			Reason:         reason,
			MatchingTags:   matchingTags,
			Ownership:      entry.Ownership,
			LegacyParent:   legacyParent,
		}
		if actionStatus == StatusConflict || actionStatus == StatusCreate {
			if migration, ok := opts.LegacyMigrations[target]; ok && ((migration.LegacyTarget == "" && actionStatus == StatusConflict) || (migration.LegacyTarget != "" && actionStatus == StatusCreate)) {
				planned, compatible, migrateErr := planLegacyMigration(entry, readSourceAbs, migration)
				if migrateErr != nil {
					return Plan{}, migrateErr
				}
				if compatible {
					action.Status = StatusMigrate
					action.Migration = &planned
					action.Reason = ""
					action.MatchingTags = nil
					if planned.LegacyTarget != "" {
						scheduledLegacyParents[planned.LegacyTarget] = struct{}{}
					}
				}
			}
		}
		if rec, ok := opts.Metadata.FindByTarget(target); ok && rec.Ownership == action.Ownership {
			if isJSONOwnership(rec.Ownership) && len(rec.OwnedContent) > 0 {
				action.PreviousContent = append([]byte(nil), rec.OwnedContent...)
			} else if rec.Ownership == "toml-subset" && len(rec.OwnedBytes) > 0 {
				action.PreviousContent = append([]byte(nil), rec.OwnedBytes...)
			} else if rec.Ownership == "seeded" {
				action.PreviousContent = append([]byte(nil), rec.SeededBaseline...)
			} else if rec.Ownership == "marked-block" {
				action.PreviousContent = append([]byte(nil), rec.OwnedBytes...)
			}
		}
		targetKey := filepath.Clean(target)
		if index, ok := actionByTarget[targetKey]; ok {
			existing := &plan.Actions[index]
			if existing.Strategy != "copy" || existing.Ownership != "json-subset" || action.Strategy != "copy" || action.Ownership != "json-subset" {
				return Plan{}, fmt.Errorf("install plan contains duplicate target %s; only copy entries with json-subset ownership can compose", targetKey)
			}
			if len(existing.Sources) == 0 {
				existing.Sources = []string{existing.Source}
				existing.ResolvedSources = []string{existing.ResolvedSource}
			}
			existing.Sources = append(existing.Sources, action.Source)
			existing.ResolvedSources = append(existing.ResolvedSources, action.ResolvedSource)
			readSourcesByTarget[targetKey] = append(readSourcesByTarget[targetKey], readSourceAbs)
			composed, err := configsubset.ComposeJSONFiles(readSourcesByTarget[targetKey])
			if err != nil {
				if strings.Contains(err.Error(), "incompatible") {
					return Plan{}, fmt.Errorf("shared target %s has incompatible json-subset sources: %w", targetKey, err)
				}
				return Plan{}, fmt.Errorf("compose shared target %s: %w", targetKey, err)
			}
			existing.Content = composed
			if rec, ok := opts.Metadata.FindByTarget(existing.Target); ok && rec.Ownership == "json-subset" && len(rec.OwnedContent) > 0 {
				existing.PreviousContent = append([]byte(nil), rec.OwnedContent...)
			}
			existing.Status, err = composedJSONStatus(*existing, opts.Metadata)
			if err != nil {
				return Plan{}, err
			}
			existing.Reason = ""
			existing.MatchingTags = nil
			continue
		}
		actionByTarget[targetKey] = len(plan.Actions)
		readSourcesByTarget[targetKey] = []string{readSourceAbs}
		plan.Actions = append(plan.Actions, action)
	}

	return plan, nil
}

func planLegacyMigration(entry manifest.Entry, currentSource string, migration LegacyMigration) (LegacyMigration, bool, error) {
	if entry.Strategy != "copy" || migration.LinkDestination == "" {
		return LegacyMigration{}, false, nil
	}
	current, err := os.ReadFile(currentSource)
	if err != nil {
		return LegacyMigration{}, false, fmt.Errorf("read migration source %s: %w", currentSource, err)
	}
	planned := migration
	expectedPath := migration.LinkDestination
	if migration.LegacyContentTarget != "" {
		expectedPath = migration.LegacyContentTarget
	}
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		return LegacyMigration{}, false, fmt.Errorf("read current legacy target %s: %w", expectedPath, err)
	}
	planned.ExpectedLinkContent = expected
	switch entry.Ownership {
	case "seeded":
		reconciliation := seededstate.Reconcile(migration.CapturedContent, migration.PreviousSourceContent, current)
		if reconciliation.Changed {
			planned.FinalContent = reconciliation.Content
			planned.RecordedBaseline = append([]byte(nil), current...)
		} else {
			planned.FinalContent = append([]byte(nil), migration.CapturedContent...)
			if reconciliation.Classification == seededstate.LocalEvolution {
				planned.RecordedBaseline = append([]byte(nil), migration.PreviousSourceContent...)
			} else {
				planned.RecordedBaseline = append([]byte(nil), current...)
			}
		}
	case "json-subset":
		reconciliation, err := configsubset.ReconcileJSON(migration.CapturedContent, migration.PreviousSourceContent, current)
		if err != nil {
			return LegacyMigration{}, false, err
		}
		if !reconciliation.Compatible {
			return LegacyMigration{}, false, nil
		}
		planned.FinalContent = reconciliation.Content
	case "jsonc-subset":
		reconciliation, err := configsubset.ReconcileJSONC(migration.CapturedContent, migration.PreviousSourceContent, current)
		if err != nil {
			return LegacyMigration{}, false, err
		}
		if !reconciliation.Compatible {
			return LegacyMigration{}, false, nil
		}
		planned.FinalContent = reconciliation.Content
	case "toml-subset":
		reconciliation, err := configsubset.ReconcileTOML(migration.CapturedContent, migration.PreviousSourceContent, current)
		if err != nil {
			return LegacyMigration{}, false, err
		}
		if !reconciliation.Compatible {
			return LegacyMigration{}, false, nil
		}
		planned.FinalContent = reconciliation.Content
	case "marked-block":
		reconciliation := textblock.MigrateLegacyOwned(migration.CapturedContent, migration.PreviousSourceContent, current, textblock.DotsManagedMarkers())
		if !reconciliation.Compatible {
			return LegacyMigration{}, false, nil
		}
		planned.FinalContent = reconciliation.Content
	default:
		if !bytes.Equal(migration.CapturedContent, migration.PreviousSourceContent) {
			return LegacyMigration{}, false, nil
		}
		planned.FinalContent = append([]byte(nil), current...)
	}
	return planned, true, nil
}

func scheduledLegacyParent(target string, parents map[string]struct{}) string {
	for parent := range parents {
		if InsideRoot(target, parent) && filepath.Clean(target) != filepath.Clean(parent) {
			return parent
		}
	}
	return ""
}

func composedJSONStatus(action Action, meta state.Metadata) (Status, error) {
	info, err := os.Lstat(action.Target)
	if os.IsNotExist(err) {
		return StatusCreate, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return StatusConflict, nil
	}
	targetData, err := os.ReadFile(action.Target)
	if err != nil {
		return "", fmt.Errorf("read shared JSON target %s: %w", action.Target, err)
	}
	if bytes.Equal(targetData, action.Content) {
		return StatusUnchanged, nil
	}
	rec, ok := meta.FindByTarget(action.Target)
	recordedSources := rec.SourceList()
	trusted := ok && rec.Strategy == action.Strategy && len(recordedSources) > 0
	selectedSources := make(map[string]struct{}, len(action.Sources))
	for _, source := range action.Sources {
		selectedSources[source] = struct{}{}
	}
	for _, source := range recordedSources {
		_, selected := selectedSources[source]
		trusted = trusted && selected
	}
	if !trusted {
		return StatusConflict, nil
	}
	if rec.Ownership == "json-subset" && len(rec.OwnedContent) > 0 {
		reconciliation, err := configsubset.ReconcileJSON(targetData, rec.OwnedContent, action.Content)
		if err != nil {
			return "", fmt.Errorf("reconcile shared JSON target %s: %w", action.Target, err)
		}
		if !reconciliation.Compatible {
			return StatusConflict, nil
		}
		if reconciliation.Changed {
			return StatusUpdate, nil
		}
		return StatusUnchanged, nil
	}
	relation, err := configsubset.AnalyzeJSON(targetData, action.Content)
	if err != nil {
		return "", fmt.Errorf("analyze shared JSON target %s: %w", action.Target, err)
	}
	switch {
	case relation.Contains:
		return StatusUnchanged, nil
	case relation.Mergeable:
		return StatusUpdate, nil
	default:
		return StatusConflict, nil
	}
}

// MatchingUnselectedSourceOverrideTags returns the deterministic set of
// unselected Tags whose declared alternate source exactly matches target.
// Alternate sources receive the same containment and existence checks as a
// selected Managed Entry source before they can produce remediation guidance.
func MatchingUnselectedSourceOverrideTags(entry manifest.Entry, selectedTags []string, target, sourceRoot string) ([]string, error) {
	return matchingUnselectedSourceOverrideTags(entry, selectedTags, target, sourceRoot, sourceRoot)
}

func matchingUnselectedSourceOverrideTags(entry manifest.Entry, selectedTags []string, target, sourceReadRoot, sourceRoot string) ([]string, error) {
	if len(entry.SourceOverrides) == 0 || (entry.Strategy != "symlink" && entry.Strategy != "copy") {
		return nil, nil
	}

	selected := make(map[string]struct{}, len(selectedTags))
	for _, tag := range selectedTags {
		selected[tag] = struct{}{}
	}
	overrideTags := make([]string, 0, len(entry.SourceOverrides))
	for tag := range entry.SourceOverrides {
		if _, ok := selected[tag]; !ok {
			overrideTags = append(overrideTags, tag)
		}
	}
	sort.Strings(overrideTags)

	matching := make([]string, 0, len(overrideTags))
	for _, tag := range overrideTags {
		sourceAbs, err := ResolveSource(entry.SourceOverrides[tag], sourceReadRoot)
		if err != nil {
			return nil, err
		}
		exists, err := safeSourceExists(sourceAbs, sourceReadRoot)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		canonicalSourceAbs, err := ResolveSource(entry.SourceOverrides[tag], sourceRoot)
		if err != nil {
			return nil, err
		}
		matches, err := targetMatchesSource(entry.Strategy, target, sourceAbs, canonicalSourceAbs)
		if err != nil {
			return nil, err
		}
		if matches {
			matching = append(matching, tag)
		}
	}
	return matching, nil
}

func targetMatchesSource(strategy, target, sourceAbs, canonicalSourceAbs string) (bool, error) {
	info, err := os.Lstat(target)
	if err != nil {
		return false, nil
	}
	switch strategy {
	case "symlink":
		if info.Mode()&os.ModeSymlink == 0 {
			return false, nil
		}
		dest, err := os.Readlink(target)
		if err != nil {
			return false, fmt.Errorf("readlink target %s: %w", target, err)
		}
		return dest == canonicalSourceAbs, nil
	case "copy":
		if !info.Mode().IsRegular() {
			return false, nil
		}
		return sameContent(target, sourceAbs)
	default:
		return false, nil
	}
}

// ResolveTarget resolves a manifest target inside home. Targets are deliberately
// limited to "~" and "~/" forms so a manifest cannot escape a sandboxed --home.
func ResolveTarget(target, home string) (string, error) {
	resolvedHome, err := filepath.Abs(home)
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	resolvedHome = filepath.Clean(resolvedHome)

	if target == "~" {
		return resolvedHome, nil
	}
	if !strings.HasPrefix(target, "~/") {
		return "", fmt.Errorf("unsafe target %q: target must be ~ or ~/...", target)
	}
	resolvedTarget := filepath.Clean(filepath.Join(resolvedHome, target[2:]))
	if !InsideRoot(resolvedTarget, resolvedHome) {
		return "", fmt.Errorf("unsafe target %q: resolved target escapes home %q", target, resolvedHome)
	}
	return resolvedTarget, nil
}

// ResolveEntryTarget resolves a Managed Entry against its allowlisted target
// root. Home targets retain the ~/... contract; xdg-state targets must be local
// relative paths beneath the already resolved XDG state home.
func ResolveEntryTarget(entry manifest.Entry, home, xdgStateHome string) (string, error) {
	if entry.TargetRoot == "" {
		return ResolveTarget(entry.Target, home)
	}
	if entry.TargetRoot != "xdg-state" {
		return "", fmt.Errorf("unsupported target root %q", entry.TargetRoot)
	}
	if xdgStateHome == "" {
		return "", fmt.Errorf("xdg-state target %q requires an XDG state home", entry.Target)
	}
	if !filepath.IsAbs(xdgStateHome) {
		return "", fmt.Errorf("unsafe XDG state home %q: XDG_STATE_HOME must be absolute", xdgStateHome)
	}
	if filepath.IsAbs(entry.Target) || !filepath.IsLocal(entry.Target) || entry.Target == "." {
		return "", fmt.Errorf("unsafe xdg-state target %q: target must be a confined relative path", entry.Target)
	}
	root, err := filepath.Abs(xdgStateHome)
	if err != nil {
		return "", fmt.Errorf("resolve XDG state home: %w", err)
	}
	root = filepath.Clean(root)
	if err := ValidateResolvedTarget(root, home); err != nil {
		return "", fmt.Errorf("unsafe XDG state home %q: %w", root, err)
	}
	target := filepath.Clean(filepath.Join(root, entry.Target))
	if !InsideRoot(target, root) {
		return "", fmt.Errorf("unsafe xdg-state target %q: resolved target escapes XDG state home %q", entry.Target, root)
	}
	if err := ValidateResolvedTarget(target, home); err != nil {
		return "", err
	}
	if err := ValidateTargetParentInsideHome(target, home); err != nil {
		return "", err
	}
	return target, nil
}

// ValidateResolvedTarget verifies that an already-resolved plan target remains
// inside home before install writes to it.
func ValidateResolvedTarget(target, home string) error {
	resolvedHome, err := filepath.Abs(home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	resolvedTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target %q: %w", target, err)
	}
	resolvedHome = filepath.Clean(resolvedHome)
	resolvedTarget = filepath.Clean(resolvedTarget)
	if !InsideRoot(resolvedTarget, resolvedHome) {
		return fmt.Errorf("unsafe target %q: resolved target escapes home %q", target, resolvedHome)
	}
	return nil
}

// ValidateTargetParentInsideHome verifies that a resolved target's existing
// parent path does not escape home through symlinks before callers inspect or
// mutate target contents.
func ValidateTargetParentInsideHome(target, home string) error {
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target %s: %w", target, err)
	}
	return ValidatePathInsideHomeNoSymlinkEscape(filepath.Dir(targetAbs), home, "target parent")
}

// ValidatePathInsideHomeNoSymlinkEscape verifies that path is lexically inside
// home and that every existing component resolves through symlinks to a path
// still contained by home. Missing suffixes are safe because they cannot yet
// redirect filesystem reads or writes.
func ValidatePathInsideHomeNoSymlinkEscape(path, home, label string) error {
	return validatePathInsideHomeNoSymlinkEscape(path, home, label, false)
}

// ValidateFilePathInsideHomeNoSymlinkEscape verifies that a file path is
// lexically inside home, existing parent components do not escape home through
// symlinks, and an existing file leaf is not a symlink. A missing file leaf is
// safe because callers may create it after validating the parent path.
func ValidateFilePathInsideHomeNoSymlinkEscape(path, home, label string) error {
	return validatePathInsideHomeNoSymlinkEscape(path, home, label, true)
}

func validatePathInsideHomeNoSymlinkEscape(path, home, label string, finalMayBeFile bool) error {
	homeAbs, err := filepath.Abs(home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %s %s: %w", label, path, err)
	}
	homeAbs = filepath.Clean(homeAbs)
	pathAbs = filepath.Clean(pathAbs)

	if !InsideRoot(pathAbs, homeAbs) {
		return fmt.Errorf("unsafe %s %q: resolved path escapes home %q", label, pathAbs, homeAbs)
	}

	homeReal, err := filepath.EvalSymlinks(homeAbs)
	if err != nil {
		return fmt.Errorf("resolve real home %s: %w", homeAbs, err)
	}
	homeReal = filepath.Clean(homeReal)

	rel, err := filepath.Rel(homeAbs, pathAbs)
	if err != nil {
		return fmt.Errorf("resolve %s %s relative to home %s: %w", label, pathAbs, homeAbs, err)
	}
	current := homeAbs
	parts := splitPath(rel)
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("stat %s %s: %w", label, current, err)
		}
		isFinal := i == len(parts)-1
		if info.Mode()&os.ModeSymlink == 0 && !info.IsDir() {
			if finalMayBeFile && isFinal && info.Mode().IsRegular() {
				return nil
			}
			return fmt.Errorf("%s %s is not a directory", label, current)
		}
		if finalMayBeFile && isFinal && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe %s %q: file leaf must not be a symlink", label, current)
		}
		realPath, err := filepath.EvalSymlinks(current)
		if err != nil {
			return fmt.Errorf("resolve %s %s: %w", label, current, err)
		}
		if !InsideRoot(realPath, homeReal) {
			return fmt.Errorf("unsafe %s %q: symlink resolves outside home %q", label, current, homeAbs)
		}
	}
	return nil
}

func splitPath(path string) []string {
	parts := []string{}
	separator := string(filepath.Separator)
	for _, part := range strings.Split(path, separator) {
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	return parts
}

// ResolveSource resolves a manifest source inside sourceRoot. Sources must be
// repository-relative so a manifest cannot escape a sandboxed --source-root.
func ResolveSource(source, sourceRoot string) (string, error) {
	if filepath.IsAbs(source) {
		return "", fmt.Errorf("unsafe source %q: source must be relative", source)
	}
	resolvedRoot, err := filepath.Abs(sourceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	resolvedRoot = filepath.Clean(resolvedRoot)
	resolvedSource := filepath.Clean(filepath.Join(resolvedRoot, source))
	if !InsideRoot(resolvedSource, resolvedRoot) {
		return "", fmt.Errorf("unsafe source %q: resolved source escapes source root %q", source, resolvedRoot)
	}
	return resolvedSource, nil
}

// InsideRoot reports whether path is root or is contained beneath root.
func InsideRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func status(entry manifest.Entry, target, sourceAbs, sourceRoot, canonicalSourceAbs string, meta state.Metadata, defaultSource string) (Status, error) {
	// The managed source must exist before any target comparison is
	// meaningful: a missing source cannot be installed, and a symlink that
	// still points at a deleted source is broken, not unchanged.
	if exists, err := safeSourceExists(sourceAbs, sourceRoot); err != nil {
		return "", err
	} else if !exists {
		return StatusMissingSource, nil
	}
	if entry.Ownership == "marked-block" {
		source, err := os.ReadFile(sourceAbs)
		if err != nil {
			return "", fmt.Errorf("read marked-block source %s: %w", sourceAbs, err)
		}
		if !textblock.ValidOwnedSource(source, textblock.DotsManagedMarkers()) {
			return "", fmt.Errorf("marked-block source %s must contain exactly one complete block and no external content", sourceAbs)
		}
	}

	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return StatusCreate, nil
	}
	if err != nil {
		return StatusConflict, nil
	}

	switch entry.Strategy {
	case "symlink":
		if info.Mode()&os.ModeSymlink == 0 {
			return StatusConflict, nil
		}
		dest, err := os.Readlink(target)
		if err != nil || dest != canonicalSourceAbs {
			return StatusConflict, nil
		}
		return StatusUnchanged, nil
	case "copy":
		if !info.Mode().IsRegular() {
			return StatusConflict, nil
		}
		if same, err := sameContent(target, sourceAbs); err != nil || !same {
			if entry.Ownership == "marked-block" {
				rec, ok := meta.FindByTarget(target)
				if !ok || rec.Strategy != entry.Strategy || rec.Source != entry.Source || rec.Ownership != "marked-block" || len(rec.OwnedBytes) == 0 {
					return StatusConflict, nil
				}
				live, readErr := os.ReadFile(target)
				if readErr != nil {
					return "", fmt.Errorf("read marked-block target %s: %w", target, readErr)
				}
				current, readErr := os.ReadFile(sourceAbs)
				if readErr != nil {
					return "", fmt.Errorf("read marked-block source %s: %w", sourceAbs, readErr)
				}
				reconciliation := textblock.ReconcileOwned(live, rec.OwnedBytes, current, textblock.DotsManagedMarkers())
				if !reconciliation.Compatible {
					return StatusConflict, nil
				}
				if reconciliation.Changed {
					return StatusUpdate, nil
				}
				return StatusUnchanged, nil
			}
			if entry.Ownership == "seeded" {
				rec, ok := meta.FindByTarget(target)
				if !ok || rec.Strategy != entry.Strategy || rec.Source != entry.Source || rec.Ownership != "seeded" {
					return StatusConflict, nil
				}
				live, readErr := os.ReadFile(target)
				if readErr != nil {
					return "", fmt.Errorf("read seeded target %s: %w", target, readErr)
				}
				current, readErr := os.ReadFile(sourceAbs)
				if readErr != nil {
					return "", fmt.Errorf("read seeded source %s: %w", sourceAbs, readErr)
				}
				if seededstate.Reconcile(live, rec.SeededBaseline, current).Classification == seededstate.AdvanceBaseline {
					return StatusUpdate, nil
				}
				return StatusUnchanged, nil
			}
			if isSubsetOwned(entry.Ownership) && (meta.MatchesEntry(target, entry.Source, entry.Strategy) || meta.MatchesEntry(target, defaultSource, entry.Strategy)) {
				if isJSONOwnership(entry.Ownership) {
					if rec, ok := meta.FindByTarget(target); ok && rec.Ownership == entry.Ownership && len(rec.OwnedContent) > 0 {
						targetData, readErr := os.ReadFile(target)
						if readErr != nil {
							return "", fmt.Errorf("read JSON target %s: %w", target, readErr)
						}
						currentData, readErr := os.ReadFile(sourceAbs)
						if readErr != nil {
							return "", fmt.Errorf("read source JSON %s: %w", sourceAbs, readErr)
						}
						var reconciliation configsubset.JSONReconciliation
						var reconcileErr error
						if entry.Ownership == "jsonc-subset" {
							reconciliation, reconcileErr = configsubset.ReconcileJSONC(targetData, rec.OwnedContent, currentData)
						} else {
							reconciliation, reconcileErr = configsubset.ReconcileJSON(targetData, rec.OwnedContent, currentData)
						}
						if reconcileErr != nil {
							return "", fmt.Errorf("reconcile JSON target %s: %w", target, reconcileErr)
						}
						if reconciliation.Compatible {
							if reconciliation.Changed {
								return StatusUpdate, nil
							}
							return StatusUnchanged, nil
						}
						return StatusConflict, nil
					}
					var relation configsubset.JSONFileRelation
					var relationErr error
					if entry.Ownership == "jsonc-subset" {
						targetData, readErr := os.ReadFile(target)
						if readErr != nil {
							return "", fmt.Errorf("read JSONC target %s: %w", target, readErr)
						}
						sourceData, readErr := os.ReadFile(sourceAbs)
						if readErr != nil {
							return "", fmt.Errorf("read source JSONC %s: %w", sourceAbs, readErr)
						}
						relation, relationErr = configsubset.AnalyzeJSONC(targetData, sourceData)
					} else {
						relation, relationErr = configsubset.AnalyzeJSONFiles(target, sourceAbs)
					}
					if relationErr != nil {
						return "", relationErr
					}
					if relation.Contains {
						return StatusUnchanged, nil
					}
					if relation.Mergeable {
						return StatusUpdate, nil
					}
				} else if entry.Ownership == "toml-subset" {
					if rec, ok := meta.FindByTarget(target); ok && rec.Ownership == "toml-subset" && len(rec.OwnedBytes) > 0 {
						targetData, readErr := os.ReadFile(target)
						if readErr != nil {
							return "", fmt.Errorf("read TOML target %s: %w", target, readErr)
						}
						currentData, readErr := os.ReadFile(sourceAbs)
						if readErr != nil {
							return "", fmt.Errorf("read source TOML %s: %w", sourceAbs, readErr)
						}
						reconciliation, reconcileErr := configsubset.ReconcileTOML(targetData, rec.OwnedBytes, currentData)
						if reconcileErr != nil {
							return "", fmt.Errorf("reconcile TOML target %s: %w", target, reconcileErr)
						}
						if reconciliation.Compatible {
							if reconciliation.Changed {
								return StatusUpdate, nil
							}
							return StatusUnchanged, nil
						}
						return StatusConflict, nil
					}
					subset, subsetErr := subsetContent(entry.Ownership, target, sourceAbs)
					if subsetErr != nil {
						return "", subsetErr
					}
					if subset {
						return StatusUnchanged, nil
					}
				} else {
					subset, subsetErr := subsetContent(entry.Ownership, target, sourceAbs)
					if subsetErr != nil {
						return "", subsetErr
					}
					if subset {
						return StatusUnchanged, nil
					}
				}
			}
			if entry.Ownership == "toml-subset" && targetContainsCompatibleRecordedSource(entry, target, sourceRoot, meta, defaultSource) {
				return StatusUpdate, nil
			}
			return StatusConflict, nil
		}
		return StatusUnchanged, nil
	default:
		return StatusConflict, nil
	}
}

func seededLocalEvolution(entry manifest.Entry, target, sourceAbs string, meta state.Metadata) (bool, error) {
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Strategy != entry.Strategy || rec.Source != entry.Source || rec.Ownership != "seeded" {
		return false, nil
	}
	live, err := os.ReadFile(target)
	if err != nil {
		return false, fmt.Errorf("read seeded target %s: %w", target, err)
	}
	current, err := os.ReadFile(sourceAbs)
	if err != nil {
		return false, fmt.Errorf("read seeded source %s: %w", sourceAbs, err)
	}
	return seededstate.Reconcile(live, rec.SeededBaseline, current).Classification == seededstate.LocalEvolution, nil
}

func targetContainsCompatibleRecordedSource(entry manifest.Entry, target, sourceRoot string, meta state.Metadata, defaultSource string) bool {
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Strategy != entry.Strategy || !compatibleEntrySource(entry, defaultSource, rec.Source) {
		return false
	}
	sourceAbs, err := ResolveSource(rec.Source, sourceRoot)
	if err != nil {
		return false
	}
	if err := ValidateResolvedSource(sourceAbs, sourceRoot); err != nil {
		return false
	}
	subset, err := subsetContent(entry.Ownership, target, sourceAbs)
	return err == nil && subset
}

func compatibleEntrySource(entry manifest.Entry, defaultSource, source string) bool {
	if source == entry.Source || source == defaultSource {
		return true
	}
	for _, override := range entry.SourceOverrides {
		if source == override {
			return true
		}
	}
	return false
}

func safeSourceExists(sourceAbs, sourceRoot string) (bool, error) {
	if err := ValidateResolvedSource(sourceAbs, sourceRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ValidateResolvedSource verifies that an already-resolved source path exists
// and resolves through symlinks to a file contained by sourceRoot.
func ValidateResolvedSource(sourceAbs, sourceRoot string) error {
	sourceReal, err := filepath.EvalSymlinks(sourceAbs)
	if err != nil {
		return fmt.Errorf("resolve source %q: %w", sourceAbs, err)
	}
	rootReal, err := filepath.EvalSymlinks(sourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source root %q: %w", sourceRoot, err)
	}
	sourceReal = filepath.Clean(sourceReal)
	rootReal = filepath.Clean(rootReal)
	if !InsideRoot(sourceReal, rootReal) {
		return fmt.Errorf("unsafe source %q: symlink resolves outside source root %q", sourceAbs, sourceRoot)
	}
	return nil
}

func sameContent(a, b string) (bool, error) {
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(da, db), nil
}

func isSubsetOwned(ownership string) bool {
	return isJSONOwnership(ownership) || ownership == "toml-subset"
}

func isJSONOwnership(ownership string) bool {
	return ownership == "json-subset" || ownership == "jsonc-subset"
}

func subsetContent(ownership, target, sourceAbs string) (bool, error) {
	switch ownership {
	case "json-subset":
		return configsubset.JSONFileContains(target, sourceAbs)
	case "jsonc-subset":
		targetData, err := os.ReadFile(target)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", target, err)
		}
		sourceData, err := os.ReadFile(sourceAbs)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", sourceAbs, err)
		}
		relation, err := configsubset.AnalyzeJSONC(targetData, sourceData)
		return relation.Contains, err
	case "toml-subset":
		return configsubset.TOMLFileContains(target, sourceAbs)
	default:
		return false, nil
	}
}
