package cli

import (
	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/bootstrap"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/gitrepo"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/status"
	"github.com/yersonargotev/dots/internal/uninstall"
	"github.com/yersonargotev/dots/internal/upgrade"
)

type initReport bootstrap.Result

type statusReport struct {
	Profile      string                 `json:"profile"`
	Entries      []status.Entry         `json:"entries"`
	Provisioners provision.StatusReport `json:"provisioners"`
}

func (r statusReport) HasFindings() bool {
	return status.Report{Profile: r.Profile, Entries: r.Entries}.HasFindings() || r.Provisioners.HasFindings()
}

type versionReport struct {
	Version string `json:"version"`
}

type manifestValidateReport struct {
	File  string `json:"file"`
	Valid bool   `json:"valid"`
}

type installReport struct {
	DryRun             bool                       `json:"dry_run"`
	Dependencies       *installDependenciesReport `json:"dependencies,omitempty"`
	Plan               plan.Plan                  `json:"plan"`
	Provisioners       provision.Plan             `json:"provisioners"`
	ProvisionerResults *provision.Report          `json:"provisioner_results,omitempty"`
}

type installDependenciesReport struct {
	Preview *deps.InstallDryRunReport `json:"preview,omitempty"`
	Result  *deps.InstallReport       `json:"result,omitempty"`
}

type updateReport struct {
	DryRun       bool           `json:"dry_run"`
	Update       gitrepo.Update `json:"update"`
	Plan         plan.Plan      `json:"plan"`
	Provisioners provision.Plan `json:"provisioners"`
}

type upgradeReport struct {
	DryRun       bool           `json:"dry_run"`
	Binary       upgrade.Plan   `json:"binary"`
	Update       gitrepo.Update `json:"update"`
	Plan         plan.Plan      `json:"plan"`
	Provisioners provision.Plan `json:"provisioners"`
}

type uninstallReport struct {
	DryRun    bool               `json:"dry_run"`
	StateRoot string             `json:"state_root"`
	Plan      plan.UninstallPlan `json:"plan"`
	Result    uninstall.Result   `json:"result,omitempty"`
}

type backupsListReport struct {
	StateRoot string           `json:"state_root"`
	Metadata  backups.Metadata `json:"metadata"`
}

type backupsRestoreReport struct {
	DryRun bool                  `json:"dry_run"`
	Set    backups.BackupSet     `json:"set"`
	Items  []backups.RestoreItem `json:"items"`
	Result backups.RestoreResult `json:"result,omitempty"`
}

type depsInstallDryRunReport struct {
	DryRun bool                     `json:"dry_run"`
	Report deps.InstallDryRunReport `json:"report"`
}

type depsInstallRunReport struct {
	DryRun bool               `json:"dry_run"`
	Report deps.InstallReport `json:"report"`
}
