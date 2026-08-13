package deps_test

import (
	"path/filepath"
	"reflect"
	"strings"
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
			report, err := deps.Plan(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), fontLookupSet(), tt.tier)
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

func TestPlanUsesDarwinAppAsAlternativeToCommandProbe(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{{
			Source: "configs/ghostty/config.ghostty", Target: "~/.config/ghostty/config.ghostty", Strategy: "symlink", Tags: []string{"desktop"},
			Dependencies: []manifest.Dependency{{
				Name: "ghostty", Command: "ghostty", DarwinApp: "Ghostty.app", BrewCask: "ghostty",
			}},
		}},
	}
	tests := []struct {
		name        string
		os          string
		apps        deps.AppLookup
		wantActions int
	}{
		{name: "installed Darwin app", os: "darwin", apps: appLookupSet("Ghostty.app"), wantActions: 0},
		{name: "missing Darwin app", os: "darwin", apps: appLookupSet(), wantActions: 1},
		{name: "Linux ignores Darwin app", os: "linux", apps: appLookupSet("Ghostty.app"), wantActions: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report, err := deps.Plan(m, deps.Options{
				Profile: "default", OS: tt.os, AppLookup: tt.apps,
			}, lookupSet(), fontLookupSet(), deps.TierHomebrew)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(report.Actions) != tt.wantActions {
				t.Fatalf("Actions = %#v, want len %d", report.Actions, tt.wantActions)
			}
		})
	}
}

func TestRepositoryManifestPlansCodexBarCaskOnDarwinOnly(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	darwin, err := deps.Plan(*m, deps.Options{
		ExtraTags: []string{"codexbar"}, OS: "darwin", AppLookup: appLookupSet(),
	}, lookupSet("brew"), fontLookupSet(), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Darwin Plan() error = %v", err)
	}
	if len(darwin.Actions) != 1 {
		t.Fatalf("Darwin Actions = %#v, want one CodexBar action", darwin.Actions)
	}
	action := darwin.Actions[0]
	if action.Dependency != "CodexBar" || action.Package != "codexbar" || action.Executable != "brew" {
		t.Fatalf("Darwin CodexBar action = %#v, want brew cask action", action)
	}
	if want := []string{"install", "--cask", "codexbar"}; !reflect.DeepEqual(action.Args, want) {
		t.Fatalf("Darwin CodexBar Args = %#v, want %#v", action.Args, want)
	}

	linux, err := deps.Plan(*m, deps.Options{
		ExtraTags: []string{"codexbar"}, OS: "linux", AppLookup: appLookupSet("CodexBar.app"),
	}, lookupSet("brew"), fontLookupSet(), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Linux Plan() error = %v", err)
	}
	if len(linux.Actions) != 0 {
		t.Fatalf("Linux Actions = %#v, want no CodexBar action", linux.Actions)
	}
}

func TestPlanUsesAtuinUserLocalProviderOnLinuxWhenDistroProviderUnavailable(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source: "configs/atuin/config.toml", Target: "~/.config/atuin/config.toml", Strategy: "copy", Ownership: "toml-subset", Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{
				Name:          "atuin",
				Command:       "atuin",
				Brew:          "atuin",
				LinuxHomebrew: true,
				UserLocal: &manifest.UserLocalProvider{
					Recipe:  "atuin",
					Version: "v18.16.1",
					Checksums: map[string]string{
						"linux_amd64": "5c41e20c0130ac84fa4bfa42c19bb55a07855838506063caad0d2922593b39be",
					},
				},
			}},
		}},
	}

	installedReport, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("atuin"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() with installed atuin error = %v", err)
	}
	if len(installedReport.Actions) != 0 {
		t.Fatalf("installed Actions len = %d, want 0 (%#v)", len(installedReport.Actions), installedReport.Actions)
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet(), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	action := report.Actions[0]
	if action.Provider != deps.TierUserLocal || action.UserLocal == nil {
		t.Fatalf("action = %#v, want Atuin user-local provider", action)
	}
	if action.UserLocal.Recipe != "atuin" || action.UserLocal.Command != "atuin" || action.UserLocal.Layout != "bundle" {
		t.Fatalf("UserLocal = %#v, want atuin bundle recipe", action.UserLocal)
	}
	if action.UserLocal.URL != "https://github.com/atuinsh/atuin/releases/download/v18.16.1/atuin-x86_64-unknown-linux-gnu.tar.gz" {
		t.Fatalf("UserLocal.URL = %q", action.UserLocal.URL)
	}
}

