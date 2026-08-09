// Package catalog builds a portable, read-only description of an Install
// Manifest. It deliberately does not inspect a workstation or installed state.
package catalog

import (
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/tagpolicy"
)

const (
	MetadataDeclared = "declared"
	MetadataDerived  = "derived"
)

// Options controls a portable catalog view. OS is darwin, linux, or all; an
// empty value uses the host OS. IncludeLegacy only affects list views: explicit
// Profile and Tag queries always return their named item.
type Options struct {
	OS            string
	IncludeLegacy bool
}

// Report is the stable shared data model for text, JSON, Markdown, and future
// presentation adapters.
type Report struct {
	OS             string           `json:"os"`
	MetadataOrigin string           `json:"metadata_origin"`
	Profiles       []ProfileSummary `json:"profiles"`
	Tags           []TagSummary     `json:"tags"`
	Hidden         Hidden           `json:"hidden"`
	Profile        *Detail          `json:"profile,omitempty"`
	Tag            *Detail          `json:"tag,omitempty"`
	Comparison     *Comparison      `json:"comparison,omitempty"`
}

// Hidden records items deliberately omitted from a compact current-only list.
type Hidden struct {
	Profiles int `json:"profiles"`
	Tags     int `json:"tags"`
}

// ProfileSummary is a compact Profile summary.
type ProfileSummary struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
}

// TagSummary is a compact Tag summary.
type TagSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	ReplacedBy  string `json:"replaced_by,omitempty"`
	Origin      string `json:"origin"`
}

// Detail explains exactly one Profile or Tag selection without composing it
// with Installed Selection or reading machine state.
type Detail struct {
	Name                string            `json:"name"`
	Description         string            `json:"description"`
	Status              string            `json:"status"`
	Kind                string            `json:"kind,omitempty"`
	ReplacedBy          string            `json:"replaced_by,omitempty"`
	ResolvedTags        []string          `json:"resolved_tags"`
	Dependencies        []Dependency      `json:"dependencies"`
	DependencySets      []DependencySet   `json:"dependency_sets"`
	ProfileDependencies []Dependency      `json:"profile_dependencies"`
	Entries             []Entry           `json:"entries"`
	SourceOverrides     []SourceOverride  `json:"source_overrides"`
	Provisioners        []Provisioner     `json:"provisioners"`
	Behaviors           []Behavior        `json:"behaviors"`
	Excluded            []ExcludedSurface `json:"excluded"`
}

// Dependency identifies a declared dependency and why the selected surface
// includes it. It contains declaration data only; no probe result is present.
type Dependency struct {
	Name        string   `json:"name"`
	Requirement string   `json:"requirement"`
	Probes      []string `json:"probes"`
	Origin      Origin   `json:"origin"`
}

// Origin pinpoints the manifest surface that selected a dependency.
type Origin struct {
	Type string   `json:"type"`
	Name string   `json:"name,omitempty"`
	Tags []string `json:"tags,omitempty"`
}

type DependencySet struct {
	Tags         []string     `json:"tags"`
	OS           []string     `json:"os"`
	Dependencies []Dependency `json:"dependencies"`
}

// Entry describes one portable Managed Entry selected by the queried intent.
type Entry struct {
	Source       string       `json:"source"`
	Target       string       `json:"target"`
	TargetRoot   string       `json:"target_root,omitempty"`
	Strategy     string       `json:"strategy"`
	Ownership    string       `json:"ownership,omitempty"`
	Tags         []string     `json:"tags"`
	OS           []string     `json:"os"`
	Dependencies []Dependency `json:"dependencies"`
}

// SourceOverride records a source change activated by a selected Tag. It is
// retained separately when the override's base Entry is not itself selected.
type SourceOverride struct {
	Tag        string   `json:"tag"`
	Source     string   `json:"source"`
	Entry      string   `json:"entry"`
	Target     string   `json:"target"`
	OS         []string `json:"os"`
	Applicable bool     `json:"applicable"`
}

// Provisioner is a safe declarative description. EnvironmentValues are never
// represented; only declared variable names may appear in EnvironmentNames.
type Provisioner struct {
	Tool             string       `json:"tool"`
	Operation        string       `json:"operation"`
	Identity         string       `json:"identity,omitempty"`
	Scope            string       `json:"scope,omitempty"`
	Agents           []string     `json:"agents"`
	Skills           []string     `json:"skills"`
	Command          []string     `json:"command,omitempty"`
	EnvironmentNames []string     `json:"environment_names"`
	Tags             []string     `json:"tags"`
	OS               []string     `json:"os"`
	Dependencies     []Dependency `json:"dependencies"`
}

