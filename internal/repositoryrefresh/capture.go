// Package repositoryrefresh captures provenance-backed workstation content
// before an Installed Repository transition can move the symlinked source.
package repositoryrefresh

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/dots/internal/gitrepo"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

// CaptureLegacyTargets returns only unambiguous legacy symlinks backed by both
// Installation Metadata provenance and the old Install Manifest. Missing or
// stale evidence is deliberately omitted so the later Install Plan reports a
// Conflict instead of guessing ownership.
func CaptureLegacyTargets(oldManifest, newManifest manifest.Manifest, meta state.Metadata, sourceRoot, home, xdgStateHome, oldRevision string) (map[string]plan.LegacyMigration, error) {
	captures := map[string]plan.LegacyMigration{}
	if !provenanceMatches(meta.Provenance, sourceRoot, oldRevision) {
		return captures, nil
	}
	counts := map[string]int{}
	for _, rec := range meta.Entries {
		counts[filepath.Clean(rec.Target)]++
	}
	for _, rec := range meta.Entries {
		if rec.Strategy != "symlink" || counts[filepath.Clean(rec.Target)] != 1 {
			continue
		}
		target, source, ok, err := declaredLegacySymlink(oldManifest, rec, sourceRoot, home)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		info, err := os.Lstat(target)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		destination, err := os.Readlink(target)
		if err != nil || filepath.Clean(destination) != filepath.Clean(source) {
			continue
		}
		sourceInfo, err := os.Stat(source)
		if err != nil {
			continue
		}
		if sourceInfo.IsDir() {
			if err := captureSeededChildren(captures, newManifest, rec, target, source, destination, sourceRoot, home, xdgStateHome, oldRevision); err != nil {
				return nil, err
			}
			continue
		}
		content, err := os.ReadFile(source)
		if err != nil {
			continue
		}
		currentDestination, err := os.Readlink(target)
		if err != nil || filepath.Clean(currentDestination) != filepath.Clean(destination) {
			continue
		}
		previous, err := gitrepo.ReadFileAtRevision(sourceRoot, oldRevision, rec.Source)
		if err != nil {
			continue
		}
		captures[target] = plan.LegacyMigration{
			LinkDestination:       destination,
			CapturedContent:       append([]byte(nil), content...),
			PreviousSourceContent: append([]byte(nil), previous...),
		}
	}
	return captures, nil
}

func captureSeededChildren(captures map[string]plan.LegacyMigration, newManifest manifest.Manifest, rec state.Record, legacyTarget, legacySource, destination, sourceRoot, home, xdgStateHome, oldRevision string) error {
	legacyPrefix := strings.TrimSuffix(filepath.ToSlash(rec.Source), "/") + "/"
	for _, entry := range newManifest.Entries {
		if entry.Strategy != "copy" || entry.Ownership != "seeded" || !strings.HasPrefix(filepath.ToSlash(entry.Source), legacyPrefix) {
			continue
		}
		rel := strings.TrimPrefix(filepath.ToSlash(entry.Source), legacyPrefix)
		if rel == "" || !filepath.IsLocal(filepath.FromSlash(rel)) {
			continue
		}
		livePath := filepath.Join(legacySource, filepath.FromSlash(rel))
		content, err := os.ReadFile(livePath)
		if err != nil {
			continue
		}
		previous, err := gitrepo.ReadFileAtRevision(sourceRoot, oldRevision, entry.Source)
		if err != nil {
			continue
		}
		currentDestination, err := os.Readlink(legacyTarget)
		if err != nil || filepath.Clean(currentDestination) != filepath.Clean(destination) {
			continue
		}
		currentContent, err := os.ReadFile(livePath)
		if err != nil || !bytes.Equal(currentContent, content) {
			continue
		}
		resolvedTarget, err := plan.ResolveEntryTarget(entry, home, xdgStateHome)
		if err != nil {
			return err
		}
		if _, exists := captures[resolvedTarget]; exists {
			continue
		}
		captures[resolvedTarget] = plan.LegacyMigration{
			LinkDestination:       destination,
			LegacyTarget:          legacyTarget,
			LegacyContentTarget:   filepath.Join(legacyTarget, filepath.FromSlash(rel)),
			CapturedContent:       append([]byte(nil), content...),
			PreviousSourceContent: append([]byte(nil), previous...),
		}
	}
	return nil
}

func provenanceMatches(provenance state.Provenance, sourceRoot, revision string) bool {
	if provenance.SourceRoot == "" || len(provenance.SourceRevision) < 7 || revision == "" {
		return false
	}
	wantRoot, err := filepath.Abs(sourceRoot)
	if err != nil {
		return false
	}
	gotRoot, err := filepath.Abs(provenance.SourceRoot)
	if err != nil || filepath.Clean(gotRoot) != filepath.Clean(wantRoot) {
		return false
	}
	recordedRevision, err := gitrepo.ResolveRevision(sourceRoot, provenance.SourceRevision)
	if err != nil {
		return false
	}
	currentRevision, err := gitrepo.ResolveRevision(sourceRoot, revision)
	return err == nil && recordedRevision == currentRevision
}

func declaredLegacySymlink(m manifest.Manifest, rec state.Record, sourceRoot, home string) (target, source string, ok bool, err error) {
	for _, entry := range m.Entries {
		if entry.Strategy != "symlink" || !declaresSource(entry, rec.Source) {
			continue
		}
		resolvedTarget, resolveErr := plan.ResolveTarget(entry.Target, home)
		if resolveErr != nil {
			return "", "", false, resolveErr
		}
		if filepath.Clean(resolvedTarget) != filepath.Clean(rec.Target) {
			continue
		}
		resolvedSource, resolveErr := plan.ResolveSource(rec.Source, sourceRoot)
		if resolveErr != nil {
			return "", "", false, resolveErr
		}
		if validateErr := plan.ValidateResolvedSource(resolvedSource, sourceRoot); validateErr != nil {
			return "", "", false, fmt.Errorf("validate legacy source %s: %w", rec.Source, validateErr)
		}
		return resolvedTarget, resolvedSource, true, nil
	}
	return "", "", false, nil
}

func declaresSource(entry manifest.Entry, source string) bool {
	if entry.Source == source {
		return true
	}
	for _, candidate := range entry.SourceOverrides {
		if candidate == source {
			return true
		}
	}
	return false
}

// Describe returns a stable count for user-facing transition reporting.
func Describe(captures map[string]plan.LegacyMigration) string {
	return fmt.Sprintf("%d provenance-backed legacy target(s)", len(captures))
}
