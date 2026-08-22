package selectedsurface_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestEvaluateFiltersByTagAndOSInManifestOrder(t *testing.T) {
	m := manifest.Manifest{
		Dependencies: []manifest.DependencySet{
			{Tags: []string{"linux"}, OS: []string{"linux"}, Dependencies: []manifest.Dependency{{Name: "linux-set"}}},
			{Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "core-set"}}},
			{Tags: []string{"core"}, OS: []string{"darwin"}, Dependencies: []manifest.Dependency{{Name: "darwin-set"}}},
		},
		Entries: []manifest.Entry{
			{Source: "skip", Target: "skip", Strategy: "copy", Tags: []string{"other"}},
			{Source: "core", Target: "core", Strategy: "copy", Tags: []string{"core"}},
			{Source: "linux", Target: "linux", Strategy: "copy", Tags: []string{"linux"}, OS: []string{"linux"}},
		},
		Provisioners: []manifest.Provisioner{
			{Tool: "core-tool", Tags: []string{"core"}},
			{Tool: "darwin-tool", Tags: []string{"core"}, OS: []string{"darwin"}},
		},
	}

	got := selectedsurface.Evaluate(m, []string{"core", "linux", "core"}, "linux")
	if want := []string{"core", "linux"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("Tags = %#v, want %#v", got.Tags, want)
	}
	if got.DependencySets[0].Dependencies[0].Name != "linux-set" || got.DependencySets[1].Dependencies[0].Name != "core-set" {
		t.Fatalf("DependencySets order = %#v", got.DependencySets)
	}
	if got.Entries[0].Entry.Target != "core" || got.Entries[1].Entry.Target != "linux" {
		t.Fatalf("Entries order = %#v", got.Entries)
	}
	if len(got.Provisioners) != 1 || got.Provisioners[0].Tool != "core-tool" {
		t.Fatalf("Provisioners = %#v", got.Provisioners)
	}
}

func TestEvaluateDeduplicatesOnlyExactSelectedDeclarations(t *testing.T) {
	set := manifest.DependencySet{Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "set"}}}
	entry := manifest.Entry{Source: "a", Target: "shared", Strategy: "copy", Tags: []string{"core"}}
	provisioner := manifest.Provisioner{Tool: "tool", Tags: []string{"core"}}
	m := manifest.Manifest{
		Dependencies: []manifest.DependencySet{set, set, {Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "different"}}}},
		Entries:      []manifest.Entry{entry, entry, {Source: "b", Target: "shared", Strategy: "copy", Tags: []string{"core"}}},
		Provisioners: []manifest.Provisioner{provisioner, provisioner, {Tool: "tool", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Scope: "user"}}},
	}
	got := selectedsurface.Evaluate(m, []string{"core"}, "linux")
	if len(got.DependencySets) != 2 || len(got.Entries) != 2 || len(got.Provisioners) != 2 {
		t.Fatalf("deduplicated surface = %#v", got)
	}
}

func TestEvaluateDependenciesPreservesOccurrencesAndPromotesRequired(t *testing.T) {
	m := manifest.Manifest{
		Dependencies: []manifest.DependencySet{{Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: " tool ", Requirement: "optional"}, {Name: "set-only"}}}},
		Entries:      []manifest.Entry{{Source: "entry", Target: "target", Strategy: "copy", Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "tool", Requirement: "required"}, {Name: "entry-only"}}}},
		Provisioners: []manifest.Provisioner{{Tool: "prov", Tags: []string{"core"}, Dependencies: []manifest.Dependency{{Name: "tool"}, {Name: "prov-only"}}}},
	}
	got := selectedsurface.Evaluate(m, []string{"core"}, "linux")
	if want := []string{"tool", "set-only", "entry-only", "prov-only"}; dependencyNames(got.Dependencies) == nil || !reflect.DeepEqual(dependencyNames(got.Dependencies), want) {
		t.Fatalf("Dependencies = %#v, want %#v", got.Dependencies, want)
	}
	if !got.Dependencies[0].IsRequired() {
		t.Fatalf("first dependency = %#v, want required after promotion", got.Dependencies[0])
	}
	if want := []string{"dependency_set", "dependency_set", "entry", "entry", "provisioner", "provisioner"}; !reflect.DeepEqual(originTypes(got.DependencyOrigins), want) {
		t.Fatalf("DependencyOrigins = %#v", got.DependencyOrigins)
	}
	if got.DependencyOrigins[0].Dependency.Name != "tool" || got.DependencyOrigins[2].Origin.Name != "target" || got.DependencyOrigins[4].Origin.Name != "prov" {
		t.Fatalf("DependencyOrigins do not retain declaration and origin: %#v", got.DependencyOrigins)
	}
}

