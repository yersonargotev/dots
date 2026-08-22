package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/selectedsurface"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/selectionreconciliation"
	"github.com/yersonargotev/dots/internal/state"
)

func buildSelectionReconciliation(m manifest.Manifest, meta state.Metadata, effective selection.Effective, installPlan plan.Plan, hostOS string, paths resolvedPaths, sourceReadRoot string) (*selectionreconciliation.Report, error) {
	installed := meta.InstalledSelection
	if installed == nil {
		return nil, nil
	}
	if sourceReadRoot == "" {
		sourceReadRoot = paths.SourceRoot
	}

	previousSurface := selectedsurface.Evaluate(m, installed.ResolvedTags, hostOS)
	currentSurface := selectedsurface.Evaluate(m, effective.Selection.Tags, hostOS)
	evidence, err := inspectSelectionReconciliation(previousSurface, currentSurface, installPlan, paths, sourceReadRoot)
	if err != nil {
		return nil, fmt.Errorf("inspect selection reconciliation: %w", err)
	}

	authority := selectionreconciliation.AuthorityManifestEvolution
	if effective.Report.Source == selection.SourceExplicit {
		authority = selectionreconciliation.AuthorityExplicitRequest
	}
	report, err := selectionreconciliation.Build(selectionreconciliation.Input{
		PreviousIntent: selectionreconciliation.Intent{
			Authority:    selectionreconciliation.AuthorityRecorded,
			Profiles:     append([]string(nil), installed.Profiles...),
			ExtraTags:    append([]string(nil), installed.ExtraTags...),
			ResolvedTags: append([]string(nil), installed.ResolvedTags...),
		},
		RequestedIntent: selectionreconciliation.Intent{
			Authority:    authority,
			Profiles:     append([]string(nil), effective.Profiles...),
			ExtraTags:    append([]string(nil), effective.ExtraTags...),
			ResolvedTags: append([]string(nil), effective.Selection.Tags...),
		},
		PreviousSurface: previousSurface,
		CurrentSurface:  currentSurface,
		Metadata:        meta,
		Evidence:        evidence,
	})
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func inspectSelectionReconciliation(previous, current selectedsurface.Surface, installPlan plan.Plan, paths resolvedPaths, sourceReadRoot string) (evidence selectionreconciliation.Evidence, err error) {
	paths.Home, err = filepath.Abs(paths.Home)
	if err != nil {
		return selectionreconciliation.Evidence{}, fmt.Errorf("resolve reconciliation home: %w", err)
	}
	paths.SourceRoot, err = filepath.Abs(paths.SourceRoot)
	if err != nil {
		return selectionreconciliation.Evidence{}, fmt.Errorf("resolve reconciliation source root: %w", err)
	}
	sourceReadRoot, err = filepath.Abs(sourceReadRoot)
	if err != nil {
		return selectionreconciliation.Evidence{}, fmt.Errorf("resolve reconciliation source read root: %w", err)
	}
	homeRoot, err := os.OpenRoot(paths.Home)
	if err != nil {
		return selectionreconciliation.Evidence{}, fmt.Errorf("open target home root: %w", err)
	}
	defer func() {
		if closeErr := homeRoot.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close target home root: %w", closeErr))
		}
	}()
	sourceRoot, err := os.OpenRoot(sourceReadRoot)
	if err != nil {
		return selectionreconciliation.Evidence{}, fmt.Errorf("open source read root: %w", err)
	}
	defer func() {
		if closeErr := sourceRoot.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close source read root: %w", closeErr))
		}
	}()

	evidence = selectionreconciliation.Evidence{
		Targets: make([]selectionreconciliation.TargetEvidence, 0),
		Sources: make([]selectionreconciliation.SourceEvidence, 0),
	}
	entries := orderedReconciliationEntries(current.Entries, previous.Entries)
	seenTargets := make(map[string]bool)
	seenSources := make(map[string]bool)
	for _, selected := range entries {
		declarativeTarget := selected.Entry.Target
		if !seenTargets[declarativeTarget] {
			target, inspectErr := inspectReconciliationTarget(selected.Entry, declarativeTarget, installPlan, paths, homeRoot)
			if inspectErr != nil {
				return selectionreconciliation.Evidence{}, inspectErr
			}
			evidence.Targets = append(evidence.Targets, target)
			seenTargets[declarativeTarget] = true
		}

		sourceKey := declarativeTarget + "\x00" + selected.Source
		if seenSources[sourceKey] {
			continue
		}
		source, inspectErr := inspectReconciliationSource(declarativeTarget, selected.Source, paths.SourceRoot, sourceReadRoot, sourceRoot)
		if inspectErr != nil {
			return selectionreconciliation.Evidence{}, inspectErr
		}
		evidence.Sources = append(evidence.Sources, source)
		seenSources[sourceKey] = true
	}
	return evidence, nil
}

func orderedReconciliationEntries(groups ...[]selectedsurface.SelectedEntry) []selectedsurface.SelectedEntry {
	result := make([]selectedsurface.SelectedEntry, 0)
	for _, entries := range groups {
		result = append(result, entries...)
	}
	return result
}

