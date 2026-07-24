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

func TestLegacyMigrationAmbiguousAndNoEvidenceStayNonAuthoritative(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*testing.T, string, string, string) state.Metadata
		wantReason string
	}{
		{
			name: "ambiguous profile coverage",
			setup: func(t *testing.T, home, manifestPath, sourceRoot string) state.Metadata {
				content, err := os.ReadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				content = []byte(strings.Replace(string(content), "profiles:\n  default:", "profiles:\n  alternate:\n    tags: [core]\n  default:", 1))
				if err := os.WriteFile(manifestPath, content, 0o600); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(home, ".zshrc")
				if err := os.Symlink(filepath.Join(sourceRoot, "configs/zsh/zshrc"), target); err != nil {
					t.Fatal(err)
				}
				return state.Metadata{Version: 2, Entries: []state.Record{{
					Target: target, Source: "configs/zsh/zshrc", Strategy: "symlink", Tags: []string{"core"},
				}}}
			},
			wantReason: "multiple_complete_profiles",
		},
		{
			name:       "no historical evidence",
			setup:      func(*testing.T, string, string, string) state.Metadata { return state.Metadata{Version: 2} },
			wantReason: "no_historical_evidence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			manifestPath, sourceRoot := writeStatusManifest(t, home, "core")
			stateRoot := filepath.Join(home, ".local", "state", "dots")
			before := tt.setup(t, home, manifestPath, sourceRoot)
			if err := state.Save(state.Path(stateRoot), before); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			code := cli.Run([]string{
				"status", "--output", "json", "--file", manifestPath,
				"--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
			}, &stdout, &stderr)
			if code != cli.ExitError {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitError, stdout.String(), stderr.String())
			}
			env := decodeEnvelopeForCommand(t, stdout.String(), "status")
			var data struct {
				Candidate struct {
					AmbiguityReasons []string `json:"ambiguity_reasons"`
				} `json:"candidate"`
			}
			if err := json.Unmarshal(env.Data, &data); err != nil {
				t.Fatalf("decode migration data: %v\n%s", err, env.Data)
			}
			if !containsString(data.Candidate.AmbiguityReasons, tt.wantReason) {
				t.Fatalf("ambiguity reasons = %#v, want %q", data.Candidate.AmbiguityReasons, tt.wantReason)
			}
			after, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if after.Version != 2 || after.InstalledSelection != nil || !reflect.DeepEqual(after.Entries, before.Entries) {
				t.Fatalf("read-only migration changed metadata: %#v", after)
			}
		})
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
