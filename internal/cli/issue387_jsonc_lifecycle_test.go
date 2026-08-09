package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

func TestJSONCSubsetLifecyclePreservesZedContentAndUninstallsOwnedContribution(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	t.Setenv("HOME", t.TempDir())

	source := filepath.Join(sourceRoot, "configs", "zed", "settings.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	previous := `{
  // Portable Zed baseline.
  "theme": "dark",
  "languages": {
    "Go": { "format_on_save": "on", },
  },
  "features": ["one", "two"],
}
`
	if err := os.WriteFile(source, []byte(previous), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	manifestData := `version: 1
profiles:
  default:
    tags: [desktop]
entries:
  - source: configs/zed/settings.json
    target: ~/.config/zed/settings.json
    strategy: copy
    ownership: jsonc-subset
    tags: [desktop]
`
	if err := os.WriteFile(manifestPath, []byte(manifestData), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

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
	installArgs := append([]string{"install", "--yes", "--skip-deps"}, common...)
	statusArgs := append([]string{"status", "--output", "json"}, common...)
	doctorArgs := append([]string{"doctor", "--output", "json"}, common...)
	run(0, installArgs...)

	target := filepath.Join(home, ".config", "zed", "settings.json")
	zedEdited := `{
  // Portable Zed baseline.
  "theme": "dark",
  "languages": {
    "Go": { "format_on_save": "on", },
    "Rust": { "language_servers": ["rust-analyzer"], },
  },
  "features": ["one", "two"],
  // Written by Zed and not owned by dots.
  "runtime": true,
}
`
	if err := os.WriteFile(target, []byte(zedEdited), 0o640); err != nil {
		t.Fatalf("simulate Zed write: %v", err)
	}
	run(0, statusArgs...)
	run(0, doctorArgs...)

	orderedArrayDrift := strings.Replace(zedEdited, `["one", "two"]`, `["two", "one"]`, 1)
	if err := os.WriteFile(target, []byte(orderedArrayDrift), 0o640); err != nil {
		t.Fatalf("write ordered array drift: %v", err)
	}
	run(2, statusArgs...)
	if err := os.WriteFile(target, []byte(zedEdited), 0o640); err != nil {
		t.Fatalf("restore compatible target: %v", err)
	}

	current := `{
  // Portable Zed baseline.
  "languages": {
    "Go": { "format_on_save": "on", "tab_size": 2, },
  },
  "features": ["one", "two"],
}
`
	if err := os.WriteFile(source, []byte(current), 0o600); err != nil {
		t.Fatalf("update source: %v", err)
	}
	run(2, statusArgs...)
	run(0, installArgs...)
	run(0, statusArgs...)
	run(0, doctorArgs...)

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read reconciled target: %v", err)
	}
	for _, want := range []string{`"Rust"`, `"runtime": true`, `Written by Zed`, `"tab_size": 2`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("reconciled target missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), `"theme"`) {
		t.Fatalf("reconciled target retained retired owned theme:\n%s", got)
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	rec, ok := meta.FindByTarget(target)
	if !ok || rec.Ownership != "jsonc-subset" || !json.Valid(rec.OwnedContent) {
		t.Fatalf("JSONC ownership metadata = %#v, want strict JSON snapshot", rec)
	}

	uninstallOutput := run(0, "uninstall", "--yes", "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if !strings.Contains(uninstallOutput, "Removed dots-owned content from 1 target.") {
		t.Fatalf("uninstall output did not describe partial removal:\n%s", uninstallOutput)
	}
	got, err = os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after partial uninstall: %v", err)
	}
	for _, want := range []string{`"Rust"`, `"runtime": true`, `Written by Zed`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("uninstall lost target-only content %s:\n%s", want, got)
		}
	}
	for _, removed := range []string{`"Go"`, `"features"`, `"tab_size"`} {
		if strings.Contains(string(got), removed) {
			t.Fatalf("uninstall retained dots-owned content %s:\n%s", removed, got)
		}
	}
}