func TestPlanReportsDependencyRequirement(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Dependencies: []manifest.DependencySet{{Tags: []string{"core"}, Dependencies: []manifest.Dependency{
			{Name: "required-tool", Brew: "required-tool"},
			{Name: "optional-tool", Requirement: manifest.DependencyRequirementOptional, Brew: "optional-tool"},
		}}},
		Entries: []manifest.Entry{{Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"}}},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("brew"), fontLookupSet(), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 2 {
		t.Fatalf("Actions len = %d, want 2 (%#v)", len(report.Actions), report.Actions)
	}
	if report.Actions[0].Requirement != manifest.DependencyRequirementRequired || report.Items[0].Requirement != manifest.DependencyRequirementRequired {
		t.Fatalf("required dependency requirement not defaulted: actions=%#v items=%#v", report.Actions, report.Items)
	}
	if report.Actions[1].Requirement != manifest.DependencyRequirementOptional || report.Items[1].Requirement != manifest.DependencyRequirementOptional {
		t.Fatalf("optional dependency requirement missing: actions=%#v items=%#v", report.Actions, report.Items)
	}
}

func TestPlanProducesHomebrewCaskActionsForMappedPackages(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zed/settings.json", Target: "~/.config/zed/settings.json", Strategy: "symlink", Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{
					{Name: "CascadiaCode Nerd Font", BrewCask: "  font-cascadia-code-nf  ", FontMatch: "CascadiaCodeNF*"},
				},
			},
		},
	}

	var commandProbes []string
	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "darwin"}, func(command string) bool {
		commandProbes = append(commandProbes, command)
		return command == "brew"
	}, fontLookupSet(), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if !reflect.DeepEqual(commandProbes, []string{"brew"}) {
		t.Fatalf("command probes = %#v, want brew provider availability probe", commandProbes)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	action := report.Actions[0]
	if action.Package != "font-cascadia-code-nf" {
		t.Fatalf("Package = %q, want font-cascadia-code-nf", action.Package)
	}
	if action.Executable != "brew" {
		t.Fatalf("Executable = %q, want brew", action.Executable)
	}
	if !reflect.DeepEqual(action.Args, []string{"install", "--cask", "font-cascadia-code-nf"}) {
		t.Fatalf("Args = %#v, want brew install --cask font-cascadia-code-nf", action.Args)
	}
	if report.Items[0].Command != "brew install --cask font-cascadia-code-nf" {
		t.Fatalf("Command = %q, want brew install --cask font-cascadia-code-nf", report.Items[0].Command)
	}
}

func TestPlanKeepsLinuxFontDependenciesManualEvenWhenBrewIsAvailable(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zed/settings.json", Target: "~/.config/zed/settings.json", Strategy: "symlink", Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{
					{Name: "CascadiaCode Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*"},
				},
			},
		},
	}

	for _, tier := range []deps.Tier{deps.TierDebian, deps.TierHomebrew} {
		t.Run(string(tier), func(t *testing.T) {
			report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, lookupSet(), fontLookupSet(), tier)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}

			if len(report.Actions) != 1 {
				t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
			}
			action := report.Actions[0]
			if action.Provider != "" || action.Executable != "" || action.Package != "" || len(action.Args) != 0 {
				t.Fatalf("linux font action = %#v, want manual guidance only", action)
			}
			if action.Manual == "" {
				t.Fatalf("Manual empty, want Linux font guidance")
			}
		})
	}
}

