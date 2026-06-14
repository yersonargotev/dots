package deps_test

import (
	"reflect"
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

func TestPlanProducesStructuredInstallActionsForMappedPackages(t *testing.T) {
	tests := []struct {
		name       string
		tier       deps.Tier
		executable string
		args       []string
	}{
		{name: "homebrew", tier: deps.TierHomebrew, executable: "brew", args: []string{"install", "starship"}},
		{name: "debian", tier: deps.TierDebian, executable: "sudo", args: []string{"apt-get", "install", "starship"}},
		{name: "fedora", tier: deps.TierFedora, executable: "sudo", args: []string{"dnf", "install", "starship"}},
		{name: "arch", tier: deps.TierArch, executable: "sudo", args: []string{"pacman", "-S", "starship"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := deps.Plan(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), tt.tier)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(report.Actions) != 1 {
				t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
			}

			action := report.Actions[0]
			if action.Dependency != "starship" {
				t.Fatalf("Actions[0].Dependency = %q, want starship", action.Dependency)
			}
			if action.Package != "starship" {
				t.Fatalf("Actions[0].Package = %q, want starship", action.Package)
			}
			if action.Executable != tt.executable {
				t.Fatalf("Actions[0].Executable = %q, want %q", action.Executable, tt.executable)
			}
			if !reflect.DeepEqual(action.Args, tt.args) {
				t.Fatalf("Actions[0].Args = %#v, want %#v", action.Args, tt.args)
			}
			if action.Manual != "" {
				t.Fatalf("Actions[0].Manual = %q, want empty for executable action", action.Manual)
			}
		})
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

func TestPlanProducesManualInstallActions(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "neovim", Command: "nvim"},
					{Name: "ripgrep", Command: "rg", Brew: "ripgrep"},
				},
			},
		},
	}

	tests := []struct {
		name string
		tier deps.Tier
	}{
		{name: "generic tier is manual guidance only", tier: deps.TierGeneric},
		{name: "missing package mapping is manual guidance", tier: deps.TierDebian},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, lookupSet(), tt.tier)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(report.Actions) != 2 {
				t.Fatalf("Actions len = %d, want 2 (%#v)", len(report.Actions), report.Actions)
			}

			for _, action := range report.Actions {
				if action.Executable != "" {
					t.Fatalf("%s Executable = %q, want empty for manual action", action.Dependency, action.Executable)
				}
				if len(action.Args) != 0 {
					t.Fatalf("%s Args = %#v, want empty for manual action", action.Dependency, action.Args)
				}
				if action.Package != "" {
					t.Fatalf("%s Package = %q, want empty for manual action", action.Dependency, action.Package)
				}
				if action.Manual == "" {
					t.Fatalf("%s Manual empty, want manual guidance", action.Dependency)
				}
			}
		})
	}
}

func TestPlanActionsUseManifestOrderAndSkipPresentDependencies(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "tmux", Brew: "tmux"},
					{Name: "starship", Brew: "starship"},
					{Name: "ripgrep", Command: "rg", Brew: "ripgrep"},
				},
			},
			{
				Source: "configs/tmux/tmux.conf", Target: "~/.tmux.conf", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					// Duplicate dependency declarations should not create another action.
					{Name: "starship", Brew: "starship"},
					{Name: "neovim", Command: "nvim", Brew: "neovim"},
				},
			},
		},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "nvim"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	got := make([]string, 0, len(report.Actions))
	for _, action := range report.Actions {
		got = append(got, action.Dependency)
		if action.Package == "" {
			t.Fatalf("%s Package empty, want one package per executable action", action.Dependency)
		}
	}
	want := []string{"starship", "ripgrep"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("action order = %#v, want %#v", got, want)
	}
}

func TestPlanIsEmptyWhenAllDependenciesPresent(t *testing.T) {
	report, err := deps.Plan(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "starship"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Items) != 0 {
		t.Fatalf("Items len = %d, want 0 when all present", len(report.Items))
	}
	if len(report.Actions) != 0 {
		t.Fatalf("Actions len = %d, want 0 when all present", len(report.Actions))
	}
}

func TestPlanIncludesSelectedProvisionerDependencies(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Provisioners: []manifest.Provisioner{
			{
				Tool: "gentle-ai",
				Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "gentle-ai", Brew: "gentleman-programming/tap/gentle-ai"},
					{Name: "engram", Brew: "gentleman-programming/tap/engram"},
				},
			},
		},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("engram"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	action := report.Actions[0]
	if action.Dependency != "gentle-ai" {
		t.Fatalf("Actions[0].Dependency = %q, want gentle-ai", action.Dependency)
	}
	if action.Package != "gentleman-programming/tap/gentle-ai" {
		t.Fatalf("Actions[0].Package = %q, want tap package", action.Package)
	}
}
