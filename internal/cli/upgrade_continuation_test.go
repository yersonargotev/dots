package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/upgrade"
)

func TestUpgradeBinaryReplacementContinuationPreservesRecordedSelection(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	tests := []struct {
		name    string
		channel string
		action  string
	}{
		{name: "homebrew", channel: upgrade.ChannelHomebrew, action: upgrade.ActionHomebrewUpgrade},
		{name: "release", channel: upgrade.ChannelRelease, action: upgrade.ActionReplaceBinary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			stateRoot := t.TempDir()
			sourceRoot := newContinuationRepo(t)
			t.Setenv("HOME", t.TempDir())

			previous := state.InstalledSelection{
				Profiles:     []string{"core"},
				ExtraTags:    []string{"extra"},
				ResolvedTags: []string{"core", "extra"},
			}
			if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
				t.Fatalf("save metadata: %v", err)
			}

			plan := upgrade.Plan{
				Channel:        tt.channel,
				CurrentVersion: "v0.63.0",
				LatestVersion:  "v0.64.0",
				Action:         tt.action,
				Executable:     "/tmp/fake-dots",
				Artifact:       "dots_v0.64.0_test",
				Checksum:       "abc123",
			}
			originalExecutable := currentExecutable
			originalPreview := previewUpgrade
			originalExecute := executeUpgrade
			originalExecBinary := execBinary
			t.Cleanup(func() {
				currentExecutable = originalExecutable
				previewUpgrade = originalPreview
				executeUpgrade = originalExecute
				execBinary = originalExecBinary
			})
			currentExecutable = func() (string, error) { return plan.Executable, nil }
			previewUpgrade = func(_ context.Context, _ upgrade.Options) (upgrade.Plan, error) { return plan, nil }
			executeUpgrade = func(_ context.Context, _ upgrade.Options) (upgrade.Plan, error) { return plan, nil }
			var continuationArgs []string
			execErr := errors.New("fake exec boundary")
			execBinary = func(_ string, args []string, _ []string) error {
				continuationArgs = append([]string(nil), args...)
				return execErr
			}

			first := NewRootCommand()
			var firstOut bytes.Buffer
			first.SetOut(&firstOut)
			first.SetErr(&firstOut)
			first.SetArgs([]string{"upgrade", "--yes", "--output", "json",
				"--file", filepath.Join(sourceRoot, "dots.yaml"),
				"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
			if err := first.Execute(); !errors.Is(err, execErr) {
				t.Fatalf("initial upgrade error = %v, want fake exec boundary\noutput:\n%s", err, firstOut.String())
			}
			assertContinuationSelectionArgs(t, continuationArgs)
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatalf("load metadata after exec failure: %v", err)
			}
			if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
				t.Fatalf("exec failure changed InstalledSelection = %#v, want %#v", meta.InstalledSelection, previous)
			}

			continued := NewRootCommand()
			var continuedOut bytes.Buffer
			continued.SetOut(&continuedOut)
			continued.SetErr(&continuedOut)
			continued.SetArgs(continuationArgs[1:])
			if err := continued.Execute(); err != nil {
				t.Fatalf("continued upgrade error = %v\noutput:\n%s", err, continuedOut.String())
			}
			for _, target := range []string{".core", ".extra"} {
				if _, err := os.Readlink(filepath.Join(home, target)); err != nil {
					t.Fatalf("continuation did not apply %s: %v", target, err)
				}
			}
			var env struct {
				Data struct {
					Selection selection.Report `json:"selection"`
				} `json:"data"`
			}
			if err := json.Unmarshal(continuedOut.Bytes(), &env); err != nil {
				t.Fatalf("unmarshal continuation JSON: %v\n%s", err, continuedOut.String())
			}
			if env.Data.Selection.Source != selection.SourceRecorded ||
				!reflect.DeepEqual(env.Data.Selection.Profiles, previous.Profiles) ||
				!reflect.DeepEqual(env.Data.Selection.ExtraTags, previous.ExtraTags) ||
				!reflect.DeepEqual(env.Data.Selection.EffectiveTags, previous.ResolvedTags) {
				t.Fatalf("continuation selection = %#v, want recorded intent %#v", env.Data.Selection, previous)
			}
		})
	}
}

func assertContinuationSelectionArgs(t *testing.T, args []string) {
	t.Helper()
	joined := strings.Join(args, "\x00")
	for _, expected := range []string{"--selection-source\x00recorded", "--selection-profile\x00core", "--selection-tag\x00extra"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("continuation args missing %q: %#v", expected, args)
		}
	}
	for _, publicFlag := range []string{"--profile", "--tag"} {
		for _, arg := range args {
			if arg == publicFlag {
				t.Fatalf("recorded selection leaked through public flag %q: %#v", publicFlag, args)
			}
		}
	}
}

func newContinuationRepo(t *testing.T) string {
	t.Helper()
	origin := t.TempDir()
	runContinuationGit(t, origin, "init", "-b", "main")
	runContinuationGit(t, origin, "config", "user.email", "tests@example.com")
	runContinuationGit(t, origin, "config", "user.name", "dots tests")
	files := map[string]string{
		"configs/core":  "core\n",
		"configs/extra": "extra\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
  - source: configs/extra
    target: ~/.extra
    strategy: symlink
    tags: [extra]
`,
	}
	for name, content := range files {
		path := filepath.Join(origin, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	runContinuationGit(t, origin, "add", "-A")
	runContinuationGit(t, origin, "commit", "-m", "initial")
	clone := t.TempDir()
	runContinuationGit(t, "", "clone", origin, clone)
	runContinuationGit(t, clone, "config", "user.email", "tests@example.com")
	runContinuationGit(t, clone, "config", "user.name", "dots tests")
	return clone
}

func runContinuationGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
