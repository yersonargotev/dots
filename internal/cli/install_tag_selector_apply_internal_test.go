package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selectionreconciliation"
	"github.com/yersonargotev/dots/internal/selectionretirement"
	"github.com/yersonargotev/dots/internal/state"
	tagselectortui "github.com/yersonargotev/dots/internal/tui/tagselector"
)

func TestInstallTagSelectorProfilePresetPersistsExplicitCurrentTags(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, browse tagselectortui.BrowseData, initial []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		if len(initial) != 0 {
			t.Fatalf("initial Tags = %#v, want empty", initial)
		}
		var preset []string
		for _, profile := range browse.Profiles {
			if profile.Name == "default" {
				preset = append([]string(nil), profile.Tags...)
				break
			}
		}
		if want := []string{"one"}; !reflect.DeepEqual(preset, want) {
			t.Fatalf("default Profile preset = %#v, want %#v", preset, want)
		}
		built, err := preview(1, preset)
		return tagselectortui.Result{Tags: preset, Preview: built}, err
	})

	out, err := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	got := loadSelectorSelection(t, stateRoot)
	if len(got.Profiles) != 0 || !reflect.DeepEqual(got.ExtraTags, []string{"one"}) || !reflect.DeepEqual(got.ResolvedTags, []string{"one"}) {
		t.Fatalf("Installed Selection = %#v, want Profile flattened to explicit current Tag one", got)
	}
}

func TestInstallTagSelectorConflictResolutionCancelPreservesInstalledSelection(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "one")
	previous := *loadSelectorSelection(t, stateRoot)
	target := filepath.Join(home, ".one")
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("operator-owned\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	setInstallTagSelectorTestHooks(t, true, acceptedSelectorTags("one", "two"))
	out, err := executeAcceptedSelector(t, "q", manifestPath, sourceRoot, home, stateRoot)
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Conflict resolution canceled; no changes applied.") {
		t.Fatalf("output missing shared Conflict Resolution cancellation\n%s", out)
	}
	if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "operator-owned\n" {
		t.Fatalf("conflict target = %q, %v; want operator content", got, readErr)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".two")); !os.IsNotExist(statErr) {
		t.Fatalf("Conflict Resolution cancellation applied later Managed Entry: %v", statErr)
	}
	if got := loadSelectorSelection(t, stateRoot); !reflect.DeepEqual(*got, previous) {
		t.Fatalf("Installed Selection = %#v, want previous %#v", got, previous)
	}
}

func TestInstallTagSelectorApplyFailurePreservesInstalledSelection(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "one")
	previous := *loadSelectorSelection(t, stateRoot)
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, []string{"one", "two"})
		if err != nil {
			return tagselectortui.Result{}, err
		}
		replacement := filepath.Join(sourceRoot, "config", "two-replacement")
		if err := os.WriteFile(replacement, []byte("stale\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(replacement, filepath.Join(sourceRoot, "config", "two")); err != nil {
			t.Fatal(err)
		}
		return tagselectortui.Result{Tags: []string{"one", "two"}, Preview: built}, nil
	})

	out, err := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if err == nil || !strings.Contains(err.Error(), "changed identity") {
		t.Fatalf("Execute() error = %v, want stale apply authority failure\n%s", err, out)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".two")); !os.IsNotExist(statErr) {
		t.Fatalf("failed apply created .two: %v", statErr)
	}
	if got := loadSelectorSelection(t, stateRoot); !reflect.DeepEqual(*got, previous) {
		t.Fatalf("Installed Selection = %#v, want previous %#v", got, previous)
	}
}

