package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestManifestProvisionerStubsRejectPackageManagers(t *testing.T) {
	stubManifestProvisionerTools(t)
	for _, name := range []string{"brew", "apt-get", "dnf"} {
		t.Run(name, func(t *testing.T) {
			output, err := exec.Command(name, "install", "example").CombinedOutput()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 97 {
				t.Fatalf("%s unexpectedly succeeded or used a host executable: %v, output %q", name, err, output)
			}
			if !strings.Contains(string(output), "unexpected package manager call:") || !strings.Contains(string(output), "install example") {
				t.Fatalf("%s did not report the blocked command: %q", name, output)
			}
		})
	}
}

func writeManifestDependencyStubs(t *testing.T, dir string) {
	t.Helper()
	for _, name := range []string{"brew", "apt", "apt-get", "dnf", "yum", "pacman", "snap", "flatpak", "curl", "wget", "npm"} {
		writeExecStub(t, filepath.Join(dir, name), "#!/bin/sh\nprintf 'unexpected package manager call: %s' \"$0\" >&2\nprintf ' %s' \"$@\" >&2\nprintf '\\n' >&2\nexit 97\n")
	}
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
		"herdr",
		"jq",
		"node",
		"nvim",
		"npx",
		"opencode",
		"playwright-cli",
		"pnpm",
		"python3",
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
