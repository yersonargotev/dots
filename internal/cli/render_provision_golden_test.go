package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/provision"
)

func TestRenderProvisionPlanGolden(t *testing.T) {
	tests := []struct {
		name   string
		plan   provision.Plan
		golden string
	}{
		{
			name: "single provisioner",
			plan: provision.Plan{
				Profile: "default",
				Steps: []provision.Step{
					{
						Tool:       "gentle-ai",
						Executable: "gentle-ai",
						Args:       []string{"install", "--scope", "global", "--persona", "neutral", "--agents", "codex"},
						Targets:    []string{"~/.claude", "~/.codex", "~/.gentle-ai"},
					},
				},
			},
			golden: "provision_single.golden",
		},
		{
			name:   "no provisioners renders nothing",
			plan:   provision.Plan{Profile: "default"},
			golden: "provision_empty.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			renderProvisionPlan(&out, tt.plan)

			goldenPath := filepath.Join("testdata", tt.golden)
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatalf("mkdir testdata: %v", err)
				}
				if err := os.WriteFile(goldenPath, out.Bytes(), 0o600); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}

			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if got := out.Bytes(); !bytes.Equal(got, want) {
				t.Fatalf("golden mismatch for %s\n got:\n%s\nwant:\n%s", tt.golden, got, want)
			}
		})
	}
}
