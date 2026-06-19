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
				Data: status.Report{Profile: "default", Entries: []status.Entry{
					{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", State: status.StateOK},
					{Source: "configs/git/config", Target: "/home/user/.gitconfig", Strategy: "copy", State: status.StateMissing},
				}},
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
						{Name: "git", Command: "git", Present: true, Warning: "git on PATH but `git --version` failed", ProbeDetail: "MUST NOT APPEAR", Hint: "MUST NOT APPEAR"},
						{Name: "rg", Command: "rg", Present: false},
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
						{Dependency: "rg", Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "-y", "ripgrep"}},
					},
					Items: []deps.Guidance{
						{Name: "rg", Command: "apt-get install -y ripgrep", Action: deps.InstallAction{Dependency: "rg", Package: "ripgrep", Executable: "apt-get", Args: []string{"install", "-y", "ripgrep"}}},
					},
				},
			},
			golden: "envelope_deps_plan.golden",
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