func TestPlanSkipsFontDependencyWhenFallbackMatchIsInstalled(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{
			{
				Source: "configs/zed/settings.json", Target: "~/.config/zed/settings.json", Strategy: "symlink", Tags: []string{"desktop"},
				Dependencies: []manifest.Dependency{
					{Name: "Desktop Nerd Font", BrewCask: "font-cascadia-code-nf", FontMatch: "CascadiaCodeNF*", FontFallbackMatches: []string{"CaskaydiaCoveNerdFont*"}},
				},
			},
		},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet(), fontLookupSet("CaskaydiaCoveNerdFont*"), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 0 {
		t.Fatalf("Actions len = %d, want 0 when fallback font is installed (%#v)", len(report.Actions), report.Actions)
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
			report, err := deps.Plan(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), fontLookupSet(), tt.tier)
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
					// Has explicit manual remediation.
					{Name: "ghostty", Command: "ghostty", ManualDebian: "Install Ghostty from https://ghostty.org/docs/install/binary, then rerun dots deps check."},
				},
			},
		},
	}

	t.Run("generic tier is always manual", func(t *testing.T) {
		report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, noProviderLookup(), fontLookupSet(), deps.TierGeneric)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}
		if len(report.Items) != 3 {
			t.Fatalf("Items len = %d, want 3", len(report.Items))
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
		report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, noProviderLookup(), fontLookupSet(), deps.TierDebian)
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
		wantManual := "Install Ghostty from https://ghostty.org/docs/install/binary, then rerun dots deps check."
		if byName["ghostty"].Manual != wantManual {
			t.Fatalf("ghostty Manual = %q, want %q", byName["ghostty"].Manual, wantManual)
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
			report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, noProviderLookup(), fontLookupSet(), tt.tier)
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

func TestPlanFallsBackFromUnavailableDistroProviderToHomebrew(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{Name: "ripgrep", Command: "rg", Apt: "ripgrep", Brew: "ripgrep", LinuxHomebrew: true}},
		}},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, func(command string) bool {
		return command == "brew"
	}, fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	action := report.Actions[0]
	if action.Status != deps.InstallActionStatusInstallable || action.Provider != deps.TierHomebrew || action.Executable != "brew" || action.Package != "ripgrep" {
		t.Fatalf("action = %#v, want installable Homebrew fallback after unavailable debian provider", action)
	}
	if len(action.Candidates) != 2 {
		t.Fatalf("Candidates len = %d, want debian and homebrew", len(action.Candidates))
	}
	if action.Candidates[0].Provider != deps.TierDebian {
		t.Fatalf("first candidate = %#v, want debian", action.Candidates[0])
	}
	if action.Candidates[1].Provider != deps.TierHomebrew {
		t.Fatalf("second candidate = %#v, want homebrew", action.Candidates[1])
	}
}

func TestPlanDoesNotFallbackToHomebrewOnLinuxWithoutOptIn(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"desktop"}}},
		Entries: []manifest.Entry{{
			Source: "configs/zed/settings.json", Target: "~/.config/zed/settings.json", Strategy: "symlink", Tags: []string{"desktop"},
			Dependencies: []manifest.Dependency{{Name: "zed", Command: "zed", Brew: "zed"}},
		}},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, lookupSet("brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	action := report.Actions[0]
	if action.Status != deps.InstallActionStatusManual || action.Provider != "" || action.Executable != "" || action.Package != "" || action.Manual == "" {
		t.Fatalf("action = %#v, want manual action without Linux Homebrew opt-in", action)
	}
	if len(action.Candidates) != 0 {
		t.Fatalf("Candidates = %#v, want no Linux Homebrew candidate without opt-in", action.Candidates)
	}
}

func TestPlanDoesNotFallbackToHomebrewOutsideLinux(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{Name: "ripgrep", Command: "rg", Brew: "ripgrep", LinuxHomebrew: true}},
		}},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "freebsd"}, lookupSet("brew"), fontLookupSet(), deps.TierGeneric)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	action := report.Actions[0]
	if action.Status != deps.InstallActionStatusManual || action.Provider != "" || action.Executable != "" || action.Package != "" || action.Manual == "" {
		t.Fatalf("action = %#v, want manual action outside Linux even with linux_homebrew opt-in", action)
	}
	if len(action.Candidates) != 0 {
		t.Fatalf("Candidates = %#v, want no Homebrew candidate outside Linux", action.Candidates)
	}
}

