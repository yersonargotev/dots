package cli

import (
	"runtime"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
)

func newInstallCommand() *cobra.Command {
	var (
		file       string
		profile    string
		sourceRoot string
		home       string
		stateRoot  string
		dryRun     bool
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install repository-managed dotfiles",
		Long:  "install computes and shows an Install Plan, then applies safe create actions unless --dry-run is set.",
		// Domain installation failures are user-facing conflicts, not command misuse.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFile(file)
			if err != nil {
				return err
			}

			paths, err := resolvePaths(home, sourceRoot, stateRoot)
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

			renderPlan(cmd.OutOrStdout(), p)
			if dryRun {
				return nil
			}

			return install.Apply(p, install.Options{SourceRoot: paths.SourceRoot, Home: paths.Home, StateRoot: paths.StateRoot})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to install")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "profile to install")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to install into (default: the current user's home); use a sandbox path to avoid touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the Install Plan without modifying files")
	return cmd
}
