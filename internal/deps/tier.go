package deps

import "strings"

// Tier is the Dependency Plan Tier: the level of OS-specific install guidance
// available for the current Supported Platform.
type Tier string

const (
	// TierHomebrew is macOS guidance via Homebrew.
	TierHomebrew Tier = "homebrew"
	// TierDebian is Debian/Ubuntu guidance via apt.
	TierDebian Tier = "debian"
	// TierFedora is Fedora/RHEL-family guidance via dnf.
	TierFedora Tier = "fedora"
	// TierArch is Arch-family guidance via pacman.
	TierArch Tier = "arch"
	// TierGeneric is the fallback for Linux distributions without a specific tier.
	TierGeneric Tier = "generic"
)

// OSRelease holds the identification fields read from /etc/os-release that
// determine the Linux Dependency Plan Tier.
type OSRelease struct {
	ID     string
	IDLike string
}

// ResolveTier maps the host OS and Linux distro identification to a Dependency
// Plan Tier. macOS is always Homebrew; Linux is matched by distro ID then by
// ID_LIKE family, falling back to generic guidance when unrecognized.
func ResolveTier(goos string, release OSRelease) Tier {
	if goos == "darwin" {
		return TierHomebrew
	}
	if goos != "linux" {
		return TierGeneric
	}

	id := strings.ToLower(strings.TrimSpace(release.ID))
	likes := strings.Fields(strings.ToLower(release.IDLike))

	switch id {
	case "debian", "ubuntu":
		return TierDebian
	case "fedora", "rhel", "centos":
		return TierFedora
	case "arch":
		return TierArch
	}

	for _, like := range likes {
		switch like {
		case "debian", "ubuntu":
			return TierDebian
		case "fedora", "rhel", "centos":
			return TierFedora
		case "arch":
			return TierArch
		}
	}

	return TierGeneric
}

// ParseOSRelease extracts the ID and ID_LIKE fields from /etc/os-release
// content, stripping surrounding quotes. Unknown lines are ignored.
func ParseOSRelease(content string) OSRelease {
	var release OSRelease
	for _, line := range strings.Split(content, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "ID":
			release.ID = value
		case "ID_LIKE":
			release.IDLike = value
		}
	}
	return release
}
