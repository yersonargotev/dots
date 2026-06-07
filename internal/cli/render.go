package cli

import (
	"fmt"
	"io"

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
