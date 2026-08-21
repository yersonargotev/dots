package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/codexconfig"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/deps/pkgmgr"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/tui"
	"github.com/yersonargotev/dots/internal/version"
)

var (
	installHostOS                           = runtime.GOOS
	installHostArch                         = runtime.GOARCH
	packageManagerDetector                  = pkgmgr.Detector{}
	packageManagerSetupRunner pkgmgr.Runner = pkgmgr.ExecRunner{}
	recordInstalledSelection                = selection.Record
)

func newInstallCommand() *cobra.Command {
	var (
		file             string
		profiles         []string
		extraTags        []string
		sourceRoot       string
		home             string
		stateRoot        string
		dryRun           bool
		yes              bool
		noTUI            bool
		skipDeps         bool
		backupAndReplace bool
		ackSelection     bool
	)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install repository-managed dotfiles",
		Long:  "install computes and shows an Install Plan, then applies safe create actions unless --dry-run is set.",
		// Domain installation failures are user-facing conflicts, not command misuse.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := resolvePaths(home, sourceRoot, stateRoot)
			if err != nil {
				return err
			}
			if backupAndReplace && !dryRun && !yes {
				return fmt.Errorf("--backup-and-replace requires --yes for non-interactive conflict replacement")
			}

			prep := installRepositoryPreparation{SourceReadRoot: paths.SourceRoot, LegacyMigrations: map[string]plan.LegacyMigration{}, cleanup: func() {}}
			defaultRepository := !cmd.Flags().Changed("file") && !cmd.Flags().Changed("source-root")
			if defaultRepository {
				prep, err = prepareInstallRepository(cmd, paths, dryRun)
				if err != nil {
					return err
				}
				defer prep.cleanup()
			}

			var m *manifest.Manifest
			if defaultRepository && prep.SourceReadRoot != paths.SourceRoot {
				m, err = manifest.LoadFile(filepath.Join(prep.SourceReadRoot, file))
			} else {
				m, err = loadManifestForCommand(cmd, file, paths.SourceRoot)
			}
			if err != nil {
				return err
			}
			intentProfiles := profiles
			intentTags := extraTags
			if len(intentProfiles) == 0 && len(intentTags) == 0 {
				requestedSelection, err := selection.Resolve(*m, nil, nil)
				if err != nil {
					return err
				}
				intentProfiles = requestedSelection.Profiles
				intentTags = requestedSelection.ExtraTags
			}
			effective, err := selection.ResolveIntent(*m, selection.Intent{
				Source:    selection.SourceExplicit,
				Profiles:  intentProfiles,
				ExtraTags: intentTags,
			})
			if err != nil {
				return err
			}
			selectedProfiles := effective.Profiles
			selectedTags := effective.ExtraTags

			meta, err := loadInstallationMetadata(paths, stateRoot)
			if err != nil {
				return err
			}
			effective = selection.CompareInstalled(*m, effective, meta.InstalledSelection, installHostOS)
			if !wantsJSON(cmd) {
				renderTagMigrations(cmd.OutOrStdout(), effective.Report.TagMigrations)
			}
			proceed, _, err := guardSelectionChange(cmd, &effective, selectionChangePolicy{
				DryRun: dryRun, Confirmed: yes, Acknowledge: ackSelection,
			})
			if err != nil {
				return err
			}
			if !proceed {
				return nil
			}
			installedSelection := effective.InstalledSelection(state.Provenance{})

			hostOS := installHostOS
			hostArch := installHostArch
			depOptions := deps.Options{Profiles: selectedProfiles, ExtraTags: selectedTags, OS: hostOS, Arch: hostArch, Home: paths.Home, StateRoot: paths.StateRoot, AppLookup: appInstalled(hostOS, paths.Home), HTTPClient: depsHTTPClient, RollingReleaseURL: depsRollingReleaseURL}
			depTier, err := resolveInstallTier(hostOS)
			if err != nil {
				return err
			}
			var depPreview deps.InstallDryRunReport
			var depPreviewReport *deps.InstallDryRunReport
			var packageManagerSetup *pkgmgr.Report
			brewDetection := packageManagerDetector.DetectHomebrew()
			depLookup := packageManagerLookup(brewDetection)
			if !skipDeps {
				depPreview, err = deps.InstallDryRun(*m, depOptions, depLookup, fontInstalled(hostOS, paths.Home), depTier)
				if err != nil {
					return err
				}
				depPreviewReport = &depPreview
				setup := pkgmgr.HomebrewSetupNeed(hostOS, depPreview, brewDetection)
				if setup.Status != pkgmgr.StatusNotNeeded || setup.Detection.NeedsPATH {
					packageManagerSetup = &setup
				}
			}

			var dependenciesReport *installDependenciesReport
			if depPreviewReport != nil {
				dependenciesReport = &installDependenciesReport{Preview: depPreviewReport}
			}
			if wantsJSON(cmd) {
				if !dryRun && !yes {
					return rejectInteractiveJSON(cmd)
				}
			} else if depPreviewReport != nil {
				renderPackageManagerSetup(cmd.OutOrStdout(), packageManagerSetup)
				renderDepsInstallPreview(cmd.OutOrStdout(), *depPreviewReport)
				fmt.Fprintln(cmd.OutOrStdout())
			} else if skipDeps {
				fmt.Fprintln(cmd.OutOrStdout(), "Dependency provisioning skipped (--skip-deps).")
				fmt.Fprintln(cmd.OutOrStdout())
			}

			if dryRun {
				p, provPlan, err := buildInstallPlanAndProvisioners(*m, meta, selectedProfiles, selectedTags, hostOS, paths, prep.SourceReadRoot, prep.LegacyMigrations)
				if err != nil {
					return err
				}
				p.Selection = &effective.Report
				if wantsJSON(cmd) {
					return emitOK(cmd, installReport{RepositoryRefresh: prep.Refresh, DryRun: true, Selection: effective.Report, PackageManagerSetup: packageManagerSetup, Dependencies: dependenciesReport, Plan: p, Provisioners: provPlan})
				}
				if err := renderInstallPlanAndProvisioners(cmd, *m, p, provPlan, selectedProfiles); err != nil {
					return err
				}
				return nil
			}

			var p plan.Plan
			var provPlan provision.Plan
			installPlanReady := false
			var dependencyEnvironment []string

			if !skipDeps {
				depsConfirmed := yes
				if packageManagerSetup != nil && packageManagerSetup.Status == pkgmgr.StatusWouldOffer {
					if yes {
						packageManagerSetup.Status = pkgmgr.StatusUnavailable
						err := fmt.Errorf("Homebrew Package Manager Setup requires interactive confirmation; rerun without --yes or install Homebrew manually with %s", packageManagerSetup.Command.Display)
						if wantsJSON(cmd) {
							return installDependencyGateError{err: err, report: installReport{RepositoryRefresh: prep.Refresh, DryRun: false, Selection: effective.Report, PackageManagerSetup: packageManagerSetup, Dependencies: dependenciesReport}}
						}
						return err
					}
					confirmed, err := confirmPackageManagerSetup(cmd.InOrStdin(), cmd.OutOrStdout(), *packageManagerSetup)
					if err != nil {
						return err
					}
					if !confirmed {
						packageManagerSetup.Status = pkgmgr.StatusDeclined
						fmt.Fprintln(cmd.OutOrStdout(), "Package Manager Setup declined; install canceled before Managed Configuration.")
						return nil
					}
					if err := pkgmgr.RunHomebrewSetup(cmd.Context(), packageManagerSetupRunner, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
						packageManagerSetup.Status = pkgmgr.StatusFailed
						return fmt.Errorf("Homebrew Package Manager Setup failed: %w", err)
					}
					brewDetection = packageManagerDetector.DetectHomebrew()
					packageManagerSetup.Detection = brewDetection
					if !brewDetection.Found {
						packageManagerSetup.Status = pkgmgr.StatusUnavailable
						return fmt.Errorf("Homebrew Package Manager Setup completed but brew was not found on PATH, /opt/homebrew/bin/brew, or /usr/local/bin/brew")
					}
					packageManagerSetup.Status = pkgmgr.StatusInstalled
					if brewDetection.PATHGuidance != "" {
						fmt.Fprintln(cmd.OutOrStdout(), brewDetection.PATHGuidance)
					}
					depLookup = packageManagerLookup(brewDetection)
					depPreview, err = deps.InstallDryRun(*m, depOptions, depLookup, fontInstalled(hostOS, paths.Home), depTier)
					if err != nil {
						return err
					}
					dependenciesReport.Preview = &depPreview
					renderDepsInstallPreview(cmd.OutOrStdout(), depPreview)
					depsConfirmed = true
				}
				p, provPlan, err = buildInstallPlanAndProvisioners(*m, meta, selectedProfiles, selectedTags, hostOS, paths, prep.SourceReadRoot, prep.LegacyMigrations)
				if err != nil {
					return err
				}
				p.Selection = &effective.Report
				installPlanReady = true

				depReport, depsApplied, runEnvironment, err := runInstallDependencies(cmd, *m, depOptions, depTier, paths.Home, depPreview, depsConfirmed, depLookup, brewDetection.Path)
				dependencyEnvironment = runEnvironment
				dependenciesReport.Result = &depReport
				if err != nil {
					if wantsJSON(cmd) {
						return installDependencyGateError{err: err, report: installReport{RepositoryRefresh: prep.Refresh, DryRun: false, Selection: effective.Report, PackageManagerSetup: packageManagerSetup, Dependencies: dependenciesReport, Plan: p, Provisioners: provPlan}}
					}
					return err
				}
				if !depsApplied {
					return nil
				}
			}

			if !installPlanReady {
				p, provPlan, err = buildInstallPlanAndProvisioners(*m, meta, selectedProfiles, selectedTags, hostOS, paths, prep.SourceReadRoot, prep.LegacyMigrations)
				if err != nil {
					return err
				}
				p.Selection = &effective.Report
			}

			if !wantsJSON(cmd) {
				if err := renderInstallPlanAndProvisioners(cmd, *m, p, provPlan, selectedProfiles); err != nil {
					return err
				}
			}

			beforeBackups, err := backups.Load(backups.Path(paths.StateRoot))
			if err != nil {
				return err
			}
			applied, err := resolveAndApply(cmd, p, paths, yes, noTUI, backupAndReplace)
			if err != nil {
				return err
			}
			createdBackups, err := createdBackupSetReports(paths.StateRoot, beforeBackups)
			if err != nil {
				return err
			}
			if !applied {
				if wantsJSON(cmd) {
					return emitOK(cmd, installReport{RepositoryRefresh: prep.Refresh, DryRun: false, Selection: effective.Report, PackageManagerSetup: packageManagerSetup, Dependencies: dependenciesReport, Plan: p, Provisioners: provPlan})
				}
				return nil
			}

			provResult, err := runProvisionersWithEnvironment(cmd, *m, selectedProfiles, selectedTags, paths.Home, paths.StateRoot, paths.SourceRoot, dependencyEnvironment)
			if err != nil {
				if wantsJSON(cmd) {
					return installProvisionerError{err: err, report: installReport{RepositoryRefresh: prep.Refresh, DryRun: false, Selection: effective.Report, PackageManagerSetup: packageManagerSetup, Dependencies: dependenciesReport, Plan: p, Provisioners: provPlan, BackupSets: createdBackups, ProvisionerResults: &provResult}}
				}
				return err
			}
			retirement, err := retireHistoricalAgentState(meta, paths.Home)
			if err != nil {
				return err
			}
			installedSelection.Provenance = state.CaptureProvenance(paths.SourceRoot, version.Value)
			if err := recordInstalledSelection(state.Path(paths.StateRoot), installedSelection); err != nil {
				return err
			}
			if !wantsJSON(cmd) {
				renderHistoricalRetirement(cmd.OutOrStdout(), retirement)
			}
			if wantsJSON(cmd) {
				return emitOK(cmd, installReport{RepositoryRefresh: prep.Refresh, DryRun: false, Selection: effective.Report, PackageManagerSetup: packageManagerSetup, Dependencies: dependenciesReport, Plan: p, Provisioners: provPlan, BackupSets: createdBackups, Retirement: retirement})
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&file, "file", "f", "dots.yaml", "manifest file to install")
	cmd.Flags().StringArrayVarP(&profiles, "profile", "p", nil, "profile to install")
	cmd.Flags().StringArrayVar(&extraTags, "tag", nil, "include an additional manifest tag; repeat to include multiple tags")
	cmd.Flags().StringVar(&sourceRoot, "source-root", "", "installed repository root (default ~/.local/share/dots)")
	cmd.Flags().StringVar(&home, "home", "", "target home directory to install into (default: the current user's home); use a sandbox path to avoid touching real config")
	cmd.Flags().StringVar(&stateRoot, "state-root", "", "state directory for Installation Metadata (default ~/.local/state/dots)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the Install Plan without modifying files")
	cmd.Flags().BoolVar(&yes, "yes", false, "apply safe install actions without prompting; conflicts default to skip")
	cmd.Flags().BoolVar(&noTUI, "no-tui", false, "use text prompts instead of the interactive TUI for conflict resolution")
	cmd.Flags().BoolVar(&skipDeps, "skip-deps", false, "skip dependency provisioning before applying managed configuration")
	cmd.Flags().BoolVar(&backupAndReplace, "backup-and-replace", false, "with --yes, replace every conflict after creating Backup Sets")
	cmd.Flags().BoolVar(&ackSelection, "acknowledge-selection-change", false, "with --yes, acknowledge removal of previously selected Profiles or extra Tags")
	return cmd
}

