package cli

import (
	"bytes"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/doctor"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/status"
)

// TestEnvelopeGolden locks the full JSON shape of the Agent Output Contract.
// Any rename, removal, or addition of an exposed domain field changes a golden
// file and must be a deliberate schema_version bump. The fixtures also prove the
// machine-local / advisory fields (resolved_source, probe_detail, hint) stay out
// of the contract.
func TestEnvelopeGolden(t *testing.T) {
	installPreview := deps.InstallDryRunReport{Profile: "default", Tier: deps.TierHomebrew, Items: []deps.InstallPreview{
		{Dependency: "starship", Requirement: "required", Status: deps.InstallPreviewWouldInstall, Provider: deps.TierHomebrew, Package: "starship", Executable: "brew", Args: []string{"install", "starship"}},
	}}
	installResult := deps.InstallReport{Profile: "default", Tier: deps.TierHomebrew, Items: []deps.InstallItem{
		{Dependency: "starship", Requirement: "required", Status: deps.InstallStatusInstalled, Provider: deps.TierHomebrew, Package: "starship", Executable: "brew", Args: []string{"install", "starship"}},
	}}
	tests := []struct {
		name   string
		env    envelope
		golden string
	}{
		{
			name: "status",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "status",
				Status:        statusFindings,
				Data: statusReport{
					Profile: "default",
					Entries: []status.Entry{
						{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", State: status.StateOK},
						{Source: "configs/git/config", Target: "/home/user/.gitconfig", Strategy: "copy", State: status.StateMissing},
					},
					Provisioners: provision.Plan{
						Profile: "default",
						Steps: []provision.Step{
							{Tool: "skills", Executable: "npx", Args: []string{"--yes", "skills@1.5.12", "add", "vercel-labs/agent-skills"}, Targets: []string{"~/.agents/skills"}},
						},
					},
				},
			},
			golden: "envelope_status.golden",
		},
		{
			name: "plan",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "plan",
				Status:        statusFindings,
				Data: plan.Plan{Profile: "default", Actions: []plan.Action{
					{Source: "configs/zsh/zshrc", ResolvedSource: "/abs/machine/local/path/MUST/NOT/APPEAR", Target: "/home/user/.zshrc", Strategy: "symlink", Status: plan.StatusCreate},
					{Source: "configs/git/config", ResolvedSource: "/abs/x", Target: "/home/user/.gitconfig", Strategy: "copy", Status: plan.StatusConflict},
				}},
			},
			golden: "envelope_plan.golden",
		},
		{
			name: "doctor",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "doctor",
				Status:        statusFindings,
				Data: doctor.Report{
					Profile:  "default",
					OS:       "darwin",
					Platform: doctor.Platform{Supported: true, OS: "darwin"},
					Dependencies: deps.CheckReport{Profile: "default", Results: []deps.Result{
						{Name: "git", Requirement: "required", Command: "git", Present: true, Warning: "git on PATH but `git --version` failed", ProbeDetail: "MUST NOT APPEAR", Hint: "MUST NOT APPEAR"},
						{Name: "rg", Requirement: "optional", Command: "rg", Present: false},
					}},
					Configuration: status.Report{Profile: "default", Entries: []status.Entry{
						{Source: "configs/zsh/zshrc", Target: "/Users/me/.zshrc", Strategy: "symlink", State: status.StateOK},
					}},
					Provisioners: provision.CheckReport{Profile: "default", Items: []provision.Readiness{
						{Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global"}, Missing: []string{"engram"}},
					}},
					SecretScan: doctor.SecretReport{Findings: []doctor.SecretFinding{
						{Source: "configs/git/config", Line: 3, Pattern: "credential-assignment"},
					}},
				},
			},
			golden: "envelope_doctor.golden",
		},
		{
			name: "deps_plan",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "deps plan",
				Status:        statusFindings,
				Data: deps.PlanReport{
					Profile: "default",
					Tier:    deps.Tier("debian"),
					Actions: []deps.InstallAction{
						{Dependency: "rg", Requirement: "optional", Status: deps.InstallActionStatusInstallable, Provider: deps.TierDebian, Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "-y", "ripgrep"}, Candidates: []deps.ProviderCandidate{{Provider: deps.TierDebian, Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "ripgrep"}}}},
					},
					Items: []deps.Guidance{
						{Name: "rg", Requirement: "optional", Command: "apt-get install -y ripgrep", Action: deps.InstallAction{Dependency: "rg", Requirement: "optional", Status: deps.InstallActionStatusInstallable, Provider: deps.TierDebian, Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "-y", "ripgrep"}, Candidates: []deps.ProviderCandidate{{Provider: deps.TierDebian, Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "ripgrep"}}}}},
					},
				},
			},
			golden: "envelope_deps_plan.golden",
		},
		{
			name: "install",
			env: envelope{
				SchemaVersion: schemaVersion,
				Command:       "install",
				Status:        statusOK,
				Data: installReport{
					DryRun: false,
					Dependencies: &installDependenciesReport{
						Preview: &installPreview,
						Result:  &installResult,
					},
					Plan: plan.Plan{Profile: "default", Actions: []plan.Action{
						{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", Status: plan.StatusCreate},
					}},
					Provisioners: provision.Plan{Profile: "default", Steps: []provision.Step{
						{Tool: "skills", Executable: "npx", Args: []string{"--yes", "skills@1.5.12", "add", "vercel-labs/agent-skills"}, Targets: []string{"~/.agents/skills"}},
					}},
				},
			},
			golden: "envelope_install.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeEnvelope(&out, tt.env); err != nil {
				t.Fatalf("writeEnvelope: %v", err)
			}
			assertGolden(t, tt.golden, out.Bytes())
		})
	}
}
