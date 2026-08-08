package gitrepo_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/gitrepo"
)

func TestIsRepoTrueForCheckout(t *testing.T) {
	requireGit(t)
	_, clone := setupRemoteAndClone(t)

	if !gitrepo.IsRepo(clone) {
		t.Fatalf("IsRepo(%q) = false, want true for a git checkout", clone)
	}
}

func TestIsRepoFalseForPlainDirectory(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()

	if gitrepo.IsRepo(dir) {
		t.Fatalf("IsRepo(%q) = true, want false for a non-git directory", dir)
	}
}

func TestIsCleanTrueForFreshClone(t *testing.T) {
	requireGit(t)
	_, clone := setupRemoteAndClone(t)

	clean, err := gitrepo.IsClean(clone)
	if err != nil {
		t.Fatalf("IsClean() error = %v", err)
	}
	if !clean {
		t.Fatalf("IsClean() = false, want true for a fresh clone")
	}
}

func TestIsCleanFalseWithLocalChange(t *testing.T) {
	requireGit(t)
	_, clone := setupRemoteAndClone(t)

	// A local modification to a tracked file must be detected as dirty so the
	// updater refuses to overwrite local work.
	writeFile(t, filepath.Join(clone, "configs/zsh/zshrc"), "local edit\n")

	clean, err := gitrepo.IsClean(clone)
	if err != nil {
		t.Fatalf("IsClean() error = %v", err)
	}
	if clean {
		t.Fatalf("IsClean() = true, want false after a local change")
	}
}

