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

// TestClaudeAgentsProfileSeedsUserBaselineInSandbox proves that dots seeds the
// user-owned Claude settings baseline and the portable statusLine script with a
// copy strategy (regular files, not symlinks) before the gentle-ai provisioner
// runs, that the provisioner is invoked for the claude-code agent, and that the
// provisioner never escapes the threaded sandbox HOME. The gentle-ai/engram
// tools are stubbed so the provisioner step exits cleanly without merging its
// own keys, keeping the sandbox aligned.
func TestClaudeAgentsProfileSeedsUserBaselineInSandbox(t *testing.T) {
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
	// and simulates stale trigger-rules output so the test can assert cleanup
	// across the supported agent instruction files. engram only needs to be
	// present on PATH for the dep check.
	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), `#!/bin/sh
printf '%s\n' "$*" >> "$HOME/gentle-ai-args"
block='before

<!-- gentle-ai:persona -->
## Personality
Senior Architect, 15+ years experience, GDE & MVP.
<!-- /gentle-ai:persona -->

<!-- gentle-ai:trigger-rules -->
stale review-readability rule
<!-- /gentle-ai:trigger-rules -->

after
'
mkdir -p "$HOME/.codex" "$HOME/.claude" "$HOME/.config/opencode" "$HOME/.gemini" "$HOME/Library/Application Support/Code/User/prompts" "$HOME/.config/Code/User/prompts"
printf '%s' "$block" > "$HOME/.codex/AGENTS.md"
printf '%s' "$block" > "$HOME/.claude/CLAUDE.md"
printf '%s' "$block" > "$HOME/.config/opencode/AGENTS.md"
printf '%s' "$block" > "$HOME/.gemini/GEMINI.md"
printf '%s' "$block" > "$HOME/Library/Application Support/Code/User/prompts/gentle-ai.instructions.md"
printf '%s' "$block" > "$HOME/.config/Code/User/prompts/gentle-ai.instructions.md"
`)
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HOME/codegraph-args\"\n")
	writeManifestDependencyStubs(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
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
	codexConfigTarget := filepath.Join(home, ".codex", "config.toml")
	copilotSettingsTarget := filepath.Join(home, ".copilot", "settings.json")
	copilotScriptTarget := filepath.Join(home, ".copilot", "statusline-command.sh")

	// The settings baseline and statusLine script are copied (not symlinked) so
	// gentle-ai can merge its own keys without writing back into the repo.
	managed := []struct {
		target string
		source string
	}{
		{target: settingsTarget, source: filepath.Join(repoRoot, "configs", "claude", "settings.json")},
		{target: scriptTarget, source: filepath.Join(repoRoot, "configs", "claude", "statusline-command.sh")},
		{target: codexConfigTarget, source: filepath.Join(repoRoot, "configs", "codex", "config.toml")},
		{target: copilotSettingsTarget, source: filepath.Join(repoRoot, "configs", "copilot", "settings.json")},
		{target: copilotScriptTarget, source: filepath.Join(repoRoot, "configs", "copilot", "statusline-command.sh")},
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
	copilotScriptInfo, err := os.Stat(copilotScriptTarget)
	if err != nil {
		t.Fatalf("stat copied copilot statusline script: %v", err)
	}
	if copilotScriptInfo.Mode()&0o111 == 0 {
		t.Fatalf("copied copilot statusline script %q is not executable (mode %v)", copilotScriptTarget, copilotScriptInfo.Mode())
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
	// dots must not version the gentle-ai-rendered opencode.json (Regenerated
	// Content). A hand-authored dots overlay fragment under configs/opencode/ is
	// allowed: OpenCode merges it via OPENCODE_CONFIG, it is never the rendered
	// config.
	if _, err := os.Stat(filepath.Join(repoRoot, "configs", "opencode", "opencode.json")); !os.IsNotExist(err) {
		t.Fatalf("repo must not version the gentle-ai-rendered configs/opencode/opencode.json: %v", err)
	}

	// The provisioner must have run, threaded to the sandbox HOME, with the
	// basic non-SDD agent set resolved on its argv — not merely shown in the
	// rendered plan.
	gotArgs, err := os.ReadFile(filepath.Join(home, "gentle-ai-args"))
	if err != nil {
		t.Fatalf("provisioner did not run under the sandbox HOME %q: %v", home, err)
	}
	if !strings.Contains(string(gotArgs), "uninstall --agents codex,claude-code,opencode,antigravity,vscode-copilot --components sdd,persona --yes") {
		t.Fatalf("provisioner argv = %q, want it to cleanup legacy SDD and persona for codex,claude-code,opencode,antigravity,vscode-copilot before install", gotArgs)
	}
	if !strings.Contains(string(gotArgs), "install --scope global --channel stable --preset custom --agents codex --components engram,context7") {
		t.Fatalf("provisioner argv = %q, want codex install without permissions", gotArgs)
	}
	if !strings.Contains(string(gotArgs), "install --scope global --channel stable --preset custom --agents claude-code --components engram,context7,permissions") {
		t.Fatalf("provisioner argv = %q, want claude-code install with permissions", gotArgs)
	}
	if !strings.Contains(string(gotArgs), "install --scope global --channel stable --preset custom --agents antigravity --components engram,context7") {
		t.Fatalf("provisioner argv = %q, want antigravity install without SDD or permissions", gotArgs)
	}
	if !strings.Contains(string(gotArgs), "install --scope global --channel stable --preset custom --agents opencode --components engram,context7") {
		t.Fatalf("provisioner argv = %q, want opencode install without SDD or permissions", gotArgs)
	}
	if !strings.Contains(string(gotArgs), "install --scope global --channel stable --preset custom --agents vscode-copilot --components engram,context7") {
		t.Fatalf("provisioner argv = %q, want vscode-copilot install without SDD or permissions", gotArgs)
	}
	for _, forbidden := range []string{
		"--agents codex --components engram,context7,persona",
		"--agents claude-code --components engram,context7,persona",
		"--agents antigravity --components engram,context7,persona",
		"--agents opencode --components engram,context7,persona",
		"--agents vscode-copilot --components engram,context7,persona",
	} {
		if strings.Contains(string(gotArgs), forbidden) {
			t.Fatalf("provisioner argv = %q, default agents profile must not install persona component %q", gotArgs, forbidden)
		}
	}
	if strings.Contains(string(gotArgs), "--agents codex --components engram,context7,permissions") {
		t.Fatalf("provisioner argv = %q, want codex install without permissions because it installs gentle-dev", gotArgs)
	}
	if strings.Contains(string(gotArgs), "--agents antigravity --components engram,context7,permissions") || strings.Contains(string(gotArgs), "--agents antigravity --components engram,context7,sdd") {
		t.Fatalf("provisioner argv = %q, want antigravity install without SDD or permissions", gotArgs)
	}
	if strings.Contains(string(gotArgs), "--agents opencode --components engram,context7,permissions") || strings.Contains(string(gotArgs), "--agents opencode --components engram,context7,sdd") {
		t.Fatalf("provisioner argv = %q, want opencode install without SDD or permissions", gotArgs)
	}
	if strings.Contains(string(gotArgs), "--agents vscode-copilot --components engram,context7,permissions") || strings.Contains(string(gotArgs), "--agents vscode-copilot --components engram,context7,sdd") {
		t.Fatalf("provisioner argv = %q, want vscode-copilot install without SDD or permissions", gotArgs)
	}
	if strings.Contains(string(gotArgs), "install --scope global --channel stable --persona neutral --preset custom --agents codex,claude-code,opencode") {
		t.Fatalf("provisioner argv = %q, want basic installs split per agent", gotArgs)
	}
	if _, err := os.ReadFile(filepath.Join(home, "codegraph-args")); !os.IsNotExist(err) {
		t.Fatalf("agents profile should not run CodeGraph without --tag codegraph: %v", err)
	}
	for _, path := range []string{
		filepath.Join(home, ".codex", "AGENTS.md"),
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		filepath.Join(home, ".gemini", "GEMINI.md"),
		filepath.Join(home, "Library", "Application Support", "Code", "User", "prompts", "gentle-ai.instructions.md"),
		filepath.Join(home, ".config", "Code", "User", "prompts", "gentle-ai.instructions.md"),
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read cleaned agent instructions %s: %v", path, err)
		}
		for _, forbidden := range []string{"gentle-ai:trigger-rules", "review-readability", "gentle-ai:persona", "Senior Architect"} {
			if strings.Contains(string(got), forbidden) {
				t.Fatalf("agent instructions %s kept stale gentle-ai content %q\ncontent:\n%s", path, forbidden, got)
			}
		}
		for _, want := range []string{"<!-- dots:rules -->", "Keep diffs surgical", "Verify before declaring success", "Use sandboxed HOME/config paths"} {
			if !strings.Contains(string(got), want) {
				t.Fatalf("agent instructions %s missing dots rules content %q\ncontent:\n%s", path, want, got)
			}
		}
	}
	// And it must never have escaped into the inherited real HOME.
	if _, err := os.Stat(filepath.Join(realHome, "gentle-ai-args")); err == nil {
		t.Fatalf("provisioner wrote into the inherited HOME %q instead of the sandbox", realHome)
	}
	if _, err := os.Stat(filepath.Join(realHome, "codegraph-args")); err == nil {
		t.Fatalf("codegraph provisioner wrote into the inherited HOME %q instead of the sandbox", realHome)
	}
	if _, err := os.Stat(filepath.Join(realHome, ".config", "opencode", "opencode.json")); err == nil {
		t.Fatalf("OpenCode generated config escaped into the inherited HOME %q", realHome)
	}

	out := installOut.String()
	for _, want := range []string{
		"configs/claude/settings.json",
		"configs/claude/statusline-command.sh",
		"--agents codex --components engram,context7",
		"--agents claude-code --components engram,context7,permissions",
		"--agents antigravity --components engram,context7",
		"--agents opencode --components engram,context7",
		"--agents vscode-copilot --components engram,context7",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("install output missing %q\noutput:\n%s", want, out)
		}
	}
	if strings.Contains(out, "--components engram,context7,persona") {
		t.Fatalf("install output must not show persona in the default agents baseline\noutput:\n%s", out)
	}
}

