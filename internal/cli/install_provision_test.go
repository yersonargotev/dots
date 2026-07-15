package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
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

const codexDelegationProfileManifest = `version: 1
profiles:
  codex-delegation:
    tags: [codex-delegation]
entries:
  - source: configs/unused
    target: ~/.unused
    strategy: copy
    tags: [unused]
provisioners:
  - tool: skills
    tags: [codex-delegation]
    spec:
      package: yersonargotev/dots/skills/delegation
      agents: [codex]
      skills: [delegation]
      global: true
      copy: true
    dependencies:
      - name: npx
        command: npx
`

var nativeCodexSparkAgentFiles = []string{"dots-explorer.toml", "dots-worker.toml"}

func writeExecStub(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", path, err)
	}
}

func assertNoNativeCodexSparkAgents(t *testing.T, home, context string) {
	t.Helper()
	for _, name := range nativeCodexSparkAgentFiles {
		if _, err := os.Stat(filepath.Join(home, ".codex", "agents", name)); !os.IsNotExist(err) {
			t.Fatalf("%s wrote native Codex agent %s; stat err = %v", context, name, err)
		}
	}
}

func assertNativeCodexSparkAgents(t *testing.T, home, context string) {
	t.Helper()
	for _, tc := range []struct {
		name string
		want string
	}{
		{"dots-explorer.toml", `name = "dots-explorer"`},
		{"dots-worker.toml", `name = "dots-worker"`},
	} {
		path := filepath.Join(home, ".codex", "agents", tc.name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s did not write native agent %s: %v", context, path, err)
		}
		if !strings.Contains(string(got), tc.want) || !strings.Contains(string(got), "gpt-5.6-sol") || !strings.Contains(string(got), `model_reasoning_effort = "low"`) {
			t.Fatalf("native Codex agent %s missing expected content\n%s", path, got)
		}
	}
}

func writeStaleNativeCodexSparkAgents(t *testing.T, home string) string {
	t.Helper()
	agentsDir := filepath.Join(home, ".codex", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir preexisting Codex agents dir: %v", err)
	}
	for _, name := range nativeCodexSparkAgentFiles {
		if err := os.WriteFile(filepath.Join(agentsDir, name), []byte("stale dots-owned agent"), 0o600); err != nil {
			t.Fatalf("write preexisting Codex agent %s: %v", name, err)
		}
	}
	return agentsDir
}

func assertRemovedNativeCodexSparkAgents(t *testing.T, agentsDir, context string) {
	t.Helper()
	for _, name := range nativeCodexSparkAgentFiles {
		if _, err := os.Stat(filepath.Join(agentsDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s kept native Codex agent %s; stat err = %v", context, name, err)
		}
	}
}

func codexHookCommandFromConfig(t *testing.T, content string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		raw, ok := strings.CutPrefix(strings.TrimSpace(line), "command = ")
		if !ok {
			continue
		}
		command, err := strconv.Unquote(raw)
		if err != nil {
			t.Fatalf("parse Codex hook command %q: %v", raw, err)
		}
		return command
	}
	t.Fatal("Codex config missing hook command")
	return ""
}

func runCodexSessionStartHook(t *testing.T, dir, command string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run CodeGraph SessionStart hook in %s: %v\noutput:\n%s", dir, err, output)
	}
}

func initGitRepo(t *testing.T, gitPath, dir string) {
	t.Helper()
	cmd := exec.Command(gitPath, "init", "-q")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git init %s: %v\noutput:\n%s", dir, err, output)
	}
}

func gitTopLevel(t *testing.T, gitPath, dir string) string {
	t.Helper()
	cmd := exec.Command(gitPath, "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git top-level %s: %v\noutput:\n%s", dir, err, output)
	}
	return strings.TrimSpace(string(output))
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
		`Provisioners for profile "default" (tags: core)`,
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

