// Package selection resolves and persists authoritative Installed Selection.
package selection

import (
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
)

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
