// Package repositoryrefresh captures provenance-backed workstation content
// before an Installed Repository transition can move the symlinked source.
package repositoryrefresh

import (
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
func CaptureLegacyTargets(oldManifest manifest.Manifest, meta state.Metadata, sourceRoot, home, oldRevision string) (map[string]plan.LegacyMigration, error) {
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

func provenanceMatches(provenance state.Provenance, sourceRoot, revision string) bool {
	if provenance.SourceRoot == "" || provenance.SourceRevision == "" || revision == "" {
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
	return strings.HasPrefix(revision, provenance.SourceRevision)
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
