package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/manifest"
)

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "dots",
		Short: "Dotfiles CLI",
		Long:  "dots is the Dotfiles CLI for managing repository-owned workstation configuration.",
	}

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
