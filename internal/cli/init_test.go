package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/bootstrap"
	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/testrepo"
	"github.com/yersonargotev/dots/internal/version"
)

func TestInitCommandClonesInstalledRepository(t *testing.T) {
	sourceRepo := newCLISourceRepo(t)
	home := t.TempDir()

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"init",
		"--home", home,
		"--repository-url", sourceRepo,
		"--repository-ref", "v0.99.0",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	sourceRoot := filepath.Join(home, ".local", "share", "dots")
	if _, err := os.Stat(filepath.Join(sourceRoot, "dots.yaml")); err != nil {
		t.Fatalf("expected cloned Installed Repository manifest: %v", err)
	}
	if !strings.Contains(out.String(), "Initialized Installed Repository at "+sourceRoot) {
		t.Fatalf("init output should report source root, got:\n%s", out.String())
	}
}

func TestDefaultManifestErrorSuggestsInit(t *testing.T) {
	home := t.TempDir()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"status", "--home", home}, &out, &errOut)

	if code != 1 {
		t.Fatalf("status without Installed Repository exit code = %d, want 1", code)
	}
	for _, want := range []string{
		"Installed Repository not found or missing dots.yaml",
		"Run `dots init`",
		"retry `dots status`",
	} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("missing %q in error:\n%s", want, errOut.String())
		}
	}
}

func newCLISourceRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for CLI init tests")
	}
	repo := t.TempDir()
	runCLIGit(t, repo, "init", "-b", "main")
	runCLIGit(t, repo, "config", "user.email", "dots@test.local")
	runCLIGit(t, repo, "config", "user.name", "dots test")
	writeCLISourceFile(t, filepath.Join(repo, "dots.yaml"), `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)
	writeCLISourceFile(t, filepath.Join(repo, "configs", "zsh", "zshrc"), "export A=1\n")
	runCLIGit(t, repo, "add", ".")
	runCLIGit(t, repo, "commit", "-m", "initial")
	runCLIGit(t, repo, "tag", "v0.99.0")
	return repo
}

func runCLIGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func writeCLISourceFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInstallDryRunPreviewsStaleDefaultInstalledRepository(t *testing.T) {
	sourceRepo := newCLISourceRepo(t)
	if err := testrepo.TagWithHerdrManifest(sourceRepo, "v0.99.1"); err != nil {
		t.Fatalf("tag Source of Truth with Herdr manifest: %v", err)
	}

	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".local", "share", "dots")
	if _, err := bootstrap.Ensure(bootstrap.Options{
		SourceRoot:    sourceRoot,
		RepositoryURL: sourceRepo,
		RepositoryRef: "v0.99.0",
	}); err != nil {
		t.Fatalf("seed stale Installed Repository: %v", err)
	}

	oldVersion := version.Value
	version.Value = "v0.99.1"
	defer func() { version.Value = oldVersion }()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"install", "--dry-run", "--profile", "default", "--home", home}, &out, &errOut)
	if code != 0 {
		t.Fatalf("install --dry-run with stale source exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "Installed Repository can fast-forward") || !strings.Contains(out.String(), "configs/herdr/config.toml") {
		t.Fatalf("dry-run should preview the incoming ref and plan, got:\n%s", out.String())
	}
	manifest, err := os.ReadFile(filepath.Join(sourceRoot, "dots.yaml"))
	if err != nil {
		t.Fatalf("read stale manifest: %v", err)
	}
	if strings.Contains(string(manifest), "configs/herdr/config.toml") {
		t.Fatalf("dry-run must not update the Installed Repository, got:\n%s", manifest)
	}
}

func TestInstallPreservesDirtyDefaultInstalledRepositoryDuringRefRefresh(t *testing.T) {
	sourceRepo := newCLISourceRepo(t)
	if err := testrepo.TagWithHerdrManifest(sourceRepo, "v0.99.1"); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	sourceRoot := filepath.Join(home, ".local", "share", "dots")
	if _, err := bootstrap.Ensure(bootstrap.Options{SourceRoot: sourceRoot, RepositoryURL: sourceRepo, RepositoryRef: "v0.99.0"}); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(sourceRoot, "local.txt")
	if err := os.WriteFile(local, []byte("preserve me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldVersion := version.Value
	version.Value = "v0.99.1"
	defer func() { version.Value = oldVersion }()

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"install", "--yes", "--skip-deps", "--profile", "default", "--home", home}, &out, &errOut)
	if code != 0 {
		t.Fatalf("install exit = %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "Preserved local Installed Repository changes in stash@{0}") {
		t.Fatalf("install did not report preserved changes:\n%s", out.String())
	}
	stashes := runCLIGitOutput(t, sourceRoot, "stash", "list")
	if !strings.Contains(stashes, "dots preserved local Installed Repository changes") {
		t.Fatalf("install stash missing:\n%s", stashes)
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Fatalf("dirty file remained in refreshed checkout: %v", err)
	}
}

func runCLIGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
