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
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
)

func newDepsCommand() *cobra.Command {
	var (
		profiles  []string
		extraTags []string
	)

	cmd := &cobra.Command{
		Use:   "deps",
		Short: "Inspect external tool Dependencies declared by managed entries",
		Long:  "deps reports which external Dependencies a profile needs, offers OS-aware installation guidance, and can execute missing install actions with explicit confirmation.",
	}
	cmd.PersistentFlags().StringArrayVarP(&profiles, "profile", "p", nil, "profile to inspect")
	cmd.PersistentFlags().StringArrayVar(&extraTags, "tag", nil, "include an additional manifest tag; repeat to include multiple tags")
	cmd.AddCommand(newDepsCheckCommand(&profiles, &extraTags))
	cmd.AddCommand(newDepsPlanCommand(&profiles, &extraTags))
	cmd.AddCommand(newDepsInstallCommand(&profiles, &extraTags))
	return cmd
}

func newDepsCheckCommand(profiles *[]string, extraTags *[]string) *cobra.Command {
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
			m, err := loadDepsManifest(cmd, file, resolvedHome)
			if err != nil {
				return err
			}

			effective, err := resolveDepsReadOnlySelection(*m, resolvedHome, *profiles, *extraTags)
			if err != nil {
				return err
			}

			report, err := deps.CheckWithToolProbes(*m, deps.Options{
				Profiles:  effective.Profiles,
				ExtraTags: effective.ExtraTags,
				Selection: &effective.Selection,
				OS:        runtime.GOOS,
				AppLookup: appInstalled(runtime.GOOS, resolvedHome),
			}, lookupCommand, fontInstalled(runtime.GOOS, resolvedHome), commandOutput)
			if err != nil {
				return err
			}
			report.Selection = &effective.Report

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

func newDepsPlanCommand(profiles *[]string, extraTags *[]string) *cobra.Command {
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
			m, err := loadDepsManifest(cmd, file, resolvedHome)
			if err != nil {
				return err
			}

			effective, err := resolveDepsReadOnlySelection(*m, resolvedHome, *profiles, *extraTags)
			if err != nil {
				return err
			}

			resolvedTier, err := resolveTier(tier)
			if err != nil {
				return err
			}

			report, err := deps.Plan(*m, deps.Options{
				Profiles:  effective.Profiles,
				ExtraTags: effective.ExtraTags,
				Selection: &effective.Selection,
				OS:        runtime.GOOS,
				AppLookup: appInstalled(runtime.GOOS, resolvedHome),
			}, lookupCommand, fontInstalled(runtime.GOOS, resolvedHome), resolvedTier)
			if err != nil {
				return err
			}
			report.Selection = &effective.Report

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

func newDepsInstallCommand(profiles *[]string, extraTags *[]string) *cobra.Command {
	var (
		file      string
		tier      string
		home      string
		stateRoot string
		dryRun    bool
		yes       bool
	)

	cmd := &cobra.Command{
		Use:          "install",
		Short:        "Install missing Dependencies with explicit confirmation",
		Long:         "install previews dependency install actions, asks for confirmation by default, and executes installable actions only after explicit approval. Use --dry-run to preview without prompting or --yes for non-interactive execution.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedHome := resolveDepsHome(home)
			resolvedStateRoot := stateRoot
			if resolvedStateRoot == "" {
				resolvedStateRoot = defaultStateRoot(resolvedHome)
			}
			m, err := loadDepsManifest(cmd, file, resolvedHome)
			if err != nil {
				return err
			}

			resolvedTier, err := resolveTier(tier)
			if err != nil {
				return err
			}

			options := deps.Options{
				Profiles:  *profiles,
				ExtraTags: *extraTags,
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
				Home:      resolvedHome,
				StateRoot: resolvedStateRoot,
				AppLookup: appInstalled(runtime.GOOS, resolvedHome),
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
	cmd.Flags().StringVar(&home, "home", "", "home directory for user-local dependency providers and font detection (default: the current user's home); use a sandbox path for validation")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Dependency Installation Metadata (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview dependency install actions without executing package managers")
	cmd.Flags().BoolVar(&yes, "yes", false, "execute dependency install actions without interactive confirmation")
	return cmd
}

func loadDepsManifest(cmd *cobra.Command, file, home string) (*manifest.Manifest, error) {
	return loadManifestForCommand(cmd, file, defaultSourceRoot(home))
}

func resolveDepsReadOnlySelection(m manifest.Manifest, home string, profiles, extraTags []string) (selection.Effective, error) {
	if len(profiles) > 0 || len(extraTags) > 0 {
		return selection.ResolveReadOnly(m, profiles, extraTags, nil)
	}
	paths, err := resolvePaths(home, "", "")
	if err != nil {
		return selection.Effective{}, err
	}
	meta, err := loadInstallationMetadata(paths, "")
	if err != nil {
		return selection.Effective{}, err
	}
	return resolveReadOnlySelection(m, meta, profiles, extraTags, readOnlySelectionOptions{
		Home: paths.Home, SourceRoot: paths.SourceRoot, StatePath: state.Path(paths.StateRoot),
	})
}

func runDepsInstall(cmd *cobra.Command, m manifest.Manifest, options deps.Options, tier deps.Tier) error {
	home := options.Home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if options.StateRoot == "" {
		options.StateRoot = defaultStateRoot(home)
	}
	stdout := cmd.OutOrStdout()
	if wantsJSON(cmd) {
		stdout = cmd.ErrOrStderr()
	}
	report, err := deps.Install(m, options, lookupCommand, fontInstalled(runtime.GOOS, home), tier, &depsExecRunner{
		ctx:       cmd.Context(),
		stdin:     cmd.InOrStdin(),
		stdout:    stdout,
		stderr:    cmd.ErrOrStderr(),
		home:      home,
		stateRoot: options.StateRoot,
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
	ctx       context.Context
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	home      string
	stateRoot string
	brewPath  string
}

func (r depsExecRunner) Run(executable string, args []string) error {
	if executable == "brew" && r.brewPath != "" {
		executable = r.brewPath
	}
	cmd := exec.CommandContext(r.ctx, executable, args...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	return cmd.Run()
}

func (r depsExecRunner) AddHomebrewFormulaToPATH(formula string) error {
	executable := "brew"
	if r.brewPath != "" {
		executable = r.brewPath
	}
	cmd := exec.CommandContext(r.ctx, executable, "--prefix", formula)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("resolve Homebrew formula prefix for %q: %w", formula, err)
	}
	prefix := strings.TrimSpace(string(output))
	if prefix == "" {
		return fmt.Errorf("resolve Homebrew formula prefix for %q: empty output", formula)
	}
	bin := filepath.Join(prefix, "bin")
	path := os.Getenv("PATH")
	for _, entry := range filepath.SplitList(path) {
		if entry == bin {
			return nil
		}
	}
	if path != "" {
		bin += string(os.PathListSeparator) + path
	}
	return os.Setenv("PATH", bin)
}

func (r depsExecRunner) Lookup(command string) bool {
	return lookupCommand(command)
}

func (r depsExecRunner) RunUserLocal(action deps.InstallAction) error {
	return deps.InstallUserLocal(r.home, action)
}

func (r depsExecRunner) RecordUserLocal(action deps.InstallAction) error {
	return deps.RecordDependencyInstallation(r.stateRoot, r.home, action)
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

// appInstalled is the production AppLookup. On macOS it checks the selected
// user's Applications directory before the system Applications directory.
// Other operating systems never satisfy a Darwin application probe.
func appInstalled(goos, home string) deps.AppLookup {
	return appInstalledIn(appDirectories(goos, home))
}

func appDirectories(goos, home string) []string {
	var dirs []string
	if goos == "darwin" {
		if home != "" {
			dirs = append(dirs, filepath.Join(home, "Applications"))
		}
		dirs = append(dirs, "/Applications")
	}
	return dirs
}

func appInstalledIn(dirs []string) deps.AppLookup {
	return func(app string) bool {
		for _, dir := range dirs {
			info, err := os.Stat(filepath.Join(dir, app))
			if err == nil && info.IsDir() {
				return true
			}
		}
		return false
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
