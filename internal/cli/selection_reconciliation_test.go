package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/state"
)

func TestSelectionReconciliationPlanAndInstallDryRunJSONParity(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, false)
	targetBefore := mustReadSelectionFile(t, fixture.retiredTarget)
	metadataBefore := mustReadSelectionFile(t, state.Path(fixture.stateRoot))

	plan := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"plan"}, fixture.args...)...)
	install := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"install", "--dry-run", "--skip-deps"}, fixture.args...)...)

	planReconciliation := jsonPathSelection(t, plan, "data", "selection_reconciliation")
	installReconciliation := jsonPathSelection(t, install, "data", "plan", "selection_reconciliation")
	planActions := jsonPathSelection(t, planReconciliation, "actions")
	installActions := jsonPathSelection(t, installReconciliation, "actions")
	if !reflect.DeepEqual(planActions, installActions) {
		t.Fatalf("reconciliation actions differ\nplan: %#v\ninstall --dry-run: %#v", planActions, installActions)
	}
	assertSelectionAction(t, planActions, "selection", "remove", "retired")
	assertSelectionAction(t, planActions, "managed-entry", "remove", fixture.retiredTarget)
	if got := mustReadSelectionFile(t, fixture.retiredTarget); !bytes.Equal(got, targetBefore) {
		t.Fatalf("planning mutated target: got %q, want %q", got, targetBefore)
	}
	if got := mustReadSelectionFile(t, state.Path(fixture.stateRoot)); !bytes.Equal(got, metadataBefore) {
		t.Fatalf("planning mutated Installation Metadata\ngot: %s\nwant: %s", got, metadataBefore)
	}
}

func TestSelectionReconciliationTextSurfacesUseIdenticalActionOrder(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, false)
	plan := runSelectionReconciliationText(t, cli.ExitOK, append([]string{"plan"}, fixture.args...)...)
	install := runSelectionReconciliationText(t, cli.ExitOK, append([]string{"install", "--dry-run", "--skip-deps"}, fixture.args...)...)

	planLines := reconciliationActionLines(plan)
	installLines := reconciliationActionLines(install)
	if len(planLines) == 0 {
		t.Fatalf("plan text has no deterministic reconciliation action lines:\n%s", plan)
	}
	want := []string{
		fmt.Sprintf("  %-24s %-14s %s", "remove", "selection", "full, retired"),
		fmt.Sprintf("  %-24s %-14s %s", "create", "selection", "reduced"),
		fmt.Sprintf("  %-24s %-14s %s", "create", "managed-entry", filepath.Join(fixture.root, "home", ".config", "kept.conf")),
		fmt.Sprintf("  %-24s %-14s %s", "remove", "managed-entry", fixture.retiredTarget),
	}
	if !reflect.DeepEqual(planLines, want) {
		t.Fatalf("plan reconciliation lines = %#v, want %#v\noutput:\n%s", planLines, want, plan)
	}
	if !reflect.DeepEqual(planLines, installLines) {
		t.Fatalf("reconciliation text action order differs\nplan: %#v\ninstall --dry-run: %#v\n\nplan output:\n%s\ninstall output:\n%s", planLines, installLines, plan, install)
	}
}

func TestSelectionReconciliationRetainsDependencyAndProvisionerExternalState(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, true)
	plan := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"plan"}, fixture.args...)...)
	actions := jsonPathSelection(t, plan, "data", "selection_reconciliation", "actions")

	assertSelectionAction(t, actions, "dependency", "retained-external-state", "retired-tool")
	assertSelectionAction(t, actions, "provisioner", "retained-external-state", "claude")
	for _, raw := range actions.([]any) {
		action := raw.(map[string]any)
		if action["scope"] == "managed-entry" && action["outcome"] == "remove" {
			t.Fatalf("external-only reduction reported Managed Entry removal: %#v", action)
		}
	}
}

