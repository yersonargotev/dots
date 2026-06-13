package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
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

func TestDoctorCommandRendersConsolidatedDiagnostics(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", home)

	srcPath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.Symlink(srcPath, filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("symlink target: %v", err)
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
	cmd.SetArgs([]string{"doctor", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`Doctor for profile "default"`,
		"Platform: ok",
		"Dependencies: ok (no dependencies declared)",
		"Configuration: ok (1 ok, 0 concerns)",
		"Secret Scan: ok (0 findings)",
		"Guardrail: Secret Scan catches obvious credential and private-key patterns only; it is not proof this repository is safe.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestDoctorCommandRendersWarningDiagnostics(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", home)

	srcPath := filepath.Join(sourceRoot, "configs/git/config")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("[credential]\napi_key = live-secret-value\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	manifestPath := filepath.Join(home, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/git/config
    target: ~/.gitconfig
    strategy: copy
    tags: [core]
    os: [darwin, linux]
    dependencies:
      - name: definitely-missing-dots-test-tool
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{
		`Doctor for profile "default"`,
		"Dependencies: warn (1 missing)",
		"missing dependency: definitely-missing-dots-test-tool",
		"Configuration: warn (1 concerns)",
		"missing: configs/git/config -> " + filepath.Join(home, ".gitconfig"),
		"Secret Scan: warn (1 findings)",
		"configs/git/config:2 credential-assignment",
		"Guardrail: Secret Scan catches obvious credential and private-key patterns only; it is not proof this repository is safe.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor warning output missing %q\noutput:\n%s", want, got)
		}
	}
	if strings.Contains(got, "live-secret-value") {
		t.Fatalf("doctor warning output leaked secret value\noutput:\n%s", got)
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

func TestInstallNoTUIPromptCanReplaceConflict(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "managed\n")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
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

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("r\n"))
	cmd.SetArgs([]string{"install", "--no-tui", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	target := filepath.Join(home, ".gitconfig")
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
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
	if !strings.Contains(out.String(), "Resolve conflict") {
		t.Fatalf("install output missing conflict prompt\noutput:\n%s", out.String())
	}
}

func TestInstallNoTUIPromptCanShowDiffThenSkipConflict(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "managed\n")
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
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

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("d\ns\n"))
	cmd.SetArgs([]string{"install", "--no-tui", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "local\n" {
		t.Fatalf("target contents = %q, want skipped local content", got)
	}
	for _, want := range []string{"--- target: " + target, "local\n", "--- source: configs/git/gitconfig", "managed\n"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("install output missing diff text %q\noutput:\n%s", want, out.String())
		}
	}
}

func TestInstallNoTUIPromptDiffDoesNotLeakTargetThroughParentSymlinkEscape(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	outsideHome := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/nvim/init.lua", "managed\n")

	if err := os.MkdirAll(filepath.Join(outsideHome, "nvim"), 0o755); err != nil {
		t.Fatalf("mkdir outside nvim: %v", err)
	}
	outsideSecret := "outside-home-secret\n"
	if err := os.WriteFile(filepath.Join(outsideHome, "nvim", "init.lua"), []byte(outsideSecret), 0o600); err != nil {
		t.Fatalf("write outside target: %v", err)
	}
	if err := os.Symlink(outsideHome, filepath.Join(home, ".config")); err != nil {
		t.Fatalf("symlink parent escape: %v", err)
	}

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim/init.lua
    target: ~/.config/nvim/init.lua
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("d\ns\n"))
	cmd.SetArgs([]string{"install", "--no-tui", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if strings.Contains(out.String(), outsideSecret) {
		t.Fatalf("prompt diff leaked outside-home target contents\noutput:\n%s", out.String())
	}
}

func TestInstallNoTUIPromptCanAdoptConflict(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	sourcePath := filepath.Join(sourceRoot, "configs/git/gitconfig")
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "managed\n")
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
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

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("a\n"))
	cmd.SetArgs([]string{"install", "--no-tui", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if string(got) != "local\n" {
		t.Fatalf("source contents = %q, want adopted local content", got)
	}
}

func TestInstallYesDefaultsConflictToSkipWithoutPrompting(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "managed\n")
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
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

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("r\n"))
	cmd.SetArgs([]string{"install", "--yes", "--no-tui", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "local\n" {
		t.Fatalf("target contents = %q, want confirmed install to skip conflict", got)
	}
	if strings.Contains(out.String(), "Resolve conflict") {
		t.Fatalf("confirmed install prompted for conflict\noutput:\n%s", out.String())
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

func TestRepositoryGitConfigInstallsAndReportsAlignedInSandbox(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	sourceRoot, err := filepath.Abs(filepath.Clean(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--yes",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	if err := install.Execute(); err != nil {
		t.Fatalf("install Execute() error = %v\noutput:\n%s", err, installOut.String())
	}

	gitconfigTarget := filepath.Join(home, ".gitconfig")
	if target, err := os.Readlink(gitconfigTarget); err != nil {
		t.Fatalf("sandbox gitconfig target is not a symlink: %v", err)
	} else if want := filepath.Join(sourceRoot, "configs/git/gitconfig"); target != want {
		t.Fatalf("sandbox gitconfig symlink = %q, want %q", target, want)
	}
	zellijConfigTarget := filepath.Join(home, ".config/zellij/config.kdl")
	if target, err := os.Readlink(zellijConfigTarget); err != nil {
		t.Fatalf("sandbox Zellij config target is not a symlink: %v", err)
	} else if want := filepath.Join(sourceRoot, "configs/zellij/config.kdl"); target != want {
		t.Fatalf("sandbox Zellij config symlink = %q, want %q", target, want)
	}
	zellijLayoutTarget := filepath.Join(home, ".config/zellij/layouts/default.kdl")
	if target, err := os.Readlink(zellijLayoutTarget); err != nil {
		t.Fatalf("sandbox Zellij default layout target is not a symlink: %v", err)
	} else if want := filepath.Join(sourceRoot, "configs/zellij/layouts/default.kdl"); target != want {
		t.Fatalf("sandbox Zellij default layout symlink = %q, want %q", target, want)
	}

	statusCmd := cli.NewRootCommand()
	var statusOut bytes.Buffer
	statusCmd.SetOut(&statusOut)
	statusCmd.SetErr(&statusOut)
	statusCmd.SetArgs([]string{
		"status",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("status Execute() error = %v\noutput:\n%s", err, statusOut.String())
	}

	got := statusOut.String()
	for _, want := range []string{
		"ok",
		"configs/git/gitconfig -> " + gitconfigTarget,
		"configs/zellij/config.kdl -> " + zellijConfigTarget,
		"configs/zellij/layouts/default.kdl -> " + zellijLayoutTarget,
		"Summary: 11 ok, 0 missing, 0 conflict, 0 skipped, 0 drifted, 0 unsupported",
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

func TestDepsCheckCommandReportsPresentAndMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Build an isolated PATH so dependency presence is deterministic: only
	// "presenttool" resolves; "absenttool" is nowhere on PATH.
	binDir := t.TempDir()
	writeFakeExecutable(t, binDir, "presenttool")
	t.Setenv("PATH", binDir)

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: presenttool
      - name: absenttool
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "check", "--file", manifestPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	for _, want := range []string{
		`Dependencies for profile "default"`,
		"present  presenttool",
		"missing  absenttool",
		"Summary: 1 present, 1 missing",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("deps check output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestDepsPlanCommandRendersTierGuidanceForMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// No tools on PATH: the dependency is missing and must appear in the plan.
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: starship
        brew: starship
        apt: starship
        dnf: starship
        pacman: starship
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// --tier makes the guidance deterministic regardless of the test host.
	cmd.SetArgs([]string{"deps", "plan", "--file", manifestPath, "--tier", "debian"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	for _, want := range []string{
		`Dependency plan for profile "default" (debian)`,
		"starship",
		"sudo apt-get install starship",
		"Summary: 1 dependency to install",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("deps plan output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestDepsPlanCommandAcceptsMixedCaseTier(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: starship
        apt: starship
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "plan", "--file", manifestPath, "--tier", "Debian"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	if !strings.Contains(got, `(debian)`) || !strings.Contains(got, "sudo apt-get install starship") {
		t.Fatalf("deps plan did not normalize mixed-case tier\noutput:\n%s", got)
	}
}

func TestDepsPlanCommandRejectsUnknownTier(t *testing.T) {
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

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"deps", "plan", "--file", manifestPath, "--tier", "windows"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want error for unknown tier")
	}
}

func writeFakeExecutable(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
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

func TestBackupsListCommandHandlesMissingHistory(t *testing.T) {
	stateRoot := t.TempDir()

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backups", "list", "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := "No Backup Sets recorded in state root: " + stateRoot + "\n"
	if got := out.String(); got != want {
		t.Fatalf("backups list output mismatch\n got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBackupsListCommandRendersBackupSetsFromStateRoot(t *testing.T) {
	stateRoot := t.TempDir()
	metadata := []byte(`{
  "version": 1,
  "sets": [
    {
      "id": "backup-001",
      "createdAt": "2026-01-02T03:04:05Z",
      "reason": "pre-install conflict protection",
      "targets": ["/home/user/.zshrc", "/home/user/.gitconfig"]
    },
    {
      "id": "backup-002",
      "createdAt": "2026-01-03T04:05:06Z",
      "reason": "manual safety snapshot",
      "targets": ["/home/user/.config/nvim/init.lua"]
    }
  ]
}
`)
	if err := os.MkdirAll(filepath.Dir(backups.Path(stateRoot)), 0o755); err != nil {
		t.Fatalf("create Backup Metadata directory: %v", err)
	}
	if err := os.WriteFile(backups.Path(stateRoot), metadata, 0o600); err != nil {
		t.Fatalf("write Backup Metadata: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backups", "list", "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := `Backup Sets

  ID: backup-001
  Created: 2026-01-02T03:04:05Z
  Reason: pre-install conflict protection
  Protected targets:
    - /home/user/.zshrc
    - /home/user/.gitconfig

  ID: backup-002
  Created: 2026-01-03T04:05:06Z
  Reason: manual safety snapshot
  Protected targets:
    - /home/user/.config/nvim/init.lua

Summary: 2 Backup Sets
`
	if got := out.String(); got != want {
		t.Fatalf("backups list output mismatch\n got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBackupsListCommandHandlesEmptyHistory(t *testing.T) {
	stateRoot := t.TempDir()
	metadata := []byte(`{"version":1,"sets":[]}` + "\n")
	if err := os.MkdirAll(filepath.Dir(backups.Path(stateRoot)), 0o755); err != nil {
		t.Fatalf("create Backup Metadata directory: %v", err)
	}
	if err := os.WriteFile(backups.Path(stateRoot), metadata, 0o600); err != nil {
		t.Fatalf("write empty Backup Metadata: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backups", "list", "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := "No Backup Sets recorded in state root: " + stateRoot + "\n"
	if got := out.String(); got != want {
		t.Fatalf("backups list output mismatch\n got:\n%q\nwant:\n%q", got, want)
	}
}

func TestBackupsListCommandRejectsDefaultStateRootSymlinkEscapeBeforeReadingMetadata(t *testing.T) {
	home := t.TempDir()
	outsideState := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	stateParent := filepath.Join(home, ".local", "state")
	if err := os.MkdirAll(stateParent, 0o755); err != nil {
		t.Fatalf("mkdir state parent: %v", err)
	}
	if err := os.Symlink(outsideState, filepath.Join(stateParent, "dots")); err != nil {
		t.Fatalf("symlink default state root: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(backups.Path(outsideState)), 0o755); err != nil {
		t.Fatalf("mkdir outside Backup Metadata directory: %v", err)
	}
	if err := os.WriteFile(backups.Path(outsideState), []byte(`{"version":1,"sets":[]}`), 0o600); err != nil {
		t.Fatalf("write outside Backup Metadata: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backups", "list", "--home", home})
	if err := cmd.Execute(); err == nil {
		t.Fatal("backups list Execute() error = nil, want default state-root symlink escape error")
	}
}

func TestBackupsListCommandRejectsDefaultMetadataLeafSymlinkEscapeBeforeReadingMetadata(t *testing.T) {
	home := t.TempDir()
	outsideState := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	stateRoot := filepath.Join(home, ".local", "state", "dots")
	metadataDir := filepath.Dir(backups.Path(stateRoot))
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		t.Fatalf("mkdir Backup Metadata directory: %v", err)
	}
	outsideMetadata := backups.Path(outsideState)
	if err := os.MkdirAll(filepath.Dir(outsideMetadata), 0o755); err != nil {
		t.Fatalf("mkdir outside Backup Metadata directory: %v", err)
	}
	if err := os.WriteFile(outsideMetadata, []byte(`{"version":1,"sets":[]}`), 0o600); err != nil {
		t.Fatalf("write outside Backup Metadata: %v", err)
	}
	if err := os.Symlink(outsideMetadata, backups.Path(stateRoot)); err != nil {
		t.Fatalf("symlink Backup Metadata leaf: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"backups", "list", "--home", home})
	if err := cmd.Execute(); err == nil {
		t.Fatal("backups list Execute() error = nil, want Backup Metadata leaf symlink escape error")
	}
}
