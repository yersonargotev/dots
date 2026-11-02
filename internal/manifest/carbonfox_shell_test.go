package manifest_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
