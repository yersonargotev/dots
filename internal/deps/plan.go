package deps

import (
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// InstallAction is the structured installation intent for one missing
// Dependency. Executable and Args are safe argv-shaped data for future runners;
// Manual is set when the dependency has no executable package-manager action for
// the active Tier.
type InstallAction struct {
	Dependency string
	Probe      string
	FontMatch  string
	Package    string
	Executable string
	Args       []string
	Manual     string
}

// Guidance is the advisory installation hint for one missing Dependency. Command
// is a human-rendered install command when the Dependency has a package mapping
// for the active Tier; otherwise Manual carries a fallback note and Command is
// empty. Action carries the structured model that Command renders from.
type Guidance struct {
	Name    string
	Command string
	Manual  string
	Action  InstallAction
}

// PlanReport is the Dependency Plan for a Profile: OS-aware guidance for the
// missing Dependencies under the active Tier. It is advisory only.
type PlanReport struct {
	Profile string
	Tier    Tier
	Actions []InstallAction
	Items   []Guidance
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
		action := actionFor(dep, tier)
		report.Actions = append(report.Actions, action)
		report.Items = append(report.Items, guidanceFor(action))
	}
	return report, nil
}

// actionFor builds the structured install action for a single missing
// Dependency under a Tier.
func actionFor(dep manifest.Dependency, tier Tier) InstallAction {
	pkg, executable, args := tierPackage(dep, tier)
	if pkg == "" {
		return InstallAction{Dependency: dep.Name, Probe: dep.Probe(), FontMatch: strings.TrimSpace(dep.FontMatch), Manual: manualNote(dep, tier)}
	}
	return InstallAction{
		Dependency: dep.Name,
		Probe:      dep.Probe(),
		FontMatch:  strings.TrimSpace(dep.FontMatch),
		Package:    pkg,
		Executable: executable,
		Args:       append(args, pkg),
	}
}

// guidanceFor renders advisory compatibility fields from the structured action.
func guidanceFor(action InstallAction) Guidance {
	if action.Executable == "" {
		return Guidance{Name: action.Dependency, Manual: action.Manual, Action: action}
	}
	return Guidance{Name: action.Dependency, Command: action.commandHint(), Action: action}
}

func (a InstallAction) commandHint() string {
	parts := append([]string{a.Executable}, a.Args...)
	return strings.Join(parts, " ")
}

// tierPackage returns the package identifier and argv prefix for a Dependency
// under a Tier. A blank package means there is no mapping and the caller must
// fall back to manual guidance.
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

func manualNote(dep manifest.Dependency, tier Tier) string {
	if dep.IsFont() && tier != TierHomebrew {
		name := strings.TrimSpace(dep.Name)
		match := strings.TrimSpace(dep.FontMatch)
		if cask := strings.TrimSpace(dep.BrewCask); cask != "" {
			return fmt.Sprintf("obtain the font files for %q manually on Linux using Homebrew cask token %q as the package/source clue; copy .ttf/.otf files into ~/.local/share/fonts; run fc-cache -f ~/.local/share/fonts; rerun dots deps check; dots will detect files matching font_match %q", name, cask, match)
		}
		return fmt.Sprintf("obtain the font files for %q manually on Linux; copy .ttf/.otf files into ~/.local/share/fonts; run fc-cache -f ~/.local/share/fonts; rerun dots deps check; dots will detect files matching font_match %q", name, match)
	}
	if tier == TierGeneric {
		return fmt.Sprintf("install %q with your distribution's package manager", dep.Name)
	}
	return fmt.Sprintf("no %s package declared for %q; install it manually", tier, dep.Name)
}
