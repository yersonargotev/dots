package install_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestApplyComposedJSONSubsetCreatesAndUpdatesOneSharedTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	for rel, content := range map[string]string{
		"configs/base.json":   `{"editor":{"theme":"dark"},"servers":["one"]}`,
		"configs/mobile.json": `{"mobile":true,"servers":["two"]}`,
	} {
		path := filepath.Join(sourceRoot, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir source: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}
	m := manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"base", "mobile"}}},
		Entries: []manifest.Entry{
			{Source: "configs/base.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"base"}},
			{Source: "configs/mobile.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"mobile"}},
		},
	}
	opts := plan.Options{Profile: "default", OS: "darwin", SourceRoot: sourceRoot, Home: home}
	p, err := plan.Build(m, opts)
	if err != nil {
		t.Fatalf("Build(create) error = %v", err)
	}
	if len(p.Actions) != 1 {
		t.Fatalf("create actions = %d, want one", len(p.Actions))
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(create) error = %v", err)
	}

	target := filepath.Join(home, ".config", "shared.json")
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatalf("metadata missing target %s", target)
	}
	wantSources := []string{"configs/base.json", "configs/mobile.json"}
	if !reflect.DeepEqual(rec.SourceList(), wantSources) {
		t.Fatalf("metadata sources = %#v, want %#v", rec.SourceList(), wantSources)
	}
	targetHash, err := state.HashFile(target)
	if err != nil {
		t.Fatalf("hash target: %v", err)
	}
	if rec.Hash != targetHash {
		t.Fatalf("metadata hash = %q, want composed target hash %q", rec.Hash, targetHash)
	}

	if err := os.WriteFile(target, []byte(`{"editor":{"theme":"dark"},"servers":["one"],"userOnly":"keep"}`), 0o640); err != nil {
		t.Fatalf("write trusted target: %v", err)
	}
	opts.Metadata = meta
	p, err = plan.Build(m, opts)
	if err != nil {
		t.Fatalf("Build(update) error = %v", err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusUpdate {
		t.Fatalf("update actions = %+v, want one update", p.Actions)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply(update) error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read updated target: %v", err)
	}
	for _, want := range []string{`"userOnly": "keep"`, `"mobile": true`, `"two"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("updated target missing %s:\n%s", want, got)
		}
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(backupMeta.Sets) != 1 {
		t.Fatalf("backup sets = %d, want one shared-target backup", len(backupMeta.Sets))
	}
	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("reload metadata: %v", err)
	}
	uninstallPlan, err := plan.BuildUninstall(meta, plan.UninstallOptions{SourceRoot: sourceRoot, Home: home})
	if err != nil {
		t.Fatalf("BuildUninstall() error = %v", err)
	}
	var shared *plan.UninstallAction
	for i := range uninstallPlan.Actions {
		if uninstallPlan.Actions[i].Target == target {
			shared = &uninstallPlan.Actions[i]
			break
		}
	}
	if shared == nil || shared.Status != plan.UninstallRemove || shared.Ownership != "json-subset" {
		t.Fatalf("shared uninstall action = %+v, want partial removal that preserves target-only state", shared)
	}
}

func TestApplyCreatesSymlinkForCreateAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "zsh", ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/zsh/zshrc",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	gotDest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink target: %v", err)
	}
	if gotDest != sourcePath {
		t.Fatalf("symlink target = %q, want %q", gotDest, sourcePath)
	}
}

func TestApplyDefaultsConflictActionsToSkipAndContinuesSafeCreates(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	createSource := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(createSource), 0o755); err != nil {
		t.Fatalf("mkdir create source: %v", err)
	}
	if err := os.WriteFile(createSource, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write create source: %v", err)
	}
	conflictSource := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(conflictSource), 0o755); err != nil {
		t.Fatalf("mkdir conflict source: %v", err)
	}
	if err := os.WriteFile(conflictSource, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write conflict source: %v", err)
	}

	createdTarget := filepath.Join(home, ".zshrc")
	conflictingTarget := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(conflictingTarget, []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write conflicting target: %v", err)
	}
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: createdTarget, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/git/gitconfig", Target: conflictingTarget, Strategy: "copy", Status: plan.StatusConflict},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Lstat(createdTarget); err != nil {
		t.Fatalf("created target missing after conflict skip default: %v", err)
	}
	got, err := os.ReadFile(conflictingTarget)
	if err != nil {
		t.Fatalf("read conflicting target: %v", err)
	}
	if string(got) != "local\n" {
		t.Fatalf("conflicting target = %q, want original local content", got)
	}
}

func TestApplyReplaceConflictCreatesBackupSetBeforeChangingTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o640); err != nil {
		t.Fatalf("write local target: %v", err)
	}
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/git/gitconfig",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusConflict,
	}}}

	if err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		StateRoot:  stateRoot,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionReplace,
		},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read replaced target: %v", err)
	}
	if string(got) != "managed\n" {
		t.Fatalf("target contents = %q, want managed source", got)
	}
	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets = %d, want 1", len(meta.Sets))
	}
	if len(meta.Sets[0].Targets) != 1 || meta.Sets[0].Targets[0] != target {
		t.Fatalf("Backup Set targets = %v, want [%s]", meta.Sets[0].Targets, target)
	}
}

func TestApplyReplaceConflictReplacesDirectoryTargetWithSymlink(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	sourceDir := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(filepath.Join(sourceDir, "lua"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "init.lua"), []byte("-- managed\n"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	target := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(filepath.Join(target, "plugin"), 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	localFile := filepath.Join(target, "plugin", "local.lua")
	if err := os.WriteFile(localFile, []byte("-- local\n"), 0o600); err != nil {
		t.Fatalf("write local target file: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusConflict,
	}}}

	if err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		StateRoot:  stateRoot,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionReplace,
		},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	dest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink target: %v", err)
	}
	if dest != sourceDir {
		t.Fatalf("symlink dest = %q, want %q", dest, sourceDir)
	}

	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets = %d, want 1", len(meta.Sets))
	}
	preserved := filepath.Join(backups.FilePath(stateRoot, meta.Sets[0].ID, 1, target), "plugin", "local.lua")
	got, err := os.ReadFile(preserved)
	if err != nil {
		t.Fatalf("read preserved directory file: %v", err)
	}
	if string(got) != "-- local\n" {
		t.Fatalf("preserved directory file = %q, want local content", got)
	}
}

func TestApplyReplaceConflictRemovesNonWritableDirectoryTargetWithSymlink(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	sourceDir := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "init.lua"), []byte("-- managed\n"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	target := filepath.Join(home, ".config", "nvim")
	lockedDir := filepath.Join(target, "plugin")
	if err := os.MkdirAll(lockedDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}
	localFile := filepath.Join(lockedDir, "local.lua")
	if err := os.WriteFile(localFile, []byte("-- local\n"), 0o400); err != nil {
		t.Fatalf("write local target file: %v", err)
	}
	if err := os.Chmod(lockedDir, 0o555); err != nil {
		t.Fatalf("chmod locked target dir: %v", err)
	}
	t.Cleanup(func() {
		makeTreeWritableForCleanup(home)
		makeTreeWritableForCleanup(stateRoot)
	})

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusConflict,
	}}}

	if err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		StateRoot:  stateRoot,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionReplace,
		},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	dest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink target: %v", err)
	}
	if dest != sourceDir {
		t.Fatalf("symlink dest = %q, want %q", dest, sourceDir)
	}

	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets = %d, want 1", len(meta.Sets))
	}
	preserved := filepath.Join(backups.FilePath(stateRoot, meta.Sets[0].ID, 1, target), "plugin", "local.lua")
	got, err := os.ReadFile(preserved)
	if err != nil {
		t.Fatalf("read preserved directory file: %v", err)
	}
	if string(got) != "-- local\n" {
		t.Fatalf("preserved directory file = %q, want local content", got)
	}
}

func TestApplyAdoptConflictCopiesTargetIntoSourceAndRecordsMetadata(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o640); err != nil {
		t.Fatalf("write local target: %v", err)
	}
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/git/gitconfig",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusConflict,
	}}}

	if err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		StateRoot:  stateRoot,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionAdopt,
		},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	gotSource, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read adopted source: %v", err)
	}
	if string(gotSource) != "local\n" {
		t.Fatalf("source contents = %q, want adopted local target", gotSource)
	}
	gotTarget, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(gotTarget) != "local\n" {
		t.Fatalf("target contents = %q, want local target left in place", gotTarget)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Installation Metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatalf("Installation Metadata missing adopted target %s", target)
	}
	wantHash, err := state.HashFile(sourcePath)
	if err != nil {
		t.Fatalf("hash adopted source: %v", err)
	}
	if rec.Hash != wantHash {
		t.Fatalf("record hash = %q, want adopted source hash %q", rec.Hash, wantHash)
	}
}

func makeTreeWritableForCleanup(root string) {
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.IsDir() {
			_ = os.Chmod(path, 0o755)
		}
		return nil
	})
}

func TestApplyCopiesRegularFileForCreateAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("[user]\n\tname = Test\n"), 0o640); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".config", "git", "config")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/git/gitconfig",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read copied target: %v", err)
	}
	if string(got) != "[user]\n\tname = Test\n" {
		t.Fatalf("copied contents = %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat copied target: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("copied mode = %v, want %v", gotMode, os.FileMode(0o640))
	}
}

func TestApplyLeavesUnchangedActionUntouched(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(target, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("write existing target: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/zsh/zshrc",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusUnchanged,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: filepath.Join(home, "missing-source-root"), Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "existing\n" {
		t.Fatalf("target contents = %q, want existing content", got)
	}
}

func TestApplyStatusUpdateMergesJSONSubsetAndRecordsMetadata(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte(`{
  "permissions": {
    "defaultMode": "bypassPermissions",
    "allow": ["Read", "Bash(git *)"]
  }
}`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{
  "permissions": {
    "allow": ["Read"],
    "deny": ["Bash(rm -rf *)"]
  },
  "enabledPlugins": {
    "chrome-devtools-mcp": true
  }
}`), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}

	p := plan.Plan{Profile: "core", Actions: []plan.Action{{
		Source:    "configs/claude/settings.json",
		Target:    target,
		Strategy:  "copy",
		Status:    plan.StatusUpdate,
		Ownership: "json-subset",
	}}}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	for _, want := range []string{
		`"defaultMode": "bypassPermissions"`,
		`"Bash(git *)"`,
		`"deny"`,
		`"enabledPlugins"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("target missing %q\ncontent:\n%s", want, got)
		}
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("target mode = %v, want 0640", gotMode)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatalf("metadata missing target %s", target)
	}
	if rec.Source != "configs/claude/settings.json" {
		t.Fatalf("metadata source = %q, want Claude settings source", rec.Source)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(backupMeta.Sets) != 1 {
		t.Fatalf("backup sets = %d, want 1", len(backupMeta.Sets))
	}
}

func TestApplyReconcilesRecordedJSONContributionAndPreservesExternalContent(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "shared.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	previous := []byte(`{"owned":{"keep":true,"retired":"old"},"items":["old"]}`)
	current := []byte(`{"owned":{"keep":true,"added":"new"},"items":["new"]}`)
	if err := os.WriteFile(source, current, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".config", "shared.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{"owned":{"keep":true,"retired":"old","external":"preserve"},"items":["old","external"],"targetOnly":true}`), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{{
		Target: target, Source: "configs/shared.json", Strategy: "copy", Ownership: "json-subset", OwnedContent: previous,
	}}}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	m := manifest.Manifest{
		Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
		Entries: []manifest.Entry{{Source: "configs/shared.json", Target: "~/.config/shared.json", Strategy: "copy", Ownership: "json-subset", Tags: []string{"core"}}},
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	p, err := plan.Build(m, plan.Options{Profile: "default", OS: "linux", SourceRoot: sourceRoot, Home: home, Metadata: meta})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Actions) != 1 || p.Actions[0].Status != plan.StatusUpdate {
		t.Fatalf("actions = %+v, want reversible update", p.Actions)
	}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	for _, want := range []string{`"added": "new"`, `"external": "preserve"`, `"targetOnly": true`, `"external"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("reconciled target missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), `"retired"`) || strings.Contains(string(got), `"old"`) {
		t.Fatalf("reconciled target retained retired contribution:\n%s", got)
	}
	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("reload metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Ownership != "json-subset" || !json.Valid(rec.OwnedContent) {
		t.Fatalf("metadata record = %+v, want valid owned JSON evidence", rec)
	}
	if meta.Version != state.CurrentVersion {
		t.Fatalf("metadata version = %d, want %d", meta.Version, state.CurrentVersion)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 1 {
		t.Fatalf("Backup Sets = %+v, err = %v; want one", backupMeta.Sets, err)
	}
}

func TestApplyStatusUpdateMergesTOMLSubsetAndRecordsMetadata(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "codex", "config-codegraph.toml")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte(`sandbox_mode = "danger-full-access"
approval_policy = "never"

[[hooks.SessionStart]]
matcher = "startup|resume"

[[hooks.SessionStart.hooks]]
type = "command"
command = "codegraph init"
timeout = 120
`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte(`model = "gpt-5.5"
sandbox_mode = "danger-full-access"
approval_policy = "never"

[tui]
theme = "catppuccin-mocha"
`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:    "configs/codex/config-codegraph.toml",
		Target:    target,
		Strategy:  "copy",
		Status:    plan.StatusUpdate,
		Ownership: "toml-subset",
	}}}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	for _, want := range []string{`model = "gpt-5.5"`, `[tui]`, `command = "codegraph init"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("target missing %q\ncontent:\n%s", want, got)
		}
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatalf("metadata missing target %s", target)
	}
	if rec.Source != "configs/codex/config-codegraph.toml" {
		t.Fatalf("metadata source = %q, want CodeGraph source", rec.Source)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(backupMeta.Sets) != 1 {
		t.Fatalf("backup sets = %d, want 1", len(backupMeta.Sets))
	}
}

