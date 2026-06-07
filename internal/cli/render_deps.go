package cli

import (
	"fmt"
	"io"

	"github.com/yersonargotev/dots/internal/deps"
)

// renderDepsCheck writes a deterministic Dependency presence report so it can be
// locked with a golden test and read predictably by the user.
func renderDepsCheck(w io.Writer, report deps.CheckReport) {
	fmt.Fprintf(w, "Dependencies for profile %q\n\n", report.Profile)

	if len(report.Results) == 0 {
		fmt.Fprintln(w, "No dependencies declared for this profile.")
		return
	}

	var present, missing int
	for _, r := range report.Results {
		state := "missing"
		if r.Present {
			state = "present"
			present++
		} else {
			missing++
		}
		fmt.Fprintf(w, "  %-8s %s\n", state, r.Name)
	}

	fmt.Fprintf(w, "\nSummary: %d present, %d missing\n", present, missing)
}

// renderDepsPlan writes a deterministic, advisory Dependency Plan: OS-aware
// installation guidance for the missing Dependencies under the active Tier.
func renderDepsPlan(w io.Writer, report deps.PlanReport) {
	fmt.Fprintf(w, "Dependency plan for profile %q (%s)\n\n", report.Profile, report.Tier)

	if len(report.Items) == 0 {
		fmt.Fprintln(w, "All declared dependencies are already installed.")
		return
	}

	for _, item := range report.Items {
		hint := item.Command
		if hint == "" {
			hint = item.Manual
		}
		fmt.Fprintf(w, "  %-12s %s\n", item.Name, hint)
	}

	fmt.Fprintf(w, "\nSummary: %s\n", pluralizeDeps(len(report.Items)))
}

func pluralizeDeps(n int) string {
	if n == 1 {
		return "1 dependency to install"
	}
	return fmt.Sprintf("%d dependencies to install", n)
}
