package cli

import (
	"fmt"
	"io"

	"github.com/yersonargotev/dots/internal/plan"
)

// renderUninstallPlan writes a deterministic preview of the Uninstall Plan: each
// recorded target with the action removing it would take, then a summary of how
// many fall into each category. The output is stable for golden testing and is
// what `dots uninstall --dry-run` shows without touching any file.
func renderUninstallPlan(w io.Writer, p plan.UninstallPlan) {
	fmt.Fprintln(w, "Uninstall Plan")
	fmt.Fprintln(w)

	var remove, modified, notOwned, skip, retain int
	for _, action := range p.Actions {
		fmt.Fprintf(w, "  %-10s %s\n", action.Status, action.Target)
		switch action.Status {
		case plan.UninstallRemove:
			remove++
		case plan.UninstallModified:
			modified++
		case plan.UninstallNotOwned:
			notOwned++
		case plan.UninstallSkip:
			skip++
		case plan.UninstallRetain:
			retain++
		}
	}

	if retain > 0 {
		fmt.Fprintf(w, "\nSummary: %d to remove, %d retained, %d modified, %d not-owned, %d skipped\n", remove, retain, modified, notOwned, skip)
	} else {
		fmt.Fprintf(w, "\nSummary: %d to remove, %d modified, %d not-owned, %d skipped\n", remove, modified, notOwned, skip)
	}
}
