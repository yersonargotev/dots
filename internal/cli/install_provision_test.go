package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
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
  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`

func writeExecStub(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub %s: %v", path, err)
	}
}

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
	cmd.SetArgs([]string{"install", "--dry-run", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the file entry; lstat err = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		`Provisioners for profile "default" (tags: core)`,
		"claude plugin marketplace add example/tools",
		"affects: ~/.claude, ~/.claude.json",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestInstallRetiresHistoricalDelegationOnlyAfterSuccessfulApply(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
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
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Provisioners: []state.ProvisionerRecord{
		{Tool: "skills", Executable: "npx", Args: []string{"--yes", "skills@1.5.12", "add", "yersonargotev/dots/skills/delegation", "--agent", "codex", "--skill", "delegation", "--global", "--copy"}},
		{Profiles: []string{"agents"}, Tags: []string{"agents"}, Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global", "--agents", "codex"}, Status: "provisioned"},
	}}); err != nil {
		t.Fatalf("save historical metadata: %v", err)
	}
	agentsPath := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(agentsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "before\n<!-- gentle-ai:trigger-rules -->\nretired\n<!-- /gentle-ai:trigger-rules -->\n<!-- dots:delegation -->\nowned\n<!-- /dots:delegation -->\nafter\n"
	if err := os.WriteFile(agentsPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--skip-deps", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install error = %v\noutput:\n%s", err, out.String())
	}
	got, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "dots:delegation") || strings.Contains(string(got), "gentle-ai:trigger-rules") || !strings.Contains(out.String(), "Historical retirement:") || !strings.Contains(out.String(), "Gentle AI blocks") {
		t.Fatalf("historical retirement was not reported and applied\ninstructions:\n%s\noutput:\n%s", got, out.String())
	}

	if err := os.WriteFile(agentsPath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	dryRun := cli.NewRootCommand()
	dryRun.SetOut(&out)
	dryRun.SetErr(&out)
	dryRun.SetArgs([]string{"install", "--dry-run", "--skip-deps", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := dryRun.Execute(); err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	got, err = os.ReadFile(agentsPath)
	if err != nil || string(got) != legacy {
		t.Fatalf("dry-run changed delegation state: %q, %v", got, err)
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
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nprintf 'ok' > \"$HOME/claude-ran\"\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, provisionerManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	// The file entry must be installed.
	if _, err := os.Lstat(filepath.Join(sandboxHome, ".zshrc")); err != nil {
		t.Fatalf("install did not create the file entry: %v", err)
	}
	// The provisioner must have run with HOME threaded to the sandbox.
	if _, err := os.Stat(filepath.Join(sandboxHome, "claude-ran")); err != nil {
		t.Fatalf("provisioner did not run under the sandbox HOME %q: %v", sandboxHome, err)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "claude-ran")); err == nil {
		t.Fatalf("provisioner wrote into the inherited HOME %q instead of the sandbox", fakeRealHome)
	}
}

func TestInstallProvisionerInheritsHomebrewRustupProxyPath(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	stubDir := t.TempDir()
	formulaPrefix := filepath.Join(t.TempDir(), "rustup")
	proxyBin := filepath.Join(formulaPrefix, "bin")
	if err := os.MkdirAll(proxyBin, 0o755); err != nil {
		t.Fatalf("create fake rustup proxy bin: %v", err)
	}
	for _, command := range []string{"rustc", "cargo"} {
		writeExecStub(t, filepath.Join(proxyBin, command), "#!/bin/sh\nexit 0\n")
	}
	writeExecStub(t, filepath.Join(stubDir, "brew"), "#!/bin/sh\nif [ \"$1\" = \"--prefix\" ]; then printf '%s\\n' "+strconv.Quote(formulaPrefix)+"; fi\n")
	writeExecStub(t, filepath.Join(stubDir, "rustup"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\ncommand -v rustc >/dev/null || exit 9\nprintf 'ok' > \"$HOME/provisioner-saw-rustc\"\n")
	t.Setenv("PATH", stubDir)

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, `version: 1
profiles:
  default:
    tags: [core]
dependencies:
  - tags: [core]
    dependencies:
      - name: Rust stable (rustup)
        commands: [rustup, rustc, cargo]
        brew: rustup
        linux_homebrew: true
        toolchain: rust-stable-rustup
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(sandboxHome, "provisioner-saw-rustc")); err != nil {
		t.Fatalf("provisioner did not inherit Homebrew rustup proxy PATH: %v\noutput:\n%s", err, out.String())
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
	cmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

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
  - tool: claude
    tags: [desktop]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

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
	cmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

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
	cmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

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
	cmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

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
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nprintf 'ran' > \"$HOME/claude-ran\"\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	previousSelection := state.InstalledSelection{
		Profiles:     []string{"default"},
		ResolvedTags: []string{"core"},
		Provenance:   state.Provenance{RecordedAt: "2026-01-02T03:04:05Z"},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		Entries:            []state.Record{},
		InstalledSelection: &previousSelection,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

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
  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("q"))
	cmd.SetArgs([]string{"install", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

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
	if _, err := os.Stat(filepath.Join(sandboxHome, "claude-ran")); !os.IsNotExist(err) {
		t.Fatalf("canceled install ran provisioner in sandbox HOME; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "claude-ran")); !os.IsNotExist(err) {
		t.Fatalf("canceled install ran provisioner in inherited HOME; stat err = %v", err)
	}
	if !strings.Contains(out.String(), "Conflict resolution canceled; no changes applied.") {
		t.Fatalf("cancel output missing abort message\noutput:\n%s", out.String())
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previousSelection) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previousSelection)
	}
}

func TestInstallPersistsFailedProvisionerForStatusResumeGuidance(t *testing.T) {
	sandboxHome := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nexit 7\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, sandboxHome, provisionerManifest)

	installCmd := cli.NewRootCommand()
	var installOut bytes.Buffer
	installCmd.SetOut(&installOut)
	installCmd.SetErr(&installOut)
	installCmd.SetArgs([]string{"install", "--yes", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := installCmd.Execute(); err == nil {
		t.Fatalf("install error = nil, want failing provisioner\noutput:\n%s", installOut.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	rec, ok := meta.FindProvisioner("default", "claude", "claude", []string{"plugin", "marketplace", "add", "example/tools"})
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
	statusCmd.SetArgs([]string{"status", "--profile", "default", "--file", manifestPath, "--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})
	requireFindings(t, statusCmd.Execute())
	got := statusOut.String()
	for _, want := range []string{
		`Declared provisioners for profile "default" (tags: core) — failed`,
		"failed               claude plugin marketplace add example/tools",
		"resume: run dots install again after addressing failed or missing provisioners.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}
}