func TestInstallCoreZimFWProvisionerCreatesRuntimeInSandbox(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "zsh"), `#!/bin/sh
printf '%s\n' "$*" > "$HOME/zimfw-args"
test -e "$HOME/.zimrc" || exit 42
mkdir -p "$HOME/.zim"
printf 'zimfw\n' > "$HOME/.zim/zimfw.zsh"
printf 'init\n' > "$HOME/.zim/init.zsh"
`)
	writeExecStub(t, filepath.Join(stubDir, "git"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "curl"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed zshrc\n")
	writeCLISource(t, sourceRoot, "configs/zsh/zimrc", "managed zimrc\n")
	manifestPath := writeCLIManifest(t, sandboxHome, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
  - source: configs/zsh/zimrc
    target: ~/.zimrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: zimfw
    tags: [core]
    spec:
      yes: true
    dependencies:
      - name: zsh
      - name: git
      - name: curl
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if _, err := os.Lstat(filepath.Join(sandboxHome, ".zshrc")); err != nil {
		t.Fatalf("install did not create .zshrc: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(sandboxHome, ".zimrc")); err != nil {
		t.Fatalf("install did not create .zimrc before provisioning: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandboxHome, ".zim", "init.zsh")); err != nil {
		t.Fatalf("zimfw provisioner did not create sandbox runtime: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".zim", "init.zsh")); err == nil {
		t.Fatalf("zimfw provisioner wrote runtime into inherited HOME %q", fakeRealHome)
	}
	args, err := os.ReadFile(filepath.Join(sandboxHome, "zimfw-args"))
	if err != nil {
		t.Fatalf("read zimfw args: %v", err)
	}
	if !strings.Contains(string(args), "zimfw.zsh") || !strings.Contains(string(args), "init -q") {
		t.Fatalf("zimfw args did not include fixed init script:\n%s", string(args))
	}
}

// TestInstallDoesNotWriteCodeGraphInstructionBlockAfterGentleAIProvisioner proves a
// non-CodeGraph provisioner does not create the dots-owned CodeGraph
// instruction block.
func TestInstallDoesNotWriteCodeGraphInstructionBlockAfterGentleAIProvisioner(t *testing.T) {
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
		t.Fatalf("install with gentle-ai provisioner did not write dots rules block: %v\noutput:\n%s", err, out.String())
	}
	if strings.Contains(string(got), "dots:codegraph-mode") {
		t.Fatalf("install with gentle-ai provisioner wrote CodeGraph instruction block without the codegraph tag\ncontent:\n%s\noutput:\n%s", got, out.String())
	}
	if !strings.Contains(string(got), "<!-- dots:rules -->") {
		t.Fatalf("install with gentle-ai provisioner did not write dots rules block\ncontent:\n%s\noutput:\n%s", got, out.String())
	}
	if strings.Contains(string(got), "<!-- dots:delegation -->") || strings.Contains(string(got), "argote:subagent-delegation") || strings.Contains(string(got), "gpt-5.6-sol") {
		t.Fatalf("install without Codex delegation tag wrote delegation guidance\ncontent:\n%s\noutput:\n%s", got, out.String())
	}
	assertNoNativeCodexSparkAgents(t, sandboxHome, "install without Codex delegation tag")
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

func TestInstallCodexDelegationTagInstallsGuidance(t *testing.T) {
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
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot, "--tag", "codex-delegation"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(filepath.Join(sandboxHome, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatalf("install with Codex delegation tag did not write Codex AGENTS.md: %v\noutput:\n%s", err, out.String())
	}
	content := string(got)
	for _, want := range []string{"<!-- dots:rules -->", "<!-- dots:delegation -->", "gpt-5.6-sol"} {
		if !strings.Contains(content, want) {
			t.Fatalf("install with Codex delegation tag missing %q\ncontent:\n%s\noutput:\n%s", want, content, out.String())
		}
	}
	assertNativeCodexSparkAgents(t, sandboxHome, "install with Codex delegation tag")
	if strings.Contains(content, "argote:subagent-delegation") {
		t.Fatalf("install with Codex delegation tag used legacy markers\ncontent:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

func TestInstallCodexDelegationProfileInstallsOnlyCodexDelegation(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "npx"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifestPath := writeCLIManifest(t, sandboxHome, codexDelegationProfileManifest)
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--skip-deps", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot, "--profile", "codex-delegation"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	content, err := os.ReadFile(filepath.Join(sandboxHome, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatalf("codex-delegation profile did not write Codex AGENTS.md: %v\noutput:\n%s", err, out.String())
	}
	for _, want := range []string{"<!-- dots:delegation -->", "gpt-5.6-sol"} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("codex-delegation profile missing %q\ncontent:\n%s\noutput:\n%s", want, content, out.String())
		}
	}
	if strings.Contains(string(content), "<!-- dots:rules -->") {
		t.Fatalf("codex-delegation profile installed the broader agent baseline\ncontent:\n%s", content)
	}
	if !strings.Contains(out.String(), "--agent codex --skill delegation --global --copy") {
		t.Fatalf("codex-delegation profile did not select the Codex-only skill provisioner\noutput:\n%s", out.String())
	}
	assertNativeCodexSparkAgents(t, sandboxHome, "codex-delegation profile")
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

func TestInstallWithoutCodexDelegationTagRemovesCurrentAndLegacyGuidance(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	codexPath := filepath.Join(sandboxHome, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(codexPath), 0o755); err != nil {
		t.Fatalf("mkdir Codex dir: %v", err)
	}
	preexisting := "before\n\n<!-- dots:delegation -->\ncurrent\n<!-- /dots:delegation -->\n\n<!-- argote:subagent-delegation -->\nlegacy\n<!-- /argote:subagent-delegation -->\n\nafter\n"
	if err := os.WriteFile(codexPath, []byte(preexisting), 0o600); err != nil {
		t.Fatalf("write preexisting Codex instructions: %v", err)
	}
	agentsDir := writeStaleNativeCodexSparkAgents(t, sandboxHome)

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, provisionerManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot, "--tag", "codex-delegation", "--tag", "without-codex-delegation"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatalf("read Codex instructions: %v", err)
	}
	content := string(got)
	for _, not := range []string{"<!-- dots:delegation -->", "argote:subagent-delegation", "\ncurrent\n", "\nlegacy\n", "gpt-5.6-sol"} {
		if strings.Contains(content, not) {
			t.Fatalf("without Codex delegation tag kept %q\ncontent:\n%s\noutput:\n%s", not, content, out.String())
		}
	}
	for _, want := range []string{"<!-- dots:rules -->", "Keep diffs surgical", "before", "after"} {
		if !strings.Contains(content, want) {
			t.Fatalf("without Codex delegation tag removed expected %q\ncontent:\n%s", want, content)
		}
	}
	assertRemovedNativeCodexSparkAgents(t, agentsDir, "without Codex delegation tag")
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

// TestInstallDoesNotWriteCodexCodeGraphInstructionBlockWithoutSelectedProvisioners proves
// the post-provision CodeGraph instruction block is scoped to the codegraph tag. A
// successful install whose active profile selects no provisioners must not
// create a sandbox Codex AGENTS.md.
func TestInstallDoesNotWriteCodexCodeGraphInstructionBlockWithoutSelectedProvisioners(t *testing.T) {
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

// TestInstallDoesNotWriteCodeGraphInstructionBlockAfterCodexProvisioner proves a Codex
// MCP provisioner alone does not create the dots-owned CodeGraph instruction
// layer.
func TestInstallDoesNotWriteCodeGraphInstructionBlockAfterCodexProvisioner(t *testing.T) {
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

	if _, err := os.Stat(filepath.Join(sandboxHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install with codex provisioner wrote CodeGraph instruction block without the codegraph tag; stat err = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

// TestInstallDoesNotWriteCodeGraphInstructionBlockAfterSkillsProvisioner proves a
// skills-only provisioner targeting Codex does not create the dots-owned
// CodeGraph instruction block.
func TestInstallDoesNotWriteCodeGraphInstructionBlockAfterSkillsProvisioner(t *testing.T) {
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

	if _, err := os.Stat(filepath.Join(sandboxHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install with skills provisioner wrote CodeGraph instruction block without the codegraph tag; stat err = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("install wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

func TestInstallAgentsCodeGraphTagWritesScopedPolicyOverlayInSandbox(t *testing.T) {
	initialPath := os.Getenv("PATH")
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is required to exercise the CodeGraph SessionStart hook")
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	sandboxHome := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), `#!/bin/sh
if [ "$1" = init ]; then
  printf '%s|%s\n' "$PWD" "$*" >> "$HOME/codegraph-init-calls"
  mkdir -p .codegraph
  exit 0
fi
printf '%s\n' "$*" >> "$HOME/codegraph-args"
mkdir -p "$HOME/.codex" "$HOME/.claude" "$HOME/.gemini" "$HOME/.config/opencode"
for file in "$HOME/.codex/AGENTS.md" "$HOME/.claude/CLAUDE.md" "$HOME/.gemini/GEMINI.md" "$HOME/.config/opencode/codegraph.md"; do
  cat > "$file" <<'EOF'
<!-- CODEGRAPH_START -->
Treat CodeGraph-returned source as already read.
<!-- CODEGRAPH_END -->
EOF
done
`)
	writeExecStub(t, filepath.Join(stubDir, "curl"), "#!/bin/sh\nexit 0\n")
	writeManifestDependencyStubs(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
		"--tag", "codegraph",
		"--source-root", repoRoot,
		"--home", sandboxHome,
		"--state-root", stateRoot,
		"--yes",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dots install --tag codegraph failed in sandbox: %v\noutput:\n%s", err, out.String())
	}

	gotArgs, err := os.ReadFile(filepath.Join(sandboxHome, "codegraph-args"))
	if err != nil {
		t.Fatalf("CodeGraph provisioner did not run under sandbox HOME %q: %v", sandboxHome, err)
	}
	wantArgs := "install --target codex,claude,antigravity,opencode --location global --yes\n"
	if string(gotArgs) != wantArgs {
		t.Fatalf("codegraph args = %q, want %q", gotArgs, wantArgs)
	}

	codexConfig, err := os.ReadFile(filepath.Join(sandboxHome, ".codex", "config.toml"))
	if err != nil {
		t.Fatalf("missing CodeGraph-tagged Codex config in sandbox HOME: %v", err)
	}
	for _, want := range []string{
		"[[hooks.SessionStart]]",
		`matcher = "startup|resume"`,
		`command = "sh -c`,
		"codegraph init",
		`[ -d \"$root/.codegraph\" ] || (cd \"$root\" && codegraph init)`,
	} {
		if !strings.Contains(string(codexConfig), want) {
			t.Fatalf("CodeGraph-tagged Codex config missing %q\ncontent:\n%s", want, codexConfig)
		}
	}
	hookCommand := codexHookCommandFromConfig(t, string(codexConfig))
	t.Setenv("HOME", sandboxHome)
	hookStubDir := t.TempDir()
	writeExecStub(t, filepath.Join(hookStubDir, "codegraph"), `#!/bin/sh
printf '%s|%s\n' "$PWD" "$*" >> "$HOME/codegraph-init-calls"
mkdir -p .codegraph
`)
	t.Setenv("PATH", hookStubDir+string(os.PathListSeparator)+filepath.Dir(realGit)+string(os.PathListSeparator)+initialPath)
	runCodexSessionStartHook(t, t.TempDir(), hookCommand)

	repoWithIndex := t.TempDir()
	initGitRepo(t, realGit, repoWithIndex)
	repoWithIndexRoot := gitTopLevel(t, realGit, repoWithIndex)
	if err := os.MkdirAll(filepath.Join(repoWithIndexRoot, ".codegraph"), 0o755); err != nil {
		t.Fatalf("seed existing CodeGraph index: %v", err)
	}
	runCodexSessionStartHook(t, repoWithIndex, hookCommand)

	repoWithoutIndex := t.TempDir()
	initGitRepo(t, realGit, repoWithoutIndex)
	repoWithoutIndexRoot := gitTopLevel(t, realGit, repoWithoutIndex)
	runCodexSessionStartHook(t, repoWithoutIndex, hookCommand)
	if _, err := os.Stat(filepath.Join(repoWithoutIndexRoot, ".codegraph")); err != nil {
		t.Fatalf("CodeGraph hook did not initialize temp repo index: %v", err)
	}
	gotInitCalls, err := os.ReadFile(filepath.Join(sandboxHome, "codegraph-init-calls"))
	if err != nil {
		t.Fatalf("CodeGraph hook did not record sandboxed init calls: %v", err)
	}
	wantInitCalls := repoWithoutIndexRoot + "|init\n"
	if string(gotInitCalls) != wantInitCalls {
		t.Fatalf("CodeGraph hook calls = %q, want %q", gotInitCalls, wantInitCalls)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "codegraph-init-calls")); !os.IsNotExist(err) {
		t.Fatalf("CodeGraph hook wrote into inherited HOME %q; stat err = %v", fakeRealHome, err)
	}

	for _, path := range []string{
		filepath.Join(sandboxHome, ".codex", "AGENTS.md"),
		filepath.Join(sandboxHome, ".claude", "CLAUDE.md"),
		filepath.Join(sandboxHome, ".gemini", "GEMINI.md"),
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("missing dots CodeGraph policy overlay %s: %v", path, err)
		}
		content := string(got)
		for _, want := range []string{
			"<!-- dots:codegraph-mode -->",
			"Use CodeGraph for architecture questions, symbol discovery, call flow, impact analysis, and locating relevant source files before edits.",
			"Do NOT use CodeGraph as proof for runtime behavior.",
			"<!-- /dots:codegraph-mode -->",
		} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s missing scoped CodeGraph policy %q\ncontent:\n%s", path, want, content)
			}
		}
		if strings.Contains(content, "codegraph_explore") || strings.Contains(content, "codegraph init -i") {
			t.Fatalf("%s duplicated generic CodeGraph installer guidance\ncontent:\n%s", path, content)
		}
		if strings.Count(content, "Treat CodeGraph-returned source as already read.") != 1 {
			t.Fatalf("%s should contain generic CodeGraph guidance only from installer-owned block\ncontent:\n%s", path, content)
		}
	}
	opencodeContent, err := os.ReadFile(filepath.Join(sandboxHome, ".config", "opencode", "codegraph.md"))
	if err != nil {
		t.Fatalf("CodeGraph installer stub did not cover OpenCode setup under sandbox HOME: %v", err)
	}
	if strings.Contains(string(opencodeContent), "<!-- dots:codegraph-mode -->") {
		t.Fatalf("dots must not create OpenCode policy overlay; CodeGraph installer owns OpenCode setup\ncontent:\n%s", opencodeContent)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "codegraph-args")); err == nil {
		t.Fatalf("CodeGraph provisioner wrote into inherited HOME %q instead of sandbox", fakeRealHome)
	}
}

func TestInstallAgentsCodeGraphTagMigratesExistingManagedCodexConfig(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	sandboxHome := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), `#!/bin/sh
printf '%s\n' "$*" >> "$HOME/codegraph-args"
mkdir -p "$HOME/.codex" "$HOME/.claude" "$HOME/.gemini" "$HOME/.config/opencode"
for file in "$HOME/.codex/AGENTS.md" "$HOME/.claude/CLAUDE.md" "$HOME/.gemini/GEMINI.md" "$HOME/.config/opencode/codegraph.md"; do
  cat > "$file" <<'EOF'
<!-- CODEGRAPH_START -->
Treat CodeGraph-returned source as already read.
<!-- CODEGRAPH_END -->
EOF
done
`)
	writeExecStub(t, filepath.Join(stubDir, "curl"), "#!/bin/sh\nexit 0\n")
	writeManifestDependencyStubs(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	base := cli.NewRootCommand()
	var baseOut bytes.Buffer
	base.SetOut(&baseOut)
	base.SetErr(&baseOut)
	base.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
		"--source-root", repoRoot,
		"--home", sandboxHome,
		"--state-root", stateRoot,
		"--yes",
	})
	if err := base.Execute(); err != nil {
		t.Fatalf("base dots install failed in sandbox: %v\noutput:\n%s", err, baseOut.String())
	}

	codexConfigPath := filepath.Join(sandboxHome, ".codex", "config.toml")
	baseCodexConfig, err := os.ReadFile(codexConfigPath)
	if err != nil {
		t.Fatalf("read base Codex config: %v", err)
	}
	if strings.Contains(string(baseCodexConfig), "codegraph init") {
		t.Fatalf("base Codex config unexpectedly contains CodeGraph hook\ncontent:\n%s", baseCodexConfig)
	}
	runtimeAugmented := append([]byte("model = \"gpt-5.5\"\n\n"), baseCodexConfig...)
	runtimeAugmented = append(runtimeAugmented, []byte("\n[mcp_servers.codegraph]\ncommand = \"codegraph\"\nargs = [\"serve\", \"--mcp\"]\n")...)
	if err := os.WriteFile(codexConfigPath, runtimeAugmented, 0o600); err != nil {
		t.Fatalf("write runtime-augmented Codex config: %v", err)
	}

	upgrade := cli.NewRootCommand()
	var upgradeOut bytes.Buffer
	upgrade.SetOut(&upgradeOut)
	upgrade.SetErr(&upgradeOut)
	upgrade.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
		"--tag", "codegraph",
		"--source-root", repoRoot,
		"--home", sandboxHome,
		"--state-root", stateRoot,
		"--yes",
	})
	if err := upgrade.Execute(); err != nil {
		t.Fatalf("CodeGraph tagged dots install failed in sandbox: %v\noutput:\n%s", err, upgradeOut.String())
	}
	if strings.Contains(upgradeOut.String(), "conflict        copy      configs/codex/config-codegraph.toml") {
		t.Fatalf("CodeGraph tagged install reported Codex config conflict\noutput:\n%s", upgradeOut.String())
	}
	got, err := os.ReadFile(codexConfigPath)
	if err != nil {
		t.Fatalf("read migrated Codex config: %v", err)
	}
	for _, want := range []string{
		`model = "gpt-5.5"`,
		`[mcp_servers.codegraph]`,
		`[[hooks.SessionStart]]`,
		"codegraph init",
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("migrated Codex config missing %q\ncontent:\n%s", want, got)
		}
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

func TestInstallPersistsFailedProvisionerForStatusResumeGuidance(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 7\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, provisionerManifest)

	installCmd := cli.NewRootCommand()
	var installOut bytes.Buffer
	installCmd.SetOut(&installOut)
	installCmd.SetErr(&installOut)
	installCmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := installCmd.Execute(); err == nil {
		t.Fatalf("install error = nil, want failing provisioner\noutput:\n%s", installOut.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	rec, ok := meta.FindProvisioner("default", "gentle-ai", "gentle-ai", []string{"install", "--scope", "global", "--persona", "neutral", "--agents", "codex"})
	if !ok {
		t.Fatalf("failed provisioner was not persisted: %+v", meta.Provisioners)
	}
	if rec.Status != "failed" {
		t.Fatalf("provisioner status = %q, want failed", rec.Status)
	}

	statusCmd := cli.NewRootCommand()
	var statusOut bytes.Buffer
	statusCmd.SetOut(&statusOut)
	statusCmd.SetErr(&statusOut)
	statusCmd.SetArgs([]string{"status", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})
	requireFindings(t, statusCmd.Execute())
	got := statusOut.String()
	for _, want := range []string{
		`Declared provisioners for profile "default" (tags: core) — failed`,
		"failed               gentle-ai install --scope global --persona neutral --agents codex",
		"resume: run dots install again after addressing failed or missing provisioners.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}
}
