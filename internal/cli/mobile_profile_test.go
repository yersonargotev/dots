package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

func TestMobileProfileRunsOnlyCodexAndClaudeProvidersInSandbox(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home, stateRoot := t.TempDir(), t.TempDir()
	log := stubMobileProviders(t)
	runMobileInstall(t, filepath.Join(repoRoot, "dots.yaml"), repoRoot, home, stateRoot)
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"npx --yes skills@1.5.12 add dart-lang/skills --agent codex --agent claude-code --global",
		"npx --yes skills@1.5.12 add flutter/skills --agent codex --agent claude-code --global",
		"npx --yes skills@1.5.12 add android/skills --agent codex --agent claude-code --skill android-cli --global",
		"claude mcp add --transport stdio dart -- dart mcp-server",
		"codex mcp add dart -- dart mcp-server --force-roots-fallback",
	}
	if got := strings.Split(strings.TrimSpace(string(data)), "\n"); !reflect.DeepEqual(got, want) {
		t.Fatalf("provider commands = %#v, want %#v", got, want)
	}
	vscode := filepath.Join(home, ".config", "Code", "User", "settings.json")
	if runtime.GOOS == "darwin" {
		vscode = filepath.Join(home, "Library", "Application Support", "Code", "User", "settings.json")
	}
	if data, err := os.ReadFile(vscode); err != nil || !strings.Contains(string(data), `"dart.mcpServer": true`) {
		t.Fatalf("VS Code mobile settings = %s, %v", data, err)
	}
	for _, dir := range []string{".gemini", ".copilot", ".config/opencode"} {
		if _, err := os.Stat(filepath.Join(home, dir)); !os.IsNotExist(err) {
			t.Fatalf("unexpected mobile integration %s: %v", dir, err)
		}
	}
}

