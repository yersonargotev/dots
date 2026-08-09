// Package selectedsurface evaluates the declarative Install Manifest surface
// selected by effective tags and an operating system. It is deliberately pure:
// it does not resolve profiles, inspect state, or touch the filesystem.
package selectedsurface

import (
	"reflect"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// Surface is the ordered, applicable declaration surface for a selection.
// Dependencies is the execution-oriented, name-deduplicated view. DependencyOrigins
// retains every declaration occurrence for callers that need to explain it.
type Surface struct {
	Tags              []string
	DependencySets    []manifest.DependencySet
	Entries           []SelectedEntry
	Dependencies      []manifest.Dependency
	DependencyOrigins []DependencyOrigin
	Provisioners      []manifest.Provisioner
	SourceOverrides   []SourceOverride
}

// SelectedEntry is an applicable Managed Entry with its selected Source of
// Truth. OverrideTag is empty when the entry's base Source wins.
type SelectedEntry struct {
	Entry       manifest.Entry
	Source      string
	OverrideTag string
}

// DependencyOrigin records one dependency declaration and its manifest origin.
type DependencyOrigin struct {
	Dependency manifest.Dependency
	Origin     Origin
}

// Origin identifies the manifest declaration that contributed a dependency.
type Origin struct {
	Type string
	Name string
	Tags []string
}

// SourceOverride is an active, OS-applicable entry source override. It remains
// present even if the entry's own Tags are not selected.
type SourceOverride struct {
	Entry  manifest.Entry
	Tag    string
	Source string
}

// Evaluate returns the pure selected surface. Effective tags are normalized to
// their first occurrence. All remaining collections retain manifest order.
func Evaluate(m manifest.Manifest, effectiveTags []string, osName string) Surface {
	return evaluate(m, effectiveTags, func(itemOS []string) bool {
		return manifest.MatchesOS(itemOS, osName)
	})
}

// EvaluateAll returns the portable selected surface across Darwin and Linux.
// It evaluates each declaration once, preserving manifest order without
// introducing an all-platform sentinel into the selected surface.
func EvaluateAll(m manifest.Manifest, effectiveTags []string) Surface {
	return evaluate(m, effectiveTags, func(itemOS []string) bool {
		return manifest.MatchesOS(itemOS, "darwin") || manifest.MatchesOS(itemOS, "linux")
	})
}

func evaluate(m manifest.Manifest, effectiveTags []string, matchesOS func([]string) bool) Surface {
	tags := uniqueTags(effectiveTags)
	result := Surface{
		Tags:              tags,
		DependencySets:    []manifest.DependencySet{},
		Entries:           []SelectedEntry{},
		Dependencies:      []manifest.Dependency{},
		DependencyOrigins: []DependencyOrigin{},
		Provisioners:      []manifest.Provisioner{},
		SourceOverrides:   []SourceOverride{},
	}

	seenSets := make([]manifest.DependencySet, 0)
	seenEntries := make([]manifest.Entry, 0)
	seenProvisioners := make([]manifest.Provisioner, 0)
	seenOverrides := make([]SourceOverride, 0)
	seenDependencies := map[string]int{}

	addDependencies := func(dependencies []manifest.Dependency, origin Origin) {
		for _, dependency := range dependencies {
			dependency.Name = strings.TrimSpace(dependency.Name)
			result.DependencyOrigins = append(result.DependencyOrigins, DependencyOrigin{
				Dependency: cloneDependency(dependency),
				Origin:     cloneOrigin(origin),
			})
			if index, ok := seenDependencies[dependency.Name]; ok {
				if dependency.IsRequired() && !result.Dependencies[index].IsRequired() {
					result.Dependencies[index].Requirement = manifest.DependencyRequirementRequired
				}
				continue
			}
			seenDependencies[dependency.Name] = len(result.Dependencies)
			result.Dependencies = append(result.Dependencies, cloneDependency(dependency))
		}
	}

	for _, set := range m.Dependencies {
		if !manifest.SharesTag(set.Tags, tags) || !matchesOS(set.OS) || containsExact(seenSets, set) {
			continue
		}
		seenSets = append(seenSets, set)
		selected := cloneDependencySet(set)
		result.DependencySets = append(result.DependencySets, selected)
		addDependencies(set.Dependencies, Origin{Type: "dependency_set", Tags: cloneStrings(set.Tags)})
	}

	for _, entry := range m.Entries {
		if matchesOS(entry.OS) {
			for _, tag := range tags {
				source, ok := entry.SourceOverrides[tag]
				if !ok {
					continue
				}
				override := SourceOverride{Entry: cloneEntry(entry), Tag: tag, Source: source}
				if !containsExact(seenOverrides, override) {
					seenOverrides = append(seenOverrides, override)
					result.SourceOverrides = append(result.SourceOverrides, override)
				}
			}
		}
		if !manifest.SharesTag(entry.Tags, tags) || !matchesOS(entry.OS) || containsExact(seenEntries, entry) {
			continue
		}
		seenEntries = append(seenEntries, entry)
		source, overrideTag := entrySource(entry, tags)
		result.Entries = append(result.Entries, SelectedEntry{Entry: cloneEntry(entry), Source: source, OverrideTag: overrideTag})
		addDependencies(entry.Dependencies, Origin{Type: "entry", Name: entry.Target, Tags: cloneStrings(entry.Tags)})
	}

	for _, provisioner := range m.Provisioners {
		if !manifest.SharesTag(provisioner.Tags, tags) || !matchesOS(provisioner.OS) || containsExact(seenProvisioners, provisioner) {
			continue
		}
		seenProvisioners = append(seenProvisioners, provisioner)
		result.Provisioners = append(result.Provisioners, cloneProvisioner(provisioner))
		addDependencies(provisioner.Dependencies, Origin{Type: "provisioner", Name: provisioner.Tool, Tags: cloneStrings(provisioner.Tags)})
	}

	return result
}

func entrySource(entry manifest.Entry, tags []string) (string, string) {
	for index := len(tags) - 1; index >= 0; index-- {
		if source, ok := entry.SourceOverrides[tags[index]]; ok {
			return source, tags[index]
		}
	}
	return entry.Source, ""
}

func uniqueTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if seen[tag] {
			continue
		}
		seen[tag] = true
		result = append(result, tag)
	}
	return result
}

