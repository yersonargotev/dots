package cli

import (
	"runtime"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
)

func newPlanCommand() *cobra.Command {
	var (
		file       string
		profile    string
		sourceRoot string
		home       string
		stateRoot  string
	)

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Preview the changes dots would apply (dry run)",
		Long:  "plan computes the Install Plan for a profile against the current workstation state without modifying any files.",
		// Domain errors (e.g. unknown profile) are user-facing messages, not
		// misuse of the command, so do not dump the usage block on failure.
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

			meta, err := loadInstallationMetadata(paths, stateRoot)
			if err != nil {
				return err
			}

			// TODO: add an --os flag to preview a cross-platform install
			// (e.g. plan a Linux install from macOS); for now the OS is locked
			// to the host so the dry-run reflects this machine.
			p, err := plan.Build(*m, plan.Options{
				Profile:    profile,
				OS:         runtime.GOOS,
				SourceRoot: paths.SourceRoot,
				Home:       paths.Home,
				Metadata:   meta,
			})
			if err != nil {
				return err
			}

			renderPlan(cmd.OutOrStdout(), p)
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to plan")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "profile to plan")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to plan against (default: the current user's home); use a sandbox path to preview without touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata (default ~/.local/state/dots)")
	return cmd
}
