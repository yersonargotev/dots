package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/state"
)

func TestRepositoryRetainedAgentConfigurationOmitsRetiredIntegrations(t *testing.T) {
	f := newCarbonfoxFixture(t, "linux")
	f.install(t, false, "neovim", "claude", "codex")
	for _, file := range []string{
		".config/dots/nvim/lazyvim.json", ".local/state/nvim/lazy-lock.json",
		".claude/settings.json", ".codex/config.toml",
	} {
		data := mustReadCarbonfoxFile(t, filepath.Join(f.home, file))
		for _, retired := range []string{"copilot", "codegraph", "chrome-devtools"} {
			if strings.Contains(strings.ToLower(string(data)), retired) {
				t.Errorf("installed %s retains %s", file, retired)
			}
		}
	}
	assertCarbonfoxInheritedHomeEmpty(t, f.realHome)
}

func TestRepositoryProfileEvolutionRetainsPreviouslyInstalledConfiguration(t *testing.T) {
	f := newCarbonfoxFixture(t, "linux")
	previousRoot := t.TempDir()
	previousManifest := filepath.Join(previousRoot, "dots.yaml")
	for name, content := range map[string]string{
		"dots.yaml": `version: 1
tags:
  tmux:
    kind: surface
    status: current
profiles:
  core:
    tags: [tmux]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [tmux]
`,
	} {
		if err := os.WriteFile(filepath.Join(previousRoot, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.run(t, ExitOK, "install", "--yes", "--skip-deps", "--profile", "core",
		"--file", previousManifest, "--source-root", f.repositoryRoot, "--home", f.home, "--state-root", f.stateRoot)
	target := filepath.Join(f.home, ".tmux.conf")
	before := mustReadCarbonfoxFile(t, target)
	args := []string{"--profile", "core", "--file", f.manifestPath, "--source-root", f.repositoryRoot, "--home", f.home, "--state-root", f.stateRoot}
	f.run(t, ExitOK, append([]string{"install", "--yes", "--skip-deps"}, args...)...)
	if got := mustReadCarbonfoxFile(t, target); !bytes.Equal(got, before) {
		t.Fatalf("Profile evolution changed retired target: %q", got)
	}
	if _, ok := f.metadata(t).FindByTarget(target); !ok {
		t.Fatal("Profile evolution discarded ownership of the retired target")
	}
	metadataBefore := mustReadCarbonfoxFile(t, state.Path(f.stateRoot))
	// An explicit reduction still requires acknowledgement before mutation.
	reduced := []string{"install", "--yes", "--skip-deps", "--tag", "starship", "--file", f.manifestPath, "--source-root", f.repositoryRoot, "--home", f.home, "--state-root", f.stateRoot}
	f.run(t, ExitError, reduced...)
	assertCarbonfoxFileBytes(t, state.Path(f.stateRoot), metadataBefore, "unacknowledged reduction changed Installation Metadata")
	assertCarbonfoxFileBytes(t, target, before, "unacknowledged reduction changed retired target")
	f.run(t, ExitOK, append(reduced, "--acknowledge-selection-change")...)
	assertCarbonfoxFileBytes(t, target, before, "explicit reduction removed manifest-retired historical target")
	assertCarbonfoxInheritedHomeEmpty(t, f.realHome)
}
