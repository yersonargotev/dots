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

// renderStatusProvisioners lists the selected Provisioners with their persisted
// completion state. It renders nothing when no Provisioner is declared.
func renderStatusProvisioners(w io.Writer, r provision.StatusReport) {
	if len(r.Items) == 0 {
		return
	}

	fmt.Fprintf(w, "\nDeclared provisioners for profile %q — %s\n\n", r.Profile, r.Summary.State)
	for _, item := range r.Items {
		fmt.Fprintf(w, "  %-20s %s %s\n", item.Status, item.Executable, strings.Join(item.Args, " "))
		if len(item.Targets) > 0 {
			fmt.Fprintf(w, "    affects: %s\n", strings.Join(item.Targets, ", "))
		}
		if len(item.Missing) > 0 {
			fmt.Fprintf(w, "    missing: %s\n", strings.Join(item.Missing, ", "))
		}
	}
	if r.Summary.State == provision.SummaryStatePending || r.Summary.State == provision.SummaryStateFailed {
		fmt.Fprintln(w, "    resume: run dots install again after addressing failed or missing provisioners.")
	}
}
