package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

// TestClaudeDefaultProfileSeedsUserBaselineInSandbox proves that dots seeds the
// user-owned Claude settings baseline and the portable statusLine script with a
// copy strategy (regular files, not symlinks) before the gentle-ai provisioner
// runs, that the provisioner is invoked for the claude-code agent, and that the
// provisioner never escapes the threaded sandbox HOME. The gentle-ai/engram
// tools are stubbed so the provisioner step exits cleanly without merging its
// own keys, keeping the sandbox aligned.
func TestClaudeDefaultProfileSeedsUserBaselineInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	// realHome is the inherited HOME that the sandboxed install must never touch.
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	// The gentle-ai stub records the argv it received (under the threaded HOME)
	// so the test can assert the resolved provisioner command, not just the
	// rendered plan. engram only needs to be present on PATH for the dep check.
	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nprintf '%s' \"$*\" > \"$HOME/gentle-ai-args\"\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "default",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})

	if err := install.Execute(); err != nil {
		t.Fatalf("dots install failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}

	settingsTarget := filepath.Join(home, ".claude", "settings.json")
	scriptTarget := filepath.Join(home, ".claude", "statusline-command.sh")

	// The settings baseline and statusLine script are copied (not symlinked) so
	// gentle-ai can merge its own keys without writing back into the repo.
	managed := []struct {
		target string
		source string
	}{
		{target: settingsTarget, source: filepath.Join(repoRoot, "configs", "claude", "settings.json")},
		{target: scriptTarget, source: filepath.Join(repoRoot, "configs", "claude", "statusline-command.sh")},
	}
	for _, m := range managed {
		info, err := os.Lstat(m.target)
		if err != nil {
			t.Fatalf("claude target missing after sandbox install: %v\ninstall output:\n%s", err, installOut.String())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("claude target %q is a symlink, want a regular copied file", m.target)
		}
		got, err := os.ReadFile(m.target)
		if err != nil {
			t.Fatalf("read copied claude target %q: %v", m.target, err)
		}
		want, err := os.ReadFile(m.source)
		if err != nil {
			t.Fatalf("read claude source %q: %v", m.source, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("copied claude target %q content differs from source %q", m.target, m.source)
		}
	}

	// The statusLine script must stay executable so Claude Code can run it.
	scriptInfo, err := os.Stat(scriptTarget)
	if err != nil {
		t.Fatalf("stat copied statusline script: %v", err)
	}
	if scriptInfo.Mode()&0o111 == 0 {
		t.Fatalf("copied statusline script %q is not executable (mode %v)", scriptTarget, scriptInfo.Mode())
	}

	settings, err := os.ReadFile(settingsTarget)
	if err != nil {
		t.Fatalf("read seeded settings.json: %v", err)
	}

	// The baseline must version every user-owned key — guard against shipping an
	// empty or gutted file that would still pass the forbidden-key checks below.
	var parsed struct {
		Model                 *string         `json:"model"`
		Theme                 *string         `json:"theme"`
		EditorMode            *string         `json:"editorMode"`
		Env                   json.RawMessage `json:"env"`
		AgentPushNotifEnabled *bool           `json:"agentPushNotifEnabled"`
		StatusLine            json.RawMessage `json:"statusLine"`
		Permissions           *struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(settings, &parsed); err != nil {
		t.Fatalf("seeded settings.json is not valid JSON: %v\ncontent:\n%s", err, settings)
	}
	switch {
	case parsed.Model == nil:
		t.Fatalf("seeded settings.json missing user-owned key %q\ncontent:\n%s", "model", settings)
	case parsed.Theme == nil:
		t.Fatalf("seeded settings.json missing user-owned key %q\ncontent:\n%s", "theme", settings)
	case parsed.EditorMode == nil:
		t.Fatalf("seeded settings.json missing user-owned key %q\ncontent:\n%s", "editorMode", settings)
	case len(parsed.Env) == 0:
		t.Fatalf("seeded settings.json missing user-owned key %q\ncontent:\n%s", "env", settings)
	case parsed.AgentPushNotifEnabled == nil:
		t.Fatalf("seeded settings.json missing user-owned key %q\ncontent:\n%s", "agentPushNotifEnabled", settings)
	case len(parsed.StatusLine) == 0:
		t.Fatalf("seeded settings.json missing user-owned key %q\ncontent:\n%s", "statusLine", settings)
	case parsed.Permissions == nil || len(parsed.Permissions.Allow) == 0:
		t.Fatalf("seeded settings.json missing user-owned key %q\ncontent:\n%s", "permissions.allow", settings)
	}

	// The baseline must not version any gentle-ai-managed or runtime-state key.
	// Keys are matched in their quoted JSON-token form to avoid substring
	// collisions with allowed values (e.g. an MCP tool name containing "hooks").
	for _, forbidden := range []string{
		"\"deny\"", "\"hooks\"", "\"outputStyle\"", "\"enabledPlugins\"",
		"\"extraKnownMarketplaces\"", "\"defaultMode\"",
		"\"skipDangerousModePermissionPrompt\"",
	} {
		if strings.Contains(string(settings), forbidden) {
			t.Fatalf("seeded settings.json must not version %s\ncontent:\n%s", forbidden, settings)
		}
	}
	// The statusLine command must be portable (no hardcoded absolute home path).
	if !strings.Contains(string(settings), "bash ~/.claude/statusline-command.sh") {
		t.Fatalf("seeded settings.json statusLine command is not normalized to ~\ncontent:\n%s", settings)
	}

	// The provisioner must have run, threaded to the sandbox HOME, with both
	// agents resolved on its argv — not merely shown in the rendered plan.
	gotArgs, err := os.ReadFile(filepath.Join(home, "gentle-ai-args"))
	if err != nil {
		t.Fatalf("provisioner did not run under the sandbox HOME %q: %v", home, err)
	}
	if !strings.Contains(string(gotArgs), "--agents codex,claude-code") {
		t.Fatalf("provisioner argv = %q, want it to install agents codex,claude-code", gotArgs)
	}
	// And it must never have escaped into the inherited real HOME.
	if _, err := os.Stat(filepath.Join(realHome, "gentle-ai-args")); err == nil {
		t.Fatalf("provisioner wrote into the inherited HOME %q instead of the sandbox", realHome)
	}

	out := installOut.String()
	for _, want := range []string{
		"configs/claude/settings.json",
		"configs/claude/statusline-command.sh",
		"codex,claude-code",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("install output missing %q\noutput:\n%s", want, out)
		}
	}
}
