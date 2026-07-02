package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
)

func TestRenderDepsCheckGolden(t *testing.T) {
	tests := []struct {
		name   string
		report deps.CheckReport
		golden string
	}{
		{
			name: "present and missing",
			report: deps.CheckReport{
				Profile: "default",
				Results: []deps.Result{
					{Name: "tmux", Command: "tmux", Present: true, Warning: "tmux 3.7a has a known synchronized-update redraw regression", ProbeDetail: "tmux 3.7a", Hint: "Upgrade tmux to 3.7b or newer, then stop old servers with `tmux kill-server` so new sessions use the fixed binary."},
					{Name: "ripgrep", Command: "rg", Present: false},
					{Name: "starship", Command: "starship", Present: false},
				},
			},
			golden: "deps_check_mixed.golden",
		},
		{
			name:   "no dependencies",
			report: deps.CheckReport{Profile: "minimal"},
			golden: "deps_check_empty.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			renderDepsCheck(&out, tt.report)
			assertGolden(t, tt.golden, out.Bytes())
		})
	}
}

func TestRenderDepsPlanGolden(t *testing.T) {
	tests := []struct {
		name   string
		report deps.PlanReport
		golden string
	}{
		{
			name: "command and manual guidance",
			report: deps.PlanReport{
				Profile: "default",
				Tier:    deps.TierDebian,
				Items: []deps.Guidance{
					{Name: "starship", Command: "sudo apt-get install starship"},
					{Name: "neovim", Manual: `no debian package declared for "neovim"; install it manually`},
				},
			},
			golden: "deps_plan_mixed.golden",
		},
		{
			name: "homebrew tap trust guidance",
			report: deps.PlanReport{
				Profile: "agents",
				Tier:    deps.TierHomebrew,
				Items: []deps.Guidance{
					{Name: "gentle-ai", Command: "brew install gentleman-programming/tap/gentle-ai", TrustCommand: "brew trust --formula gentleman-programming/tap/gentle-ai"},
				},
			},
			golden: "deps_plan_tap_trust.golden",
		},
		{
			name:   "nothing to install",
			report: deps.PlanReport{Profile: "default", Tier: deps.TierHomebrew},
			golden: "deps_plan_empty.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			renderDepsPlan(&out, tt.report)
			assertGolden(t, tt.golden, out.Bytes())
		})
	}
}

func TestRenderDepsInstallDryRunGolden(t *testing.T) {
	tests := []struct {
		name   string
		report deps.InstallDryRunReport
		golden string
	}{
		{
			name: "would-install and manual previews",
			report: deps.InstallDryRunReport{
				Profile: "default",
				Tier:    deps.TierDebian,
				Items: []deps.InstallPreview{
					{Dependency: "starship", Status: deps.InstallPreviewWouldInstall, Package: "starship", Executable: "sudo", Args: []string{"apt-get", "install", "starship"}},
					{Dependency: "neovim", Status: deps.InstallPreviewManual, Manual: `no debian package declared for "neovim"; install it manually`},
				},
			},
			golden: "deps_install_dry_run_mixed.golden",
		},
		{
			name: "homebrew tap trust preview",
			report: deps.InstallDryRunReport{
				Profile: "agents",
				Tier:    deps.TierHomebrew,
				Items: []deps.InstallPreview{
					{Dependency: "gentle-ai", Status: deps.InstallPreviewWouldInstall, Package: "gentleman-programming/tap/gentle-ai", Executable: "brew", Args: []string{"install", "gentleman-programming/tap/gentle-ai"}, TrustCommand: "brew trust --formula gentleman-programming/tap/gentle-ai"},
				},
			},
			golden: "deps_install_dry_run_tap_trust.golden",
		},
		{
			name:   "nothing to install",
			report: deps.InstallDryRunReport{Profile: "default", Tier: deps.TierHomebrew},
			golden: "deps_install_dry_run_empty.golden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			renderDepsInstallDryRun(&out, tt.report)
			assertGolden(t, tt.golden, out.Bytes())
		})
	}
}

func assertGolden(t *testing.T, golden string, got []byte) {
	t.Helper()
	goldenPath := filepath.Join("testdata", golden)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch for %s\n got:\n%s\nwant:\n%s", golden, got, want)
	}
}
