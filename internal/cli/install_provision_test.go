package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

const provisionerManifest = `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      persona: neutral
      agents: [codex]
    dependencies:
      - name: gentle-ai
      - name: engram
`

func writeExecStub(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", path, err)
	}
}

// TestInstallDryRunRendersProvisionerWithoutInvoking proves --dry-run prints the
// resolved provisioner command and the roots it affects, while creating no files
// and never invoking the tool (gentle-ai is absent, so an invocation would
// error; the run succeeds because dry-run returns before any execution).
func TestInstallDryRunRendersProvisionerWithoutInvoking(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, provisionerManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--dry-run", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the file entry; lstat err = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`Provisioners for profile "default"`,
		"gentle-ai install --scope global --persona neutral --agents codex",
		"affects: ~/.codex, ~/.gentle-ai",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q\noutput:\n%s", want, got)
		}
	}
}

// TestInstallExecutesProvisionerAfterFilesWithHomeThreaded proves a real install
// runs the provisioner after file entries, via the Runner, with HOME threaded
// from --home so the tool writes under the sandbox home and never the inherited
// one.
func TestInstallExecutesProvisionerAfterFilesWithHomeThreaded(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nprintf 'ok' > \"$HOME/gentle-ai-ran\"\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, provisionerManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	// The file entry must be installed.
	if _, err := os.Lstat(filepath.Join(sandboxHome, ".zshrc")); err != nil {
		t.Fatalf("install did not create the file entry: %v", err)
	}
	// The provisioner must have run with HOME threaded to the sandbox.
	if _, err := os.Stat(filepath.Join(sandboxHome, "gentle-ai-ran")); err != nil {
		t.Fatalf("provisioner did not run under the sandbox HOME %q: %v", sandboxHome, err)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "gentle-ai-ran")); err == nil {
		t.Fatalf("provisioner wrote into the inherited HOME %q instead of the sandbox", fakeRealHome)
	}
}

