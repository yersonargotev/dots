package manifest_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/tailscale/hujson"
)

const carbonfoxRevision = "4dacd3f0185a2227bdf3b6c0975a8f0bf87cac9a"

func TestCarbonfoxWholeFileVariantsPreserveNonThemeSettings(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	starshipBase := readTOMLMap(t, filepath.Join(root, "configs/starship/starship.toml"))
	starshipCarbonfox := readTOMLMap(t, filepath.Join(root, "configs/starship/starship-carbonfox.toml"))
	if starshipBase["palette"] != "catppuccin_mocha" || starshipCarbonfox["palette"] != "carbonfox" {
		t.Fatalf("Starship palette selections = %#v and %#v", starshipBase["palette"], starshipCarbonfox["palette"])
	}
	delete(starshipBase, "palette")
	delete(starshipBase, "palettes")
	delete(starshipCarbonfox, "palette")
	delete(starshipCarbonfox, "palettes")
	if !reflect.DeepEqual(starshipCarbonfox, starshipBase) {
		t.Fatal("Starship Carbonfox variant changed a non-palette setting, format, or icon")
	}

	for _, pair := range []struct {
		name       string
		baseline   string
		carbonfox  string
		themeField string
	}{
		{"Atuin", "configs/atuin/config.toml", "configs/atuin/config-carbonfox.toml", "theme"},
		{"tuicr", "configs/tuicr/config.toml", "configs/tuicr/config-carbonfox.toml", "theme"},
	} {
		t.Run(pair.name, func(t *testing.T) {
			baseline := readTOMLMap(t, filepath.Join(root, pair.baseline))
			carbonfox := readTOMLMap(t, filepath.Join(root, pair.carbonfox))
			delete(baseline, pair.themeField)
			delete(carbonfox, pair.themeField)
			if !reflect.DeepEqual(carbonfox, baseline) {
				t.Fatalf("%s Carbonfox variant changed a non-theme setting", pair.name)
			}
		})
	}

	batBase := strings.TrimSpace(string(readFile(t, filepath.Join(root, "configs/bat/config"))))
	batCarbonfox := strings.TrimSpace(string(readFile(t, filepath.Join(root, "configs/bat/config-carbonfox"))))
	if batBase != `--theme="Catppuccin Mocha"` || batCarbonfox != `--theme="Carbonfox"` {
		t.Fatalf("bat configs = %q and %q, want theme-only variants", batBase, batCarbonfox)
	}

	zedBase := readJSONCMap(t, filepath.Join(root, "configs/zed/settings.json"))
	zedCarbonfox := readJSONCMap(t, filepath.Join(root, "configs/zed/settings-carbonfox.json"))
	delete(zedBase, "theme")
	delete(zedCarbonfox, "theme")
	if !reflect.DeepEqual(zedCarbonfox, zedBase) {
		t.Fatal("Zed Carbonfox variant changed a non-theme setting, extension, font, or icon selection")
	}
}

