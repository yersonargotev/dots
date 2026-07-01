package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
)

func TestRenderSkippedEntryHint(t *testing.T) {
	core := manifest.Entry{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}}
	desktop := func(source string) manifest.Entry {
		return manifest.Entry{Source: source, Target: "~/" + source, Strategy: "symlink", Tags: []string{"desktop"}}
	}
	profiles := map[string]manifest.Profile{
		"default": {Tags: []string{"core"}},
		"desktop": {Tags: []string{"core", "desktop"}},
	}

	tests := []struct {
		name    string
		entries []manifest.Entry
		profile string
		want    string
	}{
		{
			name:    "plural for more than one skipped entry",
			entries: []manifest.Entry{core, desktop("configs/ghostty/config.ghostty"), desktop("configs/zed/settings.json")},
			profile: "default",
			want:    "\nNote: profile \"default\" skips 2 file entries; run with --profile desktop to include them.\n",
		},
		{
			name:    "singular for exactly one skipped entry",
			entries: []manifest.Entry{core, desktop("configs/ghostty/config.ghostty")},
			profile: "default",
			want:    "\nNote: profile \"default\" skips 1 file entry; run with --profile desktop to include them.\n",
		},
		{
			name:    "quiet when the active profile selects everything",
			entries: []manifest.Entry{core, desktop("configs/ghostty/config.ghostty")},
			profile: "desktop",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := manifest.Manifest{Version: 1, Profiles: profiles, Entries: tt.entries}
			var out bytes.Buffer
			if err := renderSkippedEntryHint(&out, m, []string{tt.profile}, "darwin"); err != nil {
				t.Fatalf("renderSkippedEntryHint() error = %v", err)
			}
			if out.String() != tt.want {
				t.Fatalf("renderSkippedEntryHint() = %q, want %q", out.String(), tt.want)
			}
		})
	}
}

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
