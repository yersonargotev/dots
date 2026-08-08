package plan_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

func buildOneWithMetadata(t *testing.T, sourceRoot, home string, e manifest.Entry, meta state.Metadata, extraTags ...string) plan.Action {
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
		ExtraTags:  extraTags,
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

func TestBuildComposesCompatibleJSONSubsetEntriesForSharedTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "configs/base.json", `{"editor":{"theme":"dark"},"servers":["one"]}`)
	writeSource(t, sourceRoot, "configs/mobile.json", `{"editor":{"theme":"dark","fontSize":14},"servers":["two"]}`)

	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"base", "mobile"}}},
		Entries: []manifest.Entry{
			{Source: "configs/base.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"base"}},
			{Source: "configs/mobile.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"mobile"}},
		},
	}

	got, err := plan.Build(m, plan.Options{Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(got.Actions) != 1 {
		t.Fatalf("len(Actions) = %d, want one coherent target operation", len(got.Actions))
	}
	action := got.Actions[0]
	if !reflect.DeepEqual(action.Sources, []string{"configs/base.json", "configs/mobile.json"}) {
		t.Fatalf("Sources = %#v, want manifest-order contributors", action.Sources)
	}
	if action.Status != plan.StatusCreate {
		t.Fatalf("Status = %q, want %q", action.Status, plan.StatusCreate)
	}
	for _, want := range []string{`"theme": "dark"`, `"fontSize": 14`, `"one"`, `"two"`} {
		if !strings.Contains(string(action.Content), want) {
			t.Fatalf("composed content missing %s:\n%s", want, action.Content)
		}
	}
}

func TestBuildRejectsIncompatibleSharedJSONSubsetBeforeApply(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "configs/base.json", `{"editor":{"theme":"dark"}}`)
	writeSource(t, sourceRoot, "configs/mobile.json", `{"editor":{"theme":"light"}}`)

	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"base", "mobile"}}},
		Entries: []manifest.Entry{
			{Source: "configs/base.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"base"}},
			{Source: "configs/mobile.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"mobile"}},
		},
	}

	_, err := plan.Build(m, plan.Options{Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err == nil {
		t.Fatal("Build() error = nil, want incompatible shared-target error")
	}
	if !strings.Contains(err.Error(), "shared target") || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("Build() error = %q, want clear shared-target incompatibility", err)
	}
}

func TestBuildExpandsTrustedSingleContributorIntoCompatibleSharedTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "configs/base.json", `{"editor":{"theme":"dark"}}`)
	writeSource(t, sourceRoot, "configs/mobile.json", `{"mobile":true}`)
	target := filepath.Join(home, ".config", "shared.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"editor":{"theme":"dark"},"userOnly":"keep"}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"base", "mobile"}}},
		Entries: []manifest.Entry{
			{Source: "configs/base.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"base"}},
			{Source: "configs/mobile.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"mobile"}},
		},
	}
	meta := state.Metadata{Version: 2, Entries: []state.Record{{
		Target: target, Source: "configs/base.json", Strategy: "copy",
	}}}

	got, err := plan.Build(m, plan.Options{
		Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home, Metadata: meta,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(got.Actions) != 1 || got.Actions[0].Status != plan.StatusUpdate {
		t.Fatalf("Actions = %+v, want one trusted expansion update", got.Actions)
	}
}

func TestBuildRejectsUnsupportedDuplicateTargetDuringPlanning(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "configs/first", "first\n")
	writeSource(t, sourceRoot, "configs/second", "second\n")

	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{Source: "configs/first", Target: "~/.shared", Strategy: "copy", Tags: []string{"core"}},
			{Source: "configs/second", Target: "~/.shared", Strategy: "copy", Tags: []string{"core"}},
		},
	}

	_, err := plan.Build(m, plan.Options{Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err == nil {
		t.Fatal("Build() error = nil, want unsupported duplicate-target error")
	}
	if !strings.Contains(err.Error(), "duplicate target") {
		t.Fatalf("Build() error = %q, want duplicate-target planning error", err)
	}
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
			name:      "trusted compatible target missing dots-owned JSON values is update",
			ownership: "json-subset",
			sourceContent: `{
  "permissions": {
    "defaultMode": "bypassPermissions",
    "allow": [
      "Bash(git status:*)",
      "Bash(go test:*)"
    ]
  }
}`,
			targetContent: `{
  "permissions": {
    "allow": ["Bash(git status:*)"]
  },
  "hooks": {
    "PostToolUse": []
  }
}`,
			metadata: func(target string) state.Metadata {
				return state.Metadata{Entries: []state.Record{{Target: target, Source: "configs/claude/settings.json", Strategy: "copy"}}}
			},
			want: plan.StatusUpdate,
		},
		{
			name:      "changed dots-owned JSON scalar is conflict even with metadata",
			ownership: "json-subset",
			sourceContent: `{
  "permissions": {
    "defaultMode": "bypassPermissions"
  }
}`,
			targetContent: `{
  "permissions": {
    "defaultMode": "default"
  }
}`,
			metadata: func(target string) state.Metadata {
				return state.Metadata{Entries: []state.Record{{Target: target, Source: "configs/claude/settings.json", Strategy: "copy"}}}
			},
			want: plan.StatusConflict,
		},
		{
			name:      "trusted target missing dots-owned JSON array element is update",
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
			want: plan.StatusUpdate,
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

func TestBuildJSONSubsetUsesRecordedContributionForReversibleRemoval(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "configs/shared.json", `{"owned":{"keep":true,"added":"new"}}`)
	target := filepath.Join(home, ".config", "shared.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"owned":{"keep":true,"retired":"old"},"external":"preserve"}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	previous := []byte(`{"owned":{"keep":true,"retired":"old"}}`)
	meta := state.Metadata{Entries: []state.Record{{
		Target: target, Source: "configs/shared.json", Strategy: "copy", Ownership: "json-subset", OwnedContent: previous,
	}}}

	action := buildOneWithMetadata(t, sourceRoot, home, manifest.Entry{
		Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"core"},
	}, meta)
	if action.Status != plan.StatusUpdate {
		t.Fatalf("Status = %q, want update", action.Status)
	}
	if !reflect.DeepEqual(action.PreviousContent, previous) {
		t.Fatalf("PreviousContent = %s, want %s", action.PreviousContent, previous)
	}

	if err := os.WriteFile(target, []byte(`{"owned":{"keep":true,"retired":"locally-changed"},"external":"preserve"}`), 0o600); err != nil {
		t.Fatalf("rewrite target: %v", err)
	}
	action = buildOneWithMetadata(t, sourceRoot, home, manifest.Entry{
		Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"core"},
	}, meta)
	if action.Status != plan.StatusConflict {
		t.Fatalf("Status = %q, want conflict for changed retired value", action.Status)
	}

	if err := os.WriteFile(target, []byte(`{"owned":{"keep":true},"external":"preserve"}`), 0o600); err != nil {
		t.Fatalf("rewrite target without retired key: %v", err)
	}
	action = buildOneWithMetadata(t, sourceRoot, home, manifest.Entry{
		Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"core"},
	}, meta)
	if action.Status != plan.StatusConflict {
		t.Fatalf("Status = %q, want conflict for missing retired key", action.Status)
	}
}

