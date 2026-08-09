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

func TestDeliveryWorkflowDocumentsDelegationPreflight(t *testing.T) {
	workflowPath := filepath.Join("..", "..", "workflows", "delivery-issue.md")
	got, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}
	workflow := string(got)
	for _, want := range []string{
		"explicit trigger plus successful admission authorizes",
		"tool-level permission required",
		"`delegation` skill",
		"Delegation Preflight",
		"docs/agents/delegation.md",
		"Subagents never change labels",
		"Independent review remains mandatory",
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

func TestDeliverySkillIsExplicitThinAdapter(t *testing.T) {
	root := filepath.Join("..", "..")
	skillPath := filepath.Join(root, ".agents", "skills", "delivery-issue", "SKILL.md")
	got, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read %s: %v", skillPath, err)
	}
	content := string(got)
	for _, want := range []string{
		"name: delivery-issue",
		"disable-model-invocation: true",
		"<issue-number-or-url>",
		"workflows/delivery-issue.md",
		"normative workflow specification",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("delivery skill missing %q", want)
		}
	}

	linkPath := filepath.Join(root, ".claude", "skills", "delivery-issue")
	if target, err := os.Readlink(linkPath); err != nil || target != "../../.agents/skills/delivery-issue" {
		t.Fatalf("delivery skill link = %q, %v", target, err)
	}

	for _, legacy := range []string{"dots-pr-creation", "dots-pr-fast-path"} {
		path := filepath.Join(root, ".agents", "skills", legacy)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy skill %s should be absent; stat err = %v", legacy, err)
		}
	}
}

func TestDeliveryContractVocabularyIsConsistent(t *testing.T) {
	root := filepath.Join("..", "..")
	terms := []string{
		"Delivery Contract",
		"Contract Source",
		"Delivery Unit",
		"Tracking Issue",
		"Execution Frontier",
	}
	for _, path := range []string{
		filepath.Join(root, "CONTEXT.md"),
		filepath.Join(root, "docs", "agents", "delivery-contract.md"),
		filepath.Join(root, "workflows", "delivery-issue.md"),
	} {
		assertDocumentContainsAll(t, path, terms)
	}
}

func TestDeliveryContractAdmissionSupportsProducerNeutralSources(t *testing.T) {
	root := filepath.Join("..", "..")
	contractPath := filepath.Join(root, "docs", "agents", "delivery-contract.md")
	contract := readContractDocument(t, contractPath)

	sources := []string{
		"complete historical Agent Brief",
		"complete standalone issue body",
		"delivery ticket body composed",
		"native parent specification",
	}
	assertDocumentContainsAll(t, contractPath, sources)
	for i := 1; i < 3; i++ {
		if strings.Index(contract, sources[i-1]) >= strings.Index(contract, sources[i]) {
			t.Fatalf("Contract Source precedence is not documented in order: %q before %q", sources[i-1], sources[i])
		}
	}

	workflowPath := filepath.Join(root, "workflows", "delivery-issue.md")
	assertDocumentContainsAll(t, workflowPath, []string{
		"complete historical Agent Brief",
		"complete standalone issue body",
		"delivery ticket body composed",
		"native parent",
		"issue-level `type:*` label is not part of admission",
		"historical Agent Brief issues remain deliverable without migration",
	})
	assertDocumentContainsAll(t, filepath.Join(root, "docs", "agents", "agent-brief.md"), []string{
		"historical Agent Brief",
		"complete Agent Brief",
		"An issue that publishes an Agent Brief",
	})
	assertDocumentContainsAll(t, contractPath, []string{"or add an Agent Brief before delivery"})
	assertDocumentContainsAll(t, filepath.Join(root, "NOTES.md"), []string{
		"selected Delivery Contract",
		"complete historical Agent Brief",
		"complete standalone issue body",
		"delivery ticket composed with its native parent specification and relationships",
		"not a mandatory comment for every issue",
	})
}

func TestDeliveryContractPreservesNativeExecutionStateAndSnapshots(t *testing.T) {
	root := filepath.Join("..", "..")
	contractPath := filepath.Join(root, "docs", "agents", "delivery-contract.md")
	workflowPath := filepath.Join(root, "workflows", "delivery-issue.md")

	assertDocumentContainsAll(t, contractPath, []string{
		"needs-triage",
		"Incomplete, contradictory, or ambiguous",
		"readiness-label churn",
		"Tracking Issue",
		"non-mutating `tracking` result",
		"creates no branch, commit, or pull",
		"SHA-256 digest",
		"updatedAt",
		"Revalidate the snapshot immediately before code mutation",
		"pull-request\ncreation, and merge",
	})
	assertDocumentContainsAll(t, workflowPath, []string{
		"needs-triage",
		"missing, incomplete, contradictory, or ambiguous",
		"Open native blockers do not invalidate completeness",
		"Tracking Issue",
		"non-mutating `tracking` Delivery Result",
		"Do not create a branch, commit, or PR",
		"SHA-256 digest",
		"updatedAt",
		"Revalidate the complete snapshot before modifying code",
		"before opening the PR",
		"before merge",
		"category; triage/readiness label",
		"issue identity, state, and `updatedAt`",
		"Any difference restarts",
		"only semantic change is blocker state",
	})

	assertDocumentContainsAll(t, filepath.Join(root, "docs", "agents", "triage-labels.md"), []string{
		"specification is complete",
		"Execution Frontier",
		"Do not use `needs-triage` as a blocked-work label",
	})
}

