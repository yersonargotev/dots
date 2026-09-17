package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRepositoryHerdrTagPlansAndExecutesPinnedPluginsInSandbox(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repositoryRoot, "dots.yaml")
	home := t.TempDir()
	stateRoot := t.TempDir()
	fakeRealHome := t.TempDir()
	stubDir := t.TempDir()
	logPath := filepath.Join(home, "herdr-test.log")

	oldOS, oldArch := installHostOS, installHostArch
	installHostOS, installHostArch = "darwin", "arm64"
	t.Cleanup(func() {
		installHostOS, installHostArch = oldOS, oldArch
	})
	t.Setenv("HOME", fakeRealHome)
	t.Setenv("PATH", stubDir)

	writeHerdrIntegrationStub(t, filepath.Join(stubDir, "herdr"), `#!/bin/sh
printf '%s\n' "$*" >> "$HOME/herdr-test.log"
`)
	for _, command := range []string{"git", "python3", "fnm", "node", "rustup", "rustc", "cargo"} {
		writeHerdrIntegrationStub(t, filepath.Join(stubDir, command), "#!/bin/sh\nexit 0\n")
	}

	dryRun := NewRootCommand()
	var dryRunOutput bytes.Buffer
	dryRun.SetOut(&dryRunOutput)
	dryRun.SetErr(&dryRunOutput)
	dryRun.SetArgs([]string{
		"install", "--dry-run", "--tag", "herdr",
		"--file", manifestPath, "--home", home, "--source-root", repositoryRoot, "--state-root", stateRoot,
	})
	if err := dryRun.Execute(); err != nil {
		t.Fatalf("Herdr dry-run failed: %v\noutput:\n%s", err, dryRunOutput.String())
	}
	for _, want := range []string{
		"szrenwei/herdr-space-tab-metadata",
		"c696c36256eddc6ee1983ab9f202848b84460e06",
		"hasuwini77/herdr-tab-git",
		"83ce41a11c5cc3ab2de1452ab303f6dfb976a937",
		"yersonargotev/tabby",
		"34c01f9791dd3228acae7ca378adb38e09d9fb6c",
	} {
		if !strings.Contains(dryRunOutput.String(), want) {
			t.Fatalf("Herdr dry-run omitted pinned value %q:\n%s", want, dryRunOutput.String())
		}
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("Herdr dry-run executed a plugin installer: %v", err)
	}

	install := NewRootCommand()
	var installOutput bytes.Buffer
	install.SetOut(&installOutput)
	install.SetErr(&installOutput)
	install.SetArgs([]string{
		"install", "--yes", "--tag", "herdr",
		"--file", manifestPath, "--home", home, "--source-root", repositoryRoot, "--state-root", stateRoot,
	})
	if err := install.Execute(); err != nil {
		t.Fatalf("Herdr sandbox install failed: %v\noutput:\n%s", err, installOutput.String())
	}

	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake Herdr invocation log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(logContent)), "\n")
	want := []string{
		"plugin install szrenwei/herdr-space-tab-metadata --ref c696c36256eddc6ee1983ab9f202848b84460e06 --yes",
		"plugin install hasuwini77/herdr-tab-git --ref 83ce41a11c5cc3ab2de1452ab303f6dfb976a937 --yes",
		"plugin install yersonargotev/tabby --ref 34c01f9791dd3228acae7ca378adb38e09d9fb6c --yes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Herdr invocations = %#v, want only the three pinned plugins %#v\noutput:\n%s", got, want, installOutput.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "herdr", "config.toml")); err != nil {
		t.Fatalf("Herdr Tag did not install its Managed Entry: %v", err)
	}
	if entries, err := os.ReadDir(fakeRealHome); err != nil || len(entries) != 0 {
		t.Fatalf("Herdr install touched inherited HOME %q: entries=%v err=%v", fakeRealHome, entries, err)
	}
}

func writeHerdrIntegrationStub(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write executable stub %s: %v", path, err)
	}
}
