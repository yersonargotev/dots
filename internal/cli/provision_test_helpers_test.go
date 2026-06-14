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
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
