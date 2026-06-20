package uninstall_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/uninstall"
)

// env wires the three sandbox roots an uninstall touches and records what it
// installs so each test can drive Apply against realistic Installation Metadata.
type env struct {
	sourceRoot string
	home       string
	stateRoot  string
	meta       state.Metadata
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return &env{
		sourceRoot: t.TempDir(),
		home:       t.TempDir(),
		stateRoot:  t.TempDir(),
		meta:       state.Metadata{Version: 1},
	}
}

func (e *env) writeSource(t *testing.T, rel, content string) string {
	t.Helper()
	abs := filepath.Join(e.sourceRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return abs
}

// installSymlink mirrors what dots install records for a symlink entry: the link
// on disk plus a Hash-less metadata record.
func (e *env) installSymlink(t *testing.T, rel, name string) string {
	t.Helper()
	src := e.writeSource(t, rel, "managed "+name+"\n")
	target := filepath.Join(e.home, name)
	if err := os.Symlink(src, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	e.meta.Entries = append(e.meta.Entries, state.Record{Target: target, Source: rel, Strategy: "symlink"})
	return target
}

// installCopy mirrors what dots install records for a copy entry: the copied file
// plus a record carrying the source content hash.
func (e *env) installCopy(t *testing.T, rel, name string) string {
	t.Helper()
	content := "managed " + name + "\n"
	src := e.writeSource(t, rel, content)
	hash, err := state.HashFile(src)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	target := filepath.Join(e.home, name)
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	e.meta.Entries = append(e.meta.Entries, state.Record{Target: target, Source: rel, Strategy: "copy", Hash: hash})
	return target
}

func (e *env) saveMeta(t *testing.T) {
	t.Helper()
	if err := state.Save(state.Path(e.stateRoot), e.meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
}

func (e *env) buildPlan(t *testing.T) plan.UninstallPlan {
	t.Helper()
	p, err := plan.BuildUninstall(e.meta, plan.UninstallOptions{SourceRoot: e.sourceRoot})
	if err != nil {
		t.Fatalf("BuildUninstall: %v", err)
	}
	return p
}

func (e *env) apply(t *testing.T, opts uninstall.Options) uninstall.Result {
	t.Helper()
	opts.SourceRoot = e.sourceRoot
	opts.Home = e.home
	opts.StateRoot = e.stateRoot
	res, err := uninstall.Apply(e.buildPlan(t), opts)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return res
}

func loadMeta(t *testing.T, stateRoot string) state.Metadata {
	t.Helper()
	m, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	return m
}

func TestApplyRemovesOwnedSymlinkAndPrunesMetadata(t *testing.T) {
	e := newEnv(t)
	target := e.installSymlink(t, "shell/zshrc", ".zshrc")
	e.saveMeta(t)

	res := e.apply(t, uninstall.Options{})

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("symlink still present, err = %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != target {
		t.Fatalf("Removed = %v, want [%s]", res.Removed, target)
	}
	if _, ok := loadMeta(t, e.stateRoot).FindByTarget(target); ok {
		t.Fatal("metadata record not pruned")
	}
}

func TestApplyRemovesOwnedCopyAndPrunesMetadata(t *testing.T) {
	e := newEnv(t)
	target := e.installCopy(t, "git/gitconfig", ".gitconfig")
	e.saveMeta(t)

	e.apply(t, uninstall.Options{})

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("copy still present, err = %v", err)
	}
	if _, ok := loadMeta(t, e.stateRoot).FindByTarget(target); ok {
		t.Fatal("metadata record not pruned")
	}
}

func TestApplyDeletesMetadataFileWhenEmpty(t *testing.T) {
	e := newEnv(t)
	e.installSymlink(t, "shell/zshrc", ".zshrc")
	e.saveMeta(t)

	e.apply(t, uninstall.Options{})

	if _, err := os.Lstat(state.Path(e.stateRoot)); !os.IsNotExist(err) {
		t.Fatalf("installed.json should be deleted when empty, err = %v", err)
	}
}

func TestApplyKeepsRecordsForUnremovedTargets(t *testing.T) {
	e := newEnv(t)
	removed := e.installSymlink(t, "shell/zshrc", ".zshrc")
	// A symlink the user repointed elsewhere is not-owned and must be kept.
	kept := filepath.Join(e.home, ".tmux.conf")
	other := e.writeSource(t, "decoy", "decoy\n")
	if err := os.Symlink(other, kept); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	e.meta.Entries = append(e.meta.Entries, state.Record{Target: kept, Source: "term/tmux.conf", Strategy: "symlink"})
	e.saveMeta(t)

	e.apply(t, uninstall.Options{})

	meta := loadMeta(t, e.stateRoot)
	if _, ok := meta.FindByTarget(removed); ok {
		t.Fatal("removed record should be pruned")
	}
	if _, ok := meta.FindByTarget(kept); !ok {
		t.Fatal("not-owned record should be kept")
	}
	if _, err := os.Lstat(kept); err != nil {
		t.Fatalf("not-owned symlink should remain, err = %v", err)
	}
}

func TestApplySkipsModifiedCopyWithoutForce(t *testing.T) {
	e := newEnv(t)
	target := e.installCopy(t, "git/gitconfig", ".gitconfig")
	if err := os.WriteFile(target, []byte("user edited\n"), 0o600); err != nil {
		t.Fatalf("edit target: %v", err)
	}
	e.saveMeta(t)

	res := e.apply(t, uninstall.Options{})

	if len(res.Removed) != 0 {
		t.Fatalf("Removed = %v, want none for modified copy", res.Removed)
	}
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("modified copy should remain, err = %v", err)
	}
	if _, ok := loadMeta(t, e.stateRoot).FindByTarget(target); !ok {
		t.Fatal("modified record should be kept")
	}
}

func TestApplyRemovesModifiedCopyWithForce(t *testing.T) {
	e := newEnv(t)
	target := e.installCopy(t, "git/gitconfig", ".gitconfig")
	if err := os.WriteFile(target, []byte("user edited\n"), 0o600); err != nil {
		t.Fatalf("edit target: %v", err)
	}
	e.saveMeta(t)

	res := e.apply(t, uninstall.Options{Force: true})

	if len(res.Removed) != 1 {
		t.Fatalf("Removed = %v, want 1 with force", res.Removed)
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("modified copy should be removed with force, err = %v", err)
	}
}

func TestApplyRestoreBackupsRestoresLatestSetAfterRemoval(t *testing.T) {
	e := newEnv(t)
	target := e.installSymlink(t, "shell/zshrc", ".zshrc")

	// Simulate the pre-install backup of the user's original file. CreateSet needs
	// the file present at backup time, so write it, back it up, then let install's
	// symlink stand in as the current managed target.
	original := "original user zshrc\n"
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove managed link for backup setup: %v", err)
	}
	if err := os.WriteFile(target, []byte(original), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	if _, err := backups.CreateSet(e.stateRoot, []string{target}, backups.CreateOptions{Reason: "pre-install conflict protection"}); err != nil {
		t.Fatalf("create backup set: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove original: %v", err)
	}
	src := filepath.Join(e.sourceRoot, "shell/zshrc")
	if err := os.Symlink(src, target); err != nil {
		t.Fatalf("reinstall symlink: %v", err)
	}
	e.saveMeta(t)

	e.apply(t, uninstall.Options{RestoreBackups: true})

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(got) != original {
		t.Fatalf("restored content = %q, want %q", got, original)
	}
}

func TestApplyRefusesTargetOutsideHome(t *testing.T) {
	e := newEnv(t)
	outside := t.TempDir()
	src := e.writeSource(t, "evil", "evil\n")
	escaped := filepath.Join(outside, ".evilrc")
	if err := os.Symlink(src, escaped); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	e.meta.Entries = append(e.meta.Entries, state.Record{Target: escaped, Source: "evil", Strategy: "symlink"})
	e.saveMeta(t)

	_, err := uninstall.Apply(e.buildPlan(t), uninstall.Options{
		SourceRoot: e.sourceRoot,
		Home:       e.home,
		StateRoot:  e.stateRoot,
	})
	if err == nil {
		t.Fatal("Apply() = nil, want error for target outside home")
	}
	if _, statErr := os.Lstat(escaped); statErr != nil {
		t.Fatalf("escaped target should be untouched, err = %v", statErr)
	}
}