func TestInstallTagSelectorTerminalCASPreservesConcurrentInstalledSelection(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorProvisionerFixture(t)
	runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "one")
	concurrentMetadata, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	concurrent := &state.InstalledSelection{
		Profiles: []string{}, ExtraTags: []string{"two"}, ResolvedTags: []string{"two"},
		Provenance: state.Provenance{SourceRoot: sourceRoot, DotsVersion: "concurrent-writer"},
	}
	concurrentMetadata.InstalledSelection = concurrent
	concurrentPath := filepath.Join(t.TempDir(), "concurrent.json")
	if err := state.Save(concurrentPath, concurrentMetadata); err != nil {
		t.Fatal(err)
	}
	stubDir := t.TempDir()
	writeSelectorExecutable(t, filepath.Join(stubDir, "claude"), fmt.Sprintf("#!/bin/sh\n[ \"$1\" = plugin ] || exit 0\ncp %q %q\n", concurrentPath, state.Path(stateRoot)))
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	setInstallTagSelectorTestHooks(t, true, acceptedSelectorTags("one", "two"))

	out, err := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if err == nil || !strings.Contains(err.Error(), "installed selection changed concurrently") {
		t.Fatalf("Execute() error = %v, want terminal CAS rejection\n%s", err, out)
	}
	if got := loadSelectorSelection(t, stateRoot); !reflect.DeepEqual(got, concurrent) {
		t.Fatalf("Installed Selection = %#v, want concurrent authority %#v", got, concurrent)
	}
}

func TestInstallTagSelectorPartialFailureRerunConverges(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorProvisionerFixture(t)
	runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "one")
	previous := *loadSelectorSelection(t, stateRoot)
	stubDir := t.TempDir()
	marker := filepath.Join(stubDir, "failed-once")
	writeSelectorExecutable(t, filepath.Join(stubDir, "claude"), fmt.Sprintf("#!/bin/sh\n[ \"$1\" = plugin ] || exit 0\nif [ ! -e %q ]; then : > %q; exit 17; fi\nexit 0\n", marker, marker))
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	setInstallTagSelectorTestHooks(t, true, acceptedSelectorTags("one", "two"))
	firstOut, firstErr := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if firstErr == nil || !strings.Contains(firstErr.Error(), `provisioner "claude" failed`) {
		t.Fatalf("first Execute() error = %v, want injected partial failure\n%s", firstErr, firstOut)
	}
	if got := loadSelectorSelection(t, stateRoot); !reflect.DeepEqual(*got, previous) {
		t.Fatalf("Installed Selection after partial failure = %#v, want previous %#v", got, previous)
	}

	setInstallTagSelectorTestHooks(t, true, acceptedSelectorTags("one", "two"))
	secondOut, secondErr := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if secondErr != nil {
		t.Fatalf("rerun Execute() error = %v\n%s", secondErr, secondOut)
	}
	if _, err := os.Lstat(filepath.Join(home, ".two")); err != nil {
		t.Fatalf("rerun did not converge Managed Entry: %v", err)
	}
	got := loadSelectorSelection(t, stateRoot)
	if !reflect.DeepEqual(got.ExtraTags, []string{"one", "two"}) || !reflect.DeepEqual(got.ResolvedTags, []string{"one", "two"}) {
		t.Fatalf("rerun Installed Selection = %#v, want explicit Tags one,two", got)
	}
}

