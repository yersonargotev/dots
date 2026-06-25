package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
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
