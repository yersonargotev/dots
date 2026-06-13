package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
)

func TestLoadFileAcceptsMinimalValidManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if got.Version != 1 {
		t.Fatalf("Version = %d, want 1", got.Version)
	}
	if len(got.Profiles) != 1 || len(got.Profiles["default"].Tags) != 1 || got.Profiles["default"].Tags[0] != "core" {
		t.Fatalf("Profiles = %#v, want default profile with core tag", got.Profiles)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(got.Entries))
	}
	entry := got.Entries[0]
	if entry.Source != "configs/zsh/zshrc" || entry.Target != "~/.zshrc" || entry.Strategy != "symlink" {
		t.Fatalf("Entry = %#v, want parsed source, target, and strategy", entry)
	}
}

func TestLoadFileParsesEntryDependencies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - name: tmux
        brew: tmux
        apt: tmux
        dnf: tmux
        pacman: tmux
      - name: ripgrep
        command: rg
        brew: ripgrep
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	deps := got.Entries[0].Dependencies
	if len(deps) != 2 {
		t.Fatalf("Dependencies len = %d, want 2", len(deps))
	}
	if deps[0].Name != "tmux" || deps[0].Brew != "tmux" || deps[0].Apt != "tmux" || deps[0].Dnf != "tmux" || deps[0].Pacman != "tmux" {
		t.Fatalf("Dependencies[0] = %#v, want fully mapped tmux dependency", deps[0])
	}
	if deps[1].Name != "ripgrep" || deps[1].Command != "rg" || deps[1].Brew != "ripgrep" {
		t.Fatalf("Dependencies[1] = %#v, want ripgrep with rg command", deps[1])
	}
}

