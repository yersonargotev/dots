package configsubset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTOMLFileContains(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	if err := os.WriteFile(source, []byte("[tui]\nstatus_line = [\"model\", \"git-branch\"]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("model = \"gpt-5.5\"\n\n[tui]\nstatus_line = [\"model\", \"git-branch\"]\ntheme = \"catppuccin\"\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	got, err := TOMLFileContains(target, source)
	if err != nil {
		t.Fatalf("TOMLFileContains() error = %v", err)
	}
	if !got {
		t.Fatal("TOMLFileContains() = false, want true")
	}
}

func TestTOMLFileContainsRejectsChangedDotsOwnedValue(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	if err := os.WriteFile(source, []byte("[tui]\nstatus_line = [\"model\", \"git-branch\"]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("[tui]\nstatus_line = [\"model\"]\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	got, err := TOMLFileContains(target, source)
	if err != nil {
		t.Fatalf("TOMLFileContains() error = %v", err)
	}
	if got {
		t.Fatal("TOMLFileContains() = true, want false")
	}
}

func TestTOMLFileContainsIgnoresUnsupportedUnownedTargetTOML(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	if err := os.WriteFile(source, []byte("[tui]\nstatus_line = [\"model\", \"git-branch\"]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte(`[[mcp_servers]]
name = "chrome-devtools"
command = "npx"
args = [
  "-y",
  "chrome-devtools-mcp@latest",
]

[tui]
status_line = ["model", "git-branch"]
`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	got, err := TOMLFileContains(target, source)
	if err != nil {
		t.Fatalf("TOMLFileContains() error = %v", err)
	}
	if !got {
		t.Fatal("TOMLFileContains() = false, want true")
	}
}

func TestTOMLFileContainsSupportsSourceArrayOfTables(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	hook := `[[hooks.SessionStart]]
matcher = "startup|resume"

[[hooks.SessionStart.hooks]]
type = "command"
command = "codegraph init"
timeout = 120
`
	if err := os.WriteFile(source, []byte(hook), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("model = \"gpt-5.5\"\n\n"+hook), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	got, err := TOMLFileContains(target, source)
	if err != nil {
		t.Fatalf("TOMLFileContains() error = %v", err)
	}
	if !got {
		t.Fatal("TOMLFileContains() = false, want true")
	}
}

func TestMergeTOMLFileAppendsMissingArrayTableBlocks(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	if err := os.WriteFile(source, []byte(`sandbox_mode = "danger-full-access"
approval_policy = "never"

[[hooks.SessionStart]]
matcher = "startup|resume"

[[hooks.SessionStart.hooks]]
type = "command"
command = "codegraph init"
timeout = 120
statusMessage = "Initializing CodeGraph index"
`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte(`model = "gpt-5.5"
sandbox_mode = "danger-full-access"
approval_policy = "never"

[tui]
theme = "catppuccin-mocha"
`), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if err := MergeTOMLFile(target, source); err != nil {
		t.Fatalf("MergeTOMLFile() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	for _, want := range []string{
		`model = "gpt-5.5"`,
		`[tui]`,
		`[[hooks.SessionStart]]`,
		`command = "codegraph init"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("merged target missing %q\ncontent:\n%s", want, got)
		}
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("target mode = %v, want 0640", gotMode)
	}
}