func TestEvaluateResolvesLastTagOverrideAndKeepsActiveOverrides(t *testing.T) {
	selected := manifest.Entry{Source: "base", Target: "selected", Strategy: "copy", Tags: []string{"core"}, SourceOverrides: map[string]string{"theme": "theme-source", "work": "work-source"}}
	unselected := manifest.Entry{Source: "other", Target: "unselected", Strategy: "copy", Tags: []string{"other"}, SourceOverrides: map[string]string{"theme": "other-theme"}}
	m := manifest.Manifest{Entries: []manifest.Entry{selected, unselected, {Source: "darwin", Target: "darwin", Strategy: "copy", Tags: []string{"core"}, OS: []string{"darwin"}, SourceOverrides: map[string]string{"theme": "skip"}}}}
	got := selectedsurface.Evaluate(m, []string{"core", "theme", "work"}, "linux")
	if len(got.Entries) != 1 || got.Entries[0].Source != "work-source" || got.Entries[0].OverrideTag != "work" {
		t.Fatalf("Entries = %#v", got.Entries)
	}
	if want := []string{"theme-source", "work-source", "other-theme"}; !reflect.DeepEqual(overrideSources(got.SourceOverrides), want) {
		t.Fatalf("SourceOverrides = %#v, want %#v", got.SourceOverrides, want)
	}
}

func TestEvaluatePreservesAndCopiesEntrySourceOverrides(t *testing.T) {
	m := manifest.Manifest{Entries: []manifest.Entry{{
		Source: "base", SourceOverrides: map[string]string{"theme": "theme-source"}, Target: "target", Strategy: "copy", Tags: []string{"core"},
	}}}
	got := selectedsurface.Evaluate(m, []string{"core", "theme"}, "linux")
	if len(got.Entries) != 1 || got.Entries[0].Entry.SourceOverrides["theme"] != "theme-source" {
		t.Fatalf("Entries = %#v, want preserved source override declaration", got.Entries)
	}
	got.Entries[0].Entry.SourceOverrides["theme"] = "changed"
	if m.Entries[0].SourceOverrides["theme"] != "theme-source" {
		t.Fatalf("Evaluate result mutated manifest source overrides: %#v", m.Entries[0].SourceOverrides)
	}
}

func TestEvaluateEntriesPreservesSkippedScopeAndResolvedSource(t *testing.T) {
	selected := manifest.Entry{Source: "base", SourceOverrides: map[string]string{"theme": "theme-source"}, Target: "selected", Strategy: "copy", Tags: []string{"core"}}
	skipped := manifest.Entry{Source: "darwin", SourceOverrides: map[string]string{"theme": "darwin-theme"}, Target: "skipped", Strategy: "copy", Tags: []string{"core"}, OS: []string{"darwin"}}
	m := manifest.Manifest{Entries: []manifest.Entry{selected, selected, skipped, {Source: "other", Target: "other", Strategy: "copy", Tags: []string{"other"}}}}
	got := selectedsurface.EvaluateEntries(m, []string{"core", "theme"}, "linux")
	if len(got) != 2 || !got[0].Applicable || got[0].Source != "theme-source" || got[1].Applicable || got[1].Source != "darwin-theme" {
		t.Fatalf("Entry scope = %#v, want de-duplicated selected and OS-skipped entries with resolved sources", got)
	}
}

