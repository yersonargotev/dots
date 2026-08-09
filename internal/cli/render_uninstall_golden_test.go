package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
)

func TestRenderUninstallPlanGolden(t *testing.T) {
	tests := []struct {
		name   string
		plan   plan.UninstallPlan
		golden string
	}{
		{
			name: "mixed actions",
			plan: plan.UninstallPlan{Actions: []plan.UninstallAction{
				{Target: "/home/user/.zshrc", Source: "shell/zshrc", Strategy: "symlink", Status: plan.UninstallRemove},
				{Target: "/home/user/.local/state/nvim/lazy-lock.json", Source: "nvim/lazy-lock.json", Strategy: "copy", Ownership: "seeded", Status: plan.UninstallRetain},
				{Target: "/home/user/.gitconfig", Source: "git/gitconfig", Strategy: "copy", ForceRemovable: true, Status: plan.UninstallModified},
				{Target: "/home/user/.tmux.conf", Source: "term/tmux.conf", Strategy: "symlink", Status: plan.UninstallNotOwned},
				{Target: "/home/user/.vimrc", Source: "vim/vimrc", Strategy: "copy", Status: plan.UninstallSkip},
			}},
			golden: "uninstall_mixed.golden",
		},
		{
			name:   "no recorded targets",
			plan:   plan.UninstallPlan{},
			golden: "uninstall_empty.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			renderUninstallPlan(&out, tt.plan)

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
