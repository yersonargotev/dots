// Package deps computes Dependency findings for a Profile: which external tools
// a workstation needs for its selected Managed Entries and Provisioners, whether
// they are present, OS-aware advisory guidance for installing the missing ones,
// and explicitly confirmed package-manager execution.
package deps

import (
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// Lookup reports whether a command is available on the workstation. It is the
// injectable boundary around PATH probing so dependency checks stay deterministic
// in tests.
type Lookup func(command string) bool

// FontLookup reports whether a font matching the declared glob is installed on
// the workstation. It is the injectable boundary around font-directory scanning,
// parallel to Lookup, so font dependency checks stay deterministic in tests.
type FontLookup func(match string) bool

// Options carries the resolved inputs needed to select Dependencies.
type Options struct {
	Profile string
	OS      string
}

// Result is the presence finding for a single declared Dependency.
type Result struct {
	Name    string
	Command string
	Present bool
}

// CheckReport is the Dependency presence report for a Profile.
type CheckReport struct {
	Profile string
	Results []Result
}

// Check reports which Dependencies declared by the Profile's selected Managed
// Entries and Provisioners are present on the workstation. Dependencies are
// deduplicated by name in first-declared order.
func Check(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup) (CheckReport, error) {
	selected, err := selectDependencies(m, opts)
	if err != nil {
		return CheckReport{}, err
	}

	report := CheckReport{Profile: opts.Profile}
	for _, dep := range selected {
		result := checkResult(dep, look, fontLook)
		report.Results = append(report.Results, result)
	}
	return report, nil
}

func dependencyPresent(dep manifest.Dependency, look Lookup, fontLook FontLookup) bool {
	if dep.IsFont() {
		return fontLook(strings.TrimSpace(dep.FontMatch))
	}
	return look(dep.Probe())
}

func checkResult(dep manifest.Dependency, look Lookup, fontLook FontLookup) Result {
	if dep.IsFont() {
		// A font has no executable on PATH; detect it as an installed asset
		// by scanning the workstation font directories for a matching file.
		match := strings.TrimSpace(dep.FontMatch)
		return Result{Name: dep.Name, Command: match, Present: fontLook(match)}
	}
	probe := dep.Probe()
	return Result{Name: dep.Name, Command: probe, Present: look(probe)}
}

// selectDependencies gathers the Dependencies of every Managed Entry and
// Provisioner that belongs to the Profile and passes the OS filter, deduplicated
// by name in first-declared order.
func selectDependencies(m manifest.Manifest, opts Options) ([]manifest.Dependency, error) {
	profile, ok := m.Profiles[opts.Profile]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", opts.Profile)
	}

	var selected []manifest.Dependency
	seen := make(map[string]bool)
	addDependencies := func(deps []manifest.Dependency) {
		for _, dep := range deps {
			// Normalize the name so padded and unpadded declarations of the same
			// dependency deduplicate and render consistently with Probe().
			dep.Name = strings.TrimSpace(dep.Name)
			if seen[dep.Name] {
				continue
			}
			seen[dep.Name] = true
			selected = append(selected, dep)
		}
	}

	for _, entry := range m.Entries {
		if !sharesTag(entry.Tags, profile.Tags) {
			continue
		}
		if !matchesOS(entry.OS, opts.OS) {
			continue
		}
		addDependencies(entry.Dependencies)
	}
	for _, prov := range m.Provisioners {
		if !sharesTag(prov.Tags, profile.Tags) {
			continue
		}
		if !matchesOS(prov.OS, opts.OS) {
			continue
		}
		addDependencies(prov.Dependencies)
	}
	return selected, nil
}

func sharesTag(entryTags, profileTags []string) bool {
	for _, et := range entryTags {
		for _, pt := range profileTags {
			if et == pt {
				return true
			}
		}
	}
	return false
}

func matchesOS(entryOS []string, current string) bool {
	if len(entryOS) == 0 {
		return true
	}
	for _, osName := range entryOS {
		if osName == current {
			return true
		}
	}
	return false
}
