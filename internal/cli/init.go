package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/bootstrap"
	"github.com/yersonargotev/dots/internal/version"
)

func newInitCommand() *cobra.Command {
	var (
		sourceRoot    string
		home          string
		repositoryURL string
		repositoryRef string
	)

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the default Installed Repository",
		Long:  "init clones the Source of Truth into the Installed Repository so package-manager installs have the manifest and managed configuration available.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths(home, sourceRoot, "")
			if err != nil {
				return err
			}
			result, err := bootstrap.Ensure(bootstrap.Options{
				SourceRoot:    paths.SourceRoot,
				RepositoryURL: repositoryURL,
				RepositoryRef: defaultInitRepositoryRef(repositoryRef, version.Value),
			})
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return emitOK(cmd, initReport(result))
			}
			if result.Cloned {
				fmt.Fprintf(cmd.OutOrStdout(), "Initialized Installed Repository at %s\n", result.SourceRoot)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed Repository already initialized at %s\n", result.SourceRoot)
			return nil
		},
	}

	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "home directory used to resolve the default Installed Repository")
	cmd.Flags().StringVar(&repositoryURL, "repository-url", bootstrap.DefaultRepositoryURL, "Source of Truth Git URL")
	cmd.Flags().StringVar(&repositoryRef, "repository-ref", "", "optional Source of Truth Git ref to clone")
	return cmd
}

func defaultInitRepositoryRef(explicitRef, currentVersion string) string {
	if strings.TrimSpace(explicitRef) != "" {
		return explicitRef
	}
	if strings.HasPrefix(currentVersion, "v") {
		return currentVersion
	}
	return ""
}
