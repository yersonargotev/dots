package cli

import (
	"fmt"
	"io"

	"strings"

	"github.com/yersonargotev/dots/internal/doctor"
	"github.com/yersonargotev/dots/internal/status"
)

func renderDoctor(w io.Writer, report doctor.Report) {
	fmt.Fprintf(w, "Doctor for %s\n\n", renderProfileSelection(report.Profile, report.Profiles))

	if report.Platform.Supported {
		fmt.Fprintf(w, "Platform: ok (%s)\n", report.Platform.OS)
	} else {
		fmt.Fprintf(w, "Platform: warn (%s is unsupported)\n", report.Platform.OS)
	}

	renderDoctorDependencies(w, report)
	renderDoctorConfiguration(w, report)
	renderDoctorProvisioners(w, report)
	renderDoctorSecrets(w, report)
}

// renderDoctorProvisioners reports the readiness of each declared provisioner:
// whether its dependencies are present so it could run. It never executes the
// tool. A provisioner is "not ready" when any declared dependency is missing.
func renderDoctorProvisioners(w io.Writer, report doctor.Report) {
	items := report.Provisioners.Items
	if len(items) == 0 {
		fmt.Fprintln(w, "Provisioners: ok (no provisioners declared)")
		return
	}

	var notReady int
	for _, item := range items {
		if len(item.Missing) > 0 {
			notReady++
		}
	}
	if notReady == 0 {
		fmt.Fprintf(w, "Provisioners: ok (%d ready, 0 not ready)\n", len(items))
		return
	}

	fmt.Fprintf(w, "Provisioners: warn (%d not ready)\n", notReady)
	for _, item := range items {
		if len(item.Missing) == 0 {
			continue
		}
		fmt.Fprintf(w, "  not ready: %s %s (missing: %s)\n",
			item.Executable, strings.Join(item.Args, " "), strings.Join(item.Missing, ", "))
	}
}

func renderDoctorDependencies(w io.Writer, report doctor.Report) {
	if len(report.Dependencies.Results) == 0 {
		fmt.Fprintln(w, "Dependencies: ok (no dependencies declared)")
		return
	}

	var missing int
	var warnings int
	for _, result := range report.Dependencies.Results {
		if !result.Present {
			missing++
		}
		if result.Warning != "" {
			warnings++
		}
	}
	if missing == 0 && warnings == 0 {
		fmt.Fprintf(w, "Dependencies: ok (%d present, 0 missing)\n", len(report.Dependencies.Results))
		return
	}

	if warnings == 0 {
		fmt.Fprintf(w, "Dependencies: warn (%d missing)\n", missing)
	} else {
		fmt.Fprintf(w, "Dependencies: warn (%d missing, %d warnings)\n", missing, warnings)
	}
	for _, result := range report.Dependencies.Results {
		if !result.Present {
			fmt.Fprintf(w, "  missing dependency: %s\n", result.Name)
		}
		if result.Warning != "" {
			fmt.Fprintf(w, "  warning: %s: %s\n", result.Name, result.Warning)
			if result.ProbeDetail != "" {
				fmt.Fprintf(w, "    detail: %s\n", result.ProbeDetail)
			}
			if result.Hint != "" {
				fmt.Fprintf(w, "    hint: %s\n", result.Hint)
			}
		}
	}
}

func renderDoctorConfiguration(w io.Writer, report doctor.Report) {
	var ok, concerns int
	for _, entry := range report.Configuration.Entries {
		switch entry.State {
		case status.StateOK, status.StateSkipped:
			ok++
		default:
			concerns++
		}
	}

	if concerns == 0 {
		fmt.Fprintf(w, "Configuration: ok (%d ok, 0 concerns)\n", ok)
		return
	}

	fmt.Fprintf(w, "Configuration: warn (%d concerns)\n", concerns)
	for _, entry := range report.Configuration.Entries {
		switch entry.State {
		case status.StateOK, status.StateSkipped:
			continue
		default:
			fmt.Fprintf(w, "  %s: %s -> %s\n", entry.State, entry.Source, entry.Target)
		}
	}
}

func renderDoctorSecrets(w io.Writer, report doctor.Report) {
	if len(report.SecretScan.Findings) == 0 {
		fmt.Fprintln(w, "Secret Scan: ok (0 findings)")
	} else {
		fmt.Fprintf(w, "Secret Scan: warn (%d findings)\n", len(report.SecretScan.Findings))
		for _, finding := range report.SecretScan.Findings {
			fmt.Fprintf(w, "  %s:%d %s\n", finding.Source, finding.Line, finding.Pattern)
		}
	}
	fmt.Fprintln(w, "Guardrail: Secret Scan catches obvious credential and private-key patterns only; it is not proof this repository is safe.")
}