func TestInstallTagSelectorMixedPreviewMatchesExplicitJSONSemanticsAndGolden(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorMixedFixture(t)
	stubDir := t.TempDir()
	writeSelectorExecutable(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "base", "retired")

	m, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := resolvePaths(home, sourceRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := buildInstallTagSelectorCandidate(*m, meta, paths, sourceRoot, nil, installHostOS, []string{"base", "next"}, installTagSelectorOptions{SkipDependencies: true})
	if err != nil {
		t.Fatalf("build selector candidate: %v", err)
	}
	t.Cleanup(func() {
		if err := candidate.releaseCapturedSources(); err != nil {
			t.Errorf("release selector candidate: %v", err)
		}
	})
	assertSelectorReconciliationAction(t, candidate.Plan.SelectionReconciliation, selectionreconciliation.ScopeSelection, selectionreconciliation.OutcomeRemove, "retired")
	assertSelectorReconciliationAction(t, candidate.Plan.SelectionReconciliation, selectionreconciliation.ScopeSelection, selectionreconciliation.OutcomeCreate, "next")
	assertSelectorReconciliationAction(t, candidate.Plan.SelectionReconciliation, selectionreconciliation.ScopeManagedEntry, selectionreconciliation.OutcomeReconcile, ".shared.json")
	assertSelectorReconciliationAction(t, candidate.Plan.SelectionReconciliation, selectionreconciliation.ScopeManagedEntry, selectionreconciliation.OutcomeRemove, ".retired")
	assertSelectorReconciliationAction(t, candidate.Plan.SelectionReconciliation, selectionreconciliation.ScopeManagedEntry, selectionreconciliation.OutcomeCreate, ".next")
	assertSelectorReconciliationAction(t, candidate.Plan.SelectionReconciliation, selectionreconciliation.ScopeDependency, selectionreconciliation.OutcomeRetainedExternalState, "retired-tool")
	assertSelectorReconciliationAction(t, candidate.Plan.SelectionReconciliation, selectionreconciliation.ScopeProvisioner, selectionreconciliation.OutcomeRetainedExternalState, "claude")

	cmd := NewRootCommand()
	var jsonOut bytes.Buffer
	cmd.SetOut(&jsonOut)
	cmd.SetErr(&jsonOut)
	cmd.SetArgs([]string{
		"--output", "json", "install", "--dry-run", "--skip-deps", "--tag", "base", "--tag", "next",
		"--file", manifestPath, "--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("explicit JSON dry-run error = %v\n%s", err, jsonOut.String())
	}
	var envelope struct {
		Data struct {
			Plan struct {
				SelectionReconciliation *selectionreconciliation.Report `json:"selection_reconciliation"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(jsonOut.Bytes(), &envelope); err != nil {
		t.Fatalf("decode explicit JSON: %v\n%s", err, jsonOut.String())
	}
	if !reflect.DeepEqual(envelope.Data.Plan.SelectionReconciliation, candidate.Plan.SelectionReconciliation) {
		t.Fatalf("selector and explicit JSON reconciliation differ\nselector: %#v\nJSON: %#v", candidate.Plan.SelectionReconciliation, envelope.Data.Plan.SelectionReconciliation)
	}

	goldenText := strings.ReplaceAll(candidate.Preview.Text, home, "/home/test")
	goldenText = strings.ReplaceAll(goldenText, sourceRoot, "/source")
	assertGolden(t, "install_tag_selector_mixed_preview.golden", []byte(goldenText))

	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, previewErr := preview(1, []string{"base", "next"})
		return tagselectortui.Result{
			Tags: []string{"base", "next"}, Preview: built, AcknowledgementAccepted: true,
		}, previewErr
	})
	resultText, err := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if err != nil {
		t.Fatalf("mixed selector apply error = %v\n%s", err, resultText)
	}
	for _, want := range []string{
		"Tag selection applied: tags=base,next",
		"Managed Entry results:",
		"reconciled",
		filepath.Join(home, ".shared.json"),
		"created",
		filepath.Join(home, ".next"),
		"Retained External State:",
		"dependency",
		"retired-tool",
		"provisioner",
		"claude",
		"Selection retirement:",
		filepath.Join(home, ".retired"),
	} {
		if !strings.Contains(resultText, want) {
			t.Fatalf("mixed selector result missing %q\n%s", want, resultText)
		}
	}
	var shared map[string]any
	sharedData, err := os.ReadFile(filepath.Join(home, ".shared.json"))
	if err != nil {
		t.Fatalf("read reconciled shared target: %v", err)
	}
	if err := json.Unmarshal(sharedData, &shared); err != nil {
		t.Fatalf("decode reconciled shared target: %v\n%s", err, sharedData)
	}
	if len(shared) != 1 || shared["base"] != true {
		t.Fatalf("reconciled shared target = %#v, want only selected base contribution", shared)
	}
	if _, err := os.Lstat(filepath.Join(home, ".retired")); !os.IsNotExist(err) {
		t.Fatalf("whole retired target still exists: %v", err)
	}
	if destination, err := os.Readlink(filepath.Join(home, ".next")); err != nil || destination != filepath.Join(sourceRoot, "config", "next") {
		t.Fatalf("added target destination = %q, %v", destination, err)
	}
	installed := loadSelectorSelection(t, stateRoot)
	if len(installed.Profiles) != 0 || !reflect.DeepEqual(installed.ExtraTags, []string{"base", "next"}) || !reflect.DeepEqual(installed.ResolvedTags, []string{"base", "next"}) {
		t.Fatalf("mixed Installed Selection = %#v, want explicit current Tags base,next", installed)
	}
	resultGolden := strings.ReplaceAll(resultText, home, "/home/test")
	resultGolden = strings.ReplaceAll(resultGolden, sourceRoot, "/source")
	assertGolden(t, "install_tag_selector_mixed_result.golden", []byte(resultGolden))
}

func TestRenderTagSelectorFinalResultReportsEffectiveConflictAndRetirementOutcomes(t *testing.T) {
	replaced := "/home/test/.replaced"
	adopted := "/home/test/.adopted"
	skipped := "/home/test/.skipped"
	drifted := "/home/test/.drifted"
	candidate := installTagSelectorCandidate{Plan: plan.Plan{
		Actions: []plan.Action{
			{Target: replaced, Status: plan.StatusConflict},
			{Target: adopted, Status: plan.StatusConflict},
			{Target: skipped, Status: plan.StatusConflict},
		},
		SelectionReconciliation: &selectionreconciliation.Report{Actions: []selectionreconciliation.Action{
			{Scope: selectionreconciliation.ScopeManagedEntry, Outcome: selectionreconciliation.OutcomeBlocked, ResolvedTarget: replaced},
			{Scope: selectionreconciliation.ScopeManagedEntry, Outcome: selectionreconciliation.OutcomeBlocked, ResolvedTarget: adopted},
			{Scope: selectionreconciliation.ScopeManagedEntry, Outcome: selectionreconciliation.OutcomeBlocked, ResolvedTarget: skipped},
			{Scope: selectionreconciliation.ScopeDependency, Outcome: selectionreconciliation.OutcomeRetainedExternalState, Names: []string{"old-tool"}},
		}},
	}}
	decisions := map[string]install.ConflictDecision{
		replaced: install.DecisionReplace,
		adopted:  install.DecisionAdopt,
		skipped:  install.DecisionSkip,
	}
	retirement := &selectionretirement.Result{Retained: []string{drifted}}

	var out bytes.Buffer
	renderTagSelectorFinalResult(&out, candidate, decisions, nil, provision.Report{}, retirement, 0)
	text := out.String()
	for _, want := range []string{
		"Managed Entry results:",
		"replaced   " + replaced,
		"adopted    " + adopted,
		"skipped    " + skipped,
		"Retained External State:",
		"dependency   old-tool",
		"retained " + drifted + " and released dots ownership",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("effective selector result missing %q\n%s", want, text)
		}
	}
	for _, contradicted := range []string{"Selection reconciliation result:", "blocked", "removed " + drifted} {
		if strings.Contains(text, contradicted) {
			t.Fatalf("effective selector result contains stale plan outcome %q\n%s", contradicted, text)
		}
	}
}

func TestInstallTagSelectorReductionFindingsGolden(t *testing.T) {
	var out bytes.Buffer
	renderPlan(&out, plan.Plan{
		Profile:  "tags only",
		Profiles: []string{},
		Tags:     []string{"kept"},
		Actions: []plan.Action{
			{Source: "config/kept", Target: "/home/test/.kept", Strategy: "symlink", Status: plan.StatusUnchanged},
		},
		SelectionReconciliation: &selectionreconciliation.Report{
			PreviousIntent:  selectionreconciliation.Intent{Authority: selectionreconciliation.AuthorityRecorded, ExtraTags: []string{"kept", "removed"}, ResolvedTags: []string{"kept", "removed"}},
			RequestedIntent: selectionreconciliation.Intent{Authority: selectionreconciliation.AuthorityExplicitRequest, ExtraTags: []string{"kept"}, ResolvedTags: []string{"kept"}},
			Actions: []selectionreconciliation.Action{
				{Scope: selectionreconciliation.ScopeSelection, Outcome: selectionreconciliation.OutcomeRemove, Names: []string{"removed"}, PreviousSources: []string{}, CurrentSources: []string{}},
				{Scope: selectionreconciliation.ScopeManagedEntry, Outcome: selectionreconciliation.OutcomeRemove, ResolvedTarget: "/home/test/.removed", PreviousSources: []string{"config/removed"}, CurrentSources: []string{}},
				{Scope: selectionreconciliation.ScopeManagedEntry, Outcome: selectionreconciliation.OutcomeRetain, Reason: selectionreconciliation.ReasonWholeTargetDrift, ResolvedTarget: "/home/test/.retained", PreviousSources: []string{"config/retained"}, CurrentSources: []string{}},
				{Scope: selectionreconciliation.ScopeManagedEntry, Outcome: selectionreconciliation.OutcomeBlocked, Reason: selectionreconciliation.ReasonAmbiguousPartialOwnership, ResolvedTarget: "/home/test/.shared.json", PreviousSources: []string{"config/base.json", "config/removed.json"}, CurrentSources: []string{"config/base.json"}},
				{Scope: selectionreconciliation.ScopeDependency, Outcome: selectionreconciliation.OutcomeRetainedExternalState, Names: []string{"removed-tool"}, PreviousSources: []string{}, CurrentSources: []string{}},
				{Scope: selectionreconciliation.ScopeProvisioner, Outcome: selectionreconciliation.OutcomeRetainedExternalState, Names: []string{"claude"}, Identity: "sha256:example", PreviousSources: []string{}, CurrentSources: []string{}},
			},
		},
	})
	assertGolden(t, "install_tag_selector_reduction_findings.golden", out.Bytes())
}

func TestInstallTagSelectorBlockedReductionPreviewSurfacesFinding(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorMixedFixture(t)
	stubDir := t.TempDir()
	writeSelectorExecutable(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "base", "retired")
	if err := os.WriteFile(filepath.Join(home, ".shared.json"), []byte("{\"base\":true,\"retired\":\"operator-change\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	paths, err := resolvePaths(home, sourceRoot, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := buildInstallTagSelectorCandidate(*m, meta, paths, sourceRoot, nil, installHostOS, []string{"base", "next"}, installTagSelectorOptions{SkipDependencies: true})
	if err != nil {
		t.Fatalf("blocked reduction preview error = %v, want a renderable non-mutating finding", err)
	}
	t.Cleanup(func() {
		if err := candidate.releaseCapturedSources(); err != nil {
			t.Errorf("release selector candidate: %v", err)
		}
	})
	for _, want := range []string{"blocked", selectionreconciliation.ReasonAmbiguousPartialOwnership, filepath.Join(home, ".shared.json")} {
		if !strings.Contains(candidate.Preview.Text, want) {
			t.Fatalf("blocked reduction preview missing %q\n%s", want, candidate.Preview.Text)
		}
	}
	previousSelection := *loadSelectorSelection(t, stateRoot)
	sharedBefore, err := os.ReadFile(filepath.Join(home, ".shared.json"))
	if err != nil {
		t.Fatal(err)
	}
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, previewErr := preview(1, []string{"base", "next"})
		return tagselectortui.Result{Tags: []string{"base", "next"}, Preview: built, AcknowledgementAccepted: true}, previewErr
	})
	out, err := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if err == nil || !strings.Contains(err.Error(), "accepted Tag selector reduction is blocked") || !strings.Contains(err.Error(), selectionreconciliation.ReasonAmbiguousPartialOwnership) {
		t.Fatalf("acknowledged blocked apply error = %v, want fail-closed finding\n%s", err, out)
	}
	if !strings.Contains(out, "blocked") || !strings.Contains(out, selectionreconciliation.ReasonAmbiguousPartialOwnership) {
		t.Fatalf("acknowledged blocked apply did not render finding\n%s", out)
	}
	if got := loadSelectorSelection(t, stateRoot); !reflect.DeepEqual(*got, previousSelection) {
		t.Fatalf("blocked apply Installed Selection = %#v, want previous %#v", got, previousSelection)
	}
	if got, readErr := os.ReadFile(filepath.Join(home, ".shared.json")); readErr != nil || !bytes.Equal(got, sharedBefore) {
		t.Fatalf("blocked apply shared target = %q, %v; want unchanged %q", got, readErr, sharedBefore)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".next")); !os.IsNotExist(statErr) {
		t.Fatalf("blocked apply created forward Managed Entry: %v", statErr)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".retired")); statErr != nil {
		t.Fatalf("blocked apply retired previous Managed Entry: %v", statErr)
	}
}

func acceptedSelectorTags(tags ...string) installTagSelectorRunnerFunc {
	selected := append([]string(nil), tags...)
	return func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, selected)
		return tagselectortui.Result{Tags: append([]string(nil), selected...), Preview: built}, err
	}
}

func executeAcceptedSelector(t *testing.T, input, manifestPath, sourceRoot, home, stateRoot string) (string, error) {
	t.Helper()
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--skip-deps", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	err := cmd.Execute()
	return out.String(), err
}

func loadSelectorSelection(t *testing.T, stateRoot string) *state.InstalledSelection {
	t.Helper()
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil {
		t.Fatal("Installed Selection = nil")
	}
	return meta.InstalledSelection
}

func writeTagSelectorProvisionerFixture(t *testing.T) (manifestPath, sourceRoot, home, stateRoot string) {
	t.Helper()
	home = t.TempDir()
	sourceRoot = t.TempDir()
	stateRoot = t.TempDir()
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Join(sourceRoot, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(sourceRoot, "config", name), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath = filepath.Join(sourceRoot, "dots.yaml")
	manifest := `version: 1
tags:
  one:
    description: Existing surface.
    kind: surface
    status: current
  two:
    description: Added surface with a Provisioner.
    kind: surface
    status: current
profiles:
  default:
    tags: [one]
entries:
  - source: config/one
    target: ~/.one
    strategy: symlink
    tags: [one]
  - source: config/two
    target: ~/.two
    strategy: symlink
    tags: [two]
provisioners:
  - tool: claude
    tags: [two]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return manifestPath, sourceRoot, home, stateRoot
}

func writeTagSelectorMixedFixture(t *testing.T) (manifestPath, sourceRoot, home, stateRoot string) {
	t.Helper()
	home = t.TempDir()
	sourceRoot = t.TempDir()
	stateRoot = t.TempDir()
	t.Setenv("HOME", t.TempDir())
	for relative, content := range map[string]string{
		"config/base.json":    "{\"base\":true}\n",
		"config/retired.json": "{\"retired\":true}\n",
		"config/retired":      "retired\n",
		"config/next":         "next\n",
	} {
		path := filepath.Join(sourceRoot, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	manifestPath = filepath.Join(sourceRoot, "dots.yaml")
	data := `version: 1
tags:
  base:
    description: Shared retained contribution.
    kind: surface
    status: current
  retired:
    description: Retired contribution and whole target.
    kind: surface
    status: current
  next:
    description: New forward surface.
    kind: surface
    status: current
profiles:
  old:
    tags: [base, retired]
  next:
    tags: [base, next]
entries:
  - source: config/base.json
    target: ~/.shared.json
    strategy: copy
    ownership: json-subset
    tags: [base]
  - source: config/retired.json
    target: ~/.shared.json
    strategy: copy
    ownership: json-subset
    tags: [retired]
  - source: config/retired
    target: ~/.retired
    strategy: copy
    tags: [retired]
  - source: config/next
    target: ~/.next
    strategy: symlink
    tags: [next]
dependencies:
  - tags: [retired]
    dependencies:
      - name: retired-tool
        command: retired-tool
        manual: retained outside dots
provisioners:
  - tool: claude
    tags: [retired]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`
	if err := os.WriteFile(manifestPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return manifestPath, sourceRoot, home, stateRoot
}

func assertSelectorReconciliationAction(t *testing.T, report *selectionreconciliation.Report, scope selectionreconciliation.Scope, outcome selectionreconciliation.Outcome, evidence string) {
	t.Helper()
	if report == nil {
		t.Fatal("Selection Reconciliation Plan = nil")
	}
	for _, action := range report.Actions {
		encoded, _ := json.Marshal(action)
		if action.Scope == scope && action.Outcome == outcome && strings.Contains(string(encoded), evidence) {
			return
		}
	}
	t.Fatalf("missing %s/%s containing %q in %#v", scope, outcome, evidence, report.Actions)
}

func writeSelectorExecutable(t *testing.T, path, script string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
}
