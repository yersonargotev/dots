package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/selection"
)

// renderPlan writes a human-readable, deterministic preview of an Install Plan.
// The output is stable for a given Plan so it can be locked with a golden test
// and read predictably by the user during a dry run.
func renderPlan(w io.Writer, p plan.Plan) {
	fmt.Fprintf(w, "Plan for %s\n\n", renderEffectiveSelection(p.Profile, p.Profiles, p.Tags, p.Selection))

	if len(p.Actions) == 0 {
		fmt.Fprintln(w, "Nothing to do.")
		return
	}

	var counts struct {
		create, update, conflict, unchanged, missingSource int
	}
	for _, a := range p.Actions {
		fmt.Fprintf(w, "  %-15s %-9s %s -> %s\n", a.Status, a.Strategy, a.Source, a.Target)
		switch a.Status {
		case plan.StatusCreate:
			counts.create++
		case plan.StatusUpdate:
			counts.update++
		case plan.StatusConflict:
			counts.conflict++
		case plan.StatusUnchanged:
			counts.unchanged++
		case plan.StatusMissingSource:
			counts.missingSource++
		default:
			// An unrecognized status is still printed above but intentionally
			// left out of the summary counts. If plan.Status gains a new value,
			// it must be added here or the summary will silently undercount.
		}
	}

	if counts.update > 0 {
		fmt.Fprintf(w, "\nSummary: %d create, %d update, %d conflict, %d unchanged, %d missing-source\n",
			counts.create, counts.update, counts.conflict, counts.unchanged, counts.missingSource)
	} else {
		fmt.Fprintf(w, "\nSummary: %d create, %d conflict, %d unchanged, %d missing-source\n",
			counts.create, counts.conflict, counts.unchanged, counts.missingSource)
	}
	if counts.conflict > 0 {
		renderConflictResolutionGuidance(w)
	}
}

// renderSkippedEntryHint prints a one-line nudge when the active profile omits
// file entries that another profile would select on this OS, so a default-profile
// user discovers the fuller profile (e.g. the desktop-only Ghostty and Zed
// configs) instead of silently missing them. It stays quiet when nothing is
// skipped, keeping the fuller profile's output noise-free. The profile is already
// validated upstream by plan.Build, so an error here only signals a programming
// mistake and is surfaced rather than swallowed.
//
// It is the file-entry twin of renderSkippedProvisionerHint: same "Note: profile
// %q skips N ...; run with --profile %s to include them." wording, and
// SuggestedProfile is rendered with %s (an unquoted, copy-pasteable --profile
// argument) while the descriptive active profile uses %q. The noun is pluralized
// because "file entry"/"file entries" does not read well as "entry(s)". Together
// the two hints close the profile-scope discoverability gap for both surfaces
// (issue #87, following #85).
func renderSkippedEntryHint(w io.Writer, m manifest.Manifest, profiles []string, os string) error {
	hint, ok, err := plan.SkippedEntries(m, plan.Options{Profiles: profiles, OS: os})
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	noun := "file entries"
	if hint.Count == 1 {
		noun = "file entry"
	}
	fmt.Fprintf(w, "\nNote: %s skips %d %s; run with %s to include them.\n",
		renderProfileSelection(hint.Profile, hint.Profiles, nil), hint.Count, noun, renderProfileFlags(hint.SuggestedProfiles))
	return nil
}

func renderProfileSelection(profile string, profiles []string, tags []string) string {
	var selection string
	if len(profiles) == 0 && profile == "" {
		selection = "tags only"
	} else if len(profiles) == 0 {
		selection = fmt.Sprintf("profile %q", profile)
	} else if len(profiles) == 1 {
		selection = fmt.Sprintf("profile %q", profiles[0])
	} else {
		quoted := make([]string, 0, len(profiles))
		for _, name := range profiles {
			quoted = append(quoted, fmt.Sprintf("%q", name))
		}
		selection = "profiles " + strings.Join(quoted, ", ")
	}
	if len(tags) == 0 {
		return selection
	}
	return selection + " (tags: " + strings.Join(tags, ", ") + ")"
}

func renderEffectiveSelection(profile string, profiles, tags []string, report *selection.Report) string {
	rendered := renderProfileSelection(profile, profiles, tags)
	if report == nil {
		return rendered
	}
	return fmt.Sprintf("%s [selection: %s]", rendered, report.Source)
}

func renderSelectionReport(w io.Writer, report selection.Report) {
	fmt.Fprintf(w, "Selection: source=%s profiles=%s extra-tags=%s effective-tags=%s\n\n",
		report.Source, renderSelectionValues(report.Profiles), renderSelectionValues(report.ExtraTags), renderSelectionValues(report.EffectiveTags))
}

func renderSelectionValues(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ",")
}

func renderProfileFlags(profiles []string) string {
	if len(profiles) == 0 {
		return "--profile"
	}
	parts := make([]string, 0, len(profiles)*2)
	for _, profile := range profiles {
		parts = append(parts, "--profile", profile)
	}
	return strings.Join(parts, " ")
}
