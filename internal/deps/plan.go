package deps

import (
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// ProviderCandidate is one ordered installation provider considered for a
// missing Dependency. Available is true only when the provider executable can be
// resolved on the host; unavailable candidates are retained so JSON output shows
// why fallback happened.
type ProviderCandidate struct {
	Provider     Tier     `json:"provider"`
	Package      string   `json:"package,omitempty"`
	Executable   string   `json:"executable,omitempty"`
	Args         []string `json:"args,omitempty"`
	Available    bool     `json:"available"`
	Manual       string   `json:"manual,omitempty"`
	TrustCommand string   `json:"trust_command,omitempty"`
}

// InstallAction is the structured installation intent for one missing
// Dependency. Executable and Args are safe argv-shaped data for future runners;
// Manual is set when no executable provider candidate is available.
type InstallAction struct {
	Dependency   string              `json:"dependency"`
	Probe        string              `json:"probe,omitempty"`
	FontMatch    string              `json:"font_match,omitempty"`
	FontMatches  []string            `json:"font_matches,omitempty"`
	Provider     Tier                `json:"provider,omitempty"`
	Package      string              `json:"package,omitempty"`
	Executable   string              `json:"executable,omitempty"`
	Args         []string            `json:"args,omitempty"`
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
	action := InstallAction{Dependency: dep.Name, Probe: dep.Probe(), FontMatch: fontMatch, FontMatches: fontMatches}

	for _, candidate := range providerCandidates(dep, opts, tier, look) {
		action.Candidates = append(action.Candidates, candidate)
		if !candidate.Available || candidate.Executable == "" {
			continue
		}
		action.Provider = candidate.Provider
		action.Package = candidate.Package
		action.Executable = candidate.Executable
		action.Args = append([]string(nil), candidate.Args...)
		action.TrustCommand = candidate.TrustCommand
		return action
	}

	action.Manual = manualNote(dep, opts, tier, action.Candidates)
	return action
}

// guidanceFor renders advisory compatibility fields from the structured action.
func guidanceFor(action InstallAction) Guidance {
	if action.Executable == "" {
		return Guidance{Name: action.Dependency, Manual: action.Manual, TrustCommand: action.TrustCommand, Action: action}
	}
	return Guidance{Name: action.Dependency, Command: action.commandHint(), TrustCommand: action.TrustCommand, Action: action}
}

func (a InstallAction) commandHint() string {
	parts := append([]string{a.Executable}, a.Args...)
	return strings.Join(parts, " ")
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
	if tier != TierHomebrew {
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
