package agentinstructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveGentleAITriggerRulesRemovesSupportedAgentBlocks(t *testing.T) {
	home := t.TempDir()
	paths := []string{
		filepath.Join(home, ".codex", "AGENTS.md"),
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		filepath.Join(home, ".gemini", "GEMINI.md"),
		filepath.Join(home, "Library", "Application Support", "Code", "User", "prompts", "gentle-ai.instructions.md"),
		filepath.Join(home, ".config", "Code", "User", "prompts", "gentle-ai.instructions.md"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		content := "before\n\n" + gentleAITriggerRulesStart + "\nstale review-readability rule\n" + gentleAITriggerRulesEnd + "\n\nafter\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if err := RemoveGentleAITriggerRules(home, "codex", "claude-code", "opencode", "antigravity", "vscode-copilot"); err != nil {
		t.Fatalf("RemoveGentleAITriggerRules() error = %v", err)
	}

	for _, path := range paths {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(got)
		for _, not := range []string{gentleAITriggerRulesStart, gentleAITriggerRulesEnd, "review-readability"} {
			if strings.Contains(content, not) {
				t.Fatalf("%s kept %q\ncontent:\n%s", path, not, content)
			}
		}
		if !strings.Contains(content, "before") || !strings.Contains(content, "after") {
			t.Fatalf("%s did not preserve surrounding content\ncontent:\n%s", path, content)
		}
	}
}
