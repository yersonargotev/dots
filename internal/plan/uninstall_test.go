package plan_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestBuildUninstallPlansOneRemovalForCompositeTarget(t *testing.T) {
	f := newUninstallFixture(t)
	target := f.target(".config/shared.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte("{\"base\":true,\"mobile\":true}\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	hash, err := state.HashFile(target)
	if err != nil {
		t.Fatalf("hash target: %v", err)
	}
	sources := []string{"configs/base.json", "configs/mobile.json"}
	p := f.build(t, state.Record{
		Target: target, Source: sources[0], Sources: sources, Strategy: "copy", Hash: hash,
	})
	if len(p.Actions) != 1 {
		t.Fatalf("actions = %d, want one physical removal", len(p.Actions))
	}
	if p.Actions[0].Status != plan.UninstallRemove || !reflect.DeepEqual(p.Actions[0].Sources, sources) {
		t.Fatalf("action = %+v, want removable composite contributors", p.Actions[0])
	}
}

func TestValidateBackupableTargetAcceptsFileDirAndSymlink(t *testing.T) {
	dir := t.TempDir()

	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	for _, target := range []string{file, subdir, link} {
		if err := plan.ValidateBackupableTarget(target); err != nil {
			t.Fatalf("ValidateBackupableTarget(%s) = %v, want nil", target, err)
		}
	}
}

func TestValidateBackupableTargetRejectsMissing(t *testing.T) {
	err := plan.ValidateBackupableTarget(filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("ValidateBackupableTarget() = nil, want error for missing target")
	}
}

// uninstallFixture builds a sourceRoot + home pair and returns a record for a
// managed target so each classification case can set up the on-disk state it
// needs before calling BuildUninstall.
type uninstallFixture struct {
	sourceRoot string
	home       string
}

func newUninstallFixture(t *testing.T) uninstallFixture {
	t.Helper()
	return uninstallFixture{sourceRoot: t.TempDir(), home: t.TempDir()}
}

func (f uninstallFixture) writeSource(t *testing.T, rel, content string) string {
	t.Helper()
	abs := filepath.Join(f.sourceRoot, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return abs
}

func (f uninstallFixture) target(name string) string {
	return filepath.Join(f.home, name)
}

func (f uninstallFixture) build(t *testing.T, recs ...state.Record) plan.UninstallPlan {
	t.Helper()
	p, err := plan.BuildUninstall(state.Metadata{Version: 1, Entries: recs}, plan.UninstallOptions{SourceRoot: f.sourceRoot, Home: f.home})
	if err != nil {
		t.Fatalf("BuildUninstall() error = %v", err)
	}
	return p
}

func statusFor(t *testing.T, p plan.UninstallPlan, target string) plan.UninstallStatus {
	t.Helper()
	for _, a := range p.Actions {
		if a.Target == target {
			return a.Status
		}
	}
	t.Fatalf("no action for target %s", target)
	return ""
}

func TestBuildUninstallSymlinkPointingAtSourceIsRemove(t *testing.T) {
	f := newUninstallFixture(t)
	src := f.writeSource(t, "shell/zshrc", "export A=1\n")
	target := f.target(".zshrc")
	if err := os.Symlink(src, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rec := state.Record{Target: target, Source: "shell/zshrc", Strategy: "symlink"}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallRemove {
		t.Fatalf("status = %q, want remove", got)
	}
}

func TestBuildUninstallSymlinkPointingElsewhereIsNotOwned(t *testing.T) {
	f := newUninstallFixture(t)
	f.writeSource(t, "shell/zshrc", "export A=1\n")
	other := f.writeSource(t, "other", "other\n")
	target := f.target(".zshrc")
	if err := os.Symlink(other, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rec := state.Record{Target: target, Source: "shell/zshrc", Strategy: "symlink"}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallNotOwned {
		t.Fatalf("status = %q, want not-owned", got)
	}
}

func TestBuildUninstallSymlinkMissingIsNotOwned(t *testing.T) {
	f := newUninstallFixture(t)
	f.writeSource(t, "shell/zshrc", "export A=1\n")
	target := f.target(".zshrc")

	rec := state.Record{Target: target, Source: "shell/zshrc", Strategy: "symlink"}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallNotOwned {
		t.Fatalf("status = %q, want not-owned", got)
	}
}

func TestBuildUninstallSymlinkReplacedByRegularFileIsNotOwned(t *testing.T) {
	f := newUninstallFixture(t)
	f.writeSource(t, "shell/zshrc", "export A=1\n")
	target := f.target(".zshrc")
	if err := os.WriteFile(target, []byte("manual\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	rec := state.Record{Target: target, Source: "shell/zshrc", Strategy: "symlink"}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallNotOwned {
		t.Fatalf("status = %q, want not-owned", got)
	}
}

func TestBuildUninstallCopyMatchingHashIsRemove(t *testing.T) {
	f := newUninstallFixture(t)
	content := "name = dots\n"
	src := f.writeSource(t, "git/gitconfig", content)
	hash, err := state.HashFile(src)
	if err != nil {
		t.Fatalf("hash source: %v", err)
	}
	target := f.target(".gitconfig")
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	rec := state.Record{Target: target, Source: "git/gitconfig", Strategy: "copy", Hash: hash}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallRemove {
		t.Fatalf("status = %q, want remove", got)
	}
}

func TestBuildUninstallCopyDriftedHashIsModified(t *testing.T) {
	f := newUninstallFixture(t)
	src := f.writeSource(t, "git/gitconfig", "name = dots\n")
	hash, err := state.HashFile(src)
	if err != nil {
		t.Fatalf("hash source: %v", err)
	}
	target := f.target(".gitconfig")
	if err := os.WriteFile(target, []byte("name = edited\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	rec := state.Record{Target: target, Source: "git/gitconfig", Strategy: "copy", Hash: hash}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallModified {
		t.Fatalf("status = %q, want modified", got)
	}
}

func TestBuildUninstallCopyMissingIsSkip(t *testing.T) {
	f := newUninstallFixture(t)
	f.writeSource(t, "git/gitconfig", "name = dots\n")
	target := f.target(".gitconfig")

	rec := state.Record{Target: target, Source: "git/gitconfig", Strategy: "copy", Hash: "deadbeef"}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallSkip {
		t.Fatalf("status = %q, want skip", got)
	}
}

// TestBuildUninstallTargetOutsideHomeIsNotOwnedWithoutInspection proves the
// confinement guard runs before any filesystem inspection: a recorded target
// outside HOME is classified not-owned even when, on disk, it would otherwise be
// a perfect remove (a symlink resolving to the source, or a copy whose content
// matches the recorded hash). If the builder hashed or readlink'd the target
// before validating HOME, these would classify as remove — so not-owned is the
// observable proof that no out-of-home filesystem operation happened.
func TestBuildUninstallTargetOutsideHomeIsNotOwnedWithoutInspection(t *testing.T) {
	f := newUninstallFixture(t)
	outside := t.TempDir() // a sibling root, deliberately not under f.home

	// A symlink outside HOME that points exactly at the recorded source.
	src := f.writeSource(t, "shell/zshrc", "export A=1\n")
	escapedLink := filepath.Join(outside, ".zshrc")
	if err := os.Symlink(src, escapedLink); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// A copy outside HOME whose content matches the recorded hash.
	copyContent := "name = dots\n"
	copySrc := f.writeSource(t, "git/gitconfig", copyContent)
	hash, err := state.HashFile(copySrc)
	if err != nil {
		t.Fatalf("hash source: %v", err)
	}
	escapedCopy := filepath.Join(outside, ".gitconfig")
	if err := os.WriteFile(escapedCopy, []byte(copyContent), 0o600); err != nil {
		t.Fatalf("write escaped copy: %v", err)
	}

	p := f.build(t,
		state.Record{Target: escapedLink, Source: "shell/zshrc", Strategy: "symlink"},
		state.Record{Target: escapedCopy, Source: "git/gitconfig", Strategy: "copy", Hash: hash},
	)

	if got := statusFor(t, p, escapedLink); got != plan.UninstallNotOwned {
		t.Fatalf("escaped symlink status = %q, want not-owned", got)
	}
	if got := statusFor(t, p, escapedCopy); got != plan.UninstallNotOwned {
		t.Fatalf("escaped copy status = %q, want not-owned", got)
	}
}

func TestBuildUninstallTargetBehindParentSymlinkEscapeIsNotOwned(t *testing.T) {
	f := newUninstallFixture(t)
	outside := t.TempDir()

	// A parent component inside HOME that is a symlink pointing outside HOME. A
	// lexical check alone would accept ~/escape/.zshrc; the parent symlink-escape
	// guard must reject it before Lstat follows the link out of HOME.
	if err := os.Symlink(outside, filepath.Join(f.home, "escape")); err != nil {
		t.Fatalf("parent symlink: %v", err)
	}
	src := f.writeSource(t, "shell/zshrc", "export A=1\n")
	target := filepath.Join(f.home, "escape", ".zshrc")
	if err := os.Symlink(src, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	rec := state.Record{Target: target, Source: "shell/zshrc", Strategy: "symlink"}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallNotOwned {
		t.Fatalf("status = %q, want not-owned for parent-symlink escape", got)
	}
}

func TestBuildUninstallRequiresHome(t *testing.T) {
	_, err := plan.BuildUninstall(
		state.Metadata{Entries: []state.Record{{Target: "/home/user/.zshrc", Strategy: "symlink"}}},
		plan.UninstallOptions{SourceRoot: t.TempDir()},
	)
	if err == nil {
		t.Fatal("BuildUninstall() = nil, want error when home is empty")
	}
}

func TestBuildUninstallCopyReplacedByDirectoryIsNotOwned(t *testing.T) {
	f := newUninstallFixture(t)
	f.writeSource(t, "git/gitconfig", "name = dots\n")
	target := f.target(".gitconfig")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}

	rec := state.Record{Target: target, Source: "git/gitconfig", Strategy: "copy", Hash: "deadbeef"}
	if got := statusFor(t, f.build(t, rec), target); got != plan.UninstallNotOwned {
		t.Fatalf("status = %q, want not-owned", got)
	}
}