func TestEvaluateSameTagsHaveNoProvenanceInputOrOutput(t *testing.T) {
	m := manifest.Manifest{Entries: []manifest.Entry{{Source: "base", Target: "target", Strategy: "copy", Tags: []string{"core"}}}}
	first := selectedsurface.Evaluate(m, []string{"core"}, "linux")
	second := selectedsurface.Evaluate(m, []string{"core"}, "linux")
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same effective tags must have the same pure surface: %#v != %#v", first, second)
	}
}

func TestRepositoryProfilesMatchEquivalentExplicitTagsAcrossSupportedOS(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for name, profile := range m.Profiles {
		profileSelection, err := manifest.ResolveReadOnlySelection(*m, []string{name}, nil)
		if err != nil {
			t.Fatalf("resolve Profile %q: %v", name, err)
		}
		tagSelection, err := manifest.ResolveReadOnlySelection(*m, nil, profile.Tags)
		if err != nil {
			t.Fatalf("resolve explicit Tags for %q: %v", name, err)
		}
		for _, osName := range []string{"darwin", "linux"} {
			t.Run(name+"/"+osName, func(t *testing.T) {
				fromProfile := selectedsurface.Evaluate(*m, profileSelection.Tags, osName)
				fromTags := selectedsurface.Evaluate(*m, tagSelection.Tags, osName)
				if !reflect.DeepEqual(fromProfile, fromTags) {
					t.Fatalf("Profile and explicit Tag surfaces differ\nProfile: %#v\nTags: %#v", fromProfile, fromTags)
				}
			})
		}
	}
}

