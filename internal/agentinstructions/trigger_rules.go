package agentinstructions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	copiedDelegationSkill      = "delegation"
)

// codexExplorerAgentSol and codexWorkerAgentSol are the last emitted native
// agent files. Retirement compares exact bytes so it never deletes user edits.
const codexExplorerAgentSol = `name = "dots-explorer"
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

const codexWorkerAgentSol = `name = "dots-worker"
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

const codexExplorerAgentSpark = `name = "dots-explorer"
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

const codexExplorerAgentSparkWithSkill = `name = "dots-explorer"
description = "Read-only dots explorer for bounded codebase exploration, impact scans, and test/log triage."
model = "gpt-5.3-codex-spark"
model_reasoning_effort = "high"
sandbox_mode = "read-only"
developer_instructions = """
You are a dots explorer subagent.

Scope:
- Inspect code, docs, tests, manifests, logs, and command output.
- Load the delegation skill when available and return concise findings with file:line references and confidence.
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

const codexWorkerAgentSpark = `name = "dots-worker"
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

const codexWorkerAgentSparkWithSkill = `name = "dots-worker"
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
- Load the delegation skill when available and return a concise handoff with changed files, tests run, and any remaining risks.

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

// RetireGentleAIState removes only marker-delimited instruction content whose
// ownership is explicit. It preserves instruction files and unmarked content;
// retired Codex delegation state is handled by a separate evidence-gated migration.
func RetireGentleAIState(home string) error {
	var errs []error
	for _, path := range instructionPaths(home, []string{"codex", "claude-code", "opencode", "antigravity", "vscode-copilot"}) {
		if err := removeMarkedBlocks(
			path,
			textblock.Markers{Start: gentleAITriggerRulesStart, End: gentleAITriggerRulesEnd},
			textblock.Markers{Start: gentleAIPersonaStart, End: gentleAIPersonaEnd},
			textblock.Markers{Start: gentleAIEngramStart, End: gentleAIEngramEnd},
			textblock.Markers{Start: dotsRulesStart, End: dotsRulesEnd},
		); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RetirementReport records only state that the narrow migration removed or
// deliberately left for the user. Paths are home-relative for portable reports.
type RetirementReport struct {
	Removed       []string `json:"removed"`
	ManualCleanup []string `json:"manual_cleanup"`
}

// RetireCodexDelegation removes exact dots-owned delegation state. It fails
// closed for malformed markers and never deletes a user-modified file, symlink,
// or copied delegation skill.
func RetireCodexDelegation(home string) (RetirementReport, error) {
	report := RetirementReport{Removed: []string{}, ManualCleanup: []string{}}
	if removed, manual, err := removeCodexDelegation(filepath.Join(home, ".codex", "AGENTS.md")); err != nil {
		return report, err
	} else if removed {
		report.Removed = append(report.Removed, "~/.codex/AGENTS.md delegation blocks")
	} else if manual {
		report.ManualCleanup = append(report.ManualCleanup, "~/.codex/AGENTS.md")
	}
	for _, name := range []string{codexExplorerAgentFile, codexWorkerAgentFile} {
		removed, manual, err := removeKnownCodexAgentFile(home, name)
		if err != nil {
			return report, err
		}
		if removed {
			report.Removed = append(report.Removed, "~/.codex/agents/"+name)
		}
		if manual {
			report.ManualCleanup = append(report.ManualCleanup, "~/.codex/agents/"+name)
		}
	}
	skillPath := filepath.Join(home, ".agents", "skills", copiedDelegationSkill)
	if _, err := os.Lstat(skillPath); err == nil {
		report.ManualCleanup = append(report.ManualCleanup, "~/.agents/skills/"+copiedDelegationSkill)
	} else if !errors.Is(err, os.ErrNotExist) {
		return report, fmt.Errorf("inspect copied delegation skill %s: %w", skillPath, err)
	}
	return report, nil
}

func removeKnownCodexAgentFile(home, name string) (removed, manual bool, err error) {
	path := filepath.Join(home, ".codex", "agents", name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("inspect Codex agent %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, true, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false, false, fmt.Errorf("read Codex agent %s: %w", path, err)
	}
	if !knownCodexAgentContent(name, content) {
		return false, true, nil
	}
	if err := os.Remove(path); err != nil {
		return false, false, fmt.Errorf("remove Codex agent %s: %w", path, err)
	}
	return true, false, nil
}

func knownCodexAgentContent(name string, content []byte) bool {
	var variants []string
	switch name {
	case codexExplorerAgentFile:
		variants = []string{codexExplorerAgentSpark, codexExplorerAgentSparkWithSkill, codexExplorerAgentSol}
	case codexWorkerAgentFile:
		variants = []string{codexWorkerAgentSpark, codexWorkerAgentSparkWithSkill, codexWorkerAgentSol}
	default:
		return false
	}
	for _, variant := range variants {
		if string(content) == variant {
			return true
		}
	}
	return false
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

func removeMarkedBlocks(path string, markers ...textblock.Markers) error {
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
	updated, err := textblock.Remove(string(content), markers...)
	if err != nil {
		return fmt.Errorf("remove marked agent instruction blocks from %s: %w", path, err)
	}
	if updated == string(content) {
		return nil
	}
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return fmt.Errorf("write agent instructions %s: %w", path, err)
	}
	return nil
}

func removeCodexDelegation(path string) (removed, manual bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("inspect Codex instructions %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, true, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false, false, fmt.Errorf("read Codex instructions %s: %w", path, err)
	}
	updated, err := textblock.Remove(
		string(content),
		textblock.Markers{Start: codexDelegationStart, End: codexDelegationEnd},
		textblock.Markers{Start: legacyDotsDelegationStart, End: legacyDotsDelegationEnd},
		textblock.Markers{Start: legacyCodexDelegationStart, End: legacyCodexDelegationEnd},
	)
	if err != nil {
		return false, false, fmt.Errorf("remove Codex delegation guidance from %s: %w", path, err)
	}
	if updated == string(content) {
		return false, false, nil
	}
	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return false, false, fmt.Errorf("write Codex instructions %s: %w", path, err)
	}
	return true, false, nil
}
