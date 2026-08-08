package agentinstructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvergeCodexDelegationIsOptInCodexOnly(t *testing.T) {
	home := t.TempDir()

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
	if strings.Count(codexDelegationContent, "gpt-5.6-sol") != 2 {
		t.Fatalf("Codex delegation block should include GPT-5.6 Sol role guidance exactly twice\n%s", codexContent)
	}
	for _, want := range []string{"delegation` skill", "dots-explorer", "dots-worker", "standing authorization", "across repositories", "tool-level permission required", "reserve this profile's GPT-5.6 Sol low default"} {
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
	for _, tc := range []struct {
		name  string
		wants []string
	}{
		{name: codexExplorerAgentFile, wants: []string{`name = "dots-explorer"`, `sandbox_mode = "read-only"`, `model = "gpt-5.6-sol"`, `model_reasoning_effort = "low"`, "Outcome:", "Success means:", "stop when the question is answered", "Load the delegation skill", "Do not edit files."}},
		{name: codexWorkerAgentFile, wants: []string{`name = "dots-worker"`, `sandbox_mode = "workspace-write"`, `model = "gpt-5.6-sol"`, `model_reasoning_effort = "low"`, "Outcome:", "Success means:", "most relevant non-destructive validation", "Load the delegation skill", "changed files"}},
	} {
		path := filepath.Join(home, ".codex", "agents", tc.name)
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read native Codex agent %s: %v", path, err)
		}
		content := string(got)
		for _, want := range tc.wants {
			if !strings.Contains(content, want) {
				t.Fatalf("native Codex agent %s missing %q\n%s", path, want, content)
			}
		}
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
		dotsRulesStart + "\nportable rules\n" + dotsRulesEnd + "\n\n" +
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
	if !strings.Contains(out, dotsRulesStart) || !strings.Contains(out, "portable rules") {
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

func TestRetireGentleAIStateRemovesOnlyOwnedMarkerBlocks(t *testing.T) {
	home := t.TempDir()
	paths := instructionPaths(home, []string{"codex", "claude-code", "opencode", "antigravity", "vscode-copilot"})
	content := "user before\n\n" +
		gentleAITriggerRulesStart + "\ntrigger rules\n" + gentleAITriggerRulesEnd + "\n\n" +
		gentleAIPersonaStart + "\npersona\n" + gentleAIPersonaEnd + "\n\n" +
		gentleAIEngramStart + "\nengram\n" + gentleAIEngramEnd + "\n\n" +
		dotsRulesStart + "\ndots rules\n" + dotsRulesEnd + "\n\n" +
		codexDelegationStart + "\nindependent delegation\n" + codexDelegationEnd + "\n\n" +
		"## Personality\nUnmarked user-owned personality.\n\nuser after\n"
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if err := RetireGentleAIState(home); err != nil {
		t.Fatalf("RetireGentleAIState() error = %v", err)
	}

	for _, path := range paths {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read preserved instruction file %s: %v", path, err)
		}
		output := string(got)
		for _, removed := range []string{
			gentleAITriggerRulesStart, gentleAIPersonaStart, gentleAIEngramStart, dotsRulesStart,
			"trigger rules", "\npersona\n", "\nengram\n", "\ndots rules\n",
		} {
			if strings.Contains(output, removed) {
				t.Errorf("retirement kept owned content %q in %s:\n%s", removed, path, output)
			}
		}
		for _, preserved := range []string{
			"user before", codexDelegationStart, "independent delegation",
			"## Personality", "Unmarked user-owned personality.", "user after",
		} {
			if !strings.Contains(output, preserved) {
				t.Errorf("retirement removed unowned content %q from %s:\n%s", preserved, path, output)
			}
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Errorf("instruction file mode = %v, %v; want 0640", info, err)
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
		"dots-explorer.toml",
		"dots-worker.toml",
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
		"~/.codex/agents/dots-explorer.toml",
		"~/.codex/agents/dots-worker.toml",
		"--tag without-codex-delegation",
	} {
		if !strings.Contains(delegationDoc, want) {
			t.Fatalf("delegation docs missing %q", want)
		}
	}
}