func TestRepositoryAtomicCapabilityTagsSelectOnlyTheirCapabilities(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		tag          string
		osName       string
		entries      []string
		dependencies []string
		provisioners []string
	}{
		{tag: "zsh", osName: "linux", entries: []string{"~/.zshrc", "~/.config/dots/zsh/zshrc", "~/.zshenv"}, dependencies: []string{"zsh"}},
		{tag: "zimfw", osName: "linux", entries: []string{"~/.zimrc"}, dependencies: []string{"zsh", "git", "curl"}, provisioners: []string{"zimfw"}},
		{tag: "git", osName: "linux", entries: []string{"~/.gitconfig", "~/.config/dots/git/gitconfig"}, dependencies: []string{"git"}},
		{tag: "starship", osName: "linux", entries: []string{"~/.config/starship.toml"}, dependencies: []string{"starship"}},
		{tag: "tmux", osName: "linux", entries: []string{"~/.config/dots/theme.sh", "~/.tmux.conf"}, dependencies: []string{"tmux"}},
		{tag: "herdr", osName: "darwin", entries: []string{"~/.config/herdr/config.toml"}, dependencies: []string{"herdr"}},
		{tag: "herdr", osName: "linux"},
		{tag: "zellij", osName: "linux", entries: []string{"~/.config/zellij/config.kdl", "~/.config/zellij/layouts/default.kdl"}, dependencies: []string{"zellij"}},
		{tag: "atuin", osName: "linux", entries: []string{"~/.config/atuin/config.toml", "~/.config/atuin/themes/catppuccin-mocha.toml"}, dependencies: []string{"atuin"}},
		{tag: "neovim", osName: "linux", entries: []string{"~/.config/dots/theme.sh", "nvim/lazy-lock.json", "~/.config/nvim/init.lua", "~/.config/dots/nvim"}, dependencies: []string{"neovim"}},
		{tag: "tuicr", osName: "linux", entries: []string{"~/.config/tuicr/config.toml"}, dependencies: []string{"tuicr"}},
		{tag: "bat", osName: "linux", entries: []string{"~/.config/bat/config"}, dependencies: []string{"bat"}},
		{tag: "node", osName: "linux", dependencies: []string{"Node LTS (fnm)", "unzip"}},
		{tag: "rust", osName: "linux", dependencies: []string{"Rust stable (rustup)"}},
		{tag: "go", osName: "linux", dependencies: []string{"go"}},
		{tag: "uv", osName: "linux", dependencies: []string{"uv"}},
		{tag: "pnpm", osName: "linux", dependencies: []string{"pnpm"}},
		{tag: "bun", osName: "linux", dependencies: []string{"bun"}},
		{tag: "fzf", osName: "linux", dependencies: []string{"fzf"}},
		{tag: "zoxide", osName: "linux", dependencies: []string{"zoxide"}},
		{tag: "lazygit", osName: "linux", dependencies: []string{"lazygit"}},
		{tag: "eza", osName: "linux", dependencies: []string{"eza"}},
		{tag: "ripgrep", osName: "linux", dependencies: []string{"ripgrep"}},
		{tag: "delta", osName: "linux", dependencies: []string{"delta"}},
		{tag: "fd", osName: "linux", dependencies: []string{"fd"}},
		{tag: "gh", osName: "linux", dependencies: []string{"GitHub CLI"}},
		{tag: "jq", osName: "linux", dependencies: []string{"jq"}},
		{tag: "ghostty", osName: "linux", entries: []string{"~/.config/ghostty/config.ghostty"}, dependencies: []string{"Desktop Nerd Font", "ghostty"}},
		{tag: "warp", osName: "linux", entries: []string{"~/.config/warp-terminal/settings.toml", "~/.config/warp-terminal/keybindings.yaml"}, dependencies: []string{"Desktop Nerd Font", "Warp"}},
		{tag: "zed", osName: "linux", entries: []string{"~/.config/zed/settings.json", "~/.config/zed/keymap.json", "~/.config/zed/themes/catppuccin-blue.json"}, dependencies: []string{"Desktop Nerd Font", "zed"}},
		{tag: "codex", osName: "linux", entries: []string{"~/.codex/config.toml"}, dependencies: []string{"Codex"}},
		{tag: "claude", osName: "linux", entries: []string{"~/.claude/settings.json", "~/.claude/statusline-command.sh"}, dependencies: []string{"Claude Code", "jq"}},
		{tag: "opencode", osName: "linux", entries: []string{"~/.config/opencode/opencode.json"}, dependencies: []string{"OpenCode"}},
		{tag: "antigravity", osName: "linux", entries: []string{"~/.gemini/antigravity-cli/settings.json"}, dependencies: []string{"Antigravity"}},
		{tag: "copilot", osName: "linux", entries: []string{"~/.copilot/settings.json", "~/.copilot/statusline-command.sh"}, dependencies: []string{"Copilot CLI", "jq"}},
		{tag: "playwright", osName: "linux", dependencies: []string{"Playwright CLI", "npx"}, provisioners: []string{"skills"}},
		{tag: "frontend-design", osName: "linux", dependencies: []string{"npx"}, provisioners: []string{"skills"}},
		{tag: "vercel-web-skills", osName: "linux", dependencies: []string{"npx"}, provisioners: []string{"skills"}},
		{tag: "claude-chrome-devtools", osName: "linux", dependencies: []string{"claude"}, provisioners: []string{"claude", "claude"}},
		{tag: "codex-chrome-devtools", osName: "linux", dependencies: []string{"codex"}, provisioners: []string{"codex"}},
		{tag: "opencode-chrome-devtools", osName: "linux", entries: []string{"~/.config/opencode/opencode.json"}, dependencies: []string{"opencode"}},
		{tag: "dart-skills", osName: "linux", dependencies: []string{"npx"}, provisioners: []string{"skills"}},
		{tag: "flutter-skills", osName: "linux", dependencies: []string{"npx"}, provisioners: []string{"skills"}},
		{tag: "android-skills", osName: "linux", dependencies: []string{"npx"}, provisioners: []string{"skills"}},
		{tag: "claude-dart-mcp", osName: "linux", dependencies: []string{"claude", "dart"}, provisioners: []string{"claude"}},
		{tag: "codex-dart-mcp", osName: "linux", dependencies: []string{"codex", "dart"}, provisioners: []string{"codex"}},
		{tag: "antigravity-dart-mcp", osName: "linux", entries: []string{"~/.gemini/antigravity-cli/settings.json"}},
		{tag: "vscode-mobile", osName: "linux", entries: []string{"~/.config/Code/User/settings.json"}},
	}

	for _, tt := range tests {
		t.Run(tt.tag+"/"+tt.osName, func(t *testing.T) {
			surface := selectedsurface.Evaluate(*m, []string{tt.tag}, tt.osName)
			if got := selectedTargets(surface.Entries); !reflect.DeepEqual(got, nonNil(tt.entries)) {
				t.Errorf("Managed Entries = %#v, want %#v", got, nonNil(tt.entries))
			}
			if got := dependencyNames(surface.Dependencies); !reflect.DeepEqual(got, nonNil(tt.dependencies)) {
				t.Errorf("Dependencies = %#v, want %#v", got, nonNil(tt.dependencies))
			}
			if got := provisionerTools(surface.Provisioners); !reflect.DeepEqual(got, nonNil(tt.provisioners)) {
				t.Errorf("Provisioners = %#v, want %#v", got, nonNil(tt.provisioners))
			}
		})
	}
}

