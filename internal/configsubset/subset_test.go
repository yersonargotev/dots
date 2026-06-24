package configsubset

import (
	"os"
	"path/filepath"
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
