package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
)

type exclusionFixture struct{ home, source, state, file string }

func newExclusionFixture(t *testing.T) exclusionFixture {
	t.Helper()
	root := t.TempDir()
	f := exclusionFixture{home: filepath.Join(root, "home"), source: filepath.Join(root, "source")}
	f.state = filepath.Join(f.home, ".local", "state", "dots")
	f.file = filepath.Join(f.source, "dots.yaml")
	for _, dir := range []string{f.home, f.source} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{"base": "base\n", "official": "official\n", "custom": "custom\n", "base.json": `{"base":true}`, "official.json": `{"official":true}`} {
		if err := os.WriteFile(filepath.Join(f.source, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manifest := `version: 1
tags:
  base: {description: Base, kind: surface, status: current}
  official: {description: Official, kind: surface, status: current}
  custom: {description: Custom, kind: surface, status: current, conflicts_with: [official]}
  unused: {description: Unused, kind: surface, status: current}
profiles:
  workstation:
    tags: [base, official]
  shell:
    tags: [base]
entries:
  - {source: base, target: ~/.base, strategy: symlink, tags: [base]}
  - {source: official, target: ~/.official, strategy: symlink, tags: [official]}
  - {source: custom, target: ~/.custom, strategy: symlink, tags: [custom]}
  - {source: base.json, target: ~/.shared, strategy: copy, ownership: json-subset, tags: [base]}
  - {source: official.json, target: ~/.shared, strategy: copy, ownership: json-subset, tags: [official]}
`
	if err := os.WriteFile(f.file, []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f exclusionFixture) args(command []string, flags ...string) []string {
	args := append([]string(nil), command...)
	args = append(args, "--file", f.file, "--home", f.home)
	if command[0] != "deps" {
		args = append(args, "--source-root", f.source, "--state-root", f.state)
	}
	args = append(args, flags...)
	return args
}

func (f exclusionFixture) run(t *testing.T, command []string, flags ...string) (string, error) {
	t.Helper()
	args := f.args(command, flags...)
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func (f exclusionFixture) runEnvelope(t *testing.T, command []string, flags ...string) (string, int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := Run(f.args(command, flags...), &out, &errOut)
	return out.String(), code
}

func (f exclusionFixture) installed(t *testing.T) *state.InstalledSelection {
	t.Helper()
	meta, err := state.Load(state.Path(f.state))
	if err != nil {
		t.Fatal(err)
	}
	return meta.InstalledSelection
}
func requireExclusionRun(t *testing.T, f exclusionFixture, command []string, flags ...string) string {
	t.Helper()
	out, err := f.run(t, command, flags...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", command, err, out)
	}
	return out
}
func TestSelectionExclusionsAcrossCommandsAndRecordedReads(t *testing.T) {
	f := newExclusionFixture(t)
	flags := []string{"--profile", "workstation", "--profile", "shell", "--exclude-tag", "official", "--exclude-tag", "official", "--tag", "custom"}
	for _, command := range [][]string{{"plan"}, {"status"}, {"doctor"}, {"deps", "check"}, {"deps", "plan"}, {"deps", "install"}, {"install"}} {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			extra := append([]string(nil), flags...)
			if command[0] == "install" {
				extra = append(extra, "--dry-run", "--skip-deps")
			}
			if command[0] == "deps" && command[1] == "install" {
				extra = append(extra, "--dry-run", "--tier", "generic")
			}
			out, err := f.run(t, command, append(extra, "--output", "json")...)
			// Read-only diagnostic findings are not execution failures.
			if err != nil && !isFindingsError(err) {
				t.Fatalf("%v\n%s", err, out)
			}
			if command[0] != "deps" || command[1] != "install" {
				if !strings.Contains(out, `"excluded_tags"`) {
					t.Fatalf("exclusion absent from JSON: %s", out)
				}
			}
		})
	}
	if f.installed(t) != nil {
		t.Fatal("preview/read-only commands persisted selection")
	}
	requireExclusionRun(t, f, []string{"install"}, append(flags, "--yes", "--skip-deps")...)
	got := f.installed(t)
	if !reflect.DeepEqual(got.Profiles, []string{"workstation", "shell"}) || !reflect.DeepEqual(got.ExtraTags, []string{"custom"}) || !reflect.DeepEqual(got.ExcludedTags, []string{"official"}) || !reflect.DeepEqual(got.ResolvedTags, []string{"base", "custom"}) {
		t.Fatalf("intent = %+v", got)
	}
	for _, command := range [][]string{{"plan"}, {"status"}, {"doctor"}, {"deps", "check"}, {"deps", "plan"}} {
		out, err := f.run(t, command, "--output", "json")
		if err != nil && !isFindingsError(err) {
			t.Fatalf("%v: %v\n%s", command, err, out)
		}
		var env struct {
			Data struct {
				Selection selection.Report `json:"selection"`
			}
		}
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Fatal(err)
		}
		if env.Data.Selection.Source != selection.SourceRecorded || !reflect.DeepEqual(env.Data.Selection.ExcludedTags, []string{"official"}) || !reflect.DeepEqual(env.Data.Selection.EffectiveTags, []string{"base", "custom"}) {
			t.Fatalf("recorded %v = %+v", command, env.Data.Selection)
		}
	}
	if out := requireExclusionRun(t, f, []string{"installed"}); !strings.Contains(out, "Excluded Tags: official") {
		t.Fatalf("installed text: %s", out)
	}
}

func TestSelectionExclusionsRejectContradictionsWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
		want  string
	}{
		{"unknown", []string{"--profile", "workstation", "--exclude-tag", "missing"}, "not declared"},
		{"unselected", []string{"--profile", "workstation", "--exclude-tag", "unused"}, "not selected"},
		{"explicit", []string{"--profile", "workstation", "--tag", "official", "--exclude-tag", "official"}, "explicitly added and excluded"},
		{"blank", []string{"--profile", "workstation", "--exclude-tag", ""}, "must not be empty"},
		{"exclusion only", []string{"--exclude-tag", "official"}, "at least one Profile"},
		{"clear", []string{"--clear-selection", "--exclude-tag", "official"}, "none of the others"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExclusionFixture(t)
			out, err := f.run(t, []string{"install"}, append(tc.flags, "--yes", "--skip-deps")...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want %q\n%s", err, tc.want, out)
			}
			if f.installed(t) != nil {
				t.Fatal("invalid request persisted intent")
			}
			if _, err := os.Lstat(filepath.Join(f.home, ".base")); !os.IsNotExist(err) {
				t.Fatalf("invalid request mutated home: %v", err)
			}
		})
	}
}

