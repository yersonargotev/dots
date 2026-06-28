package agentinstructions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yersonargotev/dots/internal/textblock"
)

const (
	gentleAITriggerRulesStart = "<!-- gentle-ai:trigger-rules -->"
	gentleAITriggerRulesEnd   = "<!-- /gentle-ai:trigger-rules -->"
)

// RemoveGentleAITriggerRules removes stale gentle-ai trigger recommendations
// from supported agent instruction files. dots does not install the referenced
// 4R review agents as portable skills, so the block must not survive the
// dots-managed agent baseline.
func RemoveGentleAITriggerRules(home string, agents ...string) error {
	for _, path := range triggerRuleInstructionPaths(home, agents) {
		if err := removeTriggerRules(path); err != nil {
			return err
		}
	}
	return nil
}

func triggerRuleInstructionPaths(home string, agents []string) []string {
	var paths []string
	seen := map[string]bool{}
	add := func(path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}

	for _, agent := range agents {
		switch agent {
		case "codex":
			add(filepath.Join(home, ".codex", "AGENTS.md"))
		case "claude", "claude-code":
			add(filepath.Join(home, ".claude", "CLAUDE.md"))
		case "opencode":
			add(filepath.Join(home, ".config", "opencode", "AGENTS.md"))
		case "antigravity":
			add(filepath.Join(home, ".gemini", "GEMINI.md"))
		case "vscode-copilot":
			add(filepath.Join(home, "Library", "Application Support", "Code", "User", "prompts", "gentle-ai.instructions.md"))
			add(filepath.Join(home, ".config", "Code", "User", "prompts", "gentle-ai.instructions.md"))
		}
	}
	return paths
}

func removeTriggerRules(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read agent instructions %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat agent instructions %s: %w", path, err)
	}
	updated, err := textblock.Remove(string(content), textblock.Markers{Start: gentleAITriggerRulesStart, End: gentleAITriggerRulesEnd})
	if err != nil {
		return fmt.Errorf("remove gentle-ai trigger rules from %s: %w", path, err)
	}
	if updated == string(content) {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return fmt.Errorf("write agent instructions %s: %w", path, err)
	}
	return nil
}