func TestApplyStatusUpdateRejectsSymlinkTargetBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	source := filepath.Join(sourceRoot, "configs", "codex", "config-codegraph.toml")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte(`[[hooks.SessionStart]]
matcher = "startup|resume"
`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.toml")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	target := filepath.Join(home, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.Symlink(outside, target); err != nil {
		t.Fatalf("symlink target: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:    "configs/codex/config-codegraph.toml",
		Target:    target,
		Strategy:  "copy",
		Status:    plan.StatusUpdate,
		Ownership: "toml-subset",
	}}}
	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Fatal("Apply() error = nil, want symlink update target rejection")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside target: %v", err)
	}
	if string(got) != "outside\n" {
		t.Fatalf("outside target changed to %q", got)
	}
}

func TestApplyRejectsMissingSourceWithoutMutatingAnyAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	wouldCreate := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: wouldCreate, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/missing", Target: filepath.Join(home, ".missing"), Strategy: "copy", Status: plan.StatusMissingSource},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want missing-source error")
	}
	if _, err := os.Lstat(wouldCreate); !os.IsNotExist(err) {
		t.Fatalf("would-create target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsUnsafeTargetWithoutMutatingAnyAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")

	sourcePath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	wouldCreate := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: wouldCreate, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/zsh/zshrc", Target: outside, Strategy: "symlink", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want unsafe target error")
	}
	if _, err := os.Lstat(wouldCreate); !os.IsNotExist(err) {
		t.Fatalf("would-create target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsUnsafeSourceWithoutCopyingOutsideFile(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	outsidePath := filepath.Join(filepath.Dir(sourceRoot), "outside")
	if err := os.WriteFile(outsidePath, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside source: %v", err)
	}

	target := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "../outside",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want unsafe source error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsTargetParentSymlinkEscape(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideHome := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.Symlink(outsideHome, filepath.Join(home, ".config")); err != nil {
		t.Fatalf("symlink escaped parent: %v", err)
	}

	target := filepath.Join(home, ".config", "zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/zsh/zshrc",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want target parent symlink escape error")
	}
	if _, err := os.Lstat(filepath.Join(outsideHome, "zshrc")); !os.IsNotExist(err) {
		t.Fatalf("outside-home target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsDuplicateCreateTargetsBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	firstSource := filepath.Join(sourceRoot, "configs", "first")
	if err := os.MkdirAll(filepath.Dir(firstSource), 0o755); err != nil {
		t.Fatalf("mkdir first source: %v", err)
	}
	if err := os.WriteFile(firstSource, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("write first source: %v", err)
	}
	secondSource := filepath.Join(sourceRoot, "configs", "second")
	if err := os.WriteFile(secondSource, []byte("second\n"), 0o600); err != nil {
		t.Fatalf("write second source: %v", err)
	}

	target := filepath.Join(home, ".dupe")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/first", Target: target, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "configs/second", Target: target, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Fatal("Apply() error = nil, want duplicate target error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("duplicate target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(state.Path(stateRoot)); !os.IsNotExist(err) {
		t.Fatalf("metadata exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsDuplicateCreateTargetsAfterNormalizingTargetPathBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	firstSource := filepath.Join(sourceRoot, "configs", "first")
	if err := os.MkdirAll(filepath.Dir(firstSource), 0o755); err != nil {
		t.Fatalf("mkdir first source: %v", err)
	}
	if err := os.WriteFile(firstSource, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("write first source: %v", err)
	}
	secondSource := filepath.Join(sourceRoot, "configs", "second")
	if err := os.WriteFile(secondSource, []byte("second\n"), 0o600); err != nil {
		t.Fatalf("write second source: %v", err)
	}

	target := filepath.Join(home, ".dupe")
	lexicalVariant := filepath.Join(home, "nested") + string(filepath.Separator) + ".." + string(filepath.Separator) + ".dupe"
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/first", Target: target, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "configs/second", Target: lexicalVariant, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want duplicate target error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("duplicate target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyRejectsSourceSymlinkEscapeWithoutMutatingAnyAction(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideRoot := t.TempDir()

	safeSource := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(safeSource), 0o755); err != nil {
		t.Fatalf("mkdir safe source: %v", err)
	}
	if err := os.WriteFile(safeSource, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write safe source: %v", err)
	}

	outsideSecret := filepath.Join(outsideRoot, "secret")
	if err := os.WriteFile(outsideSecret, []byte("secret\n"), 0o600); err != nil {
		t.Fatalf("write outside secret: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	if err := os.Symlink(outsideSecret, filepath.Join(sourceRoot, "configs", "link")); err != nil {
		t.Fatalf("symlink escaped source: %v", err)
	}

	wouldCreate := filepath.Join(home, ".zshrc")
	escapedTarget := filepath.Join(home, ".secret")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: wouldCreate, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "configs/link", Target: escapedTarget, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want source symlink escape error")
	}
	if _, err := os.Lstat(wouldCreate); !os.IsNotExist(err) {
		t.Fatalf("earlier safe target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(escapedTarget); !os.IsNotExist(err) {
		t.Fatalf("escaped source target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyStatusCreateCopyDoesNotOverwriteExistingTarget(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("user-owned\n"), 0o640); err != nil {
		t.Fatalf("write existing target: %v", err)
	}
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/git/gitconfig",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want stale create error")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read existing target: %v", err)
	}
	if string(got) != "user-owned\n" {
		t.Fatalf("target contents = %q, want original user-owned contents", got)
	}
}

func TestApplyRecordsInstallationMetadataForCreatedTargets(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	symlinkSrc := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(symlinkSrc), 0o755); err != nil {
		t.Fatalf("mkdir symlink source: %v", err)
	}
	if err := os.WriteFile(symlinkSrc, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write symlink source: %v", err)
	}
	copySrc := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(copySrc), 0o755); err != nil {
		t.Fatalf("mkdir copy source: %v", err)
	}
	if err := os.WriteFile(copySrc, []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write copy source: %v", err)
	}

	symlinkTarget := filepath.Join(home, ".zshrc")
	copyTarget := filepath.Join(home, ".gitconfig")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: symlinkTarget, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/git/gitconfig", Target: copyTarget, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(meta.Entries) != 2 {
		t.Fatalf("metadata entries = %d, want 2\n%+v", len(meta.Entries), meta.Entries)
	}

	rec, ok := meta.FindByTarget(copyTarget)
	if !ok {
		t.Fatalf("metadata missing copy target %s", copyTarget)
	}
	wantHash, err := state.HashFile(copySrc)
	if err != nil {
		t.Fatalf("hash copy source: %v", err)
	}
	if rec.Hash != wantHash {
		t.Fatalf("copy record hash = %q, want %q", rec.Hash, wantHash)
	}
	if rec.Strategy != "copy" || rec.Source != "configs/git/gitconfig" {
		t.Fatalf("copy record = %+v, want strategy copy / source configs/git/gitconfig", rec)
	}
	if rec.InstalledAt == "" {
		t.Fatalf("copy record InstalledAt is empty")
	}

	symlinkRec, ok := meta.FindByTarget(symlinkTarget)
	if !ok {
		t.Fatalf("metadata missing symlink target %s", symlinkTarget)
	}
	if symlinkRec.Hash != "" {
		t.Fatalf("symlink record hash = %q, want empty", symlinkRec.Hash)
	}
}

func TestApplyRejectsStateRootSymlinkEscapeBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideState := t.TempDir()

	src := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stateParent := filepath.Join(home, ".local", "state")
	if err := os.MkdirAll(stateParent, 0o755); err != nil {
		t.Fatalf("mkdir state parent: %v", err)
	}
	stateRoot := filepath.Join(stateParent, "dots")
	if err := os.Symlink(outsideState, stateRoot); err != nil {
		t.Fatalf("symlink state root: %v", err)
	}

	target := filepath.Join(home, ".gitconfig")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/git/gitconfig", Target: target, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Fatal("Apply() error = nil, want state root symlink escape error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(outsideState, "installed.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata was written outside home through state symlink; lstat err = %v", err)
	}
}

func TestApplyRejectsMetadataLeafSymlinkEscapeBeforeMutation(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	outsideState := t.TempDir()

	src := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	stateRoot := filepath.Join(home, ".local", "state", "dots")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	outsideMetadata := filepath.Join(outsideState, "installed.json")
	if err := os.Symlink(outsideMetadata, filepath.Join(stateRoot, "installed.json")); err != nil {
		t.Fatalf("symlink metadata leaf: %v", err)
	}

	target := filepath.Join(home, ".gitconfig")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/git/gitconfig", Target: target, Strategy: "copy", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err == nil {
		t.Fatal("Apply() error = nil, want metadata leaf symlink escape error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
	if _, err := os.Lstat(outsideMetadata); !os.IsNotExist(err) {
		t.Fatalf("metadata was written outside home through leaf symlink; lstat err = %v", err)
	}
}

func TestApplyWithoutStateRootWritesNoMetadata(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: target, Strategy: "symlink", Status: plan.StatusCreate},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("expected target created even without state root: %v", err)
	}
}

