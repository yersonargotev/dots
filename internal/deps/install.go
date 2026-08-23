package deps

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// InstallPreviewStatus describes what a dry-run install would do for one
// missing Dependency.
type InstallPreviewStatus string

const (
	InstallPreviewWouldInstall InstallPreviewStatus = "would-install"
	InstallPreviewManual       InstallPreviewStatus = "manual"
)

// InstallPreview is one dry-run installation preview item.
type InstallPreview struct {
	Dependency   string               `json:"dependency"`
	Requirement  string               `json:"requirement"`
	Status       InstallPreviewStatus `json:"status"`
	Provider     Tier                 `json:"provider,omitempty"`
	Package      string               `json:"package,omitempty"`
	Executable   string               `json:"executable,omitempty"`
	Args         []string             `json:"args,omitempty"`
	Bootstrap    []Command            `json:"bootstrap,omitempty"`
	Manual       string               `json:"manual,omitempty"`
	TrustCommand string               `json:"trust_command,omitempty"`
	Candidates   []ProviderCandidate  `json:"candidates,omitempty"`
	UserLocal    *UserLocalArtifact   `json:"user_local,omitempty"`
}

// InstallDryRunReport previews the install actions for a Profile without
// invoking any package manager.
type InstallDryRunReport struct {
	Profile  string           `json:"profile,omitempty"`
	Profiles []string         `json:"profiles,omitempty"`
	Tags     []string         `json:"tags,omitempty"`
	Tier     Tier             `json:"tier"`
	Items    []InstallPreview `json:"items"`
}

// PreparedInstall binds the exact dependency actions accepted during preview
// to their public dry-run representation. InstallPrepared executes Plan as-is;
// it never resolves providers or artifacts again.
type PreparedInstall struct {
	Plan   PlanReport
	Report InstallDryRunReport
}

// Runner executes one argv-shaped install action.
type Runner interface {
	Run(executable string, args []string) error
}

// HomebrewFormulaPATHRunner lets the Homebrew rustup provider expose its
// formula-owned tool proxies to reprobes and later child commands in this run.
type HomebrewFormulaPATHRunner interface {
	Runner
	AddHomebrewFormulaToPATH(formula string) error
	Lookup(command string) bool
}

// ToolchainEnvironment exposes an environment established by a constrained
// toolchain bootstrap. Install uses it for reprobes and later child commands in
// the same dependency run without mutating the parent process environment.
type ToolchainEnvironment interface {
	ActivateToolchain(toolchain string) error
	Lookup(command string) bool
}

// InstallStatus describes the result of a real install action.
type InstallStatus string

const (
	InstallStatusInstalled  InstallStatus = "installed"
	InstallStatusManual     InstallStatus = "manual"
	InstallStatusUnresolved InstallStatus = "unresolved"
	InstallStatusFailed     InstallStatus = "failed"
)

// InstallItem is the result of one attempted dependency installation.
type InstallItem struct {
	Dependency   string              `json:"dependency"`
	Requirement  string              `json:"requirement"`
	Status       InstallStatus       `json:"status"`
	Provider     Tier                `json:"provider,omitempty"`
	Package      string              `json:"package,omitempty"`
	Executable   string              `json:"executable,omitempty"`
	Args         []string            `json:"args,omitempty"`
	Bootstrap    []Command           `json:"bootstrap,omitempty"`
	Manual       string              `json:"manual,omitempty"`
	TrustCommand string              `json:"trust_command,omitempty"`
	Candidates   []ProviderCandidate `json:"candidates,omitempty"`
	UserLocal    *UserLocalArtifact  `json:"user_local,omitempty"`
}

// InstallReport records the stable dots summary for a real install run.
type InstallReport struct {
	Profile  string        `json:"profile,omitempty"`
	Profiles []string      `json:"profiles,omitempty"`
	Tags     []string      `json:"tags,omitempty"`
	Tier     Tier          `json:"tier"`
	Items    []InstallItem `json:"items"`
}

