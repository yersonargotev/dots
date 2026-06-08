package cli

import (
	"fmt"
	"os"

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

			if validatePaths {
				for _, target := range set.Targets {
					if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(target, paths.Home, "restore target"); err != nil {
						return err
					}
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

			overwritten := overwriteTargets(items)
			if len(overwritten) > 0 {
				safety, err := backups.CreateSet(paths.StateRoot, overwritten, backups.CreateOptions{
					Reason:  "pre-restore safety backup",
					Machine: machineName(),
					Repo:    set.Repo,
				})
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "\nBacked up %s to Backup Set %s before restoring.\n", pluralizeTargets(len(overwritten)), safety.ID)
			}

			if err := backups.ApplyRestore(items); err != nil {
				return err
			}

			fmt.Fprintf(out, "Restored %s from Backup Set %s.\n", pluralizeTargets(len(items)), set.ID)
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
// provenance tracking) is allowed because there is nothing to mismatch.
func ensureMachineMatch(set backups.BackupSet, force bool) error {
	if force || set.Machine == "" {
		return nil
	}
	current := machineName()
	if current == "" || set.Machine == current {
		return nil
	}
	return fmt.Errorf("Backup Set %s was captured on machine %q but this machine is %q; re-run with --force to restore anyway", set.ID, set.Machine, current)
}

func overwriteTargets(items []backups.RestoreItem) []string {
	targets := make([]string, 0, len(items))
	for _, item := range items {
		if item.Action == backups.RestoreOverwrite {
			targets = append(targets, item.Target)
		}
	}
	return targets
}

// machineName identifies the current workstation for Backup Set provenance.
func machineName() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}