func TestPlanReportsManualWhenNoProviderCandidateIsExecutable(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{Name: "ripgrep", Command: "rg", Apt: "ripgrep", Brew: "ripgrep", LinuxHomebrew: true}},
		}},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux"}, noProviderLookup(), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	action := report.Actions[0]
	if action.Status != deps.InstallActionStatusManual || action.Executable != "" || action.Package != "" || action.Manual == "" {
		t.Fatalf("action = %#v, want manual action with no executable provider", action)
	}
	if len(action.Candidates) != 2 {
		t.Fatalf("Candidates len = %d, want unavailable debian and homebrew", len(action.Candidates))
	}
	if action.Candidates[0].Provider != deps.TierDebian || action.Candidates[1].Provider != deps.TierHomebrew {
		t.Fatalf("Candidates = %#v, want debian and homebrew", action.Candidates)
	}
}

func TestRepositoryManifestDoesNotAdvertiseUnavailableUbuntuAptPackages(t *testing.T) {
	m, err := manifest.LoadFile("../../dots.yaml")
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	t.Run("uses Homebrew fallback when available", func(t *testing.T) {
		report, err := deps.Plan(*m, deps.Options{Profile: "core", OS: "linux"}, noProviderLookup("sudo", "apt-get", "brew"), fontLookupSet(), deps.TierDebian)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}

		githubCLI, ok := findAction(report.Actions, "GitHub CLI")
		if !ok {
			t.Fatalf("missing action for GitHub CLI in %#v", report.Actions)
		}
		if githubCLI.Provider == deps.TierDebian || githubCLI.Executable == "sudo" {
			t.Fatalf("GitHub CLI action = %#v, must not advertise unavailable Ubuntu apt package", githubCLI)
		}
		if githubCLI.Status != deps.InstallActionStatusInstallable || githubCLI.Provider != deps.TierHomebrew || githubCLI.Executable != "brew" {
			t.Fatalf("GitHub CLI action = %#v, want installable Homebrew fallback", githubCLI)
		}

		for _, name := range []string{"bat", "starship", "zellij", "pnpm"} {
			action, ok := findAction(report.Actions, name)
			if !ok {
				t.Fatalf("missing action for %q in %#v", name, report.Actions)
			}
			if action.Provider == deps.TierDebian || action.Executable == "sudo" {
				t.Fatalf("%s action = %#v, must not advertise unavailable Ubuntu apt package", name, action)
			}
			if action.Provider != deps.TierUserLocal || action.UserLocal == nil {
				t.Fatalf("%s action = %#v, want user-local before Linuxbrew", name, action)
			}
		}
	})

	t.Run("stays manual when Homebrew is unavailable", func(t *testing.T) {
		report, err := deps.Plan(*m, deps.Options{Profile: "core", OS: "linux"}, noProviderLookup("sudo", "apt-get"), fontLookupSet(), deps.TierDebian)
		if err != nil {
			t.Fatalf("Plan() error = %v", err)
		}

		githubCLI, ok := findAction(report.Actions, "GitHub CLI")
		if !ok {
			t.Fatalf("missing action for GitHub CLI in %#v", report.Actions)
		}
		if githubCLI.Status != deps.InstallActionStatusManual || githubCLI.Provider == deps.TierDebian || githubCLI.Executable == "sudo" || githubCLI.Manual == "" {
			t.Fatalf("GitHub CLI action = %#v, want manual guidance without Ubuntu apt installability", githubCLI)
		}

		for _, name := range []string{"bat", "starship", "zellij", "pnpm"} {
			action, ok := findAction(report.Actions, name)
			if !ok {
				t.Fatalf("missing action for %q in %#v", name, report.Actions)
			}
			if action.Provider != deps.TierUserLocal || action.UserLocal == nil {
				t.Fatalf("%s action = %#v, want user-local without Linuxbrew", name, action)
			}
		}
	})
}

