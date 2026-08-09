package cli_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestNeovimSeededStateLifecycleUsesConfinedXDGStateAndPreservesLocalEvolution(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	dotsStateRoot := filepath.Join(home, ".dots-state")
	xdgStateHome := filepath.Join(home, "xdg", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", xdgStateHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "xdg", "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "xdg", "cache"))

	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	oldBaseline := "{\n  \"plugin-a\": \"old\",\n  \"plugin-b\": \"retired\"\n}\n"
	newBaseline := "{\n  \"plugin-a\": \"new\"\n}\n"
	localEvolution := "{\n  \"plugin-a\": \"local-update\",\n  \"plugin-b\": \"retired\"\n}\n"
	write(filepath.Join(sourceRoot, "configs/nvim/lazy-lock.json"), oldBaseline)
	write(filepath.Join(sourceRoot, "configs/nvim/loader.lua"), "local managed = vim.fn.expand('~/.config/dots/nvim')\n")
	write(filepath.Join(sourceRoot, "configs/nvim/init.lua"), "vim.g.dots_managed = true\n")
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	write(manifestPath, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim/lazy-lock.json
    target: nvim/lazy-lock.json
    target_root: xdg-state
    strategy: copy
    ownership: seeded
    tags: [core]
  - source: configs/nvim/loader.lua
    target: ~/.config/nvim/init.lua
    strategy: copy
    tags: [core]
  - source: configs/nvim
    target: ~/.config/dots/nvim
    strategy: symlink
    tags: [core]
`)

	runIssue388Git(t, sourceRoot, "init")
	runIssue388Git(t, sourceRoot, "config", "user.email", "dots@example.test")
	runIssue388Git(t, sourceRoot, "config", "user.name", "dots test")
	runIssue388Git(t, sourceRoot, "add", ".")
	runIssue388Git(t, sourceRoot, "commit", "-m", "test baseline")

	run := func(want int, args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := cli.Run(args, &stdout, &stderr)
		if code != want {
			t.Fatalf("cli.Run(%v) code = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, want, stdout.String(), stderr.String())
		}
		return stdout.String()
	}
	common := []string{"--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", dotsStateRoot}
	installArgs := append([]string{"install", "--yes", "--skip-deps"}, common...)
	statusArgs := append([]string{"status", "--output", "json"}, common...)
	run(0, installArgs...)

	loader := filepath.Join(home, ".config/nvim/init.lua")
	if info, err := os.Lstat(loader); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("native loader = (%v, %v), want regular file", info, err)
	}
	managed := filepath.Join(home, ".config/dots/nvim")
	if target, err := os.Readlink(managed); err != nil || target != filepath.Join(sourceRoot, "configs/nvim") {
		t.Fatalf("managed config symlink = (%q, %v)", target, err)
	}
	lock := filepath.Join(xdgStateHome, "nvim/lazy-lock.json")
	if got, err := os.ReadFile(lock); err != nil || string(got) != oldBaseline {
		t.Fatalf("seeded lock = (%q, %v), want old baseline", got, err)
	}

	write(filepath.Join(sourceRoot, "configs/nvim/lazy-lock.json"), newBaseline)
	runIssue388Git(t, sourceRoot, "add", "configs/nvim/lazy-lock.json")
	runIssue388Git(t, sourceRoot, "commit", "-m", "advance lock baseline")
	write(lock, localEvolution)
	statusOutput := run(0, statusArgs...)
	if !strings.Contains(statusOutput, `"state": "ok"`) || !strings.Contains(statusOutput, `"reason": "seeded-local-evolution"`) {
		t.Fatalf("status did not report aligned local evolution:\n%s", statusOutput)
	}
	run(0, installArgs...)
	if got, err := os.ReadFile(lock); err != nil || string(got) != localEvolution {
		t.Fatalf("install changed local evolution = (%q, %v)", got, err)
	}
	if got := strings.TrimSpace(runIssue388Git(t, sourceRoot, "status", "--porcelain")); got != "" {
		t.Fatalf("simulated lazy.nvim write dirtied the repository: %q", got)
	}

	write(lock, oldBaseline)
	run(0, installArgs...)
	if got, err := os.ReadFile(lock); err != nil || string(got) != newBaseline {
		t.Fatalf("unchanged seeded state did not advance with baseline removal = (%q, %v)", got, err)
	}

	uninstallOutput := run(0, "uninstall", "--yes", "--home", home, "--source-root", sourceRoot, "--state-root", dotsStateRoot)
	if !strings.Contains(uninstallOutput, "Retained Seeded Runtime State") {
		t.Fatalf("uninstall did not report retained state:\n%s", uninstallOutput)
	}
	if got, err := os.ReadFile(lock); err != nil || string(got) != newBaseline {
		t.Fatalf("uninstall changed seeded state = (%q, %v)", got, err)
	}
	if _, err := os.Lstat(loader); !os.IsNotExist(err) {
		t.Fatalf("native loader still exists after uninstall: %v", err)
	}
	if _, err := os.Lstat(managed); !os.IsNotExist(err) {
		t.Fatalf("managed config symlink still exists after uninstall: %v", err)
	}
}

func runIssue388Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}
