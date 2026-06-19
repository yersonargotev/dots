package status_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
)

// fixture wires an isolated source root and home plus a single-entry manifest so
// each behavior is observed end-to-end against real filesystem state.
type fixture struct {
	sourceRoot string
	home       string
	manifest   manifest.Manifest
}

func newFixture(entry manifest.Entry) fixture {
	return fixture{
		manifest: manifest.Manifest{
			Version:  1,
			Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
			Entries:  []manifest.Entry{entry},
		},
	}
}

func (f fixture) build(t *testing.T, meta state.Metadata) status.Report {
	t.Helper()
	report, err := status.Build(f.manifest, meta, status.Options{
		Profile:    "default",
		OS:         "linux",
		SourceRoot: f.sourceRoot,
		Home:       f.home,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return report
}

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

func onlyEntry(t *testing.T, report status.Report) status.Entry {
	t.Helper()
	if len(report.Entries) != 1 {
		t.Fatalf("report entries = %d, want 1\nreport: %+v", len(report.Entries), report)
	}
	return report.Entries[0]
}

func TestBuildReportsOKForSymlinkPointingAtSource(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	src := writeSource(t, f.sourceRoot, "configs/zsh/zshrc", "export A=1\n")
	if err := os.Symlink(src, filepath.Join(f.home, ".zshrc")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateOK {
		t.Fatalf("state = %q, want ok", got)
	}
}

func TestBuildReportsOKForCopyMatchingSource(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/git/gitconfig", Target: "~/.gitconfig", Strategy: "copy", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/git/gitconfig", "[user]\n")
	if err := os.WriteFile(filepath.Join(f.home, ".gitconfig"), []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateOK {
		t.Fatalf("state = %q, want ok", got)
	}
}

func TestBuildReportsOKForClaudeSettingsWithProvisionerAdditionsWhenInstalled(t *testing.T) {
	f := newFixture(manifest.Entry{
		Source:    "configs/claude/settings.json",
		Target:    "~/.claude/settings.json",
		Strategy:  "copy",
		Ownership: "json-subset",
		Tags:      []string{"core"},
	})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/claude/settings.json", `{
  "env": {
    "ENABLE_TOOL_SEARCH": "true"
  },
  "permissions": {
    "allow": ["mcp__codegraph__codegraph_search"]
  },
  "model": "opus"
}
`)
	target := filepath.Join(f.home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{
  "env": {
    "ENABLE_TOOL_SEARCH": "true",
    "CLAUDE_CODE_ENABLE_TELEMETRY": "0"
  },
  "permissions": {
    "allow": [
      "mcp__codegraph__codegraph_search",
      "mcp__chrome-devtools__new_page"
    ],
    "deny": ["Bash(rm -rf *)"]
  },
  "model": "opus",
  "enabledPlugins": {
    "chrome-devtools-mcp": true
  }
}
`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	meta := state.Metadata{Version: 1, Entries: []state.Record{{
		Target: target, Source: "configs/claude/settings.json", Strategy: "copy", Hash: "installed-hash",
	}}}

	if got := onlyEntry(t, f.build(t, meta)).State; got != status.StateOK {
		t.Fatalf("state = %q, want ok because provisioner-added JSON keys/items are outside dots ownership", got)
	}
}

func TestBuildReportsDriftedForChangedDotsOwnedClaudeSettingsValues(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{
			name: "changed scalar",
			target: `{
  "permissions": {"allow": ["mcp__codegraph__codegraph_search"]},
  "model": "sonnet",
  "enabledPlugins": {"chrome-devtools-mcp": true}
}
`,
		},
		{
			name: "missing array item",
			target: `{
  "permissions": {"allow": ["mcp__chrome-devtools__new_page"]},
  "model": "opus",
  "enabledPlugins": {"chrome-devtools-mcp": true}
}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(manifest.Entry{
				Source:    "configs/claude/settings.json",
				Target:    "~/.claude/settings.json",
				Strategy:  "copy",
				Ownership: "json-subset",
				Tags:      []string{"core"},
			})
			f.sourceRoot = t.TempDir()
			f.home = t.TempDir()
			writeSource(t, f.sourceRoot, "configs/claude/settings.json", `{
  "permissions": {
    "allow": ["mcp__codegraph__codegraph_search"]
  },
  "model": "opus"
}
`)
			target := filepath.Join(f.home, ".claude", "settings.json")
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("mkdir target parent: %v", err)
			}
			if err := os.WriteFile(target, []byte(tt.target), 0o600); err != nil {
				t.Fatalf("write target: %v", err)
			}
			meta := state.Metadata{Version: 1, Entries: []state.Record{{
				Target: target, Source: "configs/claude/settings.json", Strategy: "copy", Hash: "installed-hash",
			}}}

			if got := onlyEntry(t, f.build(t, meta)).State; got != status.StateDrifted {
				t.Fatalf("state = %q, want drifted because a dots-owned JSON value diverged", got)
			}
		})
	}
}

func TestBuildReportsConflictForClaudeSettingsSubsetWithoutInstallMetadata(t *testing.T) {
	f := newFixture(manifest.Entry{
		Source:    "configs/claude/settings.json",
		Target:    "~/.claude/settings.json",
		Strategy:  "copy",
		Ownership: "json-subset",
		Tags:      []string{"core"},
	})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/claude/settings.json", `{"model": "opus"}`)
	target := filepath.Join(f.home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"model": "opus", "enabledPlugins": {"chrome-devtools-mcp": true}}`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateConflict {
		t.Fatalf("state = %q, want conflict because extra co-owned JSON is trusted only after dots install metadata", got)
	}
}

func TestBuildReportsMissingWhenTargetAbsent(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/git/gitconfig", Target: "~/.gitconfig", Strategy: "copy", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/git/gitconfig", "[user]\n")

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateMissing {
		t.Fatalf("state = %q, want missing", got)
	}
}

func TestBuildReportsDriftedForCopyDivergedFromSourceWithMetadata(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/git/gitconfig", Target: "~/.gitconfig", Strategy: "copy", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/git/gitconfig", "[user]\n\tname = Source\n")
	target := filepath.Join(f.home, ".gitconfig")
	if err := os.WriteFile(target, []byte("[user]\n\tname = Edited\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	meta := state.Metadata{Version: 1, Entries: []state.Record{{
		Target: target, Source: "configs/git/gitconfig", Strategy: "copy", Hash: "installed-hash",
	}}}

	if got := onlyEntry(t, f.build(t, meta)).State; got != status.StateDrifted {
		t.Fatalf("state = %q, want drifted (we installed it, target now differs)", got)
	}
}

