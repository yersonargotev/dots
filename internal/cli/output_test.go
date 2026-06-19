package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

// testEnvelope mirrors the agent output envelope for assertions.
type testEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	Command       string          `json:"command"`
	Status        string          `json:"status"`
	Data          json.RawMessage `json:"data"`
	Error         string          `json:"error"`
}

func decodeEnvelope(t *testing.T, out string) testEnvelope {
	t.Helper()
	var env testEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\noutput:\n%s", err, out)
	}
	if env.SchemaVersion != "1" {
		t.Fatalf("schema_version = %q, want \"1\"", env.SchemaVersion)
	}
	if env.Command != "status" {
		t.Fatalf("command = %q, want \"status\"", env.Command)
	}
	return env
}

func TestStatusJSONEnvelopeAligned(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "other") // unselected: empty report

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"status", "--output", "json", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr:\n%s", code, errOut.String())
	}
	if env := decodeEnvelope(t, out.String()); env.Status != "ok" {
		t.Fatalf("status = %q, want \"ok\"", env.Status)
	}
	if errOut.Len() != 0 {
		t.Fatalf("Machine Output Mode must keep stderr clean, got:\n%s", errOut.String())
	}
}

func TestStatusJSONEnvelopeFindings(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core") // selected but not installed: missing

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"status", "--output", "json", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2\nstdout:\n%s", code, out.String())
	}
	env := decodeEnvelope(t, out.String())
	if env.Status != "findings" {
		t.Fatalf("status = %q, want \"findings\"", env.Status)
	}
	// The data is the domain report with snake_case fields; the missing entry
	// must be visible so an agent can act on it without parsing prose.
	if data := string(env.Data); !strings.Contains(data, `"state": "missing"`) {
		t.Fatalf("data missing the divergent entry state\ndata:\n%s", data)
	}
}

func TestJSONErrorEnvelopeOnStdout(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"status", "--output", "json", "--file", filepath.Join(t.TempDir(), "missing.yaml"),
	}, &out, &errOut)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if errOut.Len() != 0 {
		t.Fatalf("Machine Output Mode must not write the human error to stderr, got:\n%s", errOut.String())
	}
	env := decodeEnvelope(t, out.String())
	if env.Status != "error" {
		t.Fatalf("status = %q, want \"error\"", env.Status)
	}
	if env.Error == "" {
		t.Fatal("error envelope must carry a non-empty error message")
	}
}

func TestInvalidOutputValueIsRejected(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"status", "--output", "yaml", "--file", "dots.yaml"}, &out, &errOut)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "invalid --output") {
		t.Fatalf("stderr should explain the invalid output value, got:\n%s", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("a rejected flag must not emit an envelope, got:\n%s", out.String())
	}
}

func TestTextModeRemainsDefault(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core")

	var out, errOut bytes.Buffer
	cli.Run([]string{
		"status", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Fatalf("default output must be human text, not JSON:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Status for profile") {
		t.Fatalf("default output should be the text report, got:\n%s", out.String())
	}
}
