package cli

import (
	"fmt"
	"io"

	"github.com/yersonargotev/dots/internal/doctor"
	"github.com/yersonargotev/dots/internal/status"
)

func renderDoctor(w io.Writer, report doctor.Report) {
	fmt.Fprintf(w, "Doctor for profile %q\n\n", report.Profile)

	if report.Platform.Supported {
		fmt.Fprintf(w, "Platform: ok (%s)\n", report.Platform.OS)
	} else {
		fmt.Fprintf(w, "Platform: warn (%s is unsupported)\n", report.Platform.OS)
	}

	renderDoctorDependencies(w, report)
	renderDoctorConfiguration(w, report)
	renderDoctorSecrets(w, report)
}

func renderDoctorDependencies(w io.Writer, report doctor.Report) {
	if len(report.Dependencies.Results) == 0 {
		fmt.Fprintln(w, "Dependencies: ok (no dependencies declared)")
		return
	}

	var missing int
	for _, result := range report.Dependencies.Results {
		if !result.Present {
			missing++
		}
	}
	if missing == 0 {
		fmt.Fprintf(w, "Dependencies: ok (%d present, 0 missing)\n", len(report.Dependencies.Results))
		return
	}

	fmt.Fprintf(w, "Dependencies: warn (%d missing)\n", missing)
	for _, result := range report.Dependencies.Results {
		if !result.Present {
			fmt.Fprintf(w, "  missing dependency: %s\n", result.Name)
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
