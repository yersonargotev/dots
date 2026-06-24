package plan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

// writeSource creates a managed source file under sourceRoot and returns its
// absolute path.
func writeSource(t *testing.T, sourceRoot, rel, content string) string {
	t.Helper()
	abs := filepath.Join(sourceRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return abs
}

func buildOne(t *testing.T, sourceRoot, home string, e manifest.Entry) plan.Action {
	t.Helper()
	return buildOneWithMetadata(t, sourceRoot, home, e, state.Metadata{})
}

func buildOneWithMetadata(t *testing.T, sourceRoot, home string, e manifest.Entry, meta state.Metadata) plan.Action {
	t.Helper()
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{e},
	}
	got, err := plan.Build(m, plan.Options{
		Profile:    "default",
		OS:         "darwin",
		SourceRoot: sourceRoot,
		Home:       home,
		Metadata:   meta,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(got.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(got.Actions))
	}
	return got.Actions[0]
}

func entry(source, strategy string, tags, osFilter []string) manifest.Entry {
	return manifest.Entry{
		Source:   source,
		Target:   "~/" + source,
		Strategy: strategy,
		Tags:     tags,
		OS:       osFilter,
	}
}

func sources(p plan.Plan) []string {
	out := make([]string, 0, len(p.Actions))
	for _, a := range p.Actions {
		out = append(out, a.Source)
	}
	return out
}

func TestBuildSymlinkPointingToSourceIsUnchanged(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourceAbs := writeSource(t, sourceRoot, "zshrc", "export A=1\n")
	target := filepath.Join(home, "zshrc")
	if err := os.Symlink(sourceAbs, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	action := buildOne(t, sourceRoot, home, entry("zshrc", "symlink", []string{"core"}, nil))

	if action.Status != plan.StatusUnchanged {
		t.Fatalf("Status = %q, want %q", action.Status, plan.StatusUnchanged)
	}
}

func TestBuildSymlinkConflicts(t *testing.T) {
	t.Run("regular file at target", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		writeSource(t, sourceRoot, "zshrc", "managed\n")
		if err := os.WriteFile(filepath.Join(home, "zshrc"), []byte("local\n"), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}

		action := buildOne(t, sourceRoot, home, entry("zshrc", "symlink", []string{"core"}, nil))

		if action.Status != plan.StatusConflict {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusConflict)
		}
	})

	t.Run("symlink pointing elsewhere", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		writeSource(t, sourceRoot, "zshrc", "managed\n")
		other := writeSource(t, sourceRoot, "other", "other\n")
		if err := os.Symlink(other, filepath.Join(home, "zshrc")); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		action := buildOne(t, sourceRoot, home, entry("zshrc", "symlink", []string{"core"}, nil))

		if action.Status != plan.StatusConflict {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusConflict)
		}
	})
}

func TestBuildCopyComparesContent(t *testing.T) {
	t.Run("identical content is unchanged", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		writeSource(t, sourceRoot, "gitconfig", "[user]\n  name = me\n")
		if err := os.WriteFile(filepath.Join(home, "gitconfig"), []byte("[user]\n  name = me\n"), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}

		action := buildOne(t, sourceRoot, home, entry("gitconfig", "copy", []string{"core"}, nil))

		if action.Status != plan.StatusUnchanged {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusUnchanged)
		}
	})

	t.Run("different content is conflict", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		writeSource(t, sourceRoot, "gitconfig", "[user]\n  name = me\n")
		if err := os.WriteFile(filepath.Join(home, "gitconfig"), []byte("[user]\n  name = other\n"), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}

		action := buildOne(t, sourceRoot, home, entry("gitconfig", "copy", []string{"core"}, nil))

		if action.Status != plan.StatusConflict {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusConflict)
		}
	})

	t.Run("missing target is create", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		writeSource(t, sourceRoot, "gitconfig", "[user]\n  name = me\n")

		action := buildOne(t, sourceRoot, home, entry("gitconfig", "copy", []string{"core"}, nil))

		if action.Status != plan.StatusCreate {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusCreate)
		}
	})
}

