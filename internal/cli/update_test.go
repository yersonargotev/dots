package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/cli"
)

func TestUpdateFastForwardsRepoThenInstalls(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})

	// The upstream gains a brand-new managed file and a manifest entry for it.
	advanceUpstream(t, origin, "add tmux config", map[string]string{
		"configs/tmux/tmux.conf": "set -g mouse on\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
`,
	})

	out := runUpdate(t, "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)

	// The working tree advanced: the pulled source now exists.
	pulled := filepath.Join(sourceRoot, "configs/tmux/tmux.conf")
	if _, err := os.Stat(pulled); err != nil {
		t.Fatalf("update did not fast-forward the repository: %v", err)
	}
	// The newly managed target is installed.
	target := filepath.Join(home, ".tmux.conf")
	dest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("update did not install the new managed target: %v", err)
	}
	if dest != pulled {
		t.Fatalf("symlink target = %q, want %q", dest, pulled)
	}
	for _, want := range []string{"add tmux config", `Plan for profile "default" (tags: core)`} {
		if !strings.Contains(out, want) {
			t.Fatalf("update output missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestUpdateDryRunReportsIncomingWithoutModifying(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})
	advanceUpstream(t, origin, "add tmux config", map[string]string{
		"configs/tmux/tmux.conf": "set -g mouse on\n",
	})

	out := runUpdate(t, "--dry-run", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot)

	// Dry run must not advance the working tree...
	if _, err := os.Stat(filepath.Join(sourceRoot, "configs/tmux/tmux.conf")); !os.IsNotExist(err) {
		t.Fatalf("dry-run modified the working tree; stat err = %v", err)
	}
	// ...and must not install anything.
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("dry-run installed a target; lstat err = %v", err)
	}
	for _, want := range []string{"add tmux config", `Plan for profile "default" (tags: core)`} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestUpdateDefaultManifestLoadsFromSourceRoot(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})

	t.Chdir(t.TempDir())

	out := runUpdate(t, "--dry-run", "--home", home, "--source-root", sourceRoot)

	if !strings.Contains(out, `Plan for profile "default" (tags: core)`) {
		t.Fatalf("update output missing default manifest plan\noutput:\n%s", out)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("dry-run installed a target; lstat err = %v", err)
	}
}

func TestUpdatePreservesDirtyRepoThenFastForwards(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})
	advanceUpstream(t, origin, "update zsh config", map[string]string{
		"configs/zsh/zshrc": "export A=2\n",
	})

	// A local edit makes the Installed Repository dirty.
	localPath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.WriteFile(localPath, []byte("local edit\n"), 0o600); err != nil {
		t.Fatalf("write local edit: %v", err)
	}

	out := runUpdate(t, "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot)

	if !strings.Contains(out, "Preserved local Installed Repository changes in stash@{0}") {
		t.Fatalf("update output missing preserved-changes notice\noutput:\n%s", out)
	}
	got, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read updated source: %v", err)
	}
	if string(got) != "export A=2\n" {
		t.Fatalf("source after update = %q, want upstream content", got)
	}
	stashes := runGitOutput(t, sourceRoot, "stash", "list")
	if !strings.Contains(stashes, "dots update preserved local Installed Repository changes") {
		t.Fatalf("local edit was not preserved in stash\nstash list:\n%s", stashes)
	}
}

func TestUpdateDirtyDivergedRepoDoesNotStash(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})
	advanceUpstream(t, origin, "update zsh config", map[string]string{
		"configs/zsh/zshrc": "export A=2\n",
	})

	writeRepoFiles(t, sourceRoot, map[string]string{
		"local-only.txt": "local commit\n",
	})
	runGit(t, sourceRoot, "add", "-A")
	runGit(t, sourceRoot, "commit", "-m", "local commit")

	localPath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.WriteFile(localPath, []byte("dirty local edit\n"), 0o600); err != nil {
		t.Fatalf("write dirty local edit: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--profile", "default", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("update Execute() error = nil, want divergence error\noutput:\n%s", out.String())
	}

	got, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read dirty local edit: %v", err)
	}
	if string(got) != "dirty local edit\n" {
		t.Fatalf("dirty local edit was mutated: got %q", got)
	}
	status := runGitOutput(t, sourceRoot, "status", "--short")
	if !strings.Contains(status, "M configs/zsh/zshrc") {
		t.Fatalf("dirty work tree was not preserved\nstatus:\n%s", status)
	}
	stashes := runGitOutput(t, sourceRoot, "stash", "list")
	if strings.Contains(stashes, "dots update preserved local Installed Repository changes") {
		t.Fatalf("diverged update stashed before proving fast-forwardability\nstash list:\n%s", stashes)
	}
}

func TestUpdateFailsWhenSourceRootNotGitRepo(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "export A=1\n")
	manifestPath := writeCLIManifest(t, sourceRoot, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("update Execute() error = nil, want not-a-git-repository error")
	}
}

func TestUpdateReplaceConflictCreatesBackupSet(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/git/gitconfig
    target: ~/.gitconfig
    strategy: copy
    tags: [core]
`,
	})
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("r\n"))
	cmd.SetArgs([]string{"update", "--profile", "default", "--no-tui", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "managed\n" {
		t.Fatalf("target contents = %q, want managed source after replace", got)
	}
	meta, err := backups.Load(backups.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	if len(meta.Sets) != 1 {
		t.Fatalf("Backup Sets = %d, want 1 after post-update replace", len(meta.Sets))
	}
}

// --- helpers ---

func requireGitCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
}

func runUpdate(t *testing.T, args ...string) string {
	t.Helper()
	hasSelection := false
	for _, arg := range args {
		if arg == "--profile" || arg == "-p" || arg == "--tag" {
			hasSelection = true
			break
		}
	}
	if !hasSelection {
		args = append([]string{"--profile", "default"}, args...)
	}
	return runBareUpdate(t, args...)
}

func runBareUpdate(t *testing.T, args ...string) string {
	t.Helper()
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"update"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v\noutput:\n%s", err, out.String())
	}
	return out.String()
}

