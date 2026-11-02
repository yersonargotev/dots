package cli

import (
	"fmt"
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
	previousManifest := filepath.Join(t.TempDir(), "dots.yaml")
	// Distinct sandbox targets cover former baseline and source-override links
	// together, including installations predating the copy/seeded migrations.
	sources := []string{
		"configs/tmux/tmux.conf",
		"configs/zellij/layouts/default.kdl",
		"configs/zellij/layouts/default-carbonfox.kdl",
		"configs/zed/themes/catppuccin-blue.json",
		"configs/tmux/carbonfox.conf",
		"configs/zellij/themes/carbonfox.kdl",
		"configs/warp/themes/carbonfox/carbonfox.yaml",
		"configs/tuicr/themes/carbonfox.toml",
		"configs/zed/themes/carbonfox.json",
		"configs/zellij/config.kdl",
		"configs/zellij/config-adaptive.kdl",
		"configs/zed/keymap.json",
		"configs/zed/settings.json",
		"configs/opencode/mcp.json",
	}
	previous := "version: 1\nprofiles:\n  core:\n    tags: [retired]\nentries:\n"
	for i, source := range sources {
		previous += fmt.Sprintf("  - source: %s\n    target: ~/.retired-sources/%d\n    strategy: symlink\n    tags: [retired]\n", source, i)
	}
	if err := os.WriteFile(previousManifest, []byte(previous), 0o600); err != nil {
		t.Fatal(err)
	}
	f.run(t, ExitOK, "install", "--yes", "--skip-deps", "--profile", "core",
		"--file", previousManifest, "--source-root", f.repositoryRoot, "--home", f.home, "--state-root", f.stateRoot)
	before := make(map[string][]byte)
	for i := range sources {
		target := filepath.Join(f.home, ".retired-sources", fmt.Sprint(i))
		before[target] = mustReadCarbonfoxFile(t, target)
	}
	args := []string{"--profile", "core", "--file", f.manifestPath, "--source-root", f.repositoryRoot, "--home", f.home, "--state-root", f.stateRoot}
	f.run(t, ExitOK, append([]string{"install", "--yes", "--skip-deps"}, args...)...)
	for target, content := range before {
		assertCarbonfoxFileBytes(t, target, content, "Profile evolution changed historical symlink content")
		if _, ok := f.metadata(t).FindByTarget(target); !ok {
			t.Fatalf("Profile evolution discarded historical ownership: %s", target)
		}
	}
	metadataBefore := mustReadCarbonfoxFile(t, state.Path(f.stateRoot))
	// An explicit reduction still requires acknowledgement before mutation.
	reduced := []string{"install", "--yes", "--skip-deps", "--tag", "starship", "--file", f.manifestPath, "--source-root", f.repositoryRoot, "--home", f.home, "--state-root", f.stateRoot}
	f.run(t, ExitError, reduced...)
	assertCarbonfoxFileBytes(t, state.Path(f.stateRoot), metadataBefore, "unacknowledged reduction changed Installation Metadata")
	for target, content := range before {
		assertCarbonfoxFileBytes(t, target, content, "unacknowledged reduction changed retired target")
	}
	f.run(t, ExitOK, append(reduced, "--acknowledge-selection-change")...)
	for target, content := range before {
		assertCarbonfoxFileBytes(t, target, content, "explicit reduction removed historical target")
	}
	assertCarbonfoxInheritedHomeEmpty(t, f.realHome)
}
