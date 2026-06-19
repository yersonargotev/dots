package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/version"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:     "dots",
		Short:   "Dotfiles CLI",
		Long:    "dots is the Dotfiles CLI for managing repository-owned workstation configuration.",
		Version: version.Value,
		// Run owns error presentation and exit-code mapping so a structured
		// FindingsError is never printed as an "Error:" line. The read-only
		// diagnostic commands set SilenceUsage themselves, so the root only
		// silences errors and leaves cobra's usage-on-misuse intact for the
		// other commands. See internal/cli/run.go.
		SilenceErrors: true,
	}
	root.SetVersionTemplate("dots {{.Version}}\n")

	// --output is the Agent Output Contract surface selector. It is persistent so
	// it is uniformly available and documented once; only the read-only
	// diagnostic commands emit a JSON envelope today.
	root.PersistentFlags().String(outputFlag, outputText, "output format for command results: text or json")
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		v, _ := cmd.Flags().GetString(outputFlag)
		if v != outputText && v != outputJSON {
			return fmt.Errorf("invalid --%s %q: must be %q or %q", outputFlag, v, outputText, outputJSON)
		}
		return nil
	}

	root.AddCommand(newVersionCommand())
	root.AddCommand(newManifestCommand())
	root.AddCommand(newPlanCommand())
	root.AddCommand(newInstallCommand())
	root.AddCommand(newUpdateCommand())
	root.AddCommand(newStatusCommand())
	root.AddCommand(newBackupsCommand())
	root.AddCommand(newDepsCommand())
	root.AddCommand(newDoctorCommand())
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the dots version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "dots %s\n", version.Value)
			return nil
		},
	}
}

func newManifestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Work with the dots install manifest",
	}
	cmd.AddCommand(newManifestValidateCommand())
	return cmd
}

func newManifestValidateCommand() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a dots.yaml manifest",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := manifest.LoadFile(file); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "manifest is valid")
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to validate")
	return cmd
}
