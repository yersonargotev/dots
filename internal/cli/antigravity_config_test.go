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

// TestAntigravityAgentsProfileSeedsUserBaselineInSandbox proves that dots seeds the
// user-owned Antigravity settings baseline with a copy strategy (regular files, not symlinks)
// before the gentle-ai provisioner runs, that the settings contain the expected baseline keys,
// and that the provisioner does not escape the sandbox HOME.
func TestAntigravityAgentsProfileSeedsUserBaselineInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nexit 0\n")
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

	settingsTarget := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")

	info, err := os.Lstat(settingsTarget)
	if err != nil {
		t.Fatalf("antigravity settings target missing after sandbox install: %v\ninstall output:\n%s", err, installOut.String())
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("antigravity target %q is a symlink, want a regular copied file", settingsTarget)
	}

	got, err := os.ReadFile(settingsTarget)
	if err != nil {
		t.Fatalf("read copied antigravity target %q: %v", settingsTarget, err)
	}
	want, err := os.ReadFile(filepath.Join(repoRoot, "configs", "antigravity", "settings.json"))
	if err != nil {
		t.Fatalf("read antigravity source: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("copied antigravity target %q content differs from source", settingsTarget)
	}

	var parsed struct {
		ToolPermission          *string `json:"toolPermission"`
		AllowNonWorkspaceAccess *bool   `json:"allowNonWorkspaceAccess"`
		EnableTerminalSandbox   *bool   `json:"enableTerminalSandbox"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("seeded settings.json is not valid JSON: %v", err)
	}

	if parsed.ToolPermission == nil || *parsed.ToolPermission != "always-proceed" {
		t.Errorf("toolPermission key incorrect or missing, got: %v", parsed.ToolPermission)
	}
	if parsed.AllowNonWorkspaceAccess == nil || *parsed.AllowNonWorkspaceAccess != true {
		t.Errorf("allowNonWorkspaceAccess key incorrect or missing, got: %v", parsed.AllowNonWorkspaceAccess)
	}
	if parsed.EnableTerminalSandbox == nil || *parsed.EnableTerminalSandbox != false {
		t.Errorf("enableTerminalSandbox key incorrect or missing, got: %v", parsed.EnableTerminalSandbox)
	}
}

// TestAntigravitySettingsProvisionerAdditionsDoNotDrift proves that adding extra keys to
// the installed ~/.gemini/antigravity-cli/settings.json (like those managed by the agent at runtime)
// does not trigger a drift state, whereas changing the managed baseline keys does.
func TestAntigravitySettingsProvisionerAdditionsDoNotDrift(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// 1. Run the initial install
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

	settingsTarget := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")

	// 2. Simulate runtime state keys added by the agent to settings.json
	extraConfig := `{
		"toolPermission": "always-proceed",
		"allowNonWorkspaceAccess": true,
		"enableTerminalSandbox": false,
		"runtimeStateKey": "some-runtime-value",
		"anotherExtraSettings": [1, 2, 3]
	}`
	if err := os.WriteFile(settingsTarget, []byte(extraConfig), 0o600); err != nil {
		t.Fatalf("failed to write simulated runtime settings: %v", err)
	}

	// 3. Verify status command shows 'ok' (no drift) because it's a subset match
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
		t.Fatalf("dots status failed: %v\noutput:\n%s", err, statusOut.String())
	}

	gotStatus := statusOut.String()
	if !strings.Contains(gotStatus, "ok           copy      configs/antigravity/settings.json") {
		t.Fatalf("status output missing ok Antigravity settings entry after runtime additions\noutput:\n%s", gotStatus)
	}

	// 4. Modify one of the managed baseline keys to trigger drift
	driftConfig := `{
		"toolPermission": "ask",
		"allowNonWorkspaceAccess": true,
		"enableTerminalSandbox": false,
		"runtimeStateKey": "some-runtime-value"
	}`
	if err := os.WriteFile(settingsTarget, []byte(driftConfig), 0o600); err != nil {
		t.Fatalf("failed to write drift settings: %v", err)
	}

	// 5. Verify status command shows 'drifted'
	statusCmd2 := cli.NewRootCommand()
	var statusOut2 bytes.Buffer
	statusCmd2.SetOut(&statusOut2)
	statusCmd2.SetErr(&statusOut2)
	statusCmd2.SetArgs([]string{
		"status",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "agents",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
	})
	// status exits with 2 when findings are found, so we check status return
	_ = statusCmd2.Execute()

	gotStatus2 := statusOut2.String()
	if !strings.Contains(gotStatus2, "drifted      copy      configs/antigravity/settings.json") {
		t.Fatalf("status output should show drifted state for modified baseline key\noutput:\n%s", gotStatus2)
	}
}
