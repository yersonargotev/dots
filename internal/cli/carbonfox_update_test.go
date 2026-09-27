package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

const carbonfoxUpdateManifest = `version: 1
profiles:
  core:
    tags: [app]
entries:
  - source: configs/app
    target: ~/.app
    strategy: copy
    tags: [app]
  - source: configs/marker
    target: ~/.config/dots/theme-carbonfox
    strategy: symlink
    tags: [theme-carbonfox]
`

func TestCarbonfoxUpdatePreferenceChangeStopsBeforeCheckout(t *testing.T) {
	for _, mode := range []string{"add", "remove", "incoming profile"} {
		t.Run(mode, func(t *testing.T) {
			requireGitCLI(t)
			t.Setenv("HOME", t.TempDir())
			home, stateRoot := t.TempDir(), t.TempDir()
			origin, sourceRoot := newInstalledRepo(t, map[string]string{
				"dots.yaml": carbonfoxUpdateManifest, "configs/app": "base\n", "configs/marker": "marker\n",
			})
			roots := []string{"--file", filepath.Join(sourceRoot, "dots.yaml"), "--source-root", sourceRoot, "--home", home, "--state-root", stateRoot}
			initial := []string{"install", "--yes", "--skip-deps", "--profile", "core"}
			if mode == "remove" {
				initial = append(initial, "--tag", "theme-carbonfox")
			}
			var out, stderr bytes.Buffer
			if code := cli.Run(append(initial, roots...), &out, &stderr); code != cli.ExitOK {
				t.Fatalf("initial install: %d %s %s", code, &out, &stderr)
			}
			before, err := os.ReadFile(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			head := runGitOutput(t, sourceRoot, "rev-parse", "HEAD")
			changed := map[string]string{"configs/app": "incoming\n"}
			if mode == "incoming profile" {
				changed["dots.yaml"] = strings.Replace(carbonfoxUpdateManifest, "tags: [app]", "tags: [app, theme-carbonfox]", 1)
			}
			advanceUpstream(t, origin, "advance theme fixture", changed)
			requested := []string{"update", "--yes", "--acknowledge-selection-change", "--profile", "core"}
			if mode == "add" {
				requested = append(requested, "--tag", "theme-carbonfox")
			}
			out.Reset()
			stderr.Reset()
			if code := cli.Run(append(append(append([]string{}, requested...), "--dry-run"), roots...), &out, &stderr); code != cli.ExitOK {
				t.Fatalf("dry run: %d %s %s", code, &out, &stderr)
			}
			if !strings.Contains(out.String(), "theme-carbonfox") {
				t.Fatalf("dry run lost membership delta: %s", &out)
			}
			out.Reset()
			stderr.Reset()
			code := cli.Run(append(requested, roots...), &out, &stderr)
			if code != cli.ExitError || !strings.Contains(out.String()+stderr.String(), "requires dots install with the complete desired selection") {
				t.Fatalf("update: %d %s %s", code, &out, &stderr)
			}
			if got := runGitOutput(t, sourceRoot, "rev-parse", "HEAD"); got != head {
				t.Fatalf("checkout changed: %s -> %s", head, got)
			}
			after, err := os.ReadFile(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("rejected preference change mutated Installation Metadata")
			}
			data, err := os.ReadFile(filepath.Join(home, ".app"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "base\n" {
				t.Fatalf("managed config changed: %q", data)
			}
		})
	}
}
