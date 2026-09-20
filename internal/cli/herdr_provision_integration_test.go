package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/state"
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
	for _, command := range []string{"git", "python3", "fnm", "node", "rustup", "rustc", "cargo", "lazygit", "fzf", "fd", "bat"} {
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

func TestRepositoryHerdrTagOnIntelExcludesArmOnlyTabbyAndRust(t *testing.T) {
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
	installHostOS, installHostArch = "darwin", "amd64"
	t.Cleanup(func() {
		installHostOS, installHostArch = oldOS, oldArch
	})
	t.Setenv("HOME", fakeRealHome)
	t.Setenv("PATH", stubDir)

	writeHerdrIntegrationStub(t, filepath.Join(stubDir, "herdr"), `#!/bin/sh
printf '%s\n' "$*" >> "$HOME/herdr-test.log"
`)
	for _, command := range []string{"git", "python3", "fnm", "node", "lazygit", "fzf", "fd", "bat"} {
		writeHerdrIntegrationStub(t, filepath.Join(stubDir, command), "#!/bin/sh\nexit 0\n")
	}
	// If architecture filtering regresses, the missing Rust toolchain must fail
	// safely through this stub instead of reaching a real package manager.
	writeHerdrIntegrationStub(t, filepath.Join(stubDir, "brew"), "#!/bin/sh\nexit 99\n")

	dryRun := NewRootCommand()
	var dryRunOutput bytes.Buffer
	dryRun.SetOut(&dryRunOutput)
	dryRun.SetErr(&dryRunOutput)
	dryRun.SetArgs([]string{
		"install", "--dry-run", "--tag", "herdr",
		"--file", manifestPath, "--home", home, "--source-root", repositoryRoot, "--state-root", stateRoot,
	})
	if err := dryRun.Execute(); err != nil {
		t.Fatalf("Intel Herdr dry-run failed: %v\noutput:\n%s", err, dryRunOutput.String())
	}
	for _, excluded := range []string{"yersonargotev/tabby", "34c01f9791dd3228acae7ca378adb38e09d9fb6c", "Rust stable (rustup)"} {
		if strings.Contains(dryRunOutput.String(), excluded) {
			t.Fatalf("Intel Herdr dry-run included arm64-only value %q:\n%s", excluded, dryRunOutput.String())
		}
	}
	if !strings.Contains(dryRunOutput.String(), "All declared dependencies are already installed.") {
		t.Fatalf("Intel Herdr dry-run tried to install the absent Rust fallback dependencies:\n%s", dryRunOutput.String())
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("Intel Herdr dry-run executed a plugin installer: %v", err)
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
		t.Fatalf("Intel Herdr sandbox install failed: %v\noutput:\n%s", err, installOutput.String())
	}

	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake Intel Herdr invocation log: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(logContent)), "\n")
	want := []string{
		"plugin install szrenwei/herdr-space-tab-metadata --ref c696c36256eddc6ee1983ab9f202848b84460e06 --yes",
		"plugin install hasuwini77/herdr-tab-git --ref 83ce41a11c5cc3ab2de1452ab303f6dfb976a937 --yes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Intel Herdr invocations = %#v, want only compatible plugins %#v\noutput:\n%s", got, want, installOutput.String())
	}
	metadata, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Intel Installation Metadata: %v", err)
	}
	if len(metadata.Provisioners) != 2 {
		t.Fatalf("Intel Provisioner inventory = %#v, want two compatible plugins", metadata.Provisioners)
	}
	for _, record := range metadata.Provisioners {
		if strings.Contains(strings.Join(record.Args, " "), "tabby") {
			t.Fatalf("Intel Provisioner inventory retained arm64-only Tabby: %#v", metadata.Provisioners)
		}
	}
	if entries, err := os.ReadDir(fakeRealHome); err != nil || len(entries) != 0 {
		t.Fatalf("Intel Herdr install touched inherited HOME %q: entries=%v err=%v", fakeRealHome, entries, err)
	}
}

func writeHerdrIntegrationStub(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write executable stub %s: %v", path, err)
	}
}
