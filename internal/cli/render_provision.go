package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/yersonargotev/dots/internal/provision"
)

// renderProvisionPlan writes a deterministic preview of the Provisioner steps an
// install would run: the exact resolved command and the roots each tool affects.
// It renders nothing when no Provisioner is selected so manifests without a
// provisioners section stay quiet. The output is stable for golden testing and
// is what `dots install --dry-run` shows without invoking any tool.
func renderProvisionPlan(w io.Writer, p provision.Plan) {
	if len(p.Steps) == 0 {
		return
	}

	fmt.Fprintf(w, "\nProvisioners for profile %q\n\n", p.Profile)
	for _, step := range p.Steps {
		fmt.Fprintf(w, "  %s %s\n", step.Executable, strings.Join(step.Args, " "))
		if len(step.Targets) > 0 {
			fmt.Fprintf(w, "    affects: %s\n", strings.Join(step.Targets, ", "))
		}
	}
	fmt.Fprintf(w, "\nSummary: %d provisioner(s)\n", len(p.Steps))
}

// renderProvisionReport writes the actual execution outcomes for provisioners.
// It is deliberately emitted even when Apply returns an error so users can see
// which provisioners completed before the failing step stopped the run.
func renderProvisionReport(w io.Writer, r provision.Report) {
	if len(r.Items) == 0 {
		return
	}

	fmt.Fprintf(w, "\nProvisioner results for profile %q\n\n", r.Profile)
	for _, item := range r.Items {
		fmt.Fprintf(w, "  %s %s — %s", item.Executable, strings.Join(item.Args, " "), item.Status)
		if len(item.Missing) > 0 {
			fmt.Fprintf(w, " (missing: %s)", strings.Join(item.Missing, ", "))
		}
		fmt.Fprintln(w)
	}
}
