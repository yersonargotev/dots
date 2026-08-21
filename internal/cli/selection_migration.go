package cli

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/selectionmigration"
	"github.com/yersonargotev/dots/internal/state"
)

const selectionMigrationRequiredCode = "selection-migration-required"

type selectionMigrationRemediation struct {
	RecommendedCommand string `json:"recommended_command,omitempty"`
}

type selectionMigrationErrorData struct {
	Code        string                        `json:"code"`
	Candidate   *selectionmigration.Candidate `json:"candidate"`
	Remediation selectionMigrationRemediation `json:"remediation"`
}

type selectionMigrationRequiredError struct {
	candidate *selectionmigration.Candidate
}

func (e *selectionMigrationRequiredError) Error() string {
	message := fmt.Sprintf("%s: Installation Metadata predates authoritative Installed Selection", selectionMigrationRequiredCode)
	if e.candidate != nil && e.candidate.RecommendedCommand != "" {
		return message + "; run " + e.candidate.RecommendedCommand
	}
	if e.candidate != nil && len(e.candidate.AmbiguityReasons) > 0 {
		message += "; candidate ambiguity: " + strings.Join(e.candidate.AmbiguityReasonStrings(), ", ")
	}
	return message + "; inspect `dots installed`, then provide the complete selection with repeated --profile and --tag flags"
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
	Home         string
	SourceRoot   string
	StatePath    string
	XDGStateHome string
}

func resolveReadOnlySelection(m manifest.Manifest, meta state.Metadata, profiles, extraTags []string, opts readOnlySelectionOptions) (selection.Effective, error) {
	// Explicit intent and an authoritative Installed Selection retain the
	// existing resolution path. In particular, malformed explicit values must
	// still be reported by selection.ResolveReadOnly.
	if len(profiles) > 0 || len(extraTags) > 0 || meta.InstalledSelection != nil {
		effective, err := selection.ResolveReadOnly(m, profiles, extraTags, meta.InstalledSelection)
		if err != nil {
			return selection.Effective{}, err
		}
		if effective.Report.Source == selection.SourceRecorded && len(effective.Report.TagMigrations) > 0 {
			return selection.Effective{}, &legacyTagMigrationRequiredError{
				migrations:         effective.Report.TagMigrations,
				recommendedCommand: recommendedSelectionCommand("dots install", effective),
			}
		}
		return effective, nil
	}
	if meta.Version != 1 && meta.Version != 2 {
		return selection.ResolveReadOnly(m, profiles, extraTags, nil)
	}

	analysis, err := selectionmigration.Analyze(m, meta, selectionmigration.Options{
		OS:           runtime.GOOS,
		Home:         opts.Home,
		SourceRoot:   opts.SourceRoot,
		StatePath:    opts.StatePath,
		XDGStateHome: opts.XDGStateHome,
	})
	if err != nil {
		return selection.Effective{}, err
	}
	return selection.Effective{}, &selectionMigrationRequiredError{
		candidate: analysis.Candidate,
	}
}