func TestCarbonfoxAdaptersUsePinnedCanonicalPalette(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	adapterPaths := []string{
		"configs/starship/starship-carbonfox.toml",
		"configs/atuin/themes/carbonfox.toml",
		"configs/tuicr/themes/carbonfox.toml",
		"configs/themes/carbonfox.tmTheme",
		"configs/zed/themes/carbonfox.json",
	}
	provenancePaths := append(append([]string(nil), adapterPaths...), "configs/themes/README.md")
	for _, path := range provenancePaths {
		content := readFile(t, filepath.Join(root, path))
		if !bytes.Contains(content, []byte(carbonfoxRevision)) {
			t.Errorf("%s does not pin the canonical Nightfox revision", path)
		}
	}
	license := readFile(t, filepath.Join(root, "configs/themes/LICENSE-nightfox"))
	for _, want := range []string{"MIT License", "Copyright (c) 2021 James Simpson", "Permission is hereby granted"} {
		if !bytes.Contains(license, []byte(want)) {
			t.Errorf("Nightfox license is missing %q", want)
		}
	}

	allowed := map[string]bool{
		"#000000": true,
		"#0c0c0c": true, "#161616": true, "#252525": true, "#353535": true, "#535353": true,
		"#f9fbff": true, "#f2f4f8": true, "#b6b8bb": true, "#7b7c7e": true, "#6e6f70": true,
		"#2a2a2a": true, "#525253": true,
		"#282828": true, "#484848": true, "#222222": true,
		"#ee5396": true, "#f16da6": true, "#ca4780": true,
		"#25be6a": true, "#46c880": true, "#1fa25a": true,
		"#08bdba": true, "#2dc7c4": true, "#07a19e": true,
		"#78a9ff": true, "#8cb6ff": true, "#6690d9": true,
		"#be95ff": true, "#c8a5ff": true, "#a27fd9": true,
		"#33b1ff": true, "#52bdff": true, "#2b96d9": true,
		"#dfdfe0": true, "#e4e4e5": true, "#bebebe": true,
		"#3ddbd9": true, "#5ae0df": true, "#34bab8": true,
		"#ff7eb6": true, "#ff91c1": true, "#d96b9b": true,
		"#172b20": true, "#311d26": true,
	}
	colorPattern := regexp.MustCompile(`(?i)#[0-9a-f]{6}(?:[0-9a-f]{2})?`)
	for _, path := range adapterPaths {
		content := readFile(t, filepath.Join(root, path))
		for _, match := range colorPattern.FindAllString(string(content), -1) {
			base := strings.ToLower(match[:7])
			if !allowed[base] {
				t.Errorf("%s contains non-Carbonfox color %s", path, match)
			}
		}
	}
}

func TestCarbonfoxNativeAssetsCoverSemanticAndTerminalRoles(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	atuin := readTOMLMap(t, filepath.Join(root, "configs/atuin/themes/carbonfox.toml"))
	atuinColors := stringMap(t, atuin["colors"], "Atuin colors")
	assertColorValues(t, atuinColors, map[string]string{
		"AlertInfo": "#78a9ff", "AlertWarn": "#be95ff", "AlertError": "#ee5396",
		"Base": "#f2f4f8", "Guidance": "#7b7c7e", "Muted": "#6e6f70",
	})

	tuicr := readTOMLMap(t, filepath.Join(root, "configs/tuicr/themes/carbonfox.toml"))
	assertColorValues(t, stringMap(t, tuicr, "tuicr theme"), map[string]string{
		"diff_add": "#25be6a", "diff_del": "#ee5396",
		"message_info_bg": "#78a9ff", "message_warning_bg": "#be95ff",
		"message_error_bg": "#ee5396", "syntax_theme": "carbonfox.tmTheme",
	})

	var zed struct {
		Themes []struct {
			Name       string                     `json:"name"`
			Appearance string                     `json:"appearance"`
			Style      map[string]json.RawMessage `json:"style"`
		} `json:"themes"`
	}
	if err := json.Unmarshal(readFile(t, filepath.Join(root, "configs/zed/themes/carbonfox.json")), &zed); err != nil {
		t.Fatalf("parse Zed Carbonfox theme: %v", err)
	}
	if len(zed.Themes) != 1 || zed.Themes[0].Name != "Carbonfox" || zed.Themes[0].Appearance != "dark" {
		t.Fatalf("Zed themes = %#v, want one dark Carbonfox theme", zed.Themes)
	}
	zedStyle := zed.Themes[0].Style
	assertJSONString(t, zedStyle, "editor.background", "#161616")
	assertJSONString(t, zedStyle, "terminal.ansi.red", "#ee5396")
	assertJSONString(t, zedStyle, "terminal.ansi.green", "#25be6a")
	assertJSONString(t, zedStyle, "terminal.ansi.bright_blue", "#8cb6ff")
	assertJSONString(t, zedStyle, "terminal.ansi.dim_magenta", "#a27fd9")
	assertJSONString(t, zedStyle, "created", "#25be6a")
	assertJSONString(t, zedStyle, "deleted", "#ee5396")
	assertJSONString(t, zedStyle, "modified", "#08bdba")
	assertJSONString(t, zedStyle, "error", "#ee5396")
	assertJSONString(t, zedStyle, "success", "#25be6a")
	assertJSONString(t, zedStyle, "text.disabled", "#7b7c7e")
	assertJSONString(t, zedStyle, "text.placeholder", "#b6b8bb")

	tmTheme := readFile(t, filepath.Join(root, "configs/themes/carbonfox.tmTheme"))
	decoder := xml.NewDecoder(bytes.NewReader(tmTheme))
	for {
		if _, err := decoder.Token(); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("parse Carbonfox tmTheme: %v", err)
		}
	}
	for _, want := range []string{
		"<string>Carbonfox</string>",
		"<string>theme.dark.carbonfox</string>",
		"<string>#2a2a2a</string>",
	} {
		if !bytes.Contains(tmTheme, []byte(want)) {
			t.Errorf("Carbonfox tmTheme missing %q", want)
		}
	}
	if bytes.Contains(tmTheme, []byte("Catppuccin")) {
		t.Fatal("Carbonfox tmTheme retains stale Catppuccin metadata")
	}
}

