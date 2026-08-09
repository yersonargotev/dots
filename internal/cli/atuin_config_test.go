package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestAtuinDefaultProfileInstallsAndReportsAlignedInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	stubManifestProvisionerTools(t)

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "core",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})

	if err := install.Execute(); err != nil {
		t.Fatalf("dots install failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}

	managed := []struct {
		target  string
		source  string
		symlink bool
	}{
		{
			target:  filepath.Join(home, ".config", "atuin", "config.toml"),
			source:  filepath.Join(repoRoot, "configs", "atuin", "config.toml"),
			symlink: false,
		},
		{
			target:  filepath.Join(home, ".config", "atuin", "themes", "catppuccin-mocha.toml"),
			source:  filepath.Join(repoRoot, "configs", "atuin", "themes", "catppuccin-mocha.toml"),
			symlink: true,
		},
	}

	for _, entry := range managed {
		info, err := os.Lstat(entry.target)
		if err != nil {
			t.Fatalf("atuin target %q missing after sandbox install: %v\ninstall output:\n%s", entry.target, err, installOut.String())
		}
		if entry.symlink {
			if info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("atuin target %q mode = %v, want symlink", entry.target, info.Mode())
			}
			gotSource, err := os.Readlink(entry.target)
			if err != nil {
				t.Fatalf("read atuin symlink %q: %v", entry.target, err)
			}
			if gotSource != entry.source {
				t.Fatalf("atuin symlink %q = %q, want %q", entry.target, gotSource, entry.source)
			}
		} else if !info.Mode().IsRegular() {
			t.Fatalf("atuin target %q mode = %v, want regular file", entry.target, info.Mode())
		}
	}

	sourceBefore, err := os.ReadFile(managed[0].source)
	if err != nil {
		t.Fatalf("read Atuin Source of Truth: %v", err)
	}
	target, err := os.OpenFile(managed[0].target, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open live Atuin config: %v", err)
	}
	if _, err := target.WriteString("\n# written like atuin config set\nsearch_mode = \"fuzzy\"\n"); err != nil {
		t.Fatalf("append live Atuin setting: %v", err)
	}
	if err := target.Close(); err != nil {
		t.Fatalf("close live Atuin config: %v", err)
	}
	sourceAfter, err := os.ReadFile(managed[0].source)
	if err != nil {
		t.Fatalf("reread Atuin Source of Truth: %v", err)
	}
	if !bytes.Equal(sourceAfter, sourceBefore) {
		t.Fatal("Atuin-like target write changed the repository Source of Truth")
	}

	// History/sync/auth state lives in the data dir, never the config dir, so a
	// fresh install must not create ~/.local/share/atuin in the sandbox home.
	dataDir := filepath.Join(home, ".local", "share", "atuin")
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("atuin data dir %q should not exist after install; stat err = %v", dataDir, err)
	}

	status := cli.NewRootCommand()
	var statusOut bytes.Buffer
	status.SetOut(&statusOut)
	status.SetErr(&statusOut)
	status.SetArgs([]string{
		"status",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "core",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
	})

	if err := status.Execute(); err != nil {
		t.Fatalf("dots status failed in sandbox: %v\noutput:\n%s", err, statusOut.String())
	}

	got := statusOut.String()
	for _, want := range []string{
		"configs/atuin/config.toml -> " + managed[0].target,
		"configs/atuin/themes/catppuccin-mocha.toml -> " + managed[1].target,
		"Summary:",
		"0 conflict",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}

	if !strings.Contains(installOut.String(), "configs/atuin/config.toml") {
		t.Fatalf("install plan did not include the Atuin managed entry\noutput:\n%s", installOut.String())
	}
}
