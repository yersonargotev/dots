package deps

import (
	"fmt"

	"github.com/yersonargotev/dots/internal/manifest"
)

// Guidance is the advisory installation hint for one missing Dependency. Command
// is a ready-to-run install command when the Dependency has a package mapping for
// the active Tier; otherwise Manual carries a fallback note and Command is empty.
type Guidance struct {
	Name    string
	Command string
	Manual  string
}

// PlanReport is the Dependency Plan for a Profile: OS-aware guidance for the
// missing Dependencies under the active Tier. It is advisory only.
type PlanReport struct {
	Profile string
	Tier    Tier
	Items   []Guidance
}

// Plan computes advisory installation guidance for the Dependencies that the
// Profile needs but the workstation is missing, tailored to the active Tier.
func Plan(m manifest.Manifest, opts Options, look Lookup, tier Tier) (PlanReport, error) {
	selected, err := selectDependencies(m, opts)
	if err != nil {
		return PlanReport{}, err
	}

	report := PlanReport{Profile: opts.Profile, Tier: tier}
	for _, dep := range selected {
		if look(dep.Probe()) {
			continue
		}
		report.Items = append(report.Items, guidanceFor(dep, tier))
	}
	return report, nil
}

// guidanceFor builds the install hint for a single missing Dependency under a
// Tier. When the Dependency declares a package for that Tier, the hint is a
// concrete install command; otherwise it is a manual fallback note.
func guidanceFor(dep manifest.Dependency, tier Tier) Guidance {
	pkg, template := tierPackage(dep, tier)
	if pkg == "" {
		return Guidance{Name: dep.Name, Manual: manualNote(dep, tier)}
	}
	return Guidance{Name: dep.Name, Command: fmt.Sprintf(template, pkg)}
}

// tierPackage returns the package identifier and the install command template
// for a Dependency under a Tier. A blank package means there is no mapping and
// the caller must fall back to manual guidance.
func tierPackage(dep manifest.Dependency, tier Tier) (pkg, template string) {
	switch tier {
	case TierHomebrew:
		return dep.Brew, "brew install %s"
	case TierDebian:
		return dep.Apt, "sudo apt-get install %s"
	case TierFedora:
		return dep.Dnf, "sudo dnf install %s"
	case TierArch:
		return dep.Pacman, "sudo pacman -S %s"
	default:
		return "", ""
	}
}

func manualNote(dep manifest.Dependency, tier Tier) string {
	if tier == TierGeneric {
		return fmt.Sprintf("install %q with your distribution's package manager", dep.Name)
	}
	return fmt.Sprintf("no %s package declared for %q; install it manually", tier, dep.Name)
}