func TestCarbonfoxSemanticForegroundsRetainReadableContrast(t *testing.T) {
	background := "#161616"
	for role, color := range map[string]string{
		"primary": "#f2f4f8",
		"error":   "#ee5396",
		"success": "#25be6a",
		"warning": "#be95ff",
		"info":    "#78a9ff",
		"accent":  "#33b1ff",
	} {
		if ratio := contrastRatio(t, color, background); ratio < 4.5 {
			t.Errorf("%s contrast ratio = %.2f, want at least 4.5", role, ratio)
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func readTOMLMap(t *testing.T, path string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := toml.Unmarshal(readFile(t, path), &parsed); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return parsed
}

func readJSONCMap(t *testing.T, path string) map[string]any {
	t.Helper()
	standard, err := hujson.Standardize(readFile(t, path))
	if err != nil {
		t.Fatalf("standardize %s: %v", path, err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(standard, &parsed); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return parsed
}

func stringMap(t *testing.T, value any, name string) map[string]string {
	t.Helper()
	source, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s type = %T, want map[string]any", name, value)
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("%s[%q] type = %T, want string", name, key, value)
		}
		result[key] = text
	}
	return result
}

func assertColorValues(t *testing.T, got, want map[string]string) {
	t.Helper()
	for key, value := range want {
		if got[key] != value {
			t.Errorf("%s = %q, want %q", key, got[key], value)
		}
	}
}

func assertJSONString(t *testing.T, values map[string]json.RawMessage, key, want string) {
	t.Helper()
	var got string
	if err := json.Unmarshal(values[key], &got); err != nil {
		t.Fatalf("parse Zed style %s: %v", key, err)
	}
	if got != want {
		t.Errorf("Zed style %s = %q, want %q", key, got, want)
	}
}

func contrastRatio(t *testing.T, foreground, background string) float64 {
	t.Helper()
	fg := relativeLuminance(t, foreground)
	bg := relativeLuminance(t, background)
	return (math.Max(fg, bg) + 0.05) / (math.Min(fg, bg) + 0.05)
}

func relativeLuminance(t *testing.T, color string) float64 {
	t.Helper()
	if len(color) != 7 || color[0] != '#' {
		t.Fatalf("invalid RGB color %q", color)
	}
	components := make([]float64, 3)
	for index := range components {
		value, err := strconv.ParseUint(color[1+index*2:3+index*2], 16, 8)
		if err != nil {
			t.Fatalf("parse RGB color %q: %v", color, err)
		}
		channel := float64(value) / 255
		if channel <= 0.04045 {
			components[index] = channel / 12.92
		} else {
			components[index] = math.Pow((channel+0.055)/1.055, 2.4)
		}
	}
	return components[0]*0.2126 + components[1]*0.7152 + components[2]*0.0722
}