// InstallDryRun computes the install preview for missing Dependencies without
// executing package managers.
func InstallDryRun(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup, tier Tier) (InstallDryRunReport, error) {
	prepared, err := PrepareInstall(m, opts, look, fontLook, tier)
	if err != nil {
		return InstallDryRunReport{}, err
	}
	return prepared.Report, nil
}

// PrepareInstall computes the dependency Plan once and derives the public
// preview from those exact actions.
func PrepareInstall(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup, tier Tier) (PreparedInstall, error) {
	plan, err := Plan(m, opts, look, fontLook, tier)
	if err != nil {
		return PreparedInstall{}, err
	}

	report := InstallDryRunReport{Profile: plan.Profile, Profiles: plan.Profiles, Tags: plan.Tags, Tier: plan.Tier}
	for _, action := range plan.Actions {
		status := InstallPreviewWouldInstall
		if !actionExecutable(action) {
			status = InstallPreviewManual
		}
		report.Items = append(report.Items, InstallPreview{
			Dependency:   action.Dependency,
			Requirement:  action.Requirement,
			Status:       status,
			Provider:     action.Provider,
			Package:      action.Package,
			Executable:   action.Executable,
			Args:         action.Args,
			Bootstrap:    append([]Command(nil), action.Bootstrap...),
			Manual:       action.Manual,
			TrustCommand: action.TrustCommand,
			Candidates:   action.Candidates,
			UserLocal:    action.UserLocal,
		})
	}
	return PreparedInstall{Plan: plan, Report: report}, nil
}

// Install executes missing executable install actions and re-probes each
// dependency after a successful package-manager command.
func Install(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup, tier Tier, runner Runner) (InstallReport, error) {
	prepared, err := PrepareInstall(m, opts, look, fontLook, tier)
	if err != nil {
		return InstallReport{}, err
	}
	return InstallPrepared(prepared, opts, look, fontLook, runner)
}

