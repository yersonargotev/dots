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
	codexDelegationStart       = "<!-- dots:codex-spark-delegation -->"
	codexDelegationEnd         = "<!-- /dots:codex-spark-delegation -->"
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

## Portable delegation policy

Use delegation when a non-trivial task has an independent slice that can return a compact summary without transferring requirements, decisions, external state, integration, or final verification away from the main agent.

Run Delegation Preflight before non-trivial work:

1. Confirm whether the active instructions include every surface-specific delegation overlay and native artifact needed by the task. For Codex Spark, check both the dots:codex-spark-delegation overlay and the native dots-explorer.toml / dots-worker.toml custom agents.
2. Decide whether the task is non-trivial.
3. Identify at least one safe explorer or worker slice, or choose one closed-list skip reason.
4. If a workflow authorizes delegation but the current tool requires explicit permission, ask once at the start or record tool-level permission required.

Closed-list skip reasons: tiny/mechanical, no independent slice, real user configuration, external state mutation, overlapping write scopes, or tool-level permission required.

| Delegate when | Model/tier choice |
| --- | --- |
| Codebase exploration, impact scans, or test/log triage can run independently | Use the fastest/cost-effective capable model or the surface's read-only/exploration agent. |
| Implementation can be split into disjoint files or modules | Use an implementation-capable worker with explicit file ownership and require a changed-file list. |
| Review, architecture, security, or other judgment-heavy work is delegated by a selected skill | Use the strongest appropriate available model, or the model the selected skill explicitly requires. |

Skip delegation only for one of the closed-list reasons. After delegated work returns, inspect the evidence or changes, accept or reject findings explicitly, run the relevant verification yourself, and summarize delegated slice, agent surface, model/tier, accepted/rejected findings or changes, main-agent verification, and skip reason when no subagent was used.`

const codexDelegationBlock = `## Subagent delegation defaults

Follow the portable delegation policy in the dots:rules block. This Codex-only overlay maps that policy to Codex subagents without replacing the main agent's responsibility for requirements, decisions, external project state, integration, and final verification.

| Delegate when | Model choice |
| --- | --- |
| Codebase exploration, impact scans, or test/log triage can run independently | Spawn a Codex explorer on gpt-5.3-codex-spark and ask for findings with file refs. |
| Implementation can be split into disjoint files/modules | Spawn a Codex worker on gpt-5.3-codex-spark, assign ownership, and require a changed-file list. |
| Review, architecture, security, or other high-judgment work is delegated by a selected skill | Use the strongest appropriate available model, or the model that the skill explicitly requires; do not force Spark for judgment-heavy review. |

Starting the dots-development-loop workflow counts as repo-level authorization to delegate safe slices under the portable policy. If the active Codex tool still requires explicit user permission before spawning subagents, ask once at workflow start or record tool-level permission required as the skip reason.

For write-heavy work, define non-overlapping ownership and remind workers not to revert others' edits. After results return, inspect/integrate the changes yourself, run the relevant verification, close finished agents, and summarize delegated work, model choice, accepted/rejected findings or changes, main-agent verification, plus any explicit skip reasons.`

const codexExplorerAgent = `name = "dots-explorer"
description = "Read-only dots explorer for bounded codebase exploration, impact scans, and test/log triage."
model = "gpt-5.3-codex-spark"
model_reasoning_effort = "high"
sandbox_mode = "read-only"
developer_instructions = """
You are a dots explorer subagent.

Scope:
- Inspect code, docs, tests, manifests, logs, and command output.
- Return concise findings with file:line references and confidence.
- Prefer CodeGraph for source-code architecture, symbols, call flow, and impact analysis.
- Prefer targeted rg/sed reads for manifests, docs, config, and scripts.

Boundaries:
- Do not edit files.
- Do not mutate GitHub, PRs, releases, package managers, or real user configuration.
- Do not validate dotfiles behavior against the operator's real home.
- Keep the main agent responsible for requirements, decisions, integration, and final verification.
"""
nickname_candidates = ["Dots Scout", "Dots Cartographer", "Dots Radar"]
`

const codexWorkerAgent = `name = "dots-worker"
description = "dots implementation worker for explicitly assigned, non-overlapping files or modules."
model = "gpt-5.3-codex-spark"
model_reasoning_effort = "high"
sandbox_mode = "workspace-write"
developer_instructions = """
You are a dots worker subagent.

Scope:
- Implement only the explicitly assigned files or modules.
- Keep diffs surgical and trace every changed line to the requested slice.
- Follow existing repository patterns and the dots domain language.
- Return a concise handoff with changed files, tests run, and any remaining risks.

Boundaries:
- You are not alone in the codebase; do not revert or overwrite edits outside your assigned ownership.
- Do not mutate GitHub, PRs, releases, package managers, or real user configuration.
- Use sandboxed --home, --source-root, --state-root, or temporary config paths for dotfiles behavior.
- Leave requirements, decisions, external state, integration, and final verification to the main agent.
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

// ConvergeCodexSparkDelegation injects the opt-in Codex Spark delegation
// guidance into Codex instructions only. Legacy argote-owned marker pairs are
// migrated to the dots-owned marker pair during the upsert.
func ConvergeCodexSparkDelegation(home string) error {
	return errors.Join(
		convergeCodexDelegation(filepath.Join(home, ".codex", "AGENTS.md")),
		writeCodexAgentFile(home, codexExplorerAgentFile, codexExplorerAgent),
		writeCodexAgentFile(home, codexWorkerAgentFile, codexWorkerAgent),
	)
}

// RemoveCodexSparkDelegation removes both current dots-owned and legacy
// argote-owned Codex Spark delegation blocks without touching the rest of the
// agent baseline.
func RemoveCodexSparkDelegation(home string) error {
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