func findAction(actions []deps.InstallAction, name string) (deps.InstallAction, bool) {
	for _, action := range actions {
		if action.Dependency == name {
			return action, true
		}
	}
	return deps.InstallAction{}, false
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

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "nvim"), fontLookupSet(), deps.TierHomebrew)
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
	report, err := deps.Plan(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux", "starship"), fontLookupSet(), deps.TierHomebrew)
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
				Tool: "claude",
				Tags: []string{"core"},
				Dependencies: []manifest.Dependency{
					{Name: "claude", Brew: "example/tools/claude"},
					{Name: "npx", Brew: "node"},
				},
			},
		},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "darwin"}, lookupSet("npx"), fontLookupSet(), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	action := report.Actions[0]
	if action.Dependency != "claude" {
		t.Fatalf("Actions[0].Dependency = %q, want claude", action.Dependency)
	}
	if action.Package != "example/tools/claude" {
		t.Fatalf("Actions[0].Package = %q, want tap package", action.Package)
	}
	if action.TrustCommand != "brew trust --formula example/tools/claude" {
		t.Fatalf("Actions[0].TrustCommand = %q, want formula trust guidance", action.TrustCommand)
	}
	if report.Items[0].TrustCommand != action.TrustCommand {
		t.Fatalf("Items[0].TrustCommand = %q, want %q", report.Items[0].TrustCommand, action.TrustCommand)
	}
}

func TestPlanDoesNotAddTapTrustGuidanceForCoreHomebrewFormula(t *testing.T) {
	report, err := deps.Plan(planManifest(), deps.Options{Profile: "default", OS: "darwin"}, lookupSet("tmux"), fontLookupSet(), deps.TierHomebrew)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	if report.Actions[0].TrustCommand != "" {
		t.Fatalf("TrustCommand = %q, want empty for core formula", report.Actions[0].TrustCommand)
	}
}

func TestPlanUsesUserLocalProviderBetweenDistroAndLinuxbrew(t *testing.T) {
	m := userLocalProviderManifest()

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 1 {
		t.Fatalf("Actions len = %d, want 1 (%#v)", len(report.Actions), report.Actions)
	}
	action := report.Actions[0]
	if action.Status != deps.InstallActionStatusInstallable || action.Provider != deps.TierUserLocal || action.UserLocal == nil {
		t.Fatalf("action = %#v, want installable user-local provider", action)
	}
	if action.UserLocal.Recipe != "uv" || action.UserLocal.Version != "0.11.25" || !strings.Contains(action.UserLocal.URL, "uv-x86_64-unknown-linux-gnu.tar.gz") {
		t.Fatalf("user-local artifact = %#v, want pinned uv linux amd64 artifact", action.UserLocal)
	}
	if len(action.Candidates) != 1 || action.Candidates[0].Provider != deps.TierUserLocal {
		t.Fatalf("Candidates = %#v, want selected user-local candidate", action.Candidates)
	}
}

func TestPlanKeepsDistroProviderBeforeUserLocal(t *testing.T) {
	m := userLocalProviderManifest()
	m.Entries[0].Dependencies[0].Apt = "uv"

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("sudo", "apt-get", "brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	action := report.Actions[0]
	if action.Provider != deps.TierDebian || action.Executable != "sudo" {
		t.Fatalf("action = %#v, want distro provider before user-local", action)
	}
	if len(action.Candidates) != 1 || action.Candidates[0].Provider != deps.TierDebian {
		t.Fatalf("Candidates = %#v, want selected distro candidate", action.Candidates)
	}
}

func userLocalProviderManifest() manifest.Manifest {
	return manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source: "configs/x", Target: "~/.x", Strategy: "symlink", Tags: []string{"core"},
			Dependencies: []manifest.Dependency{{
				Name:          "uv",
				Command:       "uv",
				Brew:          "uv",
				LinuxHomebrew: true,
				UserLocal: &manifest.UserLocalProvider{
					Recipe:  "uv",
					Version: "0.11.25",
					Checksums: map[string]string{
						"linux_amd64": "1db18b5e76fa645a7f3865773139bdec8e2d46adbdbb35e7410b34fa8015ccd2",
					},
				},
			}},
		}},
	}
}

