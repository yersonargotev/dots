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

func TestJSONSubsetLifecycleReconcilesAndUninstallsOwnedContribution(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	t.Setenv("HOME", t.TempDir())

	source := filepath.Join(sourceRoot, "configs", "shared.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	fixture := func(name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", "issue385", name))
		if err != nil {
			t.Fatalf("read fixture %s: %v", name, err)
		}
		return data
	}
	if err := os.WriteFile(source, fixture("previous.json"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	if err := os.WriteFile(manifestPath, fixture("manifest.yaml"), 0o600); err != nil {
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
	run(0, installArgs...)

	target := filepath.Join(home, ".config", "shared.json")
	if err := os.WriteFile(target, fixture("target-with-external.json"), 0o640); err != nil {
		t.Fatalf("add target-only content: %v", err)
	}
	statusArgs := append([]string{"status", "--output", "json"}, common...)
	run(0, statusArgs...)

	if err := os.WriteFile(source, fixture("current.json"), 0o600); err != nil {
		t.Fatalf("update source: %v", err)
	}
	run(2, statusArgs...)
	run(0, installArgs...)
	run(0, statusArgs...)

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read reconciled target: %v", err)
	}
	for _, want := range []string{`"added":"new"`, `"external":"preserve"`, `"targetOnly":true`, `"external"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("reconciled target missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), `"retired"`) || strings.Contains(string(got), `"old"`) {
		t.Fatalf("reconciled target retained retired contribution:\n%s", got)
	}

	uninstallOutput := run(0, "uninstall", "--yes", "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if !strings.Contains(uninstallOutput, "Removed dots-owned content from 1 target.") {
		t.Fatalf("uninstall output did not describe partial removal:\n%s", uninstallOutput)
	}
	got, err = os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target after partial uninstall: %v", err)
	}
	for _, want := range []string{`"external": "preserve"`, `"targetOnly": true`, `"external"`} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("uninstall lost target-only content %s:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), `"added"`) || strings.Contains(string(got), `"new"`) || strings.Contains(string(got), `"keep"`) {
		t.Fatalf("uninstall retained dots-owned contribution:\n%s", got)
	}
	if _, err := os.Stat(state.Path(stateRoot)); !os.IsNotExist(err) {
		t.Fatalf("metadata should be pruned after partial uninstall, err = %v", err)
	}
}