func TestMobileManifestEvolutionPreservesHistoricalContributionsAndReceipts(t *testing.T) {
	for _, withBaseline := range []bool{false, true} {
		name := "mobile only"
		if withBaseline {
			name = "shared Antigravity baseline"
		}
		t.Run(name, func(t *testing.T) {
			repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
			if err != nil {
				t.Fatal(err)
			}
			home, stateRoot, sourceRoot := t.TempDir(), t.TempDir(), t.TempDir()
			stubMobileProviders(t)
			for _, source := range []string{"configs/vscode/settings.json"} {
				data, err := os.ReadFile(filepath.Join(repoRoot, source))
				if err != nil {
					t.Fatal(err)
				}
				writeCLISource(t, sourceRoot, source, string(data))
			}
			const retiredSource = "configs/antigravity/mobile-mcp-settings.json"
			writeCLISource(t, sourceRoot, retiredSource, `{"mcpServers":{"dart-mcp-server":{"command":"dart","args":["mcp-server"]}}}`)
			current, err := os.ReadFile(filepath.Join(repoRoot, "dots.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			legacy := strings.ReplaceAll(string(current), "      - codex-dart-mcp\n", "      - codex-dart-mcp\n      - antigravity-dart-mcp\n")
			legacy = strings.Replace(legacy, "  vscode-mobile:\n", "  antigravity-dart-mcp:\n    description: Historical Antigravity Dart MCP.\n    kind: surface\n    status: current\n  vscode-mobile:\n", 1)
			legacy = strings.Replace(legacy, "entries:\n", `entries:
  - source: configs/antigravity/mobile-mcp-settings.json
    target: ~/.gemini/antigravity-cli/settings.json
    strategy: copy
    ownership: json-subset
    tags: [antigravity-dart-mcp]
    os: [darwin, linux]
`, 1)
			for _, pkg := range []string{"dart-lang/skills", "flutter/skills", "android/skills"} {
				needle := "      package: " + pkg + "\n      agents:\n        - codex\n        - claude-code\n"
				legacy = strings.Replace(legacy, needle, needle+"        - antigravity\n        - opencode\n        - github-copilot\n", 1)
			}
			if withBaseline {
				writeCLISource(t, sourceRoot, "configs/antigravity/settings.json", `{"toolPermission":"always-proceed","allowNonWorkspaceAccess":true}`)
				legacy = strings.Replace(legacy, "  vscode-mobile:\n", "  antigravity:\n    kind: surface\n    status: current\n  vscode-mobile:\n", 1)
				legacy = strings.ReplaceAll(legacy, "      - vscode-mobile\n", "      - vscode-mobile\n      - antigravity\n")
				legacy = strings.Replace(legacy, "entries:\n", `entries:
  - source: configs/antigravity/settings.json
    target: ~/.gemini/antigravity-cli/settings.json
    strategy: copy
    ownership: json-subset
    tags: [antigravity]
`, 1)
			}
			legacyPath := writeCLIManifest(t, t.TempDir(), legacy)
			runMobileInstall(t, legacyPath, sourceRoot, home, stateRoot)
			before, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(home, ".gemini", "antigravity-cli", "settings.json")
			// Preserve even externally changed former owned values and target-only keys.
			data, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.Replace(data, []byte(`"dart"`), []byte(`"externally-edited-dart"`), 1)
			data = bytes.Replace(data, []byte("{"), []byte("{\n  \"runtimeOnly\": \"keep\","), 1)
			if err := os.WriteFile(target, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(sourceRoot, retiredSource)); err != nil {
				t.Fatal(err)
			}
			var previousReceipts int
			for attempt := 0; attempt < 2; attempt++ {
				runMobileInstall(t, filepath.Join(repoRoot, "dots.yaml"), sourceRoot, home, stateRoot)
				if got, err := os.ReadFile(target); err != nil || !bytes.Equal(got, data) {
					t.Fatalf("evolution changed retired/external bytes: %s, %v; want %s", got, err, data)
				}
				after, err := state.Load(state.Path(stateRoot))
				if err != nil {
					t.Fatal(err)
				}
				oldRecord, _ := before.FindByTarget(target)
				newRecord, ok := after.FindByTarget(target)
				if !ok || !reflect.DeepEqual(oldRecord.Contributions, newRecord.Contributions) {
					t.Fatalf("evolution lost historical contributions: %+v", newRecord)
				}
				oldSkills, currentSkills := 0, 0
				for _, receipt := range after.Provisioners {
					if receipt.Tool != "skills" {
						continue
					}
					if strings.Contains(strings.Join(receipt.Args, " "), "--agent antigravity") {
						oldSkills++
						found := false
						for _, old := range before.Provisioners {
							if reflect.DeepEqual(old, receipt) {
								found = true
							}
						}
						if !found {
							t.Fatalf("legacy receipt changed: %+v", receipt)
						}
					} else {
						currentSkills++
					}
				}
				if oldSkills != 3 || currentSkills != 3 || (attempt > 0 && len(after.Provisioners) != previousReceipts) {
					t.Fatalf("receipt counts old=%d current=%d total=%d, previous=%d", oldSkills, currentSkills, len(after.Provisioners), previousReceipts)
				}
				previousReceipts = len(after.Provisioners)
			}
		})
	}
}

func stubMobileProviders(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	stubDir := t.TempDir()
	writeManifestDependencyStubs(t, stubDir)
	log := filepath.Join(t.TempDir(), "providers.log")
	t.Setenv("MOBILE_PROVIDER_LOG", log)
	for _, name := range []string{"npx", "claude", "codex"} {
		writeExecStub(t, filepath.Join(stubDir, name), "#!/bin/sh\nprintf '%s' '"+name+"' >> \"$MOBILE_PROVIDER_LOG\"\nprintf ' %s' \"$@\" >> \"$MOBILE_PROVIDER_LOG\"\nprintf '\\n' >> \"$MOBILE_PROVIDER_LOG\"\n")
	}
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func runMobileInstall(t *testing.T, manifestPath, sourceRoot, home, stateRoot string, extra ...string) {
	t.Helper()
	args := []string{"install", "--yes", "--skip-deps", "--profile", "mobile", "--file", manifestPath, "--source-root", sourceRoot, "--home", home, "--state-root", stateRoot}
	var out, errOut bytes.Buffer
	if code := cli.Run(append(args, extra...), &out, &errOut); code != cli.ExitOK {
		t.Fatalf("mobile install exited %d: %s\n%s", code, out.String(), errOut.String())
	}
}
