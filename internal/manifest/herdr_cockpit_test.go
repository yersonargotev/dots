package manifest_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

type herdrCockpitConfig struct {
	UI struct {
		SidebarWidth            int                `toml:"sidebar_width"`
		SidebarMinWidth         int                `toml:"sidebar_min_width"`
		SidebarMaxWidth         int                `toml:"sidebar_max_width"`
		PaneBorders             string             `toml:"pane_borders"`
		PaneOuterBorders        bool               `toml:"pane_outer_borders"`
		PaneScrollbars          bool               `toml:"pane_scrollbars"`
		PaneGaps                bool               `toml:"pane_gaps"`
		HideTabBarWhenSingleTab bool               `toml:"hide_tab_bar_when_single_tab"`
		TabBarPosition          string             `toml:"tab_bar_position"`
		TabBarRight             []herdrTabBarRight `toml:"tab_bar_right"`
		TabBarRightSeparator    string             `toml:"tab_bar_right_separator"`
		StatusIndicators        string             `toml:"status_indicators"`
	} `toml:"ui"`
	Keys struct {
		Command []herdrPopupCommand `toml:"command"`
	} `toml:"keys"`
}

type herdrTabBarRight struct {
	Type   string `toml:"type"`
	Format string `toml:"format"`
}

type herdrPopupCommand struct {
	Key         string `toml:"key"`
	Type        string `toml:"type"`
	Command     string `toml:"command"`
	Description string `toml:"description"`
	Width       string `toml:"width"`
	Height      string `toml:"height"`
}

func TestRepositoryHerdrCockpitMatchesApprovedLayout(t *testing.T) {
	for _, name := range []string{"config.toml", "config-adaptive.toml"} {
		t.Run(name, func(t *testing.T) {
			config := loadHerdrCockpitConfig(t, name)
			if got, want := []int{config.UI.SidebarWidth, config.UI.SidebarMinWidth, config.UI.SidebarMaxWidth}, []int{26, 18, 36}; !reflect.DeepEqual(got, want) {
				t.Fatalf("sidebar widths = %v, want %v", got, want)
			}
			if config.UI.PaneBorders != "auto" || config.UI.PaneOuterBorders || config.UI.PaneScrollbars || config.UI.PaneGaps {
				t.Fatalf("pane chrome = borders %q, outer %t, scrollbars %t, gaps %t", config.UI.PaneBorders, config.UI.PaneOuterBorders, config.UI.PaneScrollbars, config.UI.PaneGaps)
			}
			if !config.UI.HideTabBarWhenSingleTab || config.UI.TabBarPosition != "top" {
				t.Fatalf("tab row = hide-single %t, position %q", config.UI.HideTabBarWhenSingleTab, config.UI.TabBarPosition)
			}
			wantStatus := []herdrTabBarRight{{Type: "zoom"}, {Type: "hostname"}, {Type: "datetime", Format: "%H:%M"}}
			if !reflect.DeepEqual(config.UI.TabBarRight, wantStatus) || config.UI.TabBarRightSeparator != " · " {
				t.Fatalf("right status = %#v separated by %q, want %#v", config.UI.TabBarRight, config.UI.TabBarRightSeparator, wantStatus)
			}
			if config.UI.StatusIndicators != "symbols" {
				t.Fatalf("status indicators = %q, want symbols", config.UI.StatusIndicators)
			}

			wantKeys := []string{"prefix+t", "prefix+o", "prefix+alt+g", "prefix+alt+t", "prefix+alt+f"}
			if len(config.Keys.Command) != len(wantKeys) {
				t.Fatalf("custom commands = %#v, want keys %v", config.Keys.Command, wantKeys)
			}
			seenKeys := make(map[string]bool, len(config.Keys.Command))
			for i, command := range config.Keys.Command {
				if command.Key != wantKeys[i] {
					t.Errorf("custom command %d = %#v, want key %q", i, command, wantKeys[i])
				}
				if seenKeys[command.Key] {
					t.Errorf("custom key %q is declared more than once", command.Key)
				}
				seenKeys[command.Key] = true
			}

			commands := herdrPopupCommandsByKey(t, config)
			wantPluginActions := map[string]string{
				"prefix+t": "rmarganti.herdr-pluck.pluck",
				"prefix+o": "rmarganti.herdr-pluck.open-url",
			}
			for key, wantCommand := range wantPluginActions {
				action := commands[key]
				if action.Type != "plugin_action" || action.Command != wantCommand || action.Width != "" || action.Height != "" {
					t.Errorf("Pluck action %q = %#v, want plugin_action %q", key, action, wantCommand)
				}
			}
			for _, key := range []string{"prefix+alt+g", "prefix+alt+t", "prefix+alt+f"} {
				popup := commands[key]
				if popup.Type != "popup" || popup.Width != "80%" || popup.Height != "80%" {
					t.Errorf("popup %q = %#v, want 80%% popup", key, popup)
				}
				for _, forbidden := range []string{"brew ", "curl ", "wget ", "git clone", "npm install"} {
					if strings.Contains(popup.Command, forbidden) {
						t.Errorf("popup %q can install software or reach the network through %q", popup.Key, forbidden)
					}
				}
			}
		})
	}
}

