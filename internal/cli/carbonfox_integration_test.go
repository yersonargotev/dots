package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/state"
)

const carbonfoxTag = "theme-carbonfox"

type carbonfoxFixture struct {
	repositoryRoot string
	manifestPath   string
	home           string
	stateRoot      string
	realHome       string
	stubLog        string
	stubDir        string
}

func newCarbonfoxFixture(t *testing.T, hostOS string) carbonfoxFixture {
	t.Helper()

	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fixture := carbonfoxFixture{
		repositoryRoot: repositoryRoot,
		manifestPath:   filepath.Join(repositoryRoot, "dots.yaml"),
		home:           t.TempDir(),
		stateRoot:      t.TempDir(),
		realHome:       t.TempDir(),
		stubLog:        filepath.Join(t.TempDir(), "invocations.log"),
	}
	t.Setenv("HOME", fixture.realHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(fixture.home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(fixture.home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(fixture.home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(fixture.home, ".local", "state"))
	t.Setenv("DOTS_CARBONFOX_STUB_LOG", fixture.stubLog)
	fixture.stubDir = installCarbonfoxExecutableStubs(t)

	previousOS, previousArch := installHostOS, installHostArch
	installHostOS, installHostArch = hostOS, "amd64"
	t.Cleanup(func() {
		installHostOS, installHostArch = previousOS, previousArch
	})
	return fixture
}

func installCarbonfoxExecutableStubs(t *testing.T) string {
	t.Helper()

	stubDir := t.TempDir()
	blocked := `#!/bin/sh
printf '%s %s\n' "${0##*/}" "$*" >> "$DOTS_CARBONFOX_STUB_LOG"
printf 'unexpected external installer: %s %s\n' "${0##*/}" "$*" >&2
exit 97
`
	for _, name := range []string{"apt", "apt-get", "brew", "curl", "dnf", "flatpak", "npm", "pacman", "snap", "sudo", "wget", "yum"} {
		writeCarbonfoxExecutable(t, filepath.Join(stubDir, name), blocked)
	}

	tool := `#!/bin/sh
printf '%s %s\n' "${0##*/}" "$*" >> "$DOTS_CARBONFOX_STUB_LOG"
case "${0##*/}:$1" in
  claude:--version) printf '%s\n' '2.1.220' ;;
  tuicr:--version) printf '%s\n' 'tuicr 0.27.0' ;;
  warp-terminal:--version) printf '%s\n' 'warp-terminal 0.2026.06.03.09.49.stable_00' ;;
  starship:--version) printf '%s\n' 'starship 1.25.1' ;;
  atuin:--version) printf '%s\n' 'atuin 18.16.1' ;;
  zellij:--version) printf '%s\n' 'zellij 0.44.3' ;;
esac
exit 0
`
	for _, name := range []string{
		"agy", "atuin", "bun", "cargo", "claude", "codegraph", "codex", "copilot", "dart", "delta", "eza", "fd", "fnm", "fzf", "gh", "ghostty", "git", "go", "herdr", "jq", "lazygit", "node", "npx", "nvim", "open", "opencode", "pbcopy", "playwright-cli", "pnpm", "python3", "rg", "rustc", "rustup", "starship", "tar", "tmux", "tuicr", "unzip", "uv", "warp-terminal", "zed", "zellij", "zoxide", "zsh",
	} {
		writeCarbonfoxExecutable(t, filepath.Join(stubDir, name), tool)
	}

	bat := `#!/bin/sh
printf 'bat %s\n' "$*" >> "$DOTS_CARBONFOX_STUB_LOG"
case "$1" in
  --config-dir) printf '%s\n' "$HOME/.config/bat" ;;
  --config-file) printf '%s\n' "$HOME/.config/bat/config" ;;
  --cache-dir) printf '%s\n' "$HOME/.cache/bat" ;;
  --version) printf '%s\n' 'bat 0.26.1' ;;
  --list-themes) printf '%s\n' 'Carbonfox' ;;
  cache)
    test "$2" = '--build' || exit 2
    mkdir -p "$BAT_CACHE_PATH"
    printf '%s\n' 'metadata' > "$BAT_CACHE_PATH/metadata.yaml"
    printf '%s\n' 'syntaxes' > "$BAT_CACHE_PATH/syntaxes.bin"
    printf '%s\n' 'themes' > "$BAT_CACHE_PATH/themes.bin"
    ;;
  --color=always) printf '\033[38;2;120;169;255mpackage main\033[0m\n' ;;
esac
exit 0
`
	writeCarbonfoxExecutable(t, filepath.Join(stubDir, "bat"), bat)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
	return stubDir
}

func writeCarbonfoxExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable stub %s: %v", path, err)
	}
}

func (f carbonfoxFixture) install(t *testing.T, acknowledge bool, tags ...string) string {
	t.Helper()
	args := []string{
		"install", "--yes", "--skip-deps", "--output", "json",
		"--file", f.manifestPath, "--home", f.home, "--source-root", f.repositoryRoot, "--state-root", f.stateRoot,
	}
	if acknowledge {
		args = append(args, "--acknowledge-selection-change")
	}
	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}
	return f.run(t, ExitOK, args...)
}

