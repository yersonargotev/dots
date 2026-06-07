package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
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
