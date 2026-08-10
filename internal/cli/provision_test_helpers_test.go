package cli_test

import (
	"os"
	"path/filepath"
	"testing"
)

func stubManifestProvisionerTools(t *testing.T) {
	t.Helper()

	stubDir := t.TempDir()
	writeManifestDependencyStubs(t, stubDir)
	// claude backs the chrome-devtools marketplace/plugin provisioners and codex
	// backs the chrome-devtools MCP provisioner, both selected by the web
	// profile. The stubs exit cleanly so the sandboxed install never reaches the
	// network or the real agent config.
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codex"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeManifestDependencyStubs(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{
		"agy",
		"atuin",
		"bat",
		"bun",
		"claude",
		"codex",
		"copilot",
		"dart",
		"fnm",
		"flutter",
		"ghostty",
		"git",
		"go",
		"gh",
		"jq",
		"node",
		"nvim",
		"npx",
		"opencode",
		"playwright-cli",
		"pnpm",
		"rustc",
		"rustup",
		"cargo",
		"delta",
		"eza",
		"fd",
		"fzf",
		"lazygit",
		"rg",
		"starship",
		"tmux",
		"tuicr",
		"unzip",
		"uv",
		"warp-terminal",
		"zed",
		"zellij",
		"zoxide",
		"zsh",
	} {
		writeExecStub(t, filepath.Join(dir, name), "#!/bin/sh\nexit 0\n")
	}
}