// newInstalledRepo writes files into a non-bare origin repository, commits them,
// and clones it into a working directory that becomes the Installed Repository.
func newInstalledRepo(t *testing.T, files map[string]string) (origin, clone string) {
	t.Helper()
	origin = t.TempDir()
	runGit(t, origin, "init", "-b", "main")
	gitIdentity(t, origin)
	writeRepoFiles(t, origin, files)
	runGit(t, origin, "add", "-A")
	runGit(t, origin, "commit", "-m", "initial")

	clone = t.TempDir()
	runGit(t, "", "clone", origin, clone)
	gitIdentity(t, clone)
	return origin, clone
}

func advanceUpstream(t *testing.T, origin, message string, files map[string]string) {
	t.Helper()
	writeRepoFiles(t, origin, files)
	runGit(t, origin, "add", "-A")
	runGit(t, origin, "commit", "-m", message)
}

func writeRepoFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	// Deterministic order keeps commits reproducible across runs.
	paths := make([]string, 0, len(files))
	for rel := range files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(files[rel]), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

func gitIdentity(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "config", "user.email", "dots@test.local")
	runGit(t, dir, "config", "user.name", "dots test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = runGitOutput(t, dir, args...)
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
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
	} else {
		return string(out)
	}
	return ""
}

func TestUpdateRequiresSelectionBeforeFastForward(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})
	oldHead := strings.TrimSpace(runGitOutput(t, sourceRoot, "rev-parse", "HEAD"))
	advanceUpstream(t, origin, "add tmux config", map[string]string{
		"configs/tmux/tmux.conf": "set -g mouse on\n",
	})

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"), "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "selection required") {
		t.Fatalf("update error = %v, want selection remediation\noutput:\n%s", err, out.String())
	}
	if got := strings.TrimSpace(runGitOutput(t, sourceRoot, "rev-parse", "HEAD")); got != oldHead {
		t.Fatalf("update fast-forwarded before profile validation: HEAD = %s, want %s", got, oldHead)
	}
	if _, statErr := os.Stat(filepath.Join(sourceRoot, "configs/tmux/tmux.conf")); !os.IsNotExist(statErr) {
		t.Fatalf("update created incoming file before profile validation; stat err = %v", statErr)
	}
}
