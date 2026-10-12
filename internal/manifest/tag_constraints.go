package manifest

import (
	"fmt"
	"runtime"
	"slices"
)

func validateTagConstraints(m Manifest, name string, tag Tag) error {
	seen := map[string]bool{}
	for _, other := range tag.ConflictsWith {
		if _, ok := m.Tags[other]; !ok || other == name || seen[other] {
			return fmt.Errorf("tags[%q].conflicts_with must reference distinct other declared tags: %q", name, other)
		}
		seen[other] = true
	}
	seen = map[string]bool{}
	for _, platform := range tag.SupportedPlatforms {
		if !slices.Contains([]string{"darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"}, platform) || seen[platform] {
			return fmt.Errorf("tags[%q].supported_platforms contains an invalid or duplicate platform %q", name, platform)
		}
		seen[platform] = true
	}
	return nil
}

func validateTagConflicts(m Manifest, tags []string) error {
	for _, name := range tags {
		for _, other := range m.Tags[name].ConflictsWith {
			if slices.Contains(tags, other) {
				return fmt.Errorf("tags %q and %q cannot be selected together; choose one variant", name, other)
			}
		}
	}
	return nil
}

// ValidateSelection checks constraints at an execution/plan boundary, including
// selections supplied by a caller rather than resolved from profile arguments.
// Empty architecture uses the current host, matching other planning APIs.
func ValidateSelection(m Manifest, selected Selection, osName, arch string) error {
	if err := validateTagConflicts(m, selected.Tags); err != nil {
		return err
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	if osName == "" {
		osName = runtime.GOOS
	}
	platform := osName + "_" + arch
	for _, name := range selected.Tags {
		allowed := m.Tags[name].SupportedPlatforms
		if len(allowed) > 0 && !slices.Contains(allowed, platform) {
			return fmt.Errorf("tag %q does not support %s; supported platforms: %v", name, platform, allowed)
		}
	}
	return nil
}
