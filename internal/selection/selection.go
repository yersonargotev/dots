// Package selection resolves and persists authoritative Installed Selection.
package selection

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/tagpolicy"
)

// Source identifies where a selection-aware command obtained its intent.
type Source string

const (
	SourceExplicit  Source = "explicit"
	SourceRecorded  Source = "recorded"
	SourceMigration Source = "migration"
)

// ErrSelectionRequired means that neither invocation arguments nor Installation
// Metadata supplied a selection. Selection-aware commands must not silently use a
// manifest's default Profile in this case.
var ErrSelectionRequired = errors.New("selection required: provide --profile or --tag, or run dots install to record an Installed Selection")

// Report is the stable, portable description of the selection used by a
// command.
type Report struct {
	Source        Source   `json:"source"`
	Profiles      []string `json:"profiles"`
	ExtraTags     []string `json:"extra_tags"`
	EffectiveTags []string `json:"effective_tags"`
	Delta         *Delta   `json:"delta,omitempty"`
	Change        *Change  `json:"change,omitempty"`
}

// Snapshot is the portable selection state on one side of an evolution.
type Snapshot struct {
	Profiles      []string `json:"profiles"`
	ExtraTags     []string `json:"extra_tags"`
	EffectiveTags []string `json:"effective_tags"`
}

// Changes describes an ordered change to the selected manifest surface.
type Changes struct {
	Profiles       []string `json:"profiles"`
	ExtraTags      []string `json:"extra_tags"`
	EffectiveTags  []string `json:"effective_tags"`
	ManagedEntries []string `json:"managed_entries"`
	Dependencies   []string `json:"dependencies"`
	Provisioners   []string `json:"provisioners"`
}

// Delta describes how the same selection intent evolves between Install
// Manifest revisions.
type Delta struct {
	Previous        Snapshot `json:"previous"`
	Current         Snapshot `json:"current"`
	Added           Changes  `json:"added"`
	Removed         Changes  `json:"removed"`
	MissingProfiles []string `json:"missing_profiles"`
	StaleExtraTags  []string `json:"stale_extra_tags"`
}

// Change describes an explicit replacement of the authoritative Installed
// Selection. Acknowledgement state is intentionally separate from manifest
// evolution semantics.
type Change struct {
	Delta                   Delta `json:"delta"`
	AcknowledgementRequired bool  `json:"acknowledgement_required"`
	AcknowledgementAccepted bool  `json:"acknowledgement_accepted"`
}

// EvolutionError is an actionable, machine-readable blocking validation error.
type EvolutionError struct {
	Message        string `json:"message"`
	SelectionDelta Delta  `json:"selection_delta"`
}

func (e *EvolutionError) Error() string { return e.Message }

// JSONErrorData supplies the stable machine-readable payload used by the CLI
// error envelope.
func (e *EvolutionError) JSONErrorData() any {
	return struct {
		SelectionDelta Delta `json:"selection_delta"`
	}{SelectionDelta: e.SelectionDelta}
}

// Effective contains the validated inputs for downstream command builders and
// their corresponding report.
type Effective struct {
	Profiles  []string
	ExtraTags []string
	Selection manifest.Selection
	Report    Report
}

// Intent is the authoritative Profile and explicit extra Tag input together
// with its provenance. It is safe to carry across process boundaries because it
// excludes the resolved Tag snapshot, which is audit data rather than intent.
type Intent struct {
	Source    Source
	Profiles  []string
	ExtraTags []string
}

// ResolveEffective chooses and validates the selection for a command. Any
// explicit Profile or Tag wins over the recorded Installed Selection.
func ResolveEffective(m manifest.Manifest, explicitProfiles, explicitTags []string, recorded *state.InstalledSelection) (Effective, error) {
	intent := Intent{Source: SourceExplicit, Profiles: explicitProfiles, ExtraTags: explicitTags}
	if len(explicitProfiles) == 0 && len(explicitTags) == 0 {
		if recorded == nil {
			return Effective{}, ErrSelectionRequired
		}
		intent = Intent{Source: SourceRecorded, Profiles: recorded.Profiles, ExtraTags: recorded.ExtraTags}
	}
	return ResolveIntent(m, intent)
}

// ResolveReadOnly chooses and validates the selection for a read-only command.
// Any explicit Profile or tag wins over the recorded Installed Selection.
func ResolveReadOnly(m manifest.Manifest, explicitProfiles, explicitTags []string, recorded *state.InstalledSelection) (Effective, error) {
	return ResolveEffective(m, explicitProfiles, explicitTags, recorded)
}

