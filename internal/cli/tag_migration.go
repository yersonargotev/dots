package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
)

const legacyTagMigrationRequiredCode = "legacy-tag-migration-required"

type legacyTagMigrationRemediation struct {
	RecommendedCommand string `json:"recommended_command"`
}

type legacyTagMigrationErrorData struct {
	Code          string                        `json:"code"`
	TagMigrations []manifest.TagReplacement     `json:"tag_migrations"`
	Remediation   legacyTagMigrationRemediation `json:"remediation"`
}

type legacyTagMigrationRequiredError struct {
	migrations         []manifest.TagReplacement
	recommendedCommand string
}

func (e *legacyTagMigrationRequiredError) Error() string {
	return legacyTagMigrationRequiredCode +
		": recorded legacy Tag intent requires confirmation; use remediation command " + e.recommendedCommand
}

func (e *legacyTagMigrationRequiredError) JSONErrorData() any {
	return legacyTagMigrationErrorData{
		Code:          legacyTagMigrationRequiredCode,
		TagMigrations: e.migrations,
		Remediation: legacyTagMigrationRemediation{
			RecommendedCommand: e.recommendedCommand,
		},
	}
}

func guardRecordedTagMigration(cmd *cobra.Command, effective selection.Effective, nonInteractive bool) (selection.Effective, error) {
	if effective.Report.Source != selection.SourceRecorded || len(effective.Report.TagMigrations) == 0 {
		return effective, nil
	}
	required := &legacyTagMigrationRequiredError{
		migrations:         effective.Report.TagMigrations,
		recommendedCommand: explicitSelectionCommand(cmd, effective),
	}
	if effective.Report.Delta != nil {
		required.recommendedCommand = cmd.Root().Name() + " " + commandName(cmd)
	}
	if nonInteractive || wantsJSON(cmd) {
		return selection.Effective{}, required
	}

	renderTagMigrations(cmd.OutOrStdout(), effective.Report.TagMigrations)
	confirmed, err := confirmRecordedTagMigration(cmd.InOrStdin(), cmd.OutOrStdout())
	if err != nil {
		return selection.Effective{}, err
	}
	if !confirmed {
		return selection.Effective{}, required
	}
	effective.Report.Source = selection.SourceMigration
	return effective, nil
}

func explicitSelectionCommand(cmd *cobra.Command, effective selection.Effective) string {
	return recommendedSelectionCommand(cmd.Root().Name()+" "+commandName(cmd), effective)
}

func recommendedSelectionCommand(command string, effective selection.Effective) string {
	parts := strings.Fields(command)
	for _, profile := range effective.Profiles {
		parts = append(parts, "--profile", profile)
	}
	for _, tag := range effective.ExtraTags {
		parts = append(parts, "--tag", tag)
	}
	return strings.Join(parts, " ")
}

func renderTagMigrations(w io.Writer, migrations []manifest.TagReplacement) {
	for _, migration := range migrations {
		fmt.Fprintf(w, "Legacy Tag normalization: %s -> %s\n", migration.LegacyTag, strings.Join(migration.ReplacementTags, ","))
		fmt.Fprintf(w, "Warning: Tag %q is a transitional alias; use its current replacement Tags.\n", migration.LegacyTag)
	}
	if len(migrations) > 0 {
		fmt.Fprintln(w)
	}
}

func confirmRecordedTagMigration(r io.Reader, w io.Writer) (bool, error) {
	fmt.Fprint(w, "Migrate the recorded legacy Tag intent before continuing? [y/N] ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, fmt.Errorf("read legacy Tag migration confirmation: %w", err)
		}
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
