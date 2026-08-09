package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
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
  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
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
		"claude plugin marketplace add example/tools",
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
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nprintf 'ok' > \"$HOME/claude-ran\"\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})

	out := runUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", sandboxHome, "--source-root", sourceRoot, "--state-root", stateRoot)

	if _, err := os.Stat(filepath.Join(sandboxHome, "claude-ran")); err != nil {
		t.Fatalf("update did not run the provisioner under the sandbox HOME %q: %v\noutput:\n%s", sandboxHome, err, out)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "claude-ran")); err == nil {
		t.Fatalf("update ran the provisioner under the inherited HOME %q instead of the sandbox", fakeRealHome)
	}
	if !strings.Contains(out, `Provisioner results for profile "default" (tags: core)`) {
		t.Fatalf("update output missing provisioner results report\noutput:\n%s", out)
	}
}

func TestUpdateRetiresHistoricalDelegationAfterSuccessfulApply(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Provisioners: []state.ProvisionerRecord{{
		Tool: "skills", Executable: "npx", Args: []string{"--yes", "skills@1.5.12", "add", "yersonargotev/dots/skills/delegation", "--agent", "codex", "--skill", "delegation", "--global", "--copy"},
	}}}); err != nil {
		t.Fatalf("save historical metadata: %v", err)
	}
	agentsPath := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(agentsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentsPath, []byte("before\n<!-- argote:subagent-delegation -->\nowned\n<!-- /argote:subagent-delegation -->\nafter\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})

	out := runUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"), "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	got, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "argote:subagent-delegation") || !strings.Contains(out, "Delegation retirement:") {
		t.Fatalf("historical retirement was not reported and applied\ninstructions:\n%s\noutput:\n%s", got, out)
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
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nprintf 'ran' > \"$HOME/claude-ran\"\n")
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
	cmd.SetArgs([]string{"update", "--profile", "default", "--file", filepath.Join(sourceRoot, "dots.yaml"),
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
	if _, err := os.Stat(filepath.Join(sandboxHome, "claude-ran")); !os.IsNotExist(err) {
		t.Fatalf("canceled update ran the provisioner in sandbox HOME; stat err = %v", err)
	}
	if !strings.Contains(out.String(), "Conflict resolution canceled; no changes applied.") {
		t.Fatalf("cancel output missing abort message\noutput:\n%s", out.String())
	}
}
