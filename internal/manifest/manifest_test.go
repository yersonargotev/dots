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
		if strings.Contains(entry.Source, "nvim") || strings.Contains(entry.Target, "nvim") {
			t.Fatalf("repository manifest includes deferred Neovim configuration: %#v", entry)
		}
		for _, dep := range entry.Dependencies {
			if dep.Name == "nvim" || dep.Name == "neovim" {
				t.Fatalf("repository manifest includes deferred Neovim dependency: %#v", dep)
			}
		}
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
				"Local Extension Point",
				"${HOME}/.zshrc.local",
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

	if len(p.Actions) != 4 {
		t.Fatalf("len(Actions) = %d, want 4", len(p.Actions))
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
		if info.IsDir() {
			t.Fatalf("entries[%d].source %q points to a directory, want a file", i, entry.Source)
		}
	}
}