func renderInstallPlanAndProvisioners(cmd *cobra.Command, m manifest.Manifest, p plan.Plan, provPlan provision.Plan, profiles []string) error {
	renderPlan(cmd.OutOrStdout(), p)
	if len(profiles) > 0 {
		if err := renderSkippedEntryHint(cmd.OutOrStdout(), m, profiles, runtime.GOOS); err != nil {
			return err
		}
	}
	renderProvisionPlan(cmd.OutOrStdout(), provPlan)
	if len(profiles) == 0 {
		return nil
	}
	return renderSkippedProvisionerHint(cmd.OutOrStdout(), m, profiles, runtime.GOOS)
}

func buildInstallPlanAndProvisioners(m manifest.Manifest, meta state.Metadata, profiles []string, extraTags []string, hostOS string, paths resolvedPaths, sourceReadRoot string, legacyMigrations map[string]plan.LegacyMigration) (plan.Plan, provision.Plan, error) {
	p, err := plan.Build(m, plan.Options{
		Profiles:         profiles,
		ExtraTags:        extraTags,
		OS:               hostOS,
		SourceRoot:       paths.SourceRoot,
		SourceReadRoot:   sourceReadRoot,
		Home:             paths.Home,
		XDGStateHome:     paths.XDGStateHome,
		Metadata:         meta,
		LegacyMigrations: legacyMigrations,
	})
	if err != nil {
		return plan.Plan{}, provision.Plan{}, err
	}

	provPlan, err := provision.Build(m, provision.Options{Profiles: profiles, ExtraTags: extraTags, OS: hostOS})
	if err != nil {
		return plan.Plan{}, provision.Plan{}, err
	}
	return p, provPlan, nil
}

