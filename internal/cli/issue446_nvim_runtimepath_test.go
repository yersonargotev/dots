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

func TestRepositoryNeovimLoaderKeepsManagedPluginSpecsDiscoverable(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	home := t.TempDir()
	sourceRoot := t.TempDir()
	configHome := filepath.Join(home, ".config")
	dataHome := filepath.Join(home, ".local", "share")
	stateHome := filepath.Join(home, ".local", "state")
	cacheHome := filepath.Join(home, ".cache")
	dotsStateRoot := filepath.Join(home, ".local", "state", "dots")
	managedRoot := filepath.Join(configHome, "dots", "nvim")
	managedSource := filepath.Join(sourceRoot, "configs", "nvim")
	if err := os.CopyFS(managedSource, os.DirFS(filepath.Join(repositoryRoot, "configs", "nvim"))); err != nil {
		t.Fatalf("copy managed Neovim source: %v", err)
	}
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	writeIssue446File(t, manifestPath, `version: 1
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
	for key, value := range map[string]string{
		"HOME":             home,
		"XDG_CONFIG_HOME":  configHome,
		"XDG_DATA_HOME":    dataHome,
		"XDG_STATE_HOME":   stateHome,
		"XDG_CACHE_HOME":   cacheHome,
		"DOTS_STATE_ROOT":  dotsStateRoot,
		"DOTS_SOURCE_ROOT": sourceRoot,
	} {
		t.Setenv(key, value)
	}
	var stdout, stderr bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--skip-deps", "--profile", "default",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", dotsStateRoot,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("install Neovim fixture code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if info, err := os.Lstat(filepath.Join(configHome, "nvim", "init.lua")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("native Neovim loader = (%v, %v), want regular file", info, err)
	}
	if target, err := os.Readlink(managedRoot); err != nil || target != managedSource {
		t.Fatalf("managed Neovim root = (%q, %v), want symlink to %q", target, err, managedSource)
	}
	if info, err := os.Stat(filepath.Join(stateHome, "nvim", "lazy-lock.json")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("seeded Neovim lockfile = (%v, %v), want regular XDG state file", info, err)
	}
	lazyConfig, err := os.ReadFile(filepath.Join(managedRoot, "lua", "config", "lazy.lua"))
	if err != nil {
		t.Fatalf("read installed lazy config: %v", err)
	}
	const managedPathOption = `paths = { vim.fn.expand("~/.config/dots/nvim") }`
	if !strings.Contains(string(lazyConfig), managedPathOption) {
		t.Fatalf("lazy config does not preserve the managed root through performance.rtp.paths")
	}
	if pluginSpecs, err := filepath.Glob(filepath.Join(managedRoot, "lua", "plugins", "*.lua")); err != nil || len(pluginSpecs) == 0 {
		t.Fatalf("installed managed plugin specs = (%v, %v), want at least one", pluginSpecs, err)
	}

	nvim, err := exec.LookPath("nvim")
	if err != nil {
		t.Log("nvim is not installed; the hermetic installed-configuration assertions passed")
		return
	}

	// This local lazy.nvim stub reproduces the runtime-path reset at the public
	// setup boundary without cloning plugins or reading the operator's home.
	lazyStub := filepath.Join(dataHome, "nvim", "lazy", "lazy.nvim", "lua", "lazy", "init.lua")
	writeIssue446File(t, lazyStub, `
local M = {}

local function find_colorscheme(spec)
  if type(spec) ~= "table" then
    return nil
  end
  if spec[1] == "LazyVim/LazyVim" and type(spec.opts) == "table" then
    return spec.opts.colorscheme
  end
  for _, child in ipairs(spec) do
    local colorscheme = find_colorscheme(child)
    if colorscheme then
      return colorscheme
    end
  end
end

function M.setup(opts)
	local original_notify = vim.notify
	local notifications = {}
	vim.notify = function(message, level, notify_opts)
		table.insert(notifications, message)
		return original_notify(message, level, notify_opts)
	end
  local reset = vim.tbl_get(opts, "performance", "rtp", "reset") ~= false
  if reset then
    vim.opt.runtimepath = {
      vim.fn.stdpath("config"),
      vim.fn.stdpath("data") .. "/site",
      vim.env.VIMRUNTIME,
      vim.fn.stdpath("config") .. "/after",
    }
  end
  for _, path in ipairs(vim.tbl_get(opts, "performance", "rtp", "paths") or {}) do
    vim.opt.runtimepath:append(path)
  end

  vim.g.dots_test_rtp_reset = reset
  vim.g.dots_test_managed_on_rtp = vim.tbl_contains(vim.opt.runtimepath:get(), vim.fn.expand("~/.config/dots/nvim"))
  local plugin_specs = vim.api.nvim_get_runtime_file("lua/plugins/*.lua", true)
  vim.g.dots_test_plugin_specs = #plugin_specs
	if #plugin_specs == 0 then
		vim.notify('No specs found for module "plugins"', vim.log.levels.ERROR, { title = "lazy.nvim" })
	end
  for _, spec_path in ipairs(plugin_specs) do
    if spec_path:match("/colorscheme.lua$") then
      vim.g.dots_test_requested_colorscheme = find_colorscheme(dofile(spec_path))
    end
  end
  if vim.g.dots_test_requested_colorscheme then
    vim.cmd("colorscheme " .. vim.g.dots_test_requested_colorscheme)
  end
	vim.g.dots_test_lazy_notifications = #notifications
	vim.notify = original_notify
end

return M
`)
	writeIssue446File(t,
		filepath.Join(dataHome, "nvim", "site", "colors", "catppuccin-mocha.vim"),
		"let g:colors_name = 'catppuccin-mocha'\n",
	)

	assertion := strings.Join([]string{
		`local reset = vim.g.dots_test_rtp_reset == true`,
		`local managed = vim.g.dots_test_managed_on_rtp == true`,
		`local specs = (vim.g.dots_test_plugin_specs or 0) > 0`,
		`local notifications = vim.g.dots_test_lazy_notifications == 0`,
		`local requested = vim.g.dots_test_requested_colorscheme == "catppuccin-mocha"`,
		`local active = vim.g.colors_name == "catppuccin-mocha"`,
		`if not (reset and managed and specs and notifications and requested and active) then vim.api.nvim_err_writeln("lazy reset=" .. tostring(reset) .. ", managed root=" .. tostring(managed) .. ", plugin specs=" .. tostring(specs) .. ", notifications=" .. tostring(notifications) .. ", requested theme=" .. tostring(requested) .. ", active theme=" .. tostring(active)); vim.cmd("cquit 42") end`,
	}, "; ")
	cmd := exec.Command(nvim, "--headless", "-c", "lua "+assertion, "-c", "qa!")
	cmd.Env = append(issue446BaseEnv(),
		"HOME="+home,
		"XDG_CONFIG_HOME="+configHome,
		"XDG_DATA_HOME="+dataHome,
		"XDG_STATE_HOME="+stateHome,
		"XDG_CACHE_HOME="+cacheHome,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("managed Neovim startup: %v\n%s", err, output)
	}
}

func issue446BaseEnv() []string {
	var env []string
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "HOME=") || strings.HasPrefix(value, "XDG_") || strings.HasPrefix(value, "NVIM_APPNAME=") {
			continue
		}
		env = append(env, value)
	}
	return env
}

func writeIssue446File(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s parent: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
