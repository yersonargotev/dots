package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/version"
)

func NewRootCommand() *cobra.Command {
	var showVersion bool
	root := &cobra.Command{
		Use:   "dots",
		Short: "Dotfiles CLI",
		Long:  "dots is the Dotfiles CLI for managing repository-owned workstation configuration.",
		// Run owns error presentation and exit-code mapping so a structured
		// FindingsError is never printed as an "Error:" line. The read-only
		// diagnostic commands set SilenceUsage themselves, so the root only
		// silences errors and leaves cobra's usage-on-misuse intact for the
		// other commands. See internal/cli/run.go.
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				if wantsJSON(cmd) {
					return emitOK(cmd, versionReport{Version: version.Value})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "dots %s\n", version.Value)
				return nil
			}
			if wantsJSON(cmd) {
				return fmt.Errorf("--output json requires a command that emits a machine-readable result")
			}
			return cmd.Help()
		},
	}
	root.Flags().BoolVar(&showVersion, "version", false, "version for dots")

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
	root.AddCommand(newUninstallCommand())
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
			if wantsJSON(cmd) {
				return emitOK(cmd, versionReport{Version: version.Value})
			}
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
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := manifest.LoadFile(file); err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return emitOK(cmd, manifestValidateReport{File: file, Valid: true})
			}
			fmt.Fprintln(cmd.OutOrStdout(), "manifest is valid")
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to validate")
	return cmd
}

func preflightOutputMode(root *cobra.Command, args []string, stdout, stderr anyWriter) (int, bool) {
	value, ok, missing := outputArgValue(args)
	if missing {
		fmt.Fprintf(stderr, "Error: flag needs an argument: --%s\n", outputFlag)
		return ExitError, true
	}
	if !ok {
		return ExitOK, false
	}
	if value != outputText && value != outputJSON {
		fmt.Fprintf(stderr, "Error: invalid --%s %q: must be %q or %q\n", outputFlag, value, outputText, outputJSON)
		return ExitError, true
	}
	if value != outputJSON {
		return ExitOK, false
	}

	if cmdPath, unsupported := unsupportedJSONSurface(root, args); unsupported {
		emitRawError(stdout, cmdPath, fmt.Errorf("--output json is not supported for help or shell completion output"))
		return ExitError, true
	}

	cmd, _, err := root.Find(args)
	if err == nil && !cmd.Runnable() {
		emitRawError(stdout, commandName(cmd), fmt.Errorf("--output json requires a subcommand that emits a machine-readable result"))
		return ExitError, true
	}
	return ExitOK, false
}

type anyWriter interface {
	Write([]byte) (int, error)
}

func outputArgValue(args []string) (value string, ok bool, missing bool) {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--"+outputFlag {
			if i+1 >= len(args) {
				return "", true, true
			}
			value = args[i+1]
			ok = true
			continue
		}
		if strings.HasPrefix(arg, "--"+outputFlag+"=") {
			value = strings.TrimPrefix(arg, "--"+outputFlag+"=")
			ok = true
		}
	}
	return value, ok, false
}

func unsupportedJSONSurface(root *cobra.Command, args []string) (string, bool) {
	tokens := commandTokens(args)
	if helpFlagRequested(args) {
		return commandPathForArgs(root, args), true
	}
	if len(tokens) == 0 {
		return "", false
	}
	if tokens[0] == "help" {
		return "help", true
	}
	if tokens[0] == "completion" {
		if len(tokens) > 1 {
			return "completion " + tokens[1], true
		}
		return "completion", true
	}
	return "", false
}

func helpFlagRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if isHelpFlag(arg) {
			return true
		}
	}
	return false
}

func isHelpFlag(arg string) bool {
	if arg == "--help" || arg == "-h" {
		return true
	}
	for _, prefix := range []string{"--help=", "-h="} {
		if strings.HasPrefix(arg, prefix) {
			v, err := strconv.ParseBool(strings.TrimPrefix(arg, prefix))
			return err == nil && v
		}
	}
	return false
}

func commandPathForArgs(root *cobra.Command, args []string) string {
	cmd, _, err := root.Find(withoutPreflightOnlyFlags(args))
	if err == nil {
		name := commandName(cmd)
		if name == "" {
			return "dots"
		}
		return name
	}
	tokens := commandTokens(args)
	if len(tokens) == 0 {
		return "dots"
	}
	return strings.Join(tokens, " ")
}

func withoutPreflightOnlyFlags(args []string) []string {
	filtered := make([]string, 0, len(args))
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--"+outputFlag {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "--"+outputFlag+"=") || isHelpFlag(arg) {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

func commandTokens(args []string) []string {
	tokens := make([]string, 0, len(args))
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "--" {
			break
		}
		if arg == "--"+outputFlag {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "--"+outputFlag+"=") {
			continue
		}
		if flagConsumesValue(arg) {
			skipNext = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		tokens = append(tokens, arg)
	}
	return tokens
}

func flagConsumesValue(arg string) bool {
	switch arg {
	case "--file", "-f", "--home", "--source-root", "--state-root", "--profile", "--tier":
		return true
	default:
		return false
	}
}
