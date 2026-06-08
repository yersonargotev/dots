package deps_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

func TestInstallDryRunReportsInstallableActions(t *testing.T) {
	report, err := deps.InstallDryRun(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}

	if report.Profile != "default" {
		t.Fatalf("Profile = %q, want default", report.Profile)
	}
	if report.Tier != deps.TierHomebrew {
		t.Fatalf("Tier = %q, want homebrew", report.Tier)
	}
	if len(report.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
	}

	item := report.Items[0]
	if item.Dependency != "starship" {
		t.Fatalf("Items[0].Dependency = %q, want starship", item.Dependency)
	}
	if item.Status != deps.InstallPreviewWouldInstall {
		t.Fatalf("Items[0].Status = %q, want %q", item.Status, deps.InstallPreviewWouldInstall)
	}
	if item.Package != "starship" || item.Executable != "brew" {
		t.Fatalf("Items[0] package/executable = %#v, want brew install starship", item)
	}
	if !reflect.DeepEqual(item.Args, []string{"install", "starship"}) {
		t.Fatalf("Items[0].Args = %#v, want install starship", item.Args)
	}
	if item.Manual != "" {
		t.Fatalf("Items[0].Manual = %q, want empty", item.Manual)
	}
}

func TestInstallDryRunReportsManualActionsAsManual(t *testing.T) {
	report, err := deps.InstallDryRun(manualOnlyManifest(), deps.Options{Profile: "default", OS: "linux"}, lookupSet(), deps.TierGeneric)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("Items len = %d, want 1 (%#v)", len(report.Items), report.Items)
	}

	item := report.Items[0]
	if item.Status != deps.InstallPreviewManual {
		t.Fatalf("Status = %q, want %q", item.Status, deps.InstallPreviewManual)
	}
	if item.Executable != "" || len(item.Args) != 0 || item.Package != "" {
		t.Fatalf("manual item has executable fields: %#v", item)
	}
	if item.Manual == "" {
		t.Fatalf("Manual empty, want manual guidance")
	}
}

func manualOnlyManifest() manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
				Dependencies: []manifest.Dependency{{Name: "neovim", Command: "nvim"}},
			},
		},
	}
}

func TestInstallDryRunUsesPlanSelectionOrderAndSkipsPresentDependencies(t *testing.T) {
	report, err := deps.InstallDryRun(installSelectionManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "nvim"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("InstallDryRun() error = %v", err)
	}

	got := make([]string, 0, len(report.Items))
	for _, item := range report.Items {
		got = append(got, item.Dependency)
	}
	want := []string{"starship", "ripgrep"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dry-run item order = %#v, want %#v", got, want)
	}
}

func installSelectionManifest() manifest.Manifest {
	return manifest.Manifest{
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
					{Name: "starship", Brew: "starship"},
					{Name: "neovim", Command: "nvim", Brew: "neovim"},
				},
			},
		},
	}
}
