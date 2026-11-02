package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestAgentsProfileInstallsNativeBaselineWithoutAuthorizingHistoricalCleanup(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	stubDir := t.TempDir()
	writeManifestDependencyStubs(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	instructions := filepath.Join(home, ".codex", "AGENTS.md")
	auth := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(instructions), 0o755); err != nil {
		t.Fatalf("mkdir Codex config: %v", err)
	}
	legacy := `user-owned before

<!-- gentle-ai:trigger-rules -->
retired trigger rules
<!-- /gentle-ai:trigger-rules -->

<!-- gentle-ai:engram-protocol -->
retired Engram instructions
<!-- /gentle-ai:engram-protocol -->

<!-- dots:rules -->
retired global rules
<!-- /dots:rules -->

## User instructions
Keep this unmarked user-owned content.
`
	if err := os.WriteFile(instructions, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy instructions: %v", err)
	}
	authContent := []byte(`{"token":"sandbox-secret"}`)
	if err := os.WriteFile(auth, authContent, 0o600); err != nil {
		t.Fatalf("write sandbox auth fixture: %v", err)
	}

	command := cli.NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("agents install failed in sandbox: %v\noutput:\n%s", err, output.String())
	}

	managed := map[string]string{
		filepath.Join(home, ".claude", "settings.json"):         "configs/claude/settings.json",
		filepath.Join(home, ".claude", "statusline-command.sh"): "configs/claude/statusline-command.sh",
		filepath.Join(home, ".codex", "config.toml"):            "configs/codex/config.toml",
	}
	for target, source := range managed {
		got, err := os.ReadFile(target)
		if err != nil {
			t.Errorf("read native Managed Entry %s: %v", target, err)
			continue
		}
		want, err := os.ReadFile(filepath.Join(repoRoot, source))
		if err != nil {
			t.Fatalf("read source %s: %v", source, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("native Managed Entry %s differs from %s", target, source)
		}
	}

	gotInstructions, err := os.ReadFile(instructions)
	if err != nil {
		t.Fatalf("read historical instructions: %v", err)
	}
	if !bytes.Equal(gotInstructions, []byte(legacy)) {
		t.Errorf("agents selection changed historical instructions without Provisioner evidence:\n%s", gotInstructions)
	}
	if strings.Contains(output.String(), "Historical retirement:") {
		t.Errorf("agents selection reported historical retirement without evidence:\n%s", output.String())
	}

	for _, retired := range []string{".copilot/settings.json", ".gemini/antigravity-cli/settings.json", ".config/opencode/opencode.json"} {
		if _, err := os.Lstat(filepath.Join(home, retired)); !os.IsNotExist(err) {
			t.Errorf("agents installed retired target %s: %v", retired, err)
		}
	}

	gotAuth, err := os.ReadFile(auth)
	if err != nil || !bytes.Equal(gotAuth, authContent) {
		t.Errorf("authentication state changed: got %q, err %v", gotAuth, err)
	}
	if _, err := os.Stat(filepath.Join(realHome, ".codex")); !os.IsNotExist(err) {
		t.Errorf("install escaped sandbox home; real HOME stat error = %v", err)
	}
	if strings.Contains(output.String(), "gentle-ai") || strings.Contains(output.String(), "engram") {
		t.Errorf("agents install selected retired dependency or Provisioner:\n%s", output.String())
	}
}
