package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/gitrepo"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
)

func newUpdateCommand() *cobra.Command {
	var (
		file       string
		profile    string
		sourceRoot string
		home       string
		stateRoot  string
		dryRun     bool
		yes        bool
		noTUI      bool
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the Installed Repository and re-run the safe install flow",
		Long: "update fast-forwards the Installed Repository (default ~/.local/share/dots) to its " +
			"upstream, then recomputes the Install Plan so managed configuration stays aligned with " +
			"the Source of Truth. It refuses to touch a repository with local changes and only ever " +
			"applies a clean fast-forward, never a merge or rebase. Conflicts during the post-update " +
			"install are resolved exactly like dots install, creating a Backup Set before any replacement.",
		// Domain failures (dirty repo, divergence, conflicts) are user-facing
		// conditions, not command misuse, so do not dump the usage block.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths(home, sourceRoot, stateRoot)
			if err != nil {
				return err
			}

			if !gitrepo.IsRepo(paths.SourceRoot) {
				return fmt.Errorf("installed repository %s is not a git repository; clone the Source of Truth before updating", paths.SourceRoot)
			}
			clean, err := gitrepo.IsClean(paths.SourceRoot)
			if err != nil {
				return err
			}
			if !clean {
				return fmt.Errorf("installed repository %s has local changes; commit or discard them before updating", paths.SourceRoot)
			}

			out := cmd.OutOrStdout()
			if err := refreshRepository(out, paths.SourceRoot, dryRun); err != nil {
				return err
			}

			// Load the manifest after the fast-forward so a manifest change pulled
			// from upstream is honored by the recomputed plan. The default
			// manifest belongs to the Installed Repository, not the caller's cwd;
			// explicit --file values keep their normal caller-relative behavior.
			manifestPath := file
			if !cmd.Flags().Changed("file") {
				manifestPath = filepath.Join(paths.SourceRoot, file)
			}
			m, err := manifest.LoadFile(manifestPath)
			if err != nil {
				return err
			}
			p, err := plan.Build(*m, plan.Options{
				Profile:    profile,
				OS:         runtime.GOOS,
				SourceRoot: paths.SourceRoot,
				Home:       paths.Home,
			})
			if err != nil {
				return err
			}

			renderPlan(out, p)
			if dryRun {
				return nil
			}

			_, err = resolveAndApply(cmd, p, paths, yes, noTUI)
			return err
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to install after updating")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "profile to install after updating")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root to update (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to install into (default: the current user's home); use a sandbox path to avoid touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata and Backup Sets (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "fetch and report the available update and Install Plan without fast-forwarding the working tree or installing files")
	cmd.Flags().BoolVar(&yes, "yes", false, "apply safe install actions without prompting; conflicts default to skip")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "use text prompts instead of the interactive TUI for conflict resolution")
	return cmd
}

// refreshRepository previews (dry run) or applies (real run) the fast-forward
// update of the Installed Repository, reporting exactly what changed. It maps
// the gitrepo divergence sentinel to a user-facing message that points back to
// manual resolution, keeping dots out of automatic merge/rebase behavior.
func refreshRepository(out io.Writer, sourceRoot string, dryRun bool) error {
	var (
		upd gitrepo.Update
		err error
	)
	if dryRun {
		upd, err = gitrepo.Preview(sourceRoot)
	} else {
		upd, err = gitrepo.FastForward(sourceRoot)
	}
	if errors.Is(err, gitrepo.ErrNotFastForward) {
		return fmt.Errorf("installed repository %s has diverged from its upstream and cannot be fast-forwarded; resolve it manually with git", sourceRoot)
	}
	if err != nil {
		return err
	}
	renderUpdate(out, upd, dryRun)
	return nil
}

// renderUpdate writes a deterministic summary of the fast-forward so the user
// sees which commits were (or would be) applied before the Install Plan.
func renderUpdate(out io.Writer, upd gitrepo.Update, dryRun bool) {
	if !upd.Changed() {
		fmt.Fprintf(out, "Installed Repository already up to date at %s.\n\n", upd.OldRev)
		return
	}

	if dryRun {
		fmt.Fprintf(out, "Installed Repository can fast-forward %s -> %s:\n", upd.OldRev, upd.NewRev)
	} else {
		fmt.Fprintf(out, "Updated Installed Repository %s -> %s:\n", upd.OldRev, upd.NewRev)
	}
	for _, line := range upd.Incoming {
		fmt.Fprintf(out, "  %s\n", line)
	}
	if dryRun {
		fmt.Fprintln(out, "\n(dry run: working tree and managed files not modified)")
	}
	fmt.Fprintln(out)
}
