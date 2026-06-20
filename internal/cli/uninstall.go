package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/uninstall"
)

func newUninstallCommand() *cobra.Command {
	var (
		sourceRoot     string
		home           string
		stateRoot      string
		dryRun         bool
		yes            bool
		force          bool
		restoreBackups bool
	)

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Reverse a dots install using recorded Installation Metadata",
		Long: "uninstall computes an Uninstall Plan from the Installation Metadata and removes only the symlinks and copied files dots owns. " +
			"It previews the plan and prompts before mutating unless --yes is set, skips targets that drifted or that dots no longer owns, " +
			"and never touches a path it did not record. Use --restore-backups to return each removed target to its pre-install Backup Set.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths(home, sourceRoot, stateRoot)
			if err != nil {
				return err
			}

			meta, err := loadInstallationMetadata(paths, stateRoot)
			if err != nil {
				return err
			}

			p, err := plan.BuildUninstall(meta, plan.UninstallOptions{SourceRoot: paths.SourceRoot})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if len(p.Actions) == 0 {
				fmt.Fprintf(out, "No recorded targets to uninstall in state root: %s\n", paths.StateRoot)
				return nil
			}

			renderUninstallPlan(out, p)
			renderModifiedHint(out, p, force)

			if dryRun {
				fmt.Fprintln(out, "\nDry run: no files changed.")
				return nil
			}

			if !willRemove(p, force) {
				fmt.Fprintln(out, "\nNothing to remove.")
				return nil
			}

			if !yes {
				confirmed, err := confirmUninstall(cmd.InOrStdin(), out)
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(out, "Uninstall canceled; no changes applied.")
					return nil
				}
			}

			res, err := uninstall.Apply(p, uninstall.Options{
				SourceRoot:     paths.SourceRoot,
				Home:           paths.Home,
				StateRoot:      paths.StateRoot,
				Force:          force,
				RestoreBackups: restoreBackups,
			})
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "\nRemoved %s.\n", pluralizeTargets(len(res.Removed)))
			if restoreBackups && len(res.RestoredSets) > 0 {
				fmt.Fprintf(out, "Restored %s from preserved Backup Sets.\n", pluralizeBackupSets(len(res.RestoredSets)))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root used to verify symlink ownership (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to uninstall from (default: the current user's home); use a sandbox path to avoid touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory holding Installation Metadata (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the Uninstall Plan without modifying files")
	cmd.Flags().BoolVar(&yes, "yes", false, "remove owned targets without prompting")
	cmd.Flags().BoolVar(&force, "force", false, "also remove copied targets whose content drifted from the recorded hash")
	cmd.Flags().BoolVar(&restoreBackups, "restore-backups", false, "restore each removed target's most recent Backup Set after removal")
	return cmd
}

// willRemove reports whether applying the plan would delete anything, honoring
// --force for drifted copies, so the command can short-circuit and skip the
// confirmation prompt when there is nothing to do.
func willRemove(p plan.UninstallPlan, force bool) bool {
	for _, action := range p.Actions {
		switch action.Status {
		case plan.UninstallRemove:
			return true
		case plan.UninstallModified:
			if force {
				return true
			}
		}
	}
	return false
}

// renderModifiedHint nudges the user about drifted copies: that --force is needed
// to remove them, or that --force will remove them. It stays quiet when there are
// none so a clean plan reads without noise.
func renderModifiedHint(w io.Writer, p plan.UninstallPlan, force bool) {
	modified := 0
	for _, action := range p.Actions {
		if action.Status == plan.UninstallModified {
			modified++
		}
	}
	if modified == 0 {
		return
	}
	if force {
		fmt.Fprintf(w, "\nNote: --force will remove %d modified target(s) whose content drifted from the recorded hash.\n", modified)
		return
	}
	fmt.Fprintf(w, "\nNote: %d modified target(s) will be skipped; re-run with --force to remove them.\n", modified)
}

func confirmUninstall(r io.Reader, w io.Writer) (bool, error) {
	fmt.Fprint(w, "\nProceed with uninstall? [y/N] ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read uninstall confirmation: %w", err)
		}
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	switch answer {
	case "y", "yes":
		return true, nil
	case "", "n", "no":
		return false, nil
	default:
		fmt.Fprintf(w, "Response %q is not yes/y; cancelling.\n", answer)
		return false, nil
	}
}
