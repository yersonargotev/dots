package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/status"
)

func TestRenderStatusGolden(t *testing.T) {
	tests := []struct {
		name   string
		report status.Report
		golden string
	}{
		{
			name: "all states",
			report: status.Report{
				Profile: "work",
				Entries: []status.Entry{
					{Source: "configs/zsh/zshrc", Target: "/home/user/.zshrc", Strategy: "symlink", State: status.StateOK},
					{Source: "configs/nvim/lazy-lock.json", Target: "/home/user/.local/state/nvim/lazy-lock.json", Strategy: "copy", State: status.StateOK, Reason: plan.ReasonSeededLocalEvolution},
					{Source: "configs/git/gitconfig", Target: "/home/user/.gitconfig", Strategy: "copy", State: status.StateMissing},
					{Source: "configs/tmux/tmux.conf", Target: "/home/user/.tmux.conf", Strategy: "copy", State: status.StateConflict, Reason: plan.ConflictReasonSourceOverrideNotSelected, MatchingTags: []string{"adaptive-theme", "work-theme"}},
					{Source: "configs/mac/app", Target: "~/.app", Strategy: "copy", State: status.StateSkipped},
					{Source: "configs/starship.toml", Target: "/home/user/.config/starship.toml", Strategy: "copy", State: status.StateDrifted},
					{Source: "configs/nvim/init.lua", Target: "/home/user/.config/nvim/init.lua", Strategy: "template", State: status.StateUnsupported},
				},
			},
			golden: "status_all_states.golden",
		},
		{
			name:   "empty report",
			report: status.Report{Profile: "default"},
			golden: "status_empty.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			renderStatus(&out, tt.report)

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
