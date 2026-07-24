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
	"github.com/yersonargotev/dots/internal/state"
)

func TestUpdateConfirmsUnambiguousLegacySelectionBeforeRecording(t *testing.T) {
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
    tags: [core]
`,
	})
	target := filepath.Join(home, ".core")
	if err := os.Symlink(filepath.Join(sourceRoot, "configs/core"), target); err != nil {
		t.Fatalf("seed managed target: %v", err)
	}
	legacy := state.Metadata{
		Version: 2,
		Entries: []state.Record{{
			Target: target, Source: "configs/core", Strategy: "symlink",
			Profiles: []string{"core"}, Tags: []string{"core"},
		}},
	}
	if err := state.Save(state.Path(stateRoot), legacy); err != nil {
		t.Fatalf("save legacy metadata: %v", err)
	}

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader("yes\n"))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"update", "--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("update: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"Migration candidate: profiles=core extra-tags=(none) effective-tags=core confidence=high",
		"Confirm this migration candidate before update/upgrade? [y/N]",
		"Selection: source=migration profiles=core",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load migrated metadata: %v", err)
	}
	if meta.Version != state.CurrentVersion || meta.InstalledSelection == nil {
		t.Fatalf("metadata was not migrated: %#v", meta)
	}
	if got, want := meta.InstalledSelection.Profiles, []string{"core"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Profiles = %#v, want %#v", got, want)
	}
	if len(meta.Entries) != 1 || meta.Entries[0].Target != target {
		t.Fatalf("legacy inventory was not preserved: %#v", meta.Entries)
	}
}

func TestUpdateNonInteractiveLegacySelectionReturnsStructuredMigrationError(t *testing.T) {
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
    tags: [core]
`,
	})
	target := filepath.Join(home, ".core")
	if err := os.Symlink(filepath.Join(sourceRoot, "configs/core"), target); err != nil {
		t.Fatalf("seed managed target: %v", err)
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: 2,
		Entries: []state.Record{{
			Target: target, Source: "configs/core", Strategy: "symlink",
			Profiles: []string{"core"}, Tags: []string{"core"},
		}},
	}); err != nil {
		t.Fatalf("save legacy metadata: %v", err)
	}
	oldHead := strings.TrimSpace(runGitOutput(t, sourceRoot, "rev-parse", "HEAD"))
	advanceUpstream(t, origin, "incoming change", map[string]string{"incoming": "not applied\n"})

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"update", "--yes", "--output", "json",
		"--file", filepath.Join(sourceRoot, "dots.yaml"),
		"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}, &out, &errOut)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitError, out.String(), errOut.String())
	}
	var envelope struct {
		Status string `json:"status"`
		Data   struct {
			Code      string `json:"code"`
			Candidate struct {
				Profiles []string `json:"profiles"`
			} `json:"candidate"`
		} `json:"data"`
	}
	if jsonErr := json.Unmarshal(out.Bytes(), &envelope); jsonErr != nil {
		t.Fatalf("decode JSON: %v\n%s", jsonErr, out.String())
	}
	if envelope.Status != "error" || envelope.Data.Code != "selection-migration-required" ||
		!reflect.DeepEqual(envelope.Data.Candidate.Profiles, []string{"core"}) {
		t.Fatalf("unexpected migration envelope: %#v", envelope)
	}
	if got := strings.TrimSpace(runGitOutput(t, sourceRoot, "rev-parse", "HEAD")); got != oldHead {
		t.Fatalf("update mutated repository before explicit migration: HEAD=%s want=%s", got, oldHead)
	}
	meta, loadErr := state.Load(state.Path(stateRoot))
	if loadErr != nil {
		t.Fatalf("load metadata: %v", loadErr)
	}
	if meta.Version != 2 || meta.InstalledSelection != nil {
		t.Fatalf("non-interactive migration mutated metadata: %#v", meta)
	}
}
