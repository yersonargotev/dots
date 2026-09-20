package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedZshFuzzyNavigationBindings(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not available")
	}

	root := filepath.Clean(filepath.Join("..", ".."))
	zimrc, err := os.ReadFile(filepath.Join(root, "configs", "zsh", "zimrc"))
	if err != nil {
		t.Fatal(err)
	}
	zimModules := string(zimrc)
	completion := strings.Index(zimModules, "zmodule completion")
	fzfTab := strings.Index(zimModules, "zmodule Aloxaf/fzf-tab")
	autosuggestions := strings.Index(zimModules, "zmodule zsh-users/zsh-autosuggestions")
	if completion < 0 || fzfTab < completion || autosuggestions < fzfTab {
		t.Fatalf("fzf-tab module order does not place it between completion and widget wrappers:\n%s", zimModules)
	}

	home := t.TempDir()
	fakeBin := filepath.Join(home, "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".zim"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zim", "init.zsh"), []byte(`
_fzf_tab_widget() { :; }
zle -N _fzf_tab_widget
bindkey '^I' _fzf_tab_widget
`), 0o600); err != nil {
		t.Fatal(err)
	}

	writeZshTestExecutable(t, fakeBin, "fzf", `#!/bin/sh
printf 'fzf:%s\n' "$*" >>"$DOTS_TEST_LOG"
[ "$1" = "--zsh" ] || exit 1
cat <<'ZSH'
fzf-file-widget() { :; }
fzf-history-widget() { :; }
fzf-cd-widget() { :; }
zle -N fzf-file-widget
zle -N fzf-history-widget
zle -N fzf-cd-widget
bindkey '^T' fzf-file-widget
bindkey '^R' fzf-history-widget
bindkey '^[c' fzf-cd-widget
ZSH
`)
	writeZshTestExecutable(t, fakeBin, "zoxide", `#!/bin/sh
printf 'zoxide:%s\n' "$*" >>"$DOTS_TEST_LOG"
cat <<'ZSH'
z() { :; }
zi() { :; }
ZSH
`)
	writeZshTestExecutable(t, fakeBin, "atuin", `#!/bin/sh
printf 'atuin:%s\n' "$*" >>"$DOTS_TEST_LOG"
cat <<'ZSH'
atuin-search() { :; }
_atuin_up_search() { :; }
zle -N atuin-search
zle -N _atuin_up_search
bindkey '^R' atuin-search
bindkey '^[[A' _atuin_up_search
bindkey '^[OA' _atuin_up_search
ZSH
`)

	logPath := filepath.Join(home, "commands.log")
	config := filepath.Join(root, "configs", "zsh", "zshrc")
	command := `
source "$1"
bindkey '^I'
bindkey '^T'
bindkey '^[c'
bindkey '^R'
bindkey '^[[A'
bindkey '^X^E'
whence -w z zi
print -r -- "ctrl_t_opts=${FZF_CTRL_T_OPTS}"
print -r -- "alt_c_opts=${FZF_ALT_C_OPTS}"
print -r -- "default_opts=${FZF_DEFAULT_OPTS}"
`
	cmd := exec.Command(zsh, "-dfi", "-c", command, "zsh", config)
	cmd.Env = []string{
		"HOME=" + home,
		"ZDOTDIR=" + home,
		"PATH=" + fakeBin + string(os.PathListSeparator) + "/usr/bin:/bin",
		"TERM=xterm-256color",
		"DOTS_TEST_LOG=" + logPath,
		"ATUIN_CONFIG_DIR=" + filepath.Join(home, "atuin-config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("source managed interactive Zsh config: %v\n%s", err, output)
	}
	got := string(output)
	for _, want := range []string{
		`"^I" _fzf_tab_widget`,
		`"^T" fzf-file-widget`,
		`"^[c" fzf-cd-widget`,
		`"^R" atuin-search`,
		`"^[[A" _atuin_up_search`,
		`"^X^E" edit-command-line`,
		"z: function",
		"zi: function",
		"ctrl_t_opts=--walker-skip=.git,node_modules,target --preview",
		"bat --color=always",
		"alt_c_opts=--walker-skip=.git,node_modules,target --preview",
		"eza -a --color=always",
		"default_opts=--height=40%",
		"bg:#1e1e2e",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("interactive Zsh state missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"^R" fzf-history-widget`) {
		t.Fatalf("fzf retained Ctrl+R ownership:\n%s", got)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(logBytes)
	fzfCall := strings.Index(log, "fzf:--zsh")
	zoxideCall := strings.Index(log, "zoxide:init zsh")
	atuinCall := strings.Index(log, "atuin:init zsh")
	if fzfCall < 0 || zoxideCall < fzfCall || atuinCall < zoxideCall {
		t.Fatalf("tool initialization order = %q, want fzf, zoxide, then Atuin", log)
	}
}

func TestManagedZshStartsWithoutOptionalNavigationCommands(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not available")
	}

	home := t.TempDir()
	emptyBin := filepath.Join(home, "bin")
	if err := os.MkdirAll(filepath.Join(home, ".zim"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(emptyBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zim", "init.zsh"), []byte("# empty sandbox Zim runtime\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	config := filepath.Join("..", "..", "configs", "zsh", "zshrc")
	cmd := exec.Command(zsh, "-dfi", "-c", `source "$1"; print shell-ok; bindkey '^X^E'`, "zsh", config)
	cmd.Env = []string{
		"HOME=" + home,
		"ZDOTDIR=" + home,
		"PATH=" + emptyBin,
		"TERM=xterm-256color",
		"ATUIN_CONFIG_DIR=" + filepath.Join(home, "atuin-config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("source managed Zsh config without optional commands: %v\n%s", err, output)
	}
	got := string(output)
	for _, want := range []string{
		"dots: fzf not found; fuzzy completion and navigation are unavailable.",
		"shell-ok",
		`"^X^E" edit-command-line`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("degraded shell output missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"brew", "curl", "git clone", "install"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("degraded shell output contains unexpected installer/network command %q:\n%s", forbidden, got)
		}
	}
}

func writeZshTestExecutable(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o700); err != nil {
		t.Fatalf("write fake %s: %v", name, err)
	}
}