func TestBuildReportsConflictWhenMetadataSourceDoesNotMatchManifestEntry(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/git/gitconfig", Target: "~/.gitconfig", Strategy: "copy", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/git/gitconfig", "[user]\n\tname = Source\n")
	target := filepath.Join(f.home, ".gitconfig")
	if err := os.WriteFile(target, []byte("[user]\n\tname = Edited\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	meta := state.Metadata{Version: 1, Entries: []state.Record{{
		Target: target, Source: "configs/old/gitconfig", Strategy: "copy", Hash: "installed-hash",
	}}}

	if got := onlyEntry(t, f.build(t, meta)).State; got != status.StateConflict {
		t.Fatalf("state = %q, want conflict because stale metadata source does not prove ownership", got)
	}
}

func TestBuildReportsConflictWhenMetadataStrategyDoesNotMatchManifestEntry(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/git/gitconfig", Target: "~/.gitconfig", Strategy: "copy", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/git/gitconfig", "[user]\n\tname = Source\n")
	target := filepath.Join(f.home, ".gitconfig")
	if err := os.WriteFile(target, []byte("[user]\n\tname = Edited\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	meta := state.Metadata{Version: 1, Entries: []state.Record{{
		Target: target, Source: "configs/git/gitconfig", Strategy: "symlink", Hash: "installed-hash",
	}}}

	if got := onlyEntry(t, f.build(t, meta)).State; got != status.StateConflict {
		t.Fatalf("state = %q, want conflict because stale metadata strategy does not prove ownership", got)
	}
}

func TestBuildReportsConflictForCopyDivergedFromSourceWithoutMetadata(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/git/gitconfig", Target: "~/.gitconfig", Strategy: "copy", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/git/gitconfig", "[user]\n\tname = Source\n")
	if err := os.WriteFile(filepath.Join(f.home, ".gitconfig"), []byte("foreign pre-existing file\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateConflict {
		t.Fatalf("state = %q, want conflict (foreign file, never installed by dots)", got)
	}
}

