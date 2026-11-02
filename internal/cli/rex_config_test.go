package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/manifest"
	"gopkg.in/yaml.v3"
)

func TestRexConfigurationInstallsWithoutRexAndPreservesConflicts(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// Isolate the app probe as well as PATH: a real /Applications/Rex Beta.app must
	// not make this missing-application scenario depend on the operator's machine.
	// The manifest test checks the unmodified DarwinApp declaration independently.
	m, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range m.Entries {
		for j := range m.Entries[i].Dependencies {
			if m.Entries[i].Dependencies[j].Name == "rex" {
				m.Entries[i].Dependencies[j].DarwinApp = ""
			}
		}
	}
	fixture, err := yaml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(t.TempDir(), "dots.yaml")
	if err := os.WriteFile(manifestPath, fixture, 0600); err != nil {
		t.Fatal(err)
	}
	// Restrict PATH entirely: neither Rex nor any package/network executable can run.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, t.TempDir())
	}
	for _, conflict := range []bool{false, true} {
		name := "fresh"
		if conflict {
			name = "foreign-target"
		}
		t.Run(name, func(t *testing.T) {
			home, state := t.TempDir(), t.TempDir()
			target := filepath.Join(home, ".config/rex/init.lua")
			if conflict {
				if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte("-- operator config\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			run := func(command []string, wantCode int) testEnvelope {
				t.Helper()
				args := append(command, "--file", manifestPath, "--tag", "rex", "--home", home, "--output", "json")
				var out, errOut bytes.Buffer
				code := cli.Run(args, &out, &errOut)
				if code != wantCode {
					t.Fatalf("%v exit = %d, want %d\n%s\n%s", args, code, wantCode, out.String(), errOut.String())
				}
				var env testEnvelope
				if err := json.Unmarshal(out.Bytes(), &env); err != nil {
					t.Fatalf("invalid JSON: %v\n%s", err, out.String())
				}
				return env
			}
			check := run([]string{"deps", "check"}, 2)
			var checked struct {
				Results []struct {
					Name    string
					Present bool
				}
			}
			if err := json.Unmarshal(check.Data, &checked); err != nil {
				t.Fatal(err)
			}
			if len(checked.Results) != 1 || checked.Results[0].Name != "rex" || checked.Results[0].Present {
				t.Fatalf("Rex must be missing: %s", check.Data)
			}
			preview := run([]string{"deps", "plan", "--tier", "generic"}, 2)
			if !strings.Contains(string(preview.Data), `"manual"`) || !strings.Contains(string(preview.Data), "does not install Rex") {
				t.Fatalf("Rex must have manual guidance: %s", preview.Data)
			}
			install := []string{"install", "--yes", "--no-tui", "--source-root", root, "--state-root", state}
			run(install, 0)
			// A second install neither promotes a foreign target nor changes ownership.
			run(install, 0)
			for _, rel := range []string{"init.lua", "themes/Catppuccin Mocha Blue.ghostty"} {
				dst := filepath.Join(home, ".config/rex", rel)
				info, err := os.Lstat(dst)
				if err != nil {
					t.Fatal(err)
				}
				if !info.Mode().IsRegular() {
					t.Fatalf("%s must be a regular copy: %v", rel, info.Mode())
				}
				got, err := os.ReadFile(dst)
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(filepath.Join(root, "configs/rex", rel))
				if err != nil {
					t.Fatal(err)
				}
				if conflict && rel == "init.lua" {
					want = []byte("-- operator config\n")
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("%s does not preserve expected bytes", rel)
				}
			}
			if !conflict {
				run([]string{"status", "--source-root", root, "--state-root", state}, 0)
				original, err := os.ReadFile(filepath.Join(root, "configs/rex/init.lua"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte("-- client edit\n"), 0600); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 2; i++ {
					env := run([]string{"status", "--source-root", root, "--state-root", state}, 2)
					if !strings.Contains(string(env.Data), `"drifted"`) {
						t.Fatalf("edited copy must report Drift: %s", env.Data)
					}
				}
				source, err := os.ReadFile(filepath.Join(root, "configs/rex/init.lua"))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(source, original) {
					t.Fatal("client edit changed Source of Truth")
				}
			}
		})
	}
}