// InstallPrepared executes the exact actions selected by PrepareInstall. It
// may re-probe presence to avoid redundant work, but does not re-resolve an
// action's provider, package, artifact, or command.
func InstallPrepared(prepared PreparedInstall, opts Options, look Lookup, fontLook FontLookup, runner Runner) (InstallReport, error) {
	plan := prepared.Plan
	report := InstallReport{Profile: plan.Profile, Profiles: plan.Profiles, Tags: plan.Tags, Tier: plan.Tier}
	requiredUnresolved := false
	executionLook := look
	toolchainEnvironment, hasToolchainEnvironment := runner.(ToolchainEnvironment)
	if hasToolchainEnvironment {
		executionLook = toolchainEnvironment.Lookup
	}
	for _, action := range plan.Actions {
		// A dependency can become present between preview and execution (or after
		// an earlier toolchain action). It is safe to skip, but never to replace,
		// the reviewed action.
		if actionPresent(action, opts, executionLook, fontLook) {
			continue
		}
		if !actionExecutable(action) {
			if action.Requirement == manifest.DependencyRequirementRequired {
				requiredUnresolved = true
			}
			report.Items = append(report.Items, InstallItem{
				Dependency:   action.Dependency,
				Requirement:  action.Requirement,
				Status:       InstallStatusManual,
				Manual:       action.Manual,
				TrustCommand: action.TrustCommand,
				Candidates:   action.Candidates,
				UserLocal:    action.UserLocal,
			})
			continue
		}
		args := installArgsWithConfirmation(action)
		if action.UserLocal != nil {
			localRunner, ok := runner.(UserLocalRunner)
			if !ok {
				return report, fmt.Errorf("install %q: user-local runner unavailable", action.Dependency)
			}
			if err := localRunner.RunUserLocal(action); err != nil {
				report.Items = append(report.Items, InstallItem{Dependency: action.Dependency, Requirement: action.Requirement, Status: InstallStatusFailed, Provider: action.Provider, Package: action.Package, UserLocal: action.UserLocal, Candidates: action.Candidates})
				if action.Requirement == manifest.DependencyRequirementOptional {
					continue
				}
				return report, fmt.Errorf("install %q: %w", action.Dependency, err)
			}
		} else if action.Executable != "" {
			if err := runner.Run(action.Executable, args); err != nil {
				report.Items = append(report.Items, InstallItem{
					Dependency:   action.Dependency,
					Requirement:  action.Requirement,
					Status:       InstallStatusFailed,
					Provider:     action.Provider,
					Package:      action.Package,
					Executable:   action.Executable,
					Args:         args,
					Bootstrap:    append([]Command(nil), action.Bootstrap...),
					TrustCommand: action.TrustCommand,
					Candidates:   action.Candidates,
				})
				if action.Requirement == manifest.DependencyRequirementOptional {
					continue
				}
				return report, fmt.Errorf("install %q: %w", action.Dependency, err)
			}
		}
		if action.Provider == TierHomebrew && action.Toolchain == manifest.DependencyToolchainRustStableRustup {
			if pathRunner, ok := runner.(HomebrewFormulaPATHRunner); ok {
				if err := pathRunner.AddHomebrewFormulaToPATH(action.Package); err == nil {
					executionLook = pathRunner.Lookup
				}
			}
		}
		if len(action.Bootstrap) > 0 && !bootstrapRunnable(action.Bootstrap, executionLook) {
			if action.Requirement == manifest.DependencyRequirementRequired {
				requiredUnresolved = true
			}
			report.Items = append(report.Items, InstallItem{
				Dependency:   action.Dependency,
				Requirement:  action.Requirement,
				Status:       InstallStatusUnresolved,
				Provider:     action.Provider,
				Package:      action.Package,
				Executable:   action.Executable,
				Args:         args,
				Bootstrap:    append([]Command(nil), action.Bootstrap...),
				Manual:       unresolvedToolchainRemediation(action, executionLook),
				TrustCommand: action.TrustCommand,
				Candidates:   action.Candidates,
				UserLocal:    action.UserLocal,
			})
			if action.Requirement == manifest.DependencyRequirementRequired {
				return report, errors.New("unresolved required dependencies remain after install")
			}
			continue
		}
		if err := runBootstrap(action, runner); err != nil {
			report.Items = append(report.Items, InstallItem{
				Dependency:   action.Dependency,
				Requirement:  action.Requirement,
				Status:       InstallStatusFailed,
				Provider:     action.Provider,
				Package:      action.Package,
				Executable:   action.Executable,
				Args:         args,
				Bootstrap:    append([]Command(nil), action.Bootstrap...),
				TrustCommand: action.TrustCommand,
				Candidates:   action.Candidates,
				UserLocal:    action.UserLocal,
			})
			if action.Requirement == manifest.DependencyRequirementOptional {
				continue
			}
			return report, fmt.Errorf("bootstrap %q: %w", action.Dependency, err)
		}
		if hasToolchainEnvironment && action.Toolchain != "" {
			// Activation failure is represented as unresolved below: a successful
			// bootstrap is not proof that the selected runtime is executable.
			if err := toolchainEnvironment.ActivateToolchain(action.Toolchain); err == nil {
				executionLook = toolchainEnvironment.Lookup
			}
		}
		if !actionPresent(action, opts, executionLook, fontLook) {
			manual := unresolvedToolchainRemediation(action, executionLook)
			if action.Requirement == manifest.DependencyRequirementRequired {
				requiredUnresolved = true
			}
			report.Items = append(report.Items, InstallItem{
				Dependency:   action.Dependency,
				Requirement:  action.Requirement,
				Status:       InstallStatusUnresolved,
				Provider:     action.Provider,
				Package:      action.Package,
				Executable:   action.Executable,
				Args:         args,
				Bootstrap:    append([]Command(nil), action.Bootstrap...),
				Manual:       manual,
				TrustCommand: action.TrustCommand,
				Candidates:   action.Candidates,
				UserLocal:    action.UserLocal,
			})
			if action.Requirement == manifest.DependencyRequirementRequired {
				return report, errors.New("unresolved required dependencies remain after install")
			}
			continue
		}
		if action.UserLocal != nil {
			localRunner, ok := runner.(UserLocalRunner)
			if !ok {
				return report, fmt.Errorf("install %q: user-local runner unavailable", action.Dependency)
			}
			if err := localRunner.RecordUserLocal(action); err != nil {
				return report, fmt.Errorf("record dependency metadata for %q: %w", action.Dependency, err)
			}
		}
		report.Items = append(report.Items, InstallItem{
			Dependency:   action.Dependency,
			Requirement:  action.Requirement,
			Status:       InstallStatusInstalled,
			Provider:     action.Provider,
			Package:      action.Package,
			Executable:   action.Executable,
			Args:         args,
			Bootstrap:    append([]Command(nil), action.Bootstrap...),
			TrustCommand: action.TrustCommand,
			Candidates:   action.Candidates,
			UserLocal:    action.UserLocal,
		})
	}
	if requiredUnresolved {
		return report, errors.New("unresolved required dependencies remain after install")
	}
	return report, nil
}

