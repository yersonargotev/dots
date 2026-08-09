package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

func TestInstalledJSONEnvelope(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core")
	target := filepath.Join(home, ".zshrc")
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: 2, Provenance: state.Provenance{SourceRoot: sourceRoot, SourceRevision: "abc123", DotsVersion: "v0.test"}, Entries: []state.Record{{
		Target:   target,
		Source:   "configs/zsh/zshrc",
		Strategy: "symlink",
		Profiles: []string{"default"},
		Tags:     []string{"core"},
	}}}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"installed", "--output", "json", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	env := decodeEnvelopeForCommand(t, out.String(), "installed")
	if env.Status != "ok" {
		t.Fatalf("status = %q, want ok", env.Status)
	}
	data := string(env.Data)
	for _, want := range []string{`"managed_entries"`, `"tags"`, `"profiles"`, `"provenance"`, `"source_revision": "abc123"`, `"profiles_source": "recorded"`, `"selection_migration"`, `"ambiguity_reasons"`} {
		if !strings.Contains(data, want) {
			t.Fatalf("installed JSON missing %s\ndata:\n%s", want, data)
		}
	}
	if strings.Contains(out.String(), "Installed inventory") {
		t.Fatalf("JSON mode leaked human prose:\n%s", out.String())
	}
}

func TestInstalledJSONMatchesXDGStateEntryWithCompleteCoverage(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	sourceRoot := t.TempDir()
	xdgStateHome := filepath.Join(home, ".local", "state")
	t.Setenv("XDG_STATE_HOME", xdgStateHome)
	writeCLISource(t, sourceRoot, "configs/nvim/lazy-lock.json", "baseline\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [editor]
entries:
  - source: configs/nvim/lazy-lock.json
    target: nvim/lazy-lock.json
    target_root: xdg-state
    strategy: copy
    ownership: seeded
    tags: [editor]
    os: [darwin, linux]
`)
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{{
		Target:   filepath.Join(xdgStateHome, "nvim", "lazy-lock.json"),
		Source:   "configs/nvim/lazy-lock.json",
		Strategy: "copy",
		Profiles: []string{"default"},
		Tags:     []string{"editor"},
	}}}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"installed", "--output", "json", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	env := decodeEnvelopeForCommand(t, out.String(), "installed")
	var data struct {
		ManagedEntries []struct {
			ManifestMatched bool `json:"manifest_matched"`
		} `json:"managed_entries"`
		Profiles []struct {
			Name           string `json:"name"`
			State          string `json:"state"`
			CoveredEntries int    `json:"covered_entries"`
			TotalEntries   int    `json:"total_entries"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("decode installed data: %v", err)
	}
	if len(data.ManagedEntries) != 1 || !data.ManagedEntries[0].ManifestMatched {
		t.Fatalf("managed entries = %+v, want one manifest match", data.ManagedEntries)
	}
	if len(data.Profiles) != 1 || data.Profiles[0].Name != "default" || data.Profiles[0].State != "complete" || data.Profiles[0].CoveredEntries != 1 || data.Profiles[0].TotalEntries != 1 {
		t.Fatalf("profiles = %+v, want default complete 1/1 coverage", data.Profiles)
	}
}

func TestInstalledTextExplainsPartialProfile(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core")
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: 1, Entries: []state.Record{{
		Target:   filepath.Join(home, ".zshrc"),
		Source:   "configs/zsh/zshrc",
		Strategy: "symlink",
	}}}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"installed", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	for _, want := range []string{"Installed inventory", "Installed Selection: none recorded", "Historical inventory (non-authoritative)", "Managed Entries (1)", "Tags represented: core", "Profiles", "Notes", "inferred-from-manifest", "Selection migration candidate (non-authoritative)", "Confidence:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text output missing %q\noutput:\n%s", want, out.String())
		}
	}
}

func TestInstalledTextShowsAuthoritativeSelectionSeparately(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core")
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: 3,
		InstalledSelection: &state.InstalledSelection{
			Profiles:     []string{"default"},
			ExtraTags:    []string{"agents"},
			ResolvedTags: []string{"core", "agents"},
			Provenance: state.Provenance{
				SourceRoot:     sourceRoot,
				SourceRevision: "abc123",
				DotsVersion:    "v0.test",
				RecordedAt:     "2026-07-23T12:00:00Z",
			},
		},
		Entries: []state.Record{{
			Target:   filepath.Join(home, ".zshrc"),
			Source:   "configs/zsh/zshrc",
			Strategy: "symlink",
			Profiles: []string{"default"},
			Tags:     []string{"core"},
		}},
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"installed", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	for _, want := range []string{
		"Installed Selection (authoritative)",
		"Profiles: default",
		"Extra Tags: agents",
		"Resolved Tags: core, agents",
		"Source of Truth: source=" + sourceRoot + ", commit=abc123, dots=v0.test",
		"Recorded At: 2026-07-23T12:00:00Z",
		"Historical inventory (non-authoritative)",
		"Tags represented: core",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("text output missing %q\noutput:\n%s", want, out.String())
		}
	}
}
