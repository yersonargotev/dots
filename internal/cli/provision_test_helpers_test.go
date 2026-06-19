package cli_test

import (
	"os"
	"path/filepath"
	"testing"
)

func stubGentleAIProvisionerTools(t *testing.T) {
	t.Helper()

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	// claude backs the chrome-devtools marketplace/plugin provisioners and codex
	// backs the chrome-devtools MCP provisioner, both selected by the desktop
	// profile. The stubs exit cleanly so the sandboxed install never reaches the
	// network or the real agent config.
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codex"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