type Behavior struct {
	Action      string `json:"action"`
	Description string `json:"description"`
}

type ExcludedSurface struct {
	Type   string   `json:"type"`
	Name   string   `json:"name"`
	OS     []string `json:"os"`
	Reason string   `json:"reason"`
}

// Comparison describes the portable surface delta when moving from one
// Profile to another. Added belongs only to To, Removed belongs only to From,
// and Shared reports counts without repeating the common surface.
type Comparison struct {
	From    string            `json:"from"`
	To      string            `json:"to"`
	Added   ComparisonSurface `json:"added"`
	Removed ComparisonSurface `json:"removed"`
	Shared  ComparisonCounts  `json:"shared"`
}

// ComparisonSurface groups the declarative catalog items unique to one side
// of a Profile comparison.
type ComparisonSurface struct {
	ResolvedTags    []string         `json:"resolved_tags"`
	Dependencies    []Dependency     `json:"dependencies"`
	Entries         []Entry          `json:"entries"`
	SourceOverrides []SourceOverride `json:"source_overrides"`
	Provisioners    []Provisioner    `json:"provisioners"`
	Behaviors       []Behavior       `json:"behaviors"`
}

// ComparisonCounts summarizes common items between two Profiles.
type ComparisonCounts struct {
	ResolvedTags    int `json:"resolved_tags"`
	Dependencies    int `json:"dependencies"`
	Entries         int `json:"entries"`
	SourceOverrides int `json:"source_overrides"`
	Provisioners    int `json:"provisioners"`
	Behaviors       int `json:"behaviors"`
}

// Build returns deterministic Profile and Tag summaries. The manifest must
// already have passed manifest validation.
func Build(m manifest.Manifest, opts Options) (Report, error) {
	osName, err := resolveOS(opts.OS)
	if err != nil {
		return Report{}, err
	}
	r := Report{OS: osName, MetadataOrigin: metadataOrigin(m), Profiles: []ProfileSummary{}, Tags: []TagSummary{}}
	for _, name := range sortedProfileNames(m) {
		profile := summaryProfile(name, m.Profiles[name])
		if profile.Status == "legacy" && !opts.IncludeLegacy {
			r.Hidden.Profiles++
			continue
		}
		r.Profiles = append(r.Profiles, profile)
	}
	for _, tag := range allTags(m) {
		summary := summaryTag(m, tag)
		if summary.Status == "legacy" && !opts.IncludeLegacy {
			r.Hidden.Tags++
			continue
		}
		r.Tags = append(r.Tags, summary)
	}
	return r, nil
}

// Profile returns a detailed, exact Profile view. Legacy details are always
// available so prior declared intent remains inspectable.
func Profile(m manifest.Manifest, name string, opts Options) (Report, error) {
	base, err := Build(m, Options{OS: opts.OS, IncludeLegacy: true})
	if err != nil {
		return Report{}, err
	}
	p, ok := m.Profiles[name]
	if !ok {
		return Report{}, fmt.Errorf("profile %q not found", name)
	}
	summary := summaryProfile(name, p)
	base.Profile = buildDetail(m, name, summary.Description, summary.Status, "", "", unique(p.Tags), base.OS, p.Dependencies)
	return base, nil
}

// CompareProfiles returns the declarative surface delta from one Profile to
// another. It remains independent from Installed Selection and machine state.
func CompareProfiles(m manifest.Manifest, from, to string, opts Options) (Report, error) {
	fromReport, err := Profile(m, from, opts)
	if err != nil {
		return Report{}, err
	}
	toReport, err := Profile(m, to, opts)
	if err != nil {
		return Report{}, err
	}

	fromSurface := comparisonSurface(fromReport.Profile)
	toSurface := comparisonSurface(toReport.Profile)
	fromReport.Comparison = &Comparison{
		From:    from,
		To:      to,
		Added:   difference(toSurface, fromSurface),
		Removed: difference(fromSurface, toSurface),
		Shared:  sharedCounts(fromSurface, toSurface),
	}
	fromReport.Profile = nil
	return fromReport, nil
}