func TestRepositoryDesktopAndAgentProfilesPreservePreAtomizationSurfaces(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		profile      string
		osName       string
		entries      []string
		dependencies []string
	}{
		{
			profile: "desktop",
			osName:  "darwin",
			entries: []string{
				"~/.config/ghostty/config.ghostty", "~/.warp/settings.toml", "~/.warp/keybindings.yaml",
				"~/.config/zed/settings.json", "~/.config/zed/keymap.json", "~/.config/zed/themes/catppuccin-blue.json",
			},
			dependencies: []string{"Desktop Nerd Font", "CodexBar", "ghostty", "zed"},
		},
		{
			profile: "desktop",
			osName:  "linux",
			entries: []string{
				"~/.config/ghostty/config.ghostty", "~/.config/warp-terminal/settings.toml", "~/.config/warp-terminal/keybindings.yaml",
				"~/.config/zed/settings.json", "~/.config/zed/keymap.json", "~/.config/zed/themes/catppuccin-blue.json",
			},
			dependencies: []string{"Desktop Nerd Font", "ghostty", "Warp", "zed"},
		},
		{
			profile: "agents",
			osName:  "darwin",
			entries: []string{
				"~/.claude/settings.json", "~/.claude/statusline-command.sh", "~/.codex/config.toml",
				"~/.copilot/settings.json", "~/.copilot/statusline-command.sh", "~/.gemini/antigravity-cli/settings.json",
				"~/.config/opencode/opencode.json",
			},
			dependencies: []string{"Codex", "Claude Code", "OpenCode", "Antigravity", "Copilot CLI", "jq"},
		},
		{
			profile: "agents",
			osName:  "linux",
			entries: []string{
				"~/.claude/settings.json", "~/.claude/statusline-command.sh", "~/.codex/config.toml",
				"~/.copilot/settings.json", "~/.copilot/statusline-command.sh", "~/.gemini/antigravity-cli/settings.json",
				"~/.config/opencode/opencode.json",
			},
			dependencies: []string{"Codex", "Claude Code", "OpenCode", "Antigravity", "Copilot CLI", "jq"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.profile+"/"+tt.osName, func(t *testing.T) {
			selection, err := manifest.ResolveReadOnlySelection(*m, []string{tt.profile}, nil)
			if err != nil {
				t.Fatal(err)
			}
			surface := selectedsurface.Evaluate(*m, selection.Tags, tt.osName)
			if got := selectedTargets(surface.Entries); !reflect.DeepEqual(got, tt.entries) {
				t.Errorf("Managed Entries changed from the pre-atomization surface\ngot:  %#v\nwant: %#v", got, tt.entries)
			}
			if got := dependencyNames(surface.Dependencies); !reflect.DeepEqual(got, tt.dependencies) {
				t.Errorf("Dependencies changed from the pre-atomization surface\ngot:  %#v\nwant: %#v", got, tt.dependencies)
			}
			if got := provisionerTools(surface.Provisioners); len(got) != 0 {
				t.Errorf("Provisioners changed from the pre-atomization surface: %#v", got)
			}
		})
	}
}

