package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

func TestReadOnlyCommandsResolveSelectionWithoutDefaultFallback(t *testing.T) {
	sourceRoot := t.TempDir()
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	if err := os.WriteFile(manifestPath, []byte(`version: 1
profiles:
  default:
    tags: [core]
  alternate:
    tags: [alternate]
entries:
  - source: configs/missing
    target: ~/.missing
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-selection-test-tool
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())

	commands := []struct {
		name   string
		args   []string
		isDeps bool
	}{
		{name: "status", args: []string{"status"}},
		{name: "doctor", args: []string{"doctor"}},
		{name: "plan", args: []string{"plan"}},
		{name: "deps check", args: []string{"deps", "check"}, isDeps: true},
		{name: "deps plan", args: []string{"deps", "plan", "--tier", "homebrew"}, isDeps: true},
	}

	for _, command := range commands {
		t.Run(command.name, func(t *testing.T) {
			var golden []selectionGoldenOutcome
			commandHome := t.TempDir()
			stateRoot := filepath.Join(commandHome, ".local", "state", "dots")
			saveInstalledSelection(t, stateRoot, "default", "extra")

			recordedArgs := selectionCommandArgs(command.args, manifestPath, commandHome, sourceRoot, stateRoot, command.isDeps)
			code, data, envelopeError := runSelectionJSON(t, recordedArgs)
			if code != 2 {
				t.Fatalf("recorded selection exit code = %d, want 2; error=%q", code, envelopeError)
			}
			assertSelectionJSON(t, data, "recorded", []string{"default"}, []string{"extra"}, []string{"core", "extra"})
			golden = append(golden, selectionOutcome("recorded", code, data, envelopeError))

			explicitArgs := append(append([]string{}, command.args...), "--profile", "alternate")
			explicitArgs = selectionCommandArgs(explicitArgs, manifestPath, commandHome, sourceRoot, stateRoot, command.isDeps)
			code, data, envelopeError = runSelectionJSON(t, explicitArgs)
			if code != 0 {
				t.Fatalf("explicit selection exit code = %d, want 0; error=%q", code, envelopeError)
			}
			assertSelectionJSON(t, data, "explicit", []string{"alternate"}, nil, []string{"alternate"})
			golden = append(golden, selectionOutcome("explicit", code, data, envelopeError))

			tagOnlyArgs := append(append([]string{}, command.args...), "--tag", "alternate")
			tagOnlyArgs = selectionCommandArgs(tagOnlyArgs, manifestPath, commandHome, sourceRoot, stateRoot, command.isDeps)
			code, data, envelopeError = runSelectionJSON(t, tagOnlyArgs)
			if code != 0 {
				t.Fatalf("tag-only explicit selection exit code = %d, want 0; error=%q", code, envelopeError)
			}
			assertSelectionJSON(t, data, "explicit", nil, []string{"alternate"}, []string{"alternate"})
			golden = append(golden, selectionOutcome("tag-only explicit", code, data, envelopeError))

			missingHome := t.TempDir()
			missingArgs := selectionCommandArgs(command.args, manifestPath, missingHome, sourceRoot, filepath.Join(missingHome, "state"), command.isDeps)
			code, _, envelopeError = runSelectionJSON(t, missingArgs)
			if code != 1 || !strings.Contains(envelopeError, "selection required") {
				t.Fatalf("missing selection = code %d error %q, want code 1 selection-required error", code, envelopeError)
			}
			golden = append(golden, selectionOutcome("missing", code, nil, envelopeError))

			invalidHome := t.TempDir()
			invalidStateRoot := filepath.Join(invalidHome, ".local", "state", "dots")
			saveInstalledSelection(t, invalidStateRoot, "removed-profile")
			invalidArgs := selectionCommandArgs(command.args, manifestPath, invalidHome, sourceRoot, invalidStateRoot, command.isDeps)
			code, _, envelopeError = runSelectionJSON(t, invalidArgs)
			if code != 1 || !strings.Contains(envelopeError, "recorded selection") || !strings.Contains(envelopeError, `profile "removed-profile" not found`) {
				t.Fatalf("invalid recorded selection = code %d error %q", code, envelopeError)
			}
			golden = append(golden, selectionOutcome("invalid recorded", code, nil, envelopeError))

			invalidExplicitArgs := append(append([]string{}, command.args...), "--profile", "removed-profile")
			invalidExplicitArgs = selectionCommandArgs(invalidExplicitArgs, manifestPath, commandHome, sourceRoot, stateRoot, command.isDeps)
			code, _, envelopeError = runSelectionJSON(t, invalidExplicitArgs)
			if code != 1 || !strings.Contains(envelopeError, "explicit selection") || !strings.Contains(envelopeError, `profile "removed-profile" not found`) {
				t.Fatalf("invalid explicit selection = code %d error %q", code, envelopeError)
			}
			golden = append(golden, selectionOutcome("invalid explicit", code, nil, envelopeError))

			emptyRecordedHome := t.TempDir()
			emptyRecordedStateRoot := filepath.Join(emptyRecordedHome, ".local", "state", "dots")
			saveEmptyInstalledSelection(t, emptyRecordedStateRoot)
			emptyRecordedArgs := selectionCommandArgs(command.args, manifestPath, emptyRecordedHome, sourceRoot, emptyRecordedStateRoot, command.isDeps)
			code, data, envelopeError = runSelectionJSON(t, emptyRecordedArgs)
			if code != 0 || envelopeError != "" {
				t.Fatalf("empty recorded selection = code %d error %q, want valid aligned selection", code, envelopeError)
			}
			assertSelectionJSON(t, data, "recorded", nil, nil, nil)
			golden = append(golden, selectionOutcome("empty recorded", code, data, envelopeError))

			emptyExplicitArgs := append(append([]string{}, command.args...), "--profile", "")
			emptyExplicitArgs = selectionCommandArgs(emptyExplicitArgs, manifestPath, commandHome, sourceRoot, stateRoot, command.isDeps)
			code, _, envelopeError = runSelectionJSON(t, emptyExplicitArgs)
			if code != 1 || envelopeError != "explicit selection: profile names must not be empty" {
				t.Fatalf("empty explicit selection = code %d error %q", code, envelopeError)
			}
			golden = append(golden, selectionOutcome("empty explicit", code, nil, envelopeError))

			whitespaceRecordedHome := t.TempDir()
			whitespaceRecordedStateRoot := filepath.Join(whitespaceRecordedHome, ".local", "state", "dots")
			saveWhitespaceInstalledSelection(t, whitespaceRecordedStateRoot)
			whitespaceRecordedArgs := selectionCommandArgs(command.args, manifestPath, whitespaceRecordedHome, sourceRoot, whitespaceRecordedStateRoot, command.isDeps)
			code, _, envelopeError = runSelectionJSON(t, whitespaceRecordedArgs)
			if code != 1 || envelopeError != "recorded selection: tags must not be empty" {
				t.Fatalf("whitespace recorded selection = code %d error %q", code, envelopeError)
			}
			golden = append(golden, selectionOutcome("whitespace recorded", code, nil, envelopeError))

			whitespaceExplicitArgs := append(append([]string{}, command.args...), "--tag", " ")
			whitespaceExplicitArgs = selectionCommandArgs(whitespaceExplicitArgs, manifestPath, commandHome, sourceRoot, stateRoot, command.isDeps)
			code, _, envelopeError = runSelectionJSON(t, whitespaceExplicitArgs)
			if code != 1 || envelopeError != "explicit selection: tags must not be empty" {
				t.Fatalf("whitespace explicit selection = code %d error %q", code, envelopeError)
			}
			golden = append(golden, selectionOutcome("whitespace explicit", code, nil, envelopeError))

			assertReadSelectionGolden(t, strings.ReplaceAll(command.name, " ", "_"), golden)
		})
	}
}

func TestReadOnlyRecordedLegacyTagRequiresCurrentExplicitIntent(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, sourceRoot, `version: 1
tags:
  core: {description: Core, kind: surface, status: current}
  new: {description: New, kind: surface, status: current}
  old:
    description: Old
    kind: compatibility
    status: legacy
    replaced_by: [new]
profiles:
  core:
    tags: [core]
entries:
  - {source: configs/new, target: ~/.new, strategy: symlink, tags: [new]}
`)
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: state.CurrentVersion,
		InstalledSelection: &state.InstalledSelection{
			Profiles: []string{"core"}, ExtraTags: []string{"old"}, ResolvedTags: []string{"core", "old"},
		},
	}); err != nil {
		t.Fatalf("save Installed Selection: %v", err)
	}

	args := selectionCommandArgs([]string{"plan"}, manifestPath, home, sourceRoot, stateRoot, false)
	code, data, envelopeError := runSelectionJSON(t, args)
	if code != cli.ExitError || !strings.Contains(envelopeError, legacyTagMigrationCodeForTest) {
		t.Fatalf("recorded legacy selection = code %d error %q", code, envelopeError)
	}
	if got := data["code"]; got != legacyTagMigrationCodeForTest {
		t.Fatalf("data.code = %v, want %q", got, legacyTagMigrationCodeForTest)
	}
	if got := data["remediation"].(map[string]any)["recommended_command"]; got != "dots install --profile core --tag new" {
		t.Fatalf("recommended command = %v", got)
	}

	explicitArgs := append([]string{"plan", "--profile", "core", "--tag", "new"}, args[1:]...)
	code, explicitData, envelopeError := runSelectionJSON(t, explicitArgs)
	if code != cli.ExitOK || envelopeError != "" {
		t.Fatalf("explicit current selection = code %d error %q", code, envelopeError)
	}
	assertSelectionJSON(t, explicitData, "explicit", []string{"core"}, []string{"new"}, []string{"core", "new"})

	var stdout, stderr bytes.Buffer
	code = cli.Run([]string{
		"plan", "--tag", "old", "--file", manifestPath,
		"--home", home, "--source-root", sourceRoot, "--state-root", t.TempDir(),
	}, &stdout, &stderr)
	if code != cli.ExitOK {
		t.Fatalf("explicit legacy text plan = code %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{"legacy Tag normalization: old -> new", "transitional alias"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("explicit legacy text output missing %q:\n%s", want, stdout.String())
		}
	}
}

const legacyTagMigrationCodeForTest = "legacy-tag-migration-required"

func TestReadOnlySelectionTextReportsSource(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "other")
	saveInstalledSelection(t, stateRoot, "default")

	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{
			name: "recorded",
			args: []string{"status", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot},
			want: "[selection: recorded]",
		},
		{
			name: "explicit",
			args: []string{"status", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot},
			want: "[selection: explicit]",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := cli.Run(tt.args, &stdout, &stderr); code != 0 {
				t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Fatalf("output missing %q:\n%s", tt.want, stdout.String())
			}
		})
	}
}

func TestReadOnlyCommandsDiagnoseOverrideOmittedByExplicitSelection(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceRoot, "configs", "zellij"), 0o755); err != nil {
		t.Fatalf("mkdir sources: %v", err)
	}
	defaultSource := filepath.Join(sourceRoot, "configs", "zellij", "default.kdl")
	adaptiveSource := filepath.Join(sourceRoot, "configs", "zellij", "adaptive.kdl")
	if err := os.WriteFile(defaultSource, []byte("dark\n"), 0o600); err != nil {
		t.Fatalf("write default source: %v", err)
	}
	if err := os.WriteFile(adaptiveSource, []byte("adaptive\n"), 0o600); err != nil {
		t.Fatalf("write adaptive source: %v", err)
	}
	target := filepath.Join(home, ".config", "zellij", "config.kdl")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Symlink(adaptiveSource, target); err != nil {
		t.Fatalf("symlink target: %v", err)
	}
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	if err := os.WriteFile(manifestPath, []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zellij/default.kdl
    source_overrides:
      adaptive-theme: configs/zellij/adaptive.kdl
    target: ~/.config/zellij/config.kdl
    strategy: symlink
    tags: [core]
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	saveInstalledSelection(t, stateRoot, "default", "adaptive-theme")

	for _, command := range []struct {
		name       string
		args       []string
		collection string
		stateKey   string
		aligned    string
	}{
		{name: "plan", args: []string{"plan"}, collection: "actions", stateKey: "status", aligned: "unchanged"},
		{name: "status", args: []string{"status"}, collection: "entries", stateKey: "state", aligned: "ok"},
	} {
		t.Run(command.name, func(t *testing.T) {
			recordedArgs := selectionCommandArgs(command.args, manifestPath, home, sourceRoot, stateRoot, false)
			code, data, envelopeError := runSelectionJSON(t, recordedArgs)
			if code != 0 {
				t.Fatalf("recorded selection exit code = %d, want 0; error=%q", code, envelopeError)
			}
			assertSelectionJSON(t, data, "recorded", []string{"default"}, []string{"adaptive-theme"}, []string{"core", "adaptive-theme"})
			items := data[command.collection].([]any)
			if got := items[0].(map[string]any)[command.stateKey]; got != command.aligned {
				t.Fatalf("recorded %s = %v, want %q", command.stateKey, got, command.aligned)
			}

			explicitArgs := append(append([]string{}, command.args...), "--profile", "default")
			explicitArgs = selectionCommandArgs(explicitArgs, manifestPath, home, sourceRoot, stateRoot, false)
			code, data, envelopeError = runSelectionJSON(t, explicitArgs)
			if code != 2 {
				t.Fatalf("explicit selection exit code = %d, want 2; error=%q", code, envelopeError)
			}
			items = data[command.collection].([]any)
			item := items[0].(map[string]any)
			if item[command.stateKey] != "conflict" ||
				item["reason"] != "source-override-not-selected" ||
				mustJSON(t, item["matching_tags"]) != `["adaptive-theme"]` {
				t.Fatalf("explicit selection item = %#v, want diagnosed conflict", item)
			}
		})
	}
}

func TestDepsExplicitSelectionWorksWithoutInstallationMetadata(t *testing.T) {
	home := t.TempDir()
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/unused
    target: ~/.unused
    strategy: symlink
    tags: [other]
`)
	for _, args := range [][]string{
		{"deps", "check", "--profile", "default"},
		{"deps", "plan", "--profile", "default", "--tier", "homebrew"},
	} {
		args = append(args, "--output", "json", "--file", manifestPath, "--home", home)
		code, data, envelopeError := runSelectionJSON(t, args)
		if code != 0 {
			t.Fatalf("%v exit code = %d, want 0; error=%q", args[:2], code, envelopeError)
		}
		assertSelectionJSON(t, data, "explicit", []string{"default"}, nil, []string{"core"})
	}
}

func selectionCommandArgs(args []string, manifestPath, home, sourceRoot, stateRoot string, isDeps bool) []string {
	result := append(append([]string{}, args...), "--output", "json", "--file", manifestPath, "--home", home)
	if !isDeps {
		result = append(result, "--source-root", sourceRoot, "--state-root", stateRoot)
	}
	return result
}

func saveInstalledSelection(t *testing.T, stateRoot, profile string, extraTags ...string) {
	t.Helper()
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: state.CurrentVersion,
		InstalledSelection: &state.InstalledSelection{
			Profiles:     []string{profile},
			ExtraTags:    extraTags,
			ResolvedTags: append([]string{profile}, extraTags...),
		},
	}); err != nil {
		t.Fatalf("save Installed Selection: %v", err)
	}
}

