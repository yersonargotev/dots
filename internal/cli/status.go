package cli

import (
	"runtime"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
)

func newStatusCommand() *cobra.Command {
	var (
		file       string
		profile    string
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
			m, err := manifest.LoadFile(file)
			if err != nil {
				return err
			}

			paths, err := resolvePaths(home, sourceRoot, stateRoot)
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

			report, err := status.Build(*m, meta, status.Options{
				Profile:    profile,
				OS:         runtime.GOOS,
				SourceRoot: paths.SourceRoot,
				Home:       paths.Home,
			})
			if err != nil {
				return err
			}

			renderStatus(cmd.OutOrStdout(), report)
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to evaluate")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "profile to evaluate")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to evaluate (default: the current user's home); use a sandbox path to inspect without touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata (default ~/.local/state/dots)")
	return cmd
}