func (f carbonfoxFixture) transition(t *testing.T, wantCode int, replace, acknowledge bool, tags ...string) string {
	t.Helper()
	args := []string{
		"install", "--yes", "--skip-deps", "--output", "json",
		"--file", f.manifestPath, "--home", f.home, "--source-root", f.repositoryRoot, "--state-root", f.stateRoot,
	}
	if replace {
		args = append(args, "--backup-and-replace")
	}
	if acknowledge {
		args = append(args, "--acknowledge-selection-change")
	}
	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}
	return f.run(t, wantCode, args...)
}

func (f carbonfoxFixture) run(t *testing.T, wantCode int, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(args, &stdout, &stderr)
	if code != wantCode {
		t.Fatalf("dots %s = exit %d, want %d\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), code, wantCode, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func (f carbonfoxFixture) metadata(t *testing.T) state.Metadata {
	t.Helper()
	metadata, err := state.Load(state.Path(f.stateRoot))
	if err != nil {
		t.Fatalf("load Installation Metadata: %v", err)
	}
	return metadata
}

func assertCarbonfoxRecord(t *testing.T, metadata state.Metadata, home, target, source string) {
	t.Helper()
	resolved := filepath.Join(home, filepath.FromSlash(target))
	record, ok := metadata.FindByTarget(resolved)
	if !ok {
		t.Fatalf("Installation Metadata does not own %s", resolved)
	}
	if !record.HasSource(source) {
		t.Fatalf("%s sources = %#v, want %q", resolved, record.SourceList(), source)
	}
}

func TestRepositoryCarbonfoxConsumerInstallMatrix(t *testing.T) {
	tests := []struct {
		name       string
		tag        string
		hostOS     string
		target     string
		source     string
		contains   string
		additional map[string]string
	}{
		{name: "Ghostty", tag: "ghostty", hostOS: "linux", target: ".config/ghostty/config.ghostty", source: "configs/ghostty/config-carbonfox.ghostty", contains: "theme = Carbonfox"},
		{name: "Herdr", tag: "herdr", hostOS: "darwin", target: ".config/herdr/config.toml", source: "configs/herdr/config-carbonfox.toml", contains: "#78a9ff"},
		{name: "tmux", tag: "tmux", hostOS: "linux", target: ".config/tmux/carbonfox.conf", source: "configs/tmux/carbonfox.conf", contains: "#0c0c0c"},
		{name: "Zellij", tag: "zellij", hostOS: "linux", target: ".config/zellij/config.kdl", source: "configs/zellij/config-carbonfox.kdl", contains: `theme "carbonfox"`},
		{name: "Neovim", tag: "neovim", hostOS: "linux", target: ".config/dots/nvim", source: "configs/nvim", additional: map[string]string{".config/dots/nvim/lua/plugins/colorscheme.lua": `colorscheme = carbonfox and "carbonfox"`}},
		{name: "Starship", tag: "starship", hostOS: "linux", target: ".config/starship.toml", source: "configs/starship/starship-carbonfox.toml", contains: `palette = "carbonfox"`},
		{name: "Atuin", tag: "atuin", hostOS: "linux", target: ".config/atuin/config.toml", source: "configs/atuin/config-carbonfox.toml", contains: `name = "carbonfox"`},
		{name: "bat", tag: "bat", hostOS: "linux", target: ".config/bat/config", source: "configs/bat/config-carbonfox", contains: `--theme="Carbonfox"`, additional: map[string]string{".config/bat/themes/Carbonfox.tmTheme": "<string>Carbonfox</string>", ".cache/bat/themes.bin": "themes"}},
		{name: "tuicr", tag: "tuicr", hostOS: "linux", target: ".config/tuicr/config.toml", source: "configs/tuicr/config-carbonfox.toml", contains: `theme = "carbonfox"`, additional: map[string]string{".config/tuicr/themes/carbonfox.toml": `syntax_theme = "carbonfox.tmTheme"`}},
		{name: "Zed", tag: "zed", hostOS: "linux", target: ".config/zed/settings.json", source: "configs/zed/settings-carbonfox.json", contains: `"dark": "Carbonfox"`, additional: map[string]string{".config/zed/themes/carbonfox.json": `"name": "Carbonfox"`}},
		{name: "Warp", tag: "warp", hostOS: "linux", target: ".config/warp-terminal/settings.toml", source: "configs/warp/settings-carbonfox.toml", contains: `name = "Carbonfox"`, additional: map[string]string{".local/share/warp-terminal/themes/carbonfox/carbonfox.yaml": "background: '#161616'"}},
		{name: "Zsh and both fzf paths", tag: "zsh", hostOS: "linux", target: ".config/bat/config", source: "configs/bat/config-carbonfox", contains: `--theme="Carbonfox"`, additional: map[string]string{".config/dots/zsh/zshrc": "rc.d/pre", ".config/dots/theme-carbonfox": "dots_carbonfox_fzf_colors"}},
		{name: "Claude", tag: "claude", hostOS: "linux", target: ".claude/settings.json", source: "configs/claude/settings-carbonfox.json", contains: `"theme": "custom:carbonfox"`, additional: map[string]string{".claude/statusline-command.sh": "dots_apply_carbonfox_ansi_palette", ".claude/themes/carbonfox.json": `"name": "Carbonfox"`}},
		{name: "Copilot", tag: "copilot", hostOS: "linux", target: ".copilot/statusline-command.sh", source: "configs/copilot/statusline-command.sh", contains: "dots_apply_carbonfox_ansi_palette"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCarbonfoxFixture(t, test.hostOS)
			fixture.install(t, false, test.tag, carbonfoxTag)
			metadata := fixture.metadata(t)
			if metadata.InstalledSelection == nil || !reflect.DeepEqual(metadata.InstalledSelection.ExtraTags, []string{test.tag, carbonfoxTag}) || !reflect.DeepEqual(metadata.InstalledSelection.ResolvedTags, []string{test.tag, carbonfoxTag}) {
				t.Fatalf("Installed Selection = %#v, want only %s + %s", metadata.InstalledSelection, test.tag, carbonfoxTag)
			}
			assertCarbonfoxRecord(t, metadata, fixture.home, test.target, test.source)
			assertCarbonfoxFileContains(t, filepath.Join(fixture.home, filepath.FromSlash(test.target)), test.contains)
			for target, contains := range test.additional {
				assertCarbonfoxFileContains(t, filepath.Join(fixture.home, filepath.FromSlash(target)), contains)
			}
			if test.tag == "zsh" {
				for _, source := range []string{"configs/zsh/rc.d/pre/20-modules.zsh", "configs/zsh/rc.d/post/40-tools.zsh"} {
					assertCarbonfoxFileContains(t, filepath.Join(fixture.repositoryRoot, filepath.FromSlash(source)), "dots_carbonfox_fzf_colors")
				}
			}
			if _, err := os.Lstat(filepath.Join(fixture.home, ".config", "dots", carbonfoxTag)); err != nil {
				t.Fatalf("global Carbonfox marker is missing: %v", err)
			}
			assertCarbonfoxInheritedHomeEmpty(t, fixture.realHome)
		})
	}
}

func TestRepositoryCarbonfoxPreferenceAloneHasNoApplicationSurface(t *testing.T) {
	fixture := newCarbonfoxFixture(t, "linux")
	fixture.install(t, false, carbonfoxTag)
	dependencyOutput := fixture.run(t, ExitOK,
		"--output", "json", "deps", "check", "--tag", carbonfoxTag,
		"--file", fixture.manifestPath, "--home", fixture.home,
	)
	if !strings.Contains(dependencyOutput, `"results": null`) {
		t.Fatalf("preference-only dependency surface is not empty:\n%s", dependencyOutput)
	}
	metadata := fixture.metadata(t)
	if metadata.InstalledSelection == nil || !reflect.DeepEqual(metadata.InstalledSelection.ExtraTags, []string{carbonfoxTag}) || !reflect.DeepEqual(metadata.InstalledSelection.ResolvedTags, []string{carbonfoxTag}) {
		t.Fatalf("Installed Selection = %#v, want only %s", metadata.InstalledSelection, carbonfoxTag)
	}
	if len(metadata.Entries) != 1 || metadata.Entries[0].Target != filepath.Join(fixture.home, ".config", "dots", carbonfoxTag) {
		t.Fatalf("preference-only Managed Entries = %#v, want only shared preference marker", metadata.Entries)
	}
	if len(metadata.Provisioners) != 0 {
		t.Fatalf("preference-only Provisioners = %#v, want none", metadata.Provisioners)
	}
	if data, err := os.ReadFile(fixture.stubLog); err == nil && carbonfoxApplicationInvocations(data) != "" {
		t.Fatalf("preference-only install invoked application tools:\n%s", data)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	assertCarbonfoxInheritedHomeEmpty(t, fixture.realHome)
}

func TestRepositoryCarbonfoxProfileAndExplicitTagsHaveEquivalentEffectiveTags(t *testing.T) {
	profile := newCarbonfoxFixture(t, "linux")
	profile.run(t, ExitOK,
		"install", "--yes", "--skip-deps", "--output", "json", "--profile", "desktop", "--tag", carbonfoxTag,
		"--file", profile.manifestPath, "--home", profile.home, "--source-root", profile.repositoryRoot, "--state-root", profile.stateRoot,
	)
	explicit := newCarbonfoxFixture(t, "linux")
	explicit.install(t, false, "ghostty", "warp", "zed", "codexbar", carbonfoxTag)

	profileSelection := profile.metadata(t).InstalledSelection
	explicitSelection := explicit.metadata(t).InstalledSelection
	if profileSelection == nil || explicitSelection == nil || !reflect.DeepEqual(profileSelection.ResolvedTags, explicitSelection.ResolvedTags) {
		t.Fatalf("profile effective Tags = %#v, explicit effective Tags = %#v", profileSelection, explicitSelection)
	}
	if want := []string{"ghostty", "warp", "zed", "codexbar", carbonfoxTag}; !reflect.DeepEqual(profileSelection.ResolvedTags, want) {
		t.Fatalf("desktop + Carbonfox effective Tags = %#v, want %#v", profileSelection.ResolvedTags, want)
	}
	for _, item := range []struct{ target, source string }{
		{".config/ghostty/config.ghostty", "configs/ghostty/config-carbonfox.ghostty"},
		{".config/warp-terminal/settings.toml", "configs/warp/settings-carbonfox.toml"},
		{".config/zed/settings.json", "configs/zed/settings-carbonfox.json"},
	} {
		assertCarbonfoxRecord(t, profile.metadata(t), profile.home, item.target, item.source)
		assertCarbonfoxRecord(t, explicit.metadata(t), explicit.home, item.target, item.source)
	}
}

func TestRepositoryCarbonfoxWinsOverAdaptiveInBothTagOrders(t *testing.T) {
	for _, tags := range [][]string{
		{"ghostty", "adaptive-theme", carbonfoxTag},
		{"ghostty", carbonfoxTag, "adaptive-theme"},
	} {
		t.Run(strings.Join(tags, "+"), func(t *testing.T) {
			fixture := newCarbonfoxFixture(t, "darwin")
			fixture.install(t, false, tags...)
			metadata := fixture.metadata(t)
			assertCarbonfoxRecord(t, metadata, fixture.home, ".config/ghostty/config.ghostty", "configs/ghostty/config-carbonfox.ghostty")
			assertCarbonfoxFileContains(t, filepath.Join(fixture.home, ".config", "ghostty", "config.ghostty"), "theme = Carbonfox")
			targets := map[string]bool{}
			for _, record := range metadata.Entries {
				if targets[record.Target] {
					t.Fatalf("duplicate Managed Entry action persisted for %s", record.Target)
				}
				targets[record.Target] = true
			}
		})
	}
}

func TestCarbonfoxSymlinkThemeTransitions(t *testing.T) {
	for _, test := range []struct {
		name            string
		hostOS          string
		consumer        string
		adaptive        bool
		target          string
		baselineSource  string
		carbonfoxSource string
	}{
		{name: "Ghostty baseline", hostOS: "linux", consumer: "ghostty", target: ".config/ghostty/config.ghostty", baselineSource: "configs/ghostty/config.ghostty", carbonfoxSource: "configs/ghostty/config-carbonfox.ghostty"},
		{name: "Ghostty adaptive", hostOS: "darwin", consumer: "ghostty", adaptive: true, target: ".config/ghostty/config.ghostty", baselineSource: "configs/ghostty/config.ghostty", carbonfoxSource: "configs/ghostty/config-carbonfox.ghostty"},
		{name: "Starship baseline", hostOS: "linux", consumer: "starship", target: ".config/starship.toml", baselineSource: "configs/starship/starship.toml", carbonfoxSource: "configs/starship/starship-carbonfox.toml"},
		{name: "Starship adaptive", hostOS: "linux", consumer: "starship", adaptive: true, target: ".config/starship.toml", baselineSource: "configs/starship/starship.toml", carbonfoxSource: "configs/starship/starship-carbonfox.toml"},
		{name: "Zellij layout baseline", hostOS: "linux", consumer: "zellij", target: ".config/zellij/layouts/default.kdl", baselineSource: "configs/zellij/layouts/default.kdl", carbonfoxSource: "configs/zellij/layouts/default-carbonfox.kdl"},
		{name: "Zellij layout adaptive", hostOS: "linux", consumer: "zellij", adaptive: true, target: ".config/zellij/layouts/default.kdl", baselineSource: "configs/zellij/layouts/default.kdl", carbonfoxSource: "configs/zellij/layouts/default-carbonfox.kdl"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCarbonfoxFixture(t, test.hostOS)
			initial := []string{test.consumer}
			if test.adaptive {
				initial = append(initial, "adaptive-theme")
			}
			withCarbonfox := append(append([]string(nil), initial...), carbonfoxTag)
			target := filepath.Join(fixture.home, filepath.FromSlash(test.target))
			baselineSource := filepath.Join(fixture.repositoryRoot, filepath.FromSlash(test.baselineSource))
			carbonfoxSource := filepath.Join(fixture.repositoryRoot, filepath.FromSlash(test.carbonfoxSource))
			marker := filepath.Join(fixture.home, ".config", "dots", carbonfoxTag)

			fixture.install(t, false, initial...)
			assertCarbonfoxSymlinkTarget(t, target, baselineSource, "initial install", "")
			beforeAddition := mustReadCarbonfoxFile(t, state.Path(fixture.stateRoot))
			additionOutput := fixture.transition(t, ExitError, false, false, withCarbonfox...)
			if !strings.Contains(additionOutput, "requires Conflict Resolution Replace") {
				t.Fatalf("non-Replace addition lacks actionable diagnosis:\n%s", additionOutput)
			}
			assertCarbonfoxSymlinkTarget(t, target, baselineSource, "rejected Carbonfox addition", additionOutput)
			assertCarbonfoxPathAbsent(t, marker, "rejected Carbonfox addition installed preference marker")
			assertCarbonfoxFileBytes(t, state.Path(fixture.stateRoot), beforeAddition, "rejected Carbonfox addition changed Installed Selection")

			fixture.transition(t, ExitOK, true, false, withCarbonfox...)
			assertCarbonfoxSymlinkTarget(t, target, carbonfoxSource, "authorized Carbonfox addition", "")
			if _, err := os.Lstat(marker); err != nil {
				t.Fatalf("authorized Carbonfox addition did not install preference marker: %v", err)
			}
			assertCarbonfoxSelection(t, fixture.metadata(t), withCarbonfox)
			additionBackups := assertCarbonfoxBackupSymlink(t, fixture, target, baselineSource, 0)

			beforeRemoval := mustReadCarbonfoxFile(t, state.Path(fixture.stateRoot))
			removalOutput := fixture.transition(t, ExitError, false, true, initial...)
			if !strings.Contains(removalOutput, "requires Conflict Resolution Replace") {
				t.Fatalf("non-Replace removal lacks actionable diagnosis:\n%s", removalOutput)
			}
			assertCarbonfoxSymlinkTarget(t, target, carbonfoxSource, "rejected Carbonfox removal", removalOutput)
			if _, err := os.Lstat(marker); err != nil {
				t.Fatalf("rejected Carbonfox removal changed preference marker: %v", err)
			}
			assertCarbonfoxFileBytes(t, state.Path(fixture.stateRoot), beforeRemoval, "rejected Carbonfox removal changed Installed Selection")

			fixture.transition(t, ExitOK, true, true, initial...)
			assertCarbonfoxSymlinkTarget(t, target, baselineSource, "authorized Carbonfox removal", "")
			assertCarbonfoxPathAbsent(t, marker, "authorized Carbonfox removal retained preference marker")
			assertCarbonfoxSelection(t, fixture.metadata(t), initial)
			assertCarbonfoxBackupSymlink(t, fixture, target, carbonfoxSource, additionBackups)
		})
	}
}

func TestCarbonfoxSymlinkSourceSwitchRejectsSkipAndAdoptBeforeMutation(t *testing.T) {
	for _, decision := range []string{"s", "a"} {
		t.Run(decision, func(t *testing.T) {
			fixture := newCarbonfoxFixture(t, "linux")
			useCarbonfoxGhosttySourceCopy(t, &fixture)
			fixture.install(t, false, "ghostty")

			target := filepath.Join(fixture.home, ".config", "ghostty", "config.ghostty")
			baselineSource := filepath.Join(fixture.repositoryRoot, "configs", "ghostty", "config.ghostty")
			carbonfoxSource := filepath.Join(fixture.repositoryRoot, "configs", "ghostty", "config-carbonfox.ghostty")
			baselineBefore := mustReadCarbonfoxFile(t, baselineSource)
			carbonfoxBefore := mustReadCarbonfoxFile(t, carbonfoxSource)
			stateBefore := mustReadCarbonfoxFile(t, state.Path(fixture.stateRoot))

			command := NewRootCommand()
			var output bytes.Buffer
			command.SetIn(strings.NewReader(decision + "\n"))
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs([]string{
				"install", "--no-tui", "--skip-deps", "--tag", "ghostty", "--tag", carbonfoxTag,
				"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.repositoryRoot, "--state-root", fixture.stateRoot,
			})
			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "requires Conflict Resolution Replace") {
				t.Fatalf("decision %q error = %v, want source-switch Replace requirement\n%s", decision, err, output.String())
			}
			if !strings.Contains(output.String(), "Resolve conflict for "+target) {
				t.Fatalf("decision %q did not use ordinary Conflict Resolution:\n%s", decision, output.String())
			}
			assertCarbonfoxSymlinkTarget(t, target, baselineSource, "rejected interactive source switch", output.String())
			assertCarbonfoxFileBytes(t, baselineSource, baselineBefore, "rejected adopt changed baseline Source of Truth")
			assertCarbonfoxFileBytes(t, carbonfoxSource, carbonfoxBefore, "rejected adopt changed Carbonfox Source of Truth")
			assertCarbonfoxFileBytes(t, state.Path(fixture.stateRoot), stateBefore, "rejected decision changed Installed Selection")
			assertCarbonfoxPathAbsent(t, filepath.Join(fixture.home, ".config", "dots", carbonfoxTag), "rejected decision installed preference marker")
		})
	}
}