func TestBuildCopyJSONSubsetOwnership(t *testing.T) {
	tests := []struct {
		name          string
		ownership     string
		sourceContent string
		targetContent string
		metadata      func(target string) state.Metadata
		want          plan.Status
	}{
		{
			name:      "untrusted pre-existing compatible JSON superset is conflict",
			ownership: "json-subset",
			sourceContent: `{
  "permissions": {
    "allow": ["Bash(git status:*)"]
  }
}`,
			targetContent: `{
  "permissions": {
    "allow": [
      "Bash(git status:*)",
      "Bash(go test:*)"
    ]
  },
  "hooks": {
    "PostToolUse": []
  }
}`,
			want: plan.StatusConflict,
		},
		{
			name:      "trusted source values plus provisioner additions are unchanged",
			ownership: "json-subset",
			sourceContent: `{
  "permissions": {
    "allow": ["Bash(git status:*)"]
  }
}`,
			targetContent: `{
  "permissions": {
    "allow": [
      "Bash(git status:*)",
      "Bash(go test:*)"
    ]
  },
  "hooks": {
    "PostToolUse": []
  }
}`,
			metadata: func(target string) state.Metadata {
				return state.Metadata{Entries: []state.Record{{Target: target, Source: "configs/claude/settings.json", Strategy: "copy"}}}
			},
			want: plan.StatusUnchanged,
		},
		{
			name: "regular copy still requires exact content",
			sourceContent: `{
  "permissions": {
    "allow": ["Bash(git status:*)"]
  }
}`,
			targetContent: `{
  "permissions": {
    "allow": [
      "Bash(git status:*)",
      "Bash(go test:*)"
    ]
  }
}`,
			want: plan.StatusConflict,
		},
		{
			name:      "missing dots-owned JSON value is conflict even with metadata",
			ownership: "json-subset",
			sourceContent: `{
  "permissions": {
    "allow": [
      "Bash(git status:*)",
      "Bash(go test:*)"
    ]
  }
}`,
			targetContent: `{
  "permissions": {
    "allow": ["Bash(git status:*)"]
  }
}`,
			metadata: func(target string) state.Metadata {
				return state.Metadata{Entries: []state.Record{{Target: target, Source: "configs/claude/settings.json", Strategy: "copy"}}}
			},
			want: plan.StatusConflict,
		},
		{
			name:      "changed dots-owned JSON value is conflict even with metadata",
			ownership: "json-subset",
			sourceContent: `{
  "permissions": {
    "allow": ["Bash(git status:*)"]
  }
}`,
			targetContent: `{
  "permissions": {
    "allow": ["Bash(git diff:*)"]
  }
}`,
			metadata: func(target string) state.Metadata {
				return state.Metadata{Entries: []state.Record{{Target: target, Source: "configs/claude/settings.json", Strategy: "copy"}}}
			},
			want: plan.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := t.TempDir()
			home := t.TempDir()
			writeSource(t, sourceRoot, "configs/claude/settings.json", tt.sourceContent)
			target := filepath.Join(home, ".claude", "settings.json")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("mkdir target: %v", err)
			}
			if err := os.WriteFile(target, []byte(tt.targetContent), 0o600); err != nil {
				t.Fatalf("write target: %v", err)
			}

			meta := state.Metadata{}
			if tt.metadata != nil {
				meta = tt.metadata(target)
			}

			action := buildOneWithMetadata(t, sourceRoot, home, manifest.Entry{
				Source:    "configs/claude/settings.json",
				Target:    "~/.claude/settings.json",
				Strategy:  "copy",
				Ownership: tt.ownership,
				Tags:      []string{"core"},
			}, meta)

			if action.Status != tt.want {
				t.Fatalf("Status = %q, want %q", action.Status, tt.want)
			}
		})
	}
}

