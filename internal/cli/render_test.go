package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
)

func TestDefaultSourceRoot(t *testing.T) {
	home := filepath.Join("home", "user")
	want := filepath.Join(home, ".local", "share", "dots")
	if got := defaultSourceRoot(home); got != want {
		t.Fatalf("defaultSourceRoot(%q) = %q, want %q", home, got, want)
	}
}

func TestRenderPlanSingleAction(t *testing.T) {
	p := plan.Plan{
		Profile: "default",
		Actions: []plan.Action{
			{
				Source:   "configs/zsh/zshrc",
				Target:   "/home/user/.zshrc",
				Strategy: "symlink",
				Status:   plan.StatusCreate,
			},
		},
	}

	var out bytes.Buffer
	renderPlan(&out, p)

	want := `Plan for profile "default"

  create          symlink   configs/zsh/zshrc -> /home/user/.zshrc

Summary: 1 create, 0 conflict, 0 unchanged, 0 missing-source
`
	if got := out.String(); got != want {
		t.Fatalf("renderPlan() output mismatch\n got:\n%q\nwant:\n%q", got, want)
	}
}
