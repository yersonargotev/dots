// Package uninstall reverses a dots install. It removes only the symlinks and
// copied files recorded in the Installation Metadata, re-verifying ownership and
// home-confinement at apply time, and optionally restores the pre-install Backup
// Set for each target. It is the mirror of internal/install: install owns the
// forward path, uninstall owns the reverse.
package uninstall

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

// Options carries the resolved inputs needed to apply an Uninstall Plan.
type Options struct {
	SourceRoot string
	Home       string
	// StateRoot is where Installation Metadata and Backup Sets live. It is
	// required: uninstall is metadata-driven and must prune what it removes.
	StateRoot string
	// Force additionally removes copied targets whose content drifted from the
	// recorded hash. Without it, drifted copies are preserved.
	Force bool
	// RestoreBackups restores the most recent Backup Set covering each removed
	// target after removal, returning the workstation to its pre-install state.
	RestoreBackups bool
}

// Result reports what Apply did so the caller can summarize the outcome.
type Result struct {
	// Removed lists the targets successfully deleted, in plan order.
	Removed []string `json:"removed"`
	// Updated lists co-owned targets that remained after dots removed or pruned
	// its recorded contribution.
	Updated []string `json:"updated,omitempty"`
	// RestoredSets lists the IDs of Backup Sets restored when RestoreBackups is
	// set, each restored at most once even if it covers several removed targets.
	RestoredSets []string `json:"restored_sets"`
}

// Apply removes the dots-owned targets in an Uninstall Plan. It re-classifies
// each target against current metadata and disk state before touching it, so a
// stale plan or a target that drifted between preview and apply is never removed
// by surprise. Every removal is confined to home, metadata is pruned for what
// was removed, and the metadata file is deleted once empty.
func Apply(p plan.UninstallPlan, opts Options) (Result, error) {
	var result Result
	var restorableRemoved []string

	home, err := cleanAbs(opts.Home)
	if err != nil {
		return result, fmt.Errorf("resolve home: %w", err)
	}
	if opts.SourceRoot == "" {
		return result, fmt.Errorf("uninstall source root is required")
	}
	if opts.StateRoot == "" {
		return result, fmt.Errorf("uninstall state root is required")
	}
	if err := validateStateRoot(opts.StateRoot, home); err != nil {
		return result, err
	}

	meta, err := state.Load(state.Path(opts.StateRoot))
	if err != nil {
		return result, err
	}

	for _, action := range p.Actions {
		rec, ok := meta.FindByTarget(action.Target)
		if !ok {
			// The plan references a target dots no longer records; never act on it.
			continue
		}
		if rec.Ownership == "json-subset" && len(rec.OwnedContent) > 0 {
			if action.Status != plan.UninstallRemove {
				continue
			}
			applied, deleted, err := removeOwnedJSON(rec, home)
			if err != nil {
				return result, err
			}
			if applied && deleted {
				result.Removed = append(result.Removed, action.Target)
			} else if applied {
				result.Updated = append(result.Updated, action.Target)
			}
			continue
		}
		remove, err := stillRemovable(rec, opts.SourceRoot, home, opts.Force)
		if err != nil {
			return result, err
		}
		if !remove {
			continue
		}

		if err := validateRemovableTarget(action.Target, home); err != nil {
			return result, err
		}
		if err := removeTarget(action.Target); err != nil {
			return result, fmt.Errorf("remove %s: %w", action.Target, err)
		}
		result.Removed = append(result.Removed, action.Target)
		restorableRemoved = append(restorableRemoved, action.Target)
	}

	if opts.RestoreBackups {
		if err := restoreBackups(&result, restorableRemoved, opts, home); err != nil {
			return result, err
		}
	}

	prunedTargets := append(append([]string(nil), result.Removed...), result.Updated...)
	if err := pruneMetadata(opts.StateRoot, meta, prunedTargets); err != nil {
		return result, err
	}

	return result, nil
}

// removeOwnedJSON revalidates and subtracts a partial contribution at apply
// time. It preserves the physical target when external content remains and
// reports false without mutation when any formerly owned value changed.
func removeOwnedJSON(rec state.Record, home string) (applied, deleted bool, err error) {
	leaf, err := os.Lstat(rec.Target)
	if os.IsNotExist(err) {
		return true, true, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("stat owned JSON target %s: %w", rec.Target, err)
	}
	if !leaf.Mode().IsRegular() {
		return false, false, nil
	}
	if err := validateRemovableTarget(rec.Target, home); err != nil {
		return false, false, err
	}
	if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(rec.Target, home, "owned JSON target"); err != nil {
		return false, false, err
	}
	targetData, err := os.ReadFile(rec.Target)
	if err != nil {
		return false, false, fmt.Errorf("read owned JSON target %s: %w", rec.Target, err)
	}
	info, err := os.Stat(rec.Target)
	if err != nil {
		return false, false, fmt.Errorf("stat owned JSON target %s: %w", rec.Target, err)
	}
	content, changed, empty, compatible, err := configsubset.RemoveJSON(targetData, rec.OwnedContent)
	if err != nil {
		return false, false, fmt.Errorf("remove owned JSON from %s: %w", rec.Target, err)
	}
	if !compatible {
		return false, false, nil
	}
	if empty {
		if err := removeTarget(rec.Target); err != nil {
			return false, false, fmt.Errorf("remove emptied JSON target %s: %w", rec.Target, err)
		}
		return true, true, nil
	}
	if !changed {
		return true, false, nil
	}
	if err := os.WriteFile(rec.Target, content, info.Mode().Perm()); err != nil {
		return false, false, fmt.Errorf("write JSON target %s after removing owned content: %w", rec.Target, err)
	}
	return true, false, nil
}