func resolveInstallTier(goos string) (deps.Tier, error) {
	if goos == "darwin" {
		return deps.TierHomebrew, nil
	}
	return resolveTier("")
}

func packageManagerLookup(detection pkgmgr.HomebrewDetection) deps.Lookup {
	return func(command string) bool {
		if command == "brew" && detection.Found {
			return true
		}
		return lookupCommand(command)
	}
}

func renderPackageManagerSetup(w io.Writer, report *pkgmgr.Report) {
	if report == nil {
		return
	}
	if report.Status == pkgmgr.StatusWouldOffer {
		fmt.Fprintf(w, "Package Manager Setup for %s\n\n", report.Manager)
		fmt.Fprintf(w, "  would-offer %s\n", report.Reason)
		fmt.Fprintf(w, "  command     %s\n\n", report.Command.Display)
		return
	}
	if report.Detection.PATHGuidance != "" {
		fmt.Fprintln(w, report.Detection.PATHGuidance)
		fmt.Fprintln(w)
	}
}

func confirmPackageManagerSetup(r io.Reader, w io.Writer, report pkgmgr.Report) (bool, error) {
	fmt.Fprintf(w, "Run Homebrew Package Manager Setup? This will execute: %s [y/N] ", report.Command.Display)
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read package manager setup confirmation: %w", err)
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

type installDependencyGateError struct {
	err    error
	report installReport
}

func (e installDependencyGateError) Error() string { return e.err.Error() }

func (e installDependencyGateError) Unwrap() error { return e.err }

func (e installDependencyGateError) JSONErrorData() any { return e.report }

type installProvisionerError struct {
	err    error
	report installReport
}

func (e installProvisionerError) Error() string { return e.err.Error() }

func (e installProvisionerError) Unwrap() error { return e.err }

func (e installProvisionerError) JSONErrorData() any { return e.report }

// runInstallDependencies executes the dependency gate for dots install before any
// Managed Configuration is applied. Interactive runs reuse the deps confirmation
// prompt for package-manager execution; manual or unresolved Dependencies abort
// the install before filesystem targets are touched.
func runInstallDependencies(cmd *cobra.Command, m manifest.Manifest, options deps.Options, tier deps.Tier, home string, preview deps.InstallDryRunReport, yes bool, look deps.Lookup, brewPath string) (deps.InstallReport, bool, []string, error) {
	options = pinResolvedUserLocal(options, preview)
	if !yes {
		if hasRequiredInstallablePreviewAction(preview) {
			confirmed, err := confirmDepsInstall(cmd.InOrStdin(), cmd.OutOrStdout())
			if err != nil {
				return deps.InstallReport{}, false, nil, err
			}
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), "Dependency installation cancelled.")
				return deps.InstallReport{}, false, nil, nil
			}
		} else if hasInstallablePreviewAction(preview) {
			report, err := unresolvedInstallReportFromPreview(preview)
			if !wantsJSON(cmd) && (report.Profile != "" || len(report.Items) > 0) {
				renderDepsInstall(cmd.OutOrStdout(), report)
			}
			return report, true, nil, err
		}
	}

	stdout := cmd.OutOrStdout()
	if wantsJSON(cmd) {
		stdout = cmd.ErrOrStderr()
	}
	runner := &depsExecRunner{
		ctx:       cmd.Context(),
		stdin:     cmd.InOrStdin(),
		stdout:    stdout,
		stderr:    cmd.ErrOrStderr(),
		home:      home,
		stateRoot: options.StateRoot,
		brewPath:  brewPath,
	}
	report, err := deps.Install(m, options, look, fontInstalled(options.OS, home), tier, runner)
	runEnvironment := runner.Environment()
	if !wantsJSON(cmd) && (report.Profile != "" || len(report.Items) > 0) {
		renderDepsInstall(cmd.OutOrStdout(), report)
	}
	if err != nil {
		return report, true, runEnvironment, err
	}
	return report, true, runEnvironment, nil
}