func saveEmptyInstalledSelection(t *testing.T, stateRoot string) {
	t.Helper()
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		InstalledSelection: &state.InstalledSelection{},
	}); err != nil {
		t.Fatalf("save empty Installed Selection: %v", err)
	}
}

func saveWhitespaceInstalledSelection(t *testing.T, stateRoot string) {
	t.Helper()
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: state.CurrentVersion,
		InstalledSelection: &state.InstalledSelection{
			ExtraTags: []string{" "},
		},
	}); err != nil {
		t.Fatalf("save whitespace Installed Selection: %v", err)
	}
}

func runSelectionJSON(t *testing.T, args []string) (int, map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	var env struct {
		Data  map[string]any `json:"data"`
		Error string         `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	return code, env.Data, env.Error
}

func assertSelectionJSON(t *testing.T, data map[string]any, source string, profiles, extraTags, effectiveTags []string) {
	t.Helper()
	got, ok := data["selection"].(map[string]any)
	if !ok {
		t.Fatalf("data.selection = %#v, want object", data["selection"])
	}
	want := map[string]any{
		"source":         source,
		"profiles":       stringSliceToAny(profiles),
		"extra_tags":     stringSliceToAny(extraTags),
		"effective_tags": stringSliceToAny(effectiveTags),
	}
	for key, value := range want {
		if gotJSON, wantJSON := mustJSON(t, got[key]), mustJSON(t, value); gotJSON != wantJSON {
			t.Fatalf("selection.%s = %s, want %s", key, gotJSON, wantJSON)
		}
	}
}

func stringSliceToAny(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type selectionGoldenOutcome struct {
	Scenario  string `json:"scenario"`
	ExitCode  int    `json:"exit_code"`
	Selection any    `json:"selection,omitempty"`
	Error     string `json:"error,omitempty"`
}

func selectionOutcome(scenario string, exitCode int, data map[string]any, envelopeError string) selectionGoldenOutcome {
	var reported any
	if data != nil {
		reported = data["selection"]
	}
	return selectionGoldenOutcome{Scenario: scenario, ExitCode: exitCode, Selection: reported, Error: envelopeError}
}

func assertReadSelectionGolden(t *testing.T, command string, outcomes []selectionGoldenOutcome) {
	t.Helper()
	got, err := json.MarshalIndent(outcomes, "", "  ")
	if err != nil {
		t.Fatalf("marshal selection golden: %v", err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "selection_"+command+".golden")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read selection golden %s: %v\ngot:\n%s", path, err, got)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("selection golden mismatch for %s\ngot:\n%s\nwant:\n%s", command, got, want)
	}
}