// ResolveIntent re-resolves authoritative intent against the current Install
// Manifest without changing whether its source was explicit, recorded, or an
// interactively confirmed migration candidate.
func ResolveIntent(m manifest.Manifest, intent Intent) (Effective, error) {
	if intent.Source != SourceExplicit && intent.Source != SourceRecorded && intent.Source != SourceMigration {
		return Effective{}, fmt.Errorf("selection source %q is invalid", intent.Source)
	}
	if err := validateIntent(intent.Source, intent.Profiles, intent.ExtraTags); err != nil {
		return Effective{}, err
	}

	resolved, err := manifest.ResolveReadOnlySelection(m, intent.Profiles, intent.ExtraTags)
	if err != nil {
		return Effective{}, fmt.Errorf("%s selection: %w", intent.Source, err)
	}

	orderedProfiles := cloneStrings(resolved.Profiles)
	orderedExtraTags := orderedUnique(intent.ExtraTags)
	effectiveTags := cloneStrings(resolved.Tags)
	return Effective{
		Profiles:  cloneStrings(orderedProfiles),
		ExtraTags: cloneStrings(orderedExtraTags),
		Selection: manifest.Selection{
			Profile:  resolved.Profile,
			Profiles: cloneStrings(orderedProfiles),
			Tags:     cloneStrings(effectiveTags),
		},
		Report: Report{
			Source:        intent.Source,
			Profiles:      orderedProfiles,
			ExtraTags:     orderedExtraTags,
			EffectiveTags: effectiveTags,
		},
	}, nil
}

// Intent returns a detached copy of the authoritative inputs used to build e.
func (e Effective) Intent() Intent {
	return Intent{
		Source:    e.Report.Source,
		Profiles:  cloneStrings(e.Profiles),
		ExtraTags: cloneStrings(e.ExtraTags),
	}
}

// CompareEvolution re-resolves the same authoritative intent against a newer
// Install Manifest and reports deterministic changes to its selected surface.
// It is pure: it performs no filesystem or state mutation.
func CompareEvolution(previousManifest, currentManifest manifest.Manifest, previous Effective, osName string) (Effective, error) {
	intent := previous.Intent()
	delta := emptyDelta(snapshot(previous), Snapshot{
		Profiles:      cloneStrings(intent.Profiles),
		ExtraTags:     cloneStrings(intent.ExtraTags),
		EffectiveTags: make([]string, 0),
	})
	delta.MissingProfiles = missingProfiles(currentManifest, intent.Profiles)
	delta.StaleExtraTags = staleExtraTags(currentManifest, intent.ExtraTags)
	if len(delta.MissingProfiles) == 0 {
		current, err := ResolveIntent(currentManifest, intent)
		if err != nil {
			return Effective{}, err
		}
		delta.Current = snapshot(current)
		previousSurface := selectedSurface(previousManifest, previous.Selection, osName)
		currentSurface := selectedSurface(currentManifest, current.Selection, osName)
		delta.Added = surfaceDifference(currentSurface, previousSurface)
		delta.Removed = surfaceDifference(previousSurface, currentSurface)
		if len(delta.StaleExtraTags) == 0 {
			current.Report.Delta = &delta
			current.Report.Change = previous.Report.Change
			return current, nil
		}
	}

	var problems []string
	for _, profile := range delta.MissingProfiles {
		problems = append(problems, fmt.Sprintf(`%s selection: profile %q not found`, intent.Source, profile))
	}
	for _, tag := range delta.StaleExtraTags {
		problems = append(problems, fmt.Sprintf(`%s selection: extra Tag %q is no longer declared`, intent.Source, tag))
	}
	return Effective{}, &EvolutionError{
		Message:        strings.Join(problems, "; ") + "; update the selection before refreshing",
		SelectionDelta: delta,
	}
}

