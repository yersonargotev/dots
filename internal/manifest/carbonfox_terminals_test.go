package manifest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/yersonargotev/dots/internal/manifest"
)

const carbonfoxTerminalRevision = "4dacd3f0185a2227bdf3b6c0975a8f0bf87cac9a"

func TestCarbonfoxTerminalManifestMappings(t *testing.T) {
	root := repositoryRoot(t)
	got, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	overrides := map[string]string{
		"~/.config/ghostty/config.ghostty": "configs/ghostty/config-carbonfox.ghostty",
		"~/.config/herdr/config.toml":      "configs/herdr/config-carbonfox.toml",
	}
	for target, source := range overrides {
		entry := findEntry(got.Entries, target)
		if entry == nil {
			t.Errorf("manifest misses terminal target %q", target)
			continue
		}
		if selected := entry.SourceOverrides["theme-carbonfox"]; selected != source {
			t.Errorf("%s Carbonfox source = %q, want %q", target, selected, source)
		}
	}

	assets := []struct {
		target string
		source string
		tags   []string
		os     []string
	}{
		{"~/.config/ghostty/themes/Carbonfox", "configs/ghostty/themes/Carbonfox", []string{"ghostty"}, []string{"darwin", "linux"}},
	}
	for _, asset := range assets {
		entry := findEntry(got.Entries, asset.target)
		if entry == nil {
			t.Errorf("manifest misses terminal theme asset %q", asset.target)
			continue
		}
		if entry.Source != asset.source || entry.Strategy != "symlink" ||
			!reflect.DeepEqual(entry.Tags, asset.tags) || !reflect.DeepEqual(entry.OS, asset.os) {
			t.Errorf("theme asset %s = %#v, want source %q, symlink, tags %v, os %v", asset.target, *entry, asset.source, asset.tags, asset.os)
		}
	}
}

func TestCarbonfoxTerminalVariantsPreserveNonThemeBehavior(t *testing.T) {
	root := repositoryRoot(t)

	ghosttyBase := readCarbonfoxTerminalFile(t, root, "configs/ghostty/config.ghostty")
	ghosttyCarbonfox := readCarbonfoxTerminalFile(t, root, "configs/ghostty/config-carbonfox.ghostty")
	if got, want := ghosttyDirectives(ghosttyCarbonfox), ghosttyDirectives(ghosttyBase); !reflect.DeepEqual(got, want) {
		t.Fatalf("Ghostty Carbonfox variant changed a non-theme directive\ngot:  %q\nwant: %q", got, want)
	}
	adaptive := strings.Index(ghosttyCarbonfox, "config-file = ?adaptive-theme.ghostty")
	carbonfox := strings.LastIndex(ghosttyCarbonfox, "theme = Carbonfox")
	local := strings.Index(ghosttyCarbonfox, "config-file = ?config.local.ghostty")
	if adaptive < 0 || carbonfox <= adaptive || local <= carbonfox {
		t.Fatalf("Ghostty theme ordering must be adaptive include, Carbonfox, local include:\n%s", ghosttyCarbonfox)
	}

	herdrBase := readCarbonfoxTerminalTOML(t, root, "configs/herdr/config.toml")
	herdrCarbonfox := readCarbonfoxTerminalTOML(t, root, "configs/herdr/config-carbonfox.toml")
	delete(herdrBase, "theme")
	delete(herdrCarbonfox, "theme")
	deleteMapKeyRecursively(herdrBase, "fg")
	deleteMapKeyRecursively(herdrCarbonfox, "fg")
	if !reflect.DeepEqual(herdrCarbonfox, herdrBase) {
		t.Fatal("Herdr Carbonfox variant changed settings outside the theme and authored row colors")
	}

}

