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
}

// InstallDryRunReport previews the install actions for a Profile without
// invoking any package manager.
type InstallDryRunReport struct {
	Profile string           `json:"profile"`
	Tier    Tier             `json:"tier"`
	Items   []InstallPreview `json:"items"`
}

// Runner executes one argv-shaped install action.
type Runner interface {
	Run(executable string, args []string) error
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
}

// InstallReport records the stable dots summary for a real install run.
type InstallReport struct {
	Profile string        `json:"profile"`
	Tier    Tier          `json:"tier"`
	Items   []InstallItem `json:"items"`
}

// InstallDryRun computes the install preview for missing Dependencies without
// executing package managers.
func InstallDryRun(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup, tier Tier) (InstallDryRunReport, error) {
	plan, err := Plan(m, opts, look, fontLook, tier)
	if err != nil {
		return InstallDryRunReport{}, err
	}

	report := InstallDryRunReport{Profile: plan.Profile, Tier: plan.Tier}
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
		})
	}
	return report, nil
}

// Install executes missing executable install actions and re-probes each
// dependency after a successful package-manager command.
func Install(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup, tier Tier, runner Runner) (InstallReport, error) {
	plan, err := Plan(m, opts, look, fontLook, tier)
	if err != nil {
		return InstallReport{}, err
	}

	report := InstallReport{Profile: plan.Profile, Tier: plan.Tier}
	requiredUnresolved := false
	for _, action := range plan.Actions {
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
			})
			continue
		}
		args := installArgsWithConfirmation(action)
		if action.Executable != "" {
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
			})
			if action.Requirement == manifest.DependencyRequirementOptional {
				continue
			}
			return report, fmt.Errorf("bootstrap %q: %w", action.Dependency, err)
		}
		if !actionPresent(action, look, fontLook) {
			manual := unresolvedToolchainRemediation(action, look)
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
			})
			if action.Requirement == manifest.DependencyRequirementRequired {
				return report, errors.New("unresolved required dependencies remain after install")
			}
			continue
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
		})
	}
	if requiredUnresolved {
		return report, errors.New("unresolved required dependencies remain after install")
	}
	return report, nil
}

func unresolvedToolchainRemediation(action InstallAction, look Lookup) string {
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
	return fmt.Sprintf("Rust stable is selected in rustup, but %s %s not available on PATH; ensure rustup exposes its proxies (usually by adding ~/.cargo/bin to PATH), then verify with `rustup which rustc` and `rustup which cargo`", strings.Join(missing, ", "), verb)
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

func actionExecutable(action InstallAction) bool {
	return action.Executable != "" || len(action.Bootstrap) > 0
}

func runBootstrap(action InstallAction, runner Runner) error {
	for _, command := range action.Bootstrap {
		if err := runner.Run(command.Executable, append([]string(nil), command.Args...)); err != nil {
			return err
		}
	}
	return nil
}

func actionPresent(action InstallAction, look Lookup, fontLook FontLookup) bool {
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
	return commandsPresent(probes, look)
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
