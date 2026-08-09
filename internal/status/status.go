// Package status computes the Dotfiles Status: the alignment between a
// workstation and the repository-owned Source of Truth for a Profile. It
// combines the Install Manifest, current filesystem state, and Installation
// Metadata to classify each managed target, including Drift for copied targets
// where filesystem inspection alone cannot distinguish an edited managed file
// from a foreign pre-existing file.
package status

import (
	"bytes"
	"fmt"
	"os"
	"reflect"

	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/seededstate"
	"github.com/yersonargotev/dots/internal/selectedsurface"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/textblock"
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
	Source       string   `json:"source"`
	Target       string   `json:"target"`
	Strategy     string   `json:"strategy"`
	State        State    `json:"state"`
	Reason       string   `json:"reason,omitempty"`
	MatchingTags []string `json:"matching_tags,omitempty"`
}

// Report is the Dotfiles Status for a Profile, in manifest order.
type Report struct {
	Profile   string            `json:"profile,omitempty"`
	Profiles  []string          `json:"profiles,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Selection *selection.Report `json:"selection,omitempty"`
	Entries   []Entry           `json:"entries"`
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
	Profile      string
	Profiles     []string
	ExtraTags    []string
	Selection    *manifest.Selection
	OS           string
	SourceRoot   string
	Home         string
	XDGStateHome string
}

// Build evaluates the Dotfiles Status for the selected Profile. It does not
// mutate the filesystem.
func Build(m manifest.Manifest, meta state.Metadata, opts Options) (Report, error) {
	resolved := opts.Selection
	if resolved == nil {
		selection, err := manifest.ResolveSelection(m, manifest.SelectedProfileNames(opts.Profile, opts.Profiles), opts.ExtraTags)
		if err != nil {
			return Report{}, err
		}
		resolved = &selection
	}
	tags := resolved.Tags
	surface := selectedsurface.Evaluate(m, tags, opts.OS)
	jsonContentByTarget, jsonSourcesByTarget, err := selectedJSONContributions(m, surface, opts)
	if err != nil {
		return Report{}, err
	}

	report := Report{Profile: resolved.Profile, Profiles: resolved.Profiles, Tags: resolved.Tags}
	for _, entry := range m.Entries {
		selected, applicable := selectedSurfaceEntry(surface, entry)
		if !applicable && !manifest.SharesTag(entry.Tags, tags) {
			continue
		}

		source := entry.Source
		if applicable {
			source = selected.Source
		}
		evaluated := Entry{Source: source, Target: entry.Target, Strategy: entry.Strategy}
		if !applicable {
			evaluated.State = StateSkipped
			report.Entries = append(report.Entries, evaluated)
			continue
		}

		target, err := plan.ResolveEntryTarget(entry, opts.Home, opts.XDGStateHome)
		if err != nil {
			return Report{}, err
		}
		evaluated.Target = target
		if err := plan.ValidateTargetParentInsideHome(target, opts.Home); err != nil {
			return Report{}, err
		}

		entry.Source = source
		st, err := evaluate(entry, target, meta, opts.SourceRoot, selected.Entry.Source, jsonContentByTarget[target], jsonSourcesByTarget[target])
		if err != nil {
			return Report{}, err
		}
		evaluated.State = st
		if st == StateOK && entry.Ownership == "seeded" {
			local, localErr := seededLocalEvolution(entry, target, source, opts.SourceRoot, meta)
			if localErr != nil {
				return Report{}, localErr
			}
			if local {
				evaluated.Reason = plan.ReasonSeededLocalEvolution
			}
		}
		if st == StateConflict {
			matchingTags, err := plan.MatchingUnselectedSourceOverrideTags(entry, tags, target, opts.SourceRoot)
			if err != nil {
				return Report{}, err
			}
			if len(matchingTags) > 0 {
				evaluated.Reason = plan.ConflictReasonSourceOverrideNotSelected
				evaluated.MatchingTags = matchingTags
			}
		}
		report.Entries = append(report.Entries, evaluated)
	}

	return report, nil
}

func selectedJSONContributions(m manifest.Manifest, surface selectedsurface.Surface, opts Options) (map[string][]byte, map[string][]string, error) {
	pathsByTarget := map[string][]string{}
	sourcesByTarget := map[string][]string{}
	for _, entry := range m.Entries {
		selected, applicable := selectedSurfaceEntry(surface, entry)
		if !applicable || entry.Strategy != "copy" || entry.Ownership != "json-subset" {
			continue
		}
		source := selected.Source
		target, err := plan.ResolveEntryTarget(entry, opts.Home, opts.XDGStateHome)
		if err != nil {
			return nil, nil, err
		}
		if err := plan.ValidateResolvedTarget(target, opts.Home); err != nil {
			return nil, nil, err
		}
		if err := plan.ValidateTargetParentInsideHome(target, opts.Home); err != nil {
			return nil, nil, err
		}
		if _, err := os.Lstat(target); os.IsNotExist(err) {
			continue
		}
		sourcePath, err := plan.ResolveSource(source, opts.SourceRoot)
		if err != nil {
			return nil, nil, err
		}
		if err := plan.ValidateResolvedSource(sourcePath, opts.SourceRoot); err != nil {
			return nil, nil, err
		}
		pathsByTarget[target] = append(pathsByTarget[target], sourcePath)
		sourcesByTarget[target] = append(sourcesByTarget[target], source)
	}

	contentByTarget := make(map[string][]byte, len(pathsByTarget))
	for target, paths := range pathsByTarget {
		content, err := configsubset.ComposeJSONFiles(paths)
		if err != nil {
			return nil, nil, fmt.Errorf("compose JSON ownership for %s: %w", target, err)
		}
		contentByTarget[target] = content
	}
	return contentByTarget, sourcesByTarget, nil
}

func selectedSurfaceEntry(surface selectedsurface.Surface, entry manifest.Entry) (selectedsurface.SelectedEntry, bool) {
	for _, selected := range surface.Entries {
		if reflect.DeepEqual(selected.Entry, entry) {
			return selected, true
		}
	}
	return selectedsurface.SelectedEntry{}, false
}

func evaluate(entry manifest.Entry, target string, meta state.Metadata, sourceRoot string, defaultSource string, currentJSON []byte, currentSources []string) (State, error) {
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
	if entry.Ownership == "seeded" {
		info, err := os.Lstat(target)
		if err != nil {
			return "", fmt.Errorf("stat seeded target %s: %w", target, err)
		}
		rec, recorded := meta.FindByTarget(target)
		if !info.Mode().IsRegular() || !recorded || rec.Strategy != entry.Strategy || rec.Source != entry.Source || rec.Ownership != "seeded" {
			if recorded && rec.Strategy == entry.Strategy {
				return StateDrifted, nil
			}
			return StateConflict, nil
		}
		// Any trusted regular-file difference is either an advanceable old
		// baseline or expected local evolution. Neither is Drift in status;
		// install performs the conditional advancement.
		return StateOK, nil
	}
	if entry.Ownership == "marked-block" {
		info, err := os.Lstat(target)
		if err != nil {
			return "", fmt.Errorf("stat marked-block target %s: %w", target, err)
		}
		rec, recorded := meta.FindByTarget(target)
		if !info.Mode().IsRegular() || !recorded || rec.Strategy != entry.Strategy || rec.Source != entry.Source || rec.Ownership != "marked-block" || len(rec.OwnedBytes) == 0 {
			return StateConflict, nil
		}
		live, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("read marked-block target %s: %w", target, err)
		}
		current, err := os.ReadFile(sourceAbs)
		if err != nil {
			return "", fmt.Errorf("read marked-block source %s: %w", sourceAbs, err)
		}
		reconciliation := textblock.ReconcileOwned(live, rec.OwnedBytes, current, textblock.DotsManagedMarkers())
		if !reconciliation.Compatible {
			return StateConflict, nil
		}
		if reconciliation.Changed {
			return StateDrifted, nil
		}
		return StateOK, nil
	}
	if entry.Ownership == "toml-subset" {
		info, err := os.Lstat(target)
		if err != nil {
			return "", fmt.Errorf("stat TOML target %s: %w", target, err)
		}
		rec, recorded := meta.FindByTarget(target)
		if !info.Mode().IsRegular() {
			if recorded && rec.Strategy == entry.Strategy {
				return StateDrifted, nil
			}
			return StateConflict, nil
		}
		targetData, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("read TOML target %s: %w", target, err)
		}
		sourceData, err := os.ReadFile(sourceAbs)
		if err != nil {
			return "", fmt.Errorf("read source TOML %s: %w", sourceAbs, err)
		}
		if meta.MatchesEntry(target, entry.Source, entry.Strategy) || meta.MatchesEntry(target, defaultSource, entry.Strategy) {
			if rec.Ownership == "toml-subset" && len(rec.OwnedBytes) > 0 {
				reconciliation, err := configsubset.ReconcileTOML(targetData, rec.OwnedBytes, sourceData)
				if err != nil {
					return "", fmt.Errorf("reconcile TOML target %s: %w", target, err)
				}
				if reconciliation.Compatible && !reconciliation.Changed {
					return StateOK, nil
				}
				return StateDrifted, nil
			}
			relation, err := configsubset.AnalyzeTOML(targetData, sourceData)
			if err != nil {
				return "", fmt.Errorf("analyze TOML target %s: %w", target, err)
			}
			if relation.Contains {
				return StateOK, nil
			}
			return StateDrifted, nil
		}
		return StateConflict, nil
	}
	if entry.Ownership == "json-subset" && len(currentJSON) > 0 {
		info, err := os.Lstat(target)
		if err != nil {
			return "", fmt.Errorf("stat target %s: %w", target, err)
		}
		if !info.Mode().IsRegular() {
			if trustedJSONRecord(meta, target, entry.Strategy, currentSources) {
				return StateDrifted, nil
			}
			return StateConflict, nil
		}
		targetData, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("read JSON target %s: %w", target, err)
		}
		if bytes.Equal(targetData, currentJSON) {
			return StateOK, nil
		}
		rec, recorded := meta.FindByTarget(target)
		if trustedJSONRecord(meta, target, entry.Strategy, currentSources) {
			if rec.Ownership == "json-subset" && len(rec.OwnedContent) > 0 {
				reconciliation, err := configsubset.ReconcileJSON(targetData, rec.OwnedContent, currentJSON)
				if err != nil {
					return "", fmt.Errorf("reconcile JSON target %s: %w", target, err)
				}
				if reconciliation.Compatible && !reconciliation.Changed {
					return StateOK, nil
				}
				overall, err := configsubset.AnalyzeJSON(targetData, currentJSON)
				if err != nil {
					return "", fmt.Errorf("analyze current JSON ownership for %s: %w", target, err)
				}
				if overall.Contains {
					if len(currentSources) > 0 && entry.Source != currentSources[0] {
						return StateOK, nil
					}
					return StateDrifted, nil
				}
				entryRelation, err := configsubset.AnalyzeJSONFiles(target, sourceAbs)
				if err != nil {
					return "", err
				}
				if entryRelation.Contains {
					return StateOK, nil
				}
				return StateDrifted, nil
			}
			relation, err := configsubset.AnalyzeJSON(targetData, currentJSON)
			if err != nil {
				return "", fmt.Errorf("analyze JSON target %s: %w", target, err)
			}
			if relation.Contains {
				return StateOK, nil
			}
			entryRelation, err := configsubset.AnalyzeJSONFiles(target, sourceAbs)
			if err != nil {
				return "", err
			}
			if entryRelation.Contains {
				return StateOK, nil
			}
			return StateDrifted, nil
		}
		if recorded && rec.Strategy == entry.Strategy {
			return StateDrifted, nil
		}
		return StateConflict, nil
	}
	if entry.Ownership == "jsonc-subset" {
		info, err := os.Lstat(target)
		if err != nil {
			return "", fmt.Errorf("stat target %s: %w", target, err)
		}
		if !info.Mode().IsRegular() {
			if meta.MatchesEntry(target, entry.Source, entry.Strategy) {
				return StateDrifted, nil
			}
			return StateConflict, nil
		}
		targetData, err := os.ReadFile(target)
		if err != nil {
			return "", fmt.Errorf("read JSONC target %s: %w", target, err)
		}
		sourceData, err := os.ReadFile(sourceAbs)
		if err != nil {
			return "", fmt.Errorf("read source JSONC %s: %w", sourceAbs, err)
		}
		if meta.MatchesEntry(target, entry.Source, entry.Strategy) {
			rec, _ := meta.FindByTarget(target)
			if rec.Ownership == "jsonc-subset" && len(rec.OwnedContent) > 0 {
				reconciliation, err := configsubset.ReconcileJSONC(targetData, rec.OwnedContent, sourceData)
				if err != nil {
					return "", fmt.Errorf("reconcile JSONC target %s: %w", target, err)
				}
				if reconciliation.Compatible && !reconciliation.Changed {
					return StateOK, nil
				}
				return StateDrifted, nil
			}
			relation, err := configsubset.AnalyzeJSONC(targetData, sourceData)
			if err != nil {
				return "", fmt.Errorf("analyze JSONC target %s: %w", target, err)
			}
			if relation.Contains {
				return StateOK, nil
			}
			return StateDrifted, nil
		}
		return StateConflict, nil
	}

	if isSubsetOwned(entry.Ownership) && meta.MatchesEntry(target, entry.Source, entry.Strategy) {
		subset, err := subsetContent(entry.Ownership, target, sourceAbs)
		if err != nil {
			return "", err
		}
		if subset {
			return StateOK, nil
		}
	}
	if entry.Ownership == "toml-subset" && targetContainsCompatibleRecordedSource(entry, target, sourceRoot, meta, defaultSource) {
		return StateDrifted, nil
	}

	// The target diverges from the Source of Truth. Installation Metadata is the
	// discriminator: if dots installed this target, the divergence is Drift; if
	// not, it is a Conflict with a file dots never managed.
	if meta.MatchesEntry(target, entry.Source, entry.Strategy) {
		return StateDrifted, nil
	}
	return StateConflict, nil
}

func seededLocalEvolution(entry manifest.Entry, target, source, sourceRoot string, meta state.Metadata) (bool, error) {
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Strategy != entry.Strategy || rec.Source != source || rec.Ownership != "seeded" {
		return false, nil
	}
	sourceAbs, err := plan.ResolveSource(source, sourceRoot)
	if err != nil {
		return false, err
	}
	live, err := os.ReadFile(target)
	if err != nil {
		return false, fmt.Errorf("read seeded target %s: %w", target, err)
	}
	current, err := os.ReadFile(sourceAbs)
	if err != nil {
		return false, fmt.Errorf("read seeded source %s: %w", sourceAbs, err)
	}
	return seededstate.Reconcile(live, rec.SeededBaseline, current).Classification == seededstate.LocalEvolution, nil
}

func trustedJSONRecord(meta state.Metadata, target, strategy string, currentSources []string) bool {
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Strategy != strategy || len(rec.SourceList()) == 0 {
		return false
	}
	selected := make(map[string]struct{}, len(currentSources))
	for _, source := range currentSources {
		selected[source] = struct{}{}
	}
	for _, source := range rec.SourceList() {
		if _, ok := selected[source]; !ok {
			return false
		}
	}
	return true
}

func targetContainsCompatibleRecordedSource(entry manifest.Entry, target, sourceRoot string, meta state.Metadata, defaultSource string) bool {
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Strategy != entry.Strategy || !compatibleEntrySource(entry, defaultSource, rec.Source) {
		return false
	}
	sourceAbs, err := plan.ResolveSource(rec.Source, sourceRoot)
	if err != nil {
		return false
	}
	if err := plan.ValidateResolvedSource(sourceAbs, sourceRoot); err != nil {
		return false
	}
	subset, err := subsetContent(entry.Ownership, target, sourceAbs)
	return err == nil && subset
}

func compatibleEntrySource(entry manifest.Entry, defaultSource, source string) bool {
	if source == entry.Source || source == defaultSource {
		return true
	}
	for _, override := range entry.SourceOverrides {
		if source == override {
			return true
		}
	}
	return false
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

func isSubsetOwned(ownership string) bool {
	return ownership == "json-subset" || ownership == "jsonc-subset" || ownership == "toml-subset"
}

func subsetContent(ownership, target, sourceAbs string) (bool, error) {
	switch ownership {
	case "json-subset":
		return configsubset.JSONFileContains(target, sourceAbs)
	case "jsonc-subset":
		targetData, err := os.ReadFile(target)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", target, err)
		}
		sourceData, err := os.ReadFile(sourceAbs)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", sourceAbs, err)
		}
		relation, err := configsubset.AnalyzeJSONC(targetData, sourceData)
		return relation.Contains, err
	case "toml-subset":
		return configsubset.TOMLFileContains(target, sourceAbs)
	default:
		return false, nil
	}
}
