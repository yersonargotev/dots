package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

func newDepsCommand() *cobra.Command {
	var profile string

	cmd := &cobra.Command{
		Use:   "deps",
		Short: "Inspect external tool Dependencies declared by managed entries",
		Long:  "deps reports which external Dependencies a profile needs, offers OS-aware installation guidance, and can execute missing install actions with explicit confirmation.",
	}
	cmd.PersistentFlags().StringVarP(&profile, "profile", "p", "default", "profile to inspect")
	cmd.AddCommand(newDepsCheckCommand(&profile))
	cmd.AddCommand(newDepsPlanCommand(&profile))
	cmd.AddCommand(newDepsInstallCommand(&profile))
	return cmd
}

func newDepsCheckCommand(profile *string) *cobra.Command {
	var (
		file string
		home string
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Report which declared Dependencies are present or missing",
		// Domain errors (e.g. unknown profile) are user-facing messages, not
		// misuse of the command, so do not dump the usage block on failure.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedHome := resolveDepsHome(home)
			m, err := manifest.LoadFile(resolveManifestPath(cmd, file, defaultSourceRoot(resolvedHome)))
			if err != nil {
				return err
			}

			report, err := deps.Check(*m, deps.Options{
				Profile: *profile,
				OS:      runtime.GOOS,
			}, lookupCommand, fontInstalled(runtime.GOOS, resolvedHome))
			if err != nil {
				return err
			}

			return renderOrEmit(cmd, report, func() error {
				renderDepsCheck(cmd.OutOrStdout(), report)
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to inspect")
	cmd.Flags().StringVar(&home, "home", "", "home directory rooting per-user font detection (default: the current user's home); use a sandbox path for honest fresh-machine validation")
	return cmd
}

func newDepsPlanCommand(profile *string) *cobra.Command {
	var (
		file string
		tier string
		home string
	)

	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Show OS-aware installation guidance for missing Dependencies",
		Long:  "plan produces advisory installation guidance for the Dependencies a profile needs but the workstation is missing. It never installs packages.",
		// Domain errors (e.g. unknown profile or tier) are user-facing messages,
		// not misuse of the command, so do not dump the usage block on failure.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedHome := resolveDepsHome(home)
			m, err := manifest.LoadFile(resolveManifestPath(cmd, file, defaultSourceRoot(resolvedHome)))
			if err != nil {
				return err
			}

			resolvedTier, err := resolveTier(tier)
			if err != nil {
				return err
			}

			report, err := deps.Plan(*m, deps.Options{
				Profile: *profile,
				OS:      runtime.GOOS,
			}, lookupCommand, fontInstalled(runtime.GOOS, resolvedHome), resolvedTier)
			if err != nil {
				return err
			}

			return renderOrEmit(cmd, report, func() error {
				renderDepsPlan(cmd.OutOrStdout(), report)
				return nil
			})
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to inspect")
	cmd.Flags().StringVar(&tier, "tier", "", "override the dependency plan tier (homebrew, debian, fedora, arch, generic); default: auto-detect from the host")
	cmd.Flags().StringVar(&home, "home", "", "home directory rooting per-user font detection (default: the current user's home); use a sandbox path for honest fresh-machine validation")
	return cmd
}

func newDepsInstallCommand(profile *string) *cobra.Command {
	var (
		file   string
		tier   string
		dryRun bool
		yes    bool
	)

	cmd := &cobra.Command{
		Use:          "install",
		Short:        "Install missing Dependencies with explicit confirmation",
		Long:         "install previews dependency install actions, asks for confirmation by default, and executes installable actions only after explicit approval. Use --dry-run to preview without prompting or --yes for non-interactive execution.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedHome, _ := os.UserHomeDir()
			m, err := manifest.LoadFile(resolveManifestPath(cmd, file, defaultSourceRoot(resolvedHome)))
			if err != nil {
				return err
			}

			resolvedTier, err := resolveTier(tier)
			if err != nil {
				return err
			}

			options := deps.Options{
				Profile: *profile,
				OS:      runtime.GOOS,
			}

			report, err := deps.InstallDryRun(*m, options, lookupCommand, fontInstalled(runtime.GOOS, resolvedHome), resolvedTier)
			if err != nil {
				return err
			}

			if dryRun {
				if wantsJSON(cmd) {
					return emitOK(cmd, depsInstallDryRunReport{DryRun: true, Report: report})
				}
				renderDepsInstallDryRun(cmd.OutOrStdout(), report)
				return nil
			}

			if wantsJSON(cmd) && !yes {
				return rejectInteractiveJSON(cmd)
			}

			if yes {
				if !hasInstallablePreviewAction(report) {
					if wantsJSON(cmd) {
						return emitOK(cmd, depsInstallDryRunReport{DryRun: false, Report: report})
					}
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
	cmd.Flags().StringVar(&tier, "tier", "", "override the dependency install tier (homebrew, debian, fedora, arch, generic); default: auto-detect from the host")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview dependency install actions without executing package managers")
	cmd.Flags().BoolVar(&yes, "yes", false, "execute dependency install actions without interactive confirmation")
	return cmd
}

func runDepsInstall(cmd *cobra.Command, m manifest.Manifest, options deps.Options, tier deps.Tier) error {
	home, _ := os.UserHomeDir()
	stdout := cmd.OutOrStdout()
	if wantsJSON(cmd) {
		stdout = cmd.ErrOrStderr()
	}
	report, err := deps.Install(m, options, lookupCommand, fontInstalled(runtime.GOOS, home), tier, depsExecRunner{
		ctx:    cmd.Context(),
		stdin:  cmd.InOrStdin(),
		stdout: stdout,
		stderr: cmd.ErrOrStderr(),
	})
	if err != nil {
		if !wantsJSON(cmd) && (report.Profile != "" || len(report.Items) > 0) {
			renderDepsInstall(cmd.OutOrStdout(), report)
		}
		return err
	}
	if wantsJSON(cmd) {
		return emitOK(cmd, depsInstallRunReport{DryRun: false, Report: report})
	}
	if report.Profile != "" || len(report.Items) > 0 {
		renderDepsInstall(cmd.OutOrStdout(), report)
	}
	return nil
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

func commandOutput(command string, args ...string) (string, error) {
	cmd := exec.Command(command, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// fontDirectories returns the workstation directories that hold installed font
// files for the given OS, rooting the per-user locations at home. A font
// declared as a Dependency is detected by scanning these for a matching file.
// When home is empty (e.g. $HOME unset) the per-user locations are omitted
// rather than emitted as paths relative to the process working directory, so
// detection degrades to the system directories instead of scanning the wrong
// place. Font detection never aborts deps check, so a missing home is not fatal.
func fontDirectories(goos, home string) []string {
	if goos == "darwin" {
		var dirs []string
		if home != "" {
			dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
		}
		return append(dirs, "/Library/Fonts", "/System/Library/Fonts")
	}
	// Linux and other fontconfig layouts.
	var dirs []string
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", "fonts"),
			filepath.Join(home, ".fonts"),
		)
	}
	return append(dirs, "/usr/share/fonts", "/usr/local/share/fonts")
}

// resolveDepsHome returns the home directory used to root per-user font
// directories during deps font detection. An explicit --home wins so sandbox
// readiness checks scan the environment under test rather than the operator's
// real home; otherwise it falls back to the current user's home. A missing home
// is not fatal: fontDirectories omits the per-user locations and detection
// degrades to the system directories.
func resolveDepsHome(flag string) string {
	if flag != "" {
		return flag
	}
	home, _ := os.UserHomeDir()
	return home
}

// fontInstalled is the production FontLookup: it scans the OS font directories
// under home for an installed file matching the declared glob.
func fontInstalled(goos, home string) deps.FontLookup {
	dirs := fontDirectories(goos, home)
	return func(match string) bool {
		return deps.ScanFonts(dirs, match)
	}
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