func TestInstallReductionLeavesDependencyMetadataAndProvisionerEffectsUntouched(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, true)
	metadataPath := state.Path(fixture.stateRoot)
	metadata, err := state.Load(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	provisioner := state.ProvisionerRecord{
		Profiles: []string{"full"}, Tags: []string{"retired"}, Tool: "claude", Executable: "claude",
		Args: []string{"plugin", "marketplace", "add", "example/tools"}, Status: "provisioned",
	}
	metadata.Provisioners = []state.ProvisionerRecord{provisioner}
	if err := state.Save(metadataPath, metadata); err != nil {
		t.Fatal(err)
	}
	dependencyMetadata := deps.DependencyMetadata{Dependencies: []deps.DependencyRecord{{
		Dependency: "retired-tool", Provider: "user-local", Path: filepath.Join(fixture.root, "bin", "retired-tool"), InstalledAt: "2026-08-22T00:00:00Z",
	}}}
	dependencyPath := deps.DependencyMetadataPath(fixture.stateRoot)
	if err := deps.SaveDependencyMetadata(dependencyPath, dependencyMetadata); err != nil {
		t.Fatal(err)
	}
	dependencyBefore := mustReadSelectionFile(t, dependencyPath)
	externalEffect := filepath.Join(fixture.root, "provisioner-effect")
	if err := os.WriteFile(externalEffect, []byte("external\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	output := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"install", "--yes", "--acknowledge-selection-change", "--skip-deps"}, fixture.args...)...)
	actions := jsonPathSelection(t, output, "data", "plan", "selection_reconciliation", "actions")
	assertSelectionAction(t, actions, "dependency", "retained-external-state", "retired-tool")
	assertSelectionAction(t, actions, "provisioner", "retained-external-state", "claude")

	if got := mustReadSelectionFile(t, dependencyPath); !bytes.Equal(got, dependencyBefore) {
		t.Fatalf("Dependency Installation Metadata changed:\nbefore: %s\nafter: %s", dependencyBefore, got)
	}
	if got, err := os.ReadFile(externalEffect); err != nil || string(got) != "external\n" {
		t.Fatalf("Provisioner effect = %q, %v; want untouched", got, err)
	}
	gotMetadata, err := state.Load(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotMetadata.Provisioners, []state.ProvisionerRecord{provisioner}) {
		t.Fatalf("Provisioner metadata = %#v, want preserved receipt", gotMetadata.Provisioners)
	}
}

func TestSelectionReconciliationRetainsProvisionerRemovedFromManifest(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, true)
	manifestPath := fixture.args[3]
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	provisioners := bytes.Index(manifest, []byte("provisioners:\n"))
	if provisioners < 0 {
		t.Fatal("fixture manifest has no provisioners block")
	}
	manifest = append(manifest[:provisioners], []byte(`provisioners:
  - tool: claude
    tags: [kept]
    spec:
      marketplace: retained/tools
`)...)
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	metadataPath := state.Path(fixture.stateRoot)
	metadata, err := state.Load(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata.Provisioners = []state.ProvisionerRecord{
		{
			Profiles: []string{"full"}, Tags: []string{"retired"}, Tool: "claude", Executable: "claude",
			Args: []string{"plugin", "marketplace", "add", "removed/tools"}, Status: "provisioned",
		},
		{
			Profiles: []string{"full"}, Tags: []string{"kept"}, Tool: "claude", Executable: "claude",
			Args: []string{"plugin", "marketplace", "add", "retained/tools"}, Status: "provisioned",
		},
	}
	if err := state.Save(metadataPath, metadata); err != nil {
		t.Fatal(err)
	}

	plan := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"plan"}, fixture.args...)...)
	install := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"install", "--dry-run", "--skip-deps"}, fixture.args...)...)
	planActions := jsonPathSelection(t, plan, "data", "selection_reconciliation", "actions")
	installActions := jsonPathSelection(t, install, "data", "plan", "selection_reconciliation", "actions")
	assertSelectionAction(t, planActions, "provisioner", "retained-external-state", "claude")
	provisionerRetirements := 0
	provisionerIdentity := ""
	for _, raw := range planActions.([]any) {
		action := raw.(map[string]any)
		if action["scope"] == "provisioner" && action["outcome"] == "retained-external-state" {
			provisionerRetirements++
			provisionerIdentity = action["identity"].(string)
			if !strings.HasPrefix(provisionerIdentity, "sha256:") {
				t.Fatalf("Provisioner identity = %#v, want non-sensitive digest", action["identity"])
			}
		}
	}
	if provisionerRetirements != 1 {
		t.Fatalf("Provisioner retirements = %d, want 1 in %#v", provisionerRetirements, planActions)
	}
	if !reflect.DeepEqual(planActions, installActions) {
		t.Fatalf("manifest evolution actions differ\nplan: %#v\ninstall --dry-run: %#v", planActions, installActions)
	}
	planText := runSelectionReconciliationText(t, cli.ExitOK, append([]string{"plan"}, fixture.args...)...)
	installText := runSelectionReconciliationText(t, cli.ExitOK, append([]string{"install", "--dry-run", "--skip-deps"}, fixture.args...)...)
	planLines := reconciliationActionLines(planText)
	installLines := reconciliationActionLines(installText)
	if !reflect.DeepEqual(planLines, installLines) {
		t.Fatalf("manifest evolution text actions differ\nplan: %#v\ninstall --dry-run: %#v", planLines, installLines)
	}
	if !strings.Contains(strings.Join(planLines, "\n"), "claude ["+provisionerIdentity+"]") {
		t.Fatalf("text output omits Provisioner identity %q:\n%s", provisionerIdentity, planText)
	}
}