// CompareInstalled compares an explicit effective request with the
// authoritative Installed Selection recorded for the same Install Manifest.
// The recorded resolved Tags are retained as the previous audit snapshot
// rather than being re-resolved.
func CompareInstalled(m manifest.Manifest, requested Effective, recorded *state.InstalledSelection, osName string) Effective {
	requested.Report.Change = nil
	if recorded == nil {
		return requested
	}
	if equalStrings(recorded.Profiles, requested.Profiles) &&
		equalStrings(recorded.ExtraTags, requested.ExtraTags) {
		return requested
	}

	previousSelection := manifest.Selection{
		Profiles: cloneStrings(recorded.Profiles),
		Tags:     cloneStrings(recorded.ResolvedTags),
	}
	previousSurface := selectedSurface(m, previousSelection, osName)
	currentSurface := selectedSurface(m, requested.Selection, osName)
	previous := surfaceSnapshot(recorded.Profiles, recorded.ExtraTags, previousSurface)
	current := surfaceSnapshot(requested.Profiles, requested.ExtraTags, currentSurface)
	delta := emptyDelta(previous, current)
	delta.Added = surfaceDifference(currentSurface, previousSurface)
	delta.Added.Profiles = difference(current.Profiles, previous.Profiles)
	delta.Added.ExtraTags = difference(current.ExtraTags, previous.ExtraTags)
	delta.Removed = surfaceDifference(previousSurface, currentSurface)
	delta.Removed.Profiles = difference(previous.Profiles, current.Profiles)
	delta.Removed.ExtraTags = difference(previous.ExtraTags, current.ExtraTags)
	if deltaIsEmpty(delta) {
		return requested
	}
	requested.Report.Change = &Change{
		Delta: delta,
		AcknowledgementRequired: len(delta.Removed.Profiles) > 0 ||
			len(delta.Removed.ExtraTags) > 0,
	}
	return requested
}

func snapshot(e Effective) Snapshot {
	return Snapshot{
		Profiles:      cloneStrings(e.Profiles),
		ExtraTags:     cloneStrings(e.ExtraTags),
		EffectiveTags: cloneStrings(e.Selection.Tags),
	}
}

func surfaceSnapshot(profiles, extraTags []string, surface Changes) Snapshot {
	return Snapshot{
		Profiles:      cloneStrings(profiles),
		ExtraTags:     cloneStrings(extraTags),
		EffectiveTags: cloneStrings(surface.EffectiveTags),
	}
}

func emptyDelta(previous, current Snapshot) Delta {
	return Delta{
		Previous: previous,
		Current:  current,
		Added: Changes{
			Profiles:       make([]string, 0),
			ExtraTags:      make([]string, 0),
			EffectiveTags:  make([]string, 0),
			ManagedEntries: make([]string, 0),
			Dependencies:   make([]string, 0),
			Provisioners:   make([]string, 0),
		},
		Removed: Changes{
			Profiles:       make([]string, 0),
			ExtraTags:      make([]string, 0),
			EffectiveTags:  make([]string, 0),
			ManagedEntries: make([]string, 0),
			Dependencies:   make([]string, 0),
			Provisioners:   make([]string, 0),
		},
		MissingProfiles: make([]string, 0),
		StaleExtraTags:  make([]string, 0),
	}
}

func missingProfiles(m manifest.Manifest, profiles []string) []string {
	var missing []string
	for _, name := range orderedUnique(profiles) {
		if _, ok := m.Profiles[name]; !ok {
			missing = append(missing, name)
		}
	}
	return cloneStrings(missing)
}

func staleExtraTags(m manifest.Manifest, extraTags []string) []string {
	if m.Tags != nil {
		var stale []string
		for _, tag := range orderedUnique(extraTags) {
			if _, declared := m.Tags[tag]; !declared && !tagpolicy.IsBehaviorTag(tag) {
				stale = append(stale, tag)
			}
		}
		return cloneStrings(stale)
	}

	declared := make(map[string]bool)
	for _, entry := range m.Entries {
		for _, tag := range entry.Tags {
			declared[tag] = true
		}
	}
	for _, set := range m.Dependencies {
		for _, tag := range set.Tags {
			declared[tag] = true
		}
	}
	for _, provisioner := range m.Provisioners {
		for _, tag := range provisioner.Tags {
			declared[tag] = true
		}
	}
	var stale []string
	for _, tag := range orderedUnique(extraTags) {
		if !declared[tag] && !tagpolicy.IsBehaviorTag(tag) {
			stale = append(stale, tag)
		}
	}
	return cloneStrings(stale)
}

func selectedSurface(m manifest.Manifest, selected manifest.Selection, osName string) Changes {
	result := Changes{
		Profiles:       make([]string, 0),
		ExtraTags:      make([]string, 0),
		EffectiveTags:  cloneStrings(selected.Tags),
		ManagedEntries: make([]string, 0),
		Dependencies:   make([]string, 0),
		Provisioners:   make([]string, 0),
	}
	seenEntries := make(map[string]bool)
	seenDependencies := make(map[string]bool)
	seenProvisioners := make(map[string]bool)
	addDependencies := func(dependencies []manifest.Dependency) {
		for _, dependency := range dependencies {
			if !seenDependencies[dependency.Name] {
				seenDependencies[dependency.Name] = true
				result.Dependencies = append(result.Dependencies, dependency.Name)
			}
		}
	}
	for _, profileName := range selected.Profiles {
		addDependencies(m.Profiles[profileName].Dependencies)
	}
	for _, set := range m.Dependencies {
		if manifest.SharesTag(set.Tags, selected.Tags) && manifest.MatchesOS(set.OS, osName) {
			addDependencies(set.Dependencies)
		}
	}
	for _, entry := range m.Entries {
		if manifest.SharesTag(entry.Tags, selected.Tags) && manifest.MatchesOS(entry.OS, osName) {
			if !seenEntries[entry.Target] {
				seenEntries[entry.Target] = true
				result.ManagedEntries = append(result.ManagedEntries, entry.Target)
			}
			addDependencies(entry.Dependencies)
		}
	}
	for _, provisioner := range m.Provisioners {
		if manifest.SharesTag(provisioner.Tags, selected.Tags) && manifest.MatchesOS(provisioner.OS, osName) {
			if !seenProvisioners[provisioner.Tool] {
				seenProvisioners[provisioner.Tool] = true
				result.Provisioners = append(result.Provisioners, provisioner.Tool)
			}
			addDependencies(provisioner.Dependencies)
		}
	}
	return result
}