func TestBuildCopyTOMLSubsetOwnership(t *testing.T) {
	tests := []struct {
		name          string
		sourceContent string
		targetContent string
		metadata      func(target string) state.Metadata
		want          plan.Status
	}{
		{
			name:          "untrusted pre-existing compatible TOML superset is conflict",
			sourceContent: "[tui]\nstatus_line = [\"model-with-reasoning\", \"context-remaining\", \"git-branch\"]\n",
			targetContent: "model = \"gpt-5.5\"\n\n[tui]\nstatus_line = [\"model-with-reasoning\", \"context-remaining\", \"git-branch\"]\ntheme = \"catppuccin\"\n",
			want:          plan.StatusConflict,
		},
		{
			name:          "trusted Codex TOML with runtime additions is unchanged",
			sourceContent: "[tui]\nstatus_line = [\"model-with-reasoning\", \"context-remaining\", \"git-branch\"]\n",
			targetContent: "model = \"gpt-5.5\"\n\n[tui]\nstatus_line = [\"model-with-reasoning\", \"context-remaining\", \"git-branch\"]\ntheme = \"catppuccin\"\n",
			metadata: func(target string) state.Metadata {
				return state.Metadata{Entries: []state.Record{{Target: target, Source: "configs/codex/config.toml", Strategy: "copy"}}}
			},
			want: plan.StatusUnchanged,
		},
		{
			name:          "changed dots-owned TOML value is conflict even with metadata",
			sourceContent: "[tui]\nstatus_line = [\"model-with-reasoning\", \"context-remaining\", \"git-branch\"]\n",
			targetContent: "[tui]\nstatus_line = [\"model-with-reasoning\", \"current-dir\"]\n",
			metadata: func(target string) state.Metadata {
				return state.Metadata{Entries: []state.Record{{Target: target, Source: "configs/codex/config.toml", Strategy: "copy"}}}
			},
			want: plan.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := t.TempDir()
			home := t.TempDir()
			writeSource(t, sourceRoot, "configs/codex/config.toml", tt.sourceContent)
			target := filepath.Join(home, ".codex", "config.toml")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("mkdir target: %v", err)
			}
			if err := os.WriteFile(target, []byte(tt.targetContent), 0o600); err != nil {
				t.Fatalf("write target: %v", err)
			}

			meta := state.Metadata{}
			if tt.metadata != nil {
				meta = tt.metadata(target)
			}

			action := buildOneWithMetadata(t, sourceRoot, home, manifest.Entry{
				Source:    "configs/codex/config.toml",
				Target:    "~/.codex/config.toml",
				Strategy:  "copy",
				Ownership: "toml-subset",
				Tags:      []string{"core"},
			}, meta)

			if action.Status != tt.want {
				t.Fatalf("Status = %q, want %q", action.Status, tt.want)
			}
		})
	}
}

func TestBuildDanglingSymlinkIsConflict(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "zshrc", "managed\n")
	// A symlink that resolves to a path that does not exist, and is not the
	// managed source: the target is a symlink (Lstat sees ModeSymlink), so it
	// must be reported as a conflict rather than created over.
	if err := os.Symlink(filepath.Join(home, "does-not-exist"), filepath.Join(home, "zshrc")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	action := buildOne(t, sourceRoot, home, entry("zshrc", "symlink", []string{"core"}, nil))

	if action.Status != plan.StatusConflict {
		t.Fatalf("Status = %q, want %q", action.Status, plan.StatusConflict)
	}
}

func TestBuildResolvesTarget(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   func(home string) string
	}{
		{
			name:   "tilde slash expands under home",
			target: "~/.config/starship.toml",
			want:   func(home string) string { return filepath.Join(home, ".config/starship.toml") },
		},
		{
			name:   "bare tilde is home",
			target: "~",
			want:   func(home string) string { return home },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := t.TempDir()
			home := t.TempDir()
			writeSource(t, sourceRoot, "zshrc", "x\n")

			action := buildOne(t, sourceRoot, home, manifest.Entry{
				Source:   "zshrc",
				Target:   tt.target,
				Strategy: "symlink",
				Tags:     []string{"core"},
			})

			if want := tt.want(home); action.Target != want {
				t.Fatalf("Target = %q, want %q", action.Target, want)
			}
		})
	}
}

func TestBuildRejectsAbsoluteTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "zshrc", "x\n")

	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source:   "zshrc",
			Target:   filepath.Join(t.TempDir(), "outside"),
			Strategy: "symlink",
			Tags:     []string{"core"},
		}},
	}

	_, err := plan.Build(m, plan.Options{
		Profile:    "default",
		OS:         "darwin",
		SourceRoot: sourceRoot,
		Home:       home,
	})
	if err == nil {
		t.Fatal("Build() error = nil, want unsafe target error")
	}
}