func TestRepositoryWebAndMobileProfilesPreservePreAtomizationSurfaces(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		profile      string
		osName       string
		entries      []string
		dependencies []string
		provisioners []string
	}{
		{
			profile:      "web",
			osName:       "darwin",
			entries:      []string{"~/.config/opencode/opencode.json"},
			dependencies: []string{"Playwright CLI", "opencode", "npx", "claude", "codex"},
			provisioners: []string{"skills", "skills", "skills", "claude", "claude", "codex"},
		},
		{
			profile:      "web",
			osName:       "linux",
			entries:      []string{"~/.config/opencode/opencode.json"},
			dependencies: []string{"Playwright CLI", "opencode", "npx", "claude", "codex"},
			provisioners: []string{"skills", "skills", "skills", "claude", "claude", "codex"},
		},
		{
			profile:      "mobile",
			osName:       "darwin",
			entries:      []string{"~/.gemini/antigravity-cli/settings.json", "~/Library/Application Support/Code/User/settings.json"},
			dependencies: []string{"npx", "claude", "dart", "codex"},
			provisioners: []string{"skills", "skills", "skills", "claude", "codex"},
		},
		{
			profile:      "mobile",
			osName:       "linux",
			entries:      []string{"~/.gemini/antigravity-cli/settings.json", "~/.config/Code/User/settings.json"},
			dependencies: []string{"npx", "claude", "dart", "codex"},
			provisioners: []string{"skills", "skills", "skills", "claude", "codex"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.profile+"/"+tt.osName, func(t *testing.T) {
			selection, err := manifest.ResolveReadOnlySelection(*m, []string{tt.profile}, nil)
			if err != nil {
				t.Fatal(err)
			}
			surface := selectedsurface.Evaluate(*m, selection.Tags, tt.osName)
			if got := selectedTargets(surface.Entries); !reflect.DeepEqual(got, tt.entries) {
				t.Errorf("Managed Entries changed from the pre-atomization surface\ngot:  %#v\nwant: %#v", got, tt.entries)
			}
			if got := dependencyNames(surface.Dependencies); !reflect.DeepEqual(got, tt.dependencies) {
				t.Errorf("Dependencies changed from the pre-atomization surface\ngot:  %#v\nwant: %#v", got, tt.dependencies)
			}
			if got := provisionerTools(surface.Provisioners); !reflect.DeepEqual(got, tt.provisioners) {
				t.Errorf("Provisioners changed from the pre-atomization surface\ngot:  %#v\nwant: %#v", got, tt.provisioners)
			}
		})
	}
}