func surfaceDifference(left, right Changes) Changes {
	return Changes{
		Profiles:       make([]string, 0),
		ExtraTags:      make([]string, 0),
		EffectiveTags:  difference(left.EffectiveTags, right.EffectiveTags),
		ManagedEntries: difference(left.ManagedEntries, right.ManagedEntries),
		Dependencies:   difference(left.Dependencies, right.Dependencies),
		Provisioners:   difference(left.Provisioners, right.Provisioners),
	}
}

func deltaIsEmpty(delta Delta) bool {
	return len(delta.Added.Profiles) == 0 &&
		len(delta.Added.ExtraTags) == 0 &&
		len(delta.Added.EffectiveTags) == 0 &&
		len(delta.Added.ManagedEntries) == 0 &&
		len(delta.Added.Dependencies) == 0 &&
		len(delta.Added.Provisioners) == 0 &&
		len(delta.Removed.Profiles) == 0 &&
		len(delta.Removed.ExtraTags) == 0 &&
		len(delta.Removed.EffectiveTags) == 0 &&
		len(delta.Removed.ManagedEntries) == 0 &&
		len(delta.Removed.Dependencies) == 0 &&
		len(delta.Removed.Provisioners) == 0
}

func difference(left, right []string) []string {
	excluded := make(map[string]bool, len(right))
	for _, value := range right {
		excluded[value] = true
	}
	result := make([]string, 0)
	for _, value := range left {
		if !excluded[value] {
			result = append(result, value)
		}
	}
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func validateIntent(source Source, profiles, extraTags []string) error {
	for _, profile := range profiles {
		if strings.TrimSpace(profile) == "" {
			return fmt.Errorf("%s selection: profile names must not be empty", source)
		}
	}
	for _, tag := range extraTags {
		if strings.TrimSpace(tag) == "" {
			return fmt.Errorf("%s selection: tags must not be empty", source)
		}
	}
	if len(profiles) == 0 && len(extraTags) == 0 {
		return fmt.Errorf("%s selection: at least one Profile or extra Tag is required", source)
	}
	return nil
}

// Resolve validates the requested Profiles and retains both explicit intent and
// the resulting ordered Tag snapshot.
func Resolve(m manifest.Manifest, profiles, extraTags []string) (state.InstalledSelection, error) {
	resolved, err := manifest.ResolveSelection(m, profiles, extraTags)
	if err != nil {
		return state.InstalledSelection{}, err
	}
	return installedSelection(resolved.Profiles, extraTags, resolved.Tags), nil
}

// InstalledSelection converts effective command selection into the
// authoritative metadata shape committed at terminal success.
func (e Effective) InstalledSelection(provenance state.Provenance) state.InstalledSelection {
	installed := installedSelection(e.Profiles, e.ExtraTags, e.Selection.Tags)
	installed.Provenance = provenance
	return installed
}

func installedSelection(profiles, extraTags, resolvedTags []string) state.InstalledSelection {
	return state.InstalledSelection{
		Profiles:     cloneStrings(profiles),
		ExtraTags:    orderedUnique(extraTags),
		ResolvedTags: cloneStrings(resolvedTags),
	}
}

// Record reloads the latest Installation Metadata and commits only the
// authoritative Installed Selection, preserving Managed Entry and Provisioner
// inventory written earlier in the install.
func Record(path string, installed state.InstalledSelection) error {
	meta, err := state.Load(path)
	if err != nil {
		return err
	}
	meta.Version = state.CurrentVersion
	meta.InstalledSelection = &installed
	return state.Save(path, meta)
}

func orderedUnique(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func cloneStrings(values []string) []string {
	return append(make([]string, 0, len(values)), values...)
}
