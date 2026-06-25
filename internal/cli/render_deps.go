package cli

import (
	"fmt"
	"io"
	"strings"

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
		if item.TrustCommand != "" {
			fmt.Fprintf(w, "  %-12s %s\n", "trust", item.TrustCommand)
		}
	}

	fmt.Fprintf(w, "\nSummary: %s\n", pluralizeDeps(len(report.Items)))
}

func pluralizeDeps(n int) string {
	if n == 1 {
		return "1 dependency to install"
	}
	return fmt.Sprintf("%d dependencies to install", n)
}

// renderDepsInstallDryRun writes a deterministic preview of dependency install
// actions without executing package managers.
func renderDepsInstallDryRun(w io.Writer, report deps.InstallDryRunReport) {
	renderDepsInstallPreviewWithTitle(w, "Dependency install dry-run", report)
}

func renderDepsInstallPreview(w io.Writer, report deps.InstallDryRunReport) {
	renderDepsInstallPreviewWithTitle(w, "Dependency install preview", report)
}

func renderDepsInstallPreviewWithTitle(w io.Writer, title string, report deps.InstallDryRunReport) {
	fmt.Fprintf(w, "%s for profile %q (%s)\n\n", title, report.Profile, report.Tier)

	if len(report.Items) == 0 {
		fmt.Fprintln(w, "All declared dependencies are already installed.")
		return
	}

	for _, item := range report.Items {
		hint := item.Manual
		if item.Executable != "" {
			parts := append([]string{item.Executable}, item.Args...)
			hint = strings.Join(parts, " ")
		}
		fmt.Fprintf(w, "  %-13s %-24s %s\n", item.Status, item.Dependency, hint)
		if item.TrustCommand != "" {
			fmt.Fprintf(w, "  %-13s %-24s %s\n", "trust", item.Dependency, item.TrustCommand)
		}
	}

	installable, manual := countInstallPreviewActions(report)
	fmt.Fprintf(w, "\nSummary: %d installable, %d manual\n", installable, manual)
}

func hasInstallablePreviewAction(report deps.InstallDryRunReport) bool {
	installable, _ := countInstallPreviewActions(report)
	return installable > 0
}

func countInstallPreviewActions(report deps.InstallDryRunReport) (installable int, manual int) {
	for _, item := range report.Items {
		if item.Executable == "" {
			manual++
			continue
		}
		installable++
	}
	return installable, manual
}

// renderDepsInstall writes the stable dots summary after real package-manager
// execution has already streamed through the command's stdio handles.
func renderDepsInstall(w io.Writer, report deps.InstallReport) {
	fmt.Fprintf(w, "\nDependency install for profile %q (%s)\n\n", report.Profile, report.Tier)

	if len(report.Items) == 0 {
		fmt.Fprintln(w, "All declared dependencies are already installed.")
		return
	}

	var installed, manual, unresolved, failed int
	for _, item := range report.Items {
		hint := item.Manual
		if item.Executable != "" {
			parts := append([]string{item.Executable}, item.Args...)
			hint = strings.Join(parts, " ")
		}
		fmt.Fprintf(w, "  %-10s %-24s %s\n", item.Status, item.Dependency, hint)
		if item.TrustCommand != "" {
			fmt.Fprintf(w, "  %-10s %-24s %s\n", "trust", item.Dependency, item.TrustCommand)
		}
		switch item.Status {
		case deps.InstallStatusInstalled:
			installed++
		case deps.InstallStatusManual:
			manual++
		case deps.InstallStatusUnresolved:
			unresolved++
		case deps.InstallStatusFailed:
			failed++
		}
	}

	fmt.Fprintf(w, "\nSummary: %d installed, %d manual, %d unresolved, %d failed\n", installed, manual, unresolved, failed)
}
