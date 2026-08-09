package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/manifest"
)

func TestWarpDesktopProfileInstallsAndReportsAlignedInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	stubManifestProvisionerTools(t)
	seedDesktopNerdFont(t, home)

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

	wantTargets := warpTargetsForOS(t, home)
	for source, target := range wantTargets {
		info, err := os.Lstat(target)
		if err != nil {
			t.Fatalf("warp target %s missing after sandbox install: %v\ninstall output:\n%s", target, err, installOut.String())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("warp target %s mode = %v, want regular copy, not symlink", target, info.Mode())
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("warp target %s mode = %v, want regular copy", target, info.Mode())
		}
		wantSource := filepath.Join(repoRoot, filepath.FromSlash(source))
		wantContent, err := os.ReadFile(wantSource)
		if err != nil {
			t.Fatalf("read warp source %s: %v", wantSource, err)
		}
		gotContent, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read warp target %s: %v", target, err)
		}
		if string(gotContent) != string(wantContent) {
			t.Fatalf("warp target %s content differs from source %s", target, wantSource)
		}
	}

	managedSettings, err := os.ReadFile(filepath.Join(repoRoot, "configs", "warp", "settings.toml"))
	if err != nil {
		t.Fatalf("read managed warp settings: %v", err)
	}
	for _, want := range []string{
		`theme = "dark"`,
		`font_name = "Cascadia Code NF"`,
		"font_size = 20.0",
	} {
		if !strings.Contains(string(managedSettings), want) {
			t.Fatalf("managed warp settings missing %q", want)
		}
	}

	managedKeybindings, err := os.ReadFile(filepath.Join(repoRoot, "configs", "warp", "keybindings.yaml"))
	if err != nil {
		t.Fatalf("read managed warp keybindings: %v", err)
	}
	if !strings.Contains(string(managedKeybindings), `"input:clear_screen": alt-shift-K`) {
		t.Fatalf("managed warp keybindings missing clear-screen binding")
	}

	manifestFile, err := manifest.LoadFile(filepath.Join(repoRoot, "dots.yaml"))
	if err != nil {
		t.Fatalf("load dots manifest: %v", err)
	}
	wantEntries := map[string]map[string]bool{
		"configs/warp/settings.toml": {
			"~/.warp/settings.toml":                 false,
			"~/.config/warp-terminal/settings.toml": false,
		},
		"configs/warp/keybindings.yaml": {
			"~/.warp/keybindings.yaml":                 false,
			"~/.config/warp-terminal/keybindings.yaml": false,
		},
	}
	for i := range manifestFile.Entries {
		entry := &manifestFile.Entries[i]
		targets, ok := wantEntries[entry.Source]
		if !ok {
			continue
		}
		if _, ok := targets[entry.Target]; !ok {
			continue
		}
		if entry.Strategy != "copy" {
			t.Fatalf("Warp entry %s -> %s strategy = %q, want copy", entry.Source, entry.Target, entry.Strategy)
		}
		if !slices.Contains(entry.Tags, "desktop") {
			t.Fatalf("Warp entry %s -> %s tags = %#v, want desktop", entry.Source, entry.Target, entry.Tags)
		}
		wantOS := "linux"
		if strings.HasPrefix(entry.Target, "~/.warp/") {
			wantOS = "darwin"
		}
		if len(entry.OS) != 1 || entry.OS[0] != wantOS {
			t.Fatalf("Warp entry %s -> %s OS = %#v, want [%s]", entry.Source, entry.Target, entry.OS, wantOS)
		}
		targets[entry.Target] = true
	}
	for source, targets := range wantEntries {
		for target, found := range targets {
			if !found {
				t.Fatalf("dots manifest missing Warp managed entry %s -> %s", source, target)
			}
		}
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
	wantStatusLines := []string{"Summary:", "0 conflict"}
	for source, target := range wantTargets {
		wantStatusLines = append(wantStatusLines, source+" -> "+target)
	}
	for _, want := range wantStatusLines {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}
}

func warpTargetsForOS(t *testing.T, home string) map[string]string {
	t.Helper()

	switch runtime.GOOS {
	case "darwin":
		return map[string]string{
			"configs/warp/settings.toml":    filepath.Join(home, ".warp", "settings.toml"),
			"configs/warp/keybindings.yaml": filepath.Join(home, ".warp", "keybindings.yaml"),
		}
	case "linux":
		return map[string]string{
			"configs/warp/settings.toml":    filepath.Join(home, ".config", "warp-terminal", "settings.toml"),
			"configs/warp/keybindings.yaml": filepath.Join(home, ".config", "warp-terminal", "keybindings.yaml"),
		}
	default:
		t.Fatalf("unsupported OS for Warp managed entries: %s", runtime.GOOS)
		return nil
	}
}
