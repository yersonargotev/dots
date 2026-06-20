package plan_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

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
	p, err := plan.BuildUninstall(state.Metadata{Version: 1, Entries: recs}, plan.UninstallOptions{SourceRoot: f.sourceRoot})
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