func TestSelectionExclusionReductionPreviewAcknowledgementAndReconciliation(t *testing.T) {
	f := newExclusionFixture(t)
	requireExclusionRun(t, f, []string{"install"}, "--profile", "workstation", "--yes", "--skip-deps")
	before := *f.installed(t)
	// Preserve co-owned application content while subtracting the retired contribution.
	if err := os.WriteFile(filepath.Join(f.home, ".shared"), []byte(`{"base":true,"official":true,"external":"keep"}`), 0600); err != nil {
		t.Fatal(err)
	}
	flags := []string{"--profile", "workstation", "--exclude-tag", "official", "--tag", "custom", "--skip-deps"}
	out := requireExclusionRun(t, f, []string{"install"}, append(flags, "--dry-run", "--output", "json")...)
	if !strings.Contains(out, `"acknowledgement_required": true`) || !strings.Contains(out, `"reconcile"`) {
		t.Fatalf("preview missing protections: %s", out)
	}
	_, err := f.run(t, []string{"install"}, append(flags, "--yes")...)
	if err == nil || !strings.Contains(err.Error(), selectionChangeAcknowledgementRequiredCode) {
		t.Fatalf("missing acknowledgement: %v", err)
	}
	requireExclusionRun(t, f, []string{"install"}, flags...) // default input declines
	if !reflect.DeepEqual(*f.installed(t), before) {
		t.Fatal("dry, unacknowledged or cancelled run committed intent")
	}
	requireExclusionRun(t, f, []string{"install"}, append(flags, "--yes", "--acknowledge-selection-change")...)
	if _, err := os.Lstat(filepath.Join(f.home, ".official")); !os.IsNotExist(err) {
		t.Fatalf("retired target remains: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(f.home, ".shared"))
	if err != nil {
		t.Fatal(err)
	}
	var shared map[string]any
	if err := json.Unmarshal(data, &shared); err != nil {
		t.Fatal(err)
	}
	if shared["external"] != "keep" || shared["base"] != true || shared["official"] != nil {
		t.Fatalf("unsafe reconciliation: %s", data)
	}
	if !reflect.DeepEqual(f.installed(t).ExcludedTags, []string{"official"}) {
		t.Fatal("successful reduction lost exclusions")
	}
}

func TestSelectionExclusionTerminalFailurePreservesIntentAndExternalRecreation(t *testing.T) {
	f := newExclusionFixture(t)
	requireExclusionRun(t, f, []string{"install"}, "--profile", "workstation", "--yes", "--skip-deps")
	before := *f.installed(t)
	original := commitInstallationMetadata
	t.Cleanup(func() { commitInstallationMetadata = original })
	commitInstallationMetadata = func(install.MetadataCommit, *state.InstalledSelection, state.InstalledSelection) error {
		return errors.New("injected terminal failure")
	}
	flags := []string{"--profile", "workstation", "--exclude-tag", "official", "--yes", "--acknowledge-selection-change", "--skip-deps"}
	_, err := f.run(t, []string{"install"}, flags...)
	if err == nil || !strings.Contains(err.Error(), "injected terminal failure") {
		t.Fatalf("failure = %v", err)
	}
	if !reflect.DeepEqual(*f.installed(t), before) {
		t.Fatal("terminal failure committed exclusion intent")
	}
	if _, err := os.Lstat(filepath.Join(f.home, ".official")); !os.IsNotExist(err) {
		t.Fatalf("expected applied retirement progress: %v", err)
	}
	if err := os.WriteFile(filepath.Join(f.home, ".official"), []byte("external recreation"), 0600); err != nil {
		t.Fatal(err)
	}
	commitInstallationMetadata = original
	requireExclusionRun(t, f, []string{"install"}, flags...)
	content, err := os.ReadFile(filepath.Join(f.home, ".official"))
	if err != nil || string(content) != "external recreation" {
		t.Fatalf("retry removed external recreation: %q %v", content, err)
	}
	if !reflect.DeepEqual(f.installed(t).ExcludedTags, []string{"official"}) {
		t.Fatal("retry failed to commit exclusions")
	}
}

func isFindingsError(err error) bool {
	var finding *FindingsError
	return errors.As(err, &finding)
}

func TestSelectionExclusionsUpdateProfileEvolutionAndStaleIntent(t *testing.T) {
	for _, scenario := range []string{"refresh", "stale exclusion", "concurrent refresh"} {
		t.Run(scenario, func(t *testing.T) {
			f := newExclusionFixture(t)
			origin := f.source
			runContinuationGit(t, origin, "init", "-b", "main")
			runContinuationGit(t, origin, "config", "user.email", "tests@example.com")
			runContinuationGit(t, origin, "config", "user.name", "dots tests")
			runContinuationGit(t, origin, "add", "-A")
			runContinuationGit(t, origin, "commit", "-m", "initial")
			f.source = t.TempDir()
			runContinuationGit(t, "", "clone", origin, f.source)
			f.file = filepath.Join(f.source, "dots.yaml")
			requireExclusionRun(t, f, []string{"install"}, "--profile", "workstation", "--exclude-tag", "official", "--tag", "custom", "--yes", "--skip-deps")
			before := *f.installed(t)
			content, err := os.ReadFile(filepath.Join(origin, "dots.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			next := strings.Replace(string(content), "tags: [base, official]", "tags: [base, official, unused]", 1)
			if scenario == "stale exclusion" {
				next = strings.Replace(next, "tags: [base, official, unused]", "tags: [base, unused]", 1)
			}
			next += "  - {source: base, target: ~/.new, strategy: symlink, tags: [unused]}\n"
			if err := os.WriteFile(filepath.Join(origin, "dots.yaml"), []byte(next), 0600); err != nil {
				t.Fatal(err)
			}
			runContinuationGit(t, origin, "add", "-A")
			runContinuationGit(t, origin, "commit", "-m", "evolve Profile")
			concurrent := before
			if scenario == "concurrent refresh" {
				concurrent.ExcludedTags = []string{"base"}
				meta, err := state.Load(state.Path(f.state))
				if err != nil {
					t.Fatal(err)
				}
				meta.InstalledSelection = &concurrent
				replacement := filepath.Join(t.TempDir(), "concurrent.json")
				if err := state.Save(replacement, meta); err != nil {
					t.Fatal(err)
				}
				quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
				hook := "#!/bin/sh\ncp " + quote(replacement) + " " + quote(state.Path(f.state)) + "\n"
				if err := os.WriteFile(filepath.Join(f.source, ".git", "hooks", "post-merge"), []byte(hook), 0700); err != nil {
					t.Fatal(err)
				}
			}
			out, code := f.runEnvelope(t, []string{"update"}, "--yes", "--output", "json")
			err = nil
			if code != ExitOK {
				err = errors.New(out)
			}
			if scenario == "refresh" {
				if err != nil {
					t.Fatalf("update: %v\n%s", err, out)
				}
				if !reflect.DeepEqual(f.installed(t).ExcludedTags, before.ExcludedTags) || !reflect.DeepEqual(f.installed(t).ResolvedTags, []string{"base", "unused", "custom"}) {
					t.Fatalf("refreshed intent: %+v", f.installed(t))
				}
				if _, err := os.Readlink(filepath.Join(f.home, ".new")); err != nil {
					t.Fatalf("new Profile Tag not applied: %v", err)
				}
			} else {
				want := "excluded Tag"
				expected := before
				if scenario == "concurrent refresh" {
					want = "changed concurrently"
					expected = concurrent
				}
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("%s error: %v\n%s", scenario, err, out)
				}
				if scenario == "stale exclusion" {
					var env struct {
						Data struct {
							Delta selection.Delta `json:"selection_delta"`
						}
					}
					if err := json.Unmarshal([]byte(out), &env); err != nil {
						t.Fatal(err)
					}
					assertGolden(t, "selection_exclusion_evolution_error.golden", []byte(out))
					if !reflect.DeepEqual(env.Data.Delta.StaleExcludedTags, []string{"official"}) || !reflect.DeepEqual(env.Data.Delta.Previous.ExcludedTags, []string{"official"}) || !reflect.DeepEqual(env.Data.Delta.Current.ExcludedTags, []string{"official"}) {
						t.Fatalf("missing blocking exclusion delta: %s", out)
					}
				}
				if !reflect.DeepEqual(*f.installed(t), expected) {
					t.Fatalf("failure changed intent: %+v", f.installed(t))
				}
				if _, err := os.Lstat(filepath.Join(f.home, ".new")); !os.IsNotExist(err) {
					t.Fatalf("failure applied new target: %v", err)
				}
			}
		})
	}
}

func TestSelectionExclusionTerminalCASPreservesConcurrentIntent(t *testing.T) {
	f := newExclusionFixture(t)
	requireExclusionRun(t, f, []string{"install"}, "--profile", "workstation", "--yes", "--skip-deps")
	concurrent := *f.installed(t)
	concurrent.Provenance.RecordedAt = "concurrent after retirement"
	original := commitInstallationMetadata
	t.Cleanup(func() { commitInstallationMetadata = original })
	commitInstallationMetadata = func(commit install.MetadataCommit, expected *state.InstalledSelection, requested state.InstalledSelection) error {
		if err := state.Update(state.Path(f.state), func(meta *state.Metadata) error { meta.InstalledSelection = &concurrent; return nil }); err != nil {
			return err
		}
		return original(commit, expected, requested)
	}
	_, err := f.run(t, []string{"install"}, "--profile", "workstation", "--exclude-tag", "official", "--yes", "--acknowledge-selection-change", "--skip-deps")
	if err == nil || !strings.Contains(err.Error(), "changed concurrently") {
		t.Fatalf("terminal CAS error = %v", err)
	}
	if !reflect.DeepEqual(*f.installed(t), concurrent) {
		t.Fatal("terminal commit overwrote concurrent selection")
	}
}

func TestRepositoryWorkstationExcludesHerdrBeforeSelectingCustom(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	f := newExclusionFixture(t)
	f.source = root
	f.file = filepath.Join(root, "dots.yaml")
	out := requireExclusionRun(t, f, []string{"plan"}, "--profile", "workstation", "--exclude-tag", "herdr", "--tag", "herdr-custom", "--output", "json")
	var env struct {
		Data struct {
			Selection selection.Report `json:"selection"`
		}
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(env.Data.Selection.ExcludedTags, []string{"herdr"}) {
		t.Fatalf("selection=%+v", env.Data.Selection)
	}
	for _, tag := range env.Data.Selection.EffectiveTags {
		if tag == "herdr" {
			t.Fatal("official Herdr remained selected")
		}
	}
	if _, err := f.run(t, []string{"plan"}, "--profile", "workstation", "--tag", "herdr-custom"); err == nil || !strings.Contains(err.Error(), "cannot be selected together") {
		t.Fatalf("implicit replacement accepted: %v", err)
	}
}

func TestSelectionExclusionsJSONGolden(t *testing.T) {
	f := newExclusionFixture(t)
	out := requireExclusionRun(t, f, []string{"plan"}, "--profile", "workstation", "--exclude-tag", "official", "--tag", "custom", "--output", "json")
	var env struct {
		Data struct {
			Selection selection.Report `json:"selection"`
		}
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(env.Data.Selection, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "selection_exclusions.golden", append(data, '\n'))
}

func TestUpgradeContinuationRejectsMissingIdentityAndForgedRecordedIntent(t *testing.T) {
	f := newExclusionFixture(t)
	requireExclusionRun(t, f, []string{"install"}, "--profile", "workstation", "--yes", "--skip-deps")
	runContinuationGit(t, f.source, "init", "-b", "main")
	before := *f.installed(t)
	fp, err := installedSelectionFingerprint(&before)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		flags []string
		want  string
	}{
		{"missing identity", nil, "requires the previous selection identity"},
		{"forged intent", []string{"--selection-previous", fp}, "intent does not match recorded selection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := []string{"--continue", "--yes", "--selection-source", "recorded", "--selection-profile", "shell"}
			flags = append(flags, tc.flags...)
			_, err := f.run(t, []string{"upgrade"}, flags...)
			// The fixture need only be a local repository to exercise continuation resolution.
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("continuation error=%v want %s", err, tc.want)
			}
			if !reflect.DeepEqual(*f.installed(t), before) {
				t.Fatal("forged continuation changed intent")
			}
		})
	}
}

func TestUpgradeContinuationRejectsBareResume(t *testing.T) {
	f := newExclusionFixture(t)
	_, err := f.run(t, []string{"upgrade"}, "--continue", "--yes")
	if err == nil || !strings.Contains(err.Error(), "requires a selection source") {
		t.Fatalf("bare continuation error=%v", err)
	}
	if f.installed(t) != nil {
		t.Fatal("bare continuation created selection")
	}
}

func TestRecordedLegacyExclusionsRequireMigrationForReadAndRefresh(t *testing.T) {
	f := newExclusionFixture(t)
	// Previously current official becomes a legacy exclusion with two current
	// replacements. The operator must confirm this broader exclusion constraint.
	data, err := os.ReadFile(f.file)
	if err != nil {
		t.Fatal(err)
	}
	next := strings.Replace(string(data), "official: {description: Official, kind: surface, status: current}", "official: {description: Official, kind: compatibility, status: legacy, replaced_by: [custom, unused]}", 1)
	next = strings.Replace(next, "status: current, conflicts_with: [official]", "status: current", 1)
	next = strings.Replace(next, "tags: [base, official]", "tags: [base, custom, unused]", 1)
	next = strings.ReplaceAll(next, "tags: [official]", "tags: [custom]")
	if err := os.WriteFile(f.file, []byte(next), 0600); err != nil {
		t.Fatal(err)
	}
	previous := &state.InstalledSelection{Profiles: []string{"workstation"}, ExcludedTags: []string{"official"}, ResolvedTags: []string{"base"}}
	if err := state.Save(state.Path(f.state), state.Metadata{Version: state.CurrentVersion, InstalledSelection: previous}); err != nil {
		t.Fatal(err)
	}
	runContinuationGit(t, f.source, "init", "-b", "main")
	for _, command := range [][]string{{"plan"}, {"update"}} {
		flags := []string{"--output", "json"}
		if command[0] == "update" {
			flags = append(flags, "--yes")
		}
		out, code := f.runEnvelope(t, command, flags...)
		if code != ExitError || !strings.Contains(out, "legacy-tag-migration-required") || !strings.Contains(out, `"legacy_tag": "official"`) {
			t.Fatalf("%v bypassed exclusion migration: %v\n%s", command, code, out)
		}
		if !reflect.DeepEqual(f.installed(t), previous) {
			t.Fatal("unconfirmed exclusion migration rewrote intent")
		}
	}
}
