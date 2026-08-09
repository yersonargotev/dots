package install_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/repositoryrefresh"
	"github.com/yersonargotev/dots/internal/state"
)

func TestLegacyNeovimDirectorySymlinkMigratesLockWithBackupAndProvenance(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	dotsStateRoot := filepath.Join(home, ".dots-state")
	xdgStateHome := filepath.Join(home, "xdg-state")
	legacyTarget := filepath.Join(home, ".config/nvim")
	legacySource := filepath.Join(sourceRoot, "configs/nvim")
	oldBaseline := []byte("{\"plugin\":\"old\"}\n")
	newBaseline := []byte("{\"plugin\":\"new\"}\n")
	localEvolution := []byte("{\"plugin\":\"local\"}\n")

	writeMigrationFile(t, filepath.Join(legacySource, "init.lua"), []byte("require('config.lazy')\n"))
	writeMigrationFile(t, filepath.Join(legacySource, "lazy-lock.json"), oldBaseline)
	oldManifest := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries:  []manifest.Entry{{Source: "configs/nvim", Target: "~/.config/nvim", Strategy: "symlink", Tags: []string{"core"}}},
	}
	newManifest := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{
			{Source: "configs/nvim/lazy-lock.json", Target: "nvim/lazy-lock.json", TargetRoot: "xdg-state", Strategy: "copy", Ownership: "seeded", Tags: []string{"core"}},
			{Source: "configs/nvim/loader.lua", Target: "~/.config/nvim/init.lua", Strategy: "copy", Tags: []string{"core"}},
			{Source: "configs/nvim", Target: "~/.config/dots/nvim", Strategy: "symlink", Tags: []string{"core"}},
		},
	}

	runMigrationGit(t, sourceRoot, "init")
	runMigrationGit(t, sourceRoot, "config", "user.email", "dots@example.test")
	runMigrationGit(t, sourceRoot, "config", "user.name", "dots test")
	runMigrationGit(t, sourceRoot, "add", ".")
	runMigrationGit(t, sourceRoot, "commit", "-m", "legacy baseline")
	revision := runMigrationGit(t, sourceRoot, "rev-parse", "HEAD")

	if err := os.MkdirAll(filepath.Dir(legacyTarget), 0o755); err != nil {
		t.Fatalf("mkdir legacy parent: %v", err)
	}
	if err := os.Symlink(legacySource, legacyTarget); err != nil {
		t.Fatalf("symlink legacy Neovim directory: %v", err)
	}
	writeMigrationFile(t, filepath.Join(legacySource, "lazy-lock.json"), localEvolution)
	meta := state.Metadata{
		Version:    state.CurrentVersion,
		Provenance: state.Provenance{SourceRoot: sourceRoot, SourceRevision: revision},
		Entries: []state.Record{{
			Target: legacyTarget, Source: "configs/nvim", Strategy: "symlink", Ownership: "whole",
		}},
	}
	if err := state.Save(state.Path(dotsStateRoot), meta); err != nil {
		t.Fatalf("save legacy metadata: %v", err)
	}

	captures, err := repositoryrefresh.CaptureLegacyTargets(oldManifest, newManifest, meta, sourceRoot, home, xdgStateHome, revision)
	if err != nil {
		t.Fatalf("CaptureLegacyTargets() error = %v", err)
	}
	seedTarget := filepath.Join(xdgStateHome, "nvim/lazy-lock.json")
	capture, ok := captures[seedTarget]
	if !ok || capture.LegacyTarget != legacyTarget || string(capture.CapturedContent) != string(localEvolution) {
		t.Fatalf("seeded directory capture = %#v", capture)
	}

	// Simulate repository refresh after capture: the legacy symlink now exposes
	// the incoming baseline, while the captured local revisions remain durable.
	writeMigrationFile(t, filepath.Join(legacySource, "lazy-lock.json"), newBaseline)
	writeMigrationFile(t, filepath.Join(legacySource, "loader.lua"), []byte("local managed = true\n"))
	p, err := plan.Build(newManifest, plan.Options{
		Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home,
		XDGStateHome: xdgStateHome, Metadata: meta, LegacyMigrations: captures,
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Actions) != 3 || p.Actions[0].Status != plan.StatusMigrate || p.Actions[1].Status != plan.StatusCreate {
		t.Fatalf("migration plan = %#v", p.Actions)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: dotsStateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if info, err := os.Lstat(legacyTarget); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("native Neovim directory = (%v, %v), want regular directory", info, err)
	}
	if got, err := os.ReadFile(seedTarget); err != nil || string(got) != string(localEvolution) {
		t.Fatalf("migrated lock = (%q, %v), want local revisions", got, err)
	}
	backupMeta, err := backups.Load(backups.Path(dotsStateRoot))
	if err != nil || len(backupMeta.Sets) != 1 {
		t.Fatalf("Backup Metadata = (%#v, %v), want one set", backupMeta, err)
	}
	legacyLock := filepath.Join(legacyTarget, "lazy-lock.json")
	if len(backupMeta.Sets[0].Targets) != 1 || backupMeta.Sets[0].Targets[0] != legacyLock {
		t.Fatalf("backup targets = %#v, want %s", backupMeta.Sets[0].Targets, legacyLock)
	}
	backupFile := backups.FilePath(dotsStateRoot, backupMeta.Sets[0].ID, 1, legacyLock)
	if got, err := os.ReadFile(backupFile); err != nil || string(got) != string(localEvolution) {
		t.Fatalf("backup content = (%q, %v), want local revisions", got, err)
	}

	gotMeta, err := state.Load(state.Path(dotsStateRoot))
	if err != nil {
		t.Fatalf("load migrated metadata: %v", err)
	}
	if _, ok := gotMeta.FindByTarget(legacyTarget); ok {
		t.Fatal("legacy directory ownership record was not pruned")
	}
	seedRecord, ok := gotMeta.FindByTarget(seedTarget)
	if !ok || seedRecord.Ownership != "seeded" || string(seedRecord.SeededBaseline) != string(oldBaseline) {
		t.Fatalf("seeded ownership record = %#v, want preserved old baseline evidence", seedRecord)
	}
}

func writeMigrationFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runMigrationGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(bytesTrimSpace(out))
}

func bytesTrimSpace(value []byte) []byte {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\n' || value[start] == '\r' || value[start] == '\t') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\n' || value[end-1] == '\r' || value[end-1] == '\t') {
		end--
	}
	return value[start:end]
}