func TestDependencyProbeTrimsWhitespace(t *testing.T) {
	tests := []struct {
		name string
		dep  manifest.Dependency
		want string
	}{
		{name: "defaults to name", dep: manifest.Dependency{Name: "tmux"}, want: "tmux"},
		{name: "command overrides name", dep: manifest.Dependency{Name: "ripgrep", Command: "rg"}, want: "rg"},
		{name: "trims padded command", dep: manifest.Dependency{Name: "ripgrep", Command: " rg "}, want: "rg"},
		{name: "trims padded name", dep: manifest.Dependency{Name: " neovim "}, want: "neovim"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dep.Probe(); got != tt.want {
				t.Fatalf("Probe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadFileRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategey: symlink
    tags: [core]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := manifest.LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want error for unknown field")
	}
	if !strings.Contains(err.Error(), "strategey") {
		t.Fatalf("LoadFile() error = %q, want it to name the unknown field %q", err.Error(), "strategey")
	}
}

func TestLoadFileRejectsInvalidManifests(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "missing version",
			content: `profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: "version is required",
		},
		{
			name: "profile without tags",
			content: `version: 1
profiles:
  default:
    tags: []
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: `profiles["default"].tags is required`,
		},
		{
			name: "profile with empty tag",
			content: `version: 1
profiles:
  default:
    tags: ["", core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: `profiles["default"].tags[0] must not be empty`,
		},
		{
			name: "profile with whitespace-only tag",
			content: `version: 1
profiles:
  default:
    tags: ["  ", core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: `profiles["default"].tags[0] must not be empty`,
		},
		{
			name: "unsupported strategy",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: shell
    tags: [core]
`,
			want: "entries[0].strategy must be one of copy, symlink, template",
		},
		{
			name: "unsupported os filter",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: copy
    tags: [core]
    os: [windows]
`,
			want: "entries[0].os[0] must be one of darwin, linux",
		},
		{
			name: "entry with empty tag",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: copy
    tags: ["", core]
`,
			want: "entries[0].tags[0] must not be empty",
		},
		{
			name: "entry with whitespace-only tag",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: copy
    tags: ["  ", core]
`,
			want: "entries[0].tags[0] must not be empty",
		},
		{
			name: "dependency without name",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - brew: tmux
`,
			want: "entries[0].dependencies[0].name is required",
		},
		{
			name: "dependency with whitespace-only name",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - name: "  "
        brew: tmux
`,
			want: "entries[0].dependencies[0].name is required",
		},
		{
			name: "dependency with whitespace-only command",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - name: tmux
        command: "  "
`,
			want: `entries[0].dependencies[0].command must not be empty`,
		},
		{
			name: "missing entry target",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    strategy: copy
    tags: [core]
`,
			want: "entries[0].target is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "dots.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			_, err := manifest.LoadFile(path)
			if err == nil {
				t.Fatal("LoadFile() error = nil, want validation error")
			}
			if err.Error() != tt.want {
				t.Fatalf("LoadFile() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRepositoryManifestIncludesMVPConfigurationSet(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	entriesByTarget := map[string]manifest.Entry{}
	for _, entry := range got.Entries {
		entriesByTarget[entry.Target] = entry
	}

	tests := []struct {
		name     string
		target   string
		source   string
		strategy string
		dep      string
	}{
		{name: "zsh", target: "~/.zshrc", source: "configs/zsh/zshrc", strategy: "symlink", dep: "zsh"},
		{name: "git", target: "~/.gitconfig", source: "configs/git/gitconfig", strategy: "symlink", dep: "git"},
		{name: "starship", target: "~/.config/starship.toml", source: "configs/starship/starship.toml", strategy: "symlink", dep: "starship"},
		{name: "tmux", target: "~/.tmux.conf", source: "configs/tmux/tmux.conf", strategy: "symlink", dep: "tmux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := entriesByTarget[tt.target]
			if !ok {
				t.Fatalf("repository manifest missing MVP entry for target %q", tt.target)
			}
			if entry.Source != tt.source {
				t.Errorf("Source = %q, want %q", entry.Source, tt.source)
			}
			if entry.Strategy != tt.strategy {
				t.Errorf("Strategy = %q, want %q", entry.Strategy, tt.strategy)
			}
			if !hasString(entry.Tags, "core") {
				t.Errorf("Tags = %#v, want core tag", entry.Tags)
			}
			if !sameStrings(entry.OS, []string{"darwin", "linux"}) {
				t.Errorf("OS = %#v, want [darwin linux]", entry.OS)
			}
			if !hasDependency(entry.Dependencies, tt.dep) {
				t.Errorf("Dependencies = %#v, want %q", entry.Dependencies, tt.dep)
			}
		})
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasDependency(deps []manifest.Dependency, want string) bool {
	for _, dep := range deps {
		if dep.Name == want {
			return true
		}
	}
	return false
}

func TestRepositoryManagedConfigsExposeLocalExtensionPoints(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	tests := []struct {
		name     string
		path     string
		contains []string
	}{
		{
			name: "zsh has private local include",
			path: "configs/zsh/zshrc",
			contains: []string{
				"Machine-specific overrides and secrets",
				".zshrc.local",
			},
		},
		{
			name: "git has private local include",
			path: "configs/git/gitconfig",
			contains: []string{
				"Machine-specific identity belongs in ~/.gitconfig.local",
				"path = ~/.gitconfig.local",
			},
		},
		{
			name: "tmux has private local include",
			path: "configs/tmux/tmux.conf",
			contains: []string{
				"Machine-specific overrides and secrets",
				"source-file ~/.tmux.conf.local",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(root, tt.path))
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", tt.path, err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(string(content), want) {
					t.Fatalf("%s does not contain %q", tt.path, want)
				}
			}
		})
	}
}

func TestRepositoryGitConfigClassifiesPortableAndLocalConcerns(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	managedPath := filepath.Join(root, "configs/git/gitconfig")
	managedBytes, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", managedPath, err)
	}
	managed := string(managedBytes)

	for _, want := range []string{
		"defaultBranch = main",
		"rebase = false",
		"conflictStyle = diff3",
		"path = ~/.gitconfig.local",
	} {
		if !strings.Contains(managed, want) {
			t.Fatalf("managed gitconfig missing portable/local boundary %q:\n%s", want, managed)
		}
	}

	normalizedManaged := strings.ToLower(managed)
	for _, forbidden := range []string{
		"[user]",
		"signingkey",
		"gpgsign",
		"[credential]",
		"credential.helper",
		"[credential ",
		"includeif \"gitdir:~/",
		"includeif.gitdir:",
		"/users/",
		"/home/",
		"~/documents/",
	} {
		if strings.Contains(normalizedManaged, forbidden) {
			t.Fatalf("managed gitconfig contains local/private concern %q:\n%s", forbidden, managed)
		}
	}

	docPath := filepath.Join(root, "configs/git/README.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", docPath, err)
	}
	doc := string(docBytes)
	for _, want := range []string{
		"Portable",
		"Local/private",
		"Generated",
		"Machine-specific",
		"gitconfig.local.example",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("git config documentation missing %q:\n%s", want, doc)
		}
	}

	examplePath := filepath.Join(root, "configs/git/gitconfig.local.example")
	exampleBytes, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", examplePath, err)
	}
	example := string(exampleBytes)
	for _, want := range []string{
		"[user]",
		"name =",
		"email =",
		"signingKey =",
		"[credential]",
		"[includeIf \"gitdir:~/work/\"]",
	} {
		if !strings.Contains(example, want) {
			t.Fatalf("gitconfig.local.example missing %q:\n%s", want, example)
		}
	}
	for _, line := range strings.Split(example, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		normalized := strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
		if strings.Contains(normalized, "helper=store") || strings.Contains(normalized, "credential.helper=store") {
			t.Fatalf("gitconfig.local.example contains uncommented plaintext credential store guidance: %q", line)
		}
	}
}

