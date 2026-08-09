package cli

import (
	"bufio"
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/selectionmigration"
	"github.com/yersonargotev/dots/internal/state"
)

func resolveLegacyUpdateSelection(cmd *cobra.Command, m manifest.Manifest, meta state.Metadata, paths resolvedPaths, opts updateOptions) (selection.Effective, error) {
	analysis, err := selectionmigration.Analyze(m, meta, selectionmigration.Options{
		OS:           runtime.GOOS,
		Home:         paths.Home,
		SourceRoot:   paths.SourceRoot,
		StatePath:    state.Path(paths.StateRoot),
		XDGStateHome: paths.XDGStateHome,
	})
	if err != nil {
		return selection.Effective{}, err
	}
	required := &selectionMigrationRequiredError{candidate: analysis.Candidate}
	if analysis.Candidate == nil || !analysis.Candidate.Unambiguous() || opts.yes || opts.dryRun || wantsJSON(cmd) {
		return selection.Effective{}, required
	}

	renderMigrationCandidate(cmd.OutOrStdout(), *analysis.Candidate)
	confirmed, err := confirmSelectionMigration(cmd)
	if err != nil {
		return selection.Effective{}, err
	}
	if !confirmed {
		return selection.Effective{}, required
	}
	return selection.ResolveIntent(m, selection.Intent{
		Source:    selection.SourceMigration,
		Profiles:  analysis.Candidate.Profiles,
		ExtraTags: analysis.Candidate.ExtraTags,
	})
}

func renderMigrationCandidate(w interface{ Write([]byte) (int, error) }, candidate selectionmigration.Candidate) {
	fmt.Fprintln(w, "Legacy Installation Metadata requires an authoritative Installed Selection.")
	fmt.Fprintf(w, "Migration candidate: profiles=%s extra-tags=%s effective-tags=%s confidence=%s\n",
		renderSelectionValues(candidate.Profiles), renderSelectionValues(candidate.ExtraTags),
		renderSelectionValues(candidate.EffectiveTags), candidate.Confidence)
	if candidate.RecommendedCommand != "" {
		fmt.Fprintf(w, "Explicit alternative: %s\n", candidate.RecommendedCommand)
	}
}

func confirmSelectionMigration(cmd *cobra.Command) (bool, error) {
	fmt.Fprint(cmd.OutOrStdout(), "Confirm this migration candidate before update/upgrade? [y/N] ")
	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
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
