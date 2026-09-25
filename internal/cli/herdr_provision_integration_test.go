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
	for _, command := range []string{"git", "python3", "fnm", "node", "rustup", "rustc", "cargo", "curl", "tar", "pbcopy", "open", "lazygit", "fzf", "fd", "bat"} {
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
		"yersonargotev/herdr-tab-git",
		"48fe5a66c17970a77919dc50a6ab3b6512bc7300",
		"yersonargotev/tabby",
		"19031d668f0f8a79b58591db4ce8bc8f8846e329",
		"rmarganti/herdr-pluck",
		"d1eacb80956c3a23ab6f7428a9e83961fb86ba28",
	} {
		if !strings.Contains(dryRunOutput.String(), want) {
			t.Fatalf("Herdr dry-run omitted pinned value %q:\n%s", want, dryRunOutput.String())
		}
	}
	if strings.Contains(dryRunOutput.String(), "hasuwini77/herdr-tab-git") {
		t.Fatalf("Herdr dry-run retained the retired plugin:\n%s", dryRunOutput.String())
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("Herdr dry-run executed a plugin installer: %v", err)
	}
	repeatedDryRun := NewRootCommand()
	var repeatedDryRunOutput bytes.Buffer
	repeatedDryRun.SetOut(&repeatedDryRunOutput)
	repeatedDryRun.SetErr(&repeatedDryRunOutput)
	repeatedDryRun.SetArgs([]string{
		"install", "--dry-run", "--tag", "herdr",
		"--file", manifestPath, "--home", home, "--source-root", repositoryRoot, "--state-root", stateRoot,
	})
	if err := repeatedDryRun.Execute(); err != nil {
		t.Fatalf("repeated Herdr dry-run failed: %v\noutput:\n%s", err, repeatedDryRunOutput.String())
	}
	if repeatedDryRunOutput.String() != dryRunOutput.String() {
		t.Fatalf("repeated Herdr dry-run changed its plan\nfirst:\n%s\nsecond:\n%s", dryRunOutput.String(), repeatedDryRunOutput.String())
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
		"plugin install yersonargotev/herdr-tab-git --ref 48fe5a66c17970a77919dc50a6ab3b6512bc7300 --yes",
		"plugin install yersonargotev/tabby --ref 19031d668f0f8a79b58591db4ce8bc8f8846e329 --yes",
		"plugin install rmarganti/herdr-pluck --ref d1eacb80956c3a23ab6f7428a9e83961fb86ba28 --yes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Herdr invocations = %#v, want only the three pinned plugins %#v\noutput:\n%s", got, want, installOutput.String())
	}
	metadata, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Apple Silicon Installation Metadata: %v", err)
	}
	if len(metadata.Provisioners) != 3 {
		t.Fatalf("Apple Silicon Provisioner inventory = %#v, want three pinned plugins", metadata.Provisioners)
	}
	pluckRecorded := false
	for _, record := range metadata.Provisioners {
		args := strings.Join(record.Args, " ")
		if strings.Contains(args, "rmarganti/herdr-pluck") {
			pluckRecorded = strings.Contains(args, "d1eacb80956c3a23ab6f7428a9e83961fb86ba28")
		}
	}
	if !pluckRecorded {
		t.Fatalf("Installation Metadata omitted pinned Pluck contribution: %#v", metadata.Provisioners)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "herdr", "config.toml")); err != nil {
		t.Fatalf("Herdr Tag did not install its Managed Entry: %v", err)
	}
	if entries, err := os.ReadDir(fakeRealHome); err != nil || len(entries) != 0 {
		t.Fatalf("Herdr install touched inherited HOME %q: entries=%v err=%v", fakeRealHome, entries, err)
	}
}

func TestRepositoryHerdrAppleSiliconDryRunDisclosesMissingPluckDependencies(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	stubDir := t.TempDir()
	commandLog := filepath.Join(home, "external-command.log")

	oldOS, oldArch := installHostOS, installHostArch
	installHostOS, installHostArch = "darwin", "arm64"
	t.Cleanup(func() {
		installHostOS, installHostArch = oldOS, oldArch
	})
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", stubDir)

	for _, command := range []string{"herdr", "git", "python3", "fnm", "node", "lazygit", "fzf", "fd", "bat"} {
		writeHerdrIntegrationStub(t, filepath.Join(stubDir, command), "#!/bin/sh\nexit 0\n")
	}
	writeHerdrIntegrationStub(t, filepath.Join(stubDir, "brew"), "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$COMMAND_LOG\"\nexit 99\n")
	t.Setenv("COMMAND_LOG", commandLog)

	dryRun := NewRootCommand()
	var output bytes.Buffer
	dryRun.SetOut(&output)
	dryRun.SetErr(&output)
	dryRun.SetArgs([]string{
		"install", "--dry-run", "--tag", "herdr",
		"--file", filepath.Join(repositoryRoot, "dots.yaml"), "--home", home,
		"--source-root", repositoryRoot, "--state-root", stateRoot,
	})
	if err := dryRun.Execute(); err != nil {
		t.Fatalf("Herdr missing-dependency dry-run failed: %v\noutput:\n%s", err, output.String())
	}
	for _, want := range []string{
		"brew install rustup",
		"brew install curl",
		"Restore the macOS tar system utility",
		"Restore the macOS pbcopy system utility",
		"Restore the macOS open system utility",
		"herdr plugin install rmarganti/herdr-pluck --ref d1eacb80956c3a23ab6f7428a9e83961fb86ba28 --yes",
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("Apple Silicon dry-run omitted external action %q:\n%s", want, output.String())
		}
	}
	if _, err := os.Stat(commandLog); !os.IsNotExist(err) {
		t.Fatalf("dry-run executed an external command: %v", err)
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
	for _, excluded := range []string{
		"yersonargotev/tabby",
		"19031d668f0f8a79b58591db4ce8bc8f8846e329",
		"rmarganti/herdr-pluck",
		"d1eacb80956c3a23ab6f7428a9e83961fb86ba28",
		"Rust stable (rustup)",
		"tar", "pbcopy", "open",
	} {
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
		"plugin install yersonargotev/herdr-tab-git --ref 48fe5a66c17970a77919dc50a6ab3b6512bc7300 --yes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Intel Herdr invocations = %#v, want only compatible plugins %#v\noutput:\n%s", got, want, installOutput.String())
	}
	metadata, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load Intel Installation Metadata: %v", err)
	}
	if len(metadata.Provisioners) != 1 {
		t.Fatalf("Intel Provisioner inventory = %#v, want one compatible plugin", metadata.Provisioners)
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
