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
	gentleAITriggerRulesStart  = "<!-- gentle-ai:trigger-rules -->"
	gentleAITriggerRulesEnd    = "<!-- /gentle-ai:trigger-rules -->"
	gentleAIPersonaStart       = "<!-- gentle-ai:persona -->"
	gentleAIPersonaEnd         = "<!-- /gentle-ai:persona -->"
	gentleAIEngramStart        = "<!-- gentle-ai:engram-protocol -->"
	gentleAIEngramEnd          = "<!-- /gentle-ai:engram-protocol -->"
	dotsRulesStart             = "<!-- dots:rules -->"
	dotsRulesEnd               = "<!-- /dots:rules -->"
	codexDelegationStart       = "<!-- dots:delegation -->"
	codexDelegationEnd         = "<!-- /dots:delegation -->"
	legacyDotsDelegationStart  = "<!-- dots:codex-spark-delegation -->"
	legacyDotsDelegationEnd    = "<!-- /dots:codex-spark-delegation -->"
	legacyCodexDelegationStart = "<!-- argote:subagent-delegation -->"
	legacyCodexDelegationEnd   = "<!-- /argote:subagent-delegation -->"
	codexExplorerAgentFile     = "dots-explorer.toml"
	codexWorkerAgentFile       = "dots-worker.toml"
)

const dotsRulesBlock = `## Dots Agent Rules

| Boundary | Rule |
| --- | --- |
| Always | Keep diffs surgical: every changed line must trace to the user request; mention unrelated issues instead of fixing them silently. |
| Always | Choose the simplest change that satisfies the request; avoid speculative abstractions, configurability, or features not explicitly needed. |
| Always | Plan before editing: think through the target behavior, inspect existing patterns, and state the smallest intended change before coding. |
| Always | Verify before declaring success: use focused checks while iterating, then run the repo-required checks when the task is complete. |
| Always | Use sandboxed HOME/config paths for dotfiles behavior; never validate by writing to the operator's real home config. |
| Ask first | Stop when the safe path is unclear, the scope would broaden, or an action could mutate real user configuration. |

## Delegation

For non-trivial work, load the delegation skill when available. Use it for Delegation Preflight, safe slice selection, skip reasons, and final reporting. Keep external project state in the main agent.`

const codexDelegationBlock = `## Codex delegation

Load the ` + "`delegation`" + ` skill when available, then map safe Codex slices to the dots-owned native agents:

| Slice | Codex agent |
| --- | --- |
| Codebase exploration, impact scans, or test/log triage | ` + "`dots-explorer`" + ` on ` + "`gpt-5.6-sol`" + ` |
| Separable implementation over disjoint files/modules | ` + "`dots-worker`" + ` on ` + "`gpt-5.6-sol`" + ` |
| Review, architecture, security, or other judgment-heavy work | Strongest appropriate available model; reserve this profile's GPT-5.6 Sol low default for bounded exploration and implementation. |

Selecting the ` + "`codex-delegation`" + ` profile is standing authorization for safe bounded Codex delegation across repositories when the active prompt, workflow, or selected skill asks for subagents, parallel agents, or delegation. If the active Codex tool still requires prompt-level permission, ask once or record ` + "`tool-level permission required`" + ` as the skip reason.`

const codexExplorerAgent = `name = "dots-explorer"
description = "Read-only dots explorer for bounded codebase exploration, impact scans, and test/log triage."
model = "gpt-5.6-sol"
model_reasoning_effort = "low"
sandbox_mode = "read-only"
developer_instructions = """
Outcome: answer the assigned exploration question with the smallest sufficient body of repository evidence.

Success means:
- inspect the relevant code, docs, tests, manifests, logs, or command output;
- use CodeGraph for source architecture and call flow, and targeted rg/sed reads for config, docs, manifests, and scripts;
- return findings with file:line evidence and confidence, plus material gaps or uncertainty;
- stop when the question is answered or the missing evidence is identified.

Load the delegation skill when available. Do not edit files. Keep GitHub, releases, package managers, real user configuration, requirements, decisions, integration, and final verification with the main agent.
"""
nickname_candidates = ["Dots Scout", "Dots Cartographer", "Dots Radar"]
`

