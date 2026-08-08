package plan

import (
	"fmt"
	"os"

	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/state"
)

// UninstallStatus describes what removing a recorded target would do, mirroring
// Status for the forward Install Plan. It is computed from the Installation
// Metadata and the current on-disk state, never from the manifest: uninstall
// only ever acts on what dots recorded it owns.
type UninstallStatus string

const (
	// UninstallRemove means the target is still dots-owned (a symlink that
	// resolves to the recorded source, or a copy whose content matches the
	// recorded hash) and is safe to delete.
	UninstallRemove UninstallStatus = "remove"
	// UninstallSkip means there is nothing to remove: a copied target that is
	// already absent.
	UninstallSkip UninstallStatus = "skip"
	// UninstallModified means recorded content drifted. Only explicitly
	// whole-owned targets may become removable when the caller forces removal.
	UninstallModified UninstallStatus = "modified"
	// UninstallNotOwned means the target no longer matches what dots recorded (a
	// symlink that is missing or points elsewhere, or a path whose type changed),
	// so removing it could destroy something dots does not own.
	UninstallNotOwned UninstallStatus = "not-owned"
)

// UninstallAction is a single planned removal for a recorded Managed Entry.
type UninstallAction struct {
	Target         string          `json:"target"`
	Source         string          `json:"source"`
	Sources        []string        `json:"sources,omitempty"`
	Strategy       string          `json:"strategy"`
	Ownership      string          `json:"ownership,omitempty"`
	ForceRemovable bool            `json:"-"`
	Status         UninstallStatus `json:"status"`
}

// UninstallPlan is the preview of removals an uninstall would apply.
type UninstallPlan struct {
	Actions []UninstallAction `json:"actions"`
}

// Removable reports whether the plan contains at least one target that uninstall
// would delete: an owned target, or a drifted copy that --force would remove.
func (p UninstallPlan) Removable() bool {
	for _, a := range p.Actions {
		if a.Status == UninstallRemove || (a.Status == UninstallModified && a.ForceRemovable) {
			return true
		}
	}
	return false
}

// UninstallOptions carries the resolved inputs needed to classify recorded
// targets. SourceRoot is required to verify that a symlink still resolves to the
// repository source dots recorded. Home is required so classification can confine
// every filesystem inspection to the home sandbox.
type UninstallOptions struct {
	SourceRoot string
	Home       string
}

// BuildUninstall computes the Uninstall Plan from Installation Metadata without
// mutating the filesystem. Each recorded target is classified against current
// disk state so the caller can preview exactly which targets are still owned,
// which drifted, and which dots no longer recognizes.
//
// Classification confines itself to home: a recorded target is validated to be
// inside the home sandbox, with no symlink escape through its parent chain,
// before it is ever inspected. A target that fails that check is reported
// not-owned and is never Lstat'd, readlink'd, or hashed, so even a crafted or
// stale metadata record cannot make a preview read a path outside home.
func BuildUninstall(meta state.Metadata, opts UninstallOptions) (UninstallPlan, error) {
	if opts.Home == "" {
		return UninstallPlan{}, fmt.Errorf("uninstall home is required")
	}

	plan := UninstallPlan{}
	for _, rec := range meta.Entries {
		status, err := uninstallStatus(rec, opts.SourceRoot, opts.Home)
		if err != nil {
			return UninstallPlan{}, err
		}
		plan.Actions = append(plan.Actions, UninstallAction{
			Target:    rec.Target,
			Source:    rec.Source,
			Strategy:  rec.Strategy,
			Ownership: rec.Ownership,
			Status:    status,
		})
		plan.Actions[len(plan.Actions)-1].ForceRemovable = rec.Ownership == "whole"
		if len(rec.Sources) > 0 {
			plan.Actions[len(plan.Actions)-1].Sources = rec.SourceList()
		}
	}
	return plan, nil
}

func uninstallStatus(rec state.Record, sourceRoot, home string) (UninstallStatus, error) {
	// Confinement before inspection: validate the target is lexically inside home
	// and that no parent component escapes home through a symlink BEFORE any
	// Lstat/Readlink/hash touches it. A target that fails is not dots-owned (install
	// only ever writes inside home), so report not-owned without inspecting it.
	if err := ValidateResolvedTarget(rec.Target, home); err != nil {
		return UninstallNotOwned, nil
	}
	if err := ValidateTargetParentInsideHome(rec.Target, home); err != nil {
		return UninstallNotOwned, nil
	}

	info, err := os.Lstat(rec.Target)
	if os.IsNotExist(err) {
		// A missing symlink can no longer be verified to point at the recorded
		// source, so it is not-owned rather than a no-op. A missing copy is simply
		// already gone.
		if rec.Strategy == "symlink" {
			return UninstallNotOwned, nil
		}
		return UninstallSkip, nil
	}
	if err != nil {
		return "", fmt.Errorf("stat uninstall target %s: %w", rec.Target, err)
	}

	switch rec.Strategy {
	case "symlink":
		if info.Mode()&os.ModeSymlink == 0 {
			return UninstallNotOwned, nil
		}
		dest, err := os.Readlink(rec.Target)
		if err != nil {
			return UninstallNotOwned, nil
		}
		expected, err := ResolveSource(rec.Source, sourceRoot)
		if err != nil {
			return UninstallNotOwned, nil
		}
		if dest != expected {
			return UninstallNotOwned, nil
		}
		return UninstallRemove, nil
	case "copy":
		if !info.Mode().IsRegular() {
			return UninstallNotOwned, nil
		}
		if rec.Ownership == "json-subset" && len(rec.OwnedContent) > 0 {
			targetData, err := os.ReadFile(rec.Target)
			if err != nil {
				return "", fmt.Errorf("read uninstall target %s: %w", rec.Target, err)
			}
			_, _, _, compatible, err := configsubset.RemoveJSON(targetData, rec.OwnedContent)
			if err != nil {
				return "", fmt.Errorf("analyze owned JSON for %s: %w", rec.Target, err)
			}
			if compatible {
				return UninstallRemove, nil
			}
			return UninstallModified, nil
		}
		hash, err := state.HashFile(rec.Target)
		if err != nil {
			return "", err
		}
		if hash != rec.Hash {
			return UninstallModified, nil
		}
		return UninstallRemove, nil
	default:
		return UninstallNotOwned, nil
	}
}

// ValidateBackupableTarget verifies that a target is a regular file, directory,
// or symlink before dots backs it up or removes it. It rejects special files
// (devices, sockets, named pipes) that backup and removal cannot safely handle.
// It is shared by install (pre-replace backups) and uninstall (pre-removal).
func ValidateBackupableTarget(target string) error {
	info, err := os.Lstat(target)
	if err != nil {
		return fmt.Errorf("stat target %s: %w", target, err)
	}
	if info.Mode().IsRegular() || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	return fmt.Errorf("target %s is not a regular file, directory, or symlink", target)
}