func TestRepositoryStarshipConfigClassifiesPortablePromptSafely(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	managedPath := filepath.Join(root, "configs/starship/starship.toml")
	managedBytes, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", managedPath, err)
	}
	managed := string(managedBytes)

	if strings.TrimSpace(managed) == "" {
		t.Fatalf("managed starship config is empty")
	}

	for _, want := range []string{
		`palette = "catppuccin_mocha"`,
		"[palettes.catppuccin_mocha]",
		"format = ",
		"[character]",
		"[cmd_duration]",
		"[time]",
	} {
		if !strings.Contains(managed, want) {
			t.Fatalf("managed starship config missing portable prompt segment %q:\n%s", want, managed)
		}
	}

	// Guard against committing machine-specific paths or private values. The
	// [username] module renders Starship's own $user variable at runtime, so a
	// literal username must never appear in the tracked file.
	normalizedManaged := strings.ToLower(managed)
	for _, forbidden := range []string{
		"/users/",
		"/home/",
		"argote",
		"yerson",
		"[custom",
		"env_var",
		"password",
		"secret",
		"token",
		"api_key",
		"localhost",
	} {
		if strings.Contains(normalizedManaged, forbidden) {
			t.Fatalf("managed starship config contains machine-specific/private concern %q:\n%s", forbidden, managed)
		}
	}

	docPath := filepath.Join(root, "configs/starship/README.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", docPath, err)
	}
	doc := string(docBytes)
	for _, want := range []string{
		"Portable",
		"Machine-specific",
		"Private",
		"Nerd Font",
		// Starship has no native include; the README must say so honestly
		// instead of documenting a broken local-override file.
		"no native include",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("starship config documentation missing %q:\n%s", want, doc)
		}
	}

	// Starship has no include mechanism, so the repository must NOT ship a
	// local-override file that Starship would silently ignore.
	for _, stray := range []string{
		"configs/starship/starship.local.toml",
		"configs/starship/starship.local.example",
		"configs/starship/starship.toml.local",
	} {
		if _, err := os.Stat(filepath.Join(root, stray)); err == nil {
			t.Fatalf("repository ships a broken Starship local-override file Starship cannot read: %s", stray)
		}
	}
}

func TestRepositoryTmuxConfigClassifiesPortableConfigSafely(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	managedPath := filepath.Join(root, "configs/tmux/tmux.conf")
	managedBytes, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", managedPath, err)
	}
	managed := string(managedBytes)

	if strings.TrimSpace(managed) == "" {
		t.Fatalf("managed tmux config is empty")
	}

	for _, want := range []string{
		"set -g @plugin 'tmux-plugins/tpm'",
		"set -g @plugin 'catppuccin/tmux#v2.3.0'",
		"set -g prefix C-a",
		"set -g mode-keys vi",
		"set -g status-position top",
		"display-popup",
		"source-file ~/.tmux.conf.local",
	} {
		if !strings.Contains(managed, want) {
			t.Fatalf("managed tmux config missing portable segment %q:\n%s", want, managed)
		}
	}

	// Guard against committing active machine-specific shell wiring or private
	// values. Comments may explain why those concerns are excluded, so only
	// non-comment configuration lines are checked for shell commands.
	for _, line := range strings.Split(managed, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, forbidden := range []string{
			"default-command",
			"default-shell",
		} {
			if strings.Contains(trimmed, forbidden) {
				t.Fatalf("managed tmux config contains active machine-specific shell setting %q:\n%s", line, managed)
			}
		}
	}

	activeTmuxConfig := make([]string, 0)
	for _, line := range strings.Split(managed, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		activeTmuxConfig = append(activeTmuxConfig, trimmed)
	}
	normalizedManaged := strings.ToLower(strings.Join(activeTmuxConfig, "\n"))
	for _, forbidden := range []string{
		"/users/",
		"/bin/zsh",
		"argote",
		"yerson",
		"password",
		"secret",
		"token",
		"api_key",
	} {
		if strings.Contains(normalizedManaged, forbidden) {
			t.Fatalf("managed tmux config contains machine-specific/private concern %q:\n%s", forbidden, managed)
		}
	}

	tmuxDir := filepath.Join(root, "configs/tmux")
	if err := filepath.WalkDir(tmuxDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == "plugins" || d.Name() == ".tmux" {
			t.Fatalf("repository ships generated tmux plugin state under %s", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir(%q) error = %v", tmuxDir, err)
	}

	docPath := filepath.Join(root, "configs/tmux/README.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", docPath, err)
	}
	doc := string(docBytes)
	for _, want := range []string{
		"Portable",
		"Machine-specific",
		"Generated",
		"Private",
		"Tmux Plugin Manager",
		"~/.tmux.conf.local",
		"Sandbox validation",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("tmux config documentation missing %q:\n%s", want, doc)
		}
	}
}