func TestCarbonfoxSymlinkSourceSwitchRejectsLostOwnershipEvenWithReplace(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, carbonfoxFixture, string)
		check  func(*testing.T, carbonfoxFixture, string)
	}{
		{
			name: "retargeted",
			mutate: func(t *testing.T, fixture carbonfoxFixture, target string) {
				t.Helper()
				external := filepath.Join(fixture.home, "operator-ghostty.conf")
				if err := os.WriteFile(external, []byte("operator\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, target); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, fixture carbonfoxFixture, target string) {
				t.Helper()
				want := filepath.Join(fixture.home, "operator-ghostty.conf")
				if got, err := os.Readlink(target); err != nil || got != want {
					t.Fatalf("retargeted link = %q, %v; want %s", got, err, want)
				}
			},
		},
		{
			name: "missing",
			mutate: func(t *testing.T, _ carbonfoxFixture, target string) {
				t.Helper()
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, _ carbonfoxFixture, target string) {
				t.Helper()
				assertCarbonfoxPathAbsent(t, target, "missing target was recreated")
			},
		},
		{
			name: "regular file",
			mutate: func(t *testing.T, _ carbonfoxFixture, target string) {
				t.Helper()
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte("operator\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, _ carbonfoxFixture, target string) {
				t.Helper()
				assertCarbonfoxFileBytes(t, target, []byte("operator\n"), "regular operator target changed")
			},
		},
		{
			name: "legacy record",
			mutate: func(t *testing.T, fixture carbonfoxFixture, target string) {
				t.Helper()
				metadata := fixture.metadata(t)
				for index := range metadata.Entries {
					if metadata.Entries[index].Target == target {
						metadata.Entries[index].Contributions = nil
						metadata.Entries[index].Ownership = ""
					}
				}
				if err := state.Save(state.Path(fixture.stateRoot), metadata); err != nil {
					t.Fatal(err)
				}
			},
			check: func(t *testing.T, fixture carbonfoxFixture, target string) {
				t.Helper()
				assertCarbonfoxSymlinkTarget(t, target, filepath.Join(fixture.repositoryRoot, "configs", "ghostty", "config.ghostty"), "legacy ownership rejection", "")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCarbonfoxFixture(t, "linux")
			fixture.install(t, false, "ghostty")
			target := filepath.Join(fixture.home, ".config", "ghostty", "config.ghostty")
			test.mutate(t, fixture, target)
			stateBefore := mustReadCarbonfoxFile(t, state.Path(fixture.stateRoot))

			output := fixture.transition(t, ExitError, true, false, "ghostty", carbonfoxTag)
			if !strings.Contains(output, "lost-ownership") {
				t.Fatalf("lost-ownership rejection lacks diagnosis:\n%s", output)
			}
			test.check(t, fixture, target)
			assertCarbonfoxFileBytes(t, state.Path(fixture.stateRoot), stateBefore, "lost-ownership rejection changed Installed Selection")
			assertCarbonfoxPathAbsent(t, filepath.Join(fixture.home, ".config", "dots", carbonfoxTag), "lost-ownership rejection installed preference marker")
			backupMetadata, err := backups.Load(backups.Path(fixture.stateRoot))
			if err != nil || len(backupMetadata.Sets) != 0 {
				t.Fatalf("lost-ownership rejection created Backup Sets: %#v, %v", backupMetadata.Sets, err)
			}
		})
	}
}

func TestRepositoryCarbonfoxSelectionTransitionsAndRerun(t *testing.T) {
	t.Run("baseline Carbonfox baseline", func(t *testing.T) {
		fixture := newCarbonfoxFixture(t, "linux")
		fixture.install(t, false, "atuin")
		target := filepath.Join(fixture.home, ".config", "atuin", "config.toml")
		assertCarbonfoxFileContains(t, target, `name = "catppuccin-mocha"`)

		fixture.install(t, false, "atuin", carbonfoxTag)
		assertCarbonfoxFileContains(t, target, `name = "carbonfox"`)
		before := mustReadCarbonfoxFile(t, target)
		output := fixture.install(t, false, "atuin", carbonfoxTag)
		if after := mustReadCarbonfoxFile(t, target); !bytes.Equal(after, before) {
			t.Fatalf("aligned Carbonfox rerun changed Atuin config")
		}
		if !strings.Contains(output, `"status": "unchanged"`) {
			t.Fatalf("aligned rerun did not report unchanged actions:\n%s", output)
		}

		fixture.install(t, true, "atuin")
		assertCarbonfoxFileContains(t, target, `name = "catppuccin-mocha"`)
		if _, err := os.Lstat(filepath.Join(fixture.home, ".config", "dots", carbonfoxTag)); !os.IsNotExist(err) {
			t.Fatalf("Carbonfox marker remains after acknowledged removal: %v", err)
		}
	})

	t.Run("adaptive Carbonfox adaptive", func(t *testing.T) {
		fixture := newCarbonfoxFixture(t, "darwin")
		target := filepath.Join(fixture.home, ".config", "herdr", "config.toml")
		fixture.install(t, false, "herdr", "adaptive-theme")
		assertCarbonfoxFileContains(t, target, "auto_switch = true")

		fixture.install(t, false, "herdr", carbonfoxTag, "adaptive-theme")
		assertCarbonfoxFileContains(t, target, `accent = "#78a9ff"`)
		fixture.install(t, true, "herdr", "adaptive-theme")
		assertCarbonfoxFileContains(t, target, "auto_switch = true")
		if bytes.Contains(mustReadCarbonfoxFile(t, target), []byte(`accent = "#78a9ff"`)) {
			t.Fatalf("Carbonfox-specific Herdr keys remain after adaptive restoration")
		}
	})
}

func TestRepositoryCarbonfoxDeclinedReductionLeavesSelectionUnchanged(t *testing.T) {
	fixture := newCarbonfoxFixture(t, "linux")
	fixture.install(t, false, "atuin", carbonfoxTag)
	target := filepath.Join(fixture.home, ".config", "atuin", "config.toml")
	beforeTarget := mustReadCarbonfoxFile(t, target)
	beforeState := mustReadCarbonfoxFile(t, state.Path(fixture.stateRoot))

	command := NewRootCommand()
	var output bytes.Buffer
	command.SetIn(strings.NewReader("n\n"))
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{
		"install", "--no-tui", "--skip-deps", "--tag", "atuin",
		"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.repositoryRoot, "--state-root", fixture.stateRoot,
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("declined reduction returned error: %v\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), "Installed Selection change declined; operation canceled before mutation.") {
		t.Fatalf("declined reduction was not reported:\n%s", output.String())
	}
	if after := mustReadCarbonfoxFile(t, target); !bytes.Equal(after, beforeTarget) {
		t.Fatalf("declined reduction changed Atuin config")
	}
	if after := mustReadCarbonfoxFile(t, state.Path(fixture.stateRoot)); !bytes.Equal(after, beforeState) {
		t.Fatalf("declined reduction changed Installation Metadata")
	}
}

func TestRepositoryCarbonfoxTransitionsRefuseDriftAndLostOwnership(t *testing.T) {
	t.Run("edited subset copy", func(t *testing.T) {
		fixture := newCarbonfoxFixture(t, "linux")
		fixture.install(t, false, "atuin")
		target := filepath.Join(fixture.home, ".config", "atuin", "config.toml")
		edited := bytes.Replace(mustReadCarbonfoxFile(t, target), []byte(`name = "catppuccin-mocha"`), []byte(`name = "operator-theme"`), 1)
		if err := os.WriteFile(target, edited, 0o600); err != nil {
			t.Fatal(err)
		}
		output := fixture.run(t, ExitOK,
			"install", "--yes", "--skip-deps", "--output", "json", "--tag", "atuin", "--tag", carbonfoxTag,
			"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.repositoryRoot, "--state-root", fixture.stateRoot,
		)
		if !strings.Contains(output, "ambiguous-partial-ownership") {
			t.Fatalf("edited subset refusal lacks ownership diagnosis:\n%s", output)
		}
		if after := mustReadCarbonfoxFile(t, target); !bytes.Equal(after, edited) {
			t.Fatalf("blocked Carbonfox transition overwrote edited Atuin config")
		}
		if got := fixture.metadata(t).InstalledSelection; got == nil || !reflect.DeepEqual(got.ResolvedTags, []string{"atuin", carbonfoxTag}) {
			t.Fatalf("safe conflict skip did not record requested selection: %#v", got)
		}
	})

	t.Run("lost whole-target ownership", func(t *testing.T) {
		fixture := newCarbonfoxFixture(t, "linux")
		fixture.install(t, false, "ghostty")
		target := filepath.Join(fixture.home, ".config", "ghostty", "config.ghostty")
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
		local := []byte("operator-owned\n")
		if err := os.WriteFile(target, local, 0o600); err != nil {
			t.Fatal(err)
		}
		output := fixture.run(t, ExitError,
			"install", "--yes", "--skip-deps", "--output", "json", "--tag", "ghostty", "--tag", carbonfoxTag,
			"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.repositoryRoot, "--state-root", fixture.stateRoot,
		)
		if !strings.Contains(output, "lost-ownership") {
			t.Fatalf("lost ownership refusal lacks diagnosis:\n%s", output)
		}
		if after := mustReadCarbonfoxFile(t, target); !bytes.Equal(after, local) {
			t.Fatalf("blocked Carbonfox transition overwrote operator-owned Ghostty config")
		}
		if got := fixture.metadata(t).InstalledSelection; got == nil || !reflect.DeepEqual(got.ResolvedTags, []string{"ghostty"}) {
			t.Fatalf("lost-ownership rejection changed selection: %#v", got)
		}
	})
}

func TestRepositoryCarbonfoxNativePreflightRunsWithSkipDepsBeforeApply(t *testing.T) {
	for _, test := range []struct {
		name       string
		tag        string
		tool       string
		stub       string
		want       []string
		removeStub bool
	}{
		{
			name: "old tuicr", tag: "tuicr", tool: "tuicr",
			stub: "#!/bin/sh\nprintf '%s\\n' 'tuicr 0.15.0'\n",
			want: []string{"Carbonfox requires tuicr", "0.16.1"},
		},
		{
			name: "missing Claude", tag: "claude", tool: "claude", removeStub: true,
			want: []string{"claude is missing from the selected environment PATH"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCarbonfoxFixture(t, "linux")
			toolPath := filepath.Join(fixture.stubDir, test.tool)
			if test.removeStub {
				if err := os.Remove(toolPath); err != nil {
					t.Fatal(err)
				}
			} else {
				writeCarbonfoxExecutable(t, toolPath, test.stub)
			}
			output := fixture.run(t, ExitError,
				"install", "--yes", "--skip-deps", "--output", "json", "--tag", test.tag, "--tag", carbonfoxTag,
				"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.repositoryRoot, "--state-root", fixture.stateRoot,
			)
			for _, want := range test.want {
				if !strings.Contains(output, want) {
					t.Fatalf("native preflight error lacks %q:\n%s", want, output)
				}
			}
			if _, err := os.Stat(state.Path(fixture.stateRoot)); !os.IsNotExist(err) {
				t.Fatalf("failed native preflight wrote Installation Metadata: %v", err)
			}
			entries, err := os.ReadDir(fixture.home)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("failed native preflight applied targets under selected home: %v", entries)
			}
		})
	}
}

func assertCarbonfoxFileContains(t *testing.T, path, want string) {
	t.Helper()
	if want == "" {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("expected Carbonfox path %s: %v", path, err)
		}
		return
	}
	content := mustReadCarbonfoxFile(t, path)
	if !bytes.Contains(content, []byte(want)) {
		t.Fatalf("%s does not contain %q", path, want)
	}
}

func assertCarbonfoxSymlinkTarget(t *testing.T, target, want, step, output string) {
	t.Helper()
	got, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("%s: resolve %s: %v\ninstall output:\n%s", step, target, err, output)
	}
	resolvedWant, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("%s: resolve expected source %s: %v", step, want, err)
	}
	if filepath.Clean(got) != filepath.Clean(resolvedWant) {
		t.Fatalf("%s: %s resolves to %s, want %s; selection transition silently skipped its owned source override\ninstall output:\n%s", step, target, got, want, output)
	}
}