// Tag returns a detailed, exact Tag view. Registryless manifests derive a
// current surface Tag mechanically from all manifest references.
func Tag(m manifest.Manifest, name string, opts Options) (Report, error) {
	base, err := Build(m, Options{OS: opts.OS, IncludeLegacy: true})
	if err != nil {
		return Report{}, err
	}
	if !contains(allTags(m), name) {
		return Report{}, fmt.Errorf("tag %q not found", name)
	}
	t := summaryTag(m, name)
	base.Tag = buildDetail(m, name, t.Description, t.Status, t.Kind, t.ReplacedBy, []string{name}, base.OS, nil)
	return base, nil
}

func buildDetail(m manifest.Manifest, name, description, status, kind, replacedBy string, tags []string, osName string, profileDeps []manifest.Dependency) *Detail {
	d := &Detail{Name: name, Description: description, Status: status, Kind: kind, ReplacedBy: replacedBy, ResolvedTags: clone(tags), Dependencies: []Dependency{}, DependencySets: []DependencySet{}, ProfileDependencies: []Dependency{}, Entries: []Entry{}, SourceOverrides: []SourceOverride{}, Provisioners: []Provisioner{}, Behaviors: behaviors(tags), Excluded: []ExcludedSurface{}}
	for _, dep := range profileDeps {
		d.ProfileDependencies = append(d.ProfileDependencies, dependency(dep, Origin{Type: "profile", Name: name}))
	}
	for _, set := range m.Dependencies {
		if !manifest.SharesTag(set.Tags, tags) {
			continue
		}
		if !matches(set.OS, osName) {
			d.Excluded = append(d.Excluded, excluded("dependency_set", strings.Join(set.Tags, ","), set.OS, osName))
			continue
		}
		item := DependencySet{Tags: clone(set.Tags), OS: declaredOS(set.OS), Dependencies: []Dependency{}}
		for _, dep := range set.Dependencies {
			item.Dependencies = append(item.Dependencies, dependency(dep, Origin{Type: "dependency_set", Tags: clone(set.Tags)}))
			d.Dependencies = append(d.Dependencies, dependency(dep, Origin{Type: "dependency_set", Tags: clone(set.Tags)}))
		}
		d.DependencySets = append(d.DependencySets, item)
	}
	for _, entry := range m.Entries {
		for tag, source := range entry.SourceOverrides {
			if contains(tags, tag) {
				applicable := matches(entry.OS, osName)
				d.SourceOverrides = append(d.SourceOverrides, SourceOverride{Tag: tag, Source: source, Entry: entry.Source, Target: entry.Target, OS: declaredOS(entry.OS), Applicable: applicable})
				if !applicable {
					d.Excluded = append(d.Excluded, excluded("source_override", entry.Target+" ("+tag+")", entry.OS, osName))
				}
			}
		}
		if !manifest.SharesTag(entry.Tags, tags) {
			continue
		}
		if !matches(entry.OS, osName) {
			d.Excluded = append(d.Excluded, excluded("entry", entry.Target, entry.OS, osName))
			continue
		}
		selected := manifest.EntrySource(entry, tags)
		item := Entry{Source: selected, Target: entry.Target, TargetRoot: entry.TargetRoot, Strategy: entry.Strategy, Ownership: entry.Ownership, Tags: clone(entry.Tags), OS: declaredOS(entry.OS), Dependencies: []Dependency{}}
		for _, dep := range entry.Dependencies {
			x := dependency(dep, Origin{Type: "entry", Name: entry.Target, Tags: clone(entry.Tags)})
			item.Dependencies = append(item.Dependencies, x)
			d.Dependencies = append(d.Dependencies, x)
		}
		d.Entries = append(d.Entries, item)
	}
	for _, p := range m.Provisioners {
		if !manifest.SharesTag(p.Tags, tags) {
			continue
		}
		if !matches(p.OS, osName) {
			d.Excluded = append(d.Excluded, excluded("provisioner", p.Tool, p.OS, osName))
			continue
		}
		d.Provisioners = append(d.Provisioners, provisioner(p))
		for _, dep := range p.Dependencies {
			d.Dependencies = append(d.Dependencies, dependency(dep, Origin{Type: "provisioner", Name: p.Tool, Tags: clone(p.Tags)}))
		}
	}
	sort.Slice(d.SourceOverrides, func(i, j int) bool {
		if d.SourceOverrides[i].Tag == d.SourceOverrides[j].Tag {
			return d.SourceOverrides[i].Target < d.SourceOverrides[j].Target
		}
		return d.SourceOverrides[i].Tag < d.SourceOverrides[j].Tag
	})
	sort.Slice(d.Excluded, func(i, j int) bool {
		if d.Excluded[i].Type == d.Excluded[j].Type {
			return d.Excluded[i].Name < d.Excluded[j].Name
		}
		return d.Excluded[i].Type < d.Excluded[j].Type
	})
	return d
}

