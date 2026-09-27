package manifest_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

const carbonfoxShellRevision = "4dacd3f0185a2227bdf3b6c0975a8f0bf87cac9a"

func TestCarbonfoxShellMarkerExportsCanonicalPalette(t *testing.T) {
	root := repositoryRoot(t)
	marker := filepath.Join(root, "configs", "dots", "theme-carbonfox")
	cmd := exec.Command("sh", "-c", `. "$1"
dots_apply_carbonfox_ansi_palette
printf '%s\n' "$blue" "$mauve" "$green" "$yellow" "$subtext" "$overlay" "$red" "$pink" "$reset"
dots_carbonfox_fzf_colors`, "sh", marker)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("source Carbonfox marker: %v\n%s", err, out)
	}
	want := strings.Join([]string{
		`\033[38;2;120;169;255m`,
		`\033[38;2;190;149;255m`,
		`\033[38;2;37;190;106m`,
		`\033[38;2;8;189;186m`,
		`\033[38;2;182;184;187m`,
		`\033[38;2;110;111;112m`,
		`\033[38;2;238;83;150m`,
		`\033[38;2;255;126;182m`,
		`\033[0m`,
		"bg:#161616,bg+:#2a2a2a,fg:#f2f4f8,fg+:#f2f4f8,hl:#ee5396,hl+:#ee5396,info:#be95ff,marker:#78a9ff,pointer:#ff7eb6,prompt:#be95ff,spinner:#ff7eb6,border:#6e6f70",
	}, "\n") + "\n"
	if string(out) != want {
		t.Fatalf("Carbonfox shell palette mismatch\ngot:  %q\nwant: %q", out, want)
	}
}

func TestCarbonfoxAgentStatuslinesChangeOnlyPalette(t *testing.T) {
	root := repositoryRoot(t)
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "jq"), `#!/bin/sh
case "$*" in
  *model*) printf 'opus\n' ;;
esac
`)
	ansi := regexp.MustCompile("\\x1b\\[[0-9;]*m")

	for _, script := range []string{
		"configs/claude/statusline-command.sh",
		"configs/copilot/statusline-command.sh",
	} {
		t.Run(script, func(t *testing.T) {
			outputs := make(map[string]string)
			for _, theme := range []string{"baseline", "carbonfox"} {
				home := t.TempDir()
				if theme == "carbonfox" {
					dir := filepath.Join(home, ".config", "dots")
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
					marker, err := os.ReadFile(filepath.Join(root, "configs", "dots", "theme-carbonfox"))
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, "theme-carbonfox"), marker, 0o644); err != nil {
						t.Fatal(err)
					}
				}

				cmd := exec.Command("bash", filepath.Join(root, script))
				cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				cmd.Stdin = strings.NewReader(`{"model":{"display_name":"opus","displayName":"opus"}}`)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s %s: %v\n%s", script, theme, err, out)
				}
				outputs[theme] = string(out)
			}

			if !strings.Contains(outputs["baseline"], "\x1b[38;2;166;173;200m") {
				t.Fatalf("baseline did not retain Mocha fallback: %q", outputs["baseline"])
			}
			if !strings.Contains(outputs["carbonfox"], "\x1b[38;2;182;184;187m") {
				t.Fatalf("Carbonfox did not use canonical foreground: %q", outputs["carbonfox"])
			}
			if got, want := ansi.ReplaceAllString(outputs["carbonfox"], ""), ansi.ReplaceAllString(outputs["baseline"], ""); got != want {
				t.Fatalf("non-color statusline output changed\ngot:  %q\nwant: %q", got, want)
			}
		})
	}
}

func TestCarbonfoxZshFzfPathsChangeOnlyColors(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not installed")
	}
	root := repositoryRoot(t)
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "fzf"), "#!/bin/sh\nexit 0\n")

	run := func(t *testing.T, carbonfox bool, source, probe string) string {
		t.Helper()
		home := t.TempDir()
		if carbonfox {
			dir := filepath.Join(home, ".config", "dots")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			marker, err := os.ReadFile(filepath.Join(root, "configs", "dots", "theme-carbonfox"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "theme-carbonfox"), marker, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(zsh, "-f", "-c", `source "$1"; eval "$2"`, "zsh", filepath.Join(root, source), probe)
		cmd.Env = append(os.Environ(), "HOME="+home, "PATH="+bin)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("source %s: %v\n%s", source, err, out)
		}
		return string(out)
	}

	preProbe := `zstyle -a ':fzf-tab:*' fzf-flags flags; print -r -- "${(j:|:)flags}"`
	preBase := run(t, false, "configs/zsh/rc.d/pre/20-modules.zsh", preProbe)
	preCarbonfox := run(t, true, "configs/zsh/rc.d/pre/20-modules.zsh", preProbe)
	for _, fixed := range []string{"--height=40%", "--min-height=12", "--layout=reverse", "--border=top"} {
		if !strings.Contains(preBase, fixed) || !strings.Contains(preCarbonfox, fixed) {
			t.Fatalf("fzf-tab fixed option %q changed\nbaseline: %q\nCarbonfox: %q", fixed, preBase, preCarbonfox)
		}
	}
	if !strings.Contains(preBase, "bg:#1e1e2e") || !strings.Contains(preCarbonfox, "bg:#161616") {
		t.Fatalf("fzf-tab palette dispatch failed\nbaseline: %q\nCarbonfox: %q", preBase, preCarbonfox)
	}

	postProbe := `print -r -- "$FZF_DEFAULT_OPTS"; print -r -- "$FZF_CTRL_T_OPTS"; print -r -- "$FZF_ALT_C_OPTS"`
	postBase := run(t, false, "configs/zsh/rc.d/post/40-tools.zsh", postProbe)
	postCarbonfox := run(t, true, "configs/zsh/rc.d/post/40-tools.zsh", postProbe)
	if !strings.Contains(postBase, "bg:#1e1e2e") || !strings.Contains(postCarbonfox, "bg:#161616") {
		t.Fatalf("fzf default palette dispatch failed\nbaseline: %q\nCarbonfox: %q", postBase, postCarbonfox)
	}
	for _, fixed := range []string{"--height=40%", "--walker-skip=.git,node_modules,target", "bat --color=always", "eza -a --color=always"} {
		if !strings.Contains(postBase, fixed) || !strings.Contains(postCarbonfox, fixed) {
			t.Fatalf("fzf fixed behavior %q changed\nbaseline: %q\nCarbonfox: %q", fixed, postBase, postCarbonfox)
		}
	}
}

