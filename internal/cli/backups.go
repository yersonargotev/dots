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
		Short: "Inspect Backup Sets recorded by dots",
		Long:  "backups reads centralized Backup Metadata from the configured state root and reports Backup Sets created by dots.",
	}
	cmd.AddCommand(newBackupsListCommand())
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