func comparisonSurface(detail *Detail) ComparisonSurface {
	if detail == nil {
		return ComparisonSurface{ResolvedTags: []string{}, Dependencies: []Dependency{}, Entries: []Entry{}, SourceOverrides: []SourceOverride{}, Provisioners: []Provisioner{}, Behaviors: []Behavior{}}
	}
	dependencies := make([]Dependency, 0, len(detail.ProfileDependencies)+len(detail.Dependencies))
	dependencies = append(dependencies, detail.ProfileDependencies...)
	dependencies = append(dependencies, detail.Dependencies...)
	return ComparisonSurface{
		ResolvedTags:    clone(detail.ResolvedTags),
		Dependencies:    deduplicate(dependencies, dependencyKey),
		Entries:         append([]Entry{}, detail.Entries...),
		SourceOverrides: append([]SourceOverride{}, detail.SourceOverrides...),
		Provisioners:    append([]Provisioner{}, detail.Provisioners...),
		Behaviors:       append([]Behavior{}, detail.Behaviors...),
	}
}

func difference(wanted, other ComparisonSurface) ComparisonSurface {
	return ComparisonSurface{
		ResolvedTags:    sliceDifference(wanted.ResolvedTags, other.ResolvedTags, func(value string) string { return value }),
		Dependencies:    sliceDifference(wanted.Dependencies, other.Dependencies, dependencyKey),
		Entries:         sliceDifference(wanted.Entries, other.Entries, entryKey),
		SourceOverrides: sliceDifference(wanted.SourceOverrides, other.SourceOverrides, sourceOverrideKey),
		Provisioners:    sliceDifference(wanted.Provisioners, other.Provisioners, provisionerKey),
		Behaviors:       sliceDifference(wanted.Behaviors, other.Behaviors, behaviorKey),
	}
}

func sharedCounts(left, right ComparisonSurface) ComparisonCounts {
	return ComparisonCounts{
		ResolvedTags:    sharedCount(left.ResolvedTags, right.ResolvedTags, func(value string) string { return value }),
		Dependencies:    sharedCount(left.Dependencies, right.Dependencies, dependencyKey),
		Entries:         sharedCount(left.Entries, right.Entries, entryKey),
		SourceOverrides: sharedCount(left.SourceOverrides, right.SourceOverrides, sourceOverrideKey),
		Provisioners:    sharedCount(left.Provisioners, right.Provisioners, provisionerKey),
		Behaviors:       sharedCount(left.Behaviors, right.Behaviors, behaviorKey),
	}
}

func sliceDifference[T any](wanted, other []T, key func(T) string) []T {
	otherKeys := make(map[string]bool, len(other))
	for _, item := range other {
		otherKeys[key(item)] = true
	}
	result := []T{}
	for _, item := range wanted {
		if !otherKeys[key(item)] {
			result = append(result, item)
		}
	}
	return result
}

func deduplicate[T any](items []T, key func(T) string) []T {
	seen := make(map[string]bool, len(items))
	result := []T{}
	for _, item := range items {
		identity := key(item)
		if seen[identity] {
			continue
		}
		seen[identity] = true
		result = append(result, item)
	}
	return result
}

func sharedCount[T any](left, right []T, key func(T) string) int {
	return len(left) - len(sliceDifference(left, right, key))
}

func dependencyKey(value Dependency) string {
	return fmt.Sprintf("%s\x00%s\x00%v", value.Name, value.Requirement, value.Probes)
}

func entryKey(value Entry) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%v\x00%v\x00%v", value.Source, value.Target, value.TargetRoot, value.Strategy, value.Ownership, value.Tags, value.OS, value.Dependencies)
}

func sourceOverrideKey(value SourceOverride) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%v\x00%t", value.Tag, value.Source, value.Entry, value.Target, value.OS, value.Applicable)
}

func provisionerKey(value Provisioner) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%v\x00%v\x00%v\x00%v\x00%v\x00%v", value.Tool, value.Operation, value.Identity, value.Scope, value.Agents, value.Skills, value.Command, value.EnvironmentNames, value.Tags, value.OS)
}

func behaviorKey(value Behavior) string {
	return value.Action + "\x00" + value.Description
}