func TestBuildRejectsTargetParentSymlinkEscapeBeforeReadingTarget(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/git/gitconfig", Target: "~/.config/git/config", Strategy: "copy", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	outsideHome := t.TempDir()
	writeSource(t, f.sourceRoot, "configs/git/gitconfig", "[user]\n\tname = Source\n")
	if err := os.MkdirAll(filepath.Join(outsideHome, "git"), 0o755); err != nil {
		t.Fatalf("mkdir outside git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideHome, "git", "config"), []byte("foreign outside file\n"), 0o600); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Symlink(outsideHome, filepath.Join(f.home, ".config")); err != nil {
		t.Fatalf("symlink escaped target parent: %v", err)
	}

	_, err := status.Build(f.manifest, state.Metadata{}, status.Options{
		Profile:    "default",
		OS:         "linux",
		SourceRoot: f.sourceRoot,
		Home:       f.home,
	})
	if err == nil {
		t.Fatal("Build() error = nil, want target parent symlink escape error")
	}
}

func TestBuildRejectsSourceSymlinkEscape(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/secret", Target: "~/.secret", Strategy: "copy", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	outside := t.TempDir()
	outsideSecret := filepath.Join(outside, "secret")
	if err := os.WriteFile(outsideSecret, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("write outside source: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(f.sourceRoot, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.Symlink(outsideSecret, filepath.Join(f.sourceRoot, "configs", "secret")); err != nil {
		t.Fatalf("symlink escaped source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.home, ".secret"), []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	_, err := status.Build(f.manifest, state.Metadata{}, status.Options{
		Profile:    "default",
		OS:         "linux",
		SourceRoot: f.sourceRoot,
		Home:       f.home,
	})
	if err == nil {
		t.Fatal("Build() error = nil, want source symlink escape error")
	}
}

func TestBuildReportsDriftedForSymlinkPointingElsewhereWithMetadata(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/zsh/zshrc", "export A=1\n")
	target := filepath.Join(f.home, ".zshrc")
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(elsewhere, []byte("x\n"), 0o600); err != nil {
		t.Fatalf("write elsewhere: %v", err)
	}
	if err := os.Symlink(elsewhere, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	meta := state.Metadata{Version: 1, Entries: []state.Record{{
		Target: target, Source: "configs/zsh/zshrc", Strategy: "symlink", Hash: "installed-hash",
	}}}

	if got := onlyEntry(t, f.build(t, meta)).State; got != status.StateDrifted {
		t.Fatalf("state = %q, want drifted", got)
	}
}

func TestBuildReportsSkippedWhenOSFilterExcludesEntry(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/mac/app", Target: "~/.app", Strategy: "copy", Tags: []string{"core"}, OS: []string{"darwin"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateSkipped {
		t.Fatalf("state = %q, want skipped for darwin-only entry on linux", got)
	}
}

func TestBuildReportsMissingForAbsentTemplateTarget(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/starship.toml", Target: "~/.config/starship.toml", Strategy: "template", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/starship.toml", "format = '$all'\n")

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateMissing {
		t.Fatalf("state = %q, want missing", got)
	}
}

func TestBuildReportsUnsupportedForExistingTemplateTarget(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/starship.toml", Target: "~/.config/starship.toml", Strategy: "template", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()
	writeSource(t, f.sourceRoot, "configs/starship.toml", "format = '$all'\n")
	target := filepath.Join(f.home, ".config", "starship.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.WriteFile(target, []byte("format = '$directory'\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateUnsupported {
		t.Fatalf("state = %q, want unsupported (template rendering not implemented)", got)
	}
}

func TestBuildExcludesEntriesOutsideProfileTags(t *testing.T) {
	f := fixture{
		sourceRoot: t.TempDir(),
		home:       t.TempDir(),
		manifest: manifest.Manifest{
			Version:  1,
			Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
			Entries: []manifest.Entry{
				{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}},
				{Source: "configs/work/only", Target: "~/.work", Strategy: "copy", Tags: []string{"work"}},
			},
		},
	}
	writeSource(t, f.sourceRoot, "configs/zsh/zshrc", "export A=1\n")

	report := f.build(t, state.Metadata{})
	if len(report.Entries) != 1 {
		t.Fatalf("report entries = %d, want 1 (work entry excluded)", len(report.Entries))
	}
	if report.Entries[0].Source != "configs/zsh/zshrc" {
		t.Fatalf("unexpected entry %q in report", report.Entries[0].Source)
	}
}

func TestBuildFailsForUnknownProfile(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()

	_, err := status.Build(f.manifest, state.Metadata{}, status.Options{Profile: "missing", OS: "linux", SourceRoot: f.sourceRoot, Home: f.home})
	if err == nil {
		t.Fatal("Build() error = nil, want error for unknown profile")
	}
}

func TestBuildReportsOKForDirectorySymlinkPointingAtSource(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/nvim", Target: "~/.config/nvim", Strategy: "symlink", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()

	// Create a source directory with content (simulating configs/nvim/).
	sourceDirPath := filepath.Join(f.sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(filepath.Join(sourceDirPath, "lua"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDirPath, "init.lua"), []byte("-- init\n"), 0o600); err != nil {
		t.Fatalf("write init.lua: %v", err)
	}

	targetDir := filepath.Join(f.home, ".config", "nvim")
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(sourceDirPath, targetDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateOK {
		t.Fatalf("state = %q, want ok for directory symlink pointing at source dir", got)
	}
}

func TestBuildReportsConflictForDirectorySymlinkPointingElsewhere(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/nvim", Target: "~/.config/nvim", Strategy: "symlink", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()

	sourceDirPath := filepath.Join(f.sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(sourceDirPath, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}

	// The target symlink points at a different directory (not the managed source).
	otherDir := t.TempDir()
	targetDir := filepath.Join(f.home, ".config", "nvim")
	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(otherDir, targetDir); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateConflict {
		t.Fatalf("state = %q, want conflict for directory symlink pointing elsewhere", got)
	}
}

func TestBuildReportsMissingForAbsentDirectorySymlinkTarget(t *testing.T) {
	f := newFixture(manifest.Entry{Source: "configs/nvim", Target: "~/.config/nvim", Strategy: "symlink", Tags: []string{"core"}})
	f.sourceRoot = t.TempDir()
	f.home = t.TempDir()

	sourceDirPath := filepath.Join(f.sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(sourceDirPath, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}

	// Target does not exist yet.
	if got := onlyEntry(t, f.build(t, state.Metadata{})).State; got != status.StateMissing {
		t.Fatalf("state = %q, want missing when directory symlink target is absent", got)
	}
}
