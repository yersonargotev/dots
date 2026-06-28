package agentinstructions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yersonargotev/dots/internal/textblock"
)

const (
	gentleAITriggerRulesStart = "<!-- gentle-ai:trigger-rules -->"
	gentleAITriggerRulesEnd   = "<!-- /gentle-ai:trigger-rules -->"
	gentleAIPersonaStart      = "<!-- gentle-ai:persona -->"
	gentleAIPersonaEnd        = "<!-- /gentle-ai:persona -->"
	dotsRulesStart            = "<!-- dots:rules -->"
	dotsRulesEnd              = "<!-- /dots:rules -->"
)

const dotsRulesBlock = `## Dots Agent Rules

| Boundary | Rule |
| --- | --- |
| Always | Keep diffs surgical: every changed line must trace to the user request; mention unrelated issues instead of fixing them silently. |
| Always | Choose the simplest change that satisfies the request; avoid speculative abstractions, configurability, or features not explicitly needed. |
| Always | Plan before editing: think through the target behavior, inspect existing patterns, and state the smallest intended change before coding. |
| Always | Verify before declaring success: use focused checks while iterating, then run the repo-required checks when the task is complete. |
| Always | Use sandboxed HOME/config paths for dotfiles behavior; never validate by writing to the operator's real home config. |
| Ask first | Stop when the safe path is unclear, the scope would broaden, or an action could mutate real user configuration. |`

// RemoveGentleAITriggerRules removes stale gentle-ai trigger recommendations
// from supported agent instruction files. dots does not install the referenced
// 4R review agents as portable skills, so the block must not survive the
// dots-managed agent baseline.
func RemoveGentleAITriggerRules(home string, agents ...string) error {
	for _, path := range instructionPaths(home, agents) {
		if err := removeTriggerRules(path); err != nil {
			return err
		}
	}
	return nil
}

// ConvergeDotsAgentRules removes gentle-ai rules/persona blocks that conflict
// with the dots-managed baseline and injects the portable dots rules block into
// supported global agent instruction files.
func ConvergeDotsAgentRules(home string, agents ...string) error {
	for _, path := range instructionPaths(home, agents) {
		if err := convergeDotsRules(path); err != nil {
			return err
		}
	}
	return nil
}

func instructionPaths(home string, agents []string) []string {
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

func convergeDotsRules(path string) error {
	content, mode, err := readInstructionFile(path)
	if err != nil {
		return err
	}
	updated, err := textblock.Remove(
		content,
		textblock.Markers{Start: gentleAITriggerRulesStart, End: gentleAITriggerRulesEnd},
		textblock.Markers{Start: gentleAIPersonaStart, End: gentleAIPersonaEnd},
	)
	if err != nil {
		return fmt.Errorf("remove gentle-ai managed blocks from %s: %w", path, err)
	}
	updated = removeLegacyMarkerlessPersona(updated)
	updated, err = textblock.Upsert(updated, textblock.Markers{Start: dotsRulesStart, End: dotsRulesEnd}, dotsRulesBlock)
	if err != nil {
		return fmt.Errorf("upsert dots rules in %s: %w", path, err)
	}
	if updated == content {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir agent instructions %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(updated), mode); err != nil {
		return fmt.Errorf("write agent instructions %s: %w", path, err)
	}
	return nil
}

func readInstructionFile(path string) (string, os.FileMode, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", 0o600, nil
		}
		return "", 0, fmt.Errorf("read agent instructions %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, fmt.Errorf("stat agent instructions %s: %w", path, err)
	}
	return string(content), info.Mode().Perm(), nil
}

func removeLegacyMarkerlessPersona(content string) string {
	start := strings.Index(content, "\n## Personality")
	if start >= 0 {
		start++
	} else if strings.HasPrefix(content, "## Personality") {
		start = 0
	} else {
		return content
	}

	end := len(content)
	for _, heading := range []string{
		"\n## Contextual Skill Loading",
		"\n<!-- gentle-ai:engram-protocol -->",
		"\n<!-- dots:",
		"\n<!-- CODEGRAPH_START -->",
	} {
		if idx := strings.Index(content[start+len("## Personality"):], heading); idx >= 0 {
			candidate := start + len("## Personality") + idx + 1
			if candidate < end {
				end = candidate
			}
		}
	}
	if end == len(content) {
		if idx := nextHeadingAfterBehavior(content[start:]); idx >= 0 {
			end = start + idx
		}
	}
	if !isKnownLegacyMarkerlessPersona(content[start:end]) {
		return content
	}

	updated := strings.TrimRight(content[:start], "\n")
	if suffix := strings.TrimLeft(content[end:], "\n"); suffix != "" {
		if updated != "" {
			updated += "\n\n"
		}
		updated += suffix
	} else if updated != "" {
		updated += "\n"
	}
	return updated
}

func isKnownLegacyMarkerlessPersona(block string) bool {
	requiredHeadings := []string{
		"## Personality",
		"## Persona Scope",
		"## Language",
		"## Tone",
		"## Philosophy",
		"## Expertise",
		"## Behavior",
	}
	for _, heading := range requiredHeadings {
		if !strings.Contains(block, heading) {
			return false
		}
	}

	for _, phrase := range []string{
		"Senior Architect, 15+ years experience, GDE & MVP",
		"The persona styles HOW YOU TALK",
		"Match the user's current language in your REPLY ONLY",
		"CONCEPTS > CODE",
	} {
		if strings.Contains(block, phrase) {
			return true
		}
	}
	return false
}

func nextHeadingAfterBehavior(content string) int {
	behavior := strings.Index(content, "\n## Behavior")
	if behavior < 0 {
		return -1
	}
	searchFrom := behavior + len("\n## Behavior")
	next := strings.Index(content[searchFrom:], "\n## ")
	if next < 0 {
		return len(content)
	}
	return searchFrom + next + 1
}
