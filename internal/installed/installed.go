// Package installed builds a read-only inventory from Installation Metadata.
package installed

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selectedsurface"
	"github.com/yersonargotev/dots/internal/state"
)

// CoverageState describes how much of a Profile appears represented by the
// current Installation Metadata.
type CoverageState string

const (
	CoverageComplete CoverageState = "complete"
	CoveragePartial  CoverageState = "partial"
)

// MetadataSummary identifies the state file used to build the report.
type MetadataSummary struct {
	Path    string `json:"path"`
	Version int    `json:"version"`
}

// ManagedEntry is one Managed Entry recorded in Installation Metadata.
type ManagedEntry struct {
	Source          string   `json:"source"`
	Target          string   `json:"target"`
	Strategy        string   `json:"strategy"`
	Hash            string   `json:"hash,omitempty"`
	InstalledAt     string   `json:"installed_at,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	TagsSource      string   `json:"tags_source"`
	Profiles        []string `json:"profiles,omitempty"`
	ProfilesSource  string   `json:"profiles_source"`
	ManifestMatched bool     `json:"manifest_matched"`
}

// ProfileCoverage summarizes whether a Profile is explicit in metadata or can be
// inferred from represented Tags and how complete its current-OS coverage is.
type ProfileCoverage struct {
	Name                    string        `json:"name"`
	Source                  string        `json:"source"`
	State                   CoverageState `json:"state"`
	CoveredTags             []string      `json:"covered_tags,omitempty"`
	MissingTags             []string      `json:"missing_tags,omitempty"`
	CoveredEntries          int           `json:"covered_entries"`
	TotalEntries            int           `json:"total_entries"`
	RecordedProvisioners    int           `json:"recorded_provisioners"`
	ProvisionedProvisioners int           `json:"provisioned_provisioners"`
	TotalProvisioners       int           `json:"total_provisioners"`
}

// ProvisionerRun is one Provisioner outcome recorded in Installation Metadata.
type ProvisionerRun struct {
	Tool            string   `json:"tool"`
	Executable      string   `json:"executable"`
	Args            []string `json:"args"`
	Status          string   `json:"status,omitempty"`
	Missing         []string `json:"missing,omitempty"`
	LastRunAt       string   `json:"last_run_at,omitempty"`
	Profile         string   `json:"profile,omitempty"`
	Profiles        []string `json:"profiles,omitempty"`
	ProfilesSource  string   `json:"profiles_source"`
	Tags            []string `json:"tags,omitempty"`
	TagsSource      string   `json:"tags_source"`
	ManifestMatched bool     `json:"manifest_matched"`
}

// Report is the official installed inventory. It is a read-only informational
// report, so HasFindings always returns false.
type Report struct {
	Metadata           MetadataSummary           `json:"metadata"`
	Provenance         state.Provenance          `json:"provenance,omitempty"`
	InstalledSelection *state.InstalledSelection `json:"installed_selection,omitempty"`
	ManagedEntries     []ManagedEntry            `json:"managed_entries"`
	Tags               []string                  `json:"tags"`
	Profiles           []ProfileCoverage         `json:"profiles"`
	Provisioners       []ProvisionerRun          `json:"provisioners"`
	Notes              []string                  `json:"notes,omitempty"`
}

func (r Report) HasFindings() bool { return false }

type Options struct {
	StatePath    string
	SourceRoot   string
	Home         string
	XDGStateHome string
	OS           string
}

// Build joins Installation Metadata with the current Install Manifest to infer
// represented Tags and Profile coverage without touching managed targets.
func Build(m manifest.Manifest, meta state.Metadata, opts Options) (Report, error) {
	report := Report{
		Metadata:           MetadataSummary{Path: opts.StatePath, Version: meta.Version},
		Provenance:         meta.Provenance,
		InstalledSelection: meta.InstalledSelection,
		ManagedEntries:     []ManagedEntry{},
		Tags:               []string{},
		Profiles:           []ProfileCoverage{},
		Provisioners:       []ProvisionerRun{},
	}

	matchedEntryKeys := map[string]bool{}
	representedTags := orderedSet{}
	explicitProfiles := orderedSet{}
	legacyTagsInferred := false
	legacyProfilesInferred := false
	unmatchedEntries := false

	for _, rec := range expandedRecords(meta.Entries) {
		entry, matched, err := matchEntry(m, rec, opts)
		if err != nil {
			return Report{}, err
		}
		if matched {
			matchedEntryKeys[entryKey(entry, opts)] = true
		}

		tags := append([]string(nil), rec.Tags...)
		tagsSource := "recorded"
		if len(tags) == 0 && matched {
			tags = append([]string(nil), entry.Tags...)
			tagsSource = "inferred-from-manifest"
			legacyTagsInferred = true
		} else if len(tags) == 0 {
			tagsSource = "unknown"
			unmatchedEntries = true
		}
		for _, tag := range tags {
			representedTags.add(tag)
		}

		profiles := normalizeProfiles(rec.Profiles, "")
		profilesSource := "recorded"
		if len(profiles) == 0 {
			profiles = normalizeProfiles(nil, "")
			profilesSource = "unknown"
			legacyProfilesInferred = true
		}
		for _, profile := range profiles {
			explicitProfiles.add(profile)
		}

		report.ManagedEntries = append(report.ManagedEntries, ManagedEntry{
			Source:          rec.Source,
			Target:          rec.Target,
			Strategy:        rec.Strategy,
			Hash:            rec.Hash,
			InstalledAt:     rec.InstalledAt,
			Tags:            tags,
			TagsSource:      tagsSource,
			Profiles:        profiles,
			ProfilesSource:  profilesSource,
			ManifestMatched: matched,
		})
	}

	provMatches, err := provisionerMatches(m, opts.OS)
	if err != nil {
		return Report{}, err
	}
	recordedProvisioners := map[string]state.ProvisionerRecord{}
	for _, rec := range meta.Provisioners {
		key := provisionerKey(rec.Tool, rec.Executable, rec.Args)
		recordedProvisioners[key] = rec
		matched := provMatches[key]
		tags := append([]string(nil), rec.Tags...)
		tagsSource := "recorded"
		if len(tags) == 0 && len(matched.tags) > 0 {
			tags = append([]string(nil), matched.tags...)
			tagsSource = "inferred-from-manifest"
			legacyTagsInferred = true
		} else if len(tags) == 0 {
			tagsSource = "unknown"
		}
		for _, tag := range tags {
			representedTags.add(tag)
		}

		profiles := normalizeProfiles(rec.Profiles, rec.Profile)
		profilesSource := "recorded"
		if len(profiles) == 0 {
			profilesSource = "unknown"
			legacyProfilesInferred = true
		}
		for _, profile := range profiles {
			explicitProfiles.add(profile)
		}

		report.Provisioners = append(report.Provisioners, ProvisionerRun{
			Tool:            rec.Tool,
			Executable:      rec.Executable,
			Args:            append([]string(nil), rec.Args...),
			Status:          rec.Status,
			Missing:         append([]string(nil), rec.Missing...),
			LastRunAt:       rec.LastRunAt,
			Profile:         rec.Profile,
			Profiles:        profiles,
			ProfilesSource:  profilesSource,
			Tags:            tags,
			TagsSource:      tagsSource,
			ManifestMatched: len(matched.tags) > 0,
		})
	}

	report.Tags = representedTags.values()
	report.Profiles = profileCoverage(m, opts, representedTags.set(), explicitProfiles.set(), matchedEntryKeys, recordedProvisioners)
	if legacyTagsInferred {
		report.Notes = append(report.Notes, "some metadata predates recorded tags; tags were inferred from the current manifest where possible")
	}
	if legacyProfilesInferred {
		report.Notes = append(report.Notes, "some metadata predates recorded profiles; profile coverage may be inferred from represented tags")
	}
	if unmatchedEntries {
		report.Notes = append(report.Notes, "some installed entries no longer match the current manifest; their tags and profile coverage are unknown")
	}
	if meta.Provenance.Empty() {
		report.Notes = append(report.Notes, "metadata does not record Source of Truth provenance; run a future install/update to capture source revision and dots version")
	}
	return report, nil
}

func expandedRecords(records []state.Record) []state.Record {
	expanded := make([]state.Record, 0, len(records))
	for _, rec := range records {
		sources := rec.SourceList()
		if len(sources) == 0 {
			expanded = append(expanded, rec)
			continue
		}
		for _, source := range sources {
			contributor := rec
			contributor.Source = source
			contributor.Sources = nil
			expanded = append(expanded, contributor)
		}
	}
	return expanded
}

func matchEntry(m manifest.Manifest, rec state.Record, opts Options) (manifest.Entry, bool, error) {
	for _, entry := range m.Entries {
		if !manifest.MatchesOS(entry.OS, opts.OS) {
			continue
		}
		target, err := plan.ResolveEntryTarget(entry, opts.Home, opts.XDGStateHome)
		if err != nil {
			return manifest.Entry{}, false, err
		}
		if filepath.Clean(target) != filepath.Clean(rec.Target) || entry.Strategy != rec.Strategy {
			continue
		}
		if compatibleEntrySource(entry, rec.Source) {
			return entry, true, nil
		}
	}
	return manifest.Entry{}, false, nil
}

func compatibleEntrySource(entry manifest.Entry, source string) bool {
	if source == entry.Source {
		return true
	}
	for _, override := range entry.SourceOverrides {
		if source == override {
			return true
		}
	}
	return false
}

func entryKey(entry manifest.Entry, opts Options) string {
	target, err := plan.ResolveEntryTarget(entry, opts.Home, opts.XDGStateHome)
	if err != nil {
		return entry.Target + "\x00" + entry.Strategy
	}
	return filepath.Clean(target) + "\x00" + entry.Strategy
}

type provisionerMatch struct{ tags []string }

func provisionerMatches(m manifest.Manifest, osName string) (map[string]provisionerMatch, error) {
	matches := map[string]provisionerMatch{}
	for _, prov := range m.Provisioners {
		if !manifest.MatchesOS(prov.OS, osName) {
			continue
		}
		executable, args := provision.RenderCommand(prov)
		key := provisionerKey(prov.Tool, executable, args)
		match := matches[key]
		match.tags = appendUnique(match.tags, prov.Tags...)
		matches[key] = match
	}
	return matches, nil
}

func profileCoverage(m manifest.Manifest, opts Options, representedTags map[string]bool, explicitProfiles map[string]bool, matchedEntryKeys map[string]bool, recordedProvisioners map[string]state.ProvisionerRecord) []ProfileCoverage {
	profileNames := make([]string, 0, len(m.Profiles))
	for name := range m.Profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)

	out := []ProfileCoverage{}
	for _, name := range profileNames {
		profile := m.Profiles[name]
		surface := selectedsurface.Evaluate(m, profile.Tags, opts.OS)
		coveredTags, missingTags := splitTags(profile.Tags, representedTags)
		if len(coveredTags) == 0 && !explicitProfiles[name] {
			continue
		}

		coverage := ProfileCoverage{Name: name, CoveredTags: coveredTags, MissingTags: missingTags}
		coverage.Source = "inferred"
		if explicitProfiles[name] && len(coveredTags) > 0 {
			coverage.Source = "recorded+inferred"
		} else if explicitProfiles[name] {
			coverage.Source = "recorded"
		}

		coverage.TotalEntries, coverage.CoveredEntries = entryCoverageForProfile(surface.Entries, opts, matchedEntryKeys)
		coverage.TotalProvisioners, coverage.RecordedProvisioners, coverage.ProvisionedProvisioners = provisionerCoverageForProfile(surface.Provisioners, recordedProvisioners)
		coverage.State = CoverageComplete
		if len(missingTags) > 0 || coverage.CoveredEntries < coverage.TotalEntries || coverage.RecordedProvisioners < coverage.TotalProvisioners {
			coverage.State = CoveragePartial
		}
		out = append(out, coverage)
	}
	return out
}

func entryCoverageForProfile(entries []selectedsurface.SelectedEntry, opts Options, matchedEntryKeys map[string]bool) (int, int) {
	total, covered := 0, 0
	for _, selected := range entries {
		entry := selected.Entry
		total++
		if matchedEntryKeys[entryKey(entry, opts)] {
			covered++
		}
	}
	return total, covered
}

func provisionerCoverageForProfile(provisioners []manifest.Provisioner, records map[string]state.ProvisionerRecord) (int, int, int) {
	total, recorded, provisioned := 0, 0, 0
	seen := map[string]bool{}
	for _, prov := range provisioners {
		executable, args := provision.RenderCommand(prov)
		key := provisionerKey(prov.Tool, executable, args)
		if seen[key] {
			continue
		}
		seen[key] = true
		total++
		if rec, ok := records[key]; ok {
			recorded++
			if rec.Status == string(provision.RunStatusProvisioned) {
				provisioned++
			}
		}
	}
	return total, recorded, provisioned
}

func splitTags(profileTags []string, represented map[string]bool) ([]string, []string) {
	var covered, missing []string
	for _, tag := range profileTags {
		if represented[tag] {
			covered = append(covered, tag)
		} else {
			missing = append(missing, tag)
		}
	}
	return covered, missing
}

func provisionerKey(tool, executable string, args []string) string {
	return tool + "\x00" + executable + "\x00" + strings.Join(args, "\x00")
}

func normalizeProfiles(profiles []string, legacy string) []string {
	set := orderedSet{}
	for _, profile := range profiles {
		set.add(profile)
	}
	for _, profile := range strings.Split(legacy, ",") {
		set.add(profile)
	}
	return set.values()
}

func appendUnique(values []string, additions ...string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

type orderedSet struct {
	order []string
	seen  map[string]bool
}

func (s *orderedSet) add(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if s.seen[value] {
		return
	}
	s.seen[value] = true
	s.order = append(s.order, value)
}

func (s orderedSet) values() []string {
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}

func (s orderedSet) set() map[string]bool {
	out := map[string]bool{}
	for _, value := range s.order {
		out[value] = true
	}
	return out
}

func (r Report) String() string {
	return fmt.Sprintf("%d managed entries, %d tags, %d profiles, %d provisioners", len(r.ManagedEntries), len(r.Tags), len(r.Profiles), len(r.Provisioners))
}