func TestRepositoryCoreProfilePreservesPreAtomizationSurface(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	selection, err := manifest.ResolveReadOnlySelection(*m, []string{"core"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	wantEntries := map[string][]string{
		"darwin": {"~/.zshrc", "~/.config/dots/zsh/zshrc", "~/.zimrc", "~/.zshenv", "~/.gitconfig", "~/.config/dots/git/gitconfig", "~/.config/tuicr/config.toml", "~/.config/dots/theme.sh", "~/.config/starship.toml", "~/.tmux.conf", "~/.config/herdr/config.toml", "~/.config/zellij/config.kdl", "~/.config/zellij/layouts/default.kdl", "~/.config/atuin/config.toml", "~/.config/atuin/themes/catppuccin-mocha.toml", "~/.config/bat/config", "nvim/lazy-lock.json", "~/.config/nvim/init.lua", "~/.config/dots/nvim"},
		"linux":  {"~/.zshrc", "~/.config/dots/zsh/zshrc", "~/.zimrc", "~/.zshenv", "~/.gitconfig", "~/.config/dots/git/gitconfig", "~/.config/tuicr/config.toml", "~/.config/dots/theme.sh", "~/.config/starship.toml", "~/.tmux.conf", "~/.config/zellij/config.kdl", "~/.config/zellij/layouts/default.kdl", "~/.config/atuin/config.toml", "~/.config/atuin/themes/catppuccin-mocha.toml", "~/.config/bat/config", "nvim/lazy-lock.json", "~/.config/nvim/init.lua", "~/.config/dots/nvim"},
	}
	wantDependencies := map[string][]string{
		"darwin": {"Node LTS (fnm)", "Rust stable (rustup)", "go", "uv", "pnpm", "bun", "fzf", "zoxide", "lazygit", "eza", "ripgrep", "delta", "unzip", "fd", "GitHub CLI", "jq", "zsh", "git", "tuicr", "starship", "tmux", "herdr", "zellij", "atuin", "bat", "neovim", "curl"},
		"linux":  {"Node LTS (fnm)", "Rust stable (rustup)", "go", "uv", "pnpm", "bun", "fzf", "zoxide", "lazygit", "eza", "ripgrep", "delta", "unzip", "fd", "GitHub CLI", "jq", "zsh", "git", "tuicr", "starship", "tmux", "zellij", "atuin", "bat", "neovim", "curl"},
	}

	for _, osName := range []string{"darwin", "linux"} {
		t.Run(osName, func(t *testing.T) {
			surface := selectedsurface.Evaluate(*m, selection.Tags, osName)
			if got := selectedTargets(surface.Entries); !reflect.DeepEqual(got, wantEntries[osName]) {
				t.Errorf("Managed Entries changed from the pre-atomization surface\ngot:  %#v\nwant: %#v", got, wantEntries[osName])
			}
			if got := dependencyNames(surface.Dependencies); !reflect.DeepEqual(got, wantDependencies[osName]) {
				t.Errorf("Dependencies changed from the pre-atomization surface\ngot:  %#v\nwant: %#v", got, wantDependencies[osName])
			}
			if got := provisionerTools(surface.Provisioners); !reflect.DeepEqual(got, []string{"zimfw"}) {
				t.Errorf("Provisioners = %#v, want zimfw", got)
			}
		})
	}
}

func selectedTargets(entries []selectedsurface.SelectedEntry) []string {
	result := make([]string, len(entries))
	for i, entry := range entries {
		result[i] = entry.Entry.Target
	}
	return result
}

func provisionerTools(provisioners []manifest.Provisioner) []string {
	result := make([]string, len(provisioners))
	for i, provisioner := range provisioners {
		result[i] = provisioner.Tool
	}
	return result
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func dependencyNames(dependencies []manifest.Dependency) []string {
	names := make([]string, len(dependencies))
	for i, dependency := range dependencies {
		names[i] = dependency.Name
	}
	return names
}

func originTypes(origins []selectedsurface.DependencyOrigin) []string {
	types := make([]string, len(origins))
	for i, origin := range origins {
		types[i] = origin.Origin.Type
	}
	return types
}

func overrideSources(overrides []selectedsurface.SourceOverride) []string {
	sources := make([]string, len(overrides))
	for i, override := range overrides {
		sources[i] = override.Source
	}
	return sources
}
