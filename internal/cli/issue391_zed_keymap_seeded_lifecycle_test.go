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

func TestZedKeymapMigratesToSeededStateAndPreservesOrderedLocalEvolution(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".dots-state")
	t.Setenv("HOME", t.TempDir())

	legacyManifest := `version: 1
profiles:
  default:
    tags: [desktop]
entries:
  - source: configs/zed/keymap.json
    target: ~/.config/zed/keymap.json
    strategy: symlink
    tags: [desktop]
`
	seededManifest := `version: 1
profiles:
  default:
    tags: [desktop]
entries:
  - source: configs/zed/keymap.json
    target: ~/.config/zed/keymap.json
    strategy: copy
    ownership: seeded
    tags: [desktop]
`
	oldBaseline := `// Ordered Zed baseline.
[
  { "context": "Workspace", "bindings": { "cmd-a": "action::First", }, },
  { "context": "Workspace", "bindings": { "cmd-a": "action::Second", }, },
]
`
	newBaseline := `// Updated ordered Zed baseline.
[
  { "context": "Workspace", "bindings": { "cmd-a": "action::First", }, },
  { "context": "Editor", "bindings": { "cmd-b": "action::Current", }, },
]
`
	localEvolution := `// Edited by Zed; later bindings retain precedence.
[
  { "context": "Workspace", "bindings": { "cmd-a": "action::Second", }, },
  { "context": "Workspace", "bindings": { "cmd-a": "action::Local", }, },
]
`
	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zed/keymap.json": oldBaseline,
		"dots.yaml":               legacyManifest,
	})
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")

	run := func(want int, args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := cli.Run(args, &stdout, &stderr)
		if code != want {
			t.Fatalf("cli.Run(%v) code = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, want, stdout.String(), stderr.String())
		}
		return stdout.String()
	}
	common := []string{"--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}
	run(0, append([]string{"install", "--yes", "--skip-deps"}, common...)...)

	target := filepath.Join(home, ".config", "zed", "keymap.json")
	if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("legacy Zed keymap = (%v, %v), want symlink", info, err)
	}
	if err := os.WriteFile(target, []byte(localEvolution), 0o600); err != nil {
		t.Fatalf("simulate Zed Keymap Editor write: %v", err)
	}
	advanceUpstream(t, origin, "seed ordered Zed keymap", map[string]string{
		"configs/zed/keymap.json": newBaseline,
		"dots.yaml":               seededManifest,
	})

	updateOutput := runUpdate(t, "--yes", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if !strings.Contains(updateOutput, "migrate") {
		t.Fatalf("update output missing seeded migration evidence:\n%s", updateOutput)
	}
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("migrated Zed keymap = (%v, %v), want regular file", info, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != localEvolution {
		t.Fatalf("migrated keymap = (%q, %v), want byte-exact local ordering", got, err)
	}
	if dirty := strings.TrimSpace(runGitOutput(t, sourceRoot, "status", "--porcelain")); dirty != "" {
		t.Fatalf("simulated Zed edit dirtied the Installed Repository after migration: %q", dirty)
	}

	statusOutput := run(0, append([]string{"status", "--output", "json"}, common...)...)
	if !strings.Contains(statusOutput, `"state": "ok"`) || !strings.Contains(statusOutput, `"reason": "seeded-local-evolution"`) {
		t.Fatalf("status did not report aligned local evolution:\n%s", statusOutput)
	}
	run(0, append([]string{"install", "--yes", "--skip-deps"}, common...)...)
	if got, err := os.ReadFile(target); err != nil || string(got) != localEvolution {
		t.Fatalf("install changed locally evolved keymap = (%q, %v)", got, err)
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load seeded metadata: %v", err)
	}
	record, ok := meta.FindByTarget(target)
	if !ok || record.Ownership != "seeded" || string(record.SeededBaseline) != oldBaseline {
		t.Fatalf("seeded metadata = %#v, want previous byte-exact baseline", record)
	}

	if err := os.WriteFile(target, []byte(oldBaseline), 0o600); err != nil {
		t.Fatalf("restore untouched prior seed: %v", err)
	}
	run(0, append([]string{"install", "--yes", "--skip-deps"}, common...)...)
	if got, err := os.ReadFile(target); err != nil || string(got) != newBaseline {
		t.Fatalf("untouched seeded keymap did not advance = (%q, %v)", got, err)
	}

	uninstallOutput := run(0, "uninstall", "--yes", "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if !strings.Contains(uninstallOutput, "Retained Seeded Runtime State") {
		t.Fatalf("uninstall output missing retained-state evidence:\n%s", uninstallOutput)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != newBaseline {
		t.Fatalf("uninstall changed Zed keymap = (%q, %v)", got, err)
	}
}
