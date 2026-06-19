package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/status"
)

// renderStatus writes a human-readable, deterministic Dotfiles Status report.
// The output is stable for a given Report so it can be locked with a golden test
// and read predictably by the user.
func renderStatus(w io.Writer, report status.Report) {
	fmt.Fprintf(w, "Status for profile %q\n\n", report.Profile)

	if len(report.Entries) == 0 {
		fmt.Fprintln(w, "No managed entries for this profile.")
		return
	}

	var counts struct {
		ok, missing, conflict, skipped, drifted, unsupported int
	}
	for _, e := range report.Entries {
		fmt.Fprintf(w, "  %-12s %-9s %s -> %s\n", e.State, e.Strategy, e.Source, e.Target)
		switch e.State {
		case status.StateOK:
			counts.ok++
		case status.StateMissing:
			counts.missing++
		case status.StateConflict:
			counts.conflict++
		case status.StateSkipped:
			counts.skipped++
		case status.StateDrifted:
			counts.drifted++
		case status.StateUnsupported:
			counts.unsupported++
		default:
			// An unrecognized state is still printed above but intentionally left
			// out of the summary counts. If status.State gains a new value, it must
			// be added here or the summary will silently undercount.
		}
	}

	fmt.Fprintf(w, "\nSummary: %d ok, %d missing, %d conflict, %d skipped, %d drifted, %d unsupported\n",
		counts.ok, counts.missing, counts.conflict, counts.skipped, counts.drifted, counts.unsupported)
	if counts.conflict > 0 {
		renderConflictResolutionGuidance(w)
	}
}

// renderStatusProvisioners lists the provisioners declared for the profile as
// read-only, informational context. It deliberately makes no alignment claim:
// whether a provisioner has actually been applied is verified by the tool
// itself, not by dots. It renders nothing when no provisioner is declared.
func renderStatusProvisioners(w io.Writer, p provision.Plan) {
	if len(p.Steps) == 0 {
		return
	}

	fmt.Fprintf(w, "\nDeclared provisioners for profile %q\n\n", p.Profile)
	for _, step := range p.Steps {
		fmt.Fprintf(w, "  %s %s\n", step.Executable, strings.Join(step.Args, " "))
		if len(step.Targets) > 0 {
			fmt.Fprintf(w, "    affects: %s\n", strings.Join(step.Targets, ", "))
		}
	}
}
