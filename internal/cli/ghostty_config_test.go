package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestGhosttyDesktopProfileInstallsAndReportsAlignedInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	install := cli.NewRootCommand()
	var installOut bytes.Buffer
	install.SetOut(&installOut)
	install.SetErr(&installOut)
	install.SetArgs([]string{
		"install",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "desktop",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})

	if err := install.Execute(); err != nil {
		t.Fatalf("dots install failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}

	target := filepath.Join(home, ".config", "ghostty", "config.ghostty")
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatalf("ghostty target missing after sandbox install: %v\ninstall output:\n%s", err, installOut.String())
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("ghostty target mode = %v, want symlink", info.Mode())
	}
	wantSource := filepath.Join(repoRoot, "configs", "ghostty", "config.ghostty")
	gotSource, err := os.Readlink(target)
	if err != nil {
		t.Fatalf("read ghostty symlink: %v", err)
	}
	if gotSource != wantSource {
		t.Fatalf("ghostty symlink = %q, want %q", gotSource, wantSource)
	}

	localExample := filepath.Join(repoRoot, "configs", "ghostty", "config.local.ghostty.example")
	if _, err := os.Stat(localExample); err != nil {
		t.Fatalf("local override example missing: %v", err)
	}

	status := cli.NewRootCommand()
	var statusOut bytes.Buffer
	status.SetOut(&statusOut)
	status.SetErr(&statusOut)
	status.SetArgs([]string{
		"status",
		"--file", filepath.Join(repoRoot, "dots.yaml"),
		"--profile", "desktop",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
	})

	if err := status.Execute(); err != nil {
		t.Fatalf("dots status failed in sandbox: %v\noutput:\n%s", err, statusOut.String())
	}

	got := statusOut.String()
	for _, want := range []string{
		"configs/ghostty/config.ghostty -> " + target,
		"Summary:",
		"0 conflict",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}

	if !strings.Contains(installOut.String(), "configs/ghostty/config.ghostty") {
		t.Fatalf("install plan did not include the Ghostty managed entry\noutput:\n%s", installOut.String())
	}
}
