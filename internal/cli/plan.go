package cli

import (
	"runtime"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func newPlanCommand() *cobra.Command {
	var (
		file       string
		profiles   []string
		extraTags  []string
		sourceRoot string
		home       string
		stateRoot  string
	)

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview the changes dots would apply (dry run)",
		Long:  "plan computes the Install Plan for the complete Profile/Tag selection against the current workstation state without modifying any files.",
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

			meta, err := loadInstallationMetadata(paths, stateRoot)
			if err != nil {
				return err
			}

			effective, err := resolveReadOnlySelection(*m, meta, profiles, extraTags, readOnlySelectionOptions{
				Home: paths.Home, SourceRoot: paths.SourceRoot, StatePath: state.Path(paths.StateRoot),
				XDGStateHome: paths.XDGStateHome,
			})
			if err != nil {
				return err
			}

			// TODO: add an --os flag to preview a cross-platform install
			// (e.g. plan a Linux install from macOS); for now the OS is locked
			// to the host so the dry-run reflects this machine.
			p, err := plan.Build(*m, plan.Options{
				Profiles:     effective.Profiles,
				ExtraTags:    effective.ExtraTags,
				Selection:    &effective.Selection,
				OS:           runtime.GOOS,
				SourceRoot:   paths.SourceRoot,
				Home:         paths.Home,
				XDGStateHome: paths.XDGStateHome,
				Metadata:     meta,
			})
			if err != nil {
				return err
			}
			p.Selection = &effective.Report

			return renderOrEmit(cmd, p, func() error {
				renderPlan(cmd.OutOrStdout(), p)
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to plan")
	cmd.Flags().StringArrayVarP(&profiles, "profile", "p", nil, selectionProfileHelp)
	cmd.Flags().StringArrayVar(&extraTags, "tag", nil, selectionTagHelp)
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to plan against (default: the current user's home); use a sandbox path to preview without touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata (default ~/.local/state/dots)")
	registerSelectionFlagCompletion(cmd)
	return cmd
}
