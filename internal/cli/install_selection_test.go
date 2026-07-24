package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

func TestInstallRecordsAuthoritativeSelectionAfterSuccessfulApply(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  core:
    tags: [core, shared]
  agents:
    tags: [agents, shared]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes", "--skip-deps",
		"--profile", "core", "--profile", "agents",
		"--tag", "shared", "--tag", "adaptive-theme", "--tag", "adaptive-theme",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.Version != state.CurrentVersion {
		t.Fatalf("metadata version = %d, want %d", meta.Version, state.CurrentVersion)
	}
	if meta.InstalledSelection == nil {
		t.Fatal("InstalledSelection = nil after successful install")
	}
	got := meta.InstalledSelection
	if want := []string{"core", "agents"}; !reflect.DeepEqual(got.Profiles, want) {
		t.Fatalf("Profiles = %#v, want %#v", got.Profiles, want)
	}
	if want := []string{"shared", "adaptive-theme"}; !reflect.DeepEqual(got.ExtraTags, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got.ExtraTags, want)
	}
	if want := []string{"core", "shared", "agents", "adaptive-theme"}; !reflect.DeepEqual(got.ResolvedTags, want) {
		t.Fatalf("ResolvedTags = %#v, want %#v", got.ResolvedTags, want)
	}
	if got.Provenance.SourceRoot != sourceRoot {
		t.Fatalf("SourceRoot = %q, want %q", got.Provenance.SourceRoot, sourceRoot)
	}
	if got.Provenance.RecordedAt == "" {
		t.Fatal("RecordedAt is empty")
	}
	if len(meta.Entries) != 1 {
		t.Fatalf("Entries = %#v, want the Managed Entry inventory preserved", meta.Entries)
	}
}

func TestInstallFailurePreservesPreviousInstalledSelection(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 7\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	previous := state.InstalledSelection{
		Profiles:     []string{"old"},
		ExtraTags:    []string{"old-extra"},
		ResolvedTags: []string{"old", "old-extra"},
		Provenance:   state.Provenance{SourceRoot: "/old/source", RecordedAt: "2026-01-02T03:04:05Z"},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		Entries:            []state.Record{},
		InstalledSelection: &previous,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  new:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      persona: neutral
      agents: [codex]
    dependencies:
      - name: gentle-ai
      - name: engram
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--profile", "new",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want failing provisioner\noutput:\n%s", out.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.Version != state.CurrentVersion {
		t.Fatalf("metadata version = %d, want preserved v%d", meta.Version, state.CurrentVersion)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
	if len(meta.Entries) != 1 {
		t.Fatalf("Entries = %#v, want partial Managed Entry inventory retained", meta.Entries)
	}
	if len(meta.Provisioners) != 1 || meta.Provisioners[0].Status != "failed" {
		t.Fatalf("Provisioners = %#v, want failed inventory retained", meta.Provisioners)
	}
}

func TestInstallYesSelectionReductionRequiresDedicatedAcknowledgement(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	previous := state.InstalledSelection{
		Profiles:     []string{"old"},
		ExtraTags:    []string{"old-extra"},
		ResolvedTags: []string{"old", "old-extra"},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		InstalledSelection: &previous,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  new:
    tags: [new]
entries:
  - source: configs/new
    target: ~/.new
    strategy: symlink
    tags: [new]
`)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--skip-deps", "--output", "json", "--profile", "new",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitError, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "selection-change-acknowledgement-required") {
		t.Fatalf("JSON error missing acknowledgement code:\n%s", out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".new")); !os.IsNotExist(err) {
		t.Fatalf("selection rejection applied Managed Configuration: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
}

func TestInstallPostProvisionerConvergenceFailurePreservesPreviousSelection(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nmkdir -p \"$HOME/.codex/AGENTS.md\"\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	previous := state.InstalledSelection{
		Profiles:     []string{"old"},
		ResolvedTags: []string{"old"},
		Provenance:   state.Provenance{RecordedAt: "2026-01-02T03:04:05Z"},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		Entries:            []state.Record{},
		InstalledSelection: &previous,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  new:
    tags: [core, codegraph]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: codegraph
    tags: [codegraph]
    spec:
      scope: global
      agents: [codex]
      yes: true
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--profile", "new",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want post-Provisioner convergence failure\noutput:\n%s", out.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
	if len(meta.Provisioners) != 1 || meta.Provisioners[0].Status != "provisioned" {
		t.Fatalf("Provisioners = %#v, want successful inventory before convergence failure", meta.Provisioners)
	}
}

func TestInstallWithoutExplicitProfileMutatesNothing(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HOME", t.TempDir())

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: must-not-run
        command: definitely-missing-profile-guard-probe
provisioners:
  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      persona: neutral
      agents: [codex]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "at least one --profile is required") {
		t.Fatalf("Execute() error = %v, want explicit Profile guidance\noutput:\n%s", err, out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("target mutated without Profile; lstat err = %v", err)
	}
	if _, err := os.Stat(stateRoot); !os.IsNotExist(err) {
		t.Fatalf("state root created without Profile; stat err = %v", err)
	}
}
