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

func TestLegacyMetadataBlocksReadOnlyCommandsWithMigrationCandidate(t *testing.T) {
	home := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core")
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: 1,
		Entries: []state.Record{{
			Target: filepath.Join(home, ".zshrc"), Source: "configs/zsh/zshrc",
			Strategy: "symlink", Profiles: []string{"default"}, Tags: []string{"core"},
		}},
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	commands := []struct {
		name string
		args []string
	}{
		{"status", []string{"status", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}},
		{"doctor", []string{"doctor", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}},
		{"plan", []string{"plan", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}},
		{"deps check", []string{"deps", "check", "--file", manifestPath, "--home", home}},
		{"deps plan", []string{"deps", "plan", "--file", manifestPath, "--home", home, "--tier", "generic"}},
	}
	for _, tt := range commands {
		t.Run(tt.name, func(t *testing.T) {
			args := append(append([]string{}, tt.args...), "--output", "json")
			var stdout, stderr bytes.Buffer
			if code := cli.Run(args, &stdout, &stderr); code != 1 {
				t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
			}
			env := decodeEnvelopeForCommand(t, stdout.String(), tt.name)
			if env.Status != "error" || !strings.Contains(env.Error, "selection-migration-required") {
				t.Fatalf("envelope = status %q error %q", env.Status, env.Error)
			}
			var data struct {
				Code      string `json:"code"`
				Candidate struct {
					Profiles         []string `json:"profiles"`
					ExtraTags        []string `json:"extra_tags"`
					EffectiveTags    []string `json:"effective_tags"`
					AmbiguityReasons []string `json:"ambiguity_reasons"`
				} `json:"candidate"`
				Remediation json.RawMessage `json:"remediation"`
			}
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatalf("decode migration data: %v\ndata:\n%s", err, env.Data)
			}
			if data.Code != "selection-migration-required" {
				t.Fatalf("code = %q", data.Code)
			}
			if data.Candidate.Profiles == nil || data.Candidate.ExtraTags == nil || data.Candidate.EffectiveTags == nil || data.Candidate.AmbiguityReasons == nil {
				t.Fatalf("candidate arrays must be stable empty arrays: %#v", data.Candidate)
			}
			if data.Remediation == nil {
				t.Fatal("remediation missing")
			}
		})
	}
}
