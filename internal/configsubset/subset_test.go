package configsubset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeJSONFilesComposesSourcesInManifestOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	third := filepath.Join(dir, "third.json")
	for path, content := range map[string]string{
		first:  `{"permissions":{"allow":["Read"],"mode":"default"},"runtimeCounter":9007199254740993}`,
		second: `{"permissions":{"allow":["Bash(git *)"],"mode":"default"},"hooks":{"PostToolUse":[]}}`,
		third:  `{"permissions":{"allow":["Read","Bash(go test *)"]}}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	got, err := ComposeJSONFiles([]string{first, second, third})
	if err != nil {
		t.Fatalf("ComposeJSONFiles() error = %v", err)
	}
	want := `{
  "hooks": {
    "PostToolUse": []
  },
  "permissions": {
    "allow": [
      "Read",
      "Bash(git *)",
      "Bash(go test *)"
    ],
    "mode": "default"
  },
  "runtimeCounter": 9007199254740993
}
`
	if string(got) != want {
		t.Fatalf("ComposeJSONFiles() =\n%s\nwant:\n%s", got, want)
	}

	var decoded map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(got)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if gotNumber := decoded["runtimeCounter"]; gotNumber != json.Number("9007199254740993") {
		t.Fatalf("runtimeCounter = %#v, want preserved json.Number", gotNumber)
	}
}

func TestComposeJSONFilesRejectsIncompatibleOverlap(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	if err := os.WriteFile(first, []byte(`{"permissions":{"mode":"default"}}`), 0o600); err != nil {
		t.Fatalf("write first: %v", err)
	}
	if err := os.WriteFile(second, []byte(`{"permissions":{"mode":{"name":"default"}}}`), 0o600); err != nil {
		t.Fatalf("write second: %v", err)
	}

	_, err := ComposeJSONFiles([]string{first, second})
	if err == nil {
		t.Fatal("ComposeJSONFiles() error = nil, want incompatible overlap")
	}
	if !strings.Contains(err.Error(), second) {
		t.Fatalf("error %q does not contain conflicting source path %q", err, second)
	}
}

func TestComposeJSONFilesReportsInvalidSourcePath(t *testing.T) {
	dir := t.TempDir()
	invalid := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"permissions":`), 0o600); err != nil {
		t.Fatalf("write invalid source: %v", err)
	}

	_, err := ComposeJSONFiles([]string{invalid})
	if err == nil {
		t.Fatal("ComposeJSONFiles() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), invalid) {
		t.Fatalf("error %q does not contain invalid source path %q", err, invalid)
	}
}

