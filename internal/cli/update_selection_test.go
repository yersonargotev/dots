package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
)

func TestUpdateReusesRecordedSelectionAndRefreshesResolvedSnapshot(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/core": "core\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core, extra]
`,
	})
	saveInstalledSelection(t, stateRoot, "core", "extra")
	advanceUpstream(t, origin, "expand core profile", map[string]string{
		"configs/new": "new\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core, new]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core, extra]
  - source: configs/new
    target: ~/.new
    strategy: symlink
    tags: [new]
`,
	})

	out := runBareUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)

	if want := "Selection: source=recorded profiles=core extra-tags=extra effective-tags=core,new,extra"; !strings.Contains(out, want) {
		t.Fatalf("output missing selection report %q:\n%s", want, out)
	}
	for _, want := range []string{
		"Selection evolution:",
		"Previous: profiles=core extra-tags=extra effective-tags=core,extra",
		"Current: profiles=core extra-tags=extra effective-tags=core,new,extra",
		"Added: effective-tags=new managed-entries=~/.new dependencies=(none) provisioners=(none)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing evolution report %q:\n%s", want, out)
		}
	}
	if _, err := os.Readlink(filepath.Join(home, ".new")); err != nil {
		t.Fatalf("recorded selection did not apply post-refresh Profile tags: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if got, want := meta.InstalledSelection.Profiles, []string{"core"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Profiles = %#v, want %#v", got, want)
	}
	if got, want := meta.InstalledSelection.ExtraTags, []string{"extra"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got, want)
	}
	if got, want := meta.InstalledSelection.ResolvedTags, []string{"core", "new", "extra"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolvedTags = %#v, want %#v", got, want)
	}
}

func TestUpdateExplicitSelectionCompletelyOverridesRecordedIntent(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/core":  "core\n",
		"configs/work":  "work\n",
		"configs/extra": "extra\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
  work:
    tags: [work]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
  - source: configs/work
    target: ~/.work
    strategy: symlink
    tags: [work]
  - source: configs/extra
    target: ~/.extra
    strategy: symlink
    tags: [extra]
`,
	})
	saveInstalledSelection(t, stateRoot, "core")

	out := runUpdate(t, "--yes", "--acknowledge-selection-change", "--profile", "work", "--tag", "extra",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)

	if want := "Selection: source=explicit profiles=work extra-tags=extra effective-tags=work,extra"; !strings.Contains(out, want) {
		t.Fatalf("output missing selection report %q:\n%s", want, out)
	}
	if want := "Acknowledgement: required=true accepted=true"; !strings.Contains(out, want) {
		t.Fatalf("output missing accepted selection change %q:\n%s", want, out)
	}
	if _, err := os.Lstat(filepath.Join(home, ".core")); !os.IsNotExist(err) {
		t.Fatalf("recorded core selection leaked into explicit update: %v", err)
	}
	for _, target := range []string{".work", ".extra"} {
		if _, err := os.Readlink(filepath.Join(home, target)); err != nil {
			t.Fatalf("explicit selection did not install %s: %v", target, err)
		}
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if got, want := meta.InstalledSelection.Profiles, []string{"work"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Profiles = %#v, want %#v", got, want)
	}
	if got, want := meta.InstalledSelection.ExtraTags, []string{"extra"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got, want)
	}
}

func TestUpdateYesSelectionReductionRequiresDedicatedAcknowledgement(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/core": "core\n",
		"configs/work": "work\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
  work:
    tags: [work]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
  - source: configs/work
    target: ~/.work
    strategy: symlink
    tags: [work]
`,
	})
	saveInstalledSelection(t, stateRoot, "core")

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"update", "--yes", "--output", "json", "--profile", "work",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitError, out.String(), errOut.String())
	}
	var env struct {
		Data struct {
			Code   string           `json:"code"`
			Change selection.Change `json:"selection_change"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal JSON: %v\n%s", err, out.String())
	}
	if env.Data.Code != "selection-change-acknowledgement-required" ||
		!env.Data.Change.AcknowledgementRequired ||
		env.Data.Change.AcknowledgementAccepted {
		t.Fatalf("selection change error data = %#v", env.Data)
	}
	if got, want := env.Data.Change.Delta.Removed.Profiles, []string{"core"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed Profiles = %#v, want %#v", got, want)
	}
	if _, err := os.Lstat(filepath.Join(home, ".work")); !os.IsNotExist(err) {
		t.Fatalf("selection rejection applied Managed Configuration: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if got, want := meta.InstalledSelection.Profiles, []string{"core"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Installed Selection Profiles = %#v, want %#v", got, want)
	}
}

func TestUpdateAdditiveSelectionChangeAppliesWithoutDedicatedAcknowledgement(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/core":  "core\n",
		"configs/extra": "extra\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
  - source: configs/extra
    target: ~/.extra
    strategy: symlink
    tags: [extra]
`,
	})
	saveInstalledSelection(t, stateRoot, "core")

	out := runUpdate(t,
		"--yes", "--profile", "core", "--tag", "extra",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	)
	for _, want := range []string{
		"Added: profiles=(none) extra-tags=extra effective-tags=extra",
		"Acknowledgement: required=false accepted=true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing additive change %q:\n%s", want, out)
		}
	}
	if _, err := os.Readlink(filepath.Join(home, ".extra")); err != nil {
		t.Fatalf("additive selection did not apply extra Managed Entry: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if got, want := meta.InstalledSelection.ExtraTags, []string{"extra"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Installed Selection extra Tags = %#v, want %#v", got, want)
	}
}