// stillRemovable re-derives the record's status from current disk state using the
// same classifier as the preview, so apply and preview never diverge. A record is
// removed when it is still owned, or when it drifted and the caller forced it.
func stillRemovable(rec state.Record, sourceRoot, home string, force bool) (bool, error) {
	p, err := plan.BuildUninstall(state.Metadata{Entries: []state.Record{rec}}, plan.UninstallOptions{SourceRoot: sourceRoot, Home: home})
	if err != nil {
		return false, err
	}
	if len(p.Actions) != 1 {
		return false, fmt.Errorf("reclassify %s: unexpected plan size %d", rec.Target, len(p.Actions))
	}
	switch p.Actions[0].Status {
	case plan.UninstallRemove:
		return true, nil
	case plan.UninstallModified:
		if rec.Ownership != "" {
			return false, nil
		}
		return force, nil
	default:
		return false, nil
	}
}

// validateRemovableTarget enforces the critical invariant that no removal escapes
// home. It validates the lexical target and its existing parent chain, then
// confirms the leaf is a regular file, directory, or symlink before deletion.
func validateRemovableTarget(target, home string) error {
	if err := plan.ValidateResolvedTarget(target, home); err != nil {
		return err
	}
	if err := plan.ValidateTargetParentInsideHome(target, home); err != nil {
		return err
	}
	return plan.ValidateBackupableTarget(target)
}

// removeTarget deletes a single owned target. Records only ever describe regular
// files and symlinks, so os.Remove is correct and fails closed on a directory,
// which uninstall must never recursively delete.
func removeTarget(target string) error {
	return os.Remove(target)
}

// restoreBackups restores the most recent Backup Set covering each removed
// target. Install records single-target pre-install Backup Sets, so this returns
// each target to the file that existed before dots managed it. A set is restored
// at most once even when it covers several removed targets.
func restoreBackups(result *Result, targets []string, opts Options, home string) error {
	meta, err := backups.Load(backups.Path(opts.StateRoot))
	if err != nil {
		return err
	}

	restored := map[string]struct{}{}
	for _, target := range targets {
		set, ok := latestSetContaining(meta, target)
		if !ok {
			continue
		}
		if _, done := restored[set.ID]; done {
			continue
		}
		// Restore writes to the set's recorded targets, so they must be confined to
		// home regardless of where the state root lives.
		for _, t := range set.Targets {
			if err := plan.ValidateResolvedTarget(t, home); err != nil {
				return err
			}
			if err := plan.ValidateTargetParentInsideHome(t, home); err != nil {
				return err
			}
		}
		if _, err := backups.Restore(opts.StateRoot, set, backups.RestoreOptions{
			Machine: backups.MachineName(),
			Repo:    set.Repo,
		}); err != nil {
			return err
		}
		restored[set.ID] = struct{}{}
		result.RestoredSets = append(result.RestoredSets, set.ID)
	}
	return nil
}

// latestSetContaining returns the most recently recorded Backup Set whose targets
// include target. Sets are appended in creation order, so the last match is the
// newest.
func latestSetContaining(meta backups.Metadata, target string) (backups.BackupSet, bool) {
	for i := len(meta.Sets) - 1; i >= 0; i-- {
		for _, t := range meta.Sets[i].Targets {
			if t == target {
				return meta.Sets[i], true
			}
		}
	}
	return backups.BackupSet{}, false
}

// pruneMetadata drops the records for removed targets and persists the result. To
// leave a clean state, the metadata file is deleted entirely once no records
// remain rather than left as an empty entry list.
func pruneMetadata(stateRoot string, meta state.Metadata, removed []string) error {
	if len(removed) == 0 {
		return nil
	}
	pruned := meta.Remove(removed...)
	path := state.Path(stateRoot)
	if len(pruned.Entries) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove emptied installation metadata: %w", err)
		}
		return nil
	}
	return state.Save(path, pruned)
}

// validateStateRoot mirrors install's guard: a state root inside home must not
// escape home through symlinks before uninstall writes pruned metadata there. An
// explicit state root outside home is trusted caller-controlled storage.
func validateStateRoot(stateRoot, home string) error {
	stateAbs, err := cleanAbs(stateRoot)
	if err != nil {
		return fmt.Errorf("resolve state root %s: %w", stateRoot, err)
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
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
