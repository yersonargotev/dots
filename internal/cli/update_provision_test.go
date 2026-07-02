package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

const updateProvisionerManifest = `version: 1
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
`

// TestUpdateDryRunRendersProvisionerPlan proves a dry-run update renders the
// Provisioners plan section for parity with install, without executing the tool.
func TestUpdateDryRunRendersProvisionerPlan(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})

	out := runUpdate(t, "--dry-run", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot)

	for _, want := range []string{
		`Provisioners for profile "default" (tags: core)`,
		"gentle-ai install --scope global --persona neutral --agents codex",
		"Summary: 1 provisioner(s)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("update --dry-run output missing %q\noutput:\n%s", want, out)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("dry-run installed a target; lstat err = %v", err)
	}
}

// TestUpdateExecutesProvisionersAfterApply proves a real update runs the selected
// provisioners after the file-plan apply (mirroring install), with HOME threaded
// to the sandbox, and reports the results.
func TestUpdateExecutesProvisionersAfterApply(t *testing.T) {
	requireGitCLI(t)
	sandboxHome := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nprintf 'ok' > \"$HOME/gentle-ai-ran\"\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})

	out := runUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot)

	if _, err := os.Stat(filepath.Join(sandboxHome, "gentle-ai-ran")); err != nil {
		t.Fatalf("update did not run the provisioner under the sandbox HOME %q: %v\noutput:\n%s", sandboxHome, err, out)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "gentle-ai-ran")); err == nil {
		t.Fatalf("update ran the provisioner under the inherited HOME %q instead of the sandbox", fakeRealHome)
	}
	if !strings.Contains(out, `Provisioner results for profile "default" (tags: core)`) {
		t.Fatalf("update output missing provisioner results report\noutput:\n%s", out)
	}
}