func TestUpdateInteractiveSelectionReductionCanBeDeclinedBeforeMutation(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/core": "core\n",
		"configs/work": "work\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
  work:
    tags: [work]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
  - source: configs/work
    target: ~/.work
    strategy: symlink
    tags: [work]
`,
	})
	saveInstalledSelection(t, stateRoot, "core")

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{
		"update", "--profile", "work", "--no-tui",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Installed Selection change declined; operation canceled before mutation.") {
		t.Fatalf("output missing decline confirmation:\n%s", out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".work")); !os.IsNotExist(err) {
		t.Fatalf("declined selection change applied Managed Configuration: %v", err)
	}
}

func TestUpdateInvalidRecordedSelectionAfterRefreshStopsApplicationAndPreservesIntent(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/core": "core\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core, extra]
`,
	})
	previous := state.InstalledSelection{Profiles: []string{"core"}, ResolvedTags: []string{"core"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	advanceUpstream(t, origin, "remove core profile", map[string]string{
		"configs/work": "work\n",
		"dots.yaml": `version: 1
profiles:
  work:
    tags: [work]
entries:
  - source: configs/work
    target: ~/.work
    strategy: symlink
    tags: [work]
`,
	})

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `recorded selection: profile "core" not found`) {
		t.Fatalf("error = %v, want invalid refreshed selection\noutput:\n%s", err, out.String())
	}
	for _, want := range []string{
		"Selection evolution:",
		"Blocking validation: missing-profiles=core stale-extra-tags=(none)",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "configs/work")); err != nil {
		t.Fatalf("Source of Truth did not refresh before stale selection was detected: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".work")); !os.IsNotExist(err) {
		t.Fatalf("Managed Configuration applied after stale selection: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
}

func TestUpdateReportsProfileTagRemovalsWithoutDeletingRetiredSurfaces(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/core":    "core\n",
		"configs/retired": "retired\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core, retired]
dependencies:
  - tags: [retired]
    dependencies:
      - name: retired-tool
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
  - source: configs/retired
    target: ~/.retired
    strategy: symlink
    tags: [retired]
provisioners:
  - tool: gentle-ai
    tags: [retired]
    spec:
      scope: global
      agents: [codex]
`,
	})
	previous := state.InstalledSelection{Profiles: []string{"core"}, ResolvedTags: []string{"core", "retired"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	retiredTarget := filepath.Join(home, ".retired")
	if err := os.WriteFile(retiredTarget, []byte("keep me\n"), 0o600); err != nil {
		t.Fatalf("write retired target: %v", err)
	}
	advanceUpstream(t, origin, "remove retired profile tag", map[string]string{
		"configs/core": "core\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
`,
	})

	out := runBareUpdate(t, "--yes", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if want := "Removed: effective-tags=retired managed-entries=~/.retired dependencies=retired-tool provisioners=gentle-ai"; !strings.Contains(out, want) {
		t.Fatalf("output missing retired surfaces %q:\n%s", want, out)
	}
	got, err := os.ReadFile(retiredTarget)
	if err != nil {
		t.Fatalf("retired target was deleted: %v", err)
	}
	if string(got) != "keep me\n" {
		t.Fatalf("retired target changed to %q", got)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if got, want := meta.InstalledSelection.ResolvedTags, []string{"core"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolvedTags = %#v, want %#v", got, want)
	}
}

func TestUpdateProvisionerFailurePreservesPreviousInstalledSelection(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "gentle-ai"), "#!/bin/sh\nexit 7\n")
	writeExecStub(t, filepath.Join(stubDir, "engram"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	previous := state.InstalledSelection{Profiles: []string{"core"}, ResolvedTags: []string{"core"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/git/gitconfig": "managed\n",
		"dots.yaml":             updateProvisionerManifest,
	})

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--yes", "--profile", "default",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("update error = nil, want provisioner failure\noutput:\n%s", out.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
}

func TestUpdateStaleExtraTagReturnsStructuredDeltaAndPreservesIntent(t *testing.T) {
	requireGitCLI(t)
	t.Setenv("HOME", t.TempDir())

	setup := func() (home, stateRoot, sourceRoot string, previous state.InstalledSelection) {
		home = t.TempDir()
		stateRoot = t.TempDir()
		origin, sourceRoot := newInstalledRepo(t, map[string]string{
			"configs/retired": "retired\n",
			"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/retired
    target: ~/.retired
    strategy: symlink
    tags: [retired]
`,
		})
		previous = state.InstalledSelection{
			Profiles: []string{"core"}, ExtraTags: []string{"retired"}, ResolvedTags: []string{"core", "retired"},
		}
		if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
			t.Fatalf("save metadata: %v", err)
		}
		advanceUpstream(t, origin, "retire optional surface", map[string]string{
			"configs/core": "core\n",
			"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
`,
		})
		return home, stateRoot, sourceRoot, previous
	}

	home, stateRoot, sourceRoot, _ := setup()
	textCmd := cli.NewRootCommand()
	var textOut bytes.Buffer
	textCmd.SetOut(&textOut)
	textCmd.SetErr(&textOut)
	textCmd.SetArgs([]string{"update", "--yes",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := textCmd.Execute(); err == nil {
		t.Fatalf("text update error = nil, want stale extra Tag\noutput:\n%s", textOut.String())
	}
	for _, want := range []string{
		"Selection evolution:",
		"Removed: effective-tags=(none) managed-entries=~/.retired dependencies=(none) provisioners=(none)",
		"Blocking validation: missing-profiles=(none) stale-extra-tags=retired",
	} {
		if !strings.Contains(textOut.String(), want) {
			t.Fatalf("text output missing %q:\n%s", want, textOut.String())
		}
	}

	home, stateRoot, sourceRoot, previous := setup()
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"update", "--yes", "--output", "json",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}, &out, &errOut)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitError, out.String(), errOut.String())
	}
	var env struct {
		Data struct {
			SelectionDelta selection.Delta `json:"selection_delta"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal JSON: %v\n%s", err, out.String())
	}
	if !strings.Contains(env.Error, `recorded selection: extra Tag "retired" is no longer declared`) {
		t.Fatalf("error = %q", env.Error)
	}
	if got, want := env.Data.SelectionDelta.StaleExtraTags, []string{"retired"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stale_extra_tags = %#v, want %#v", got, want)
	}
	if got, want := env.Data.SelectionDelta.Removed.ManagedEntries, []string{"~/.retired"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("removed managed_entries = %#v, want %#v", got, want)
	}
	if _, err := os.Lstat(filepath.Join(home, ".core")); !os.IsNotExist(err) {
		t.Fatalf("Managed Configuration applied after stale extra Tag: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
}

func TestUpdateJSONReportsRecordedSelection(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	_, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/core": "core\n",
		"dots.yaml": `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core, extra]
`,
	})
	saveInstalledSelection(t, stateRoot, "core", "extra")

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"update", "--dry-run", "--output", "json",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	var env struct {
		Data struct {
			Selection struct {
				Source        string           `json:"source"`
				Profiles      []string         `json:"profiles"`
				ExtraTags     []string         `json:"extra_tags"`
				EffectiveTags []string         `json:"effective_tags"`
				Delta         *selection.Delta `json:"delta"`
			} `json:"selection"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal JSON: %v\n%s", err, out.String())
	}
	if env.Data.Selection.Source != "recorded" ||
		!reflect.DeepEqual(env.Data.Selection.Profiles, []string{"core"}) ||
		!reflect.DeepEqual(env.Data.Selection.ExtraTags, []string{"extra"}) ||
		!reflect.DeepEqual(env.Data.Selection.EffectiveTags, []string{"core", "extra"}) {
		t.Fatalf("selection = %#v", env.Data.Selection)
	}
	if env.Data.Selection.Delta == nil {
		t.Fatalf("selection delta = nil\n%s", out.String())
	}
	if env.Data.Selection.Delta.Added.EffectiveTags == nil ||
		env.Data.Selection.Delta.Removed.ManagedEntries == nil ||
		env.Data.Selection.Delta.MissingProfiles == nil ||
		env.Data.Selection.Delta.StaleExtraTags == nil {
		t.Fatalf("selection delta omitted required empty arrays: %#v", env.Data.Selection.Delta)
	}
}