func TestApplyMergesMetadataAcrossRunsWithoutDuplicating(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()
	stateRoot := t.TempDir()

	src := filepath.Join(sourceRoot, "configs/git/gitconfig")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("[user]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target := filepath.Join(home, ".gitconfig")
	create := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/git/gitconfig", Target: target, Strategy: "copy", Status: plan.StatusCreate},
	}}
	if err := install.Apply(create, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}

	// Re-running with the same target already in place yields an Unchanged
	// action; metadata must refresh in place rather than appending a duplicate.
	unchanged := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/git/gitconfig", Target: target, Strategy: "copy", Status: plan.StatusUnchanged},
	}}
	if err := install.Apply(unchanged, install.Options{SourceRoot: sourceRoot, Home: home, StateRoot: stateRoot}); err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if len(meta.Entries) != 1 {
		t.Fatalf("metadata entries = %d, want 1 (no duplicate)\n%+v", len(meta.Entries), meta.Entries)
	}
}

func TestApplyStatusCreateSymlinkRejectsMissingSource(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	target := filepath.Join(home, ".zshrc")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/zsh/zshrc",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want missing source error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("dangling symlink exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyCreatesDirectorySymlinkForDirectorySource(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	// Create a source directory with content (simulating configs/nvim/).
	sourceDirPath := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(filepath.Join(sourceDirPath, "lua"), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDirPath, "init.lua"), []byte("-- init\n"), 0o600); err != nil {
		t.Fatalf("write init.lua: %v", err)
	}

	target := filepath.Join(home, ".config", "nvim")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("lstat target: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("target mode = %v, want symlink", info.Mode())
	}
	gotDest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink target: %v", err)
	}
	if gotDest != sourceDirPath {
		t.Fatalf("symlink dest = %q, want %q", gotDest, sourceDirPath)
	}
}

