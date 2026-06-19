package cli

import (
	"fmt"
	"io"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
)

// renderPlan writes a human-readable, deterministic preview of an Install Plan.
// The output is stable for a given Plan so it can be locked with a golden test
// and read predictably by the user during a dry run.
func renderPlan(w io.Writer, p plan.Plan) {
	fmt.Fprintf(w, "Plan for profile %q\n\n", p.Profile)

	if len(p.Actions) == 0 {
		fmt.Fprintln(w, "Nothing to do.")
		return
	}

	var counts struct {
		create, conflict, unchanged, missingSource int
	}
	for _, a := range p.Actions {
		fmt.Fprintf(w, "  %-15s %-9s %s -> %s\n", a.Status, a.Strategy, a.Source, a.Target)
		switch a.Status {
		case plan.StatusCreate:
			counts.create++
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

	fmt.Fprintf(w, "\nSummary: %d create, %d conflict, %d unchanged, %d missing-source\n",
		counts.create, counts.conflict, counts.unchanged, counts.missingSource)
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
func renderSkippedEntryHint(w io.Writer, m manifest.Manifest, profile, os string) error {
	hint, ok, err := plan.SkippedEntries(m, plan.Options{Profile: profile, OS: os})
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
	fmt.Fprintf(w, "\nNote: profile %q skips %d %s; run with --profile %s to include them.\n",
		hint.Profile, hint.Count, noun, hint.SuggestedProfile)
	return nil
}
