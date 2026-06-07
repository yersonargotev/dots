package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestRootHelpIdentifiesDotsCLI(t *testing.T) {
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "dots") || !strings.Contains(got, "Dotfiles CLI") {
		t.Fatalf("help output = %q, want command name and description", got)
	}
}

func TestPlanCommandRendersPreviewForResolvedEnvironment(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", home)

	// The managed source must exist under the resolved Installed Repository so
	// the plan reports a real create against a clean home.
	srcPath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	manifestPath := filepath.Join(home, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"plan", "--file", manifestPath, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	wantTarget := filepath.Join(home, ".zshrc")
	for _, want := range []string{
		`Plan for profile "default"`,
		"create",
		"symlink",
		"configs/zsh/zshrc -> " + wantTarget,
		"Summary: 1 create, 0 conflict, 0 unchanged, 0 missing-source",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plan output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestPlanCommandHomeFlagOverridesRealHome(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	// Point the real HOME elsewhere to prove --home wins and the user's actual
	// configuration is never read.
	t.Setenv("HOME", t.TempDir())

	srcPath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	manifestPath := filepath.Join(sandboxHome, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"plan", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	wantTarget := filepath.Join(sandboxHome, ".zshrc")
	if !strings.Contains(got, "configs/zsh/zshrc -> "+wantTarget) {
		t.Fatalf("plan did not resolve target under --home\nwant target %q\noutput:\n%s", wantTarget, got)
	}
	if !strings.Contains(got, "Summary: 1 create, 0 conflict, 0 unchanged, 0 missing-source") {
		t.Fatalf("plan output missing expected create summary against sandbox home\noutput:\n%s", got)
	}
}

func TestPlanCommandDefaultsSourceRootUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No --source-root and no installed repository on disk: the default
	// (~/.local/share/dots) does not exist, so the managed source is absent.
	manifestPath := filepath.Join(home, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"plan", "--file", manifestPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := out.String(); !strings.Contains(got, "Summary: 0 create, 0 conflict, 0 unchanged, 1 missing-source") {
		t.Fatalf("plan output missing expected missing-source summary\noutput:\n%s", got)
	}
}

func TestPlanCommandFailsOnUnknownProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	manifestPath := filepath.Join(home, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"plan", "--file", manifestPath, "--profile", "missing"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want error for unknown profile")
	}
}

func TestManifestValidateCommandAcceptsValidManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"manifest", "validate", "--file", manifestPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "manifest is valid" {
		t.Fatalf("output = %q, want %q", got, "manifest is valid")
	}
}

func TestInstallCommandAppliesPlanAgainstSandboxHome(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	srcPath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	manifestPath := filepath.Join(home, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	target := filepath.Join(home, ".zshrc")
	gotDest, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("readlink installed target: %v", err)
	}
	if gotDest != srcPath {
		t.Fatalf("symlink target = %q, want %q", gotDest, srcPath)
	}
	if !strings.Contains(out.String(), `Plan for profile "default"`) {
		t.Fatalf("install output did not include plan\noutput:\n%s", out.String())
	}
}

func TestInstallDryRunRendersPlanWithoutCreatingTarget(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	srcPath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	manifestPath := filepath.Join(home, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--dry-run", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	target := filepath.Join(home, ".zshrc")
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("dry-run created target; lstat err = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`Plan for profile "default"`,
		"create",
		"Summary: 1 create, 0 conflict, 0 unchanged, 0 missing-source",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestInstallCommandRejectsAbsoluteTargetWithoutCreatingOutsidePath(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "export A=1\n")

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: `+outside+`
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want unsafe target error")
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside target exists after rejected install; lstat err = %v", err)
	}
}

func TestInstallCommandRejectsTargetTraversalWithoutCreatingOutsidePath(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	outside := filepath.Join(filepath.Dir(home), "outside")
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "export A=1\n")

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/../outside
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want unsafe target error")
	}
	if _, err := os.Lstat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside target exists after rejected install; lstat err = %v", err)
	}
}

func TestInstallCommandRejectsSourceTraversalWithoutCopyingOutsideFile(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	outsideSource := filepath.Join(filepath.Dir(sourceRoot), "outside")
	if err := os.WriteFile(outsideSource, []byte("outside\n"), 0o600); err != nil {
		t.Fatalf("write outside source: %v", err)
	}
	target := filepath.Join(home, ".zshrc")

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: ../outside
    target: ~/.zshrc
    strategy: copy
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want unsafe source error")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Fatalf("target exists after rejected install; lstat err = %v", err)
	}
}

func TestInstallCommandRejectsUnsafeActionBeforeEarlierSafeCreate(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "export A=1\n")

	safeTarget := filepath.Join(home, ".zshrc")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
  - source: configs/zsh/zshrc
    target: ~/../outside
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want unsafe target error")
	}
	if _, err := os.Lstat(safeTarget); !os.IsNotExist(err) {
		t.Fatalf("safe target exists after rejected install; lstat err = %v", err)
	}
}

