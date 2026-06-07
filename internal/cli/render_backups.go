package cli

import (
	"fmt"
	"io"

	"github.com/yersonargotev/dots/internal/backups"
)

// renderBackupSets writes deterministic Backup Metadata history for CLI output.
func renderBackupSets(w io.Writer, meta backups.Metadata) {
	fmt.Fprintln(w, "Backup Sets")
	fmt.Fprintln(w)

	for i, set := range meta.Sets {
		if i > 0 {
			fmt.Fprintln(w)
		}
		if set.ID != "" {
			fmt.Fprintf(w, "  ID: %s\n", set.ID)
		}
		fmt.Fprintf(w, "  Created: %s\n", set.CreatedAt)
		fmt.Fprintf(w, "  Reason: %s\n", set.Reason)
		fmt.Fprintln(w, "  Protected targets:")
		for _, target := range set.Targets {
			fmt.Fprintf(w, "    - %s\n", target)
		}
	}

	fmt.Fprintf(w, "\nSummary: %s\n", pluralizeBackupSets(len(meta.Sets)))
}

func pluralizeBackupSets(n int) string {
	if n == 1 {
		return "1 Backup Set"
	}
	return fmt.Sprintf("%d Backup Sets", n)
}
