package plan

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// Status describes what installing an Action would do to its target.
type Status string

const (
	StatusCreate        Status = "create"
	StatusConflict      Status = "conflict"
	StatusUnchanged     Status = "unchanged"
	StatusMissingSource Status = "missing-source"
)

// Action is a single planned filesystem change for a Managed Entry.
type Action struct {
	Source   string
	Target   string
	Strategy string
	Status   Status
}

// Plan is the preview of changes the installer would apply for a Profile.
type Plan struct {
	Profile string
	Actions []Action
}

// Options carries the resolved inputs needed to compute a Plan.
type Options struct {
	Profile    string
	OS         string
	SourceRoot string
	Home       string
}

// Build computes the Install Plan for the selected Profile without mutating
// the filesystem. It selects Managed Entries whose tags intersect the
// Profile's tags and that pass the OS filter, then determines each target's
// status against the current workstation state.
func Build(m manifest.Manifest, opts Options) (Plan, error) {
	profile, ok := m.Profiles[opts.Profile]
	if !ok {
		return Plan{}, fmt.Errorf("profile %q not found", opts.Profile)
	}

	plan := Plan{Profile: opts.Profile}
	for _, entry := range m.Entries {
		if !sharesTag(entry.Tags, profile.Tags) {
			continue
		}
		if !matchesOS(entry.OS, opts.OS) {
			continue
		}

		target := resolveTarget(entry.Target, opts.Home)
		sourceAbs := filepath.Join(opts.SourceRoot, entry.Source)
		plan.Actions = append(plan.Actions, Action{
			Source:   entry.Source,
			Target:   target,
			Strategy: entry.Strategy,
			Status:   status(entry.Strategy, target, sourceAbs),
		})
	}

	return plan, nil
}

func sharesTag(entryTags, profileTags []string) bool {
	for _, et := range entryTags {
		for _, pt := range profileTags {
			if et == pt {
				return true
			}
		}
	}
	return false
}

func matchesOS(entryOS []string, current string) bool {
	if len(entryOS) == 0 {
		return true
	}
	for _, osName := range entryOS {
		if osName == current {
			return true
		}
	}
	return false
}

func resolveTarget(target, home string) string {
	if target == "~" {
		return home
	}
	if strings.HasPrefix(target, "~/") {
		return filepath.Join(home, target[2:])
	}
	return target
}

func status(strategy, target, sourceAbs string) Status {
	// The managed source must exist before any target comparison is
	// meaningful: a missing source cannot be installed, and a symlink that
	// still points at a deleted source is broken, not unchanged.
	if _, err := os.Stat(sourceAbs); err != nil {
		return StatusMissingSource
	}

	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return StatusCreate
	}
	if err != nil {
		return StatusConflict
	}

	switch strategy {
	case "symlink":
		if info.Mode()&os.ModeSymlink == 0 {
			return StatusConflict
		}
		dest, err := os.Readlink(target)
		if err != nil || dest != sourceAbs {
			return StatusConflict
		}
		return StatusUnchanged
	case "copy":
		if !info.Mode().IsRegular() {
			return StatusConflict
		}
		if same, err := sameContent(target, sourceAbs); err != nil || !same {
			return StatusConflict
		}
		return StatusUnchanged
	default:
		return StatusConflict
	}
}

func sameContent(a, b string) (bool, error) {
	da, err := os.ReadFile(a)
	if err != nil {
		return false, err
	}
	db, err := os.ReadFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(da, db), nil
}
