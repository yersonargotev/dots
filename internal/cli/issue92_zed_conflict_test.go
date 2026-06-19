package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestStatusCommandReportsDesktopZedConflictsWithNextActionGuidance(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	writeCLISource(t, sourceRoot, "configs/zed/settings.json", "{\"managed\": true}\n")
	writeCLISource(t, sourceRoot, "configs/zed/keymap.json", "[]\n")
	writeCLISource(t, sourceRoot, "configs/zed/themes/catppuccin-blue.json", "{\"theme\": \"managed\"}\n")
	for rel, content := range map[string]string{
		".config/zed/settings.json":               "{\"local\": true}\n",
		".config/zed/keymap.json":                 "[{\"local\": true}]\n",
		".config/zed/themes/catppuccin-blue.json": "{\"theme\": \"local\"}\n",
	} {
		path := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write target: %v", err)
		}
	}

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  desktop:
    tags: [desktop]
entries:
  - source: configs/zed/settings.json
    target: ~/.config/zed/settings.json
    strategy: symlink
    tags: [desktop]
  - source: configs/zed/keymap.json
    target: ~/.config/zed/keymap.json
    strategy: symlink
    tags: [desktop]
  - source: configs/zed/themes/catppuccin-blue.json
    target: ~/.config/zed/themes/catppuccin-blue.json
    strategy: symlink
    tags: [desktop]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"status", "--profile", "desktop", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("status Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	for _, want := range []string{
		"conflict     symlink   configs/zed/settings.json -> " + filepath.Join(home, ".config/zed/settings.json"),
		"conflict     symlink   configs/zed/keymap.json -> " + filepath.Join(home, ".config/zed/keymap.json"),
		"conflict     symlink   configs/zed/themes/catppuccin-blue.json -> " + filepath.Join(home, ".config/zed/themes/catppuccin-blue.json"),
		"Summary: 0 ok, 0 missing, 3 conflict, 0 skipped, 0 drifted, 0 unsupported",
		"Conflicts mean this profile is only partially managed until you choose a resolution.",
		"skip keeps the local file untouched",
		"replace creates a Backup Set before installing the Source of Truth",
		"adopt is only for supported regular-file conflicts; it copies local file content into the Source of Truth after you choose it",
		"Non-interactive install/update keeps conflicts skipped by default.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestInstallNoTUIOutputExplainsConflictResolutionTradeoffs(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zed/settings.json", "{\"managed\": true}\n")
	target := filepath.Join(home, ".config/zed/settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.WriteFile(target, []byte("{\"local\": true}\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  desktop:
    tags: [desktop]
entries:
  - source: configs/zed/settings.json
    target: ~/.config/zed/settings.json
    strategy: symlink
    tags: [desktop]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("s\n"))
	cmd.SetArgs([]string{"install", "--no-tui", "--profile", "desktop", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	for _, want := range []string{
		"Resolve conflict for " + target,
		"skip keeps the local file untouched",
		"replace creates a Backup Set before installing the Source of Truth",
		"adopt is only for supported regular-file conflicts; it copies local file content into the Source of Truth after you choose it",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("install output missing %q\noutput:\n%s", want, got)
		}
	}
}