func TestCarbonfoxNeovimSelectorAndPin(t *testing.T) {
	root := repositoryRoot(t)
	lockBytes, err := os.ReadFile(filepath.Join(root, "configs", "nvim", "lazy-lock.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock map[string]struct {
		Commit string `json:"commit"`
	}
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		t.Fatal(err)
	}
	if got := lock["nightfox.nvim"].Commit; got != carbonfoxShellRevision {
		t.Fatalf("Nightfox pin = %q, want %q", got, carbonfoxShellRevision)
	}

	nvim, err := exec.LookPath("nvim")
	if err != nil {
		t.Skip("nvim is not installed")
	}
	config := filepath.Join(root, "configs", "nvim", "lua", "plugins", "colorscheme.lua")
	for _, tt := range []struct {
		name       string
		marker     bool
		wantPlugin string
		wantTheme  string
	}{
		{name: "baseline", wantPlugin: "catppuccin/nvim", wantTheme: "catppuccin-mocha"},
		{name: "Carbonfox", marker: true, wantPlugin: "EdenEast/nightfox.nvim", wantTheme: "carbonfox"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			if tt.marker {
				dir := filepath.Join(home, ".config", "dots")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "theme-carbonfox"), nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			lua := `local s=dofile(vim.env.DOTS_COLORSCHEME_TEST); print(s[1][1][1]); print(s[1][2].opts.colorscheme)`
			cmd := exec.Command(nvim, "--clean", "--headless", "-u", "NONE", "-c", "lua "+lua, "+qa")
			cmd.Env = append(os.Environ(), "HOME="+home, "DOTS_COLORSCHEME_TEST="+config)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("nvim selector: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), tt.wantPlugin) || !strings.Contains(string(out), tt.wantTheme) {
				t.Fatalf("selector output %q, want plugin %q and theme %q", out, tt.wantPlugin, tt.wantTheme)
			}
		})
	}
}

func TestCarbonfoxTmuxPaletteAndReloadOrder(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	root := repositoryRoot(t)
	configBytes, err := os.ReadFile(filepath.Join(root, "configs", "tmux", "tmux.conf"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(configBytes)
	pluginLoad := `run "$HOME/.tmux/plugins/tmux/catppuccin.tmux"`
	first := strings.Index(config, pluginLoad)
	adapter := strings.Index(config, `source-file "$HOME/.config/tmux/carbonfox.conf"`)
	second := strings.LastIndex(config, pluginLoad)
	if first == -1 || adapter < first || second <= adapter {
		t.Fatalf("Carbonfox palette must load between Catppuccin reset and module generation")
	}

	socketFile, err := os.CreateTemp("", "dots-carbonfox-tmux-")
	if err != nil {
		t.Fatal(err)
	}
	socket := socketFile.Name()
	if err := socketFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(socket); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(socket) })
	cmd := exec.Command(tmux, "-S", socket, "-f", "/dev/null", "new-session", "-d")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("start isolated tmux: %v\n%s", err, out)
	}
	defer func() {
		kill := exec.Command(tmux, "-S", socket, "kill-server")
		_ = kill.Run()
	}()
	palette := filepath.Join(root, "configs", "tmux", "carbonfox.conf")
	source := exec.Command(tmux, "-S", socket, "source-file", palette)
	if out, err := source.CombinedOutput(); err != nil {
		t.Fatalf("source Carbonfox tmux palette: %v\n%s", err, out)
	}
	for option, want := range map[string]string{
		"@thm_bg":        "#161616",
		"@thm_fg":        "#f2f4f8",
		"@thm_red":       "#ee5396",
		"@thm_green":     "#25be6a",
		"@thm_blue":      "#78a9ff",
		"@thm_mauve":     "#be95ff",
		"@thm_surface_0": "#2a2a2a",
		"@thm_crust":     "#0c0c0c",
	} {
		show := exec.Command(tmux, "-S", socket, "show-options", "-gqv", option)
		out, err := show.CombinedOutput()
		if err != nil {
			t.Fatalf("show %s: %v\n%s", option, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Fatalf("%s = %q, want %q", option, got, want)
		}
	}
}

