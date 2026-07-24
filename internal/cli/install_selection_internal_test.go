package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/state"
)

func TestInstallSelectionCommitFailurePreservesPreviousSelection(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	source := filepath.Join(sourceRoot, "configs", "zsh", "zshrc")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(source, []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifestPath := filepath.Join(home, "dots.yaml")
	if err := os.WriteFile(manifestPath, []byte(`version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

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

	originalRecord := recordInstalledSelection
	recordInstalledSelection = func(string, state.InstalledSelection) error {
		return errors.New("injected Installed Selection commit failure")
	}
	t.Cleanup(func() { recordInstalledSelection = originalRecord })

	cmd := newInstallCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{
		"--yes", "--acknowledge-selection-change", "--skip-deps", "--profile", "core",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want selection commit failure\noutput:\n%s", output.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("Managed Entry was not applied before terminal selection commit: %v", err)
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
}