func TestPlanResolvesPNPMUserLocalArtifact(t *testing.T) {
	m := userLocalProviderManifest()
	m.Entries[0].Dependencies[0] = manifest.Dependency{
		Name:          "pnpm",
		Command:       "pnpm",
		Brew:          "pnpm",
		LinuxHomebrew: true,
		UserLocal: &manifest.UserLocalProvider{
			Recipe:  "pnpm",
			Version: "11.9.0",
			Checksums: map[string]string{
				"linux_amd64": "69af6c012a5f12b4460f8e2280368cbe10551ab328516fc5b665f292b5991017",
				"linux_arm64": "ced48cd1bab413bdde54fee686eaa1c98bb50ee47a518c91d1decb5f2578737b",
			},
		},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	action := report.Actions[0]
	if action.Provider != deps.TierUserLocal || action.UserLocal == nil {
		t.Fatalf("action = %#v, want pnpm user-local provider", action)
	}
	if action.UserLocal.Recipe != "pnpm" || action.UserLocal.Command != "pnpm" || action.UserLocal.Layout != "single-binary" || action.UserLocal.URL != "https://registry.npmjs.org/@pnpm/linux-x64/-/linux-x64-11.9.0.tgz" {
		t.Fatalf("user-local artifact = %#v, want pinned pnpm linux amd64 artifact", action.UserLocal)
	}

	report, err = deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("pnpm", "brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() with present pnpm error = %v", err)
	}
	if len(report.Actions) != 0 {
		t.Fatalf("Actions = %#v, want no install when pnpm is present", report.Actions)
	}
}

func TestPlanResolvesBatUserLocalArtifact(t *testing.T) {
	m := userLocalProviderManifest()
	m.Entries[0].Dependencies[0] = manifest.Dependency{
		Name:          "bat",
		Command:       "bat",
		Brew:          "bat",
		LinuxHomebrew: true,
		Dnf:           "bat",
		Pacman:        "bat",
		UserLocal: &manifest.UserLocalProvider{
			Recipe:  "bat",
			Version: "v0.26.1",
			Checksums: map[string]string{
				"linux_amd64": "726f04c8f576a7fd18b7634f1bbf2f915c43494c1c0f013baa3287edb0d5a2a3",
				"linux_arm64": "422eb73e11c854fddd99f5ca8461c2f1d6e6dce0a2a8c3d5daade5ffcb6564aa",
			},
		},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	action := report.Actions[0]
	if action.Provider != deps.TierUserLocal || action.UserLocal == nil {
		t.Fatalf("action = %#v, want bat user-local provider", action)
	}
	if action.UserLocal.Recipe != "bat" || action.UserLocal.Command != "bat" || action.UserLocal.Layout != "single-binary" || !strings.Contains(action.UserLocal.URL, "bat-v0.26.1-x86_64-unknown-linux-gnu.tar.gz") {
		t.Fatalf("user-local artifact = %#v, want pinned bat linux amd64 artifact", action.UserLocal)
	}

	report, err = deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("sudo", "dnf", "brew"), fontLookupSet(), deps.TierFedora)
	if err != nil {
		t.Fatalf("Plan() with Fedora tier error = %v", err)
	}
	if got := report.Actions[0]; got.Provider != deps.TierFedora || got.UserLocal != nil {
		t.Fatalf("Fedora action = %#v, want distro provider before user-local", got)
	}

	report, err = deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("bat", "brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() with present bat error = %v", err)
	}
	if len(report.Actions) != 0 {
		t.Fatalf("Actions = %#v, want no install when bat is present", report.Actions)
	}
}

func TestPlanResolvesStarshipUserLocalArtifact(t *testing.T) {
	m := userLocalProviderManifest()
	m.Entries[0].Dependencies[0] = manifest.Dependency{
		Name:          "starship",
		Command:       "starship",
		Brew:          "starship",
		LinuxHomebrew: true,
		Dnf:           "starship",
		Pacman:        "starship",
		UserLocal: &manifest.UserLocalProvider{
			Recipe:  "starship",
			Version: "v1.25.1",
			Checksums: map[string]string{
				"linux_amd64": "4488c11ca632327d1f1f16fb2f102c0646094c35479cd5435991385da43c61ac",
				"linux_arm64": "01517aab398959ea9ea73bdb4f032ea4dbb51dff5c8e5eb05b4a1b9b7ab872b8",
			},
		},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	action := report.Actions[0]
	if action.Provider != deps.TierUserLocal || action.UserLocal == nil {
		t.Fatalf("action = %#v, want starship user-local provider", action)
	}
	if action.UserLocal.Recipe != "starship" || action.UserLocal.Layout != "single-binary" || !strings.Contains(action.UserLocal.URL, "starship-x86_64-unknown-linux-gnu.tar.gz") {
		t.Fatalf("user-local artifact = %#v, want pinned starship linux amd64 artifact", action.UserLocal)
	}

	report, err = deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("sudo", "dnf", "brew"), fontLookupSet(), deps.TierFedora)
	if err != nil {
		t.Fatalf("Plan() with Fedora tier error = %v", err)
	}
	if got := report.Actions[0]; got.Provider != deps.TierFedora || got.UserLocal != nil {
		t.Fatalf("Fedora action = %#v, want distro provider before user-local", got)
	}

	report, err = deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("starship", "brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() with present starship error = %v", err)
	}
	if len(report.Actions) != 0 {
		t.Fatalf("Actions = %#v, want no install when starship is present", report.Actions)
	}
}

