package cli

import (
	"fmt"
	"runtime"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/selectionmigration"
	"github.com/yersonargotev/dots/internal/state"
)

const selectionMigrationRequiredCode = "selection-migration-required"

type selectionMigrationCandidate struct {
	Profiles           []string `json:"profiles"`
	ExtraTags          []string `json:"extra_tags"`
	EffectiveTags      []string `json:"effective_tags"`
	Confidence         string   `json:"confidence"`
	AmbiguityReasons   []string `json:"ambiguity_reasons"`
	RecommendedCommand string   `json:"recommended_command,omitempty"`
}

type selectionMigrationRemediation struct {
	RecommendedCommand string `json:"recommended_command,omitempty"`
}

type selectionMigrationErrorData struct {
	Code        string                        `json:"code"`
	Candidate   *selectionMigrationCandidate  `json:"candidate"`
	Remediation selectionMigrationRemediation `json:"remediation"`
}

type selectionMigrationRequiredError struct {
	candidate *selectionMigrationCandidate
}

func (e *selectionMigrationRequiredError) Error() string {
	return fmt.Sprintf("%s: Installation Metadata predates authoritative Installed Selection; choose an explicit selection", selectionMigrationRequiredCode)
}

func (e *selectionMigrationRequiredError) JSONErrorData() any {
	command := ""
	if e.candidate != nil {
		command = e.candidate.RecommendedCommand
	}
	return selectionMigrationErrorData{
		Code:      selectionMigrationRequiredCode,
		Candidate: e.candidate,
		Remediation: selectionMigrationRemediation{
			RecommendedCommand: command,
		},
	}
}

type readOnlySelectionOptions struct {
	Home       string
	SourceRoot string
	StatePath  string
}

func resolveReadOnlySelection(m manifest.Manifest, meta state.Metadata, profiles, extraTags []string, opts readOnlySelectionOptions) (selection.Effective, error) {
	// Explicit intent and an authoritative Installed Selection retain the
	// existing resolution path. In particular, malformed explicit values must
	// still be reported by selection.ResolveReadOnly.
	if len(profiles) > 0 || len(extraTags) > 0 || meta.InstalledSelection != nil {
		return selection.ResolveReadOnly(m, profiles, extraTags, meta.InstalledSelection)
	}
	if meta.Version != 1 && meta.Version != 2 {
		return selection.ResolveReadOnly(m, profiles, extraTags, nil)
	}

	analysis, err := selectionmigration.Analyze(m, meta, selectionmigration.Options{
		OS:         runtime.GOOS,
		Home:       opts.Home,
		SourceRoot: opts.SourceRoot,
		StatePath:  opts.StatePath,
	})
	if err != nil {
		return selection.Effective{}, err
	}
	return selection.Effective{}, &selectionMigrationRequiredError{
		candidate: migrationCandidate(analysis.Candidate),
	}
}

func migrationCandidate(candidate *selectionmigration.Candidate) *selectionMigrationCandidate {
	if candidate == nil {
		return nil
	}
	return &selectionMigrationCandidate{
		Profiles:           append([]string{}, candidate.Profiles...),
		ExtraTags:          append([]string{}, candidate.ExtraTags...),
		EffectiveTags:      append([]string{}, candidate.EffectiveTags...),
		Confidence:         candidate.Confidence,
		AmbiguityReasons:   append([]string{}, candidate.AmbiguityReasons...),
		RecommendedCommand: candidate.RecommendedCommand,
	}
}
