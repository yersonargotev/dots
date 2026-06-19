// Package status computes the Dotfiles Status: the alignment between a
// workstation and the repository-owned Source of Truth for a Profile. It
// combines the Install Manifest, current filesystem state, and Installation
// Metadata to classify each managed target, including Drift for copied targets
// where filesystem inspection alone cannot distinguish an edited managed file
// from a foreign pre-existing file.
package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

// State classifies a managed target's alignment with the Source of Truth.
type State string

const (
	// StateOK means the target matches the repository-owned Source of Truth.
	StateOK State = "ok"
	// StateMissing means the managed target is not installed.
	StateMissing State = "missing"
	// StateConflict means a target exists that dots never installed and does not
	// match the Source of Truth. Installing would require explicit resolution.
	StateConflict State = "conflict"
	// StateSkipped means the entry belongs to the Profile but is excluded here by
	// its OS Filter, so this machine intentionally does not manage it.
	StateSkipped State = "skipped"
	// StateDrifted means dots installed this target (it has Installation
	// Metadata) but it no longer matches the Source of Truth.
	StateDrifted State = "drifted"
	// StateUnsupported means the entry's strategy cannot yet be evaluated, such
	// as template rendering, which is not implemented.
	StateUnsupported State = "unsupported"
)

// Entry is the evaluated alignment of one Managed Entry.
type Entry struct {
	Source   string
	Target   string
	Strategy string
	State    State
}

// Report is the Dotfiles Status for a Profile, in manifest order.
type Report struct {
	Profile string
	Entries []Entry
}

// HasFindings reports whether the Dotfiles Status contains any entry that
// diverges from the Source of Truth and requires action: missing, conflict,
// drifted, or unsupported. An ok or intentionally skipped entry is not a
// finding.
func (r Report) HasFindings() bool {
	for _, e := range r.Entries {
		switch e.State {
		case StateMissing, StateConflict, StateDrifted, StateUnsupported:
			return true
		}
	}
	return false
}

// Options carries the resolved inputs needed to evaluate status.
type Options struct {
	Profile    string
	OS         string
	SourceRoot string
	Home       string
}

// Build evaluates the Dotfiles Status for the selected Profile. It does not
// mutate the filesystem.
func Build(m manifest.Manifest, meta state.Metadata, opts Options) (Report, error) {
	profile, ok := m.Profiles[opts.Profile]
	if !ok {
		return Report{}, fmt.Errorf("profile %q not found", opts.Profile)
	}

	report := Report{Profile: opts.Profile}
	for _, entry := range m.Entries {
		if !manifest.SharesTag(entry.Tags, profile.Tags) {
			continue
		}

		evaluated := Entry{Source: entry.Source, Target: entry.Target, Strategy: entry.Strategy}
		if !manifest.MatchesOS(entry.OS, opts.OS) {
			evaluated.State = StateSkipped
			report.Entries = append(report.Entries, evaluated)
			continue
		}

		target, err := plan.ResolveTarget(entry.Target, opts.Home)
		if err != nil {
			return Report{}, err
		}
		evaluated.Target = target
		if err := plan.ValidateTargetParentInsideHome(target, opts.Home); err != nil {
			return Report{}, err
		}

		st, err := evaluate(entry, target, meta, opts.SourceRoot)
		if err != nil {
			return Report{}, err
		}
		evaluated.State = st
		report.Entries = append(report.Entries, evaluated)
	}

	return report, nil
}

func evaluate(entry manifest.Entry, target string, meta state.Metadata, sourceRoot string) (State, error) {
	if _, err := os.Lstat(target); err != nil {
		if os.IsNotExist(err) {
			return StateMissing, nil
		}
		return "", fmt.Errorf("stat target %s: %w", target, err)
	}

	// Template rendering is not implemented, so the expected installed content of
	// an existing templated target cannot be computed; report it honestly as
	// unsupported after missing targets have been classified as missing.
	if entry.Strategy == "template" {
		return StateUnsupported, nil
	}

	sourceAbs, err := plan.ResolveSource(entry.Source, sourceRoot)
	if err != nil {
		return "", err
	}
	if err := plan.ValidateResolvedSource(sourceAbs, sourceRoot); err != nil {
		return "", err
	}

	aligned, err := matchesSource(entry.Strategy, target, sourceAbs)
	if err != nil {
		return "", err
	}
	if aligned {
		return StateOK, nil
	}

	if entry.Ownership == "json-subset" && metadataMatchesEntry(meta, target, entry.Source, entry.Strategy) {
		subset, err := jsonSubsetContent(target, sourceAbs)
		if err != nil {
			return "", err
		}
		if subset {
			return StateOK, nil
		}
	}

	// The target diverges from the Source of Truth. Installation Metadata is the
	// discriminator: if dots installed this target, the divergence is Drift; if
	// not, it is a Conflict with a file dots never managed.
	if metadataMatchesEntry(meta, target, entry.Source, entry.Strategy) {
		return StateDrifted, nil
	}
	return StateConflict, nil
}

func metadataMatchesEntry(meta state.Metadata, target, source, strategy string) bool {
	rec, ok := meta.FindByTarget(target)
	return ok && rec.Source == source && rec.Strategy == strategy
}

func matchesSource(strategy, target, sourceAbs string) (bool, error) {
	switch strategy {
	case "symlink":
		info, err := os.Lstat(target)
		if err != nil {
			return false, fmt.Errorf("stat target %s: %w", target, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return false, nil
		}
		dest, err := os.Readlink(target)
		if err != nil {
			return false, fmt.Errorf("readlink target %s: %w", target, err)
		}
		return dest == sourceAbs, nil
	case "copy":
		info, err := os.Lstat(target)
		if err != nil {
			return false, fmt.Errorf("stat target %s: %w", target, err)
		}
		if !info.Mode().IsRegular() {
			return false, nil
		}
		return sameContent(target, sourceAbs)
	default:
		return false, nil
	}
}

func sameContent(a, b string) (bool, error) {
	da, err := os.ReadFile(a)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", a, err)
	}
	db, err := os.ReadFile(b)
	if err != nil {
		// A missing or unreadable Source of Truth means the target cannot be
		// confirmed aligned; treat it as a divergence rather than an error so the
		// caller can still classify it as conflict or drift.
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", b, err)
	}
	return bytes.Equal(da, db), nil
}

func jsonSubsetContent(target, sourceAbs string) (bool, error) {
	sourceData, err := os.ReadFile(sourceAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", sourceAbs, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", target, err)
	}

	var sourceValue, targetValue any
	if err := json.Unmarshal(sourceData, &sourceValue); err != nil {
		return false, fmt.Errorf("parse source JSON %s: %w", sourceAbs, err)
	}
	if err := json.Unmarshal(targetData, &targetValue); err != nil {
		return false, nil
	}
	return jsonContains(targetValue, sourceValue), nil
}

func jsonContains(target, source any) bool {
	switch sourceTyped := source.(type) {
	case map[string]any:
		targetTyped, ok := target.(map[string]any)
		if !ok {
			return false
		}
		for key, sourceChild := range sourceTyped {
			targetChild, ok := targetTyped[key]
			if !ok || !jsonContains(targetChild, sourceChild) {
				return false
			}
		}
		return true
	case []any:
		targetTyped, ok := target.([]any)
		if !ok {
			return false
		}
		for _, sourceItem := range sourceTyped {
			found := false
			for _, targetItem := range targetTyped {
				if jsonContains(targetItem, sourceItem) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(target, source)
	}
}