func TestDeliveryContractAdmissionScenarioMatrix(t *testing.T) {
	root := filepath.Join("..", "..")
	path := filepath.Join(root, "docs", "agents", "delivery-contract.md")
	scenarios := readMarkdownScenarioMatrix(t, path)

	wants := map[string][2]string{
		"historical-agent-brief": {"complete Agent Brief", "using the Agent Brief as the Contract Source"},
		"standalone-body":        {"complete issue body", "using the standalone body as the Contract Source"},
		"composed-ticket":        {"native parent specification", "using the composed ticket Contract Source"},
		"blocked-unit":           {"open native `blockedBy`", "retain `ready-for-agent`, and create no branch or PR"},
		"tracking-issue":         {"native sub-issues", "Return `tracking`"},
		"incomplete-source":      {"omits required scope", "Return `needs-triage`"},
		"contradictory-source":   {"Duplicate or conflicting", "Return `needs-triage`"},
		"stale-snapshot":         {"relationship identity, state, or `updatedAt` snapshot differs", "Restart admission before code mutation, PR creation, or merge"},
	}
	if len(scenarios) != len(wants) {
		t.Fatalf("admission scenario count = %d, want %d: %#v", len(scenarios), len(wants), scenarios)
	}
	for name, want := range wants {
		got, ok := scenarios[name]
		if !ok {
			t.Errorf("admission scenario %q is missing", name)
			continue
		}
		if !strings.Contains(got[0], want[0]) {
			t.Errorf("admission scenario %q evidence = %q, want %q", name, got[0], want[0])
		}
		if !strings.Contains(got[1], want[1]) {
			t.Errorf("admission scenario %q result = %q, want %q", name, got[1], want[1])
		}
	}
}

func TestIssueCreationSupportsDirectProducerPathWithoutVendoringExternalSkills(t *testing.T) {
	root := filepath.Join("..", "..")
	issueCreationPath := filepath.Join(root, ".agents", "skills", "dots-issue-creation", "SKILL.md")
	assertDocumentContainsAll(t, issueCreationPath, []string{
		"--parent <parent-number>",
		"--add-sub-issue <child-number>",
		"--add-blocked-by <blocker-number>",
		"parent,subIssues,blockedBy,blocking,labels",
		"`to-spec` → `to-tickets` → `delivery-issue`",
		"mandatory post-processor",
	})
	assertDocumentContainsAll(t, filepath.Join(root, "docs", "agents", "issue-tracker.md"), []string{
		"`to-spec` → `to-tickets` → `delivery-issue`",
		"unmodified and unvendored",
		"not a mandatory post-processor",
	})

	for _, externalSkill := range []string{"grill-with-docs", "to-spec", "to-tickets", "triage"} {
		path := filepath.Join(root, ".agents", "skills", externalSkill)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("external skill %q must not be vendored at %s; stat error = %v", externalSkill, path, err)
		}
	}
}

func TestDeliveryContractEvidenceAndRepositoryLanguage(t *testing.T) {
	root := filepath.Join("..", "..")
	assertDocumentContainsAll(t, filepath.Join(root, ".github", "PULL_REQUEST_TEMPLATE.md"), []string{
		"exactly one `type:*` label",
		"Delivery Contract",
		"Contract Source snapshot",
		"Acceptance coverage",
		"Independent review",
	})

	for _, path := range []string{
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, "NOTES.md"),
		filepath.Join(root, "docs", "agents", "agent-brief.md"),
		filepath.Join(root, "docs", "agents", "issue-tracker.md"),
		filepath.Join(root, "docs", "agents", "triage-labels.md"),
		filepath.Join(root, ".agents", "skills", "dots-issue-creation", "SKILL.md"),
		filepath.Join(root, "workflows", "delivery-issue.md"),
	} {
		assertDocumentOmitsAll(t, path, []string{
			"sole readiness producer",
			"sole producer of `ready-for-agent`",
			"requires both the label and exactly one complete",
			"Every issue handed to `delivery-issue` requires exactly one complete Agent Brief",
			"Each issue has exactly one comment headed `## Agent Brief`",
			"only an in-place Agent Brief revision does",
		})
	}
}

func readContractDocument(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(got)
}

func assertDocumentContainsAll(t *testing.T, path string, wants []string) {
	t.Helper()
	content := readContractDocument(t, path)
	for _, want := range wants {
		if !strings.Contains(content, want) {
			t.Errorf("delivery contract document %s missing %q", path, want)
		}
	}
}

func assertDocumentOmitsAll(t *testing.T, path string, forbidden []string) {
	t.Helper()
	content := readContractDocument(t, path)
	for _, phrase := range forbidden {
		if strings.Contains(content, phrase) {
			t.Errorf("delivery contract document %s retains obsolete language %q", path, phrase)
		}
	}
}

func readMarkdownScenarioMatrix(t *testing.T, path string) map[string][2]string {
	t.Helper()
	scenarios := make(map[string][2]string)
	for _, line := range strings.Split(readContractDocument(t, path), "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) != 5 {
			t.Fatalf("malformed scenario row in %s: %q", path, line)
		}
		name := strings.Trim(strings.TrimSpace(cells[1]), "`")
		scenarios[name] = [2]string{strings.TrimSpace(cells[2]), strings.TrimSpace(cells[3])}
	}
	return scenarios
}
