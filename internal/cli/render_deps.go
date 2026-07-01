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
	fmt.Fprintf(w, "Dependencies for %s\n\n", renderProfileSelection(report.Profile, report.Profiles))

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
		fmt.Fprintf(w, "  %-8s %s (%s)\n", state, r.Name, dependencyRequirementLabel(r.Requirement))
	}

	fmt.Fprintf(w, "\nSummary: %d present, %d missing\n", present, missing)
}

// renderDepsPlan writes a deterministic, advisory Dependency Plan: OS-aware
// installation guidance for the missing Dependencies under the active Tier.
func renderDepsPlan(w io.Writer, report deps.PlanReport) {
	fmt.Fprintf(w, "Dependency plan for %s (%s)\n\n", renderProfileSelection(report.Profile, report.Profiles), report.Tier)

	if len(report.Items) == 0 {
		fmt.Fprintln(w, "All declared dependencies are already installed.")
		return
	}

	for _, item := range report.Items {
		hint := item.Command
		if hint == "" {
			hint = item.Manual
		}
		fmt.Fprintf(w, "  %-12s %s (%s)\n", item.Name, hint, dependencyRequirementLabel(item.Requirement))
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
	fmt.Fprintf(w, "%s for %s (%s)\n\n", title, renderProfileSelection(report.Profile, report.Profiles), report.Tier)

	if len(report.Items) == 0 {
		fmt.Fprintln(w, "All declared dependencies are already installed.")
		return
	}

	for _, item := range report.Items {
		hint := item.Manual
		if item.Status == deps.InstallPreviewWouldInstall && item.UserLocal != nil {
			hint = item.UserLocal.Hint()
		} else if item.Status == deps.InstallPreviewWouldInstall && item.Executable != "" {
			hint = commandHint(item.Executable, item.Args)
		} else if item.Status == deps.InstallPreviewWouldInstall && len(item.Bootstrap) > 0 {
			hint = "see bootstrap command(s)"
		}
		fmt.Fprintf(w, "  %-13s %-24s %s (%s)\n", item.Status, item.Dependency, hint, dependencyRequirementLabel(item.Requirement))
		for _, command := range item.Bootstrap {
			fmt.Fprintf(w, "  %-13s %-24s %s\n", "bootstrap", item.Dependency, commandHint(command.Executable, command.Args))
		}
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

func hasRequiredInstallablePreviewAction(report deps.InstallDryRunReport) bool {
	for _, item := range report.Items {
		if item.Status == deps.InstallPreviewWouldInstall && dependencyRequirementLabel(item.Requirement) == "required" {
			return true
		}
	}
	return false
}

func unresolvedInstallReportFromPreview(preview deps.InstallDryRunReport) (deps.InstallReport, error) {
	report := deps.InstallReport{Profile: preview.Profile, Profiles: preview.Profiles, Tags: preview.Tags, Tier: preview.Tier}
	requiredUnresolved := false
	for _, item := range preview.Items {
		status := deps.InstallStatusUnresolved
		if item.Status == deps.InstallPreviewManual {
			status = deps.InstallStatusManual
		}
		requirement := dependencyRequirementLabel(item.Requirement)
		if requirement == "required" {
			requiredUnresolved = true
		}
		report.Items = append(report.Items, deps.InstallItem{
			Dependency:   item.Dependency,
			Requirement:  requirement,
			Status:       status,
			Provider:     item.Provider,
			Package:      item.Package,
			Executable:   item.Executable,
			Args:         append([]string(nil), item.Args...),
			Bootstrap:    append([]deps.Command(nil), item.Bootstrap...),
			Manual:       item.Manual,
			TrustCommand: item.TrustCommand,
			Candidates:   append([]deps.ProviderCandidate(nil), item.Candidates...),
			UserLocal:    item.UserLocal,
		})
	}
	if requiredUnresolved {
		return report, fmt.Errorf("unresolved required dependencies remain after install")
	}
	return report, nil
}

func countInstallPreviewActions(report deps.InstallDryRunReport) (installable int, manual int) {
	for _, item := range report.Items {
		switch item.Status {
		case deps.InstallPreviewWouldInstall:
			installable++
		case deps.InstallPreviewManual:
			manual++
		}
	}
	return installable, manual
}

// renderDepsInstall writes the stable dots summary after real package-manager
// execution has already streamed through the command's stdio handles.
func renderDepsInstall(w io.Writer, report deps.InstallReport) {
	fmt.Fprintf(w, "\nDependency install for %s (%s)\n\n", renderProfileSelection(report.Profile, report.Profiles), report.Tier)

	if len(report.Items) == 0 {
		fmt.Fprintln(w, "All declared dependencies are already installed.")
		return
	}

	var installed, manual, unresolved, failed int
	for _, item := range report.Items {
		hint := item.Manual
		if item.UserLocal != nil {
			hint = item.UserLocal.Hint()
		} else if item.Executable != "" {
			hint = commandHint(item.Executable, item.Args)
		} else if len(item.Bootstrap) > 0 {
			hint = "see bootstrap command(s)"
		}
		fmt.Fprintf(w, "  %-10s %-24s %s (%s)\n", item.Status, item.Dependency, hint, dependencyRequirementLabel(item.Requirement))
		for _, command := range item.Bootstrap {
			fmt.Fprintf(w, "  %-10s %-24s %s\n", "bootstrap", item.Dependency, commandHint(command.Executable, command.Args))
		}
		if item.TrustCommand != "" {
			fmt.Fprintf(w, "  %-10s %-24s %s\n", "trust", item.Dependency, item.TrustCommand)
		}
		if item.Status == deps.InstallStatusUnresolved && item.Manual != "" {
			fmt.Fprintf(w, "  %-10s %-24s %s\n", "repair", item.Dependency, item.Manual)
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

func commandHint(executable string, args []string) string {
	parts := append([]string{executable}, args...)
	for i, part := range parts {
		parts[i] = shellQuoteIfNeeded(part)
	}
	return strings.Join(parts, " ")
}

func shellQuoteIfNeeded(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			strings.ContainsRune("_@%+=:,./-", r))
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func dependencyRequirementLabel(requirement string) string {
	if strings.TrimSpace(requirement) == "" {
		return "required"
	}
	return requirement
}