func provisioner(p manifest.Provisioner) Provisioner {
	s := p.Spec
	result := Provisioner{Tool: p.Tool, Scope: s.Scope, Agents: clone(s.Agents), Skills: clone(s.Skills), Tags: clone(p.Tags), OS: declaredOS(p.OS), Dependencies: []Dependency{}, EnvironmentNames: []string{}}
	switch {
	case s.Marketplace != "":
		result.Operation, result.Identity = "marketplace", s.Marketplace
	case s.Plugin != "":
		result.Operation, result.Identity = "plugin", s.Plugin+"@"+s.From
	case s.MCP != "":
		result.Operation, result.Identity, result.Command = "mcp", s.MCP, clone(s.Command)
	case s.Package != "":
		result.Operation, result.Identity = "skills", s.Package
	case p.Tool == "codegraph":
		result.Operation = "install"
	case p.Tool == "zimfw":
		result.Operation = "initialize"
	default:
		result.Operation = "declared"
	}
	for key := range s.Env {
		result.EnvironmentNames = append(result.EnvironmentNames, key)
	}
	sort.Strings(result.EnvironmentNames)
	for _, dep := range p.Dependencies {
		result.Dependencies = append(result.Dependencies, dependency(dep, Origin{Type: "provisioner", Name: p.Tool, Tags: clone(p.Tags)}))
	}
	return result
}

func dependency(dep manifest.Dependency, origin Origin) Dependency {
	return Dependency{Name: dep.Name, Requirement: dep.RequirementValue(), Probes: dep.Probes(), Origin: origin}
}

func behaviors(tags []string) []Behavior {
	result := []Behavior{}
	for _, action := range tagpolicy.Actions(tags) {
		result = append(result, Behavior{Action: string(action), Description: behaviorDescription(action)})
	}
	return result
}

func behaviorDescription(action tagpolicy.Action) string {
	switch action {
	case tagpolicy.ActionRetireGentleAIState:
		return "Retire dots-owned Gentle AI state."
	default:
		return "Allowlisted selection behavior."
	}
}

func summaryProfile(name string, p manifest.Profile) ProfileSummary {
	status := p.Status
	if status == "" {
		status = "current"
	}
	return ProfileSummary{Name: name, Description: p.Description, Status: status, Tags: clone(p.Tags)}
}
func summaryTag(m manifest.Manifest, name string) TagSummary {
	if t, ok := m.Tags[name]; ok {
		return TagSummary{Name: name, Description: t.Description, Kind: t.Kind, Status: t.Status, ReplacedBy: t.ReplacedBy, Origin: MetadataDeclared}
	}
	return TagSummary{Name: name, Kind: "surface", Status: "current", Origin: MetadataDerived}
}
func metadataOrigin(m manifest.Manifest) string {
	if len(m.Tags) == 0 {
		return MetadataDerived
	}
	return MetadataDeclared
}

func allTags(m manifest.Manifest) []string {
	seen := map[string]bool{}
	add := func(tags []string) {
		for _, tag := range tags {
			seen[tag] = true
		}
	}
	for name := range m.Tags {
		seen[name] = true
	}
	for _, p := range m.Profiles {
		add(p.Tags)
	}
	for _, s := range m.Dependencies {
		add(s.Tags)
	}
	for _, e := range m.Entries {
		add(e.Tags)
		for tag := range e.SourceOverrides {
			seen[tag] = true
		}
	}
	for _, p := range m.Provisioners {
		add(p.Tags)
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func sortedProfileNames(m manifest.Manifest) []string {
	names := make([]string, 0, len(m.Profiles))
	for name := range m.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func resolveOS(value string) (string, error) {
	if value == "" {
		value = runtime.GOOS
	}
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "darwin", "linux", "all":
		return value, nil
	default:
		return "", fmt.Errorf("catalog OS %q is invalid: choose darwin, linux, or all", value)
	}
}
func matches(os []string, value string) bool { return value == "all" || manifest.MatchesOS(os, value) }
func declaredOS(os []string) []string {
	if len(os) == 0 {
		return []string{"darwin", "linux"}
	}
	return clone(os)
}
func excluded(kind, name string, os []string, requested string) ExcludedSurface {
	return ExcludedSurface{Type: kind, Name: name, OS: declaredOS(os), Reason: "not applicable to " + requested}
}
func clone(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string{}, values...)
}
func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
func unique(values []string) []string {
	result := []string{}
	for _, value := range values {
		if !contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}
