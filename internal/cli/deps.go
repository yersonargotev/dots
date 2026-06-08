package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

func newDepsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deps",
		Short: "Inspect external tool Dependencies declared by managed entries",
		Long:  "deps reports which external Dependencies a profile needs, offers OS-aware installation guidance, and can execute missing install actions with explicit confirmation.",
	}
	cmd.AddCommand(newDepsCheckCommand())
	cmd.AddCommand(newDepsPlanCommand())
	cmd.AddCommand(newDepsInstallCommand())
	return cmd
}

func newDepsCheckCommand() *cobra.Command {
	var (
		file    string
		profile string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Report which declared Dependencies are present or missing",
		// Domain errors (e.g. unknown profile) are user-facing messages, not
		// misuse of the command, so do not dump the usage block on failure.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFile(file)
			if err != nil {
				return err
			}

			report, err := deps.Check(*m, deps.Options{
				Profile: profile,
				OS:      runtime.GOOS,
			}, lookupCommand)
			if err != nil {
				return err
			}

			renderDepsCheck(cmd.OutOrStdout(), report)
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to inspect")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "profile to inspect")
	return cmd
}

func newDepsPlanCommand() *cobra.Command {
	var (
		file    string
		profile string
		tier    string
	)

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show OS-aware installation guidance for missing Dependencies",
		Long:  "plan produces advisory installation guidance for the Dependencies a profile needs but the workstation is missing. It never installs packages.",
		// Domain errors (e.g. unknown profile or tier) are user-facing messages,
		// not misuse of the command, so do not dump the usage block on failure.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFile(file)
			if err != nil {
				return err
			}

			resolvedTier, err := resolveTier(tier)
			if err != nil {
				return err
			}

			report, err := deps.Plan(*m, deps.Options{
				Profile: profile,
				OS:      runtime.GOOS,
			}, lookupCommand, resolvedTier)
			if err != nil {
				return err
			}

			renderDepsPlan(cmd.OutOrStdout(), report)
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to inspect")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "profile to inspect")
	cmd.Flags().StringVar(&tier, "tier", "", "override the dependency plan tier (homebrew, debian, fedora, arch, generic); default: auto-detect from the host")
	return cmd
}

func newDepsInstallCommand() *cobra.Command {
	var (
		file    string
		profile string
		tier    string
		dryRun  bool
		yes     bool
	)

	cmd := &cobra.Command{
		Use:          "install",
		Short:        "Install missing Dependencies with explicit confirmation",
		Long:         "install previews dependency install actions, asks for confirmation by default, and executes installable actions only after explicit approval. Use --dry-run to preview without prompting or --yes for non-interactive execution.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.LoadFile(file)
			if err != nil {
				return err
			}

			resolvedTier, err := resolveTier(tier)
			if err != nil {
				return err
			}

			options := deps.Options{
				Profile: profile,
				OS:      runtime.GOOS,
			}

			report, err := deps.InstallDryRun(*m, options, lookupCommand, resolvedTier)
			if err != nil {
				return err
			}

			if dryRun {
				renderDepsInstallDryRun(cmd.OutOrStdout(), report)
				return nil
			}

			if yes {
				if !hasInstallablePreviewAction(report) {
					renderDepsInstallPreview(cmd.OutOrStdout(), report)
					return nil
				}
				return runDepsInstall(cmd, *m, options, resolvedTier)
			}

			renderDepsInstallPreview(cmd.OutOrStdout(), report)
			if !hasInstallablePreviewAction(report) {
				return nil
			}
			confirmed, err := confirmDepsInstall(cmd.InOrStdin(), cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), "Dependency installation cancelled.")
				return nil
			}
			return runDepsInstall(cmd, *m, options, resolvedTier)
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to inspect")
	cmd.Flags().StringVarP(&profile, "profile", "p", "default", "profile to inspect")
	cmd.Flags().StringVar(&tier, "tier", "", "override the dependency install tier (homebrew, debian, fedora, arch, generic); default: auto-detect from the host")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview dependency install actions without executing package managers")
	cmd.Flags().BoolVar(&yes, "yes", false, "execute dependency install actions without interactive confirmation")
	return cmd
}

func runDepsInstall(cmd *cobra.Command, m manifest.Manifest, options deps.Options, tier deps.Tier) error {
	report, err := deps.Install(m, options, lookupCommand, tier, depsExecRunner{
		ctx:    cmd.Context(),
		stdin:  cmd.InOrStdin(),
		stdout: cmd.OutOrStdout(),
		stderr: cmd.ErrOrStderr(),
	})
	if report.Profile != "" || len(report.Items) > 0 {
		renderDepsInstall(cmd.OutOrStdout(), report)
	}
	return err
}

func confirmDepsInstall(r io.Reader, w io.Writer) (bool, error) {
	fmt.Fprint(w, "Proceed with dependency installation? [y/N] ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read dependency install confirmation: %w", err)
		}
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	switch answer {
	case "y", "yes":
		return true, nil
	case "", "n", "no":
		return false, nil
	default:
		fmt.Fprintf(w, "Response %q is not yes/y; cancelling.\n", answer)
		return false, nil
	}
}

type depsExecRunner struct {
	ctx    context.Context
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (r depsExecRunner) Run(executable string, args []string) error {
	cmd := exec.CommandContext(r.ctx, executable, args...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	return cmd.Run()
}

// lookupCommand reports whether a command resolves on the current PATH. It is
// the production Lookup used by the deps commands.
func lookupCommand(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// resolveTier returns the Dependency Plan Tier, either from an explicit override
// or by detecting it from the host OS and /etc/os-release.
func resolveTier(override string) (deps.Tier, error) {
	override = strings.ToLower(strings.TrimSpace(override))
	if override == "" {
		return detectTier(), nil
	}
	switch deps.Tier(override) {
	case deps.TierHomebrew, deps.TierDebian, deps.TierFedora, deps.TierArch, deps.TierGeneric:
		return deps.Tier(override), nil
	default:
		return "", fmt.Errorf("unknown tier %q: must be one of homebrew, debian, fedora, arch, generic", override)
	}
}

// detectTier resolves the Dependency Plan Tier for the current host. On Linux it
// reads /etc/os-release to distinguish distributions; a missing or unreadable
// file falls back to generic guidance.
func detectTier() deps.Tier {
	var release deps.OSRelease
	if runtime.GOOS == "linux" {
		if content, err := os.ReadFile("/etc/os-release"); err == nil {
			release = deps.ParseOSRelease(string(content))
		}
	}
	return deps.ResolveTier(runtime.GOOS, release)
}
