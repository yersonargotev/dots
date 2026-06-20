package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/plan"
)

func newBackupsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backups",
		Short: "Inspect and restore Backup Sets recorded by dots",
		Long:  "backups reads centralized Backup Metadata from the configured state root and reports or restores Backup Sets created by dots.",
	}
	cmd.AddCommand(newBackupsListCommand())
	cmd.AddCommand(newBackupsRestoreCommand())
	return cmd
}

func newBackupsListCommand() *cobra.Command {
	var (
		home      string
		stateRoot string
	)

	cmd := &cobra.Command{
		Use:          "list",
		Short:        "List recorded Backup Sets",
		Long:         "list shows when each Backup Set was created, which targets were protected, and why the backup was taken.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths(home, "", stateRoot)
			if err != nil {
				return err
			}

			if stateRoot == "" || plan.InsideRoot(paths.StateRoot, paths.Home) {
				if err := plan.ValidatePathInsideHomeNoSymlinkEscape(paths.StateRoot, paths.Home, "state root"); err != nil {
					return err
				}
				if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(backups.Path(paths.StateRoot), paths.Home, "Backup Metadata"); err != nil {
					return err
				}
			}

			meta, err := backups.Load(backups.Path(paths.StateRoot))
			if err != nil {
				return err
			}

			if len(meta.Sets) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No Backup Sets recorded in state root: %s\n", paths.StateRoot)
				return nil
			}

			renderBackupSets(cmd.OutOrStdout(), meta)
			return nil
		},
	}

	cmd.Flags().StringVar(&home, "home", "", "home directory used to resolve the default state root (default: the current user's home)")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Backup Metadata (default ~/.local/state/dots)")
	return cmd
}

func newBackupsRestoreCommand() *cobra.Command {
	var (
		home      string
		stateRoot string
		dryRun    bool
		force     bool
	)

	cmd := &cobra.Command{
		Use:   "restore <set>",
		Short: "Restore a recorded Backup Set to its original targets",
		Long: "restore returns the targets recorded in a Backup Set to the content preserved when the set was created. " +
			"It refuses sets captured on a different machine unless --force is given, and backs up any current targets it would overwrite before restoring.",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			setID := args[0]

			paths, err := resolvePaths(home, "", stateRoot)
			if err != nil {
				return err
			}

			validatePaths := stateRoot == "" || plan.InsideRoot(paths.StateRoot, paths.Home)
			if validatePaths {
				if err := plan.ValidatePathInsideHomeNoSymlinkEscape(paths.StateRoot, paths.Home, "state root"); err != nil {
					return err
				}
				if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(backups.Path(paths.StateRoot), paths.Home, "Backup Metadata"); err != nil {
					return err
				}
			}

			meta, err := backups.Load(backups.Path(paths.StateRoot))
			if err != nil {
				return err
			}

			set, ok := meta.FindSet(setID)
			if !ok {
				return fmt.Errorf("Backup Set %q not found in state root: %s", setID, paths.StateRoot)
			}

			if err := ensureMachineMatch(set, force); err != nil {
				return err
			}

			// Restore writes to the recorded targets, so they must always be
			// inside the home sandbox regardless of where the state root lives.
			// Trusting an explicit external state root's *location* must never
			// extend to trusting the *targets* recorded in its metadata.
			//
			// Validate the parent path rather than rejecting a symlink leaf: the
			// normal install replace flow can leave a managed symlink at the
			// target, and restore must be able to back up and remove that symlink
			// without following it.
			for _, target := range set.Targets {
				if err := plan.ValidateResolvedTarget(target, paths.Home); err != nil {
					return err
				}
				if err := plan.ValidateTargetParentInsideHome(target, paths.Home); err != nil {
					return err
				}
			}

			items, err := backups.PlanRestore(paths.StateRoot, set)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			renderRestorePlan(out, set, items)

			if dryRun {
				fmt.Fprintln(out, "\nDry run: no files changed.")
				return nil
			}

			result, err := backups.Restore(paths.StateRoot, set, backups.RestoreOptions{
				Machine: backups.MachineName(),
				Repo:    set.Repo,
			})
			if err != nil {
				return err
			}

			if result.SafetySet != nil {
				fmt.Fprintf(out, "\nBacked up %s to Backup Set %s before restoring.\n", pluralizeTargets(len(result.SafetySet.Targets)), result.SafetySet.ID)
			}

			fmt.Fprintf(out, "Restored %s from Backup Set %s.\n", pluralizeTargets(len(result.Items)), set.ID)
			return nil
		},
	}

	cmd.Flags().StringVar(&home, "home", "", "home directory used to resolve the default state root (default: the current user's home)")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Backup Metadata (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what restore would change without touching any files")
	cmd.Flags().BoolVar(&force, "force", false, "restore even when the Backup Set was captured on a different machine")
	return cmd
}

// ensureMachineMatch refuses a Backup Set captured on a different machine unless
// the operator forces it. A set with no recorded machine (created before
// provenance tracking) is allowed because there is nothing to mismatch. If the
// current machine name cannot be determined it fails closed, requiring --force,
// rather than silently treating an unknown machine as a match.
func ensureMachineMatch(set backups.BackupSet, force bool) error {
	if force || set.Machine == "" {
		return nil
	}
	current := backups.MachineName()
	if current == "" {
		return fmt.Errorf("cannot determine current machine to verify Backup Set %s captured on %q; re-run with --force to restore anyway", set.ID, set.Machine)
	}
	if set.Machine == current {
		return nil
	}
	return fmt.Errorf("Backup Set %s was captured on machine %q but this machine is %q; re-run with --force to restore anyway", set.ID, set.Machine, current)
}
