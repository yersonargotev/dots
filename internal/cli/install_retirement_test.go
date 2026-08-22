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
	"github.com/yersonargotev/dots/internal/state"
)

func TestInstallExplicitReductionRemovesOwnedWholeTarget(t *testing.T) {
	fixture := newWholeTargetRetirementFixture(t)
	backupSentinel := filepath.Join(fixture.stateRoot, "backups", "preserved")
	if err := os.MkdirAll(filepath.Dir(backupSentinel), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupSentinel, []byte("backup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	userState := filepath.Join(fixture.home, ".local", "share", "app", "history")
	if err := os.MkdirAll(filepath.Dir(userState), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userState, []byte("user\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := fixture.reduce(t, "--profile", "keep")

	if _, err := os.Lstat(filepath.Join(fixture.home, ".old")); !os.IsNotExist(err) {
		t.Fatalf("retired owned target still exists: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(fixture.home, ".keep")); err != nil || string(got) != "keep\n" {
		t.Fatalf("selected target = %q, %v", got, err)
	}
	for path, want := range map[string]string{backupSentinel: "backup\n", userState: "user\n"} {
		if got, err := os.ReadFile(path); err != nil || string(got) != want {
			t.Fatalf("preserved state %s = %q, %v; want %q", path, got, err, want)
		}
	}
	meta, err := state.Load(state.Path(fixture.stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Entries) != 1 || meta.Entries[0].Target != filepath.Join(fixture.home, ".keep") {
		t.Fatalf("entries = %#v, want only selected target", meta.Entries)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"keep"}) {
		t.Fatalf("Installed Selection = %#v, want keep", meta.InstalledSelection)
	}
	result := parseSelectionRetirementResult(t, out)
	if !reflect.DeepEqual(result.Removed, []string{filepath.Join(fixture.home, ".old")}) || len(result.Retained) != 0 {
		t.Fatalf("JSON result does not distinguish removed retirement:\n%s", out)
	}
}

func TestInstallExplicitReductionPreservesDriftAndReleasesOwnership(t *testing.T) {
	fixture := newWholeTargetRetirementFixture(t)
	retired := filepath.Join(fixture.home, ".old")
	if err := os.WriteFile(retired, []byte("external\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var planOut, planErr bytes.Buffer
	planCode := cli.Run([]string{
		"plan", "--output", "json", "--profile", "keep",
		"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.sourceRoot, "--state-root", fixture.stateRoot,
	}, &planOut, &planErr)
	if planCode != cli.ExitFindings || !strings.Contains(planOut.String(), `"status": "findings"`) {
		t.Fatalf("Drifted reduction plan = code %d\nstdout:\n%s\nstderr:\n%s", planCode, planOut.String(), planErr.String())
	}

	out := fixture.reduce(t, "--profile", "keep")

	if got, err := os.ReadFile(retired); err != nil || string(got) != "external\n" {
		t.Fatalf("Drifted target = %q, %v; want preserved external bytes", got, err)
	}
	meta, err := state.Load(state.Path(fixture.stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.FindByTarget(retired); ok {
		t.Fatalf("Drifted retired target remains dots-owned: %#v", meta.Entries)
	}
	result := parseSelectionRetirementResult(t, out)
	if len(result.Removed) != 0 || !reflect.DeepEqual(result.Retained, []string{retired}) {
		t.Fatalf("JSON result claims the preserved target was deleted:\n%s", out)
	}
}

func TestInstallExplicitReductionPreservesNoLongerOwnedTarget(t *testing.T) {
	fixture := newWholeTargetRetirementFixture(t)
	retired := filepath.Join(fixture.home, ".old")
	if err := os.Remove(retired); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(retired, 0o700); err != nil {
		t.Fatal(err)
	}

	out := fixture.reduce(t, "--profile", "keep")

	if info, err := os.Stat(retired); err != nil || !info.IsDir() {
		t.Fatalf("no-longer-owned target = %#v, %v; want preserved directory", info, err)
	}
	meta, err := state.Load(state.Path(fixture.stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.FindByTarget(retired); ok {
		t.Fatalf("no-longer-owned target remains dots-owned: %#v", meta.Entries)
	}
	result := parseSelectionRetirementResult(t, out)
	if len(result.Removed) != 0 || !reflect.DeepEqual(result.Retained, []string{retired}) {
		t.Fatalf("JSON result claims no-longer-owned target was deleted:\n%s", out)
	}
}

func TestInstallMixedSelectionChangeAddsBeforeRetiringWholeTarget(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/old", "old\n")
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  previous:
    tags: [old]
  current:
    tags: [new]
entries:
  - source: configs/old
    target: ~/.old
    strategy: copy
    tags: [old]
  - source: configs/new
    target: ~/.new
    strategy: copy
    tags: [new]
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "previous")

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "current",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("mixed selection change = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".old")); !os.IsNotExist(err) {
		t.Fatalf("mixed change retained retired target: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(home, ".new")); err != nil || string(got) != "new\n" {
		t.Fatalf("mixed change new target = %q, %v", got, err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if len(meta.Entries) != 1 || meta.Entries[0].Target != filepath.Join(home, ".new") || meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"current"}) {
		t.Fatalf("mixed change metadata = %#v", meta)
	}
}

func TestInstallClearSelectionRequiresExplicitAuthorityAndRecordsEmptyIntent(t *testing.T) {
	fixture := newWholeTargetRetirementFixture(t)

	var rejectedOut, rejectedErr bytes.Buffer
	code := cli.Run([]string{
		"install", "--clear-selection", "--yes", "--skip-deps", "--output", "json",
		"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.sourceRoot, "--state-root", fixture.stateRoot,
	}, &rejectedOut, &rejectedErr)
	if code != cli.ExitError || !strings.Contains(rejectedOut.String(), "selection-change-acknowledgement-required") {
		t.Fatalf("clear without reduction acknowledgement = code %d\nstdout:\n%s\nstderr:\n%s", code, rejectedOut.String(), rejectedErr.String())
	}
	for _, name := range []string{".old", ".keep"} {
		if _, err := os.Lstat(filepath.Join(fixture.home, name)); err != nil {
			t.Fatalf("rejected clear changed %s: %v", name, err)
		}
	}

	out := fixture.reduce(t, "--clear-selection")
	for _, name := range []string{".old", ".keep"} {
		if _, err := os.Lstat(filepath.Join(fixture.home, name)); !os.IsNotExist(err) {
			t.Fatalf("clear selection retained owned target %s: %v", name, err)
		}
	}
	meta, err := state.Load(state.Path(fixture.stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || len(meta.InstalledSelection.Profiles) != 0 || len(meta.InstalledSelection.ExtraTags) != 0 || len(meta.InstalledSelection.ResolvedTags) != 0 {
		t.Fatalf("Installed Selection = %#v, want valid empty selection", meta.InstalledSelection)
	}
	if len(meta.Entries) != 0 || !strings.Contains(out, `"profiles": []`) || !strings.Contains(out, `"effective_tags": []`) {
		t.Fatalf("clear result/state is incomplete: entries=%#v\n%s", meta.Entries, out)
	}
}

func TestInstallClearSelectionDryRunRequiresNonInteractiveAcknowledgement(t *testing.T) {
	fixture := newWholeTargetRetirementFixture(t)
	args := []string{
		"install", "--clear-selection", "--dry-run", "--skip-deps", "--output", "json",
		"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.sourceRoot, "--state-root", fixture.stateRoot,
	}
	var rejectedOut, rejectedErr bytes.Buffer
	code := cli.Run(args, &rejectedOut, &rejectedErr)
	if code != cli.ExitError || !strings.Contains(rejectedOut.String(), "selection-change-acknowledgement-required") {
		t.Fatalf("clear dry-run without acknowledgement = code %d\nstdout:\n%s\nstderr:\n%s", code, rejectedOut.String(), rejectedErr.String())
	}

	args = append(args, "--yes", "--acknowledge-selection-change")
	var acceptedOut, acceptedErr bytes.Buffer
	code = cli.Run(args, &acceptedOut, &acceptedErr)
	if code != cli.ExitOK || !strings.Contains(acceptedOut.String(), `"dry_run": true`) {
		t.Fatalf("acknowledged clear dry-run = code %d\nstdout:\n%s\nstderr:\n%s", code, acceptedOut.String(), acceptedErr.String())
	}
	for _, name := range []string{".old", ".keep"} {
		if _, err := os.Lstat(filepath.Join(fixture.home, name)); err != nil {
			t.Fatalf("clear dry-run changed %s: %v", name, err)
		}
	}
}

func TestInstallExplicitReductionDoesNotRetireManifestEvolutionOnlyEntry(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/old", "old\n")
	writeCLISource(t, sourceRoot, "configs/keep", "keep\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  all:
    tags: [old, keep]
  keep:
    tags: [keep]
entries:
  - source: configs/old
    target: ~/.old
    strategy: copy
    tags: [old]
  - source: configs/keep
    target: ~/.keep
    strategy: copy
    tags: [keep]
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "all")

	manifestPath = writeCLIManifest(t, home, `version: 1
profiles:
  all:
    tags: [keep]
  keep:
    tags: [keep]
entries:
  - source: configs/keep
    target: ~/.keep
    strategy: copy
    tags: [keep]
`)
	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "keep",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("mixed manifest evolution/reduction = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	oldTarget := filepath.Join(home, ".old")
	if got, err := os.ReadFile(oldTarget); err != nil || string(got) != "old\n" {
		t.Fatalf("manifest-evolution target = %q, %v; want preserved", got, err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.FindByTarget(oldTarget); !ok {
		t.Fatal("explicit reduction released manifest-evolution-only ownership")
	}
	if strings.Contains(out.String(), `"selection_retirement"`) {
		t.Fatalf("manifest evolution leaked into selection retirement result:\n%s", out.String())
	}
}

func TestInstallClearSelectionInteractiveRequiresLiteralClear(t *testing.T) {
	for _, test := range []struct {
		name    string
		input   string
		cleared bool
	}{
		{name: "ordinary yes declines", input: "yes\n"},
		{name: "literal clear accepts", input: "clear\n", cleared: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWholeTargetRetirementFixture(t)
			cmd := cli.NewRootCommand()
			var out bytes.Buffer
			cmd.SetIn(strings.NewReader(test.input))
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{
				"install", "--clear-selection", "--skip-deps", "--no-tui",
				"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.sourceRoot, "--state-root", fixture.stateRoot,
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\n%s", err, out.String())
			}
			if !strings.Contains(out.String(), "Type clear to remove every selected Managed Entry") {
				t.Fatalf("clear confirmation prompt missing:\n%s", out.String())
			}
			for _, name := range []string{".old", ".keep"} {
				_, err := os.Lstat(filepath.Join(fixture.home, name))
				if test.cleared && !os.IsNotExist(err) {
					t.Fatalf("accepted clear retained %s: %v", name, err)
				}
				if !test.cleared && err != nil {
					t.Fatalf("declined clear changed %s: %v", name, err)
				}
			}
			meta, err := state.Load(state.Path(fixture.stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if test.cleared {
				if meta.InstalledSelection == nil || len(meta.InstalledSelection.ResolvedTags) != 0 {
					t.Fatalf("accepted clear selection = %#v, want empty", meta.InstalledSelection)
				}
			} else if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"all"}) {
				t.Fatalf("declined clear selection = %#v, want previous", meta.InstalledSelection)
			}
		})
	}
}

func TestInstallClearSelectionIsMutuallyExclusiveWithSelectionFlags(t *testing.T) {
	fixture := newWholeTargetRetirementFixture(t)
	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--clear-selection", "--profile", "keep", "--yes", "--acknowledge-selection-change", "--skip-deps",
		"--file", fixture.manifestPath, "--home", fixture.home, "--source-root", fixture.sourceRoot, "--state-root", fixture.stateRoot,
	}, &out, &errOut)
	if code != cli.ExitError || !strings.Contains(errOut.String(), "clear-selection") {
		t.Fatalf("mixed clear/selection flags = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	for _, name := range []string{".old", ".keep"} {
		if _, err := os.Lstat(filepath.Join(fixture.home, name)); err != nil {
			t.Fatalf("flag rejection changed %s: %v", name, err)
		}
	}
}

func TestInstallRetiresEntireProvenSubsetOwnedTarget(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/shared.json", "{\"owned\":true}\n")
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  old:
    tags: [shared]
  next:
    tags: [new]
entries:
  - source: configs/shared.json
    target: ~/.shared.json
    strategy: copy
    ownership: json-subset
    tags: [shared]
  - source: configs/new
    target: ~/.new
    strategy: copy
    tags: [new]
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "old")

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "next",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("entire subset target retirement = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if got, err := os.ReadFile(filepath.Join(home, ".new")); err != nil || string(got) != "new\n" {
		t.Fatalf("forward addition = %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".shared.json")); !os.IsNotExist(err) {
		t.Fatalf("entire proven subset target remains: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"next"}) {
		t.Fatalf("terminal selection = %#v, want next", meta.InstalledSelection)
	}
}

func TestInstallRetainsDriftedEntireSubsetTargetAndReleasesOwnership(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/shared.json", "{\"owned\":true}\n")
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  old:
    tags: [shared]
  next:
    tags: [new]
entries:
  - source: configs/shared.json
    target: ~/.shared.json
    strategy: copy
    ownership: json-subset
    tags: [shared]
  - source: configs/new
    target: ~/.new
    strategy: copy
    tags: [new]
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "old")
	target := filepath.Join(home, ".shared.json")
	drifted := []byte("{\"owned\":false,\"external\":true}\n")
	if err := os.WriteFile(target, drifted, 0o600); err != nil {
		t.Fatal(err)
	}

	var planOut, planErr bytes.Buffer
	planCode := cli.Run([]string{
		"plan", "--output", "json", "--profile", "next",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &planOut, &planErr)
	if planCode != cli.ExitFindings || !strings.Contains(planOut.String(), `"reason": "ambiguous-partial-ownership"`) {
		t.Fatalf("drifted subset plan = code %d\nstdout:\n%s\nstderr:\n%s", planCode, planOut.String(), planErr.String())
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "next",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("drifted subset retirement = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, drifted) {
		t.Fatalf("drifted subset target = %q, %v; want preserved bytes", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(home, ".new")); err != nil || string(got) != "new\n" {
		t.Fatalf("forward addition = %q, %v", got, err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.FindByTarget(target); ok {
		t.Fatalf("retained target ownership was not released: %#v", meta.Entries)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"next"}) {
		t.Fatalf("terminal selection = %#v, want next", meta.InstalledSelection)
	}
	result := parseSelectionRetirementResult(t, out.String())
	if !reflect.DeepEqual(result.Retained, []string{target}) || len(result.Removed) != 0 {
		t.Fatalf("JSON result does not report retained retirement:\n%s", out.String())
	}
}

func TestInstallReconcilesOpenCodeSharedJSONContribution(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/opencode/opencode.json", "{\"base\":true}\n")
	writeCLISource(t, sourceRoot, "configs/opencode/mcp.json", "{\"retired\":true}\n")
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  old:
    tags: [opencode, opencode-chrome-devtools]
  next:
    tags: [opencode, new]
entries:
  - source: configs/opencode/opencode.json
    target: ~/.config/opencode/opencode.json
    strategy: copy
    ownership: json-subset
    tags: [opencode]
  - source: configs/opencode/mcp.json
    target: ~/.config/opencode/opencode.json
    strategy: copy
    ownership: json-subset
    tags: [opencode-chrome-devtools]
  - source: configs/new
    target: ~/.new
    strategy: copy
    tags: [new]
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "old")
	target := filepath.Join(home, ".config", "opencode", "opencode.json")
	if err := os.WriteFile(target, []byte("{\"base\":true,\"retired\":true,\"external\":\"preserve\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "next",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("still-selected subset retirement = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if got, err := os.ReadFile(filepath.Join(home, ".new")); err != nil || string(got) != "new\n" {
		t.Fatalf("forward addition = %q, %v; want new content", got, err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]any
	if err := json.Unmarshal(got, &content); err != nil {
		t.Fatalf("decode reconciled shared target: %v\n%s", err, got)
	}
	if content["base"] != true || content["external"] != "preserve" {
		t.Fatalf("reconciled shared target = %#v, want selected and external content", content)
	}
	if _, retired := content["retired"]; retired {
		t.Fatalf("reconciled shared target retained retired contribution: %#v", content)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"next"}) {
		t.Fatalf("reconciled retirement selection = %#v, want next", meta.InstalledSelection)
	}
	record, ok := meta.FindByTarget(target)
	if !ok || len(record.Contributions) != 1 || record.Contributions[0].Source != "configs/opencode/opencode.json" || !reflect.DeepEqual(record.Contributions[0].SelectorTags, []string{"opencode"}) {
		t.Fatalf("reconciled contribution evidence = %#v, want selected base only", record)
	}
	if !strings.Contains(out.String(), `"outcome": "reconcile"`) || !strings.Contains(out.String(), `"previous_sources": [`) {
		t.Fatalf("JSON result does not explain contribution reconciliation:\n%s", out.String())
	}
}

func TestInstallReconcilesAntigravitySharedJSONContribution(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/antigravity/settings.json", "{\"base\":true}\n")
	writeCLISource(t, sourceRoot, "configs/antigravity/mobile-mcp-settings.json", "{\"retired\":true}\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  old:
    tags: [antigravity, antigravity-dart-mcp]
  next:
    tags: [antigravity]
entries:
  - source: configs/antigravity/settings.json
    target: ~/.gemini/antigravity-cli/settings.json
    strategy: copy
    ownership: json-subset
    tags: [antigravity]
  - source: configs/antigravity/mobile-mcp-settings.json
    target: ~/.gemini/antigravity-cli/settings.json
    strategy: copy
    ownership: json-subset
    tags: [antigravity-dart-mcp]
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "old")
	target := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
	if err := os.WriteFile(target, []byte("{\"base\":true,\"retired\":true,\"external\":\"preserve\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "next",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("Antigravity shared contribution retirement = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]any
	if err := json.Unmarshal(got, &content); err != nil {
		t.Fatalf("decode reconciled Antigravity target: %v\n%s", err, got)
	}
	if content["base"] != true || content["external"] != "preserve" {
		t.Fatalf("reconciled Antigravity target = %#v, want selected and external content", content)
	}
	if _, retired := content["retired"]; retired {
		t.Fatalf("reconciled Antigravity target retained retired contribution: %#v", content)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	record, ok := meta.FindByTarget(target)
	if !ok || len(record.Contributions) != 1 || record.Contributions[0].Source != "configs/antigravity/settings.json" || !reflect.DeepEqual(record.Contributions[0].SelectorTags, []string{"antigravity"}) {
		t.Fatalf("reconciled Antigravity contribution evidence = %#v, want selected base only", record)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"next"}) || !strings.Contains(out.String(), `"outcome": "reconcile"`) {
		t.Fatalf("Antigravity reconciliation result is incomplete: selection=%#v\n%s", meta.InstalledSelection, out.String())
	}
}

func TestInstallReconcilesStructuredSourceOverridesToSelectedBase(t *testing.T) {
	block := func(body string) string {
		return "# >>> dots managed block >>>\n" + body + "\n# <<< dots managed block <<<\n"
	}
	tests := []struct {
		name        string
		ownership   string
		base        string
		override    string
		live        string
		want        func(*testing.T, []byte)
		wantOutcome string
	}{
		{
			name:        "Herdr adaptive TOML",
			ownership:   "toml-subset",
			base:        "# managed base\nbase = true\n",
			override:    "# managed adaptive\nadaptive = true\n",
			live:        "# external sentinel\nexternal = \"keep\"\n# managed adaptive\nadaptive = true\n",
			wantOutcome: "reconcile",
			want: func(t *testing.T, got []byte) {
				t.Helper()
				if !bytes.HasPrefix(got, []byte("# external sentinel\nexternal = \"keep\"\n")) || !bytes.Contains(got, []byte("base = true")) || bytes.Contains(got, []byte("adaptive = true")) {
					t.Fatalf("reconciled TOML did not preserve external bytes and selected base:\n%s", got)
				}
			},
		},
		{
			name:        "Codex codegraph TOML",
			ownership:   "toml-subset",
			base:        "model = \"base\"\n",
			override:    "model = \"base\"\n[hooks]\ncodegraph = true\n",
			live:        "# external sentinel\ntui = \"alternate\"\nmodel = \"base\"\n[hooks]\ncodegraph = true\n",
			wantOutcome: "reconcile",
			want: func(t *testing.T, got []byte) {
				t.Helper()
				if !bytes.HasPrefix(got, []byte("# external sentinel\ntui = \"alternate\"\n")) || !bytes.Contains(got, []byte("model = \"base\"")) || bytes.Contains(got, []byte("codegraph")) {
					t.Fatalf("reconciled Codex TOML did not preserve external bytes and remove CodeGraph:\n%s", got)
				}
			},
		},
		{
			name:        "JSONC",
			ownership:   "jsonc-subset",
			base:        "{\n  // managed base\n  \"base\": true,\n}\n",
			override:    "{\n  // managed override\n  \"override\": true,\n}\n",
			live:        "{\n  // managed override\n  \"override\": true,\n  // external sentinel\n  \"external\": true,\n}\n",
			wantOutcome: "reconcile",
			want: func(t *testing.T, got []byte) {
				t.Helper()
				if !bytes.Contains(got, []byte("  // external sentinel\n  \"external\": true,")) || !bytes.Contains(got, []byte("\"base\": true")) || bytes.Contains(got, []byte("\"override\"")) {
					t.Fatalf("reconciled JSONC did not preserve external bytes and selected base:\n%s", got)
				}
			},
		},
		{
			name:        "marked block",
			ownership:   "marked-block",
			base:        block("base=1"),
			override:    block("override=1"),
			live:        block("override=1") + "# external sentinel\nexternal=keep\n",
			wantOutcome: "reconcile",
			want: func(t *testing.T, got []byte) {
				t.Helper()
				want := block("base=1") + "# external sentinel\nexternal=keep\n"
				if string(got) != want {
					t.Fatalf("reconciled marked block = %q, want exact %q", got, want)
				}
			},
		},
		{
			name:        "Zellij adaptive whole target",
			ownership:   "",
			base:        "theme \"base\"\n",
			override:    "theme \"adaptive\"\n",
			live:        "theme \"adaptive\"\n",
			wantOutcome: "update",
			want: func(t *testing.T, got []byte) {
				t.Helper()
				if string(got) != "theme \"base\"\n" {
					t.Fatalf("reconciled whole target = %q, want selected base", got)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			sourceRoot := t.TempDir()
			stateRoot := t.TempDir()
			t.Setenv("HOME", t.TempDir())
			writeCLISource(t, sourceRoot, "configs/base", test.base)
			writeCLISource(t, sourceRoot, "configs/override", test.override)
			ownership := ""
			if test.ownership != "" {
				ownership = "\n    ownership: " + test.ownership
			}
			manifestPath := writeCLIManifest(t, home, fmt.Sprintf(`version: 1
profiles:
  previous:
    tags: [consumer, opt-in]
  current:
    tags: [consumer]
entries:
  - source: configs/base
    source_overrides:
      opt-in: configs/override
    target: ~/.config/app/settings
    strategy: copy%s
    tags: [consumer]
`, ownership))
			runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "previous")
			target := filepath.Join(home, ".config", "app", "settings")
			if err := os.WriteFile(target, []byte(test.live), 0o600); err != nil {
				t.Fatal(err)
			}

			var out, errOut bytes.Buffer
			code := cli.Run([]string{
				"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "current",
				"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
			}, &out, &errOut)
			if code != cli.ExitOK {
				t.Fatalf("structured source override retirement = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			test.want(t, got)
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			record, ok := meta.FindByTarget(target)
			if !ok || len(record.Contributions) != 1 || record.Contributions[0].Source != "configs/base" || !reflect.DeepEqual(record.Contributions[0].SelectorTags, []string{"consumer"}) {
				t.Fatalf("reconciled contribution evidence = %#v, want selected base only", record)
			}
			if !strings.Contains(out.String(), `"outcome": "`+test.wantOutcome+`"`) || !strings.Contains(out.String(), `"previous_sources": [`) {
				t.Fatalf("JSON result does not explain structured reconciliation:\n%s", out.String())
			}
		})
	}
}

func TestInstallBlocksAmbiguousSharedContributionBeforeAnyMutation(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/base.json", "{\"base\":true}\n")
	writeCLISource(t, sourceRoot, "configs/retired.json", "{\"retired\":\"owned\"}\n")
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  old:
    tags: [base, retired]
  next:
    tags: [base, new]
entries:
  - source: configs/base.json
    target: ~/.shared.json
    strategy: copy
    ownership: json-subset
    tags: [base]
  - source: configs/retired.json
    target: ~/.shared.json
    strategy: copy
    ownership: json-subset
    tags: [retired]
  - source: configs/new
    target: ~/.new
    strategy: copy
    tags: [new]
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "old")
	target := filepath.Join(home, ".shared.json")
	drifted := []byte("{\"base\":true,\"retired\":\"changed\",\"external\":\"preserve\"}\n")
	if err := os.WriteFile(target, drifted, 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "next",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitError || !strings.Contains(out.String(), "ambiguous-partial-ownership") {
		t.Fatalf("ambiguous shared retirement = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".new")); !os.IsNotExist(err) {
		t.Fatalf("blocked reconciliation applied forward addition: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, drifted) {
		t.Fatalf("blocked reconciliation changed target: %q, %v", got, err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"old"}) {
		t.Fatalf("blocked reconciliation selection = %#v, want previous", meta.InstalledSelection)
	}
	if record, ok := meta.FindByTarget(target); !ok || len(record.Contributions) != 2 {
		t.Fatalf("blocked reconciliation changed contribution evidence: %#v", record)
	}
}

func TestInstallSharedReconciliationFailurePreservesIntentAndRerunsSafely(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/base.json", "{\"base\":true}\n")
	writeCLISource(t, sourceRoot, "configs/retired.json", "{\"retired\":true}\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  old:
    tags: [base, retired]
  next:
    tags: [base, provision]
entries:
  - source: configs/base.json
    target: ~/.shared.json
    strategy: copy
    ownership: json-subset
    tags: [base]
  - source: configs/retired.json
    target: ~/.shared.json
    strategy: copy
    ownership: json-subset
    tags: [retired]
provisioners:
  - tool: claude
    tags: [provision]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "old")
	target := filepath.Join(home, ".shared.json")
	binDir := t.TempDir()
	claude := filepath.Join(binDir, "claude")
	writeExecStub(t, claude, "#!/bin/sh\nexit 17\n")
	t.Setenv("PATH", binDir)

	run := func() (int, string, string) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := cli.Run([]string{
			"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "next",
			"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
		}, &out, &errOut)
		return code, out.String(), errOut.String()
	}

	code, out, errOut := run()
	if code != cli.ExitError {
		t.Fatalf("failing reconciliation run = code %d\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var content map[string]any
	if err := json.Unmarshal(got, &content); err != nil {
		t.Fatal(err)
	}
	if content["base"] != true {
		t.Fatalf("failed run did not apply safe forward reconciliation: %#v", content)
	}
	if _, retired := content["retired"]; retired {
		t.Fatalf("failed run retained retired contribution in live target: %#v", content)
	}
	failedMeta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	failedRecord, ok := failedMeta.FindByTarget(target)
	if !ok || len(failedRecord.Contributions) != 2 {
		t.Fatalf("failed run replaced previous contribution evidence: %#v", failedRecord)
	}
	if failedMeta.InstalledSelection == nil || !reflect.DeepEqual(failedMeta.InstalledSelection.Profiles, []string{"old"}) {
		t.Fatalf("failed run committed requested selection: %#v", failedMeta.InstalledSelection)
	}

	writeExecStub(t, claude, "#!/bin/sh\nexit 0\n")
	code, out, errOut = run()
	if code != cli.ExitOK {
		t.Fatalf("rerun reconciliation = code %d\nstdout:\n%s\nstderr:\n%s", code, out, errOut)
	}
	finalMeta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	finalRecord, ok := finalMeta.FindByTarget(target)
	if !ok || len(finalRecord.Contributions) != 1 || finalRecord.Contributions[0].Source != "configs/base.json" {
		t.Fatalf("rerun contribution evidence = %#v, want selected base only", finalRecord)
	}
	if finalMeta.InstalledSelection == nil || !reflect.DeepEqual(finalMeta.InstalledSelection.Profiles, []string{"next"}) {
		t.Fatalf("rerun Installed Selection = %#v, want next", finalMeta.InstalledSelection)
	}
}

func TestInstallReleasesSeededRuntimeStateWithoutRemovingItsBytes(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, sourceRoot, "configs/seeded", "baseline\n")
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  old:
    tags: [seeded]
  next:
    tags: [new]
entries:
  - source: configs/seeded
    target: ~/.config/app/runtime
    strategy: copy
    ownership: seeded
    tags: [seeded]
  - source: configs/new
    target: ~/.new
    strategy: copy
    tags: [new]
`)
	runInstallForRetirementTest(t, manifestPath, home, sourceRoot, stateRoot, "old")
	target := filepath.Join(home, ".config", "app", "runtime")
	evolved := []byte("application-owned evolution\n")
	if err := os.WriteFile(target, evolved, 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "next",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("seeded retirement = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, evolved) {
		t.Fatalf("Seeded Runtime State = %q, %v; want exact evolved bytes", got, err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := meta.FindByTarget(target); ok {
		t.Fatalf("Seeded Runtime State remains dots-owned: %#v", meta.Entries)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"next"}) || !strings.Contains(out.String(), `"retained": [`) {
		t.Fatalf("seeded retirement result is incomplete: selection=%#v\n%s", meta.InstalledSelection, out.String())
	}
}

type wholeTargetRetirementFixture struct {
	home         string
	sourceRoot   string
	stateRoot    string
	manifestPath string
}

func newWholeTargetRetirementFixture(t *testing.T) wholeTargetRetirementFixture {
	t.Helper()
	fixture := wholeTargetRetirementFixture{
		home:       t.TempDir(),
		sourceRoot: t.TempDir(),
		stateRoot:  t.TempDir(),
	}
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, fixture.sourceRoot, "configs/old", "old\n")
	writeCLISource(t, fixture.sourceRoot, "configs/keep", "keep\n")
	fixture.manifestPath = writeCLIManifest(t, fixture.home, `version: 1
profiles:
  all:
    tags: [old, keep]
  keep:
    tags: [keep]
entries:
  - source: configs/old
    target: ~/.old
    strategy: copy
    tags: [old]
  - source: configs/keep
    target: ~/.keep
    strategy: copy
    tags: [keep]
`)

	runInstallForRetirementTest(t, fixture.manifestPath, fixture.home, fixture.sourceRoot, fixture.stateRoot, "all")
	return fixture
}

func runInstallForRetirementTest(t *testing.T, manifestPath, home, sourceRoot, stateRoot, profile string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--skip-deps", "--output", "json", "--profile", profile,
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("initial install = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
}

func (f wholeTargetRetirementFixture) reduce(t *testing.T, selectionArgs ...string) string {
	t.Helper()
	args := []string{"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json"}
	args = append(args, selectionArgs...)
	args = append(args, "--file", f.manifestPath, "--home", f.home, "--source-root", f.sourceRoot, "--state-root", f.stateRoot)
	var out, errOut bytes.Buffer
	code := cli.Run(args, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("selection reduction = code %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	return out.String()
}

func parseSelectionRetirementResult(t *testing.T, output string) struct {
	Removed  []string `json:"removed"`
	Retained []string `json:"retained"`
} {
	t.Helper()
	var envelope struct {
		Data struct {
			SelectionRetirement struct {
				Removed  []string `json:"removed"`
				Retained []string `json:"retained"`
			} `json:"selection_retirement"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(output), &envelope); err != nil {
		t.Fatalf("decode install envelope: %v\n%s", err, output)
	}
	return envelope.Data.SelectionRetirement
}
