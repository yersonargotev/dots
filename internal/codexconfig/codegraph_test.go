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
	assertCodeGraphBlock(t, filepath.Join(home, ".codex", "AGENTS.md"), got)
}

func TestEnsureCodeGraphModeCreatesSelectedAgentInstructionFiles(t *testing.T) {
	home := t.TempDir()

	if err := EnsureCodeGraphMode(home, "codex", "claude", "antigravity", "opencode"); err != nil {
		t.Fatalf("EnsureCodeGraphMode() error = %v", err)
	}

	for _, path := range []string{
		filepath.Join(home, ".codex", "AGENTS.md"),
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".gemini", "GEMINI.md"),
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read generated instructions %s: %v", path, err)
		}
		assertCodeGraphBlock(t, path, string(got))
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode")); !os.IsNotExist(err) {
		t.Fatalf("dots policy overlay must not create OpenCode instructions; CodeGraph installer owns OpenCode setup: %v", err)
	}
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
	assertCodeGraphBlock(t, path, got)
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
	assertCodeGraphBlock(t, filepath.Join(home, ".codex", "AGENTS.md"), got)
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
	assertCodeGraphBlock(t, filepath.Join(home, ".codex", "AGENTS.md"), got)
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

func assertCodeGraphBlock(t *testing.T, path, content string) {
	t.Helper()
	for _, want := range []string{
		codeGraphStart,
		"CodeGraph Mode: enabled",
		"Use CodeGraph for architecture questions",
		"Never use CodeGraph just because `.codegraph/` exists.",
		"Treat CodeGraph-returned source as already read.",
		"Do NOT use CodeGraph as proof for runtime behavior.",
		codeGraphEnd,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("%s missing %q\ncontent:\n%s", path, want, content)
		}
	}
	for _, ghostTool := range []string{"codegraph_context", "codegraph_trace", "codegraph_files", "codegraph_status", "codegraph_explore", "codegraph_node", "codegraph_search", "codegraph_callers"} {
		if strings.Contains(content, ghostTool) {
			t.Fatalf("%s kept generic CodeGraph tool guidance %q\ncontent:\n%s", path, ghostTool, content)
		}
	}
	for _, oldInstruction := range []string{
		"If `.codegraph/` exists in the project, use CodeGraph first",
		"use CodeGraph first",
		"If `.codegraph/` is missing, ask before running `codegraph init -i`.",
	} {
		if strings.Contains(content, oldInstruction) {
			t.Fatalf("%s kept mandatory CodeGraph wording %q\ncontent:\n%s", path, oldInstruction, content)
		}
	}
}
