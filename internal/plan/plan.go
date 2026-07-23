package plan

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/dots/internal/configsubset"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
)

// Status describes what installing an Action would do to its target.
type Status string

const (
	StatusCreate        Status = "create"
	StatusUpdate        Status = "update"
	StatusConflict      Status = "conflict"
	StatusUnchanged     Status = "unchanged"
	StatusMissingSource Status = "missing-source"
)

// Action is a single planned filesystem change for a Managed Entry.
type Action struct {
	Source string `json:"source"`
	// ResolvedSource is an absolute, machine-local path and is therefore kept out
	// of the Agent Output Contract: it differs between machines and install
	// locations, which would break idempotent envelope comparisons. Agents key on
	// the portable, manifest-relative Source instead.
	ResolvedSource string `json:"-"`
	Target         string `json:"target"`
	Strategy       string `json:"strategy"`
	Status         Status `json:"status"`
	Ownership      string `json:"-"`
}

// Plan is the preview of changes the installer would apply for a Profile.
type Plan struct {
	Profile   string            `json:"profile,omitempty"`
	Profiles  []string          `json:"profiles,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Selection *selection.Report `json:"selection,omitempty"`
	Actions   []Action          `json:"actions"`
}

// HasFindings reports whether the Install Plan contains an action the caller
// must act on before a clean apply: an unresolved Conflict, or a missing source
// the manifest declares but the Installed Repository does not provide.
func (p Plan) HasFindings() bool {
	for _, a := range p.Actions {
		switch a.Status {
		case StatusConflict, StatusMissingSource:
			return true
		}
	}
	return false
}

// Options carries the resolved inputs needed to compute a Plan.
type Options struct {
	Profile    string
	Profiles   []string
	ExtraTags  []string
	Selection  *manifest.Selection
	OS         string
	SourceRoot string
	Home       string
	Metadata   state.Metadata
}

// Build computes the Install Plan for the selected Profile without mutating
// the filesystem. It selects Managed Entries whose tags intersect the
// Profile's tags and that pass the OS filter, then determines each target's
// status against the current workstation state.
func Build(m manifest.Manifest, opts Options) (Plan, error) {
	resolved := opts.Selection
	if resolved == nil {
		selection, err := manifest.ResolveSelection(m, manifest.SelectedProfileNames(opts.Profile, opts.Profiles), opts.ExtraTags)
		if err != nil {
			return Plan{}, err
		}
		resolved = &selection
	}
	tags := resolved.Tags

	plan := Plan{Profile: resolved.Profile, Profiles: resolved.Profiles, Tags: resolved.Tags}
	for _, entry := range m.Entries {
		if !manifest.SharesTag(entry.Tags, tags) {
			continue
		}
		if !manifest.MatchesOS(entry.OS, opts.OS) {
			continue
		}

		defaultSource := entry.Source
		source := manifest.EntrySource(entry, tags)
		target, err := ResolveTarget(entry.Target, opts.Home)
		if err != nil {
			return Plan{}, err
		}
		sourceAbs, err := ResolveSource(source, opts.SourceRoot)
		if err != nil {
			return Plan{}, err
		}
		entry.Source = source
		actionStatus, err := status(entry, target, sourceAbs, opts.SourceRoot, opts.Metadata, defaultSource)
		if err != nil {
			return Plan{}, err
		}
		plan.Actions = append(plan.Actions, Action{
			Source:         source,
			ResolvedSource: sourceAbs,
			Target:         target,
			Strategy:       entry.Strategy,
			Status:         actionStatus,
			Ownership:      entry.Ownership,
		})
	}

	return plan, nil
}

// ResolveTarget resolves a manifest target inside home. Targets are deliberately
// limited to "~" and "~/" forms so a manifest cannot escape a sandboxed --home.
func ResolveTarget(target, home string) (string, error) {
	resolvedHome, err := filepath.Abs(home)
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	resolvedHome = filepath.Clean(resolvedHome)

	if target == "~" {
		return resolvedHome, nil
	}
	if !strings.HasPrefix(target, "~/") {
		return "", fmt.Errorf("unsafe target %q: target must be ~ or ~/...", target)
	}
	resolvedTarget := filepath.Clean(filepath.Join(resolvedHome, target[2:]))
	if !InsideRoot(resolvedTarget, resolvedHome) {
		return "", fmt.Errorf("unsafe target %q: resolved target escapes home %q", target, resolvedHome)
	}
	return resolvedTarget, nil
}

// ValidateResolvedTarget verifies that an already-resolved plan target remains
// inside home before install writes to it.
func ValidateResolvedTarget(target, home string) error {
	resolvedHome, err := filepath.Abs(home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	resolvedTarget, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target %q: %w", target, err)
	}
	resolvedHome = filepath.Clean(resolvedHome)
	resolvedTarget = filepath.Clean(resolvedTarget)
	if !InsideRoot(resolvedTarget, resolvedHome) {
		return fmt.Errorf("unsafe target %q: resolved target escapes home %q", target, resolvedHome)
	}
	return nil
}

// ValidateTargetParentInsideHome verifies that a resolved target's existing
// parent path does not escape home through symlinks before callers inspect or
// mutate target contents.
func ValidateTargetParentInsideHome(target, home string) error {
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve target %s: %w", target, err)
	}
	return ValidatePathInsideHomeNoSymlinkEscape(filepath.Dir(targetAbs), home, "target parent")
}

// ValidatePathInsideHomeNoSymlinkEscape verifies that path is lexically inside
// home and that every existing component resolves through symlinks to a path
// still contained by home. Missing suffixes are safe because they cannot yet
// redirect filesystem reads or writes.
func ValidatePathInsideHomeNoSymlinkEscape(path, home, label string) error {
	return validatePathInsideHomeNoSymlinkEscape(path, home, label, false)
}

// ValidateFilePathInsideHomeNoSymlinkEscape verifies that a file path is
// lexically inside home, existing parent components do not escape home through
// symlinks, and an existing file leaf is not a symlink. A missing file leaf is
// safe because callers may create it after validating the parent path.
func ValidateFilePathInsideHomeNoSymlinkEscape(path, home, label string) error {
	return validatePathInsideHomeNoSymlinkEscape(path, home, label, true)
}

func validatePathInsideHomeNoSymlinkEscape(path, home, label string, finalMayBeFile bool) error {
	homeAbs, err := filepath.Abs(home)
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %s %s: %w", label, path, err)
	}
	homeAbs = filepath.Clean(homeAbs)
	pathAbs = filepath.Clean(pathAbs)

	if !InsideRoot(pathAbs, homeAbs) {
		return fmt.Errorf("unsafe %s %q: resolved path escapes home %q", label, pathAbs, homeAbs)
	}

	homeReal, err := filepath.EvalSymlinks(homeAbs)
	if err != nil {
		return fmt.Errorf("resolve real home %s: %w", homeAbs, err)
	}
	homeReal = filepath.Clean(homeReal)

	rel, err := filepath.Rel(homeAbs, pathAbs)
	if err != nil {
		return fmt.Errorf("resolve %s %s relative to home %s: %w", label, pathAbs, homeAbs, err)
	}
	current := homeAbs
	parts := splitPath(rel)
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("stat %s %s: %w", label, current, err)
		}
		isFinal := i == len(parts)-1
		if info.Mode()&os.ModeSymlink == 0 && !info.IsDir() {
			if finalMayBeFile && isFinal && info.Mode().IsRegular() {
				return nil
			}
			return fmt.Errorf("%s %s is not a directory", label, current)
		}
		if finalMayBeFile && isFinal && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe %s %q: file leaf must not be a symlink", label, current)
		}
		realPath, err := filepath.EvalSymlinks(current)
		if err != nil {
			return fmt.Errorf("resolve %s %s: %w", label, current, err)
		}
		if !InsideRoot(realPath, homeReal) {
			return fmt.Errorf("unsafe %s %q: symlink resolves outside home %q", label, current, homeAbs)
		}
	}
	return nil
}

func splitPath(path string) []string {
	parts := []string{}
	separator := string(filepath.Separator)
	for _, part := range strings.Split(path, separator) {
		if part != "" && part != "." {
			parts = append(parts, part)
		}
	}
	return parts
}

// ResolveSource resolves a manifest source inside sourceRoot. Sources must be
// repository-relative so a manifest cannot escape a sandboxed --source-root.
func ResolveSource(source, sourceRoot string) (string, error) {
	if filepath.IsAbs(source) {
		return "", fmt.Errorf("unsafe source %q: source must be relative", source)
	}
	resolvedRoot, err := filepath.Abs(sourceRoot)
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	resolvedRoot = filepath.Clean(resolvedRoot)
	resolvedSource := filepath.Clean(filepath.Join(resolvedRoot, source))
	if !InsideRoot(resolvedSource, resolvedRoot) {
		return "", fmt.Errorf("unsafe source %q: resolved source escapes source root %q", source, resolvedRoot)
	}
	return resolvedSource, nil
}

// InsideRoot reports whether path is root or is contained beneath root.
func InsideRoot(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func status(entry manifest.Entry, target, sourceAbs, sourceRoot string, meta state.Metadata, defaultSource string) (Status, error) {
	// The managed source must exist before any target comparison is
	// meaningful: a missing source cannot be installed, and a symlink that
	// still points at a deleted source is broken, not unchanged.
	if exists, err := safeSourceExists(sourceAbs, sourceRoot); err != nil {
		return "", err
	} else if !exists {
		return StatusMissingSource, nil
	}

	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return StatusCreate, nil
	}
	if err != nil {
		return StatusConflict, nil
	}

	switch entry.Strategy {
	case "symlink":
		if info.Mode()&os.ModeSymlink == 0 {
			return StatusConflict, nil
		}
		dest, err := os.Readlink(target)
		if err != nil || dest != sourceAbs {
			return StatusConflict, nil
		}
		return StatusUnchanged, nil
	case "copy":
		if !info.Mode().IsRegular() {
			return StatusConflict, nil
		}
		if same, err := sameContent(target, sourceAbs); err != nil || !same {
			if isSubsetOwned(entry.Ownership) && metadataMatchesEntry(meta, target, entry.Source, entry.Strategy) {
				if entry.Ownership == "json-subset" {
					relation, relationErr := configsubset.AnalyzeJSONFiles(target, sourceAbs)
					if relationErr != nil {
						return "", relationErr
					}
					if relation.Contains {
						return StatusUnchanged, nil
					}
					if relation.Mergeable {
						return StatusUpdate, nil
					}
				} else {
					subset, subsetErr := subsetContent(entry.Ownership, target, sourceAbs)
					if subsetErr != nil {
						return "", subsetErr
					}
					if subset {
						return StatusUnchanged, nil
					}
				}
			}
			if entry.Ownership == "toml-subset" && targetContainsCompatibleRecordedSource(entry, target, sourceRoot, meta, defaultSource) {
				return StatusUpdate, nil
			}
			return StatusConflict, nil
		}
		return StatusUnchanged, nil
	default:
		return StatusConflict, nil
	}
}

func metadataMatchesEntry(meta state.Metadata, target, source, strategy string) bool {
	rec, ok := meta.FindByTarget(target)
	return ok && rec.Source == source && rec.Strategy == strategy
}

func targetContainsCompatibleRecordedSource(entry manifest.Entry, target, sourceRoot string, meta state.Metadata, defaultSource string) bool {
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Strategy != entry.Strategy || !compatibleEntrySource(entry, defaultSource, rec.Source) {
		return false
	}
	sourceAbs, err := ResolveSource(rec.Source, sourceRoot)
	if err != nil {
		return false
	}
	if err := ValidateResolvedSource(sourceAbs, sourceRoot); err != nil {
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

func safeSourceExists(sourceAbs, sourceRoot string) (bool, error) {
	if err := ValidateResolvedSource(sourceAbs, sourceRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ValidateResolvedSource verifies that an already-resolved source path exists
// and resolves through symlinks to a file contained by sourceRoot.
func ValidateResolvedSource(sourceAbs, sourceRoot string) error {
	sourceReal, err := filepath.EvalSymlinks(sourceAbs)
	if err != nil {
		return fmt.Errorf("resolve source %q: %w", sourceAbs, err)
	}
	rootReal, err := filepath.EvalSymlinks(sourceRoot)
	if err != nil {
		return fmt.Errorf("resolve source root %q: %w", sourceRoot, err)
	}
	sourceReal = filepath.Clean(sourceReal)
	rootReal = filepath.Clean(rootReal)
	if !InsideRoot(sourceReal, rootReal) {
		return fmt.Errorf("unsafe source %q: symlink resolves outside source root %q", sourceAbs, sourceRoot)
	}
	return nil
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

func isSubsetOwned(ownership string) bool {
	return ownership == "json-subset" || ownership == "toml-subset"
}

func subsetContent(ownership, target, sourceAbs string) (bool, error) {
	switch ownership {
	case "json-subset":
		return configsubset.JSONFileContains(target, sourceAbs)
	case "toml-subset":
		return configsubset.TOMLFileContains(target, sourceAbs)
	default:
		return false, nil
	}
}