func TestBuildRejectsTargetTraversal(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "zshrc", "x\n")

	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source:   "zshrc",
			Target:   "~/../outside",
			Strategy: "symlink",
			Tags:     []string{"core"},
		}},
	}

	_, err := plan.Build(m, plan.Options{
		Profile:    "default",
		OS:         "darwin",
		SourceRoot: sourceRoot,
		Home:       home,
	})
	if err == nil {
		t.Fatal("Build() error = nil, want unsafe target error")
	}
}

func TestBuildRejectsUnsafeSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name:   "absolute source",
			source: filepath.Join(t.TempDir(), "outside"),
		},
		{
			name:   "source traversal",
			source: "../outside",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRoot := t.TempDir()
			home := t.TempDir()

			m := manifest.Manifest{
				Version:  1,
				Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
				Entries: []manifest.Entry{{
					Source:   tt.source,
					Target:   "~/.zshrc",
					Strategy: "symlink",
					Tags:     []string{"core"},
				}},
			}

			_, err := plan.Build(m, plan.Options{
				Profile:    "default",
				OS:         "darwin",
				SourceRoot: sourceRoot,
				Home:       home,
			})
			if err == nil {
				t.Fatal("Build() error = nil, want unsafe source error")
			}
		})
	}
}

func TestBuildRejectsSourceSymlinkEscape(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideRoot := t.TempDir()

	outsideSecret := filepath.Join(outsideRoot, "secret")
	if err := os.WriteFile(outsideSecret, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("write outside secret: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.Symlink(outsideSecret, filepath.Join(sourceRoot, "configs", "link")); err != nil {
		t.Fatalf("symlink source: %v", err)
	}

	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{
			Source:   "configs/link",
			Target:   "~/.secret",
			Strategy: "copy",
			Tags:     []string{"core"},
		}},
	}

	_, err := plan.Build(m, plan.Options{
		Profile:    "default",
		OS:         "darwin",
		SourceRoot: sourceRoot,
		Home:       home,
	})
	if err == nil {
		t.Fatal("Build() error = nil, want source symlink escape error")
	}
}

// Template rendering is not implemented yet, so an existing target cannot be
// verified as a match. The plan is conservative: missing target is create, any
// existing target is a conflict until render-aware comparison lands.
func TestBuildTemplateIsConservative(t *testing.T) {
	t.Run("missing target is create", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		writeSource(t, sourceRoot, "starship.toml", "format = '$all'\n")

		action := buildOne(t, sourceRoot, home, entry("starship.toml", "template", []string{"core"}, nil))

		if action.Status != plan.StatusCreate {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusCreate)
		}
	})

	t.Run("existing target is conflict", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		writeSource(t, sourceRoot, "starship.toml", "format = '$all'\n")
		if err := os.WriteFile(filepath.Join(home, "starship.toml"), []byte("format = '$all'\n"), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}

		action := buildOne(t, sourceRoot, home, entry("starship.toml", "template", []string{"core"}, nil))

		if action.Status != plan.StatusConflict {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusConflict)
		}
	})
}

func TestBuildMissingSource(t *testing.T) {
	t.Run("copy with absent source", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		// No source file is written under sourceRoot.
		if err := os.WriteFile(filepath.Join(home, "gitconfig"), []byte("local\n"), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}

		action := buildOne(t, sourceRoot, home, entry("gitconfig", "copy", []string{"core"}, nil))

		if action.Status != plan.StatusMissingSource {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusMissingSource)
		}
	})

	t.Run("symlink to deleted source is not reported unchanged", func(t *testing.T) {
		sourceRoot := t.TempDir()
		home := t.TempDir()
		// The symlink points at the declared source path, but that source
		// never exists under sourceRoot.
		sourceAbs := filepath.Join(sourceRoot, "zshrc")
		if err := os.Symlink(sourceAbs, filepath.Join(home, "zshrc")); err != nil {
			t.Fatalf("symlink: %v", err)
		}

		action := buildOne(t, sourceRoot, home, entry("zshrc", "symlink", []string{"core"}, nil))

		if action.Status != plan.StatusMissingSource {
			t.Fatalf("Status = %q, want %q", action.Status, plan.StatusMissingSource)
		}
	})
}

