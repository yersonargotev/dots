package cli_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

// requireFindings asserts that err is a FindingsError, the outcome a read-only
// diagnostic command returns when it surfaces a non-error divergence (Drift, an
// unresolved Conflict, a missing Dependency, or a doctor concern).
func requireFindings(t *testing.T, err error) {
	t.Helper()
	var fe *cli.FindingsError
	if !errors.As(err, &fe) {
		t.Fatalf("expected FindingsError, got %v", err)
	}
}

// writeStatusManifest writes a manifest with one symlink entry and returns the
// manifest path and the source root that holds the managed source file.
func writeStatusManifest(t *testing.T, home string, entryTags string) (manifestPath, sourceRoot string) {
	t.Helper()
	sourceRoot = t.TempDir()
	srcPath := filepath.Join(sourceRoot, "configs/zsh/zshrc")
	if err := os.MkdirAll(filepath.Dir(srcPath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("export A=1\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	manifestPath = filepath.Join(home, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [` + entryTags + `]
    os: [darwin, linux]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return manifestPath, sourceRoot
}

func TestRunReturnsZeroWhenAligned(t *testing.T) {
	// An entry tagged outside the default profile is not selected, so the status
	// report is empty: aligned, no findings.
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "other")

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"status", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 0 {
		t.Fatalf("aligned status exit code = %d, want 0\nstderr:\n%s", code, errOut.String())
	}
}

func TestRunReturnsTwoOnFindings(t *testing.T) {
	// The managed target is never installed, so status reports it as missing: a
	// finding the caller should act on.
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core")

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"status", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 2 {
		t.Fatalf("status with findings exit code = %d, want 2\nstdout:\n%s", code, out.String())
	}
}

func TestRunReturnsOneOnError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"status", "--file", filepath.Join(t.TempDir(), "missing.yaml")}, &out, &errOut)

	if code != 1 {
		t.Fatalf("status with bad manifest exit code = %d, want 1", code)
	}
	if errOut.Len() == 0 {
		t.Fatal("an execution error must be reported on stderr")
	}
}

func TestRunReturnsZeroOnSuccessfulNonDiagnosticCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cli.Run([]string{"version"}, &out, &errOut); code != 0 {
		t.Fatalf("version exit code = %d, want 0", code)
	}
}
