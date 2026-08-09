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

func TestGhosttyDesktopProfileInstallsAndReportsAlignedInSandbox(t *testing.T) {
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
		"--profile", "core",
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

	managedConfig, err := os.ReadFile(wantSource)
	if err != nil {
		t.Fatalf("read managed ghostty config: %v", err)
	}
	for _, want := range []string{
		`font-family = "Cascadia Code NF"`,
		"font-size = 20",
		"theme = Catppuccin Mocha",
		"config-file = ?adaptive-theme.ghostty",
	} {
		if !strings.Contains(string(managedConfig), want) {
			t.Fatalf("managed ghostty config missing %q", want)
		}
	}

	manifestFile, err := manifest.LoadFile(filepath.Join(repoRoot, "dots.yaml"))
	if err != nil {
		t.Fatalf("load dots manifest: %v", err)
	}
	var ghosttyEntry *manifest.Entry
	for i := range manifestFile.Entries {
		entry := &manifestFile.Entries[i]
		if entry.Source == "configs/ghostty/config.ghostty" {
			ghosttyEntry = entry
			break
		}
	}
	if ghosttyEntry == nil {
		t.Fatal("dots manifest missing Ghostty managed entry")
	}
	if !slices.Contains(ghosttyEntry.Tags, "desktop") {
		t.Fatalf("Ghostty managed entry tags = %#v, want desktop", ghosttyEntry.Tags)
	}
	foundDesktopFontDependency := false
	for _, set := range manifestFile.Dependencies {
		if !slices.Contains(set.Tags, "desktop") {
			continue
		}
		for _, dep := range set.Dependencies {
			if dep.BrewCask == "font-cascadia-code-nf" && dep.FontMatch == "CascadiaCodeNF*" && len(dep.FontFallbackMatches) == 1 && dep.FontFallbackMatches[0] == "CaskaydiaCoveNerdFont*" {
				foundDesktopFontDependency = true
				break
			}
		}
	}
	if !foundDesktopFontDependency {
		t.Fatalf("desktop dependency set is missing font-cascadia-code-nf with CascadiaCodeNF* and Caskaydia fallback")
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
		"--profile", "core",
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
	if strings.Contains(installOut.String(), "adaptive-theme.ghostty") {
		t.Fatalf("install without adaptive-theme tag included adaptive Ghostty fragment\noutput:\n%s", installOut.String())
	}
}

func TestAdaptiveThemeTagInstallsMarkerAndGhosttyFragmentInSandbox(t *testing.T) {
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
		"--profile", "core",
		"--profile", "desktop",
		"--tag", "adaptive-theme",
		"--source-root", repoRoot,
		"--home", home,
		"--state-root", stateRoot,
		"--yes",
	})

	if err := install.Execute(); err != nil {
		t.Fatalf("dots install with adaptive-theme failed in sandbox: %v\noutput:\n%s", err, installOut.String())
	}

	targets := map[string]string{
		filepath.Join(home, ".config", "dots", "theme.sh"):       filepath.Join(repoRoot, "configs", "dots", "theme.sh"),
		filepath.Join(home, ".config", "dots", "adaptive-theme"): filepath.Join(repoRoot, "configs", "dots", "adaptive-theme"),
	}
	ghosttyAdaptiveTarget := filepath.Join(home, ".config", "ghostty", "adaptive-theme.ghostty")
	if runtime.GOOS == "darwin" {
		targets[ghosttyAdaptiveTarget] = filepath.Join(repoRoot, "configs", "ghostty", "adaptive", "adaptive-theme.ghostty")
	}
	for target, wantSource := range targets {
		gotSource, err := os.Readlink(target)
		if err != nil {
			t.Fatalf("adaptive-theme target %s is not a symlink: %v\noutput:\n%s", target, err, installOut.String())
		}
		if gotSource != wantSource {
			t.Fatalf("adaptive-theme symlink %s = %q, want %q", target, gotSource, wantSource)
		}
	}

	fragment, err := os.ReadFile(filepath.Join(repoRoot, "configs", "ghostty", "adaptive", "adaptive-theme.ghostty"))
	if err != nil {
		t.Fatalf("read Ghostty adaptive fragment: %v", err)
	}
	if !strings.Contains(string(fragment), "theme = light:Catppuccin Latte,dark:Catppuccin Mocha") {
		t.Fatalf("Ghostty adaptive fragment does not use native light/dark theme syntax\ncontent:\n%s", fragment)
	}
	for _, invalidName := range []string{"catppuccin-latte", "catppuccin-mocha"} {
		if strings.Contains(string(fragment), invalidName) {
			t.Fatalf("Ghostty adaptive fragment uses invalid built-in theme name %q\ncontent:\n%s", invalidName, fragment)
		}
	}
	if runtime.GOOS != "darwin" {
		if _, err := os.Lstat(ghosttyAdaptiveTarget); !os.IsNotExist(err) {
			t.Fatalf("non-Darwin adaptive-theme install created Ghostty adaptive fragment; err=%v output:\n%s", err, installOut.String())
		}
	}
}
