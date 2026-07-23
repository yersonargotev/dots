package cli

import (
	"runtime"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
)

func newStatusCommand() *cobra.Command {
	var (
		file       string
		profiles   []string
		extraTags  []string
		sourceRoot string
		home       string
		stateRoot  string
	)

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report alignment between the workstation and the Source of Truth",
		Long:  "status evaluates each managed entry against the filesystem and Installation Metadata, reporting whether targets are ok, missing, conflict, skipped, drifted, or unsupported.",
		// Domain errors (e.g. unknown profile) are user-facing messages, not
		// misuse of the command, so do not dump the usage block on failure.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths(home, sourceRoot, stateRoot)
			if err != nil {
				return err
			}

			m, err := loadManifestForCommand(cmd, file, paths.SourceRoot)
			if err != nil {
				return err
			}

			if stateRoot == "" || plan.InsideRoot(paths.StateRoot, paths.Home) {
				if err := plan.ValidatePathInsideHomeNoSymlinkEscape(paths.StateRoot, paths.Home, "state root"); err != nil {
					return err
				}
				if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(state.Path(paths.StateRoot), paths.Home, "installation metadata"); err != nil {
					return err
				}
			}

			meta, err := state.Load(state.Path(paths.StateRoot))
			if err != nil {
				return err
			}

			effective, err := selection.ResolveReadOnly(*m, profiles, extraTags, meta.InstalledSelection)
			if err != nil {
				return err
			}

			report, err := status.Build(*m, meta, status.Options{
				Profiles:   effective.Profiles,
				ExtraTags:  effective.ExtraTags,
				Selection:  &effective.Selection,
				OS:         runtime.GOOS,
				SourceRoot: paths.SourceRoot,
				Home:       paths.Home,
			})
			if err != nil {
				return err
			}
			report.Selection = &effective.Report

			provPlan, err := provision.Build(*m, provision.Options{Profiles: effective.Profiles, ExtraTags: effective.ExtraTags, Selection: &effective.Selection, OS: runtime.GOOS})
			if err != nil {
				return err
			}
			fullReport := statusReport{
				Profile:      report.Profile,
				Profiles:     report.Profiles,
				Tags:         report.Tags,
				Selection:    report.Selection,
				Entries:      report.Entries,
				Provisioners: provision.BuildStatus(provPlan, meta),
			}

			// Provisioner completion is now part of status because a profile can have
			// aligned Managed Entries while provisioning is still pending or failed.
			return renderOrEmit(cmd, fullReport, func() error {
				renderStatus(cmd.OutOrStdout(), report)
				renderStatusProvisioners(cmd.OutOrStdout(), provision.BuildStatus(provPlan, meta))
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to evaluate")
	cmd.Flags().StringArrayVarP(&profiles, "profile", "p", nil, "profile to evaluate")
	cmd.Flags().StringArrayVar(&extraTags, "tag", nil, "include an additional manifest tag; repeat to include multiple tags")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to evaluate (default: the current user's home); use a sandbox path to inspect without touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata (default ~/.local/state/dots)")
	return cmd
}