func TestCarbonfoxTmuxReloadRestoresCatppuccinPalette(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	root := repositoryRoot(t)
	home := t.TempDir()
	pluginDir := filepath.Join(home, ".tmux", "plugins", "tmux")
	configDir := filepath.Join(home, ".config", "tmux")
	markerDir := filepath.Join(home, ".config", "dots")
	for _, dir := range []string{pluginDir, configDir, markerDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	copyFile := func(source, target string, mode os.FileMode) {
		t.Helper()
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, mode); err != nil {
			t.Fatal(err)
		}
	}
	config := filepath.Join(home, ".tmux.conf")
	copyFile(filepath.Join(root, "configs", "tmux", "tmux.conf"), config, 0o644)
	copyFile(filepath.Join(root, "configs", "tmux", "carbonfox.conf"), filepath.Join(configDir, "carbonfox.conf"), 0o644)
	marker := filepath.Join(markerDir, "theme-carbonfox")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(pluginDir, "catppuccin.tmux"), `#!/bin/sh
if [ "$(tmux show-options -gqv @catppuccin_reset)" = true ]; then
  tmux set-option -gu @thm_bg
  tmux set-option -gu @thm_fg
  tmux set-option -gu @thm_crust
  tmux set-option -gu @catppuccin_reset
fi
tmux set-option -ogq @thm_bg '#1e1e2e'
tmux set-option -ogq @thm_fg '#cdd6f4'
tmux set-option -ogq @thm_crust '#11111b'
tmux set-option -g @catppuccin_status_directory 'directory'
tmux set-option -g @catppuccin_status_cpu 'cpu'
tmux set-option -g @catppuccin_status_ram 'ram'
tmux set-option -g @catppuccin_status_session 'session'
tmux set-option -g @catppuccin_status_uptime 'uptime'
`)

	socketFile, err := os.CreateTemp("", "dots-carbonfox-reload-")
	if err != nil {
		t.Fatal(err)
	}
	socket := socketFile.Name()
	if err := socketFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(socket); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(socket) })
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(tmux, append([]string{"-S", socket}, args...)...)
		cmd.Env = append(os.Environ(), "HOME="+home)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("-f", config, "new-session", "-d")
	defer func() {
		kill := exec.Command(tmux, "-S", socket, "kill-server")
		_ = kill.Run()
	}()
	if got := run("show-options", "-gqv", "@thm_bg"); got != "#161616" {
		t.Fatalf("Carbonfox tmux background = %q", got)
	}
	statusRight := run("show-options", "-gqv", "status-right")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	run("source-file", config)
	if got := run("show-options", "-gqv", "@thm_bg"); got != "#1e1e2e" {
		t.Fatalf("tmux background after Carbonfox removal = %q, want Catppuccin Mocha", got)
	}
	if got := run("show-options", "-gqv", "status-right"); got != statusRight {
		t.Fatalf("tmux status layout changed across theme reload\nbefore: %q\nafter:  %q", statusRight, got)
	}
}

func TestCarbonfoxClaudeSettingsPreserveNonThemeConfiguration(t *testing.T) {
	root := repositoryRoot(t)
	read := func(t *testing.T, path string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	baseline := read(t, "configs/claude/settings.json")
	carbonfox := read(t, "configs/claude/settings-carbonfox.json")
	if got := carbonfox["theme"]; got != "custom:carbonfox" {
		t.Fatalf("Carbonfox Claude theme = %#v", got)
	}
	delete(baseline, "theme")
	delete(carbonfox, "theme")
	if !reflect.DeepEqual(carbonfox, baseline) {
		t.Fatalf("Claude settings variants differ outside theme\nCarbonfox: %#v\nbaseline: %#v", carbonfox, baseline)
	}

	theme := read(t, "configs/claude/themes/carbonfox.json")
	if theme["name"] != "Carbonfox" || theme["base"] != "dark" {
		t.Fatalf("unexpected Claude custom theme identity: %#v", theme)
	}
	overrides, ok := theme["overrides"].(map[string]any)
	if !ok {
		t.Fatal("Claude theme overrides are missing")
	}
	for token, want := range map[string]string{
		"text":        "#f2f4f8",
		"success":     "#25be6a",
		"error":       "#ee5396",
		"warning":     "#08bdba",
		"selectionBg": "#525253",
	} {
		if got := overrides[token]; got != want {
			t.Fatalf("Claude theme token %s = %#v, want %q", token, got, want)
		}
	}
	if _, exists := overrides["effortUltra"]; exists {
		t.Fatal("effortUltra requires Claude Code newer than the supported 2.1.220 floor")
	}
}
