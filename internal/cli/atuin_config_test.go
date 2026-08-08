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
		target string
		source string
	}{
		{
			target: filepath.Join(home, ".config", "atuin", "config.toml"),
			source: filepath.Join(repoRoot, "configs", "atuin", "config.toml"),
		},
		{
			target: filepath.Join(home, ".config", "atuin", "themes", "catppuccin-mocha.toml"),
			source: filepath.Join(repoRoot, "configs", "atuin", "themes", "catppuccin-mocha.toml"),
		},
	}

	for _, entry := range managed {
		info, err := os.Lstat(entry.target)
		if err != nil {
			t.Fatalf("atuin target %q missing after sandbox install: %v\ninstall output:\n%s", entry.target, err, installOut.String())
		}
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
