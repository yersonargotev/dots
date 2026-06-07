package install_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/plan"
)

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

func TestApplyRejectsConflictWithoutMutatingAnyAction(t *testing.T) {
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
	conflictingTarget := filepath.Join(home, ".gitconfig")
	p := plan.Plan{Profile: "default", Actions: []plan.Action{
		{Source: "configs/zsh/zshrc", Target: wouldCreate, Strategy: "symlink", Status: plan.StatusCreate},
		{Source: "configs/git/gitconfig", Target: conflictingTarget, Strategy: "symlink", Status: plan.StatusConflict},
	}}

	if err := install.Apply(p, install.Options{SourceRoot: sourceRoot, Home: home}); err == nil {
		t.Fatal("Apply() error = nil, want conflict error")
	}
	if _, err := os.Lstat(wouldCreate); !os.IsNotExist(err) {
		t.Fatalf("would-create target exists after rejected install; lstat err = %v", err)
	}
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
