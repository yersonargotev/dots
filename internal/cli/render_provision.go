package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
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

// renderSkippedProvisionerHint prints a one-line nudge when the active profile
// omits provisioners that another profile would select on this OS, so a
// default-profile user discovers the fuller profile (e.g. the desktop-only
// chrome-devtools integration) instead of silently missing it. It stays quiet
// when nothing is skipped, keeping the fuller profile's output noise-free. The
// profile is already validated upstream by plan.Build, so an error here only
// signals a programming mistake and is surfaced rather than swallowed.
//
// Scope is deliberately provisioner-only: it targets the silent
// agent-provisioning gap from issue #85 (chrome-devtools for Claude, Codex, and
// the OpenCode overlay). File entries are profile-scoped the same way and are a
// candidate for the same nudge, but that is a separate surface and is out of
// scope here.
//
// SuggestedProfile is rendered with %s, not %q: it is a copy-pasteable shell
// argument the user types as `--profile desktop`, so it is intentionally
// unquoted while the descriptive active profile uses %q.
func renderSkippedProvisionerHint(w io.Writer, m manifest.Manifest, profile, os string) error {
	hint, ok, err := provision.SkippedProvisioners(m, provision.Options{Profile: profile, OS: os})
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	fmt.Fprintf(w, "\nNote: profile %q skips %d provisioner(s); run with --profile %s to include them.\n",
		hint.Profile, hint.Count, hint.SuggestedProfile)
	return nil
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