func TestRepositoryZellijConfigClassifiesPortableConfigSafely(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	entriesByTarget := map[string]manifest.Entry{}
	for _, entry := range got.Entries {
		entriesByTarget[entry.Target] = entry
	}

	tests := []struct {
		name   string
		target string
		source string
	}{
		{
			name:   "config",
			target: "~/.config/zellij/config.kdl",
			source: "configs/zellij/config.kdl",
		},
		{
			name:   "default layout",
			target: "~/.config/zellij/layouts/default.kdl",
			source: "configs/zellij/layouts/default.kdl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := entriesByTarget[tt.target]
			if !ok {
				t.Fatalf("repository manifest missing Zellij entry for target %q", tt.target)
			}
			if entry.Source != tt.source {
				t.Errorf("Source = %q, want %q", entry.Source, tt.source)
			}
			if entry.Strategy != "symlink" {
				t.Errorf("Strategy = %q, want symlink", entry.Strategy)
			}
			if !hasString(entry.Tags, "core") {
				t.Errorf("Tags = %#v, want core tag", entry.Tags)
			}
			if !sameStrings(entry.OS, []string{"darwin", "linux"}) {
				t.Errorf("OS = %#v, want [darwin linux]", entry.OS)
			}
			if !hasDependency(entry.Dependencies, "zellij") {
				t.Errorf("Dependencies = %#v, want zellij", entry.Dependencies)
			}
		})
	}

	configPath := filepath.Join(root, "configs/zellij/config.kdl")
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}
	config := string(configBytes)

	if strings.TrimSpace(config) == "" {
		t.Fatalf("managed Zellij config is empty")
	}

	for _, want := range []string{
		`keybinds clear-defaults=true`,
		`bind "Ctrl g" { SwitchToMode "normal"; }`,
		`bind "Ctrl g" { SwitchToMode "locked"; }`,
		`theme "catppuccin-mocha"`,
		`default_mode "locked"`,
		`mouse_mode true`,
		`scroll_buffer_size 10000`,
		`default_layout "default"`,
		`scrollback_editor "nvim"`,
		`plugins {`,
		`session-manager location="zellij:session-manager"`,
		`plugin-manager location="zellij:plugin-manager"`,
		`pane_frames {`,
		`rounded_corners true`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("managed Zellij config missing portable segment %q:\n%s", want, config)
		}
	}

	layoutPath := filepath.Join(root, "configs/zellij/layouts/default.kdl")
	layoutBytes, err := os.ReadFile(layoutPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", layoutPath, err)
	}
	layout := string(layoutBytes)
	for _, want := range []string{
		`layout {`,
		`default_tab_template {`,
		`plugin location="file:~/.config/zellij/plugins/zjstatus.wasm"`,
		`datetime_timezone "America/Bogota"`,
	} {
		if !strings.Contains(layout, want) {
			t.Fatalf("managed Zellij layout missing portable segment %q:\n%s", want, layout)
		}
	}

	for name, content := range map[string]string{
		"config": config,
		"layout": layout,
	} {
		normalized := strings.ToLower(content)
		for _, forbidden := range []string{
			"/users/",
			"/home/",
			"argote",
			"yerson",
			"password",
			"secret",
			"token",
			"api_key",
			"hostname",
		} {
			if strings.Contains(normalized, forbidden) {
				t.Fatalf("managed Zellij %s contains machine-specific/private concern %q:\n%s", name, forbidden, content)
			}
		}
	}

	zellijDir := filepath.Join(root, "configs/zellij")
	if err := filepath.WalkDir(zellijDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "plugins", "cache", "logs", "sessions":
				t.Fatalf("repository ships generated Zellij runtime state under %s", path)
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		for _, forbiddenSuffix := range []string{".wasm", ".bak", ".log"} {
			if strings.HasSuffix(name, forbiddenSuffix) {
				t.Fatalf("repository ships generated Zellij runtime file %s", path)
			}
		}
		for _, forbiddenName := range []string{"plugexit", "plugin-manager"} {
			if name == forbiddenName {
				t.Fatalf("repository ships generated Zellij runtime file %s", path)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir(%q) error = %v", zellijDir, err)
	}

	docPath := filepath.Join(root, "configs/zellij/README.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", docPath, err)
	}
	doc := string(docBytes)
	for _, want := range []string{
		"Portable",
		"Machine-specific",
		"Generated",
		"Private",
		"ZELLIJ_CONFIG_FILE",
		"ZELLIJ_CONFIG_DIR",
		"zellij --config",
		"zellij --config-dir",
		"zjstatus",
		"manual prerequisite",
		"America/Bogota",
		"Sandbox validation",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("Zellij config documentation missing %q:\n%s", want, doc)
		}
	}
}