const codexWorkerAgent = `name = "dots-worker"
description = "dots implementation worker for explicitly assigned, non-overlapping files or modules."
model = "gpt-5.6-sol"
model_reasoning_effort = "low"
sandbox_mode = "workspace-write"
developer_instructions = """
Outcome: complete the assigned implementation slice without changing anything outside its declared ownership.

Success means:
- make the smallest diff that satisfies the slice and follows repository patterns and dots domain language;
- preserve concurrent and unrelated edits;
- run the most relevant non-destructive validation available;
- return changed files, tests run, remaining risks, and any blocker that prevented completion.

Load the delegation skill when available. Use sandboxed --home, --source-root, --state-root, or temporary config paths for dotfiles behavior. Keep GitHub, releases, package managers, real user configuration, requirements, decisions, external state, integration, and final verification with the main agent.
"""
nickname_candidates = ["Dots Builder", "Dots Mason", "Dots Stitcher"]
`

type instructionTarget struct {
	agent string
	path  string
}

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
	for _, target := range instructionTargets(home, agents) {
		if err := convergeDotsRules(target.path); err != nil {
			return err
		}
	}
	return nil
}

// SyncCopilotCLIEngramProtocol copies the gentle-ai generated Engram protocol
// for vscode-copilot into Copilot CLI's global instruction file. gentle-ai does
// not write Copilot CLI instructions directly, but the protocol is portable
// Markdown and required for the same Engram behavior in Copilot CLI.
func SyncCopilotCLIEngramProtocol(home string) error {
	sourceBlock, ok, err := firstMarkedBlock(
		textblock.Markers{Start: gentleAIEngramStart, End: gentleAIEngramEnd},
		filepath.Join(home, "Library", "Application Support", "Code", "User", "prompts", "gentle-ai.instructions.md"),
		filepath.Join(home, ".config", "Code", "User", "prompts", "gentle-ai.instructions.md"),
	)
	if err != nil || !ok {
		return err
	}
	target := filepath.Join(home, ".copilot", "copilot-instructions.md")
	content, mode, err := readInstructionFile(target)
	if err != nil {
		return err
	}
	updated, err := textblock.Upsert(content, textblock.Markers{Start: gentleAIEngramStart, End: gentleAIEngramEnd}, sourceBlock)
	if err != nil {
		return fmt.Errorf("upsert gentle-ai Engram protocol in %s: %w", target, err)
	}
	if updated == content {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("mkdir Copilot CLI instructions %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, []byte(updated), mode); err != nil {
		return fmt.Errorf("write Copilot CLI instructions %s: %w", target, err)
	}
	return nil
}

// ConvergeCodexDelegation injects opt-in delegation guidance into Codex
// instructions only. Legacy Spark-specific and argote-owned marker pairs are
// migrated to the generic dots-owned marker pair during the upsert.
func ConvergeCodexDelegation(home string) error {
	return errors.Join(
		convergeCodexDelegation(filepath.Join(home, ".codex", "AGENTS.md")),
		writeCodexAgentFile(home, codexExplorerAgentFile, codexExplorerAgent),
		writeCodexAgentFile(home, codexWorkerAgentFile, codexWorkerAgent),
	)
}

// RemoveCodexDelegation removes current and legacy dots-owned delegation blocks
// plus the legacy argote-owned block without touching the rest of the baseline.
func RemoveCodexDelegation(home string) error {
	return errors.Join(
		removeCodexDelegation(filepath.Join(home, ".codex", "AGENTS.md")),
		removeCodexAgentFile(home, codexExplorerAgentFile),
		removeCodexAgentFile(home, codexWorkerAgentFile),
	)
}

func writeCodexAgentFile(home, name, content string) error {
	path := filepath.Join(home, ".codex", "agents", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir Codex agents %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write Codex agent %s: %w", path, err)
	}
	return nil
}

func removeCodexAgentFile(home, name string) error {
	path := filepath.Join(home, ".codex", "agents", name)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove Codex agent %s: %w", path, err)
	}
	return nil
}

