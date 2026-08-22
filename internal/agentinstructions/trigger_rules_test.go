package agentinstructions

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yersonargotev/dots/internal/textblock"
)

func TestRetireCodexDelegationRemovesOnlyKnownOwnedState(t *testing.T) {
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

	if err := os.WriteFile(filepath.Join(agentsDir, codexExplorerAgentFile), []byte(codexExplorerAgentSol), 0o600); err != nil {
		t.Fatalf("write Explorer agent: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, codexWorkerAgentFile), []byte(codexWorkerAgentSparkWithSkill), 0o600); err != nil {
		t.Fatalf("write Worker agent: %v", err)
	}
	skillPath := filepath.Join(home, ".agents", "skills", copiedDelegationSkill)
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir copied skill: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("copied skill"), 0o600); err != nil {
		t.Fatalf("write copied skill: %v", err)
	}

	report, err := RetireCodexDelegation(home)
	if err != nil {
		t.Fatalf("RetireCodexDelegation() error = %v", err)
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
			t.Fatalf("RetireCodexDelegation should remove native Codex agent %s; stat err = %v", name, err)
		}
	}
	if !strings.Contains(strings.Join(report.Removed, "\n"), "AGENTS.md delegation blocks") || len(report.Removed) != 3 {
		t.Fatalf("removed report = %#v", report.Removed)
	}
	if got := report.ManualCleanup; len(got) != 1 || got[0] != "~/.agents/skills/delegation" {
		t.Fatalf("manual cleanup report = %#v", got)
	}
	if _, err := os.Stat(skillPath); err != nil {
		t.Fatalf("copied skill was deleted: %v", err)
	}
}