// TestInstallWritesCodexCodeGraphOverlayAfterProvisioners proves dots adds its
// own Codex AGENTS.md instruction layer only after a successful provisioner run,
// under the sandbox HOME threaded through --home.
func TestInstallWritesCodexCodeGraphOverlayAfterProvisioners(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, provisionerManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(filepath.Join(sandboxHome, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read sandbox Codex AGENTS.md: %v", err)
	}
	content := string(got)
	for _, want := range []string{
		"<!-- dots:codegraph-mode -->",
		"CodeGraph Mode: enabled",
		"Use CodeGraph for architecture questions",
		"Never use CodeGraph just because `.codegraph/` exists.",
		"<!-- /dots:codegraph-mode -->",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Codex AGENTS.md missing %q\ncontent:\n%s", want, content)
		}
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

// TestInstallDoesNotWriteCodexCodeGraphOverlayWithoutSelectedProvisioners proves
// the post-provision Codex layer is scoped to selected Codex-related
// provisioners. A successful install whose active profile selects no
// provisioners must not create a sandbox Codex AGENTS.md.
func TestInstallDoesNotWriteCodexCodeGraphOverlayWithoutSelectedProvisioners(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, `version: 1
profiles:
  default:
    tags: [core]
  desktop:
    tags: [desktop]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: gentle-ai
    tags: [desktop]
    spec:
      scope: global
      agents: [codex]
    dependencies:
      - name: gentle-ai
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(sandboxHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install without selected provisioners created sandbox Codex AGENTS.md; stat err = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

// TestInstallWritesCodexCodeGraphOverlayAfterCodexProvisioner proves a selected
// Codex MCP provisioner also qualifies for the post-provision Codex layer, not
// only the gentle-ai provisioner that installs Codex agents.
func TestInstallWritesCodexCodeGraphOverlayAfterCodexProvisioner(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "codex"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: codex
    tags: [core]
    spec:
      mcp: codegraph
      command: [codegraph, serve, --mcp]
    dependencies:
      - name: codex
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if _, err := os.ReadFile(filepath.Join(sandboxHome, ".codex", "AGENTS.md")); err != nil {
		t.Fatalf("read sandbox Codex AGENTS.md after codex provisioner: %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

// TestInstallWritesCodexCodeGraphOverlayAfterSkillsProvisioner proves a
// skills-only provisioner targeting Codex also qualifies for the post-provision
// Codex layer.
func TestInstallWritesCodexCodeGraphOverlayAfterSkillsProvisioner(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "npx"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
      agents: [codex]
      skills: [web-design-guidelines]
      global: true
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if _, err := os.ReadFile(filepath.Join(sandboxHome, ".codex", "AGENTS.md")); err != nil {
		t.Fatalf("read sandbox Codex AGENTS.md after skills provisioner: %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

// TestInstallExecutesClaudeProvisionerHomeThreaded proves a claude provisioner
// renders and runs the exact `claude plugin ...` invocations, in manifest order,
// under the sandbox HOME and never the inherited one.
func TestInstallExecutesClaudeProvisionerHomeThreaded(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	// The claude stub appends each invocation's argv to a log under the threaded
	// HOME so the test can assert the resolved commands and their order.
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HOME/claude-calls\"\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: claude
    tags: [core]
    spec:
      marketplace: ChromeDevTools/chrome-devtools-mcp
    dependencies:
      - name: claude
  - tool: claude
    tags: [core]
    spec:
      plugin: chrome-devtools-mcp
      from: chrome-devtools-plugins
    dependencies:
      - name: claude
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	calls, err := os.ReadFile(filepath.Join(sandboxHome, "claude-calls"))
	if err != nil {
		t.Fatalf("claude provisioner did not run under the sandbox HOME %q: %v", sandboxHome, err)
	}
	want := "plugin marketplace add ChromeDevTools/chrome-devtools-mcp\n" +
		"plugin install chrome-devtools-mcp@chrome-devtools-plugins --scope user\n"
	if string(calls) != want {
		t.Fatalf("claude calls = %q, want %q", calls, want)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "claude-calls")); err == nil {
		t.Fatalf("claude provisioner wrote into the inherited HOME %q instead of the sandbox", fakeRealHome)
	}
	if _, err := os.Stat(filepath.Join(sandboxHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("claude-only provisioners created sandbox Codex AGENTS.md; stat err = %v", err)
	}
}

// TestInstallTUICancelDoesNotRunProvisioners proves canceling conflict
// resolution aborts the whole install before any provisioner can mutate
// tool-managed config roots.
func TestInstallTUICancelDoesNotRunProvisioners(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nprintf 'ran' > \"$HOME/gentle-ai-ran\"\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/git/gitconfig", "managed\n")
	if err := os.WriteFile(filepath.Join(sandboxHome, ".gitconfig"), []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
	}
	manifestPath := writeCLIManifest(t, sandboxHome, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/git/gitconfig
    target: ~/.gitconfig
    strategy: copy
    tags: [core]
provisioners:
  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      persona: neutral
      agents: [codex]
    dependencies:
      - name: gentle-ai
      - name: engram
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("q"))
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(filepath.Join(sandboxHome, ".gitconfig"))
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "local\n" {
		t.Fatalf("canceled install changed conflict target to %q", got)
	}
	if _, err := os.Stat(filepath.Join(sandboxHome, "gentle-ai-ran")); !os.IsNotExist(err) {
		t.Fatalf("canceled install ran provisioner in sandbox HOME; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "gentle-ai-ran")); !os.IsNotExist(err) {
		t.Fatalf("canceled install ran provisioner in inherited HOME; stat err = %v", err)
	}
	if !strings.Contains(out.String(), "Conflict resolution canceled; no changes applied.") {
		t.Fatalf("cancel output missing abort message\noutput:\n%s", out.String())
	}
}