func unresolvedToolchainRemediation(action InstallAction, look Lookup) string {
	if action.UserLocal != nil {
		missing := missingCommandProbes(action.Probes, look)
		if len(missing) == 0 {
			return ""
		}
		return fmt.Sprintf("%s installed through the user-local provider, but %s is still not available on PATH; ensure ~/.local/bin is on PATH and rerun dots deps check", action.Dependency, strings.Join(missing, ", "))
	}
	if action.Toolchain == manifest.DependencyToolchainNodeLTSFNM {
		return "Node LTS was installed through fnm, but node is not executable in fnm's selected environment; verify with `fnm exec --using=lts/latest node --version`, then rerun dots deps install"
	}
	if action.Toolchain != manifest.DependencyToolchainRustStableRustup {
		return ""
	}
	missing := missingCommandProbes(action.Probes, look)
	if len(missing) == 0 {
		return ""
	}
	verb := "are"
	if len(missing) == 1 {
		verb = "is"
	}
	pathGuidance := "ensure rustup exposes its proxies (usually by adding ~/.cargo/bin to PATH)"
	if action.Provider == TierHomebrew {
		pathGuidance = "add the Homebrew rustup proxy directory from `brew --prefix rustup` (usually `$(brew --prefix rustup)/bin`) to PATH"
	}
	return fmt.Sprintf("Rust stable is selected in rustup, but %s %s not available on PATH; %s, then verify with `rustup which rustc` and `rustup which cargo`", strings.Join(missing, ", "), verb, pathGuidance)
}

func missingCommandProbes(probes []string, look Lookup) []string {
	missing := make([]string, 0, len(probes))
	for _, probe := range probes {
		probe = strings.TrimSpace(probe)
		if probe == "" || look(probe) {
			continue
		}
		missing = append(missing, probe)
	}
	return missing
}

type UserLocalRunner interface {
	RunUserLocal(action InstallAction) error
	RecordUserLocal(action InstallAction) error
}

func actionExecutable(action InstallAction) bool {
	return action.Status == InstallActionStatusInstallable
}

func runBootstrap(action InstallAction, runner Runner) error {
	for _, command := range action.Bootstrap {
		if err := runner.Run(command.Executable, append([]string(nil), command.Args...)); err != nil {
			return err
		}
	}
	return nil
}

func actionPresent(action InstallAction, opts Options, look Lookup, fontLook FontLookup) bool {
	matches := action.FontMatches
	if len(matches) == 0 && action.FontMatch != "" {
		matches = []string{action.FontMatch}
	}
	if len(matches) > 0 {
		return fontPresent(matches, fontLook)
	}
	probes := action.Probes
	if len(probes) == 0 && action.Probe != "" {
		probes = []string{action.Probe}
	}
	return commandsPresent(probes, look) || darwinAppPresent(action.DarwinApp, opts)
}

func installArgsWithConfirmation(action InstallAction) []string {
	args := append([]string(nil), action.Args...)
	if len(args) == 0 {
		return args
	}
	pkg := args[len(args)-1]
	prefix := append([]string(nil), args[:len(args)-1]...)
	switch action.Provider {
	case TierDebian, TierFedora:
		prefix = append(prefix, "-y")
	case TierArch:
		prefix = append(prefix, "--noconfirm")
	}
	return append(prefix, pkg)
}
