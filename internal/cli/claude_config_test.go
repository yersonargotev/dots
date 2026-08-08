package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestClaudeSettingsCoOwnedAdditionsDoNotDriftAfterInstall(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	realHome := t.TempDir()
	t.Setenv("HOME", realHome)

	stubDir := t.TempDir()
	writeManifestDependencyStubs(t, stubDir)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "core",
		"--profile", "agents",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})
	if err := install.Execute(); err != nil {
		t.Fatalf("dots install failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	settings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read installed Claude settings: %v", err)
	}
	updated := strings.TrimSuffix(string(settings), "\n}\n") + `,
  "enabledPlugins": {"chrome-devtools-mcp": true},
  "runtimeOnly": "keep"
}
`
	if err := os.WriteFile(settingsPath, []byte(updated), 0o600); err != nil {
		t.Fatalf("write co-owned Claude additions: %v", err)
	}

	statusCmd := cli.NewRootCommand()
	var statusOut bytes.Buffer
	statusCmd.SetOut(&statusOut)
	statusCmd.SetErr(&statusOut)
	statusCmd.SetArgs([]string{
		"status",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "core",
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
		t.Fatalf("status output missing ok Claude settings entry after co-owned additions\noutput:\n%s", got)
	}
	if strings.Contains(got, "drifted      copy      configs/claude/settings.json") ||
		strings.Contains(got, "conflict     copy      configs/claude/settings.json") {
		t.Fatalf("status output reports Claude settings drift/conflict for co-owned additions\noutput:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(realHome, ".claude", "settings.json")); err == nil {
		t.Fatalf("install wrote Claude settings into inherited real HOME %q instead of sandbox", realHome)
	}
}