func instructionPaths(home string, agents []string) []string {
	targets := instructionTargets(home, agents)
	paths := make([]string, 0, len(targets))
	for _, target := range targets {
		paths = append(paths, target.path)
	}
	return paths
}

func instructionTargets(home string, agents []string) []instructionTarget {
	seen := map[string]bool{}
	var targets []instructionTarget
	add := func(agent, path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		targets = append(targets, instructionTarget{agent: agent, path: path})
	}

	for _, agent := range agents {
		switch agent {
		case "codex":
			add(agent, filepath.Join(home, ".codex", "AGENTS.md"))
		case "claude", "claude-code":
			add(agent, filepath.Join(home, ".claude", "CLAUDE.md"))
		case "opencode":
			add(agent, filepath.Join(home, ".config", "opencode", "AGENTS.md"))
		case "antigravity":
			add(agent, filepath.Join(home, ".gemini", "GEMINI.md"))
		case "vscode-copilot":
			// gentle-ai exposes Copilot through its vscode-copilot agent key.
			// dots uses that completed provisioner as the ownership boundary for
			// Copilot portable-policy files only: VS Code prompt instructions and
			// Copilot CLI global instructions. This does not add a native Copilot
			// custom-agent artifact.
			addCopilotPortablePolicyTargets(add, agent, home)
		}
	}
	return targets
}

func addCopilotPortablePolicyTargets(add func(agent, path string), agent, home string) {
	add(agent, filepath.Join(home, "Library", "Application Support", "Code", "User", "prompts", "gentle-ai.instructions.md"))
	add(agent, filepath.Join(home, ".config", "Code", "User", "prompts", "gentle-ai.instructions.md"))
	add(agent, filepath.Join(home, ".copilot", "copilot-instructions.md"))
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

func convergeCodexDelegation(path string) error {
	content, mode, err := readInstructionFile(path)
	if err != nil {
		return err
	}
	updated, err := textblock.Upsert(
		content,
		textblock.Markers{Start: codexDelegationStart, End: codexDelegationEnd},
		codexDelegationBlock,
		textblock.Markers{Start: legacyDotsDelegationStart, End: legacyDotsDelegationEnd},
		textblock.Markers{Start: legacyCodexDelegationStart, End: legacyCodexDelegationEnd},
	)
	if err != nil {
		return fmt.Errorf("upsert Codex delegation guidance in %s: %w", path, err)
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

func removeCodexDelegation(path string) error {
	content, mode, err := readInstructionFile(path)
	if err != nil {
		return err
	}
	updated, err := textblock.Remove(
		content,
		textblock.Markers{Start: codexDelegationStart, End: codexDelegationEnd},
		textblock.Markers{Start: legacyDotsDelegationStart, End: legacyDotsDelegationEnd},
		textblock.Markers{Start: legacyCodexDelegationStart, End: legacyCodexDelegationEnd},
	)
	if err != nil {
		return fmt.Errorf("remove Codex delegation guidance from %s: %w", path, err)
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

func firstMarkedBlock(markers textblock.Markers, paths ...string) (string, bool, error) {
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", false, fmt.Errorf("read agent instructions %s: %w", path, err)
		}
		block, ok, err := textblock.ExtractBody(string(content), markers)
		if err != nil {
			return "", false, fmt.Errorf("read marked block from %s: %w", path, err)
		}
		if ok {
			return block, true, nil
		}
	}
	return "", false, nil
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