func TestRepositoryHerdrConfigsPassInstalledHerdrValidation(t *testing.T) {
	herdr, err := exec.LookPath("herdr")
	if err != nil {
		t.Skip("Herdr is not installed")
	}
	for _, name := range []string{"config.toml", "config-adaptive.toml"} {
		t.Run(name, func(t *testing.T) {
			root := repositoryRoot(t)
			home := t.TempDir()
			cmd := exec.Command(herdr, "config", "check")
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
				"HERDR_CONFIG_PATH="+filepath.Join(root, "configs", "herdr", name),
				"HERDR_SESSION=dots-issue-512-check",
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("herdr config check: %v\n%s", err, output)
			}
		})
	}
}

func TestHerdrTagSelectsPopupDependenciesOnBothMacArchitectures(t *testing.T) {
	root := repositoryRoot(t)
	m, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"herdr", "lazygit", "fzf", "fd", "bat", "git"}
	for _, arch := range []string{"arm64", "amd64"} {
		t.Run(arch, func(t *testing.T) {
			surface := selectedsurface.EvaluateForPlatform(*m, []string{"herdr"}, "darwin", arch)
			got := make(map[string]bool, len(surface.Dependencies))
			for _, dependency := range surface.Dependencies {
				got[dependency.Name] = true
			}
			for _, name := range want {
				if !got[name] {
					t.Errorf("herdr Selected Surface on %s misses %q; got %v", arch, name, got)
				}
			}
		})
	}
}

func TestHerdrTagSelectsPluckOnlyOnAppleSilicon(t *testing.T) {
	root := repositoryRoot(t)
	m, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	arm64 := selectedsurface.EvaluateForPlatform(*m, []string{"herdr"}, "darwin", "arm64")
	amd64 := selectedsurface.EvaluateForPlatform(*m, []string{"herdr"}, "darwin", "amd64")

	const plugin = "rmarganti/herdr-pluck"
	const ref = "d1eacb80956c3a23ab6f7428a9e83961fb86ba28"
	found := false
	for _, provisioner := range arm64.Provisioners {
		if provisioner.Spec.Plugin != plugin {
			continue
		}
		found = true
		if provisioner.Spec.Ref != ref || !reflect.DeepEqual(provisioner.OS, []string{"darwin"}) || !reflect.DeepEqual(provisioner.Arch, []string{"arm64"}) {
			t.Fatalf("Apple Silicon Pluck Provisioner = %#v, want reviewed darwin/arm64 pin", provisioner)
		}
	}
	if !found {
		t.Fatalf("Apple Silicon Selected Surface omitted %s@%s", plugin, ref)
	}
	for _, provisioner := range amd64.Provisioners {
		if provisioner.Spec.Plugin == plugin {
			t.Fatalf("Intel Selected Surface included Apple Silicon-only Pluck: %#v", provisioner)
		}
	}

	for _, name := range []string{"curl", "tar", "pbcopy", "open", "Rust stable (rustup)"} {
		if !selectedDependencyNamed(arm64, name) {
			t.Errorf("Apple Silicon Pluck surface misses Dependency %q", name)
		}
	}
	if selectedDependencyNamed(amd64, "curl") || selectedDependencyNamed(amd64, "tar") || selectedDependencyNamed(amd64, "pbcopy") || selectedDependencyNamed(amd64, "open") || selectedDependencyNamed(amd64, "Rust stable (rustup)") {
		t.Fatalf("Intel Selected Surface retained Pluck-only Dependencies: %#v", amd64.Dependencies)
	}
}

func selectedDependencyNamed(surface selectedsurface.Surface, name string) bool {
	for _, dependency := range surface.Dependencies {
		if dependency.Name == name {
			return true
		}
	}
	return false
}

