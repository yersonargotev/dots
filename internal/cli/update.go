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
	"github.com/yersonargotev/dots/internal/provision"
)

type updateOptions struct {
	file       string
	profiles   []string
	extraTags  []string
	sourceRoot string
	home       string
	stateRoot  string
	dryRun     bool
	yes        bool
	noTUI      bool
}

func newUpdateCommand() *cobra.Command {
	var (
		file       string
		profiles   []string
		extraTags  []string
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
			"upstream, then recomputes the Install Plan and re-runs the selected provisioners so managed " +
			"configuration stays aligned with the Source of Truth. If the Installed Repository has local " +
			"changes, update preserves them in Git's stash before continuing, and only ever applies a clean " +
			"fast-forward, never a merge or rebase. Conflicts " +
			"during the post-update install are resolved exactly like dots install, creating a Backup Set " +
			"before any replacement.",
		// Domain failures (dirty repo, divergence, conflicts) are user-facing
		// conditions, not command misuse, so do not dump the usage block.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := runUpdateWorkflow(cmd, updateOptions{file: file, profiles: profiles, extraTags: extraTags, sourceRoot: sourceRoot, home: home, stateRoot: stateRoot, dryRun: dryRun, yes: yes, noTUI: noTUI}, true)
			return err
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to install after updating")
	cmd.Flags().StringArrayVarP(&profiles, "profile", "p", nil, "profile to install after updating")
	cmd.Flags().StringArrayVar(&extraTags, "tag", nil, "include an additional manifest tag; repeat to include multiple tags")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root to update (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to install into (default: the current user's home); use a sandbox path to avoid touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata and Backup Sets (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "fetch and report the available update and Install Plan without fast-forwarding the working tree or installing files")
	cmd.Flags().BoolVar(&yes, "yes", false, "apply safe install actions without prompting; conflicts default to skip")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "use text prompts instead of the interactive TUI for conflict resolution")
	return cmd
}

// refreshRepository previews (dry run) or applies (real run) the fast-forward
// update of the Installed Repository, preserving local changes before a real
// update so customers do not need to recover with manual Git commands. It maps
// the gitrepo divergence sentinel to a user-facing message that points back to
// manual resolution, keeping dots out of automatic merge/rebase behavior.
func refreshRepository(out io.Writer, sourceRoot string, dryRun bool) (gitrepo.Update, error) {
	var (
		upd gitrepo.Update
		err error
	)
	if dryRun {
		upd, err = gitrepo.Preview(sourceRoot)
	} else {
		upd, err = gitrepo.FastForwardPreservingLocalChanges(sourceRoot)
	}
	if errors.Is(err, gitrepo.ErrNotFastForward) {
		return gitrepo.Update{}, fmt.Errorf("installed repository %s has diverged from its upstream and cannot be fast-forwarded; resolve it manually with git", sourceRoot)
	}
	if err != nil {
		return gitrepo.Update{}, err
	}
	renderUpdate(out, upd, dryRun)
	return upd, nil
}

// renderUpdate writes a deterministic summary of the fast-forward so the user
// sees which commits were (or would be) applied before the Install Plan.
func renderUpdate(out io.Writer, upd gitrepo.Update, dryRun bool) {
	if upd.PreservedChanges != "" {
		fmt.Fprintf(out, "Preserved local Installed Repository changes in %s.\n", upd.PreservedChanges)
	}
	if upd.AttachedBranch != "" {
		if dryRun {
			fmt.Fprintf(out, "Detached Installed Repository would attach to %s.\n", upd.AttachedBranch)
		} else {
			fmt.Fprintf(out, "Attached detached Installed Repository to %s.\n", upd.AttachedBranch)
		}
	}
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

func validateProfileSelectionFile(manifestPath string, profiles []string, extraTags []string) error {
	m, err := manifest.LoadFile(manifestPath)
	if err != nil {
		return err
	}
	_, err = manifest.ResolveSelection(*m, profiles, extraTags)
	return err
}

func runUpdateWorkflow(cmd *cobra.Command, opts updateOptions, emit bool) (updateReport, error) {
	paths, err := resolvePaths(opts.home, opts.sourceRoot, opts.stateRoot)
	if err != nil {
		return updateReport{}, err
	}
	if !gitrepo.IsRepo(paths.SourceRoot) {
		return updateReport{}, fmt.Errorf("installed repository %s is not a git repository; clone the Source of Truth before updating", paths.SourceRoot)
	}
	manifestPath := opts.file
	if !cmd.Flags().Changed("file") {
		manifestPath = filepath.Join(paths.SourceRoot, opts.file)
	}
	if err := validateProfileSelectionFile(manifestPath, opts.profiles, opts.extraTags); err != nil {
		return updateReport{}, err
	}

	out := cmd.OutOrStdout()
	var update gitrepo.Update
	if wantsJSON(cmd) {
		if opts.dryRun {
			update, err = gitrepo.Preview(paths.SourceRoot)
		} else {
			if !opts.yes {
				return updateReport{}, rejectInteractiveJSON(cmd)
			}
			update, err = gitrepo.FastForwardPreservingLocalChanges(paths.SourceRoot)
		}
		if errors.Is(err, gitrepo.ErrNotFastForward) {
			return updateReport{}, fmt.Errorf("installed repository %s has diverged from its upstream and cannot be fast-forwarded; resolve it manually with git", paths.SourceRoot)
		}
		if err != nil {
			return updateReport{}, err
		}
	} else {
		update, err = refreshRepository(out, paths.SourceRoot, opts.dryRun)
		if err != nil {
			return updateReport{}, err
		}
	}

	m, err := manifest.LoadFile(manifestPath)
	if err != nil {
		return updateReport{}, err
	}
	meta, err := loadInstallationMetadata(paths, opts.stateRoot)
	if err != nil {
		return updateReport{}, err
	}
	p, err := plan.Build(*m, plan.Options{Profiles: opts.profiles, ExtraTags: opts.extraTags, OS: runtime.GOOS, SourceRoot: paths.SourceRoot, Home: paths.Home, Metadata: meta})
	if err != nil {
		return updateReport{}, err
	}
	provPlan, err := provision.Build(*m, provision.Options{Profiles: opts.profiles, ExtraTags: opts.extraTags, OS: runtime.GOOS})
	if err != nil {
		return updateReport{}, err
	}
	report := updateReport{DryRun: opts.dryRun, Update: update, Plan: p, Provisioners: provPlan}
	if wantsJSON(cmd) {
		if opts.dryRun && emit {
			return report, emitOK(cmd, report)
		}
	} else {
		renderPlan(out, p)
		if err := renderSkippedEntryHint(out, *m, opts.profiles, runtime.GOOS); err != nil {
			return updateReport{}, err
		}
		renderProvisionPlan(out, provPlan)
		if err := renderSkippedProvisionerHint(out, *m, opts.profiles, runtime.GOOS); err != nil {
			return updateReport{}, err
		}
	}
	if opts.dryRun {
		return report, nil
	}
	applied, err := resolveAndApply(cmd, p, paths, opts.yes, opts.noTUI, false)
	if err != nil {
		return updateReport{}, err
	}
	if applied {
		if _, err := runProvisioners(cmd, *m, opts.profiles, opts.extraTags, paths.Home, paths.StateRoot, paths.SourceRoot); err != nil {
			return updateReport{}, err
		}
	}
	if wantsJSON(cmd) && emit {
		return report, emitOK(cmd, report)
	}
	return report, nil
}