func TestStatusCommandReportsOKAfterInstall(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "export A=1\n")

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{"install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := install.Execute(); err != nil {
		t.Fatalf("install Execute() error = %v\noutput:\n%s", err, installOut.String())
	}

	statusCmd := cli.NewRootCommand()
	var out bytes.Buffer
	statusCmd.SetOut(&out)
	statusCmd.SetErr(&out)
	statusCmd.SetArgs([]string{"status", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("status Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`Status for profile "default"`,
		"ok",
		"configs/zsh/zshrc -> " + filepath.Join(home, ".zshrc"),
		"Summary: 1 ok, 0 missing, 0 conflict, 0 skipped, 0 drifted, 0 unsupported",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestStatusCommandReportsConflictForForeignTarget(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "[user]\n\tname = Source\n")

	// A pre-existing foreign file dots never installed: no metadata, content
	// differs from the Source of Truth, so status must report a conflict.
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("foreign\n"), 0o600); err != nil {
		t.Fatalf("write foreign target: %v", err)
	}

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/git/gitconfig
    target: ~/.gitconfig
    strategy: copy
    tags: [core]
`)

	statusCmd := cli.NewRootCommand()
	var out bytes.Buffer
	statusCmd.SetOut(&out)
	statusCmd.SetErr(&out)
	statusCmd.SetArgs([]string{"status", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("status Execute() error = %v", err)
	}

	if got := out.String(); !strings.Contains(got, "Summary: 0 ok, 0 missing, 1 conflict, 0 skipped, 0 drifted, 0 unsupported") {
		t.Fatalf("status output missing expected conflict summary\noutput:\n%s", got)
	}
}

func TestStatusCommandRejectsDefaultStateRootSymlinkEscapeBeforeReadingMetadata(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	outsideState := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "[user]\n")

	stateParent := filepath.Join(home, ".local", "state")
	if err := os.MkdirAll(stateParent, 0o755); err != nil {
		t.Fatalf("mkdir state parent: %v", err)
	}
	if err := os.Symlink(outsideState, filepath.Join(stateParent, "dots")); err != nil {
		t.Fatalf("symlink default state root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideState, "installed.json"), []byte(`{"version":1,"entries":[]}`), 0o600); err != nil {
		t.Fatalf("write outside metadata: %v", err)
	}

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/git/gitconfig
    target: ~/.gitconfig
    strategy: copy
    tags: [core]
`)

	statusCmd := cli.NewRootCommand()
	var out bytes.Buffer
	statusCmd.SetOut(&out)
	statusCmd.SetErr(&out)
	statusCmd.SetArgs([]string{"status", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})
	if err := statusCmd.Execute(); err == nil {
		t.Fatal("status Execute() error = nil, want default state-root symlink escape error")
	}
}

func TestStatusCommandRejectsDefaultMetadataLeafSymlinkEscapeBeforeReadingMetadata(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	outsideState := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "[user]\n")

	stateRoot := filepath.Join(home, ".local", "state", "dots")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		t.Fatalf("mkdir state root: %v", err)
	}
	outsideMetadata := filepath.Join(outsideState, "installed.json")
	if err := os.WriteFile(outsideMetadata, []byte(`{"version":1,"entries":[]}`), 0o600); err != nil {
		t.Fatalf("write outside metadata: %v", err)
	}
	if err := os.Symlink(outsideMetadata, filepath.Join(stateRoot, "installed.json")); err != nil {
		t.Fatalf("symlink metadata leaf: %v", err)
	}

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/git/gitconfig
    target: ~/.gitconfig
    strategy: copy
    tags: [core]
`)

	statusCmd := cli.NewRootCommand()
	var out bytes.Buffer
	statusCmd.SetOut(&out)
	statusCmd.SetErr(&out)
	statusCmd.SetArgs([]string{"status", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})
	if err := statusCmd.Execute(); err == nil {
		t.Fatal("status Execute() error = nil, want metadata leaf symlink escape error")
	}
}

func TestStatusCommandFailsOnUnknownProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)

	statusCmd := cli.NewRootCommand()
	var out bytes.Buffer
	statusCmd.SetOut(&out)
	statusCmd.SetErr(&out)
	statusCmd.SetArgs([]string{"status", "--file", manifestPath, "--profile", "missing"})
	if err := statusCmd.Execute(); err == nil {
		t.Fatal("status Execute() error = nil, want error for unknown profile")
	}
}

func writeCLISource(t *testing.T, sourceRoot, rel, content string) {
	t.Helper()
	srcPath := filepath.Join(sourceRoot, rel)
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
}

func writeCLIManifest(t *testing.T, dir, content string) string {
	t.Helper()
	manifestPath := filepath.Join(dir, "dots.yaml")
	if err := os.WriteFile(manifestPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath
}
