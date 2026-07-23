// Package selection resolves and persists authoritative Installed Selection.
package selection

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
)

// Source identifies where a read-only command obtained its selection.
type Source string

const (
	SourceExplicit Source = "explicit"
	SourceRecorded Source = "recorded"
)

// ErrSelectionRequired means that neither invocation arguments nor Installation
// Metadata supplied a selection. Read-only commands must not silently use a
// manifest's default Profile in this case.
var ErrSelectionRequired = errors.New("selection required: provide --profile or --tag, or run dots install to record an Installed Selection")

// Report is the stable, read-only description of the selection used by a
// command.
type Report struct {
	Source        Source   `json:"source"`
	Profiles      []string `json:"profiles"`
	ExtraTags     []string `json:"extra_tags"`
	EffectiveTags []string `json:"effective_tags"`
}

// Effective contains the validated inputs for downstream command builders and
// their corresponding report.
type Effective struct {
	Profiles  []string
	ExtraTags []string
	Selection manifest.Selection
	Report    Report
}

// ResolveReadOnly chooses and validates the selection for a read-only command.
// Any explicit Profile or tag wins over the recorded Installed Selection.
func ResolveReadOnly(m manifest.Manifest, explicitProfiles, explicitTags []string, recorded *state.InstalledSelection) (Effective, error) {
	source := SourceExplicit
	profiles := explicitProfiles
	extraTags := explicitTags
	if len(explicitProfiles) == 0 && len(explicitTags) == 0 {
		if recorded == nil {
			return Effective{}, ErrSelectionRequired
		}
		source = SourceRecorded
		profiles = recorded.Profiles
		extraTags = recorded.ExtraTags
	}
	if err := validateIntent(source, profiles, extraTags); err != nil {
		return Effective{}, err
	}

	resolved, err := manifest.ResolveReadOnlySelection(m, profiles, extraTags)
	if err != nil {
		return Effective{}, fmt.Errorf("%s selection: %w", source, err)
	}

	orderedProfiles := cloneStrings(resolved.Profiles)
	orderedExtraTags := orderedUnique(extraTags)
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
			Source:        source,
			Profiles:      orderedProfiles,
			ExtraTags:     orderedExtraTags,
			EffectiveTags: effectiveTags,
		},
	}, nil
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
	return state.InstalledSelection{
		Profiles:     append([]string(nil), resolved.Profiles...),
		ExtraTags:    orderedUnique(extraTags),
		ResolvedTags: append([]string(nil), resolved.Tags...),
	}, nil
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
