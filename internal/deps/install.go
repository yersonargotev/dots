package deps

import "github.com/yersonargotev/dots/internal/manifest"

// InstallPreviewStatus describes what a dry-run install would do for one
// missing Dependency.
type InstallPreviewStatus string

const (
	InstallPreviewWouldInstall InstallPreviewStatus = "would-install"
	InstallPreviewManual       InstallPreviewStatus = "manual"
)

// InstallPreview is one dry-run installation preview item.
type InstallPreview struct {
	Dependency string
	Status     InstallPreviewStatus
	Package    string
	Executable string
	Args       []string
	Manual     string
}

// InstallDryRunReport previews the install actions for a Profile without
// invoking any package manager.
type InstallDryRunReport struct {
	Profile string
	Tier    Tier
	Items   []InstallPreview
}

// InstallDryRun computes the install preview for missing Dependencies without
// executing package managers.
func InstallDryRun(m manifest.Manifest, opts Options, look Lookup, tier Tier) (InstallDryRunReport, error) {
	plan, err := Plan(m, opts, look, tier)
	if err != nil {
		return InstallDryRunReport{}, err
	}

	report := InstallDryRunReport{Profile: plan.Profile, Tier: plan.Tier}
	for _, action := range plan.Actions {
		status := InstallPreviewWouldInstall
		if action.Executable == "" {
			status = InstallPreviewManual
		}
		report.Items = append(report.Items, InstallPreview{
			Dependency: action.Dependency,
			Status:     status,
			Package:    action.Package,
			Executable: action.Executable,
			Args:       action.Args,
			Manual:     action.Manual,
		})
	}
	return report, nil
}