// runProvisioners executes the selected provisioners after dependency installs
// and file entries, in the same install run. It threads HOME from the resolved
// --home so a sandboxed install lands every tool-managed file under the
// temporary home. Apply stops at the first failing provisioner and returns the
// error, which the caller surfaces; the tool's own stdout/stderr are streamed
// through so its progress is visible.
func runProvisioners(cmd *cobra.Command, m manifest.Manifest, profiles []string, extraTags []string, home string, stateRoot string, sourceRoot string) (provision.Report, error) {
	return runProvisionersWithEnvironment(cmd, m, profiles, extraTags, home, stateRoot, sourceRoot, nil)
}

func runProvisionersWithEnvironment(cmd *cobra.Command, m manifest.Manifest, profiles []string, extraTags []string, home string, stateRoot string, sourceRoot string, baseEnv []string) (provision.Report, error) {
	return runProvisionersWithOptionsAndEnvironment(cmd, m, provision.Options{Profiles: profiles, ExtraTags: extraTags, OS: runtime.GOOS}, home, stateRoot, sourceRoot, baseEnv)
}

func runProvisionersWithOptions(cmd *cobra.Command, m manifest.Manifest, provisionOpts provision.Options, home string, stateRoot string, sourceRoot string) (provision.Report, error) {
	return runProvisionersWithOptionsAndEnvironment(cmd, m, provisionOpts, home, stateRoot, sourceRoot, nil)
}

