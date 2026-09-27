package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/upgrade"
)

func TestCarbonfoxUpgradePreferenceChangeStopsBeforeBinaryReplacement(t *testing.T) {
	for _, continuation := range []bool{false, true} {
		for _, removing := range []bool{false, true} {
			name := "add"
			if removing {
				name = "remove"
			}
			if continuation {
				name += "/continuation"
			}
			t.Run(name, func(t *testing.T) {
				home, stateRoot := t.TempDir(), t.TempDir()
				t.Setenv("HOME", t.TempDir())
				_, sourceRoot := newContinuationRepo(t)
				manifestPath := filepath.Join(sourceRoot, "dots.yaml")
				const manifest = `version: 1
profiles:
  core:
    tags: [app]
entries:
  - source: app
    target: ~/.app
    strategy: copy
    tags: [app]
  - source: marker
    target: ~/.config/dots/theme-carbonfox
    strategy: symlink
    tags: [theme-carbonfox]
`
				if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
					t.Fatal(err)
				}
				runContinuationGit(t, sourceRoot, "add", "dots.yaml")
				runContinuationGit(t, sourceRoot, "commit", "-m", "add Carbonfox fixture")
				previous := state.InstalledSelection{Profiles: []string{"core"}, ResolvedTags: []string{"app"}}
				if removing {
					previous.ExtraTags = []string{"theme-carbonfox"}
					previous.ResolvedTags = append(previous.ResolvedTags, "theme-carbonfox")
				}
				if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(state.Path(stateRoot))
				if err != nil {
					t.Fatal(err)
				}
				oldCurrent, oldPreview, oldExecute, oldExec := currentExecutable, previewUpgrade, executeUpgrade, execBinary
				t.Cleanup(func() {
					currentExecutable, previewUpgrade, executeUpgrade, execBinary = oldCurrent, oldPreview, oldExecute, oldExec
				})
				currentExecutable = func() (string, error) { return "/tmp/fake-dots", nil }
				plan := upgrade.Plan{Channel: upgrade.ChannelRelease, Action: upgrade.ActionReplaceBinary}
				previewUpgrade = func(context.Context, upgrade.Options) (upgrade.Plan, error) { return plan, nil }
				executions, continuations := 0, 0
				executeUpgrade = func(context.Context, upgrade.Options) (upgrade.Plan, error) { executions++; return plan, nil }
				execBinary = func(string, []string, []string) error { continuations++; return nil }
				args := []string{"upgrade", "--yes", "--acknowledge-selection-change", "--profile", "core", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}
				if !removing {
					args = append(args, "--tag", "theme-carbonfox")
				}
				if continuation {
					args = append(args, "--continue")
				}
				cmd := NewRootCommand()
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&out)
				cmd.SetArgs(args)
				err = cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), "requires dots install with the complete desired selection") {
					t.Fatalf("error = %v; output %s", err, &out)
				}
				if executions != 0 || continuations != 0 {
					t.Fatalf("binary effects: execute=%d, exec=%d", executions, continuations)
				}
				after, err := os.ReadFile(state.Path(stateRoot))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("rejected preference change altered Installation Metadata")
				}
				entries, err := os.ReadDir(home)
				if err != nil || len(entries) != 0 {
					t.Fatalf("home changed: %v, %v", entries, err)
				}
			})
		}
	}
}
