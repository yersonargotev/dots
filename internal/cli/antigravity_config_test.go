package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

func TestWorkstationAndMobileComposeAntigravitySettingsInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	t.Setenv("HOME", t.TempDir())
	stubManifestProvisionerTools(t)

	install := cli.NewRootCommand()
	var out bytes.Buffer
	install.SetOut(&out)
	install.SetErr(&out)
	install.SetArgs([]string{
		"install", "--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "workstation", "--profile", "mobile", "--skip-deps",
		"--source-root", repoRoot, "--home", home, "--state-root", stateRoot, "--yes",
	})
	if err := install.Execute(); err != nil {
		t.Fatalf("composed install failed: %v\noutput:\n%s", err, out.String())
	}

	target := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read composed target: %v", err)
	}
	for _, want := range []string{`"toolPermission"`, `"allowNonWorkspaceAccess"`, `"dart-mcp-server"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("composed target missing %s:\n%s", want, data)
		}
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok {
		t.Fatal("metadata missing composed Antigravity target")
	}
	wantSources := []string{"configs/antigravity/settings.json", "configs/antigravity/mobile-mcp-settings.json"}
	if !reflect.DeepEqual(rec.SourceList(), wantSources) {
		t.Fatalf("metadata sources = %#v, want %#v", rec.SourceList(), wantSources)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"workstation", "mobile"}) {
		t.Fatalf("installed selection = %+v, want workstation + mobile", meta.InstalledSelection)
	}

	statusCmd := cli.NewRootCommand()
	var statusOut bytes.Buffer
	statusCmd.SetOut(&statusOut)
	statusCmd.SetErr(&statusOut)
	statusCmd.SetArgs([]string{
		"status", "--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "workstation", "--profile", "mobile",
		"--source-root", repoRoot, "--home", home, "--state-root", stateRoot,
	})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("status failed for composed install: %v\noutput:\n%s", err, statusOut.String())
	}
	for _, source := range wantSources {
		if !strings.Contains(statusOut.String(), "ok") || !strings.Contains(statusOut.String(), source) {
			t.Fatalf("status does not prove contributor %s aligned:\n%s", source, statusOut.String())
		}
	}
}

func TestWorkstationAndMobileExpandLegacyAntigravityMetadataInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	t.Setenv("HOME", t.TempDir())
	stubManifestProvisionerTools(t)

	source := filepath.Join(repoRoot, "configs", "antigravity", "settings.json")
	baseline, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}
	var targetJSON map[string]any
	if err := json.Unmarshal(baseline, &targetJSON); err != nil {
		t.Fatalf("parse baseline: %v", err)
	}
	targetJSON["runtimeOnly"] = "keep"
	targetData, err := json.Marshal(targetJSON)
	if err != nil {
		t.Fatalf("encode legacy target: %v", err)
	}
	target := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, targetData, 0o600); err != nil {
		t.Fatalf("write legacy target: %v", err)
	}
	hash, err := state.HashFile(source)
	if err != nil {
		t.Fatalf("hash baseline: %v", err)
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: 2, Entries: []state.Record{{
		Target: target, Source: "configs/antigravity/settings.json", Strategy: "copy", Hash: hash,
		Profiles: []string{"workstation"}, Tags: []string{"agents"},
	}}}); err != nil {
		t.Fatalf("save legacy metadata: %v", err)
	}

	install := cli.NewRootCommand()
	var out bytes.Buffer
	install.SetOut(&out)
	install.SetErr(&out)
	install.SetArgs([]string{
		"install", "--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "workstation", "--profile", "mobile", "--skip-deps",
		"--source-root", repoRoot, "--home", home, "--state-root", stateRoot, "--yes",
	})
	if err := install.Execute(); err != nil {
		t.Fatalf("legacy expansion install failed: %v\noutput:\n%s", err, out.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read expanded target: %v", err)
	}
	for _, want := range []string{`"runtimeOnly": "keep"`, `"dart-mcp-server"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("expanded target missing %s:\n%s", want, got)
		}
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load migrated metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok || !rec.HasSource("configs/antigravity/mobile-mcp-settings.json") {
		t.Fatalf("migrated target record = %+v, want mobile contributor", rec)
	}
	if meta.Version != state.CurrentVersion || meta.InstalledSelection == nil ||
		!reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"workstation", "mobile"}) {
		t.Fatalf("migrated metadata = %+v, want terminal v%d selection", meta, state.CurrentVersion)
	}
}

// TestAntigravityAgentsProfileSeedsUserBaselineInSandbox proves that dots seeds the
// user-owned Antigravity settings baseline with a copy strategy (regular files, not symlinks)
// before any Provisioner runs and that the settings contain the expected baseline keys.
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
		MCPServers              map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
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
	if _, ok := parsed.MCPServers["dart-mcp-server"]; ok {
		t.Fatalf("agents profile must not include mobile Dart MCP in Antigravity settings")
	}
}

// TestAntigravityMobileProfileSeedsOnlyDartMCPInSandbox proves the mobile profile
// owns only the Dart/Flutter MCP fragment, not the broad agents baseline.
func TestAntigravityMobileProfileSeedsOnlyDartMCPInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	stubDir := t.TempDir()
	for _, name := range []string{"npx", "claude", "codex", "dart"} {
		writeExecStub(t, filepath.Join(stubDir, name), "#!/bin/sh\nexit 0\n")
	}
	writeManifestDependencyStubs(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "mobile",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})

	if err := install.Execute(); err != nil {
		t.Fatalf("dots install failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}

	settingsTarget := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	got, err := os.ReadFile(settingsTarget)
	if err != nil {
		t.Fatalf("read copied antigravity mobile target %q: %v", settingsTarget, err)
	}

	var parsed struct {
		ToolPermission          *string `json:"toolPermission"`
		AllowNonWorkspaceAccess *bool   `json:"allowNonWorkspaceAccess"`
		EnableTerminalSandbox   *bool   `json:"enableTerminalSandbox"`
		MCPServers              map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("seeded mobile settings.json is not valid JSON: %v", err)
	}

	if parsed.ToolPermission != nil || parsed.AllowNonWorkspaceAccess != nil || parsed.EnableTerminalSandbox != nil {
		t.Fatalf("mobile Antigravity settings must not manage broad baseline keys: %s", string(got))
	}
	dartMCP, ok := parsed.MCPServers["dart-mcp-server"]
	if !ok {
		t.Fatalf("mcpServers.dart-mcp-server missing from mobile Antigravity settings")
	}
	if dartMCP.Command != "dart" || !reflect.DeepEqual(dartMCP.Args, []string{"mcp-server"}) {
		t.Fatalf("mcpServers.dart-mcp-server = %#v, want command dart with args [mcp-server]", dartMCP)
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
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nexit 0\n")
	writeManifestDependencyStubs(t, stubDir)
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
