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
	}
	root.SetVersionTemplate("dots {{.Version}}\n")

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
