package agentinstructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveGentleAITriggerRulesRemovesSupportedAgentBlocks(t *testing.T) {
	home := t.TempDir()
	paths := instructionPaths(home, []string{"codex", "claude-code", "opencode", "antigravity", "vscode-copilot"})
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

func TestConvergeDotsAgentRulesRemovesPersonaAndInjectsRules(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	content := "before\n\n" +
		gentleAIPersonaStart + "\n## Personality\nSenior Architect, 15+ years experience, GDE & MVP.\n" + gentleAIPersonaEnd + "\n\n" +
		gentleAITriggerRulesStart + "\nstale review-readability rule\n" + gentleAITriggerRulesEnd + "\n\n" +
		"after\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	if err := ConvergeDotsAgentRules(home, "codex"); err != nil {
		t.Fatalf("ConvergeDotsAgentRules() error = %v", err)
	}
	if err := ConvergeDotsAgentRules(home, "codex"); err != nil {
		t.Fatalf("ConvergeDotsAgentRules() second run error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := string(got)
	for _, not := range []string{
		gentleAIPersonaStart,
		gentleAIPersonaEnd,
		gentleAITriggerRulesStart,
		gentleAITriggerRulesEnd,
		"Senior Architect",
		"review-readability",
	} {
		if strings.Contains(out, not) {
			t.Fatalf("content kept %q\n%s", not, out)
		}
	}
	if strings.Count(out, dotsRulesStart) != 1 || strings.Count(out, dotsRulesEnd) != 1 {
		t.Fatalf("dots rules block should be present once\n%s", out)
	}
	if strings.Contains(out, codexDelegationStart) || strings.Contains(out, codexDelegationEnd) {
		t.Fatalf("baseline dots rules should not install Codex Spark delegation\n%s", out)
	}
	for _, name := range []string{codexExplorerAgentFile, codexWorkerAgentFile} {
		if _, err := os.Stat(filepath.Join(home, ".codex", "agents", name)); !os.IsNotExist(err) {
			t.Fatalf("baseline dots rules should not install native Codex agent %s; stat err = %v", name, err)
		}
	}
	for _, want := range []string{"Keep diffs surgical", "Choose the simplest change", "Plan before editing", "Verify before declaring success", "Use sandboxed HOME/config paths", "Delegation", "delegation skill", "Delegation Preflight", "external project state"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dots rules missing %q\n%s", want, out)
		}
	}
	for _, not := range []string{"Portable delegation policy", "agent surface", "model/tier", "strongest appropriate available model"} {
		if strings.Contains(out, not) {
			t.Fatalf("dots rules kept verbose delegation phrase %q\n%s", not, out)
		}
	}
	if strings.Contains(out, "Prefer dots JSON output") || strings.Contains(out, "scraping human prose") {
		t.Fatalf("dots rules kept JSON-output scope creep\n%s", out)
	}
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Fatalf("surrounding content not preserved\n%s", out)
	}
}