func TestBuildSelection(t *testing.T) {
	tests := []struct {
		name    string
		profile manifest.Profile
		entries []manifest.Entry
		os      string
		want    []string
	}{
		{
			name:    "shared tag is selected",
			profile: manifest.Profile{Tags: []string{"core"}},
			entries: []manifest.Entry{entry("a", "symlink", []string{"core", "dev"}, nil)},
			os:      "darwin",
			want:    []string{"a"},
		},
		{
			name:    "no shared tag is excluded",
			profile: manifest.Profile{Tags: []string{"core"}},
			entries: []manifest.Entry{entry("a", "symlink", []string{"dev"}, nil)},
			os:      "darwin",
			want:    []string{},
		},
		{
			name:    "empty os filter matches any os",
			profile: manifest.Profile{Tags: []string{"core"}},
			entries: []manifest.Entry{entry("a", "symlink", []string{"core"}, nil)},
			os:      "linux",
			want:    []string{"a"},
		},
		{
			name:    "os filter excludes other os",
			profile: manifest.Profile{Tags: []string{"core"}},
			entries: []manifest.Entry{entry("a", "symlink", []string{"core"}, []string{"linux"})},
			os:      "darwin",
			want:    []string{},
		},
		{
			name:    "declaration order is preserved",
			profile: manifest.Profile{Tags: []string{"core"}},
			entries: []manifest.Entry{
				entry("first", "symlink", []string{"core"}, nil),
				entry("skip", "symlink", []string{"dev"}, nil),
				entry("second", "symlink", []string{"core"}, nil),
			},
			os:   "darwin",
			want: []string{"first", "second"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := manifest.Manifest{
				Version:  1,
				Profiles: map[string]manifest.Profile{"default": tt.profile},
				Entries:  tt.entries,
			}

			got, err := plan.Build(m, plan.Options{
				Profile:    "default",
				OS:         tt.os,
				SourceRoot: t.TempDir(),
				Home:       t.TempDir(),
			})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			gotSources := sources(got)
			if len(gotSources) != len(tt.want) {
				t.Fatalf("selected sources = %v, want %v", gotSources, tt.want)
			}
			for i := range tt.want {
				if gotSources[i] != tt.want[i] {
					t.Fatalf("selected sources = %v, want %v", gotSources, tt.want)
				}
			}
		})
	}
}

func TestBuildRejectsUnknownProfile(t *testing.T) {
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{entry("a", "symlink", []string{"core"}, nil)},
	}

	_, err := plan.Build(m, plan.Options{
		Profile:    "work",
		OS:         "darwin",
		SourceRoot: t.TempDir(),
		Home:       t.TempDir(),
	})
	if err == nil {
		t.Fatal("Build() error = nil, want error for unknown profile")
	}
	if err.Error() != `profile "work" not found` {
		t.Fatalf("Build() error = %q, want %q", err.Error(), `profile "work" not found`)
	}
}

func TestBuildSelectsMatchingEntryAsCreate(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	writeSource(t, sourceRoot, "configs/zsh/zshrc", "export A=1\n")

	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {Tags: []string{"core"}},
		},
		Entries: []manifest.Entry{
			{
				Source:   "configs/zsh/zshrc",
				Target:   "~/.zshrc",
				Strategy: "symlink",
				Tags:     []string{"core"},
				OS:       []string{"darwin", "linux"},
			},
		},
	}

	got, err := plan.Build(m, plan.Options{
		Profile:    "default",
		OS:         "darwin",
		SourceRoot: sourceRoot,
		Home:       home,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if got.Profile != "default" {
		t.Fatalf("Plan.Profile = %q, want %q", got.Profile, "default")
	}
	if len(got.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want 1", len(got.Actions))
	}

	action := got.Actions[0]
	wantTarget := filepath.Join(home, ".zshrc")
	if action.Source != "configs/zsh/zshrc" {
		t.Errorf("Action.Source = %q, want %q", action.Source, "configs/zsh/zshrc")
	}
	if action.Target != wantTarget {
		t.Errorf("Action.Target = %q, want %q", action.Target, wantTarget)
	}
	if action.Strategy != "symlink" {
		t.Errorf("Action.Strategy = %q, want %q", action.Strategy, "symlink")
	}
	if action.Status != plan.StatusCreate {
		t.Errorf("Action.Status = %q, want %q", action.Status, plan.StatusCreate)
	}
}