func TestCarbonfoxTerminalAdaptersUsePinnedPalette(t *testing.T) {
	root := repositoryRoot(t)
	provenanceFiles := []string{
		"configs/ghostty/themes/Carbonfox",
		"configs/herdr/config-carbonfox.toml",
	}
	for _, path := range provenanceFiles {
		content := readCarbonfoxTerminalFile(t, root, path)
		if !strings.Contains(content, carbonfoxTerminalRevision) || !strings.Contains(content, "LICENSE-nightfox") {
			t.Errorf("%s does not retain the pinned Nightfox revision and license reference", path)
		}
	}

	ghostty := readCarbonfoxTerminalFile(t, root, "configs/ghostty/themes/Carbonfox")
	for _, want := range []string{
		"palette = 0=#282828", "palette = 1=#ee5396", "palette = 2=#25be6a", "palette = 3=#08bdba",
		"palette = 4=#78a9ff", "palette = 5=#be95ff", "palette = 6=#33b1ff", "palette = 7=#dfdfe0",
		"palette = 8=#484848", "palette = 9=#f16da6", "palette = 10=#46c880", "palette = 11=#2dc7c4",
		"palette = 12=#8cb6ff", "palette = 13=#c8a5ff", "palette = 14=#52bdff", "palette = 15=#e4e4e5",
		"background = #161616", "foreground = #f2f4f8", "selection-background = #2a2a2a",
	} {
		if !strings.Contains(ghostty, want) {
			t.Errorf("Ghostty Carbonfox theme misses %q", want)
		}
	}

	herdr := readCarbonfoxTerminalTOML(t, root, "configs/herdr/config-carbonfox.toml")
	theme := nestedMap(t, herdr, "theme")
	if _, ok := theme["auto_switch"]; ok {
		t.Fatal("Herdr Carbonfox theme must remain dark-only")
	}
	wantHerdrPalette := map[string]any{
		"accent": "#78a9ff", "panel_bg": "#161616", "sidebar_bg": "#0c0c0c",
		"active_row_bg": "#353535", "selection_bg": "#525253", "surface0": "#252525",
		"surface1": "#353535", "surface_dim": "#525253", "overlay0": "#535353",
		"overlay1": "#7b7c7e", "text": "#f2f4f8", "subtext0": "#b6b8bb",
		"mauve": "#be95ff", "green": "#25be6a", "yellow": "#08bdba", "red": "#ee5396",
		"blue": "#78a9ff", "teal": "#33b1ff", "peach": "#3ddbd9",
	}
	if got := nestedMap(t, theme, "custom"); !reflect.DeepEqual(got, wantHerdrPalette) {
		t.Fatalf("Herdr Carbonfox palette = %#v, want %#v", got, wantHerdrPalette)
	}
	herdrText := readCarbonfoxTerminalFile(t, root, "configs/herdr/config-carbonfox.toml")
	for _, rowColor := range []string{
		`"$tab_name", fg = "#f2f4f8"`, `"$gitbranch", fg = "#be95ff"`,
		`"$gitstaged", fg = "#25be6a"`, `"$gitworkmodified", fg = "#08bdba"`,
		`"$gituntracked", fg = "#ee5396"`, `"$gitforward", fg = "#2dc7c4"`,
		`"$gitbackward", fg = "#3ddbd9"`, `"$gitdiverged", fg = "#be95ff"`, `"$gitclean", fg = "#6e6f70"`,
		`"agent", fg = "#be95ff"`, `"$project", fg = "#6e6f70"`,
	} {
		if !strings.Contains(herdrText, rowColor) {
			t.Errorf("Herdr Carbonfox authored rows miss %q", rowColor)
		}
	}

}

func TestCarbonfoxTerminalConfigsPassAvailableNativeValidators(t *testing.T) {
	root := repositoryRoot(t)
	sandbox := t.TempDir()
	env := append(os.Environ(),
		"HOME="+sandbox,
		"XDG_CONFIG_HOME="+filepath.Join(sandbox, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(sandbox, ".cache"),
		"XDG_DATA_HOME="+filepath.Join(sandbox, ".local", "share"),
		"XDG_STATE_HOME="+filepath.Join(sandbox, ".local", "state"),
	)

	if ghostty, err := exec.LookPath("ghostty"); err == nil {
		t.Run("Ghostty", func(t *testing.T) {
			configDir := filepath.Join(sandbox, ".config", "ghostty")
			if err := os.MkdirAll(filepath.Join(configDir, "themes"), 0o700); err != nil {
				t.Fatal(err)
			}
			mustSymlink(t, filepath.Join(root, "configs/ghostty/config-carbonfox.ghostty"), filepath.Join(configDir, "config.ghostty"))
			mustSymlink(t, filepath.Join(root, "configs/ghostty/themes/Carbonfox"), filepath.Join(configDir, "themes", "Carbonfox"))
			cmd := exec.Command(ghostty, "+validate-config", "--config-file="+filepath.Join(configDir, "config.ghostty"))
			cmd.Env = env
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("ghostty validate Carbonfox config: %v\n%s", err, output)
			}
			show := exec.Command(ghostty, "+show-config", "--changes-only")
			show.Env = env
			output, err := show.CombinedOutput()
			if err != nil {
				t.Fatalf("ghostty load Carbonfox config: %v\n%s", err, output)
			}
			for _, want := range []string{"theme = Carbonfox", "background = #161616", "palette = 1=#ee5396"} {
				if !strings.Contains(string(output), want) {
					t.Errorf("ghostty effective config misses %q", want)
				}
			}
		})
	}

	if herdr, err := exec.LookPath("herdr"); err == nil {
		t.Run("Herdr", func(t *testing.T) {
			cmd := exec.Command(herdr, "config", "check")
			cmd.Env = append(env,
				"HERDR_CONFIG_PATH="+filepath.Join(root, "configs/herdr/config-carbonfox.toml"),
				"HERDR_SOCKET_PATH="+filepath.Join(sandbox, "missing.sock"),
				"HERDR_SESSION=dots-carbonfox-check",
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("herdr validate Carbonfox config: %v\n%s", err, output)
			}
		})
	}

}

func readCarbonfoxTerminalFile(t *testing.T, root, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func readCarbonfoxTerminalTOML(t *testing.T, root, name string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := toml.Unmarshal([]byte(readCarbonfoxTerminalFile(t, root, name)), &result); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return result
}

func nestedMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	result, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%q = %#v, want TOML table", key, parent[key])
	}
	return result
}

func deleteMapKeyRecursively(value any, key string) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, key)
		for _, child := range typed {
			deleteMapKeyRecursively(child, key)
		}
	case []any:
		for _, child := range typed {
			deleteMapKeyRecursively(child, key)
		}
	}
}

func ghosttyDirectives(content string) []string {
	var result []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "theme = ") {
			continue
		}
		result = append(result, line)
	}
	return result
}

func mustSymlink(t *testing.T, source, target string) {
	t.Helper()
	if err := os.Symlink(source, target); err != nil {
		t.Fatal(err)
	}
}