func runProvisionersWithOptionsAndEnvironment(cmd *cobra.Command, m manifest.Manifest, provisionOpts provision.Options, home string, stateRoot string, sourceRoot string, baseEnv []string) (provision.Report, error) {
	if provisionOpts.AppLookup == nil {
		provisionOpts.AppLookup = appInstalled(provisionOpts.OS, home)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	stdout := cmd.OutOrStdout()
	if wantsJSON(cmd) {
		stdout = cmd.ErrOrStderr()
	}
	runner := provisionExecRunner{
		ctx:     ctx,
		home:    home,
		stdin:   cmd.InOrStdin(),
		stdout:  stdout,
		stderr:  cmd.ErrOrStderr(),
		baseEnv: baseEnv,
	}
	selected, selectErr := provision.Select(m, provisionOpts)
	if selectErr != nil {
		return provision.Report{}, selectErr
	}
	report, err := provision.Apply(m, provisionOpts, runner.Lookup, fontInstalled(runtime.GOOS, home), runner)
	if !wantsJSON(cmd) {
		renderProvisionReport(cmd.OutOrStdout(), report)
	}
	if recordErr := recordProvisionerMetadata(stateRoot, sourceRoot, report); recordErr != nil {
		return report, recordErr
	}
	if err != nil {
		return report, err
	}
	if agents := selectedCodeGraphAgents(selected); len(agents) > 0 {
		return report, codexconfig.EnsureCodeGraphMode(home, agents...)
	}
	return report, nil
}

func selectedCodeGraphAgents(selected []manifest.Provisioner) []string {
	var agents []string
	seen := map[string]bool{}
	for _, prov := range selected {
		if prov.Tool != "codegraph" {
			continue
		}
		for _, agent := range prov.Spec.Agents {
			if seen[agent] {
				continue
			}
			seen[agent] = true
			agents = append(agents, agent)
		}
	}
	return agents
}

// resolveAndApply resolves the plan's conflicts (via the TUI, text prompts, or
// the conservative --yes default) and applies it with Backup Set protection. It
// is shared by install and update so post-update installation reuses identical
// Conflict Resolution and filesystem machinery instead of reimplementing it.
func resolveAndApply(cmd *cobra.Command, p plan.Plan, paths resolvedPaths, yes, noTUI, backupAndReplace bool) (bool, error) {
	var (
		decisions map[string]install.ConflictDecision
		err       error
	)
	switch {
	case yes:
		if backupAndReplace {
			decisions = replaceAllConflictDecisions(p)
		}
		// Non-interactive default: conflicts deliberately default to skip.
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

func replaceAllConflictDecisions(p plan.Plan) map[string]install.ConflictDecision {
	decisions := map[string]install.ConflictDecision{}
	for _, action := range conflictActions(p) {
		decisions[action.Target] = install.DecisionReplace
	}
	return decisions
}

func createdBackupSetReports(stateRoot string, before backups.Metadata) ([]installBackupSetReport, error) {
	after, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, set := range before.Sets {
		seen[set.ID] = true
	}
	var created []installBackupSetReport
	for _, set := range after.Sets {
		if seen[set.ID] {
			continue
		}
		created = append(created, installBackupSetReport{BackupSet: set, Path: backups.SetDir(stateRoot, set.ID)})
	}
	return created, nil
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

func recordProvisionerMetadata(stateRoot, sourceRoot string, report provision.Report) error {
	if stateRoot == "" || len(report.Items) == 0 {
		return nil
	}
	path := state.Path(stateRoot)
	meta, err := state.Load(path)
	if err != nil {
		return err
	}
	if meta.Version < 2 {
		meta.Version = 2
	}
	meta.Provenance = state.CaptureProvenance(sourceRoot, version.Value)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, item := range report.Items {
		meta.UpsertProvisioner(state.ProvisionerRecord{
			Profile:    report.Profile,
			Profiles:   append([]string(nil), report.Profiles...),
			Tags:       append([]string(nil), report.Tags...),
			Tool:       item.Tool,
			Executable: item.Executable,
			Args:       append([]string(nil), item.Args...),
			Status:     string(item.Status),
			Missing:    append([]string(nil), item.Missing...),
			LastRunAt:  now,
		})
	}
	return state.Save(path, meta)
}