func TestBuildJSONSubsetLegacyMetadataDoesNotAuthorizeRemoval(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "configs/shared.json", `{"owned":{"keep":true}}`)
	target := filepath.Join(home, ".config", "shared.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"owned":{"keep":true,"retired":"old"},"external":"preserve"}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	meta := state.Metadata{Version: 3, Entries: []state.Record{{Target: target, Source: "configs/shared.json", Strategy: "copy"}}}

	action := buildOneWithMetadata(t, sourceRoot, home, manifest.Entry{
		Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"core"},
	}, meta)
	if action.Status != plan.StatusUnchanged || len(action.PreviousContent) != 0 {
		t.Fatalf("action = %+v, want unchanged without removal evidence", action)
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

func TestBuildReportsUpdateForTOMLSubsetSourceOverrideFromCompatibleManagedTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "configs/codex/config.toml", "sandbox_mode = \"danger-full-access\"\napproval_policy = \"never\"\n")
	writeSource(t, sourceRoot, "configs/codex/config-codegraph.toml", `sandbox_mode = "danger-full-access"
approval_policy = "never"

[[hooks.SessionStart]]
matcher = "startup|resume"

[[hooks.SessionStart.hooks]]
type = "command"
command = "codegraph init"
timeout = 120
`)
	target := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte("model = \"gpt-5.5\"\nsandbox_mode = \"danger-full-access\"\napproval_policy = \"never\"\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	action := buildOneWithMetadata(t, sourceRoot, home, manifest.Entry{
		Source: "configs/codex/config.toml",
		SourceOverrides: map[string]string{
			"codegraph": "configs/codex/config-codegraph.toml",
		},
		Target:    "~/.codex/config.toml",
		Strategy:  "copy",
		Ownership: "toml-subset",
		Tags:      []string{"core"},
	}, state.Metadata{Entries: []state.Record{{
		Target: target, Source: "configs/codex/config.toml", Strategy: "copy",
	}}}, "codegraph")

	if action.Status != plan.StatusUpdate {
		t.Fatalf("Status = %q, want %q", action.Status, plan.StatusUpdate)
	}
	if action.Source != "configs/codex/config-codegraph.toml" {
		t.Fatalf("Source = %q, want CodeGraph override", action.Source)
	}
	if action.Ownership != "toml-subset" {
		t.Fatalf("Ownership = %q, want toml-subset", action.Ownership)
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

func TestBuildUsesSourceOverrideForSelectedExtraTag(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "default.conf", "dark\n")
	writeSource(t, sourceRoot, "adaptive.conf", "light\n")

	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {Tags: []string{"core"}},
		},
		Entries: []manifest.Entry{{
			Source:          "default.conf",
			SourceOverrides: map[string]string{"adaptive-theme": "adaptive.conf"},
			Target:          "~/.config/app/config",
			Strategy:        "symlink",
			Tags:            []string{"core"},
		}},
	}

	withoutTag, err := plan.Build(m, plan.Options{Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() without tag error = %v", err)
	}
	if got := withoutTag.Actions[0].Source; got != "default.conf" {
		t.Fatalf("source without extra tag = %q, want default.conf", got)
	}

	withTag, err := plan.Build(m, plan.Options{Profile: "default", ExtraTags: []string{"adaptive-theme"}, OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() with tag error = %v", err)
	}
	if got := withTag.Actions[0].Source; got != "adaptive.conf" {
		t.Fatalf("source with adaptive-theme = %q, want adaptive.conf", got)
	}
	if len(withTag.Actions) != 1 {
		t.Fatalf("len(Actions) with source override = %d, want 1", len(withTag.Actions))
	}
}

func TestBuildDiagnosesUnselectedSourceOverrides(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "herdr/default.toml", "dark\n")
	herdrAdaptive := writeSource(t, sourceRoot, "herdr/adaptive.toml", "adaptive\n")
	writeSource(t, sourceRoot, "zellij/default.kdl", "dark\n")
	zellijAdaptive := writeSource(t, sourceRoot, "zellij/adaptive.kdl", "adaptive\n")

	entries := []manifest.Entry{
		{
			Source:          "herdr/default.toml",
			SourceOverrides: map[string]string{"adaptive-theme": "herdr/adaptive.toml"},
			Target:          "~/.config/herdr/config.toml",
			Strategy:        "symlink",
			Tags:            []string{"core"},
			OS:              []string{"darwin"},
		},
		{
			Source:          "zellij/default.kdl",
			SourceOverrides: map[string]string{"adaptive-theme": "zellij/adaptive.kdl"},
			Target:          "~/.config/zellij/config.kdl",
			Strategy:        "symlink",
			Tags:            []string{"core"},
			OS:              []string{"darwin"},
		},
	}
	for target, source := range map[string]string{
		filepath.Join(home, ".config", "herdr", "config.toml"): herdrAdaptive,
		filepath.Join(home, ".config", "zellij", "config.kdl"): zellijAdaptive,
	} {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir target parent: %v", err)
		}
		if err := os.Symlink(source, target); err != nil {
			t.Fatalf("symlink target: %v", err)
		}
	}
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries:  entries,
	}

	withoutTag, err := plan.Build(m, plan.Options{Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() without tag error = %v", err)
	}
	for _, action := range withoutTag.Actions {
		if action.Status != plan.StatusConflict {
			t.Fatalf("Status = %q, want conflict", action.Status)
		}
		if action.Reason != plan.ConflictReasonSourceOverrideNotSelected {
			t.Fatalf("Reason = %q, want %q", action.Reason, plan.ConflictReasonSourceOverrideNotSelected)
		}
		if got, want := action.MatchingTags, []string{"adaptive-theme"}; len(got) != 1 || got[0] != want[0] {
			t.Fatalf("MatchingTags = %v, want %v", got, want)
		}
	}

	withTag, err := plan.Build(m, plan.Options{Profile: "default", ExtraTags: []string{"adaptive-theme"}, OS: "darwin", SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() with tag error = %v", err)
	}
	for _, action := range withTag.Actions {
		if action.Status != plan.StatusUnchanged || action.Reason != "" || len(action.MatchingTags) != 0 {
			t.Fatalf("selected override action = %+v, want unchanged without diagnostic", action)
		}
	}
}

func TestBuildSourceOverrideDiagnosisIsDeterministicAndExact(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	writeSource(t, sourceRoot, "default.conf", "default\n")
	writeSource(t, sourceRoot, "adaptive.conf", "adaptive\n")
	target := filepath.Join(home, "config")
	if err := os.WriteFile(target, []byte("adaptive\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	e := manifest.Entry{
		Source: "default.conf",
		SourceOverrides: map[string]string{
			"z-last":  "adaptive.conf",
			"a-first": "adaptive.conf",
		},
		Target:   "~/config",
		Strategy: "copy",
		Tags:     []string{"core"},
	}

	action := buildOne(t, sourceRoot, home, e)
	if got, want := action.MatchingTags, []string{"a-first", "z-last"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("MatchingTags = %v, want %v", got, want)
	}

	if err := os.WriteFile(target, []byte("local divergence\n"), 0o600); err != nil {
		t.Fatalf("rewrite target: %v", err)
	}
	action = buildOne(t, sourceRoot, home, e)
	if action.Status != plan.StatusConflict || action.Reason != "" || len(action.MatchingTags) != 0 {
		t.Fatalf("ordinary conflict = %+v, want no source-override diagnosis", action)
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

func TestBuildReadsSnapshotContentWhileKeepingCanonicalSymlinkPaths(t *testing.T) {
	canonicalRoot := t.TempDir()
	readRoot := t.TempDir()
	home := t.TempDir()
	for _, root := range []string{canonicalRoot, readRoot} {
		if err := os.MkdirAll(filepath.Join(root, "configs"), 0o755); err != nil {
			t.Fatalf("mkdir configs: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "configs", "tool"), []byte("managed\n"), 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}
	canonicalSource := filepath.Join(canonicalRoot, "configs", "tool")
	if err := os.Symlink(canonicalSource, filepath.Join(home, ".tool")); err != nil {
		t.Fatalf("write target symlink: %v", err)
	}
	m := manifest.Manifest{
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{{Source: "configs/tool", Target: "~/.tool", Strategy: "symlink", Tags: []string{"core"}}},
	}

	got, err := plan.Build(m, plan.Options{Profile: "default", OS: "darwin", SourceRoot: canonicalRoot, SourceReadRoot: readRoot, Home: home})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(got.Actions) != 1 || got.Actions[0].Status != plan.StatusUnchanged {
		t.Fatalf("Actions = %#v, want unchanged canonical symlink", got.Actions)
	}
	if got.Actions[0].ResolvedSource != canonicalSource {
		t.Fatalf("ResolvedSource = %q, want canonical %q", got.Actions[0].ResolvedSource, canonicalSource)
	}
}
