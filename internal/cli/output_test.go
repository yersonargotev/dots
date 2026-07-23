package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
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
	if env.SchemaVersion != "5" {
		t.Fatalf("schema_version = %q, want \"4\"", env.SchemaVersion)
	}
	if env.Command != "status" {
		t.Fatalf("command = %q, want \"status\"", env.Command)
	}
	return env
}

func decodeEnvelopeForCommand(t *testing.T, out string, command string) testEnvelope {
	t.Helper()
	var env testEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\noutput:\n%s", err, out)
	}
	if env.SchemaVersion != "5" {
		t.Fatalf("schema_version = %q, want \"4\"", env.SchemaVersion)
	}
	if env.Command != command {
		t.Fatalf("command = %q, want %q", env.Command, command)
	}
	return env
}

func TestNonDiagnosticCommandsHonorJSONOutput(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core")

	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{
			name:    "version",
			command: "version",
			args:    []string{"version", "--output", "json"},
		},
		{
			name:    "manifest validate",
			command: "manifest validate",
			args:    []string{"manifest", "validate", "--output", "json", "--file", manifestPath},
		},
		{
			name:    "backups list",
			command: "backups list",
			args:    []string{"backups", "list", "--output", "json", "--state-root", stateRoot},
		},
		{
			name:    "install dry run",
			command: "install",
			args: []string{
				"install", "--dry-run", "--output", "json", "--file", manifestPath,
				"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
			},
		},
		{
			name:    "uninstall dry run",
			command: "uninstall",
			args: []string{
				"uninstall", "--dry-run", "--output", "json",
				"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := cli.Run(tt.args, &out, &errOut)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0\nstderr:\n%s\nstdout:\n%s", code, errOut.String(), out.String())
			}
			env := decodeEnvelopeForCommand(t, out.String(), tt.command)
			if env.Status != "ok" {
				t.Fatalf("status = %q, want ok", env.Status)
			}
			if len(env.Data) == 0 || string(env.Data) == "null" {
				t.Fatalf("expected data payload, got %s", env.Data)
			}
			if strings.Contains(out.String(), "manifest is valid") || strings.Contains(out.String(), "Plan for profile") || strings.Contains(out.String(), "No Backup Sets") {
				t.Fatalf("JSON mode leaked human prose:\n%s", out.String())
			}
		})
	}
}

func TestJSONModeRejectsInteractiveActionCommands(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "core")

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--output", "json", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if errOut.Len() != 0 {
		t.Fatalf("Machine Output Mode must keep stderr clean, got:\n%s", errOut.String())
	}
	env := decodeEnvelopeForCommand(t, out.String(), "install")
	if env.Status != "error" {
		t.Fatalf("status = %q, want error", env.Status)
	}
	if !strings.Contains(env.Error, "--yes or --dry-run") {
		t.Fatalf("error = %q, want guidance to use --yes or --dry-run", env.Error)
	}
}

func TestRootVersionFlagHonorsJSONOutput(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"--output", "json", "--version"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr:\n%s", code, errOut.String())
	}
	env := decodeEnvelopeForCommand(t, out.String(), "dots")
	if env.Status != "ok" {
		t.Fatalf("status = %q, want ok", env.Status)
	}
}

func TestStatusJSONEnvelopeAligned(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	manifestPath, sourceRoot := writeStatusManifest(t, home, "other") // unselected: empty report

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"status", "--profile", "default", "--output", "json", "--file", manifestPath,
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
		"status", "--profile", "default", "--output", "json", "--file", manifestPath,
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
		"status", "--profile", "default", "--output", "json", "--file", filepath.Join(t.TempDir(), "missing.yaml"),
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
		"status", "--profile", "default", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}, &out, &errOut)

	if strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Fatalf("default output must be human text, not JSON:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Status for profile") {
		t.Fatalf("default output should be the text report, got:\n%s", out.String())
	}
}

func TestJSONModeRejectsHumanOnlySurfaces(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "root help", command: "dots", args: []string{"--output", "json"}},
		{name: "group help", command: "deps", args: []string{"deps", "--output", "json"}},
		{name: "explicit help", command: "help", args: []string{"help", "deps", "--output", "json"}},
		{name: "completion", command: "completion bash", args: []string{"completion", "bash", "--output", "json"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := cli.Run(tt.args, &out, &errOut)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
			}
			if errOut.Len() != 0 {
				t.Fatalf("Machine Output Mode must keep stderr clean, got:\n%s", errOut.String())
			}
			env := decodeEnvelopeForCommand(t, out.String(), tt.command)
			if env.Status != "error" {
				t.Fatalf("status = %q, want error", env.Status)
			}
			if strings.Contains(out.String(), "Usage:") || strings.Contains(out.String(), "bash completion") {
				t.Fatalf("JSON mode leaked human-only output:\n%s", out.String())
			}
		})
	}
}

