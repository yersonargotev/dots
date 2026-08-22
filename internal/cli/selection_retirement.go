package cli

import (
	"fmt"
	"io"

	"github.com/yersonargotev/dots/internal/selectionretirement"
)

func renderSelectionRetirement(w io.Writer, result *selectionretirement.Result) {
	if result == nil || len(result.Removed)+len(result.Retained) == 0 {
		return
	}
	fmt.Fprintln(w, "Selection retirement:")
	for _, target := range result.Removed {
		fmt.Fprintf(w, "  removed %s and released dots ownership\n", target)
	}
	for _, target := range result.Retained {
		fmt.Fprintf(w, "  retained %s and released dots ownership\n", target)
	}
	fmt.Fprintln(w)
}
