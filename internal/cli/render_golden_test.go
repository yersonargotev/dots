package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
)

var update = flag.Bool("update", false, "update golden files")

func TestRenderPlanGolden(t *testing.T) {
	tests := []struct {
		name   string
		plan   plan.Plan
		golden string
	}{
		{
			name: "all statuses",
			plan: plan.Plan{
				Profile: "work",
				Actions: []plan.Action{
					{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", Status: plan.StatusCreate},
					{Source: "configs/git/gitconfig", Target: "/home/user/.gitconfig", Strategy: "symlink", Status: plan.StatusConflict},
					{Source: "configs/starship.toml", Target: "/home/user/.config/starship.toml", Strategy: "copy", Status: plan.StatusUnchanged},
					{Source: "configs/nvim/init.lua", Target: "/home/user/.config/nvim/init.lua", Strategy: "template", Status: plan.StatusMissingSource},
				},
			},
			golden: "plan_all_statuses.golden",
		},
		{
			name:   "empty plan",
			plan:   plan.Plan{Profile: "default"},
			golden: "plan_empty.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			renderPlan(&out, tt.plan)

			goldenPath := filepath.Join("testdata", tt.golden)
			if *update {
				if err := os.MkdirAll("testdata", 0o755); err != nil {
					t.Fatalf("mkdir testdata: %v", err)
				}
				if err := os.WriteFile(goldenPath, out.Bytes(), 0o600); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				// In update mode we only regenerate the fixture; the real
				// comparison happens on a subsequent run without -update.
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