func TestJSONModeRejectsHelpFlags(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{name: "root long help flag", command: "dots", args: []string{"--output", "json", "--help"}},
		{name: "root long help flag with explicit true", command: "dots", args: []string{"--output", "json", "--help=true"}},
		{name: "simple subcommand long help flag", command: "version", args: []string{"version", "--output", "json", "--help"}},
		{name: "later JSON output wins before help preflight", command: "version", args: []string{"--output", "text", "version", "--output", "json", "--help"}},
		{name: "simple subcommand long help flag with explicit true", command: "version", args: []string{"version", "--output", "json", "--help=true"}},
		{name: "nested subcommand long help flag", command: "manifest validate", args: []string{"manifest", "validate", "--output", "json", "--help"}},
		{name: "nested subcommand long help flag with explicit true", command: "manifest validate", args: []string{"manifest", "validate", "--output", "json", "--help=true"}},
		{name: "nested subcommand flag value before help", command: "manifest validate", args: []string{"manifest", "validate", "--file", "dots.yaml", "--output", "json", "--help"}},
		{name: "root short help flag", command: "dots", args: []string{"--output", "json", "-h"}},
		{name: "root short help flag with explicit true", command: "dots", args: []string{"--output", "json", "-h=true"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := cli.Run(tt.args, &out, &errOut)
			if code != 1 {
				t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
			}
			if errOut.Len() != 0 {
				t.Fatalf("Machine Output Mode must keep stderr clean, got:\n%s", errOut.String())
			}
			env := decodeEnvelopeForCommand(t, out.String(), tt.command)
			if env.Status != "error" {
				t.Fatalf("status = %q, want error", env.Status)
			}
			if !strings.Contains(env.Error, "--output json is not supported") {
				t.Fatalf("error = %q, want unsupported JSON help guidance", env.Error)
			}
			if strings.Contains(out.String(), "Usage:") || strings.Contains(out.String(), "Dotfiles CLI") || strings.Contains(out.String(), "Print the dots version") {
				t.Fatalf("JSON mode leaked help prose:\n%s", out.String())
			}
		})
	}
}

func TestTextHelpRemainsAvailable(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"manifest", "validate", "--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("text help should not emit stderr, got:\n%s", errOut.String())
	}
	if !strings.Contains(out.String(), "Usage:") || !strings.Contains(out.String(), "Validate a dots.yaml manifest") {
		t.Fatalf("expected text help, got:\n%s", out.String())
	}
	if strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
		t.Fatalf("text help must not be JSON:\n%s", out.String())
	}
}

func TestGroupCommandRejectsInvalidOutputValue(t *testing.T) {
	var out, errOut bytes.Buffer
	code := cli.Run([]string{"deps", "--output", "yaml"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "invalid --output") {
		t.Fatalf("stderr should explain the invalid output value, got:\n%s", errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("invalid output value must not emit stdout, got:\n%s", out.String())
	}
}

func TestManifestValidateRejectsExtraArgsInJSONMode(t *testing.T) {
	home := t.TempDir()
	manifestPath, _ := writeStatusManifest(t, home, "core")

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"manifest", "validate", "--output", "json", "--file", manifestPath, "bogus"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("Machine Output Mode must keep stderr clean, got:\n%s", errOut.String())
	}
	env := decodeEnvelopeForCommand(t, out.String(), "manifest validate")
	if env.Status != "error" {
		t.Fatalf("status = %q, want error", env.Status)
	}
}

func TestAdditionalActionCommandsHonorJSONOutput(t *testing.T) {
	t.Run("deps install dry run", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		manifestPath := writeCLIManifest(t, t.TempDir(), `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-starship
        command: definitely-missing-starship-probe
        brew: starship
`)

		var out, errOut bytes.Buffer
		code := cli.Run([]string{"deps", "install", "--dry-run", "--output", "json", "--file", manifestPath, "--tier", "homebrew"}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
		}
		env := decodeEnvelopeForCommand(t, out.String(), "deps install")
		if env.Status != "ok" {
			t.Fatalf("status = %q, want ok", env.Status)
		}
		if strings.Contains(out.String(), "Dependency install preview") {
			t.Fatalf("JSON mode leaked deps prose:\n%s", out.String())
		}
		data := string(env.Data)
		for _, want := range []string{`"candidates"`, `"provider": "homebrew"`, `"status": "manual"`} {
			if !strings.Contains(data, want) {
				t.Fatalf("deps install JSON missing provider-aware field %q\ndata:\n%s", want, data)
			}
		}
		if strings.Contains(data, `"available"`) {
			t.Fatalf("deps install JSON leaked provider availability\ndata:\n%s", data)
		}
	})

	t.Run("backups restore dry run", func(t *testing.T) {
		home := t.TempDir()
		stateRoot := t.TempDir()
		set := newPreservedSet(t, home, stateRoot, ".zshrc", "original\n", "")
		if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("drifted\n"), 0o600); err != nil {
			t.Fatalf("drift target: %v", err)
		}

		var out, errOut bytes.Buffer
		code := cli.Run([]string{"backups", "restore", set.ID, "--dry-run", "--output", "json", "--home", home, "--state-root", stateRoot}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
		}
		env := decodeEnvelopeForCommand(t, out.String(), "backups restore")
		if env.Status != "ok" {
			t.Fatalf("status = %q, want ok", env.Status)
		}
		if strings.Contains(string(env.Data), "backup_file") || strings.Contains(string(env.Data), stateRoot) {
			t.Fatalf("restore JSON leaked internal backup storage details:\n%s", env.Data)
		}
	})

	t.Run("update dry run", func(t *testing.T) {
		requireGitCLI(t)
		home := t.TempDir()
		t.Setenv("HOME", t.TempDir())
		origin, sourceRoot := newInstalledRepo(t, map[string]string{
			"configs/zsh/zshrc": "export A=1\n",
			"dots.yaml": `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
		})
		advanceUpstream(t, origin, "add tmux config", map[string]string{
			"configs/tmux/tmux.conf": "set -g mouse on\n",
		})

		var out, errOut bytes.Buffer
		code := cli.Run([]string{"update", "--dry-run", "--output", "json", "--file", filepath.Join(sourceRoot, "dots.yaml"), "--home", home, "--source-root", sourceRoot}, &out, &errOut)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, out.String(), errOut.String())
		}
		env := decodeEnvelopeForCommand(t, out.String(), "update")
		if env.Status != "ok" {
			t.Fatalf("status = %q, want ok", env.Status)
		}
		if strings.Contains(out.String(), "Plan for profile") {
			t.Fatalf("JSON mode leaked update prose:\n%s", out.String())
		}
	})
}