func TestRetireCodexDelegationPreservesModifiedAndSymlinkAgents(t *testing.T) {
	home := t.TempDir()
	agentsDir := filepath.Join(home, ".codex", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modified := filepath.Join(agentsDir, codexExplorerAgentFile)
	if err := os.WriteFile(modified, []byte(codexExplorerAgentSol+"\nuser edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "user-worker.toml")
	if err := os.WriteFile(target, []byte(codexWorkerAgentSol), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(agentsDir, codexWorkerAgentFile)
	if err := os.Symlink(target, linked); err != nil {
		t.Fatal(err)
	}
	report, err := RetireCodexDelegation(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(modified); err != nil {
		t.Fatalf("modified agent removed: %v", err)
	}
	info, err := os.Lstat(linked)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink agent changed: %v, %v", info, err)
	}
	if got := strings.Join(report.ManualCleanup, ","); !strings.Contains(got, codexExplorerAgentFile) || !strings.Contains(got, codexWorkerAgentFile) {
		t.Fatalf("manual cleanup report = %#v", report.ManualCleanup)
	}
}

func TestRetireCodexDelegationFailsClosedForMalformedMarkers(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	malformed := "before\n" + codexDelegationStart + "\nunclosed\n"
	if err := os.WriteFile(path, []byte(malformed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RetireCodexDelegation(home); err == nil {
		t.Fatal("RetireCodexDelegation() error = nil, want malformed marker failure")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != malformed {
		t.Fatalf("malformed instructions changed: %q, %v", got, err)
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
		"## User instructions\nKeep this unmarked user-owned text.\n\nuser after\n"
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	report, err := RetireGentleAIState(home)
	if err != nil {
		t.Fatalf("RetireGentleAIState() error = %v", err)
	}
	if len(report.Removed) != len(paths) || len(report.ManualCleanup) != 0 {
		t.Fatalf("RetireGentleAIState() report = %#v, want one removal per instruction target", report)
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
			"user before", "## User instructions", "Keep this unmarked user-owned text.", "user after",
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

func TestRetireGentleAIStatePreservesAndReportsSymlinks(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "user-owned.md")
	content := gentleAITriggerRulesStart + "\nowned\n" + gentleAITriggerRulesEnd + "\n"
	if err := os.WriteFile(target, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}

	report, err := RetireGentleAIState(home)
	if err != nil {
		t.Fatalf("RetireGentleAIState() error = %v", err)
	}
	if len(report.Removed) != 0 || !reflect.DeepEqual(report.ManualCleanup, []string{"~/.codex/AGENTS.md"}) {
		t.Fatalf("RetireGentleAIState() report = %#v", report)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != content {
		t.Fatalf("symlink target changed: %q, %v", got, err)
	}
}

func TestRetireGentleAIStateRejectsInstructionPathOutsideHome(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	path := filepath.Join(outside, "AGENTS.md")
	content := gentleAITriggerRulesStart + "\nowned\n" + gentleAITriggerRulesEnd + "\n"
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".codex")); err != nil {
		t.Fatal(err)
	}

	report, err := RetireGentleAIState(home)
	if err == nil {
		t.Fatal("RetireGentleAIState() error = nil, want path escape failure")
	}
	if len(report.Removed) != 0 {
		t.Fatalf("RetireGentleAIState() report = %#v, want no removal", report)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != content {
		t.Fatalf("outside-home instructions changed: %q, %v", got, readErr)
	}
}

func TestRemoveMarkedBlocksRejectsSymlinkReplacementBeforeOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instructions.md")
	target := filepath.Join(dir, "user-owned.md")
	owned := gentleAITriggerRulesStart + "\nowned\n" + gentleAITriggerRulesEnd + "\n"
	userOwned := "keep user-owned target\n"
	if err := os.WriteFile(path, []byte(owned), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(userOwned), 0o600); err != nil {
		t.Fatal(err)
	}

	ops := systemMarkedBlockFileOps()
	systemLstat := ops.lstat
	replaced := false
	ops.lstat = func(name string) (os.FileInfo, error) {
		info, err := systemLstat(name)
		if err != nil || replaced {
			return info, err
		}
		replaced = true
		if err := os.Remove(name); err != nil {
			t.Fatalf("remove checked instructions: %v", err)
		}
		if err := os.Symlink(target, name); err != nil {
			t.Fatalf("replace instructions with symlink: %v", err)
		}
		return info, nil
	}

	removed, manual, err := removeMarkedBlocksWithFileOps(
		path,
		ops,
		textblock.Markers{Start: gentleAITriggerRulesStart, End: gentleAITriggerRulesEnd},
	)
	if err == nil {
		t.Fatal("removeMarkedBlocksWithFileOps() error = nil, want replacement failure")
	}
	if removed || manual {
		t.Fatalf("removeMarkedBlocksWithFileOps() = (%v, %v, %v), want no reported mutation", removed, manual, err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != userOwned {
		t.Fatalf("replacement symlink target changed: %q, %v", got, readErr)
	}
}

func TestRemoveMarkedBlocksRejectsSymlinkReplacementAfterOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instructions.md")
	checked := filepath.Join(dir, "checked-instructions.md")
	target := filepath.Join(dir, "user-owned.md")
	owned := gentleAITriggerRulesStart + "\nowned\n" + gentleAITriggerRulesEnd + "\n"
	userOwned := "keep user-owned target\n"
	if err := os.WriteFile(path, []byte(owned), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(userOwned), 0o600); err != nil {
		t.Fatal(err)
	}

	ops := systemMarkedBlockFileOps()
	systemOpenFile := ops.openFile
	ops.openFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		file, err := systemOpenFile(name, flag, perm)
		if err != nil {
			return nil, err
		}
		if err := os.Rename(name, checked); err != nil {
			file.Close()
			t.Fatalf("move checked instructions: %v", err)
		}
		if err := os.Symlink(target, name); err != nil {
			file.Close()
			t.Fatalf("replace instructions with symlink: %v", err)
		}
		return file, nil
	}

	removed, manual, err := removeMarkedBlocksWithFileOps(
		path,
		ops,
		textblock.Markers{Start: gentleAITriggerRulesStart, End: gentleAITriggerRulesEnd},
	)
	if err == nil {
		t.Fatal("removeMarkedBlocksWithFileOps() error = nil, want replacement failure")
	}
	if removed || manual {
		t.Fatalf("removeMarkedBlocksWithFileOps() = (%v, %v, %v), want no reported mutation", removed, manual, err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != userOwned {
		t.Fatalf("replacement symlink target changed: %q, %v", got, readErr)
	}
	got, readErr = os.ReadFile(checked)
	if readErr != nil || string(got) != owned {
		t.Fatalf("checked instructions changed: %q, %v", got, readErr)
	}
}

func TestRemoveMarkedBlocksRejectsSymlinkReplacementBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instructions.md")
	checked := filepath.Join(dir, "checked-instructions.md")
	target := filepath.Join(dir, "user-owned.md")
	owned := gentleAITriggerRulesStart + "\nowned\n" + gentleAITriggerRulesEnd + "\n"
	userOwned := "keep user-owned target\n"
	if err := os.WriteFile(path, []byte(owned), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(userOwned), 0o600); err != nil {
		t.Fatal(err)
	}

	ops := systemMarkedBlockFileOps()
	ops.beforeWrite = func() {
		if err := os.Rename(path, checked); err != nil {
			t.Fatalf("move checked instructions: %v", err)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatalf("replace instructions with symlink: %v", err)
		}
	}

	removed, manual, err := removeMarkedBlocksWithFileOps(
		path,
		ops,
		textblock.Markers{Start: gentleAITriggerRulesStart, End: gentleAITriggerRulesEnd},
	)
	if err == nil {
		t.Fatal("removeMarkedBlocksWithFileOps() error = nil, want replacement failure")
	}
	if removed || manual {
		t.Fatalf("removeMarkedBlocksWithFileOps() = (%v, %v, %v), want no reported mutation", removed, manual, err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != userOwned {
		t.Fatalf("replacement symlink target changed: %q, %v", got, readErr)
	}
	if info, statErr := os.Lstat(path); statErr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("replacement symlink changed: %v, %v", info, statErr)
	}
}

func TestRemoveMarkedBlocksRejectsDeletionAfterOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "instructions.md")
	owned := gentleAITriggerRulesStart + "\nowned\n" + gentleAITriggerRulesEnd + "\n"
	if err := os.WriteFile(path, []byte(owned), 0o640); err != nil {
		t.Fatal(err)
	}

	ops := systemMarkedBlockFileOps()
	systemOpenFile := ops.openFile
	ops.openFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		file, err := systemOpenFile(name, flag, perm)
		if err != nil {
			return nil, err
		}
		if err := os.Remove(name); err != nil {
			file.Close()
			t.Fatalf("delete checked instructions: %v", err)
		}
		return file, nil
	}

	removed, manual, err := removeMarkedBlocksWithFileOps(
		path,
		ops,
		textblock.Markers{Start: gentleAITriggerRulesStart, End: gentleAITriggerRulesEnd},
	)
	if err == nil {
		t.Fatal("removeMarkedBlocksWithFileOps() error = nil, want deletion failure")
	}
	if removed || manual {
		t.Fatalf("removeMarkedBlocksWithFileOps() = (%v, %v, %v), want no reported mutation", removed, manual, err)
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("deleted instructions recreated: %v", statErr)
	}
}

func TestRetireGentleAIStateDoesNotRewriteUnownedInstructions(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "AGENTS.md")
	content := "user-owned instructions\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o440); err != nil {
		t.Fatal(err)
	}
	modified := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}

	report, err := RetireGentleAIState(home)
	if err != nil {
		t.Fatalf("RetireGentleAIState() error = %v", err)
	}
	if len(report.Removed) != 0 || len(report.ManualCleanup) != 0 {
		t.Fatalf("RetireGentleAIState() report = %#v, want no change", report)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != content {
		t.Fatalf("unowned instructions changed: %q, %v", got, readErr)
	}
	info, statErr := os.Stat(path)
	if statErr != nil || info.Mode().Perm() != 0o440 || !info.ModTime().Equal(modified) {
		t.Fatalf("unowned instructions metadata = (%v, %v), want mode 0440 and mtime %v", info, statErr, modified)
	}
}

func TestRetireGentleAIStateReportsPartialFailureWithoutChangingMalformedFile(t *testing.T) {
	home := t.TempDir()
	codexPath := filepath.Join(home, ".codex", "AGENTS.md")
	claudePath := filepath.Join(home, ".claude", "CLAUDE.md")
	for _, path := range []string{codexPath, claudePath} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	malformed := "before\n" + gentleAITriggerRulesStart + "\nunclosed\n"
	if err := os.WriteFile(codexPath, []byte(malformed), 0o600); err != nil {
		t.Fatal(err)
	}
	owned := gentleAIPersonaStart + "\nowned\n" + gentleAIPersonaEnd + "\nuser\n"
	if err := os.WriteFile(claudePath, []byte(owned), 0o640); err != nil {
		t.Fatal(err)
	}

	report, err := RetireGentleAIState(home)
	if err == nil {
		t.Fatal("RetireGentleAIState() error = nil, want malformed marker failure")
	}
	if !reflect.DeepEqual(report.Removed, []string{"~/.claude/CLAUDE.md Gentle AI blocks"}) {
		t.Fatalf("RetireGentleAIState() report = %#v", report)
	}
	got, readErr := os.ReadFile(codexPath)
	if readErr != nil || string(got) != malformed {
		t.Fatalf("malformed instructions changed: %q, %v", got, readErr)
	}
	got, readErr = os.ReadFile(claudePath)
	if readErr != nil || string(got) != "\nuser\n" {
		t.Fatalf("independent valid target was not retired: %q, %v", got, readErr)
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

func TestDeliveryWorkflowInvokesMutationSafetyGate(t *testing.T) {
	root := filepath.Join("..", "..")
	workflowPath := filepath.Join(root, "workflows", "delivery-issue.md")
	workflow := readContractDocument(t, workflowPath)

	resume := strings.Index(workflow, "## Resume and isolation")
	gate := strings.Index(workflow, "## Mutation safety gate")
	implementation := strings.Index(workflow, "## Implementation and local gates")
	if resume < 0 || gate < 0 || implementation < 0 || !(resume < gate && gate < implementation) {
		t.Fatalf("mutation safety gate order is resume=%d gate=%d implementation=%d", resume, gate, implementation)
	}

	normalized := strings.Join(strings.Fields(workflow), " ")
	for _, want := range []string{
		"[mutation-safety-gate.md](mutation-safety-gate.md)",
		"managed filesystem mutation",
		"persisted metadata or receipts",
		"recovery or rollback",
		"authority or identity that may change concurrently",
		"read the reference completely",
		"Before implementation begins",
		"changes the approved mutation model",
		"repeat the gate before further implementation",
		"final independent review remains required",
		"mutation-safety result or its `not applicable` evidence",
	} {
		if !strings.Contains(strings.ToLower(normalized), strings.ToLower(want)) {
			t.Errorf("delivery workflow missing %q", want)
		}
	}
}

func TestMutationSafetyGateContract(t *testing.T) {
	root := filepath.Join("..", "..")
	path := filepath.Join(root, "workflows", "mutation-safety-gate.md")

	assertDocumentContainsAllNormalized(t, path, []string{
		"Compatibility map",
		"Transaction boundary",
		"Threat and failure matrix",
		"Fault seams",
		"Acceptance evidence",
		"Independent design challenge",
		"validated authority",
		"captured inputs",
		"durable evidence",
		"Every affected public or persisted behavior",
		"preserved or explicitly authorized to change",
		"Every applicable threat or failure",
		"mitigation, planned test, or specific `not applicable` reason",
		"zero actionable findings",
		"does not replace final Spec and Standards review",
	})

	scenarios := readMarkdownScenarioMatrix(t, path)
	wants := map[string][2]string{
		"required-mutation":      {"filesystem, persisted evidence, recovery, rollback, or concurrent authority", "Complete the safety case before implementation"},
		"not-applicable":         {"Documentation, skill, CI, or metadata-only change with no mutation boundary", "Record `not applicable` with direct evidence"},
		"unauthorized-decision":  {"Material product or architecture decision", "Return `needs-triage`"},
		"missing-capability":     {"Required independent design capability is unavailable", "Return `blocked`"},
		"mutation-model-changed": {"Implementation changes the approved mutation model", "Invalidate the result and repeat the gate before further implementation"},
	}
	if len(scenarios) != len(wants) {
		t.Fatalf("mutation safety scenario count = %d, want %d: %#v", len(scenarios), len(wants), scenarios)
	}
	for name, want := range wants {
		got, ok := scenarios[name]
		if !ok {
			t.Errorf("mutation safety scenario %q is missing", name)
			continue
		}
		if !strings.Contains(got[0], want[0]) {
			t.Errorf("mutation safety scenario %q evidence = %q, want %q", name, got[0], want[0])
		}
		if !strings.Contains(got[1], want[1]) {
			t.Errorf("mutation safety scenario %q result = %q, want %q", name, got[1], want[1])
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

func assertDocumentContainsAllNormalized(t *testing.T, path string, wants []string) {
	t.Helper()
	content := strings.Join(strings.Fields(readContractDocument(t, path)), " ")
	for _, want := range wants {
		if !strings.Contains(strings.ToLower(content), strings.ToLower(want)) {
			t.Errorf("document %s missing %q", path, want)
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
		if _, exists := scenarios[name]; exists {
			t.Fatalf("duplicate scenario %q in %s", name, path)
		}
		scenarios[name] = [2]string{strings.TrimSpace(cells[2]), strings.TrimSpace(cells[3])}
	}
	return scenarios
}