func TestSelectionReconciliationManifestEvolutionNeverAuthorizesRetirement(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, false)
	manifestPath := fixture.args[3]
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest = bytes.Replace(manifest, []byte("  full:\n    tags: [kept, retired]"), []byte("  full:\n    tags: [kept]"), 1)
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}

	plan := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"plan"}, fixture.args[2:]...)...)
	actions := jsonPathSelection(t, plan, "data", "selection_reconciliation", "actions")
	assertSelectionAction(t, actions, "managed-entry", "retain", "manifest-evolution-report-only")
	for _, raw := range actions.([]any) {
		action := raw.(map[string]any)
		if action["scope"] == "managed-entry" && action["outcome"] == "remove" {
			t.Fatalf("manifest evolution authorized removal: %#v", action)
		}
	}
}

func TestSelectionReconciliationExplicitUnchangedIntentDoesNotAuthorizeManifestEvolution(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, false)
	manifestPath := fixture.args[3]
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest = bytes.Replace(manifest, []byte("  full:\n    tags: [kept, retired]"), []byte("  full:\n    tags: [kept]"), 1)
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}

	args := append([]string(nil), fixture.args...)
	args[1] = "full"
	plan := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"plan"}, args...)...)
	actions := jsonPathSelection(t, plan, "data", "selection_reconciliation", "actions")
	assertSelectionAction(t, actions, "managed-entry", "retain", "manifest-evolution-report-only")
	for _, raw := range actions.([]any) {
		action := raw.(map[string]any)
		if action["scope"] == "managed-entry" && action["outcome"] == "remove" {
			t.Fatalf("unchanged explicit intent authorized manifest retirement: %#v", action)
		}
	}
}

func TestSelectionReconciliationReportsManifestRemovedRecordedEntry(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, false)
	manifestPath := fixture.args[3]
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest = bytes.Replace(manifest, []byte(`  - source: configs/retired.conf
    target: ~/.config/retired.conf
    strategy: copy
    tags: [retired]
`), nil, 1)
	if err := os.WriteFile(manifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}

	plan := runSelectionReconciliationJSON(t, cli.ExitOK, append([]string{"plan"}, fixture.args[2:]...)...)
	actions := jsonPathSelection(t, plan, "data", "selection_reconciliation", "actions")
	assertSelectionAction(t, actions, "managed-entry", "retain", fixture.retiredTarget)
	assertSelectionAction(t, actions, "managed-entry", "retain", "manifest-evolution-report-only")
}

func TestSelectionReconciliationParentSymlinkEscapeIsBlockedWithoutReadingTarget(t *testing.T) {
	fixture := newSelectionReconciliationFixture(t, false)
	external := filepath.Join(fixture.root, "external")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	const secret = "must-not-be-read-across-parent-symlink"
	if err := os.WriteFile(filepath.Join(external, "retired.conf"), []byte(secret), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "kept.conf"), []byte(secret), 0o000); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fixture.retiredTarget); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(fixture.retiredTarget)
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, parent); err != nil {
		t.Fatal(err)
	}

	planOut := runSelectionReconciliationJSONBytes(t, cli.ExitFindings, append([]string{"plan"}, fixture.args...)...)
	installOut := runSelectionReconciliationJSONBytes(t, cli.ExitOK, append([]string{"install", "--dry-run", "--skip-deps"}, fixture.args...)...)
	for name, output := range map[string][]byte{"plan": planOut, "install --dry-run": installOut} {
		if bytes.Contains(output, []byte(secret)) {
			t.Fatalf("%s exposed content read through parent symlink: %s", name, output)
		}
		var envelope map[string]any
		if err := json.Unmarshal(output, &envelope); err != nil {
			t.Fatalf("decode %s JSON: %v\n%s", name, err, output)
		}
		actions := envelope["data"].(map[string]any)
		if name == "install --dry-run" {
			actions = actions["plan"].(map[string]any)
		}
		assertSelectionAction(t, actions["selection_reconciliation"].(map[string]any)["actions"], "managed-entry", "blocked", "lost-ownership")
	}
}

type selectionReconciliationFixture struct {
	root, stateRoot, retiredTarget string
	args                           []string
}