func containsExact[T any](values []T, candidate T) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, candidate) {
			return true
		}
	}
	return false
}

func cloneStrings(values []string) []string { return append([]string(nil), values...) }

func cloneDependency(dependency manifest.Dependency) manifest.Dependency {
	dependency.FontFallbackMatches = cloneStrings(dependency.FontFallbackMatches)
	dependency.Commands = cloneStrings(dependency.Commands)
	if dependency.UserLocal != nil {
		userLocal := *dependency.UserLocal
		if userLocal.Checksums != nil {
			userLocal.Checksums = make(map[string]string, len(userLocal.Checksums))
			for key, value := range dependency.UserLocal.Checksums {
				userLocal.Checksums[key] = value
			}
		}
		dependency.UserLocal = &userLocal
	}
	if dependency.RollingUserLocal != nil {
		rollingUserLocal := *dependency.RollingUserLocal
		dependency.RollingUserLocal = &rollingUserLocal
	}
	return dependency
}

func cloneDependencySet(set manifest.DependencySet) manifest.DependencySet {
	set.Tags = cloneStrings(set.Tags)
	set.OS = cloneStrings(set.OS)
	set.Dependencies = cloneDependencies(set.Dependencies)
	return set
}

func cloneEntry(entry manifest.Entry) manifest.Entry {
	entry.Tags = cloneStrings(entry.Tags)
	entry.OS = cloneStrings(entry.OS)
	entry.Dependencies = cloneDependencies(entry.Dependencies)
	if entry.SourceOverrides != nil {
		entry.SourceOverrides = make(map[string]string, len(entry.SourceOverrides))
		for tag, source := range entry.SourceOverrides {
			entry.SourceOverrides[tag] = source
		}
	}
	return entry
}

func cloneProvisioner(provisioner manifest.Provisioner) manifest.Provisioner {
	provisioner.Tags = cloneStrings(provisioner.Tags)
	provisioner.OS = cloneStrings(provisioner.OS)
	provisioner.Dependencies = cloneDependencies(provisioner.Dependencies)
	provisioner.Spec.Agents = cloneStrings(provisioner.Spec.Agents)
	provisioner.Spec.Skills = cloneStrings(provisioner.Spec.Skills)
	provisioner.Spec.Command = cloneStrings(provisioner.Spec.Command)
	if provisioner.Spec.Env != nil {
		environment := provisioner.Spec.Env
		provisioner.Spec.Env = make(map[string]string, len(environment))
		for key, value := range environment {
			provisioner.Spec.Env[key] = value
		}
	}
	return provisioner
}

func cloneDependencies(dependencies []manifest.Dependency) []manifest.Dependency {
	result := make([]manifest.Dependency, len(dependencies))
	for index, dependency := range dependencies {
		result[index] = cloneDependency(dependency)
	}
	return result
}

func cloneOrigin(origin Origin) Origin {
	origin.Tags = cloneStrings(origin.Tags)
	return origin
}
