package codexconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCodeGraphModeCreatesAgentsFileWhenMissing(t *testing.T) {
	home := t.TempDir()

	if err := EnsureCodeGraphMode(home); err != nil {
		t.Fatalf("EnsureCodeGraphMode() error = %v", err)
	}

	got := readCodexAgents(t, home)
	assertCodeGraphBlock(t, got)
}

func TestEnsureCodeGraphModePreservesExistingContent(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	before := "# Personal Codex Rules\n\nKeep this line.\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatalf("write existing AGENTS.md: %v", err)
	}

	if err := EnsureCodeGraphMode(home); err != nil {
		t.Fatalf("EnsureCodeGraphMode() error = %v", err)
	}

	got := readCodexAgents(t, home)
	if !strings.Contains(got, before) {
		t.Fatalf("AGENTS.md did not preserve existing content %q\ncontent:\n%s", before, got)
	}
	assertCodeGraphBlock(t, got)
}

func TestEnsureCodeGraphModeUpdatesExistingDotsBlockIdempotently(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	existing := "before\n\n" + codeGraphStart + "\nstale instructions\n" + codeGraphEnd + "\n\nafter\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing AGENTS.md: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := EnsureCodeGraphMode(home); err != nil {
			t.Fatalf("EnsureCodeGraphMode() run %d error = %v", i+1, err)
		}
	}

	got := readCodexAgents(t, home)
	assertCodeGraphBlock(t, got)
	if strings.Contains(got, "stale instructions") {
		t.Fatalf("AGENTS.md kept stale managed block content\ncontent:\n%s", got)
	}
	if count := strings.Count(got, codeGraphStart); count != 1 {
		t.Fatalf("dots start marker count = %d, want 1\ncontent:\n%s", count, got)
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("AGENTS.md did not preserve content around managed block\ncontent:\n%s", got)
	}
}

func TestEnsureCodeGraphModeMigratesLegacyGentleAIBlock(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir codex dir: %v", err)
	}
	existing := "before\n\n" + legacyCodeGraphStart + "\nmanual codegraph instructions\n" + legacyCodeGraphEnd + "\n\nafter\n"
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing AGENTS.md: %v", err)
	}

	if err := EnsureCodeGraphMode(home); err != nil {
		t.Fatalf("EnsureCodeGraphMode() error = %v", err)
	}

	got := readCodexAgents(t, home)
	assertCodeGraphBlock(t, got)
	for _, legacy := range []string{legacyCodeGraphStart, legacyCodeGraphEnd, "manual codegraph instructions"} {
		if strings.Contains(got, legacy) {
			t.Fatalf("AGENTS.md kept legacy content %q\ncontent:\n%s", legacy, got)
		}
	}
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Fatalf("AGENTS.md did not preserve content around legacy block\ncontent:\n%s", got)
	}
}

func readCodexAgents(t *testing.T, home string) string {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(home, ".codex", "AGENTS.md"))
	if err != nil {
		t.Fatalf("read Codex AGENTS.md: %v", err)
	}
	return string(got)
}

func assertCodeGraphBlock(t *testing.T, content string) {
	t.Helper()
	for _, want := range []string{
		codeGraphStart,
		"CodeGraph Mode: enabled",
		"If `.codegraph/` exists in the project, use CodeGraph first",
		"`codegraph_context`: map a feature or area first.",
		"Treat CodeGraph-returned source as already read.",
		"If `.codegraph/` is missing, ask before running `codegraph init -i`.",
		codeGraphEnd,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("AGENTS.md missing %q\ncontent:\n%s", want, content)
		}
	}
}
