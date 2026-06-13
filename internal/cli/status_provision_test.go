package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

// TestStatusListsDeclaredProvisioners proves status surfaces the provisioners
// declared for the profile as informational, read-only context: it shows the
// resolved command and affected roots without claiming an alignment state and
// without executing anything.
func TestStatusListsDeclaredProvisioners(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, provisionerManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"status", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	got := out.String()
	for _, want := range []string{
		`Declared provisioners for profile "default"`,
		"gentle-ai install --scope global --persona neutral --agents codex",
		"affects: ~/.claude, ~/.codex, ~/.gentle-ai",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q\noutput:\n%s", want, got)
		}
	}
}