func TestAnalyzeAndMergeJSONContent(t *testing.T) {
	target := []byte(`{"permissions":{"allow":["Read"]},"runtimeCounter":9007199254740993}`)
	source := []byte(`{"permissions":{"allow":["Read","Bash(git *)"]}}`)

	relation, err := AnalyzeJSON(target, source)
	if err != nil {
		t.Fatalf("AnalyzeJSON() error = %v", err)
	}
	if !relation.Mergeable || relation.Contains {
		t.Fatalf("AnalyzeJSON() = %#v, want mergeable only", relation)
	}

	got, err := MergeJSON(target, source)
	if err != nil {
		t.Fatalf("MergeJSON() error = %v", err)
	}
	want := `{
  "permissions": {
    "allow": [
      "Read",
      "Bash(git *)"
    ]
  },
  "runtimeCounter": 9007199254740993
}
`
	if string(got) != want {
		t.Fatalf("MergeJSON() =\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeJSONFileAddsMissingValuesAndPreservesTargetState(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.json")
	target := filepath.Join(dir, "target.json")

	if err := os.WriteFile(source, []byte(`{
  "permissions": {
    "defaultMode": "bypassPermissions",
    "allow": ["Read", "Bash(git *)"]
  }
}`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte(`{
  "permissions": {
    "allow": ["Read", "Bash(go test *)"],
    "deny": ["Bash(rm -rf *)"]
  },
  "hooks": {
    "PostToolUse": []
  },
  "runtimeCounter": 9007199254740993
}`), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if err := MergeJSONFile(target, source); err != nil {
		t.Fatalf("MergeJSONFile() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	wantJSON := `{
  "hooks": {
    "PostToolUse": []
  },
  "permissions": {
    "allow": [
      "Read",
      "Bash(go test *)",
      "Bash(git *)"
    ],
    "defaultMode": "bypassPermissions",
    "deny": [
      "Bash(rm -rf *)"
    ]
  },
  "runtimeCounter": 9007199254740993
}
`
	if string(got) != wantJSON {
		t.Fatalf("merged JSON is not deterministic\ngot:\n%s\nwant:\n%s", got, wantJSON)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("target mode = %v, want 0640", gotMode)
	}
}

func TestMergeJSONFileRejectsIncompatibleValuesWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		source string
		target string
	}{
		{
			name:   "changed scalar",
			source: `{"permissions":{"defaultMode":"bypassPermissions"}}`,
			target: `{"permissions":{"defaultMode":"default"},"hooks":{}}`,
		},
		{
			name:   "object type mismatch",
			source: `{"permissions":{"allow":["Read"]}}`,
			target: `{"permissions":"managed elsewhere","hooks":{}}`,
		},
		{
			name:   "array type mismatch",
			source: `{"permissions":{"allow":["Read"]}}`,
			target: `{"permissions":{"allow":"Read"},"hooks":{}}`,
		},
		{
			name:   "distinct large numbers",
			source: `{"runtimeCounter":9007199254740993}`,
			target: `{"runtimeCounter":9007199254740992,"hooks":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.json")
			target := filepath.Join(dir, "target.json")
			if err := os.WriteFile(source, []byte(tt.source), 0o600); err != nil {
				t.Fatalf("write source: %v", err)
			}
			if err := os.WriteFile(target, []byte(tt.target), 0o600); err != nil {
				t.Fatalf("write target: %v", err)
			}

			if err := MergeJSONFile(target, source); err == nil {
				t.Fatal("MergeJSONFile() error = nil, want incompatible-value error")
			}
			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("read target: %v", err)
			}
			if string(got) != tt.target {
				t.Fatalf("target changed after rejected merge\ngot:  %s\nwant: %s", got, tt.target)
			}
		})
	}
}

func TestTOMLFileContains(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	if err := os.WriteFile(source, []byte("[tui]\nstatus_line = [\"model\", \"git-branch\"]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("model = \"gpt-5.5\"\n\n[tui]\nstatus_line = [\"model\", \"git-branch\"]\ntheme = \"catppuccin\"\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	got, err := TOMLFileContains(target, source)
	if err != nil {
		t.Fatalf("TOMLFileContains() error = %v", err)
	}
	if !got {
		t.Fatal("TOMLFileContains() = false, want true")
	}
}

func TestTOMLFileContainsRejectsChangedDotsOwnedValue(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	if err := os.WriteFile(source, []byte("[tui]\nstatus_line = [\"model\", \"git-branch\"]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("[tui]\nstatus_line = [\"model\"]\n"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	got, err := TOMLFileContains(target, source)
	if err != nil {
		t.Fatalf("TOMLFileContains() error = %v", err)
	}
	if got {
		t.Fatal("TOMLFileContains() = true, want false")
	}
}

func TestTOMLFileContainsIgnoresUnsupportedUnownedTargetTOML(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	if err := os.WriteFile(source, []byte("[tui]\nstatus_line = [\"model\", \"git-branch\"]\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte(`[[mcp_servers]]
name = "chrome-devtools"
command = "npx"
args = [
  "-y",
  "chrome-devtools-mcp@latest",
]

[tui]
status_line = ["model", "git-branch"]
`), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	got, err := TOMLFileContains(target, source)
	if err != nil {
		t.Fatalf("TOMLFileContains() error = %v", err)
	}
	if !got {
		t.Fatal("TOMLFileContains() = false, want true")
	}
}

func TestTOMLFileContainsSupportsSourceArrayOfTables(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	hook := `[[hooks.SessionStart]]
matcher = "startup|resume"

[[hooks.SessionStart.hooks]]
type = "command"
command = "codegraph init"
timeout = 120
`
	if err := os.WriteFile(source, []byte(hook), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte("model = \"gpt-5.5\"\n\n"+hook), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	got, err := TOMLFileContains(target, source)
	if err != nil {
		t.Fatalf("TOMLFileContains() error = %v", err)
	}
	if !got {
		t.Fatal("TOMLFileContains() = false, want true")
	}
}

func TestMergeTOMLFileAppendsMissingArrayTableBlocks(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.toml")
	target := filepath.Join(dir, "target.toml")

	if err := os.WriteFile(source, []byte(`sandbox_mode = "danger-full-access"
approval_policy = "never"

[[hooks.SessionStart]]
matcher = "startup|resume"

[[hooks.SessionStart.hooks]]
type = "command"
command = "codegraph init"
timeout = 120
statusMessage = "Initializing CodeGraph index"
`), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(target, []byte(`model = "gpt-5.5"
sandbox_mode = "danger-full-access"
approval_policy = "never"

[tui]
theme = "catppuccin-mocha"
`), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}

	if err := MergeTOMLFile(target, source); err != nil {
		t.Fatalf("MergeTOMLFile() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	for _, want := range []string{
		`model = "gpt-5.5"`,
		`[tui]`,
		`[[hooks.SessionStart]]`,
		`command = "codegraph init"`,
	} {
		if !strings.Contains(string(got), want) {
			t.Fatalf("merged target missing %q\ncontent:\n%s", want, got)
		}
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("target mode = %v, want 0640", gotMode)
	}
}