// TestClaudeAgentsPersonaTagDoesNotInstallPersona proves that the manifest
// schema still accepts the persona tag, but the repository no longer uses it to
// install gentle-ai persona content.
func TestClaudeAgentsPersonaTagDoesNotInstallPersona(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HOME/gentle-ai-args\"\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$HOME/codegraph-args\"\n")
	writeManifestDependencyStubs(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
		"--tag", "persona",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})

	if err := install.Execute(); err != nil {
		t.Fatalf("dots install --tag persona failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}

	gotArgs, err := os.ReadFile(filepath.Join(home, "gentle-ai-args"))
	if err != nil {
		t.Fatalf("gentle-ai provisioner did not run under the sandbox HOME %q: %v", home, err)
	}
	wantPersona := "install --scope global --channel stable --persona neutral --preset custom --agents codex,claude-code,opencode,antigravity,vscode-copilot --components persona"
	if strings.Contains(string(gotArgs), wantPersona) || strings.Contains(string(gotArgs), "--components persona") && strings.Contains(string(gotArgs), "install ") {
		t.Fatalf("provisioner argv = %q, repository must not install persona even with --tag persona", gotArgs)
	}
	if _, err := os.Stat(filepath.Join(realHome, "gentle-ai-args")); err == nil {
		t.Fatalf("persona provisioner wrote into the inherited HOME %q instead of the sandbox", realHome)
	}
	if _, err := os.Stat(filepath.Join(realHome, ".claude", "settings.json")); err == nil {
		t.Fatalf("persona-tag install wrote Claude settings into the inherited HOME %q", realHome)
	}
}

func TestClaudeSettingsProvisionerAdditionsDoNotDriftAfterInstall(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), `#!/bin/sh
mkdir -p "$HOME/.claude"
cat > "$HOME/.claude/settings.json" <<'JSON'
{
  "agentPushNotifEnabled": true,
  "editorMode": "vim",
  "env": {
    "ENABLE_TOOL_SEARCH": "true",
    "CLAUDE_CODE_ENABLE_TELEMETRY": "0"
  },
  "model": "opus",
  "permissions": {
    "allow": [
      "mcp__codegraph__codegraph_search",
      "mcp__codegraph__codegraph_context",
      "mcp__codegraph__codegraph_callers",
      "mcp__codegraph__codegraph_callees",
      "mcp__codegraph__codegraph_impact",
      "mcp__codegraph__codegraph_node",
      "mcp__codegraph__codegraph_status",
      "mcp__chrome-devtools__new_page"
    ],
    "deny": ["Bash(rm -rf *)"]
  },
  "statusLine": {
    "command": "bash ~/.claude/statusline-command.sh",
    "type": "command"
  },
  "theme": "dark-ansi",
  "enabledPlugins": {
    "chrome-devtools-mcp": true
  }
}
JSON
`)
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nexit 0\n")
	writeManifestDependencyStubs(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})
	if err := install.Execute(); err != nil {
		t.Fatalf("dots install failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}

	statusCmd := cli.NewRootCommand()
	var statusOut bytes.Buffer
	statusCmd.SetOut(&statusOut)
	statusCmd.SetErr(&statusOut)
	statusCmd.SetArgs([]string{
		"status",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
	})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("dots status failed in sandbox: %v\noutput:\n%s", err, statusOut.String())
	}

	got := statusOut.String()
	if !strings.Contains(got, "ok           copy      configs/claude/settings.json") {
		t.Fatalf("status output missing ok Claude settings entry after provisioner additions\noutput:\n%s", got)
	}
	if strings.Contains(got, "drifted      copy      configs/claude/settings.json") ||
		strings.Contains(got, "conflict     copy      configs/claude/settings.json") {
		t.Fatalf("status output reports Claude settings drift/conflict for expected provisioner additions\noutput:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(realHome, ".claude", "settings.json")); err == nil {
		t.Fatalf("provisioner wrote Claude settings into inherited real HOME %q instead of sandbox", realHome)
	}
}
