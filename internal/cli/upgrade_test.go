package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestUpgradeDryRunPreviewsBinaryAndSourceOfTruthWithoutModifying(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})
	advanceUpstream(t, origin, "update zsh config", map[string]string{"configs/zsh/zshrc": "export A=2\n"})

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"upgrade", "--profile", "default", "--dry-run", "--file", filepath.Join(sourceRoot, "dots.yaml"), "--home", home, "--source-root", sourceRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("upgrade --dry-run error = %v\noutput:\n%s", err, out.String())
	}
	for _, want := range []string{"Binary upgrade preview", "action=manual-rebuild", "update zsh config", `Plan for profile "default" (tags: core)`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q\n%s", want, out.String())
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("dry-run installed a target; lstat err = %v", err)
	}
}

func TestUpgradeJSONRequiresDryRunOrYes(t *testing.T) {
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"upgrade", "--output", "json"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--output json requires --yes or --dry-run") {
		t.Fatalf("error = %v, want JSON gating error", err)
	}
}

func TestUpgradeDevBuildAbortsBeforeSourceOfTruthPhase(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"upgrade", "--home", home, "--source-root", sourceRoot})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "development/local dots build detected") {
		t.Fatalf("error = %v, want development build rejection", err)
	}
	if strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("upgrade reached Source of Truth phase after binary rejection: %v", err)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(statErr) {
		t.Fatalf("upgrade touched HOME after binary rejection; lstat err = %v", statErr)
	}
}

func TestUpgradeContinueReusesUpdateWorkflow(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})
	advanceUpstream(t, origin, "add tmux config", map[string]string{
		"configs/tmux/tmux.conf": "set -g mouse on\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
`,
	})
	saveInstalledSelection(t, stateRoot, "default")

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"upgrade", "--continue", "--file", filepath.Join(sourceRoot, "dots.yaml"), "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("upgrade --continue error = %v\noutput:\n%s", err, out.String())
	}
	if want := "Selection: source=recorded profiles=default extra-tags=(none) effective-tags=core"; !strings.Contains(out.String(), want) {
		t.Fatalf("upgrade continuation output missing selection report %q:\n%s", want, out.String())
	}
	if _, err := os.Readlink(filepath.Join(home, ".tmux.conf")); err != nil {
		t.Fatalf("continuation did not install updated Managed Entry: %v", err)
	}
}

func TestUpgradeContinueJSONEmitsUpgradeReportWithBinaryPhase(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})
	saveInstalledSelection(t, stateRoot, "default")

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"upgrade", "--continue", "--yes", "--output", "json",
		"--selection-source", "recorded", "--selection-profile", "default",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot,
		"--state-root", stateRoot,
		"--binary-channel", "homebrew",
		"--binary-current-version", "v0.18.0",
		"--binary-latest-version", "v0.19.0",
		"--binary-action", "homebrew-upgrade",
		"--binary-executable", "/opt/homebrew/bin/dots",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("upgrade --continue --output json error = %v\noutput:\n%s", err, out.String())
	}
	var env struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Data    struct {
			DryRun    bool `json:"dry_run"`
			Selection struct {
				Source   string   `json:"source"`
				Profiles []string `json:"profiles"`
			} `json:"selection"`
			Binary struct {
				Channel        string `json:"channel"`
				CurrentVersion string `json:"current_version"`
				LatestVersion  string `json:"latest_version"`
				Action         string `json:"action"`
			} `json:"binary"`
			Update struct {
				OldRev string `json:"old_rev"`
			} `json:"update"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v\n%s", err, out.String())
	}
	if env.Command != "upgrade" || env.Status != "ok" || env.Data.DryRun {
		t.Fatalf("envelope header = %+v, want upgrade ok real run", env)
	}
	if env.Data.Binary.Channel != "homebrew" || env.Data.Binary.CurrentVersion != "v0.18.0" || env.Data.Binary.LatestVersion != "v0.19.0" || env.Data.Binary.Action != "homebrew-upgrade" {
		t.Fatalf("binary report = %+v, want preserved binary phase", env.Data.Binary)
	}
	if env.Data.Selection.Source != "recorded" || !reflect.DeepEqual(env.Data.Selection.Profiles, []string{"default"}) {
		t.Fatalf("selection report = %+v, want recorded default intent", env.Data.Selection)
	}
	if env.Data.Update.OldRev == "" {
		t.Fatalf("upgrade JSON lost Source of Truth update report: %+v", env.Data.Update)
	}
}

func TestUpgradeDryRunJSONUsesAgentEnvelope(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zsh/zshrc": "export A=1\n",
		"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
	})

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"upgrade", "--profile", "default", "--dry-run", "--output", "json", "--file", filepath.Join(sourceRoot, "dots.yaml"), "--home", home, "--source-root", sourceRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("upgrade --dry-run --output json error = %v\noutput:\n%s", err, out.String())
	}
	var env struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Data    struct {
			DryRun bool `json:"dry_run"`
			Binary struct {
				Channel string `json:"channel"`
				Action  string `json:"action"`
			} `json:"binary"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v\n%s", err, out.String())
	}
	if env.Command != "upgrade" || env.Status != "ok" || !env.Data.DryRun || env.Data.Binary.Action != "manual-rebuild" {
		t.Fatalf("envelope = %+v, want upgrade ok dry-run manual rebuild", env)
	}
}
