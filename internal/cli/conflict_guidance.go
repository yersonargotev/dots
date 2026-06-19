package cli

import (
	"fmt"
	"io"
)

// renderConflictResolutionGuidance teaches the safe next action for unresolved
// conflicts without changing the safety model: unattended install/update keeps
// conflicts skipped, while interactive paths require an explicit choice.
func renderConflictResolutionGuidance(w io.Writer) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Conflict guidance:")
	fmt.Fprintln(w, "  Conflicts mean this profile is only partially managed until you choose a resolution.")
	fmt.Fprintln(w, "  - skip keeps the local file untouched; dots leaves that target unmanaged for this run.")
	fmt.Fprintln(w, "  - replace creates a Backup Set before installing the Source of Truth.")
	fmt.Fprintln(w, "  - adopt is only for supported regular-file conflicts; it copies local file content into the Source of Truth after you choose it.")
	fmt.Fprintln(w, "  - diff previews target versus source before you decide.")
	fmt.Fprintln(w, "  Non-interactive install/update keeps conflicts skipped by default.")
}
