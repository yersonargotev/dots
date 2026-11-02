package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestClaudeAutoModePreservesCoOwnedSettings(t *testing.T) {
	for _, tags := range [][]string{{"claude"}, {"claude", "theme-carbonfox"}} {
		t.Run(tags[len(tags)-1], func(t *testing.T) {
			f := newCarbonfoxFixture(t, "linux")
			f.install(t, false, tags...)
			path := filepath.Join(f.home, ".claude/settings.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var settings map[string]any
			if err := json.Unmarshal(data, &settings); err != nil {
				t.Fatal(err)
			}
			settings["runtimeOnly"] = "keep"
			permissions := settings["permissions"].(map[string]any)
			permissions["ask"] = []string{"Bash(git push *)"}
			permissions["allow"] = []any{"Read(~/notes/**)"}
			data, err = json.Marshal(settings)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			f.install(t, false, tags...)
			data, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var installed struct {
				RuntimeOnly string
				Permissions struct {
					DefaultMode string
					Ask, Allow  []string
				}
			}
			if err := json.Unmarshal(data, &installed); err != nil {
				t.Fatal(err)
			}
			if installed.Permissions.DefaultMode != "auto" {
				t.Fatalf("permission mode = %q, want auto", installed.Permissions.DefaultMode)
			}
			if installed.RuntimeOnly != "keep" || len(installed.Permissions.Ask) != 1 || installed.Permissions.Ask[0] != "Bash(git push *)" || installed.Permissions.Allow[len(installed.Permissions.Allow)-1] != "Read(~/notes/**)" {
				t.Fatalf("co-owned settings lost: %s", data)
			}
			f.run(t, ExitOK, "status", "--file", f.manifestPath, "--source-root", f.repositoryRoot, "--home", f.home, "--state-root", f.stateRoot, "--output", "json")
		})
	}
}
