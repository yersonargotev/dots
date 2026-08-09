package plan

import (
	"fmt"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/profilesel"
)

// selectedEntryIndices maps the Selected Surface back to manifest positions so
// SkippedEntries can compare entry identity across Profiles without repeating
// Tag or OS selection. It mirrors provision.selectedIndices for Provisioners.
func selectedEntryIndices(m manifest.Manifest, profileName, os string) (map[int]bool, error) {
	profile, ok := m.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}

	indices := make(map[int]bool)
	for _, selected := range selectedEntries(m, profile.Tags, os) {
		indices[selected.Index] = true
	}
	return indices, nil
}

// SkippedEntries reports whether the active profile omits file entries that
// another profile would select on this OS, and which single profile best recovers
// them. It is a thin adapter over profilesel.Skipped, injecting the entry index
// selection; provision.SkippedProvisioners is its provisioner twin over the same
// shared math. It is PURE: no I/O and safe in a dry-run.
func SkippedEntries(m manifest.Manifest, opts Options) (profilesel.Hint, bool, error) {
	if len(opts.Profiles) > 1 {
		return profilesel.SkippedSelection(m.Profiles, opts.Profiles, opts.OS, func(name, os string) (map[int]bool, error) {
			return selectedEntryIndices(m, name, os)
		})
	}
	active := opts.Profile
	if len(opts.Profiles) == 1 {
		active = opts.Profiles[0]
	} else if active == "" {
		if _, ok := m.Profiles["default"]; ok {
			active = "default"
		}
	}
	return profilesel.Skipped(m.Profiles, active, opts.OS, func(name, os string) (map[int]bool, error) {
		return selectedEntryIndices(m, name, os)
	})
}
