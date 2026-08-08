package cli

import (
	"bytes"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/doctor"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/status"
)

func TestRenderDoctorGolden(t *testing.T) {
	tests := []struct {
		name   string
		report doctor.Report
		golden string
	}{
		{
			name: "concerns",
			report: doctor.Report{
				Profile:  "default",
				Platform: doctor.Platform{Supported: true, OS: "linux"},
				Dependencies: deps.CheckReport{Profile: "default", Results: []deps.Result{
					{Name: "git", Command: "git", Present: true},
					{Name: "starship", Command: "starship", Present: false},
				}},
				Configuration: status.Report{Profile: "default", Entries: []status.Entry{
					{Source: "configs/git/config", Target: "/home/user/.gitconfig", Strategy: "copy", State: status.StateOK},
					{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", State: status.StateMissing},
				}},
				SecretScan: doctor.SecretReport{Findings: []doctor.SecretFinding{
					{Source: "configs/git/config", Line: 3, Pattern: "credential-assignment"},
				}},
			},
			golden: "doctor_concerns.golden",
		},
		{
			name: "clean",
			report: doctor.Report{
				Profile:       "minimal",
				Platform:      doctor.Platform{Supported: true, OS: "darwin"},
				Dependencies:  deps.CheckReport{Profile: "minimal"},
				Configuration: status.Report{Profile: "minimal", Entries: []status.Entry{{Source: "configs/zsh/zshrc", Target: "/Users/me/.zshrc", Strategy: "symlink", State: status.StateOK}}},
				SecretScan:    doctor.SecretReport{},
			},
			golden: "doctor_clean.golden",
		},
		{
			name: "provisioner not ready",
			report: doctor.Report{
				Profile:       "default",
				Platform:      doctor.Platform{Supported: true, OS: "darwin"},
				Dependencies:  deps.CheckReport{Profile: "default"},
				Configuration: status.Report{Profile: "default"},
				Provisioners: provision.CheckReport{Profile: "default", Items: []provision.Readiness{
					{
						Tool:       "claude",
						Executable: "claude",
						Args:       []string{"plugin", "marketplace", "add", "example/tools"},
						Missing:    []string{"npx"},
					},
				}},
				SecretScan: doctor.SecretReport{},
			},
			golden: "doctor_provisioner_not_ready.golden",
		},
		{
			name: "git toolchain warning",
			report: doctor.Report{
				Profile:  "default",
				Platform: doctor.Platform{Supported: true, OS: "darwin"},
				Dependencies: deps.CheckReport{Profile: "default", Results: []deps.Result{
					{
						Name:        "git",
						Command:     "git",
						Present:     true,
						Warning:     "git resolved on PATH but `git --version` failed",
						ProbeDetail: "xcrun: error: invalid active developer path (/Library/Developer/CommandLineTools), missing xcrun",
						Hint:        "Repair Xcode Command Line Tools with `xcode-select --install` or reinstall them, then rerun `dots doctor`.",
					},
				}},
				Configuration: status.Report{Profile: "default"},
				SecretScan:    doctor.SecretReport{},
			},
			golden: "doctor_git_toolchain_warning.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			renderDoctor(&out, tt.report)
			assertGolden(t, tt.golden, out.Bytes())
		})
	}
}
