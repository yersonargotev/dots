package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/completion"
)

const (
	selectionProfileHelp = "select an ordered Profile preset; repeat to compose presets; any explicit Profile/Tag request is the complete selection and is not merged with Installed Selection"
	selectionTagHelp     = "select an independently meaningful Tag capability; repeat to compose a complete selection without a Profile; any explicit Profile/Tag request is the complete selection and is not merged with Installed Selection"
)

func registerSelectionFlagCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("profile", completeSelectionFlag(completion.Profile))
	_ = cmd.RegisterFlagCompletionFunc("tag", completeSelectionFlag(completion.Tag))
}

func completeSelectionFlag(kind completion.Kind) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		path, err := selectionCompletionManifestPath(cmd)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		values, err := completion.Complete(path, kind, toComplete)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return values, cobra.ShellCompDirectiveNoFileComp
	}
}

// selectionCompletionManifestPath mirrors command manifest resolution without
// preparing or refreshing the Installed Repository.
func selectionCompletionManifestPath(cmd *cobra.Command) (string, error) {
	fileFlag := cmd.Flag("file")
	if fileFlag == nil {
		return "", fmt.Errorf("completion command %q has no --file flag", cmd.CommandPath())
	}
	file := fileFlag.Value.String()
	if fileFlag.Changed {
		return file, nil
	}

	sourceRoot := stringFlagValue(cmd, "source-root")
	if sourceRoot == "" {
		home := stringFlagValue(cmd, "home")
		if home == "" {
			resolvedHome, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			home = resolvedHome
		}
		sourceRoot = defaultSourceRoot(home)
	}
	return filepath.Join(sourceRoot, file), nil
}

func stringFlagValue(cmd *cobra.Command, name string) string {
	flag := cmd.Flag(name)
	if flag == nil {
		return ""
	}
	return flag.Value.String()
}