func TestConvergeDotsAgentRulesPreservesCustomPersonalitySection(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	content := "# Local Agent Guide\n\n" +
		"## Personality\n\n" +
		"Be concise, skeptical, and protect production data.\n\n" +
		"## Project Workflow\n\n" +
		"Keep this user-owned workflow section.\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	if err := ConvergeDotsAgentRules(home, "codex"); err != nil {
		t.Fatalf("ConvergeDotsAgentRules() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := string(got)
	for _, want := range []string{"## Personality", "Be concise, skeptical, and protect production data.", "## Project Workflow", "Keep this user-owned workflow section."} {
		if !strings.Contains(out, want) {
			t.Fatalf("custom content missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, dotsRulesStart) {
		t.Fatalf("dots rules not injected\n%s", out)
	}
}

func TestSyncCopilotCLIEngramProtocolCopiesVSCodePromptBlock(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "Library", "Application Support", "Code", "User", "prompts", "gentle-ai.instructions.md")
	target := filepath.Join(home, ".copilot", "copilot-instructions.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	sourceContent := "prefix\n\n" +
		gentleAIEngramStart + "\n## Engram Persistent Memory — Protocol\n\nUse Engram proactively.\n" + gentleAIEngramEnd +
		"\n\nsuffix\n"
	if err := os.WriteFile(source, []byte(sourceContent), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("existing\n\n"+dotsRulesStart+"\n"+dotsRulesBlock+"\n"+dotsRulesEnd+"\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if err := SyncCopilotCLIEngramProtocol(home); err != nil {
		t.Fatalf("SyncCopilotCLIEngramProtocol() error = %v", err)
	}
	if err := SyncCopilotCLIEngramProtocol(home); err != nil {
		t.Fatalf("SyncCopilotCLIEngramProtocol() second run error = %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	out := string(got)
	for _, want := range []string{"existing", dotsRulesStart, "Keep diffs surgical", gentleAIEngramStart, "## Engram Persistent Memory — Protocol", "Use Engram proactively.", gentleAIEngramEnd} {
		if !strings.Contains(out, want) {
			t.Fatalf("Copilot CLI instructions missing %q\n%s", want, out)
		}
	}
	if strings.Count(out, gentleAIEngramStart) != 1 || strings.Count(out, gentleAIEngramEnd) != 1 {
		t.Fatalf("Engram block should be present once\n%s", out)
	}
	if strings.Contains(out, "prefix") || strings.Contains(out, "suffix") {
		t.Fatalf("sync should copy only the marked Engram block body\n%s", out)
	}
}

func TestConvergeCodexDelegationIsOptInCodexOnly(t *testing.T) {
	home := t.TempDir()

	if err := ConvergeDotsAgentRules(home, "codex", "claude-code", "opencode", "antigravity", "vscode-copilot"); err != nil {
		t.Fatalf("ConvergeDotsAgentRules() error = %v", err)
	}
	if err := ConvergeCodexDelegation(home); err != nil {
		t.Fatalf("ConvergeCodexDelegation() error = %v", err)
	}
	if err := ConvergeCodexDelegation(home); err != nil {
		t.Fatalf("ConvergeCodexDelegation() second run error = %v", err)
	}

	codexPath := filepath.Join(home, ".codex", "AGENTS.md")
	codex, err := os.ReadFile(codexPath)
	if err != nil {
		t.Fatalf("read %s: %v", codexPath, err)
	}
	codexContent := string(codex)
	if strings.Count(codexContent, codexDelegationStart) != 1 || strings.Count(codexContent, codexDelegationEnd) != 1 {
		t.Fatalf("Codex delegation block should be present once\n%s", codexContent)
	}
	blockStart := strings.Index(codexContent, codexDelegationStart)
	blockEnd := strings.Index(codexContent, codexDelegationEnd)
	if blockStart < 0 || blockEnd < 0 || blockEnd <= blockStart {
		t.Fatalf("Codex delegation block bounds missing\n%s", codexContent)
	}
	codexDelegationContent := codexContent[blockStart:blockEnd]
	if strings.Count(strings.ToLower(codexDelegationContent), "gpt-5.6 sol") != 3 {
		t.Fatalf("Codex delegation block should include the configured GPT-5.6 Sol references exactly three times\n%s", codexContent)
	}
	for _, want := range []string{"delegation` skill", "dots-explorer", "dots-worker", "standing authorization", "across repositories", "tool-level permission required", "reservar", "GPT-5.6 Sol"} {
		if !strings.Contains(codexDelegationContent, want) {
			t.Fatalf("Codex delegation block missing policy phrase %q\n%s", want, codexContent)
		}
	}
	if strings.Contains(codexDelegationContent, "dots-development-loop") || strings.Contains(codexDelegationContent, "dots-named workflow") {
		t.Fatalf("Codex delegation block should be portable and avoid dots-specific workflow names\n%s", codexContent)
	}
	for _, duplicatedPolicy := range []string{"Delegate by default for non-trivial work", "would mutate GitHub/PR/release or other external state", "accepted/rejected findings", "Run Delegation Preflight before non-trivial work"} {
		if strings.Contains(codexDelegationContent, duplicatedPolicy) {
			t.Fatalf("Codex delegation overlay should not duplicate portable policy phrase %q\n%s", duplicatedPolicy, codexContent)
		}
	}
	if strings.Contains(codexContent, legacyCodexDelegationStart) || strings.Contains(codexContent, legacyCodexDelegationEnd) {
		t.Fatalf("Codex delegation block should use dots-owned markers, not legacy markers\n%s", codexContent)
	}
	if strings.Contains(codexContent, "dots:codex-spark-delegation") {
		t.Fatalf("Codex delegation block should use the model-neutral dots:delegation marker\n%s", codexContent)
	}
	for _, name := range []string{codexExplorerAgentFile, codexWorkerAgentFile} {
		if _, err := os.Stat(filepath.Join(home, ".codex", "agents", name)); !os.IsNotExist(err) {
			t.Fatalf("Codex delegation should not install native agent file %s; stat err = %v", name, err)
		}
	}

	nonCodexPaths := instructionPaths(home, []string{"claude-code", "opencode", "antigravity", "vscode-copilot"})
	for _, path := range nonCodexPaths {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(got)
		if !strings.Contains(content, dotsRulesStart) {
			t.Fatalf("%s missing shared dots rules\n%s", path, content)
		}
		for _, want := range []string{"delegation skill", "Delegation Preflight", "external project state"} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s missing portable delegation policy phrase %q\n%s", path, want, content)
			}
		}
		for _, not := range []string{codexDelegationStart, codexDelegationEnd, "gpt-5.6 Sol", "gpt-5.6-sol"} {
			if strings.Contains(content, not) {
				t.Fatalf("%s unexpectedly contains Codex delegation guidance %q\n%s", path, not, content)
			}
		}
	}
}

func TestConvergeDotsAgentRulesRemovesLegacyMarkerlessPersona(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	content := "prefix\n\n" +
		"## Personality\n\nSenior Architect, 15+ years experience, GDE & MVP.\n\n" +
		"## Persona Scope (CRITICAL — read this first)\n\nThe persona styles HOW YOU TALK, not WHAT YOU BUILD.\n\n" +
		"## Language\n\n- Match the user's current language in your REPLY ONLY (see Persona Scope above).\n\n" +
		"## Tone\n\nPassionate and direct, but from a place of CARING.\n\n" +
		"## Philosophy\n\n- CONCEPTS > CODE\n\n" +
		"## Expertise\n\nClean/Hexagonal/Screaming Architecture.\n\n" +
		"## Behavior\n\n- Correct errors ruthlessly but explain WHY technically\n\n" +
		"## Existing Section\n\nkeep me\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	if err := ConvergeDotsAgentRules(home, "claude-code"); err != nil {
		t.Fatalf("ConvergeDotsAgentRules() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := string(got)
	for _, not := range []string{"## Personality", "Senior Architect", "## Persona Scope", "## Language", "## Tone", "## Philosophy", "## Expertise", "## Behavior"} {
		if strings.Contains(out, not) {
			t.Fatalf("legacy persona cleanup kept %q\n%s", not, out)
		}
	}
	if !strings.Contains(out, "## Existing Section\n\nkeep me") {
		t.Fatalf("legacy persona cleanup removed unrelated section\n%s", out)
	}
	if !strings.Contains(out, dotsRulesStart) {
		t.Fatalf("dots rules not injected\n%s", out)
	}
}

func TestConvergeCodexDelegationMigratesLegacyMarkers(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	content := "before\n\n" +
		legacyDotsDelegationStart + "\nstale dots spark guidance\n" + legacyDotsDelegationEnd + "\n\n" +
		legacyCodexDelegationStart + "\nstale argote guidance\n" + legacyCodexDelegationEnd + "\n\nafter\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	if err := ConvergeCodexDelegation(home); err != nil {
		t.Fatalf("ConvergeCodexDelegation() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := string(got)
	for _, not := range []string{legacyDotsDelegationStart, legacyDotsDelegationEnd, legacyCodexDelegationStart, legacyCodexDelegationEnd, "stale dots spark guidance", "stale argote guidance"} {
		if strings.Contains(out, not) {
			t.Fatalf("legacy content kept %q\n%s", not, out)
		}
	}
	if strings.Count(out, codexDelegationStart) != 1 || strings.Count(out, codexDelegationEnd) != 1 {
		t.Fatalf("dots-owned Codex delegation block should be present once\n%s", out)
	}
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Fatalf("surrounding content not preserved\n%s", out)
	}
}

func TestRemoveCodexDelegationRemovesCurrentAndLegacyMarkers(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	content := "before\n\n" +
		dotsRulesStart + "\n" + dotsRulesBlock + "\n" + dotsRulesEnd + "\n\n" +
		codexDelegationStart + "\ncurrent\n" + codexDelegationEnd + "\n\n" +
		legacyDotsDelegationStart + "\nlegacy dots\n" + legacyDotsDelegationEnd + "\n\n" +
		legacyCodexDelegationStart + "\nlegacy\n" + legacyCodexDelegationEnd + "\n\nafter\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	agentsDir := filepath.Join(home, ".codex", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatalf("mkdir Codex agents dir: %v", err)
	}
	for _, name := range []string{codexExplorerAgentFile, codexWorkerAgentFile} {
		if err := os.WriteFile(filepath.Join(agentsDir, name), []byte("stale dots-owned agent"), 0o600); err != nil {
			t.Fatalf("write native Codex agent %s: %v", name, err)
		}
	}

	if err := RemoveCodexDelegation(home); err != nil {
		t.Fatalf("RemoveCodexDelegation() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := string(got)
	for _, not := range []string{codexDelegationStart, codexDelegationEnd, legacyDotsDelegationStart, legacyDotsDelegationEnd, legacyCodexDelegationStart, legacyCodexDelegationEnd, "\ncurrent\n", "\nlegacy dots\n", "\nlegacy\n"} {
		if strings.Contains(out, not) {
			t.Fatalf("delegation cleanup kept %q\n%s", not, out)
		}
	}
	if !strings.Contains(out, dotsRulesStart) || !strings.Contains(out, "Keep diffs surgical") {
		t.Fatalf("delegation cleanup removed dots rules\n%s", out)
	}
	if !strings.Contains(out, "before") || !strings.Contains(out, "after") {
		t.Fatalf("surrounding content not preserved\n%s", out)
	}
	for _, name := range []string{codexExplorerAgentFile, codexWorkerAgentFile} {
		if _, err := os.Stat(filepath.Join(home, ".codex", "agents", name)); !os.IsNotExist(err) {
			t.Fatalf("RemoveCodexDelegation should remove native Codex agent %s; stat err = %v", name, err)
		}
	}
}

func TestDelegationWorkflowDocumentsPreflightAndToolLevelConflict(t *testing.T) {
	workflowPath := filepath.Join("..", "..", "workflows", "dots-development-loop.md")
	got, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}
	workflow := string(got)
	for _, want := range []string{
		"Starting this workflow counts as repo-level authorization",
		"tool-level permission required",
		"`delegation` skill",
		"Delegation Preflight",
		"~/.codex/AGENTS.md",
		"dots:delegation",
		"~/.codex/AGENTS.md",
		"without-codex-delegation",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("workflow delegation docs missing %q", want)
		}
	}

	delegationDocPath := filepath.Join("..", "..", "docs", "agents", "delegation.md")
	got, err = os.ReadFile(delegationDocPath)
	if err != nil {
		t.Fatalf("read %s: %v", delegationDocPath, err)
	}
	delegationDoc := string(got)
	for _, want := range []string{
		"detailed procedure lives in the `delegation` skill",
		"Delegation Preflight is required for non-trivial work",
		"tool-level permission required",
		"~/.codex/AGENTS.md",
		"--tag without-codex-delegation",
		"codex-delegation",
	} {
		if !strings.Contains(delegationDoc, want) {
			t.Fatalf("delegation docs missing %q", want)
		}
	}
}