func TestRepositoryManifestPlansMVPConfigurationSetSafely(t *testing.T) {
	root, err := filepath.Abs(filepath.Clean(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	p, err := plan.Build(*got, plan.Options{
		Profile:    "default",
		OS:         "darwin",
		SourceRoot: root,
		Home:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(p.Actions) != 12 {
		t.Fatalf("len(Actions) = %d, want 12", len(p.Actions))
	}
	for _, action := range p.Actions {
		if action.Status != plan.StatusCreate {
			t.Fatalf("Action for %s has Status = %q, want %q", action.Target, action.Status, plan.StatusCreate)
		}
	}
}

func TestRepositoryManifestSourcesExist(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	for i, entry := range got.Entries {
		sourcePath := filepath.Join(root, entry.Source)
		info, err := os.Stat(sourcePath)
		if err != nil {
			t.Fatalf("entries[%d].source %q does not exist at %s: %v", i, entry.Source, sourcePath, err)
		}
		// For symlink strategy a directory source is valid: dots creates a directory
		// symlink at the target pointing at the source directory. For copy and
		// template strategies the source must be a regular file because those
		// strategies operate on file content.
		if info.IsDir() && entry.Strategy != "symlink" {
			t.Fatalf("entries[%d].source %q points to a directory, want a file for strategy %q", i, entry.Source, entry.Strategy)
		}
	}
}

func TestRepositoryManifestNeovimEntry(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	entriesByTarget := map[string]manifest.Entry{}
	for _, entry := range got.Entries {
		entriesByTarget[entry.Target] = entry
	}

	entry, ok := entriesByTarget["~/.config/nvim"]
	if !ok {
		t.Fatal("repository manifest missing Neovim entry for target ~/.config/nvim")
	}
	if entry.Source != "configs/nvim" {
		t.Errorf("Source = %q, want %q", entry.Source, "configs/nvim")
	}
	if entry.Strategy != "symlink" {
		t.Errorf("Strategy = %q, want symlink", entry.Strategy)
	}
	if !hasString(entry.Tags, "core") {
		t.Errorf("Tags = %#v, want core tag", entry.Tags)
	}
	if !sameStrings(entry.OS, []string{"darwin", "linux"}) {
		t.Errorf("OS = %#v, want [darwin linux]", entry.OS)
	}
	if !hasDependency(entry.Dependencies, "neovim") {
		t.Errorf("Dependencies = %#v, want neovim dependency", entry.Dependencies)
	}

	// The source directory must exist in the repository.
	sourcePath := filepath.Join(root, entry.Source)
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("source %q does not exist: %v", sourcePath, err)
	}
	if !info.IsDir() {
		t.Fatalf("source %q is not a directory, want directory for symlink entry", sourcePath)
	}
}

func TestDirectorySourceSymlinkStrategyPassesValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim
    target: ~/.config/nvim
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v; directory source with symlink strategy should pass validation", err)
	}
	if got.Entries[0].Source != "configs/nvim" {
		t.Fatalf("Entry source = %q, want configs/nvim", got.Entries[0].Source)
	}
}

func TestDirectorySourceCopyStrategyIsAcceptedByManifestValidation(t *testing.T) {
	// The manifest itself accepts any source string; the copy strategy rejects
	// directory sources at plan/install time, not at manifest validation time.
	// This test documents the contract: manifest.LoadFile does not reject a copy
	// entry whose source happens to be a directory path — that is caught later.
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim
    target: ~/.config/nvim
    strategy: copy
    tags: [core]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v; manifest validation does not inspect source type", err)
	}
}