func TestHerdrPopupWorkflowsAreSandboxedAndContextAware(t *testing.T) {
	config := loadHerdrCockpitConfig(t, "config.toml")
	commands := herdrPopupCommandsByKey(t, config)

	t.Run("LazyGit inherits focused directory", func(t *testing.T) {
		sandbox := newPopupSandbox(t, "lazygit")
		focused := t.TempDir()
		sandbox.writeExecutable("lazygit", `printf '%s\n' "$PWD" > "$CAPTURE/lazygit.cwd"`)
		result := sandbox.run(commands["prefix+alt+g"], focused, nil)
		result.requireSuccess(t)
		if got := strings.TrimSpace(sandbox.read(t, "lazygit.cwd")); !sameDirectory(t, got, focused) {
			t.Fatalf("lazygit cwd = %q, want %q", got, focused)
		}
	})

	t.Run("scratch uses selected shell", func(t *testing.T) {
		sandbox := newPopupSandbox(t, "selected-shell")
		shell := sandbox.writeExecutable("selected-shell", `printf '%s\n' "$PWD" > "$CAPTURE/shell.cwd"`)
		focused := t.TempDir()
		result := sandbox.run(commands["prefix+alt+t"], focused, []string{"SHELL=" + shell})
		result.requireSuccess(t)
		if got := strings.TrimSpace(sandbox.read(t, "shell.cwd")); !sameDirectory(t, got, focused) {
			t.Fatalf("scratch shell cwd = %q, want %q", got, focused)
		}
	})

	t.Run("scratch falls back to sh", func(t *testing.T) {
		sandbox := newPopupSandbox(t, "shell-fallback")
		result := sandbox.run(commands["prefix+alt+t"], t.TempDir(), []string{"SHELL=/missing/dots-shell"})
		result.requireSuccess(t)
		if !strings.Contains(result.stderr, "using /bin/sh") {
			t.Fatalf("stderr = %q, want fallback message", result.stderr)
		}
	})

	t.Run("finder uses Git root and VISUAL", func(t *testing.T) {
		sandbox := newFinderSandbox(t)
		repository := t.TempDir()
		focused := filepath.Join(repository, "nested")
		for _, path := range []string{"nested", "src", ".git", "node_modules", "target"} {
			if err := os.Mkdir(filepath.Join(repository, path), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		for _, path := range []string{"src/main.go", ".hidden", ".git/ignored", "node_modules/ignored.js", "target/ignored"} {
			if err := os.WriteFile(filepath.Join(repository, path), []byte(path), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		sandbox.gitRoot = repository
		result := sandbox.runFinder(commands["prefix+alt+f"], focused, "src/main.go", false)
		result.requireSuccess(t)

		if got := strings.TrimSpace(sandbox.popupSandbox.read(t, "git.args")); got != "-C "+focused+" rev-parse --show-toplevel" {
			t.Fatalf("git args = %q", got)
		}
		fdArgs := sandbox.popupSandbox.read(t, "fd.args")
		for _, want := range []string{"--hidden", "--type", "f", "--exclude", ".git", "node_modules", "target"} {
			if !strings.Contains(fdArgs, want) {
				t.Errorf("fd args %q miss %q", fdArgs, want)
			}
		}
		if preview := sandbox.popupSandbox.read(t, "fzf.args"); !strings.Contains(preview, "bat --color=always") {
			t.Fatalf("fzf args = %q, want bat preview", preview)
		}
		finderInput := sandbox.popupSandbox.read(t, "fzf.input")
		for _, want := range []string{"src/main.go", ".hidden"} {
			if !strings.Contains(finderInput, want) {
				t.Errorf("finder input %q misses %q", finderInput, want)
			}
		}
		for _, excluded := range []string{".git/ignored", "node_modules/ignored.js", "target/ignored"} {
			if strings.Contains(finderInput, excluded) {
				t.Errorf("finder input %q includes excluded %q", finderInput, excluded)
			}
		}
		if got := strings.TrimSpace(sandbox.popupSandbox.read(t, "editor.cwd")); !sameDirectory(t, got, repository) {
			t.Fatalf("editor cwd = %q, want Git root %q", got, repository)
		}
		if got := strings.TrimSpace(sandbox.popupSandbox.read(t, "editor.args")); got != "src/main.go" {
			t.Fatalf("editor args = %q, want selected file", got)
		}
	})

	t.Run("finder preserves editor arguments without evaluating them", func(t *testing.T) {
		sandbox := newFinderSandbox(t)
		focused := t.TempDir()
		result := sandbox.run(commands["prefix+alt+f"], focused, []string{
			"VISUAL=visual --wait ; touch " + filepath.Join(sandbox.capture, "injected"),
			"GIT_FAIL=1",
			"FZF_SELECTION=.hidden",
		})
		result.requireSuccess(t)
		if got := strings.TrimSpace(sandbox.read(t, "editor.args")); got != "--wait ; touch "+filepath.Join(sandbox.capture, "injected")+" .hidden" {
			t.Fatalf("editor args = %q, want configured argv followed by selected file", got)
		}
		if _, err := os.Stat(filepath.Join(sandbox.capture, "injected")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("editor metacharacters were evaluated: %v", err)
		}
	})

	t.Run("finder falls back from unavailable VISUAL to EDITOR arguments", func(t *testing.T) {
		sandbox := newFinderSandbox(t)
		focused := t.TempDir()
		result := sandbox.run(commands["prefix+alt+f"], focused, []string{
			"VISUAL=missing-visual --wait",
			"EDITOR=visual --reuse-window",
			"GIT_FAIL=1",
			"FZF_SELECTION=.hidden",
		})
		result.requireSuccess(t)
		if got := strings.TrimSpace(sandbox.read(t, "editor.args")); got != "--reuse-window .hidden" {
			t.Fatalf("EDITOR fallback args = %q, want configured argument followed by selected file", got)
		}
	})

	t.Run("finder falls back to focused directory", func(t *testing.T) {
		sandbox := newFinderSandbox(t)
		focused := t.TempDir()
		result := sandbox.runFinder(commands["prefix+alt+f"], focused, ".hidden", true)
		result.requireSuccess(t)
		if got := strings.TrimSpace(sandbox.popupSandbox.read(t, "editor.cwd")); !sameDirectory(t, got, focused) {
			t.Fatalf("editor cwd = %q, want fallback %q", got, focused)
		}
	})

	t.Run("finder cancellation changes nothing", func(t *testing.T) {
		sandbox := newFinderSandbox(t)
		focused := t.TempDir()
		result := sandbox.runFinder(commands["prefix+alt+f"], focused, "", true)
		result.requireSuccess(t)
		if _, err := os.Stat(filepath.Join(sandbox.capture, "editor.args")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("editor ran after cancellation: %v", err)
		}
	})

	for _, missing := range []string{"git", "fd", "fzf", "bat"} {
		t.Run("finder reports missing "+missing, func(t *testing.T) {
			sandbox := newFinderSandbox(t)
			if err := os.Remove(filepath.Join(sandbox.bin, missing)); err != nil {
				t.Fatal(err)
			}
			result := sandbox.run(commands["prefix+alt+f"], t.TempDir(), []string{"VISUAL=visual"})
			result.requireExitCode(t, 127)
			if !strings.Contains(result.stderr, "dots: "+missing+" is required") {
				t.Fatalf("stderr = %q, want missing %s", result.stderr, missing)
			}
		})
	}

	t.Run("finder reports missing editor fallback chain", func(t *testing.T) {
		sandbox := newFinderSandbox(t)
		if err := os.Remove(filepath.Join(sandbox.bin, "visual")); err != nil {
			t.Fatal(err)
		}
		result := sandbox.run(commands["prefix+alt+f"], t.TempDir(), []string{
			"VISUAL=/missing/dots-visual",
			"EDITOR=/missing/dots-editor",
		})
		result.requireExitCode(t, 127)
		if !strings.Contains(result.stderr, "VISUAL, EDITOR, or vi is required") {
			t.Fatalf("stderr = %q, want missing editor message", result.stderr)
		}
	})

	t.Run("LazyGit reports missing command", func(t *testing.T) {
		sandbox := newPopupSandbox(t, "missing-lazygit")
		result := sandbox.run(commands["prefix+alt+g"], t.TempDir(), nil)
		result.requireExitCode(t, 127)
		if !strings.Contains(result.stderr, "dots: lazygit is required") {
			t.Fatalf("stderr = %q", result.stderr)
		}
	})
}

type popupSandbox struct {
	t       *testing.T
	bin     string
	capture string
}

type finderSandbox struct {
	*popupSandbox
	gitRoot string
}

type popupResult struct {
	err    error
	stderr string
}

func newPopupSandbox(t *testing.T, name string) *popupSandbox {
	t.Helper()
	root := t.TempDir()
	return &popupSandbox{t: t, bin: filepath.Join(root, "bin"), capture: filepath.Join(root, name)}
}

func (s *popupSandbox) writeExecutable(name, body string) string {
	s.t.Helper()
	if err := os.MkdirAll(s.bin, 0o700); err != nil {
		s.t.Fatal(err)
	}
	if err := os.MkdirAll(s.capture, 0o700); err != nil {
		s.t.Fatal(err)
	}
	path := filepath.Join(s.bin, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o700); err != nil {
		s.t.Fatal(err)
	}
	return path
}

func (s *popupSandbox) run(command herdrPopupCommand, directory string, extraEnv []string) popupResult {
	s.t.Helper()
	cmd := exec.Command("/bin/sh", "-c", command.Command)
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader("\n")
	cmd.Env = append([]string{
		"PATH=" + s.bin,
		"HOME=" + s.t.TempDir(),
		"CAPTURE=" + s.capture,
		"HERDR_ACTIVE_PANE_CWD=" + directory,
	}, extraEnv...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	return popupResult{err: err, stderr: stderr.String()}
}

func (s *popupSandbox) read(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.capture, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func newFinderSandbox(t *testing.T) *finderSandbox {
	t.Helper()
	sandbox := newPopupSandbox(t, "finder")
	finder := &finderSandbox{popupSandbox: sandbox}
	sandbox.writeExecutable("git", `
printf '%s\n' "$*" > "$CAPTURE/git.args"
if [ "${GIT_FAIL:-}" = 1 ]; then exit 1; fi
printf '%s\n' "$GIT_ROOT"
`)
	sandbox.writeExecutable("fd", `
printf '%s\n' "$*" > "$CAPTURE/fd.args"
printf '%s\n' 'src/main.go' '.hidden'
`)
	sandbox.writeExecutable("fzf", `
printf '%s\n' "$*" > "$CAPTURE/fzf.args"
: > "$CAPTURE/fzf.input"
while IFS= read -r line; do printf '%s\n' "$line" >> "$CAPTURE/fzf.input"; done
if [ "${FZF_CANCEL:-}" = 1 ]; then exit 130; fi
printf '%s\n' "$FZF_SELECTION"
`)
	sandbox.writeExecutable("bat", `exit 0`)
	sandbox.writeExecutable("visual", `
printf '%s\n' "$PWD" > "$CAPTURE/editor.cwd"
printf '%s\n' "$*" > "$CAPTURE/editor.args"
`)
	return finder
}

func (s *finderSandbox) runFinder(command herdrPopupCommand, focused, selection string, gitFail bool) popupResult {
	s.t.Helper()
	extra := []string{
		"VISUAL=visual",
		"GIT_ROOT=" + s.gitRoot,
		"FZF_SELECTION=" + selection,
	}
	if gitFail {
		extra = append(extra, "GIT_FAIL=1")
	}
	if selection == "" {
		extra = append(extra, "FZF_CANCEL=1")
	}
	return s.run(command, focused, extra)
}

func (r popupResult) requireSuccess(t *testing.T) {
	t.Helper()
	if r.err != nil {
		t.Fatalf("popup error = %v, stderr = %q", r.err, r.stderr)
	}
}

func (r popupResult) requireExitCode(t *testing.T, want int) {
	t.Helper()
	var exitErr *exec.ExitError
	if !errors.As(r.err, &exitErr) || exitErr.ExitCode() != want {
		t.Fatalf("popup error = %v, stderr = %q; want exit %d", r.err, r.stderr, want)
	}
}

func loadHerdrCockpitConfig(t *testing.T, name string) herdrCockpitConfig {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "configs", "herdr", name))
	if err != nil {
		t.Fatal(err)
	}
	var config herdrCockpitConfig
	if err := toml.Unmarshal(data, &config); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return config
}

func herdrPopupCommandsByKey(t *testing.T, config herdrCockpitConfig) map[string]herdrPopupCommand {
	t.Helper()
	commands := make(map[string]herdrPopupCommand, len(config.Keys.Command))
	for _, command := range config.Keys.Command {
		if _, exists := commands[command.Key]; exists {
			t.Fatalf("duplicate Herdr custom key %q", command.Key)
		}
		commands[command.Key] = command
	}
	return commands
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func sameDirectory(t *testing.T, left, right string) bool {
	t.Helper()
	leftInfo, err := os.Stat(left)
	if err != nil {
		t.Fatal(err)
	}
	rightInfo, err := os.Stat(right)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(leftInfo, rightInfo)
}