func TestFastForwardAdvancesToUpstream(t *testing.T) {
	requireGit(t)
	origin, clone := setupRemoteAndClone(t)

	// Advance the upstream with a new managed file so the fast-forward has
	// something to apply.
	writeFile(t, filepath.Join(origin, "configs/git/gitconfig"), "[user]\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "add gitconfig")

	upd, err := gitrepo.FastForward(clone)
	if err != nil {
		t.Fatalf("FastForward() error = %v", err)
	}
	if !upd.Changed() {
		t.Fatalf("FastForward() Changed() = false, want true after upstream advanced")
	}
	if upd.OldRev == "" || upd.NewRev == "" || upd.OldRev == upd.NewRev {
		t.Fatalf("FastForward() revs = %q -> %q, want distinct non-empty revs", upd.OldRev, upd.NewRev)
	}
	if len(upd.Incoming) != 1 {
		t.Fatalf("FastForward() Incoming = %v, want 1 incoming commit", upd.Incoming)
	}
	// The working tree must actually contain the pulled file.
	if _, err := os.Stat(filepath.Join(clone, "configs/git/gitconfig")); err != nil {
		t.Fatalf("fast-forward did not materialize upstream file: %v", err)
	}
}

func TestFastForwardUpToDateReportsNoChange(t *testing.T) {
	requireGit(t)
	_, clone := setupRemoteAndClone(t)

	upd, err := gitrepo.FastForward(clone)
	if err != nil {
		t.Fatalf("FastForward() error = %v", err)
	}
	if upd.Changed() {
		t.Fatalf("FastForward() Changed() = true, want false when already up to date")
	}
	if len(upd.Incoming) != 0 {
		t.Fatalf("FastForward() Incoming = %v, want none when up to date", upd.Incoming)
	}
}

func TestPreviewReportsIncomingWithoutModifyingWorkingTree(t *testing.T) {
	requireGit(t)
	origin, clone := setupRemoteAndClone(t)

	writeFile(t, filepath.Join(origin, "configs/git/gitconfig"), "[user]\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "add gitconfig")

	upd, err := gitrepo.Preview(clone)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if !upd.Changed() {
		t.Fatalf("Preview() Changed() = false, want true when an update is available")
	}
	if len(upd.Incoming) != 1 {
		t.Fatalf("Preview() Incoming = %v, want 1 incoming commit", upd.Incoming)
	}
	// Preview must not touch the working tree.
	if _, err := os.Stat(filepath.Join(clone, "configs/git/gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("Preview() modified the working tree; stat err = %v", err)
	}
}

func TestReadFileAtRevisionReadsIncomingFileWithoutModifyingWorkingTree(t *testing.T) {
	requireGit(t)
	origin, clone := setupRemoteAndClone(t)

	writeFile(t, filepath.Join(origin, "dots.yaml"), "version: 1\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "add manifest")
	update, err := gitrepo.Preview(clone)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	content, err := gitrepo.ReadFileAtRevision(clone, update.NewRev, "dots.yaml")
	if err != nil {
		t.Fatalf("ReadFileAtRevision() error = %v", err)
	}
	if string(content) != "version: 1\n" {
		t.Fatalf("ReadFileAtRevision() = %q, want incoming manifest", content)
	}
	snapshot := t.TempDir()
	if err := gitrepo.ExportRevision(clone, update.NewRev, snapshot); err != nil {
		t.Fatalf("ExportRevision() error = %v", err)
	}
	exported, err := os.ReadFile(filepath.Join(snapshot, "dots.yaml"))
	if err != nil || string(exported) != "version: 1\n" {
		t.Fatalf("ExportRevision() manifest = %q, err=%v", exported, err)
	}
	if _, err := os.Stat(filepath.Join(clone, "dots.yaml")); !os.IsNotExist(err) {
		t.Fatalf("ReadFileAtRevision() changed work tree; stat err = %v", err)
	}
}

func TestPreviewSupportsDetachedInstalledRepositoryAtReleaseTag(t *testing.T) {
	requireGit(t)
	origin, clone := setupRemoteAndClone(t)
	gitExec(t, origin, "tag", "v0.99.0")
	gitExec(t, clone, "fetch", "--tags")
	gitExec(t, clone, "switch", "--detach", "v0.99.0")

	writeFile(t, filepath.Join(origin, "configs/git/gitconfig"), "[user]\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "add gitconfig")

	upd, err := gitrepo.Preview(clone)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if !upd.Changed() {
		t.Fatalf("Preview() Changed() = false, want true from detached release tag")
	}
	if upd.AttachedBranch != "main" {
		t.Fatalf("Preview() AttachedBranch = %q, want main", upd.AttachedBranch)
	}
	if got := gitOutput(t, clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "HEAD" {
		t.Fatalf("Preview() attached working tree branch %q, want still detached", got)
	}
}

func TestFastForwardPreservingLocalChangesAttachesDetachedInstalledRepository(t *testing.T) {
	requireGit(t)
	origin, clone := setupRemoteAndClone(t)
	gitExec(t, origin, "tag", "v0.99.0")
	gitExec(t, clone, "fetch", "--tags")
	gitExec(t, clone, "switch", "--detach", "v0.99.0")

	writeFile(t, filepath.Join(clone, "configs/nvim/lazy-lock.json"), "local editor update\n")

	writeFile(t, filepath.Join(origin, "configs/git/gitconfig"), "[user]\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "add gitconfig")

	upd, err := gitrepo.FastForwardPreservingLocalChanges(clone)
	if err != nil {
		t.Fatalf("FastForwardPreservingLocalChanges() error = %v", err)
	}
	if !upd.Changed() {
		t.Fatalf("FastForwardPreservingLocalChanges() Changed() = false, want true")
	}
	if upd.AttachedBranch != "main" {
		t.Fatalf("FastForwardPreservingLocalChanges() AttachedBranch = %q, want main", upd.AttachedBranch)
	}
	if upd.PreservedChanges == "" {
		t.Fatal("FastForwardPreservingLocalChanges() did not preserve dirty Installed Repository changes")
	}
	if got := gitOutput(t, clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Fatalf("HEAD branch = %q, want main", got)
	}
	if _, err := os.Stat(filepath.Join(clone, "configs/git/gitconfig")); err != nil {
		t.Fatalf("fast-forward did not materialize upstream file: %v", err)
	}
	if status := gitOutput(t, clone, "status", "--porcelain"); status != "" {
		t.Fatalf("working tree status = %q, want clean after preserving changes", status)
	}
}

func TestFastForwardPreservingLocalChangesDoesNotResetDivergedLocalBranch(t *testing.T) {
	requireGit(t)
	origin, clone := setupRemoteAndClone(t)
	gitExec(t, origin, "tag", "v0.99.0")
	gitExec(t, clone, "fetch", "--tags")

	writeFile(t, filepath.Join(clone, "configs/zsh/zshrc"), "local branch change\n")
	gitExec(t, clone, "add", "-A")
	gitExec(t, clone, "commit", "-m", "local branch change")
	localMain := gitOutput(t, clone, "rev-parse", "--short", "main")

	gitExec(t, clone, "switch", "--detach", "v0.99.0")
	writeFile(t, filepath.Join(clone, "configs/nvim/lazy-lock.json"), "local editor update\n")

	writeFile(t, filepath.Join(origin, "configs/zsh/zshrc"), "upstream branch change\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "upstream branch change")

	_, err := gitrepo.FastForwardPreservingLocalChanges(clone)
	if !errors.Is(err, gitrepo.ErrNotFastForward) {
		t.Fatalf("FastForwardPreservingLocalChanges() error = %v, want ErrNotFastForward", err)
	}
	if got := gitOutput(t, clone, "rev-parse", "--short", "main"); got != localMain {
		t.Fatalf("local main was reset: got %s, want %s", got, localMain)
	}
	if got := gitOutput(t, clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "HEAD" {
		t.Fatalf("HEAD branch = %q, want still detached after failed recovery", got)
	}
	if status := gitOutput(t, clone, "status", "--porcelain"); status == "" {
		t.Fatal("local dirty editor change was stashed despite failed recovery")
	}
}

func TestFastForwardSupportsShallowTagCloneWithoutRemoteBranches(t *testing.T) {
	requireGit(t)
	origin := setupOriginWithReleaseTag(t, "v0.99.0")
	clone := filepath.Join(t.TempDir(), "clone")
	gitExec(t, "", "clone", "--depth", "1", "--branch", "v0.99.0", "file://"+origin, clone)
	configIdentity(t, clone)

	writeFile(t, filepath.Join(origin, "configs/git/gitconfig"), "[user]\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "add gitconfig")

	upd, err := gitrepo.FastForwardPreservingLocalChanges(clone)
	if err != nil {
		t.Fatalf("FastForwardPreservingLocalChanges() error = %v", err)
	}
	if !upd.Changed() {
		t.Fatalf("FastForwardPreservingLocalChanges() Changed() = false, want true")
	}
	if got := gitOutput(t, clone, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Fatalf("HEAD branch = %q, want main", got)
	}
	if _, err := os.Stat(filepath.Join(clone, "configs/git/gitconfig")); err != nil {
		t.Fatalf("fast-forward did not materialize upstream file: %v", err)
	}
}

func TestFastForwardReturnsErrNotFastForwardOnDivergence(t *testing.T) {
	requireGit(t)
	origin, clone := setupRemoteAndClone(t)

	// Diverge: a local commit and an upstream commit that touch the same file.
	writeFile(t, filepath.Join(clone, "configs/zsh/zshrc"), "local change\n")
	gitExec(t, clone, "add", "-A")
	gitExec(t, clone, "commit", "-m", "local change")

	writeFile(t, filepath.Join(origin, "configs/zsh/zshrc"), "upstream change\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "upstream change")

	_, err := gitrepo.FastForward(clone)
	if !errors.Is(err, gitrepo.ErrNotFastForward) {
		t.Fatalf("FastForward() error = %v, want ErrNotFastForward on divergence", err)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(bytes.TrimSpace(out))
}

// --- helpers ---

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
}

// setupRemoteAndClone creates a non-bare origin repository with an initial
// managed file and clones it into a separate working directory. The clone's
// upstream tracks origin/main, so fetching the advanced origin exercises the
// fast-forward path without any network access.
func setupRemoteAndClone(t *testing.T) (origin, clone string) {
	t.Helper()
	origin = setupOriginWithReleaseTag(t, "")

	clone = t.TempDir()
	gitExec(t, "", "clone", origin, clone)
	configIdentity(t, clone)
	return origin, clone
}

func setupOriginWithReleaseTag(t *testing.T, tag string) string {
	t.Helper()
	origin := t.TempDir()
	gitExec(t, origin, "init", "-b", "main")
	configIdentity(t, origin)
	writeFile(t, filepath.Join(origin, "configs/zsh/zshrc"), "export A=1\n")
	gitExec(t, origin, "add", "-A")
	gitExec(t, origin, "commit", "-m", "initial")
	if tag != "" {
		gitExec(t, origin, "tag", tag)
	}
	return origin
}

func configIdentity(t *testing.T, dir string) {
	t.Helper()
	gitExec(t, dir, "config", "user.email", "dots@test.local")
	gitExec(t, dir, "config", "user.name", "dots test")
	gitExec(t, dir, "config", "commit.gpgsign", "false")
}

func gitExec(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
