// Package selection resolves and persists authoritative Installed Selection.
package selection

import (
	"errors"
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
)

// Source identifies where a selection-aware command obtained its intent.
type Source string

const (
	SourceExplicit Source = "explicit"
	SourceRecorded Source = "recorded"
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
// Manifest without changing whether its source was explicit or recorded.
func ResolveIntent(m manifest.Manifest, intent Intent) (Effective, error) {
	if intent.Source != SourceExplicit && intent.Source != SourceRecorded {
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
