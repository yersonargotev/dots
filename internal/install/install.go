package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/configsubset"
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

// Apply performs safe filesystem changes described by an Install Plan.
func Apply(p plan.Plan, opts Options) error {
	resolvedSources, err := validatePlan(p, opts)
	if err != nil {
		return err
	}

	for i, action := range p.Actions {
		source := resolvedSources[i][0]
		switch action.Status {
		case plan.StatusUnchanged:
			continue
		case plan.StatusCreate:
			if err := applyCreate(action, source); err != nil {
				return err
			}
		case plan.StatusUpdate:
			if err := applyUpdate(action, source, opts); err != nil {
				return err
			}
		case plan.StatusMigrate:
			if err := applyMigration(action, source, opts); err != nil {
				return err
			}
		case plan.StatusConflict:
			switch conflictDecision(action, opts) {
			case DecisionSkip:
				// Safe default for unresolved conflicts is skip: do not mutate the
				// existing workstation target, but continue applying safe actions.
				continue
			case DecisionReplace:
				if err := applyReplace(action, source, opts); err != nil {
					return err
				}
			case DecisionAdopt:
				if err := applyAdopt(action, source, opts); err != nil {
					return err
				}
			}
		}
	}

	return recordMetadata(p, resolvedSources, opts)
}

// recordMetadata upserts Installation Metadata for every managed target the plan
// installed or confirmed unchanged, so dots status has an authoritative record
// of what dots owns. Copy-like records include a Source of Truth hash; symlink
// records leave Hash empty because status compares the link destination.
func recordMetadata(p plan.Plan, resolvedSources [][]string, opts Options) error {
	if opts.StateRoot == "" {
		return nil
	}

	path := state.Path(opts.StateRoot)
	meta, err := state.Load(path)
	if err != nil {
		return err
	}
	meta.Version = state.CurrentVersion
	meta.Provenance = state.CaptureProvenance(opts.SourceRoot, version.Value)

	now := time.Now().UTC().Format(time.RFC3339)
	legacyTargets := map[string]struct{}{}
	for i, action := range p.Actions {
		if !recordsMetadata(action, conflictDecision(action, opts)) {
			continue
		}
		if action.Ownership == "seeded" && action.Reason == plan.ReasonSeededLocalEvolution {
			// Preserve the baseline that originally seeded locally evolved state.
			// Replacing it with the new Source of Truth baseline would destroy the
			// evidence needed to recognize a later reset and advance safely.
			continue
		}
		hash := ""
		var ownedContent []byte
		var ownedBytes []byte
		var seededBaseline []byte
		recordedOwnership := action.Ownership
		if recordedOwnership == "" {
			recordedOwnership = "whole"
		}
		if action.Strategy != "symlink" {
			if len(action.Sources) > 0 {
				hash = state.HashBytes(action.Content)
				if action.Ownership == "json-subset" {
					ownedContent = append([]byte(nil), action.Content...)
				}
			} else {
				var err error
				hash, err = state.HashFile(resolvedSources[i][0])
				if err != nil {
					return err
				}
				if action.Ownership == "json-subset" {
					ownedContent, err = os.ReadFile(resolvedSources[i][0])
					if err != nil {
						return fmt.Errorf("read owned JSON contribution %s: %w", resolvedSources[i][0], err)
					}
				} else if action.Ownership == "jsonc-subset" {
					rawContent, readErr := os.ReadFile(resolvedSources[i][0])
					if readErr != nil {
						return fmt.Errorf("read owned JSONC contribution %s: %w", resolvedSources[i][0], readErr)
					}
					ownedContent, err = configsubset.CanonicalJSONC(rawContent)
					if err != nil {
						return fmt.Errorf("canonicalize owned JSONC contribution %s: %w", resolvedSources[i][0], err)
					}
				} else if action.Ownership == "toml-subset" {
					ownedBytes, err = os.ReadFile(resolvedSources[i][0])
					if err != nil {
						return fmt.Errorf("read owned TOML contribution %s: %w", resolvedSources[i][0], err)
					}
				} else if action.Ownership == "seeded" {
					if action.Migration != nil && action.Migration.RecordedBaseline != nil {
						seededBaseline = append([]byte(nil), action.Migration.RecordedBaseline...)
					} else {
						seededBaseline, err = os.ReadFile(resolvedSources[i][0])
						if err != nil {
							return fmt.Errorf("read seeded baseline %s: %w", resolvedSources[i][0], err)
						}
					}
				} else if action.Ownership == "marked-block" {
					ownedBytes, err = os.ReadFile(resolvedSources[i][0])
					if err != nil {
						return fmt.Errorf("read owned marked block %s: %w", resolvedSources[i][0], err)
					}
				}
			}
		}
		upsertRecord(&meta, state.Record{
			Target:         action.Target,
			Source:         action.Source,
			Sources:        append([]string(nil), action.Sources...),
			Strategy:       action.Strategy,
			Ownership:      recordedOwnership,
			OwnedContent:   ownedContent,
			OwnedBytes:     ownedBytes,
			SeededBaseline: seededBaseline,
			Hash:           hash,
			InstalledAt:    now,
			Profiles:       append([]string(nil), p.Profiles...),
			Tags:           append([]string(nil), p.Tags...),
		})
		if action.Migration != nil && action.Migration.LegacyTarget != "" {
			legacyTargets[action.Migration.LegacyTarget] = struct{}{}
		}
	}
	for target := range legacyTargets {
		meta = meta.Remove(target)
	}

	return state.Save(path, meta)
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
		sources := []string{action.Source}
		declaredResolved := []string{action.ResolvedSource}
		if len(action.Sources) > 0 {
			sources = action.Sources
			declaredResolved = action.ResolvedSources
			if action.Strategy != "copy" || action.Ownership != "json-subset" || len(sources) < 2 {
				return nil, fmt.Errorf("composed target %s requires at least two copy/json-subset sources", action.Target)
			}
		}
		for j, sourceName := range sources {
			source, err := plan.ResolveSource(sourceName, sourceRoot)
			if err != nil {
				return nil, err
			}
			if j < len(declaredResolved) && declaredResolved[j] != "" && declaredResolved[j] != source {
				return nil, fmt.Errorf("install plan source %q resolved to %q, want %q", sourceName, declaredResolved[j], source)
			}
			resolvedSources[i] = append(resolvedSources[i], source)
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
			if action.Strategy != "copy" || (action.Ownership != "json-subset" && action.Ownership != "jsonc-subset" && action.Ownership != "toml-subset" && action.Ownership != "marked-block" && action.Ownership != "seeded") {
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

func applyUpdate(action plan.Action, source string, opts Options) error {
	if err := createBackupSet(opts, action.Target); err != nil {
		return err
	}
	switch action.Ownership {
	case "json-subset":
		if len(action.PreviousContent) > 0 {
			current := action.Content
			if len(current) == 0 {
				var err error
				current, err = os.ReadFile(source)
				if err != nil {
					return fmt.Errorf("read current owned JSON %s: %w", source, err)
				}
			}
			if err := reconcileJSONContentFile(action.Target, action.PreviousContent, current); err != nil {
				return fmt.Errorf("reconcile JSON update for %s: %w", action.Target, err)
			}
			return nil
		}
		if len(action.Content) > 0 {
			if err := mergeJSONContentFile(action.Target, action.Content); err != nil {
				return fmt.Errorf("merge composed JSON update for %s: %w", action.Target, err)
			}
			return nil
		}
		if err := configsubset.MergeJSONFile(action.Target, source); err != nil {
			return fmt.Errorf("merge JSON update for %s: %w", action.Target, err)
		}
	case "jsonc-subset":
		current, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read current owned JSONC %s: %w", source, err)
		}
		if len(action.PreviousContent) > 0 {
			if err := reconcileJSONCContentFile(action.Target, action.PreviousContent, current); err != nil {
				return fmt.Errorf("reconcile JSONC update for %s: %w", action.Target, err)
			}
			return nil
		}
		if err := mergeJSONCContentFile(action.Target, current); err != nil {
			return fmt.Errorf("merge JSONC update for %s: %w", action.Target, err)
		}
	case "toml-subset":
		if len(action.PreviousContent) > 0 {
			current, err := os.ReadFile(source)
			if err != nil {
				return fmt.Errorf("read current owned TOML %s: %w", source, err)
			}
			if err := reconcileTOMLContentFile(action.Target, action.PreviousContent, current); err != nil {
				return fmt.Errorf("reconcile TOML update for %s: %w", action.Target, err)
			}
			return nil
		}
		if err := configsubset.MergeTOMLFile(action.Target, source); err != nil {
			return fmt.Errorf("merge TOML update for %s: %w", action.Target, err)
		}
	case "seeded":
		live, err := os.ReadFile(action.Target)
		if err != nil {
			return fmt.Errorf("read seeded target %s: %w", action.Target, err)
		}
		if !bytes.Equal(live, action.PreviousContent) {
			return fmt.Errorf("install plan is stale: seeded target %s evolved before baseline advancement", action.Target)
		}
		current, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read seeded baseline %s: %w", source, err)
		}
		info, err := os.Stat(action.Target)
		if err != nil {
			return fmt.Errorf("stat seeded target %s: %w", action.Target, err)
		}
		if err := os.WriteFile(action.Target, current, info.Mode().Perm()); err != nil {
			return fmt.Errorf("advance seeded target %s: %w", action.Target, err)
		}
	case "marked-block":
		live, err := os.ReadFile(action.Target)
		if err != nil {
			return fmt.Errorf("read marked-block target %s: %w", action.Target, err)
		}
		current, err := os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("read marked-block source %s: %w", source, err)
		}
		reconciliation := textblock.ReconcileOwned(live, action.PreviousContent, current, textblock.DotsManagedMarkers())
		if !reconciliation.Compatible {
			return fmt.Errorf("install plan is stale: marked block %s changed before update", action.Target)
		}
		info, err := os.Stat(action.Target)
		if err != nil {
			return fmt.Errorf("stat marked-block target %s: %w", action.Target, err)
		}
		if err := os.WriteFile(action.Target, reconciliation.Content, info.Mode().Perm()); err != nil {
			return fmt.Errorf("update marked block %s: %w", action.Target, err)
		}
	default:
		return fmt.Errorf("update ownership %q is not supported for %s", action.Ownership, action.Target)
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

func mergeJSONContentFile(target string, sourceData []byte) error {
	targetData, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	merged, err := configsubset.MergeJSON(targetData, sourceData)
	if err != nil {
		return err
	}
	return os.WriteFile(target, merged, info.Mode().Perm())
}

func reconcileJSONContentFile(target string, previousData, currentData []byte) error {
	targetData, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	reconciliation, err := configsubset.ReconcileJSON(targetData, previousData, currentData)
	if err != nil {
		return err
	}
	if !reconciliation.Compatible {
		return fmt.Errorf("live target changed a previously owned JSON value")
	}
	if !reconciliation.Changed {
		return nil
	}
	return os.WriteFile(target, reconciliation.Content, info.Mode().Perm())
}

func mergeJSONCContentFile(target string, sourceData []byte) error {
	targetData, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	merged, err := configsubset.MergeJSONC(targetData, sourceData)
	if err != nil {
		return err
	}
	if bytes.Equal(merged, targetData) {
		return nil
	}
	return os.WriteFile(target, merged, info.Mode().Perm())
}

func reconcileJSONCContentFile(target string, previousData, currentData []byte) error {
	targetData, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	reconciliation, err := configsubset.ReconcileJSONC(targetData, previousData, currentData)
	if err != nil {
		return err
	}
	if !reconciliation.Compatible {
		return fmt.Errorf("live target changed a previously owned JSONC value")
	}
	if !reconciliation.Changed {
		return nil
	}
	return os.WriteFile(target, reconciliation.Content, info.Mode().Perm())
}

func reconcileTOMLContentFile(target string, previousData, currentData []byte) error {
	targetData, err := os.ReadFile(target)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	reconciliation, err := configsubset.ReconcileTOML(targetData, previousData, currentData)
	if err != nil {
		return err
	}
	if !reconciliation.Compatible {
		return fmt.Errorf("live target changed a previously owned TOML value")
	}
	if !reconciliation.Changed {
		return nil
	}
	return os.WriteFile(target, reconciliation.Content, info.Mode().Perm())
}