func assertCarbonfoxSelection(t *testing.T, metadata state.Metadata, want []string) {
	t.Helper()
	if metadata.InstalledSelection == nil || !reflect.DeepEqual(metadata.InstalledSelection.ExtraTags, want) || !reflect.DeepEqual(metadata.InstalledSelection.ResolvedTags, want) {
		t.Fatalf("Installed Selection = %#v, want Tags %#v", metadata.InstalledSelection, want)
	}
}

func assertCarbonfoxBackupSymlink(t *testing.T, fixture carbonfoxFixture, target, wantDestination string, previousCount int) int {
	t.Helper()
	metadata, err := backups.Load(backups.Path(fixture.stateRoot))
	if err != nil {
		t.Fatalf("load Backup Metadata: %v", err)
	}
	matching := make([]backups.BackupSet, 0)
	for _, set := range metadata.Sets {
		for _, candidate := range set.Targets {
			if candidate == target {
				matching = append(matching, set)
				break
			}
		}
	}
	if len(matching) != previousCount+1 {
		t.Fatalf("Backup Sets protecting %s = %#v, want count %d", target, matching, previousCount+1)
	}
	set := matching[len(matching)-1]
	if set.Reason != "pre-install conflict protection" {
		t.Fatalf("Backup Set reason = %q, want pre-install conflict protection", set.Reason)
	}
	index := 0
	for i, candidate := range set.Targets {
		if candidate == target {
			index = i + 1
			break
		}
	}
	if index == 0 {
		t.Fatalf("Backup Set %#v does not contain %s", set, target)
	}
	preserved := backups.FilePath(fixture.stateRoot, set.ID, index, target)
	destination, err := os.Readlink(preserved)
	if err != nil {
		t.Fatalf("read preserved symlink %s: %v", preserved, err)
	}
	if filepath.Clean(destination) != filepath.Clean(wantDestination) {
		t.Fatalf("preserved symlink %s -> %s, want %s", preserved, destination, wantDestination)
	}
	return len(matching)
}

