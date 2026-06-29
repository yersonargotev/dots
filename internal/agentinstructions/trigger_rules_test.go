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
	for _, want := range []string{"Keep diffs surgical", "Choose the simplest change", "Plan before editing", "Verify before declaring success", "Use sandboxed HOME/config paths"} {
		if !strings.Contains(out, want) {
			t.Fatalf("dots rules missing %q\n%s", want, out)
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

func TestConvergeCodexSparkDelegationIsOptInCodexOnly(t *testing.T) {
	home := t.TempDir()

	if err := ConvergeDotsAgentRules(home, "codex", "claude-code", "opencode", "antigravity", "vscode-copilot"); err != nil {
		t.Fatalf("ConvergeDotsAgentRules() error = %v", err)
	}
	if err := ConvergeCodexSparkDelegation(home); err != nil {
		t.Fatalf("ConvergeCodexSparkDelegation() error = %v", err)
	}
	if err := ConvergeCodexSparkDelegation(home); err != nil {
		t.Fatalf("ConvergeCodexSparkDelegation() second run error = %v", err)
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
	if strings.Count(codexContent, "gpt-5.3-codex-spark") != 2 {
		t.Fatalf("Codex delegation block should include Spark role guidance exactly twice\n%s", codexContent)
	}
	for _, want := range []string{"Delegate by default for non-trivial work", "If a selected skill requires subagents, follow the skill", "would mutate GitHub/PR/release or other external state", "Use the strongest appropriate available model", "do not force Spark for judgment-heavy review"} {
		if !strings.Contains(codexContent, want) {
			t.Fatalf("Codex delegation block missing policy phrase %q\n%s", want, codexContent)
		}
	}
	if strings.Contains(codexContent, legacyCodexDelegationStart) || strings.Contains(codexContent, legacyCodexDelegationEnd) {
		t.Fatalf("Codex delegation block should use dots-owned markers, not legacy markers\n%s", codexContent)
	}

	nonCodexPaths := []string{
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".config", "opencode", "AGENTS.md"),
		filepath.Join(home, ".gemini", "GEMINI.md"),
		filepath.Join(home, "Library", "Application Support", "Code", "User", "prompts", "gentle-ai.instructions.md"),
		filepath.Join(home, ".config", "Code", "User", "prompts", "gentle-ai.instructions.md"),
	}
	for _, path := range nonCodexPaths {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(got)
		if !strings.Contains(content, dotsRulesStart) {
			t.Fatalf("%s missing shared dots rules\n%s", path, content)
		}
		for _, not := range []string{codexDelegationStart, codexDelegationEnd, "gpt-5.3-codex-spark"} {
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

func TestConvergeCodexSparkDelegationMigratesLegacyMarkers(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	content := "before\n\n" + legacyCodexDelegationStart + "\nstale spark guidance\n" + legacyCodexDelegationEnd + "\n\nafter\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	if err := ConvergeCodexSparkDelegation(home); err != nil {
		t.Fatalf("ConvergeCodexSparkDelegation() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := string(got)
	for _, not := range []string{legacyCodexDelegationStart, legacyCodexDelegationEnd, "stale spark guidance"} {
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

func TestRemoveCodexSparkDelegationRemovesCurrentAndLegacyMarkers(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	content := "before\n\n" +
		dotsRulesStart + "\n" + dotsRulesBlock + "\n" + dotsRulesEnd + "\n\n" +
		codexDelegationStart + "\ncurrent\n" + codexDelegationEnd + "\n\n" +
		legacyCodexDelegationStart + "\nlegacy\n" + legacyCodexDelegationEnd + "\n\nafter\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	if err := RemoveCodexSparkDelegation(home); err != nil {
		t.Fatalf("RemoveCodexSparkDelegation() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := string(got)
	for _, not := range []string{codexDelegationStart, codexDelegationEnd, legacyCodexDelegationStart, legacyCodexDelegationEnd, "current", "legacy"} {
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
}
