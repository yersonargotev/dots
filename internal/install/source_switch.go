package install

import (
	"bytes"
	"fmt"
	"os"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/ownershipevidence"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/selectionreconciliation"
	"github.com/yersonargotev/dots/internal/state"
)

// validateOwnedSourceSwitch is a preflight; the receipt transaction repeats
// record/selection/source checks and the confined transform consumes live bytes.
func validateOwnedSourceSwitch(action plan.Action, source string, opts Options) error {
	if action.Status != plan.StatusConflict || action.Reason != selectionreconciliation.ReasonOwnedSourceConflict || action.Strategy != "copy" || action.Ownership != "toml-subset" || len(action.Sources) > 0 || action.SourceSwitch.TargetIdentity == nil || action.SourceSwitch.LiveContent == nil || action.SourceSwitch.SourceContent == nil {
		return fmt.Errorf("invalid owned source-switch authority for %s", action.Target)
	}
	decision := conflictDecision(action, opts)
	if decision != DecisionSkip && decision != DecisionReplace {
		return fmt.Errorf("owned source switch for %s requires Skip or Replace", action.Target)
	}
	meta, err := state.Load(state.Path(opts.StateRoot))
	if err != nil {
		return fmt.Errorf("load source-switch authority: %w", err)
	}
	if err := validateAuthorizingRecord(meta, action, action.PreviousReconciliationReceipt); err != nil {
		return err
	}
	record, _ := meta.FindByTarget(action.Target)
	projected, err := ownershipevidence.ProjectRecord(record)
	if err != nil || record.Strategy != "copy" || record.Ownership != "toml-subset" || !bytes.Equal(projected.OwnedBytes, action.PreviousContent) || projected.Hash == "" || projected.Hash != state.HashBytes(projected.OwnedBytes) {
		return fmt.Errorf("source switch for %s lacks exact previous TOML attribution", action.Target)
	}
	current, err := readConfinedRegularFile(source, opts.SourceRoot)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, action.SourceSwitch.SourceContent) {
		return fmt.Errorf("source-switch source changed after inspection for %s", action.Target)
	}
	info, err := os.Lstat(action.Target)
	if err != nil || !info.Mode().IsRegular() || (!os.SameFile(action.SourceSwitch.TargetIdentity, info) || action.SourceSwitch.TargetIdentity.Mode().Perm() != info.Mode().Perm()) {
		return fmt.Errorf("source-switch target %s is not a regular file", action.Target)
	}
	live, err := readConfinedRegularFile(action.Target, opts.Home)
	if err != nil {
		return err
	}
	if !bytes.Equal(live, action.SourceSwitch.LiveContent) {
		return fmt.Errorf("source-switch target changed after inspection for %s", action.Target)
	}
	replacement, err := configsubset.ReplaceOwnedTOML(live, action.PreviousContent, current)
	if err != nil {
		return fmt.Errorf("validate owned TOML source switch: %w", err)
	}
	if !replacement.Compatible {
		return fmt.Errorf("owned TOML source switch for %s is ambiguous", action.Target)
	}
	return nil
}

func applyOwnedSourceSwitch(action plan.Action, opts Options) (managedActionResult, error) {
	return updateConfinedRegularFileWithIdentity(action.Target, opts.Home, action.SourceSwitch.TargetIdentity, func(live []byte) ([]byte, bool, error) {
		if !bytes.Equal(live, action.SourceSwitch.LiveContent) {
			return nil, false, fmt.Errorf("source-switch target changed after inspection for %s", action.Target)
		}
		replacement, err := configsubset.ReplaceOwnedTOML(live, action.PreviousContent, action.Content)
		if err != nil {
			return nil, false, fmt.Errorf("replace owned TOML source switch: %w", err)
		}
		if !replacement.Compatible {
			return nil, false, fmt.Errorf("owned TOML source switch for %s is ambiguous", action.Target)
		}
		return replacement.Content, replacement.Changed, nil
	}, func(live []byte, mode os.FileMode) error {
		_, err := backups.CreateContentSet(opts.StateRoot, action.Target, live, mode, backups.CreateOptions{Reason: "pre-install conflict protection", Machine: backups.MachineName(), Repo: opts.SourceRoot})
		return err
	})
}