func assertCarbonfoxPathAbsent(t *testing.T, path, message string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("%s: %v", message, err)
	}
}

func assertCarbonfoxFileBytes(t *testing.T, path string, want []byte, message string) {
	t.Helper()
	if got := mustReadCarbonfoxFile(t, path); !bytes.Equal(got, want) {
		t.Fatalf("%s", message)
	}
}

func mustReadCarbonfoxFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func assertCarbonfoxInheritedHomeEmpty(t *testing.T, home string) {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != home {
			paths = append(paths, strings.TrimPrefix(path, home+string(os.PathSeparator)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	if len(paths) != 0 {
		t.Fatalf("install touched inherited HOME %s: %v", home, paths)
	}
}

func useCarbonfoxGhosttySourceCopy(t *testing.T, fixture *carbonfoxFixture) {
	t.Helper()
	sourceRoot := t.TempDir()
	for _, relative := range []string{
		"dots.yaml",
		"configs/dots/theme-carbonfox",
		"configs/ghostty/config.ghostty",
		"configs/ghostty/config-carbonfox.ghostty",
		"configs/ghostty/themes/Carbonfox",
	} {
		source := filepath.Join(fixture.repositoryRoot, filepath.FromSlash(relative))
		target := filepath.Join(sourceRoot, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fixture.repositoryRoot = sourceRoot
	fixture.manifestPath = filepath.Join(sourceRoot, "dots.yaml")
}

func carbonfoxApplicationInvocations(data []byte) string {
	var unexpected []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" || strings.HasPrefix(line, "git -C ") {
			continue
		}
		unexpected = append(unexpected, line)
	}
	return strings.Join(unexpected, "\n")
}