func TestApplyCopyStrategyRejectsDirectorySourceBeforeInstall(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	// Create a source directory (not a file).
	sourceDirPath := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(sourceDirPath, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}

	target := filepath.Join(home, ".config", "nvim")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "copy",
		Status:   plan.StatusCreate,
	}}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want error for directory source with copy strategy")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
}

func TestApplyAdoptConflictRejectsDirectoryTargetWithActionableError(t *testing.T) {
	sourceRoot := t.TempDir()
	home := t.TempDir()

	sourceDir := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	target := filepath.Join(home, ".config", "nvim")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir target dir: %v", err)
	}

	p := plan.Plan{Profile: "default", Actions: []plan.Action{{
		Source:   "configs/nvim",
		Target:   target,
		Strategy: "symlink",
		Status:   plan.StatusConflict,
	}}}

	err := install.Apply(p, install.Options{
		SourceRoot: sourceRoot,
		Home:       home,
		ConflictDecisions: map[string]install.ConflictDecision{
			target: install.DecisionAdopt,
		},
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want directory adopt error")
	}
	if !strings.Contains(err.Error(), "Adopting directory target") && !strings.Contains(err.Error(), "adopting directory target") {
		t.Fatalf("error %q does not explain directory adopt is unsupported", err)
	}
	if !strings.Contains(err.Error(), "replace") {
		t.Fatalf("error %q does not suggest replace", err)
	}
}