func inspectReconciliationTarget(entry manifest.Entry, declarativeTarget string, installPlan plan.Plan, paths resolvedPaths, root *os.Root) (selectionreconciliation.TargetEvidence, error) {
	resolved, err := plan.ResolveEntryTarget(entry, paths.Home, paths.XDGStateHome)
	if err != nil {
		return selectionreconciliation.TargetEvidence{}, fmt.Errorf("resolve reconciliation target %s: %w", declarativeTarget, err)
	}
	evidence := selectionreconciliation.TargetEvidence{
		DeclarativeTarget: declarativeTarget,
		ResolvedTarget:    resolved,
		ForwardStatus:     reconciliationForwardStatus(installPlan, resolved),
	}
	if err := plan.ValidateResolvedTarget(resolved, paths.Home); err != nil {
		evidence.Exists = true
		evidence.Kind = selectionreconciliation.TargetKindOther
		return evidence, nil
	}
	if err := plan.ValidateTargetParentInsideHome(resolved, paths.Home); err != nil {
		evidence.Exists = true
		evidence.Kind = selectionreconciliation.TargetKindOther
		return evidence, nil
	}
	relative, err := filepath.Rel(paths.Home, resolved)
	if err != nil || !filepath.IsLocal(relative) {
		evidence.Exists = true
		evidence.Kind = selectionreconciliation.TargetKindOther
		return evidence, nil
	}
	info, err := root.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return evidence, nil
	}
	if err != nil {
		evidence.Exists = true
		evidence.Kind = selectionreconciliation.TargetKindOther
		return evidence, nil
	}
	evidence.Exists = true
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		evidence.Kind = selectionreconciliation.TargetKindSymlink
		destination, readErr := os.Readlink(resolved)
		if readErr != nil {
			evidence.Kind = selectionreconciliation.TargetKindOther
			return evidence, nil
		}
		if current, statErr := root.Lstat(relative); statErr != nil || current.Mode()&os.ModeSymlink == 0 {
			evidence.Kind = selectionreconciliation.TargetKindOther
			return evidence, nil
		}
		if err := plan.ValidateTargetParentInsideHome(resolved, paths.Home); err != nil {
			evidence.Kind = selectionreconciliation.TargetKindOther
			return evidence, nil
		}
		evidence.LinkDestination = destination
	case info.Mode().IsRegular():
		evidence.Kind = selectionreconciliation.TargetKindRegular
		content, readErr := readReconciliationRootFile(root, relative)
		if readErr != nil {
			evidence.Kind = selectionreconciliation.TargetKindOther
			return evidence, nil
		}
		evidence.Content = content
	default:
		evidence.Kind = selectionreconciliation.TargetKindOther
	}
	return evidence, nil
}

func inspectReconciliationSource(declarativeTarget, source, canonicalRoot, readRoot string, root *os.Root) (selectionreconciliation.SourceEvidence, error) {
	canonical, err := plan.ResolveSource(source, canonicalRoot)
	if err != nil {
		return selectionreconciliation.SourceEvidence{}, fmt.Errorf("resolve canonical reconciliation source %s: %w", source, err)
	}
	inspected, err := plan.ResolveSource(source, readRoot)
	if err != nil {
		return selectionreconciliation.SourceEvidence{}, fmt.Errorf("resolve reconciliation source %s: %w", source, err)
	}
	evidence := selectionreconciliation.SourceEvidence{
		DeclarativeTarget: declarativeTarget,
		Source:            source,
		ResolvedSource:    canonical,
	}
	if err := plan.ValidateResolvedSource(inspected, readRoot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return evidence, nil
		}
		return evidence, nil
	}
	relative, err := filepath.Rel(readRoot, inspected)
	if err != nil || !filepath.IsLocal(relative) {
		return evidence, nil
	}
	content, err := readReconciliationRootFile(root, relative)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return evidence, nil
		}
		return evidence, nil
	}
	evidence.Exists = true
	evidence.Content = content
	return evidence, nil
}

func readReconciliationRootFile(root *os.Root, name string) (content []byte, err error) {
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close reconciliation file %s: %w", name, closeErr))
		}
	}()
	return io.ReadAll(file)
}

func reconciliationForwardStatus(installPlan plan.Plan, target string) selectionreconciliation.ForwardStatus {
	for _, action := range installPlan.Actions {
		if action.Target != target {
			continue
		}
		switch action.Status {
		case plan.StatusCreate:
			return selectionreconciliation.ForwardCreate
		case plan.StatusUpdate, plan.StatusMigrate:
			return selectionreconciliation.ForwardUpdate
		case plan.StatusUnchanged:
			return selectionreconciliation.ForwardUnchanged
		case plan.StatusConflict:
			return selectionreconciliation.ForwardConflict
		case plan.StatusMissingSource:
			return selectionreconciliation.ForwardMissingSource
		}
	}
	return ""
}
