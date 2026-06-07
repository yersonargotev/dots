package cli

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"

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
		yes        bool
		noTUI      bool
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

			var decisions map[string]install.ConflictDecision
			if noTUI && !yes {
				decisions, err = promptConflictDecisions(cmd, p, paths.Home, paths.SourceRoot)
				if err != nil {
					return err
				}
			}

			return install.Apply(p, install.Options{SourceRoot: paths.SourceRoot, Home: paths.Home, StateRoot: paths.StateRoot, ConflictDecisions: decisions})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to install")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "profile to install")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to install into (default: the current user's home); use a sandbox path to avoid touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the Install Plan without modifying files")
	cmd.Flags().BoolVar(&yes, "yes", false, "apply safe install actions without prompting; conflicts default to skip")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "use text prompts instead of the interactive TUI for conflict resolution")
	return cmd
}

func promptConflictDecisions(cmd *cobra.Command, p plan.Plan, home, sourceRoot string) (map[string]install.ConflictDecision, error) {
	decisions := map[string]install.ConflictDecision{}
	reader := bufio.NewReader(cmd.InOrStdin())
	for _, action := range p.Actions {
		if action.Status != plan.StatusConflict {
			continue
		}
		decision, err := promptConflictDecision(cmd, reader, action, home, sourceRoot)
		if err != nil {
			return nil, err
		}
		if decision != install.DecisionSkip {
			decisions[action.Target] = decision
		}
	}
	return decisions, nil
}

func promptConflictDecision(cmd *cobra.Command, reader *bufio.Reader, action plan.Action, home, sourceRoot string) (install.ConflictDecision, error) {
	for {
		fmt.Fprintf(cmd.OutOrStdout(), "Resolve conflict for %s [s]kip/[r]eplace/[a]dopt/[d]iff: ", action.Target)
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return "", fmt.Errorf("read conflict decision: %w", err)
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		switch choice {
		case "", "s", "skip":
			return install.DecisionSkip, nil
		case "r", "replace":
			return install.DecisionReplace, nil
		case "a", "adopt":
			return install.DecisionAdopt, nil
		case "d", "diff":
			renderConflictDiff(cmd.OutOrStdout(), action, home, sourceRoot)
		default:
			fmt.Fprintln(cmd.OutOrStdout(), "Please choose skip, replace, adopt, or diff.")
		}
	}
}

func renderConflictDiff(w interface{ Write([]byte) (int, error) }, action plan.Action, home, sourceRoot string) {
	fmt.Fprintf(w, "--- target: %s\n", action.Target)
	writeTargetFileForPromptDiff(w, action.Target, home)
	fmt.Fprintf(w, "--- source: %s\n", action.Source)
	writeSourceFileForPromptDiff(w, action, sourceRoot)
}

func writeTargetFileForPromptDiff(w interface{ Write([]byte) (int, error) }, path, home string) {
	if err := plan.ValidateFilePathInsideHomeNoSymlinkEscape(path, home, "prompt diff target"); err != nil {
		fmt.Fprintln(w, "(target content not shown: unsafe or non-regular path)")
		return
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintln(w, "(target content not shown: unsafe or non-regular path)")
		return
	}
	writeFileForPromptDiff(w, path, "target")
}

func writeSourceFileForPromptDiff(w interface{ Write([]byte) (int, error) }, action plan.Action, sourceRoot string) {
	source, err := plan.ResolveSource(action.Source, sourceRoot)
	if err != nil {
		fmt.Fprintln(w, "(source content not shown: unsafe or unreadable source)")
		return
	}
	if action.ResolvedSource != "" && action.ResolvedSource != source {
		fmt.Fprintln(w, "(source content not shown: unsafe or unreadable source)")
		return
	}
	if err := plan.ValidateResolvedSource(source, sourceRoot); err != nil {
		fmt.Fprintln(w, "(source content not shown: unsafe or unreadable source)")
		return
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		fmt.Fprintln(w, "(source content not shown: unsafe or unreadable source)")
		return
	}
	writeFileForPromptDiff(w, source, "source")
}

func writeFileForPromptDiff(w interface{ Write([]byte) (int, error) }, path, label string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(w, "(%s content not shown: unsafe or unreadable path)\n", label)
		return
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	_, _ = w.Write(data)
}