func newSelectionReconciliationFixture(t *testing.T, externalOnly bool) selectionReconciliationFixture {
	t.Helper()
	root := t.TempDir()
	home := filepath.Join(root, "home")
	sourceRoot := filepath.Join(root, "source")
	stateRoot := filepath.Join(root, "state")
	for _, dir := range []string{home, sourceRoot, stateRoot, filepath.Join(home, ".config")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", filepath.Join(root, "unused-real-home"))

	retiredTarget := filepath.Join(home, ".config", "retired.conf")
	retiredContent := []byte("retired managed content\n")
	writeSelectionSource(t, sourceRoot, "configs/kept.conf", "kept managed content\n")
	if !externalOnly {
		writeSelectionSource(t, sourceRoot, "configs/retired.conf", string(retiredContent))
		if err := os.WriteFile(retiredTarget, retiredContent, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	manifest := `version: 1
tags:
  kept:
    description: retained surface
    kind: surface
    status: current
  retired:
    description: retired surface
    kind: surface
    status: current
profiles:
  full:
    tags: [kept, retired]
  reduced:
    tags: [kept]
`
	if externalOnly {
		manifest += `entries:
  - source: configs/kept.conf
    target: ~/.config/kept.conf
    strategy: copy
    tags: [kept]
dependencies:
  - tags: [retired]
    dependencies:
      - name: retired-tool
provisioners:
  - tool: claude
    tags: [retired]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`
	} else {
		manifest += `entries:
  - source: configs/kept.conf
    target: ~/.config/kept.conf
    strategy: copy
    tags: [kept]
  - source: configs/retired.conf
    target: ~/.config/retired.conf
    strategy: copy
    tags: [retired]
`
	}
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	metadata := state.Metadata{
		Version: state.CurrentVersion,
		InstalledSelection: &state.InstalledSelection{
			Profiles: []string{"full"}, ResolvedTags: []string{"kept", "retired"},
			Provenance: state.Provenance{SourceRoot: sourceRoot, RecordedAt: "2026-08-22T00:00:00Z"},
		},
	}
	if !externalOnly {
		metadata.Entries = []state.Record{{
			Target: retiredTarget, Source: "configs/retired.conf", Strategy: "copy", Ownership: "whole",
			Hash: state.HashBytes(retiredContent), InstalledAt: "2026-08-22T00:00:00Z", Tags: []string{"retired"},
			Contributions: []state.Contribution{{
				Source: "configs/retired.conf", SelectorTags: []string{"retired"}, Ownership: "whole",
				EvidenceRecorded: true, Hash: state.HashBytes(retiredContent),
			}},
		}}
	}
	if err := state.Save(state.Path(stateRoot), metadata); err != nil {
		t.Fatal(err)
	}
	return selectionReconciliationFixture{
		root: root, stateRoot: stateRoot, retiredTarget: retiredTarget,
		args: []string{"--profile", "reduced", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot},
	}
}

func runSelectionReconciliationJSON(t *testing.T, want int, args ...string) map[string]any {
	t.Helper()
	var envelope map[string]any
	out := runSelectionReconciliationJSONBytes(t, want, append(args, "--output", "json")...)
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out)
	}
	if envelope["schema_version"] != "10" {
		t.Fatalf("schema_version = %#v, want 10", envelope["schema_version"])
	}
	return envelope
}

func runSelectionReconciliationJSONBytes(t *testing.T, want int, args ...string) []byte {
	t.Helper()
	if !containsSelectionArg(args, "--output") {
		args = append(args, "--output", "json")
	}
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	if code != want {
		t.Fatalf("cli.Run(%v) code = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, want, stdout.String(), stderr.String())
	}
	return stdout.Bytes()
}

func runSelectionReconciliationText(t *testing.T, want int, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	if code != want {
		t.Fatalf("cli.Run(%v) code = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, want, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func jsonPathSelection(t *testing.T, value any, path ...string) any {
	t.Helper()
	current := value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("JSON path %v: %#v is not an object", path, current)
		}
		current, ok = object[key]
		if !ok {
			t.Fatalf("JSON path %v: missing %q in %#v", path, key, object)
		}
	}
	return current
}

func assertSelectionAction(t *testing.T, raw any, scope, outcome, evidence string) {
	t.Helper()
	for _, item := range raw.([]any) {
		action := item.(map[string]any)
		encoded, _ := json.Marshal(action)
		if action["scope"] == scope && action["outcome"] == outcome && strings.Contains(string(encoded), evidence) {
			return
		}
	}
	t.Fatalf("missing %s/%s action containing %q in %#v", scope, outcome, evidence, raw)
}

func reconciliationActionLines(output string) []string {
	var lines []string
	inBlock := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Selection reconciliation:" {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		if trimmed == "" {
			break
		}
		lines = append(lines, line)
	}
	return lines
}

func writeSelectionSource(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustReadSelectionFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func containsSelectionArg(args []string, needle string) bool {
	for _, arg := range args {
		if arg == needle {
			return true
		}
	}
	return false
}
