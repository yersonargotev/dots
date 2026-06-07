package deps_test

import (
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

func planManifest() manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/tmux/tmux.conf", Target: "~/.tmux.conf", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "starship", Brew: "starship", Apt: "starship", Dnf: "starship", Pacman: "starship"},
					{Name: "tmux", Brew: "tmux", Apt: "tmux", Dnf: "tmux", Pacman: "tmux"},
				},
			},
		},
	}
}

func TestPlanProducesTierGuidanceForMissingDependencies(t *testing.T) {
	tests := []struct {
		name        string
		tier        deps.Tier
		wantCommand string
	}{
		{name: "homebrew", tier: deps.TierHomebrew, wantCommand: "brew install starship"},
		{name: "debian", tier: deps.TierDebian, wantCommand: "sudo apt-get install starship"},
		{name: "fedora", tier: deps.TierFedora, wantCommand: "sudo dnf install starship"},
		{name: "arch", tier: deps.TierArch, wantCommand: "sudo pacman -S starship"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// tmux is present, starship missing: only starship should be planned.
			report, err := deps.Plan(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), tt.tier)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if report.Tier != tt.tier {
				t.Fatalf("Tier = %q, want %q", report.Tier, tt.tier)
			}
			if len(report.Items) != 1 {
				t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
			}
			item := report.Items[0]
			if item.Name != "starship" {
				t.Fatalf("Items[0].Name = %q, want starship", item.Name)
			}
			if item.Command != tt.wantCommand {
				t.Fatalf("Items[0].Command = %q, want %q", item.Command, tt.wantCommand)
			}
			if item.Manual != "" {
				t.Fatalf("Items[0].Manual = %q, want empty for mapped dependency", item.Manual)
			}
		})
	}
}

func TestPlanFallsBackToManualGuidance(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					// No package mapping at all.
					{Name: "neovim", Command: "nvim"},
					// Has brew but no apt: manual on debian.
					{Name: "ripgrep", Command: "rg", Brew: "ripgrep"},
				},
			},
		},
	}

	t.Run("generic tier is always manual", func(t *testing.T) {
		report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, lookupSet(), deps.TierGeneric)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(report.Items) != 2 {
			t.Fatalf("Items len = %d, want 2", len(report.Items))
		}
		for _, item := range report.Items {
			if item.Command != "" {
				t.Fatalf("generic Command = %q, want empty", item.Command)
			}
			if item.Manual == "" {
				t.Fatalf("generic Manual empty for %q, want guidance", item.Name)
			}
		}
	})

	t.Run("missing package field falls back to manual", func(t *testing.T) {
		report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, lookupSet(), deps.TierDebian)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		byName := map[string]deps.Guidance{}
		for _, item := range report.Items {
			byName[item.Name] = item
		}
		if byName["ripgrep"].Command != "" || byName["ripgrep"].Manual == "" {
			t.Fatalf("ripgrep guidance = %#v, want manual fallback on debian", byName["ripgrep"])
		}
		if byName["neovim"].Command != "" || byName["neovim"].Manual == "" {
			t.Fatalf("neovim guidance = %#v, want manual fallback", byName["neovim"])
		}
	})
}

func TestPlanIsEmptyWhenAllDependenciesPresent(t *testing.T) {
	report, err := deps.Plan(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "starship"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Items) != 0 {
		t.Fatalf("Items len = %d, want 0 when all present", len(report.Items))
	}
}