func TestPlanResolvesNeovimUserLocalArtifact(t *testing.T) {
	m := userLocalProviderManifest()
	m.Entries[0].Dependencies[0] = manifest.Dependency{
		Name:          "neovim",
		Command:       "nvim",
		Brew:          "neovim",
		LinuxHomebrew: true,
		Dnf:           "neovim",
		Pacman:        "neovim",
		UserLocal: &manifest.UserLocalProvider{
			Recipe:  "nvim",
			Version: "v0.12.3",
			Checksums: map[string]string{
				"linux_amd64": "c441b547142860bf01bcce39e36cbed185c41112813e15443b16e5237750724d",
				"linux_arm64": "e055af73fa9c72b37456da8d204fa5c09850bc07e80e9176fe3b87d4afb7a3fc",
			},
		},
	}

	report, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	action := report.Actions[0]
	if action.Provider != deps.TierUserLocal || action.UserLocal == nil {
		t.Fatalf("action = %#v, want neovim user-local provider", action)
	}
	if action.UserLocal.Recipe != "nvim" || action.UserLocal.Command != "nvim" || action.UserLocal.Layout != "bundle" || action.UserLocal.URL != "https://github.com/neovim/neovim/releases/download/v0.12.3/nvim-linux-x86_64.tar.gz" {
		t.Fatalf("user-local artifact = %#v, want pinned Neovim linux amd64 bundle", action.UserLocal)
	}

	report, err = deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("sudo", "dnf", "brew"), fontLookupSet(), deps.TierFedora)
	if err != nil {
		t.Fatalf("Plan() with Fedora tier error = %v", err)
	}
	if got := report.Actions[0]; got.Provider != deps.TierFedora || got.UserLocal != nil {
		t.Fatalf("Fedora action = %#v, want distro provider before user-local", got)
	}

	report, err = deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("nvim", "brew"), fontLookupSet(), deps.TierDebian)
	if err != nil {
		t.Fatalf("Plan() with present nvim error = %v", err)
	}
	if len(report.Actions) != 0 {
		t.Fatalf("Actions = %#v, want no install when nvim is present", report.Actions)
	}
}

func TestPlanRejectsInvalidUserLocalOptIn(t *testing.T) {
	tests := []struct {
		name string
		dep  manifest.Dependency
		want string
	}{
		{
			name: "unknown recipe",
			dep:  manifest.Dependency{Name: "bad", Command: "bad", UserLocal: &manifest.UserLocalProvider{Recipe: "bad", Version: "1.0.0", Checksum: "abc"}},
			want: "unsupported user_local recipe",
		},
		{
			name: "missing version",
			dep:  manifest.Dependency{Name: "uv", Command: "uv", UserLocal: &manifest.UserLocalProvider{Recipe: "uv", Checksum: "abc"}},
			want: "user_local.version is required",
		},
		{
			name: "missing checksum",
			dep:  manifest.Dependency{Name: "uv", Command: "uv", UserLocal: &manifest.UserLocalProvider{Recipe: "uv", Version: "0.11.25"}},
			want: "user_local checksum is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := userLocalProviderManifest()
			m.Entries[0].Dependencies[0] = tt.dep

			_, err := deps.Plan(m, deps.Options{Profile: "default", OS: "linux", Arch: "amd64"}, lookupSet("brew"), fontLookupSet(), deps.TierDebian)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Plan() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
