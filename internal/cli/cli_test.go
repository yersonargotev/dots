package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestRootHelpIdentifiesDotsCLI(t *testing.T) {
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "dots") || !strings.Contains(got, "Dotfiles CLI") {
		t.Fatalf("help output = %q, want command name and description", got)
	}
}

func TestManifestValidateCommandAcceptsValidManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)
	if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"manifest", "validate", "--file", manifestPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "manifest is valid" {
		t.Fatalf("output = %q, want %q", got, "manifest is valid")
	}
}