// TestUpdateDoesNotWriteCodeGraphInstructionBlockAfterGentleAIProvisioner proves update
// does not create the dots-owned CodeGraph instruction block unless the
// CodeGraph provisioner is selected.
func TestUpdateDoesNotWriteCodeGraphInstructionBlockAfterGentleAIProvisioner(t *testing.T) {
	requireGitCLI(t)
	sandboxHome := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})

	out := runUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot)

	got, err := os.ReadFile(filepath.Join(sandboxHome, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatalf("update with gentle-ai provisioner did not write dots rules block: %v\noutput:\n%s", err, out)
	}
	if strings.Contains(string(got), "dots:codegraph-mode") {
		t.Fatalf("update with gentle-ai provisioner wrote CodeGraph instruction block without the codegraph tag\ncontent:\n%s\noutput:\n%s", got, out)
	}
	if !strings.Contains(string(got), "<!-- dots:rules -->") {
		t.Fatalf("update with gentle-ai provisioner did not write dots rules block\ncontent:\n%s\noutput:\n%s", got, out)
	}
	if strings.Contains(string(got), "dots:codex-spark-delegation") || strings.Contains(string(got), "argote:subagent-delegation") || strings.Contains(string(got), "gpt-5.3-codex-spark") {
		t.Fatalf("update without Codex Spark tag wrote delegation guidance\ncontent:\n%s\noutput:\n%s", got, out)
	}
	if _, err := os.Stat(filepath.Join(sandboxHome, ".codex", "agents", "dots-explorer.toml")); !os.IsNotExist(err) {
		t.Fatalf("update without Codex Spark tag wrote native explorer agent; stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("update wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

func TestUpdateCodexSparkDelegationTagInstallsGuidance(t *testing.T) {
	requireGitCLI(t)
	sandboxHome := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})

	out := runUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot, "--tag", "codex-spark-delegation")

	got, err := os.ReadFile(filepath.Join(sandboxHome, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatalf("update with Codex Spark tag did not write Codex AGENTS.md: %v\noutput:\n%s", err, out)
	}
	content := string(got)
	for _, want := range []string{"<!-- dots:rules -->", "<!-- dots:codex-spark-delegation -->", "gpt-5.3-codex-spark"} {
		if !strings.Contains(content, want) {
			t.Fatalf("update with Codex Spark tag missing %q\ncontent:\n%s\noutput:\n%s", want, content, out)
		}
	}
	for _, tc := range []struct {
		path string
		want string
	}{
		{filepath.Join(sandboxHome, ".codex", "agents", "dots-explorer.toml"), `name = "dots-explorer"`},
		{filepath.Join(sandboxHome, ".codex", "agents", "dots-worker.toml"), `name = "dots-worker"`},
	} {
		got, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("update with Codex Spark tag did not write native agent %s: %v", tc.path, err)
		}
		if !strings.Contains(string(got), tc.want) || !strings.Contains(string(got), "gpt-5.3-codex-spark") {
			t.Fatalf("native Codex agent %s missing expected content\n%s", tc.path, got)
		}
	}
	if strings.Contains(content, "argote:subagent-delegation") {
		t.Fatalf("update with Codex Spark tag used legacy markers\ncontent:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("update wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

func TestUpdateWithoutCodexSparkDelegationTagRemovesGuidanceAndNativeAgents(t *testing.T) {
	requireGitCLI(t)
	sandboxHome := t.TempDir()
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
	preexisting := "before\n\n<!-- dots:codex-spark-delegation -->\ncurrent\n<!-- /dots:codex-spark-delegation -->\n\n<!-- argote:subagent-delegation -->\nlegacy\n<!-- /argote:subagent-delegation -->\n\nafter\n"
	if err := os.WriteFile(codexPath, []byte(preexisting), 0o600); err != nil {
		t.Fatalf("write preexisting Codex instructions: %v", err)
	}
	agentsDir := filepath.Join(sandboxHome, ".codex", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir preexisting Codex agents dir: %v", err)
	}
	for _, name := range []string{"dots-explorer.toml", "dots-worker.toml"} {
		if err := os.WriteFile(filepath.Join(agentsDir, name), []byte("stale dots-owned agent"), 0o600); err != nil {
			t.Fatalf("write preexisting Codex agent %s: %v", name, err)
		}
	}

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})

	out := runUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot, "--tag", "codex-spark-delegation", "--tag", "without-codex-spark-delegation")

	got, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatalf("read Codex instructions: %v", err)
	}
	content := string(got)
	for _, not := range []string{"dots:codex-spark-delegation", "argote:subagent-delegation", "\ncurrent\n", "\nlegacy\n", "gpt-5.3-codex-spark"} {
		if strings.Contains(content, not) {
			t.Fatalf("without Codex Spark tag kept %q\ncontent:\n%s\noutput:\n%s", not, content, out)
		}
	}
	for _, want := range []string{"<!-- dots:rules -->", "Keep diffs surgical", "before", "after"} {
		if !strings.Contains(content, want) {
			t.Fatalf("without Codex Spark tag removed expected %q\ncontent:\n%s", want, content)
		}
	}
	for _, name := range []string{"dots-explorer.toml", "dots-worker.toml"} {
		if _, err := os.Stat(filepath.Join(agentsDir, name)); !os.IsNotExist(err) {
			t.Fatalf("without Codex Spark tag kept native Codex agent %s; stat err = %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("update wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

// TestUpdateDoesNotWriteCodeGraphInstructionBlockAfterSkillsProvisioner proves update
// does not create the dots-owned CodeGraph instruction block for a skills-only
// Codex provisioner.
func TestUpdateDoesNotWriteCodeGraphInstructionBlockAfterSkillsProvisioner(t *testing.T) {
	requireGitCLI(t)
	sandboxHome := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "npx"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

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
provisioners:
  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
      agents: [codex]
      skills: [web-design-guidelines]
      global: true
`,
	})

	out := runUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot)

	if _, err := os.Stat(filepath.Join(sandboxHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("update with skills provisioner wrote CodeGraph instruction block without the codegraph tag; stat err = %v\noutput:\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("update wrote Codex AGENTS.md in inherited HOME %q; stat err = %v", fakeRealHome, err)
	}
}

// TestUpdateCanceledConflictDoesNotRunProvisioners proves canceling conflict
// resolution during a post-update install aborts before any provisioner runs —
// the same applied-gating install enforces.
func TestUpdateCanceledConflictDoesNotRunProvisioners(t *testing.T) {
	requireGitCLI(t)
	sandboxHome := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	t.Setenv("HOME", fakeRealHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nprintf 'ran' > \"$HOME/gentle-ai-ran\"\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})
	// A divergent local target forces a conflict the user then cancels.
	target := filepath.Join(sandboxHome, ".gitconfig")
	if err := os.WriteFile(target, []byte("local\n"), 0o600); err != nil {
		t.Fatalf("write local target: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("q"))
	cmd.SetArgs([]string{"update", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "local\n" {
		t.Fatalf("canceled update changed conflict target to %q", got)
	}
	if _, err := os.Stat(filepath.Join(sandboxHome, "gentle-ai-ran")); !os.IsNotExist(err) {
		t.Fatalf("canceled update ran the provisioner in sandbox HOME; stat err = %v", err)
	}
	if !strings.Contains(out.String(), "Conflict resolution canceled; no changes applied.") {
		t.Fatalf("cancel output missing abort message\noutput:\n%s", out.String())
	}
}
