package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

// TestClaudeDefaultProfileSeedsUserBaselineInSandbox proves that dots seeds the
// user-owned Claude settings baseline and the portable statusLine script with a
// copy strategy (regular files, not symlinks) before the gentle-ai provisioner
// runs, and that the provisioner is rendered to install the claude-code agent.
// The gentle-ai/engram tools are stubbed so the provisioner step exits cleanly
// without merging its own keys, keeping the sandbox aligned.
func TestClaudeDefaultProfileSeedsUserBaselineInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	stubGentleAIProvisionerTools(t)

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "default",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})

	if err := install.Execute(); err != nil {
		t.Fatalf("dots install failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}

	// The settings baseline and statusLine script are copied (not symlinked) so
	// gentle-ai can merge its own keys without writing back into the repo.
	managed := []struct {
		target string
		source string
	}{
		{
			target: filepath.Join(home, ".claude", "settings.json"),
			source: filepath.Join(repoRoot, "configs", "claude", "settings.json"),
		},
		{
			target: filepath.Join(home, ".claude", "statusline-command.sh"),
			source: filepath.Join(repoRoot, "configs", "claude", "statusline-command.sh"),
		},
	}
	for _, m := range managed {
		info, err := os.Lstat(m.target)
		if err != nil {
			t.Fatalf("claude target missing after sandbox install: %v\ninstall output:\n%s", err, installOut.String())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("claude target %q is a symlink, want a regular copied file", m.target)
		}
		got, err := os.ReadFile(m.target)
		if err != nil {
			t.Fatalf("read copied claude target %q: %v", m.target, err)
		}
		want, err := os.ReadFile(m.source)
		if err != nil {
			t.Fatalf("read claude source %q: %v", m.source, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("copied claude target %q content differs from source %q", m.target, m.source)
		}
	}

	// The baseline must not version any gentle-ai-managed or runtime-state key.
	settings, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read seeded settings.json: %v", err)
	}
	for _, forbidden := range []string{
		"permissions.deny", "\"deny\"", "hooks", "outputStyle",
		"enabledPlugins", "extraKnownMarketplaces", "defaultMode",
		"skipDangerousModePermissionPrompt",
	} {
		if strings.Contains(string(settings), forbidden) {
			t.Fatalf("seeded settings.json must not version %q\ncontent:\n%s", forbidden, settings)
		}
	}
	// The statusLine command must be portable (no hardcoded absolute home path).
	if !strings.Contains(string(settings), "bash ~/.claude/statusline-command.sh") {
		t.Fatalf("seeded settings.json statusLine command is not normalized to ~\ncontent:\n%s", settings)
	}

	out := installOut.String()
	for _, want := range []string{
		"configs/claude/settings.json",
		"configs/claude/statusline-command.sh",
		// The single gentle-ai provisioner now installs both agents.
		"codex,claude-code",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("install output missing %q\noutput:\n%s", want, out)
		}
	}
}
