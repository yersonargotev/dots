package deps

import (
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// ProviderCandidate is one ordered installation provider considered for a
// missing Dependency. Availability is used internally to select an executable
// provider, but is deliberately excluded from JSON so machine-local state does
// not leak into the Agent Output Contract.
type ProviderCandidate struct {
	Provider     Tier     `json:"provider"`
	Package      string   `json:"package,omitempty"`
	Executable   string   `json:"executable,omitempty"`
	Args         []string `json:"args,omitempty"`
	Available    bool     `json:"-"`
	Manual       string   `json:"manual,omitempty"`
	TrustCommand string   `json:"trust_command,omitempty"`
}

// Command is one deterministic argv-shaped command dots may execute as part of
// a constrained dependency action.
type Command struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

// InstallActionStatus describes the stable outcome for a missing Dependency.
type InstallActionStatus string

const (
	InstallActionStatusInstallable InstallActionStatus = "installable"
	InstallActionStatusManual      InstallActionStatus = "manual"
)

// InstallAction is the structured installation intent for one missing
// Dependency. Executable and Args are safe argv-shaped data for future runners;
// Manual is set when no executable provider candidate is available.
type InstallAction struct {
	Dependency   string              `json:"dependency"`
	Requirement  string              `json:"requirement"`
	Status       InstallActionStatus `json:"status"`
	Probe        string              `json:"probe,omitempty"`
	Probes       []string            `json:"probes,omitempty"`
	FontMatch    string              `json:"font_match,omitempty"`
	FontMatches  []string            `json:"font_matches,omitempty"`
	Toolchain    string              `json:"toolchain,omitempty"`
	Provider     Tier                `json:"provider,omitempty"`
	Package      string              `json:"package,omitempty"`
	Executable   string              `json:"executable,omitempty"`
	Args         []string            `json:"args,omitempty"`
	Bootstrap    []Command           `json:"bootstrap,omitempty"`
	Manual       string              `json:"manual,omitempty"`
	TrustCommand string              `json:"trust_command,omitempty"`
	Candidates   []ProviderCandidate `json:"candidates,omitempty"`
}

// Guidance is the advisory installation hint for one missing Dependency. Command
// is a human-rendered install command when the Dependency has a package mapping
// for the active Tier; otherwise Manual carries a fallback note and Command is
// empty. Action carries the structured model that Command renders from.
type Guidance struct {
	Name         string        `json:"name"`
	Requirement  string        `json:"requirement"`
	Command      string        `json:"command,omitempty"`
	Manual       string        `json:"manual,omitempty"`
	TrustCommand string        `json:"trust_command,omitempty"`
	Action       InstallAction `json:"action"`
}

// PlanReport is the Dependency Plan for a Profile: OS-aware guidance for the
// missing Dependencies under the active Tier. It is advisory only.
type PlanReport struct {
	Profile string          `json:"profile"`
	Tier    Tier            `json:"tier"`
	Actions []InstallAction `json:"actions"`
	Items   []Guidance      `json:"items"`
}

// HasFindings reports whether the Dependency Plan lists any missing Dependency.
// A PlanReport only carries guidance for absent Dependencies, so a non-empty
// plan is itself the finding.
func (r PlanReport) HasFindings() bool {
	return len(r.Items) > 0
}

// Plan computes advisory installation guidance for the Dependencies that the
// Profile needs but the workstation is missing, tailored to the active Tier.
func Plan(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup, tier Tier) (PlanReport, error) {
	selected, err := selectDependencies(m, opts)
	if err != nil {
		return PlanReport{}, err
	}

	report := PlanReport{Profile: opts.Profile, Tier: tier}
	for _, dep := range selected {
		if dependencyPresent(dep, look, fontLook) {
			continue
		}
		action := actionFor(dep, opts, tier, look)
		report.Actions = append(report.Actions, action)
		report.Items = append(report.Items, guidanceFor(action))
	}
	return report, nil
}

// actionFor builds the structured install action for a single missing
// Dependency under a Tier, falling through ordered provider candidates before
// returning manual guidance.
func actionFor(dep manifest.Dependency, opts Options, tier Tier, look Lookup) InstallAction {
	fontMatches := dep.FontMatches()
	fontMatch := ""
	if len(fontMatches) > 0 {
		fontMatch = fontMatches[0]
	}
	probes := dep.Probes()
	action := InstallAction{Dependency: dep.Name, Requirement: dep.RequirementValue(), Status: InstallActionStatusManual, Probe: dep.Probe(), Probes: probes, FontMatch: fontMatch, FontMatches: fontMatches, Toolchain: strings.TrimSpace(dep.Toolchain), Bootstrap: bootstrapCommands(dep)}

	if officialRustupInstallerRunnable(action, opts, look) {
		action.Status = InstallActionStatusInstallable
		action.Executable = "sh"
		action.Args = []string{"-c", officialRustupInstallerScript}
		return action
	}

	candidates := providerCandidates(dep, opts, tier, look)
	if officialFNMInstallerRunnable(action, opts, look, candidates) {
		action.Status = InstallActionStatusInstallable
		action.Executable = "bash"
		action.Args = []string{"-c", officialFNMInstallerScript}
		action.Candidates = append(action.Candidates, candidates...)
		return action
	}

	for _, candidate := range candidates {
		action.Candidates = append(action.Candidates, candidate)
		if !candidate.Available || candidate.Executable == "" {
			continue
		}
		action.Status = InstallActionStatusInstallable
		action.Provider = candidate.Provider
		action.Package = candidate.Package
		action.Executable = candidate.Executable
		action.Args = append([]string(nil), candidate.Args...)
		action.TrustCommand = candidate.TrustCommand
		return action
	}

	if bootstrapRunnable(action.Bootstrap, look) {
		action.Status = InstallActionStatusInstallable
		return action
	}

	action.Manual = manualNote(dep, opts, tier, action.Candidates)
	return action
}

// guidanceFor renders advisory compatibility fields from the structured action.
func guidanceFor(action InstallAction) Guidance {
	if action.Executable == "" {
		return Guidance{Name: action.Dependency, Requirement: action.Requirement, Manual: action.Manual, TrustCommand: action.TrustCommand, Action: action}
	}
	return Guidance{Name: action.Dependency, Requirement: action.Requirement, Command: action.commandHint(), TrustCommand: action.TrustCommand, Action: action}
}

const officialRustupInstallerScript = "curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --no-modify-path"
const officialFNMInstallerScript = "curl -fsSL https://fnm.vercel.app/install | bash -s -- --skip-shell"

func officialRustupInstallerRunnable(action InstallAction, opts Options, look Lookup) bool {
	return opts.OS == "linux" &&
		action.Toolchain == manifest.DependencyToolchainRustStableRustup &&
		!look("rustup") &&
		look("curl") &&
		look("sh")
}

func officialFNMInstallerRunnable(action InstallAction, opts Options, look Lookup, candidates []ProviderCandidate) bool {
	if opts.OS != "linux" ||
		action.Toolchain != manifest.DependencyToolchainNodeLTSFNM ||
		look("fnm") ||
		!look("curl") ||
		!look("bash") ||
		!look("unzip") {
		return false
	}
	for _, candidate := range candidates {
		if candidate.Available && candidate.Executable != "" {
			return false
		}
	}
	return true
}

func bootstrapRunnable(commands []Command, look Lookup) bool {
	if len(commands) == 0 {
		return false
	}
	for _, command := range commands {
		if !look(command.Executable) {
			return false
		}
	}
	return true
}

func (a InstallAction) commandHint() string {
	parts := append([]string{a.Executable}, a.Args...)
	for i, part := range parts {
		parts[i] = shellQuoteIfNeeded(part)
	}
	return strings.Join(parts, " ")
}

func shellQuoteIfNeeded(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			strings.ContainsRune("_@%+=:,./-", r))
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func bootstrapCommands(dep manifest.Dependency) []Command {
	switch strings.TrimSpace(dep.Toolchain) {
	case manifest.DependencyToolchainNodeLTSFNM:
		return []Command{{Executable: "fnm", Args: []string{"install", "--lts"}}}
	case manifest.DependencyToolchainRustStableRustup:
		return []Command{{Executable: "rustup", Args: []string{"default", "stable"}}}
	default:
		return nil
	}
}

func homebrewTapTrustCommand(dep manifest.Dependency, tier Tier) string {
	if tier != TierHomebrew || strings.TrimSpace(dep.BrewCask) != "" {
		return ""
	}
	formula := strings.TrimSpace(dep.Brew)
	if strings.Count(formula, "/") < 2 {
		return ""
	}
	return "brew trust --formula " + formula
}

func providerCandidates(dep manifest.Dependency, opts Options, tier Tier, look Lookup) []ProviderCandidate {
	if opts.OS == "linux" && dep.IsFont() {
		return nil
	}

	candidateTiers := []Tier{tier}
	if opts.OS == "linux" && tier != TierHomebrew && dep.LinuxHomebrew {
		candidateTiers = append(candidateTiers, TierHomebrew)
	}

	candidates := make([]ProviderCandidate, 0, len(candidateTiers))
	for _, candidateTier := range candidateTiers {
		pkg, executable, args := tierPackage(dep, candidateTier)
		if pkg == "" || executable == "" {
			continue
		}
		fullArgs := append(args, pkg)
		candidates = append(candidates, ProviderCandidate{
			Provider:     candidateTier,
			Package:      pkg,
			Executable:   executable,
			Args:         fullArgs,
			Available:    providerAvailable(candidateTier, look),
			TrustCommand: homebrewTapTrustCommand(dep, candidateTier),
		})
	}
	return candidates
}

func providerAvailable(tier Tier, look Lookup) bool {
	switch tier {
	case TierHomebrew:
		return look("brew")
	case TierDebian:
		return look("apt-get") && look("sudo")
	case TierFedora:
		return look("dnf") && look("sudo")
	case TierArch:
		return look("pacman") && look("sudo")
	default:
		return false
	}
}

// tierPackage returns the package identifier and argv prefix for a Dependency
// under a Tier. A blank package means there is no mapping and the caller must
// fall back to another provider or manual guidance.
func tierPackage(dep manifest.Dependency, tier Tier) (pkg, executable string, args []string) {
	switch tier {
	case TierHomebrew:
		if pkg := strings.TrimSpace(dep.BrewCask); pkg != "" {
			return pkg, "brew", []string{"install", "--cask"}
		}
		return strings.TrimSpace(dep.Brew), "brew", []string{"install"}
	case TierDebian:
		return strings.TrimSpace(dep.Apt), "sudo", []string{"apt-get", "install"}
	case TierFedora:
		return strings.TrimSpace(dep.Dnf), "sudo", []string{"dnf", "install"}
	case TierArch:
		return strings.TrimSpace(dep.Pacman), "sudo", []string{"pacman", "-S"}
	default:
		return "", "", nil
	}
}

func manualNote(dep manifest.Dependency, opts Options, tier Tier, candidates []ProviderCandidate) string {
	if dep.IsFont() && opts.OS == "linux" {
		name := strings.TrimSpace(dep.Name)
		matchHint := fontMatchHint(dep.FontMatches())
		if cask := strings.TrimSpace(dep.BrewCask); cask != "" {
			return fmt.Sprintf("obtain the font files for %q manually on Linux using Homebrew cask token %q as the package/source clue; copy .ttf/.otf files into ~/.local/share/fonts; run fc-cache -f ~/.local/share/fonts; rerun dots deps check; dots will detect files matching %s", name, cask, matchHint)
		}
		return fmt.Sprintf("obtain the font files for %q manually on Linux; copy .ttf/.otf files into ~/.local/share/fonts; run fc-cache -f ~/.local/share/fonts; rerun dots deps check; dots will detect files matching %s", name, matchHint)
	}
	if len(candidates) > 0 {
		return fmt.Sprintf("no executable dependency provider available for %q; install it manually", dep.Name)
	}
	if tier == TierGeneric {
		return fmt.Sprintf("install %q with your distribution's package manager", dep.Name)
	}
	return fmt.Sprintf("no %s package declared for %q; install it manually", tier, dep.Name)
}

func fontMatchHint(matches []string) string {
	if len(matches) == 0 {
		return `font_match ""`
	}
	if len(matches) == 1 {
		return fmt.Sprintf("font_match %q", matches[0])
	}
	return fmt.Sprintf("font_match %q or compatible fallback patterns %q", matches[0], matches[1:])
}
