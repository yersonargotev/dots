package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/codexconfig"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/tui"
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
			if err := renderSkippedEntryHint(cmd.OutOrStdout(), *m, profile, runtime.GOOS); err != nil {
				return err
			}

			provPlan, err := provision.Build(*m, provision.Options{Profile: profile, OS: runtime.GOOS})
			if err != nil {
				return err
			}
			renderProvisionPlan(cmd.OutOrStdout(), provPlan)
			if err := renderSkippedProvisionerHint(cmd.OutOrStdout(), *m, profile, runtime.GOOS); err != nil {
				return err
			}

			if dryRun {
				return nil
			}

			applied, err := resolveAndApply(cmd, p, paths, yes, noTUI)
			if err != nil {
				return err
			}
			if !applied {
				return nil
			}

			return runProvisioners(cmd, *m, profile, paths.Home)
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

// runProvisioners executes the selected provisioners after dependency installs
// and file entries, in the same install run. It threads HOME from the resolved
// --home so a sandboxed install lands every tool-managed file under the
// temporary home. Apply stops at the first failing provisioner and returns the
// error, which the caller surfaces; the tool's own stdout/stderr are streamed
// through so its progress is visible.
func runProvisioners(cmd *cobra.Command, m manifest.Manifest, profile, home string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	runner := provisionExecRunner{
		ctx:    ctx,
		home:   home,
		stdin:  cmd.InOrStdin(),
		stdout: cmd.OutOrStdout(),
		stderr: cmd.ErrOrStderr(),
	}
	report, err := provision.Apply(m, provision.Options{Profile: profile, OS: runtime.GOOS}, lookupCommand, fontInstalled(runtime.GOOS, home), runner)
	renderProvisionReport(cmd.OutOrStdout(), report)
	if err != nil {
		return err
	}
	selected, err := provision.Select(m, provision.Options{Profile: profile, OS: runtime.GOOS})
	if err != nil {
		return err
	}
	if !selectedProvisionersAffectCodex(selected) {
		return nil
	}
	return codexconfig.EnsureCodeGraphMode(home)
}

func selectedProvisionersAffectCodex(selected []manifest.Provisioner) bool {
	for _, prov := range selected {
		if prov.Tool == "codex" {
			return true
		}
		if prov.Tool == "gentle-ai" && provisionerAgentsInclude(prov, "codex") {
			return true
		}
	}
	return false
}

func provisionerAgentsInclude(prov manifest.Provisioner, want string) bool {
	for _, agent := range prov.Spec.Agents {
		if agent == want {
			return true
		}
	}
	return false
}

// resolveAndApply resolves the plan's conflicts (via the TUI, text prompts, or
// the conservative --yes default) and applies it with Backup Set protection. It
// is shared by install and update so post-update installation reuses identical
// Conflict Resolution and filesystem machinery instead of reimplementing it.
func resolveAndApply(cmd *cobra.Command, p plan.Plan, paths resolvedPaths, yes, noTUI bool) (bool, error) {
	var (
		decisions map[string]install.ConflictDecision
		err       error
	)
	switch {
	case yes:
		// Non-interactive: conflicts deliberately default to skip.
	case noTUI:
		decisions, err = promptConflictDecisions(cmd, p, paths.Home, paths.SourceRoot)
		if err != nil {
			return false, err
		}
	default:
		decisions, err = resolveConflictsTUI(cmd, p, paths.Home, paths.SourceRoot)
		if errors.Is(err, tui.ErrCanceled) {
			fmt.Fprintln(cmd.OutOrStdout(), "Conflict resolution canceled; no changes applied.")
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}

	return true, install.Apply(p, install.Options{SourceRoot: paths.SourceRoot, Home: paths.Home, StateRoot: paths.StateRoot, ConflictDecisions: decisions})
}

// resolveConflictsTUI launches the Bubble Tea conflict resolver for the plan's
// conflicts. The diff provider reuses the same path-safety-validated rendering
// as the text prompt, so the TUI never reads files itself.
func resolveConflictsTUI(cmd *cobra.Command, p plan.Plan, home, sourceRoot string) (map[string]install.ConflictDecision, error) {
	actions := conflictActions(p)
	if len(actions) == 0 {
		return nil, nil
	}

	conflicts := make([]tui.Conflict, len(actions))
	for i, action := range actions {
		conflicts[i] = tui.Conflict{
			Target:   action.Target,
			Source:   action.Source,
			Strategy: action.Strategy,
		}
	}

	diff := conflictDiffProvider(actions, home, sourceRoot)
	return tui.ResolveConflicts(cmd.InOrStdin(), cmd.OutOrStdout(), conflicts, diff)
}

// conflictDiffProvider returns a tui.DiffFunc that renders the path-safety
// validated diff for a conflict by looking up its plan action. Keeping it
// separate makes the Conflict-to-Action mapping unit testable without driving
// the frame-throttled Bubble Tea renderer.
func conflictDiffProvider(actions []plan.Action, home, sourceRoot string) tui.DiffFunc {
	actionByTarget := make(map[string]plan.Action, len(actions))
	for _, action := range actions {
		actionByTarget[action.Target] = action
	}
	return func(c tui.Conflict) string {
		var buf bytes.Buffer
		renderConflictDiff(&buf, actionByTarget[c.Target], home, sourceRoot)
		return buf.String()
	}
}

// conflictActions returns the plan actions that require a conflict decision, so
// both the TUI and text-prompt resolution paths filter conflicts identically.
func conflictActions(p plan.Plan) []plan.Action {
	var actions []plan.Action
	for _, action := range p.Actions {
		if action.Status == plan.StatusConflict {
			actions = append(actions, action)
		}
	}
	return actions
}

func promptConflictDecisions(cmd *cobra.Command, p plan.Plan, home, sourceRoot string) (map[string]install.ConflictDecision, error) {
	decisions := map[string]install.ConflictDecision{}
	reader := bufio.NewReader(cmd.InOrStdin())
	for _, action := range conflictActions(p) {
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
