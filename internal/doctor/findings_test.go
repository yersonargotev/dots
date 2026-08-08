package doctor

import (
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/status"
)

// healthyReport is a doctor Report with no concern in any dimension.
func healthyReport() Report {
	return Report{
		Platform:      Platform{Supported: true},
		Dependencies:  deps.CheckReport{Results: []deps.Result{{Name: "git", Present: true}}},
		Configuration: status.Report{Entries: []status.Entry{{State: status.StateOK}}},
		Provisioners:  provision.CheckReport{Items: []provision.Readiness{{Tool: "claude"}}},
		SecretScan:    SecretReport{},
	}
}

func TestDoctorReportHasFindings(t *testing.T) {
	if healthyReport().HasFindings() {
		t.Fatal("a healthy report must not report findings")
	}

	cases := []struct {
		name   string
		mutate func(*Report)
	}{
		{"unsupported platform", func(r *Report) { r.Platform.Supported = false }},
		{"missing dependency", func(r *Report) {
			r.Dependencies.Results = append(r.Dependencies.Results, deps.Result{Name: "rg", Present: false})
		}},
		{"configuration drift", func(r *Report) {
			r.Configuration.Entries = append(r.Configuration.Entries, status.Entry{State: status.StateDrifted})
		}},
		{"provisioner not ready", func(r *Report) {
			r.Provisioners.Items = append(r.Provisioners.Items, provision.Readiness{Tool: "claude", Missing: []string{"claude"}})
		}},
		{"secret finding", func(r *Report) {
			r.SecretScan.Findings = append(r.SecretScan.Findings, SecretFinding{Source: "configs/git/gitconfig", Line: 1, Pattern: "api_key"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := healthyReport()
			tc.mutate(&r)
			if !r.HasFindings() {
				t.Fatalf("%s must report findings", tc.name)
			}
		})
	}
}
