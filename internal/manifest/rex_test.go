package manifest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestRexSelectedSurfaceAndManualPrerequisite(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join(repositoryRoot(t), "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tag := m.Tags["rex"]
	if tag.Kind != "surface" || tag.Status != "current" {
		t.Fatalf("Rex Tag = %+v", tag)
	}
	for _, profile := range []string{"desktop", "workstation"} {
		selection, err := manifest.ResolveSelection(*m, []string{profile}, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, platform := range []struct{ os, arch string }{{"darwin", "arm64"}, {"linux", "arm64"}, {"linux", "amd64"}} {
			surface := selectedsurface.EvaluateForPlatform(*m, selection.Tags, platform.os, platform.arch)
			for _, rel := range []string{"init.lua", "themes/Catppuccin Mocha Blue.ghostty"} {
				var entry *manifest.Entry
				for _, selected := range surface.Entries {
					if selected.Entry.Target == "~/.config/rex/"+rel {
						e := selected.Entry
						entry = &e
						break
					}
				}
				if entry == nil {
					t.Fatalf("%s/%s/%s omits Rex %s", profile, platform.os, platform.arch, rel)
				}
				if entry.Source != "configs/rex/"+rel || entry.Strategy != "copy" || len(entry.SourceOverrides) != 0 {
					t.Fatalf("Rex must use regular copies without theme variants: %+v", entry)
				}
			}
			dep := findDependency(surface.Dependencies, "rex")
			if dep == nil {
				t.Fatal("Rex prerequisite missing")
			}
			// Comparing the whole declaration also rejects accidental executable providers.
			want := manifest.Dependency{Name: "rex", Command: "rex", DarwinApp: "Rex Beta.app", Requirement: "optional", Manual: "Install Rex manually from https://www.superlogical.com/rex/ to use the managed configuration. dots manages the configuration and theme even when Rex is absent; dots does not install Rex."}
			if !reflect.DeepEqual(*dep, want) {
				t.Fatalf("Rex prerequisite = %+v, want %+v", *dep, want)
			}
		}
	}
	surface := selectedsurface.EvaluateForPlatform(*m, []string{"rex", "adaptive-theme", "theme-carbonfox"}, "darwin", "arm64")
	for _, p := range surface.Provisioners {
		if hasString(p.Tags, "rex") {
			t.Fatalf("Rex must not select a Provisioner: %+v", p)
		}
	}
	for _, entry := range surface.Entries {
		if hasString(entry.Entry.Tags, "rex") && entry.Source != "configs/rex/init.lua" && entry.Source != "configs/rex/themes/Catppuccin Mocha Blue.ghostty" {
			t.Fatalf("unexpected Rex variant: %+v", entry)
		}
	}
}

func TestRexAndGhosttyAgreedBindings(t *testing.T) {
	root := repositoryRoot(t)
	rex := map[string]string{
		"cmd+t": `"client.tab.new"`, "cmd+shift+h": `"client.tab.previous"`, "cmd+shift+l": `"client.tab.next"`, "cmd+shift+w": `"client.tab.close"`,
		"cmd+shift+k": `"session.previous"`, "cmd+shift+j": `"session.next"`, "cmd+k": `"session.switch"`, "cmd+shift+n": `"session.new"`,
		"cmd+d": `"pane.split", { direction = "right" }`, "cmd+shift+d": `"pane.split", { direction = "down" }`,
		"cmd+shift+enter": `"pane.zoom"`, "cmd+w": `"pane.close"`, "cmd+r": `"client.mode.enter", { name = "resize" }`, "resize/escape": `"client.mode.exit"`,
	}
	ghostty := map[string]string{
		"super+t": "new_tab", "super+shift+h": "previous_tab", "super+shift+l": "next_tab", "super+shift+w": "close_tab",
		"super+shift+k": "goto_window:previous", "super+shift+j": "goto_window:next", "super+shift+n": "new_window",
		"super+d": "new_split:right", "super+shift+d": "new_split:down", "super+shift+enter": "toggle_split_zoom", "super+w": "close_surface",
		"super+r": "activate_key_table:resize", "resize/escape": "deactivate_key_table",
	}
	for index := 1; index <= 9; index++ {
		n := strconv.Itoa(index)
		target := n
		action := "goto_tab:" + n
		if index == 9 {
			target = "-1"
			action = "last_tab"
		}
		rex["cmd+"+n] = `"client.tab.goto", { index = ` + target + ` }`
		ghostty["super+"+n] = action
	}
	for key, direction := range map[string]string{"h": "left", "j": "down", "k": "up", "l": "right"} {
		rex["ctrl+alt+"+key] = `"pane.focus", { direction = "` + direction + `", wrap = false }`
		rex["resize/"+key] = `"pane.resize", { direction = "` + direction + `", amount = 5 }`
		ghostty["ctrl+alt+"+key] = "goto_split:" + direction
		ghostty["resize/"+key] = "resize_split:" + direction + ",10"
	}
	// The Lua source deliberately contains only declarative single-line calls.
	// Checking all executable lines catches legacy custom bindings and bulk removals.
	var expected []string
	for key, action := range rex {
		expected = append(expected, `rex.bind("`+key+`", `+action+`)`)
	}
	expected = append(expected, `rex.mode("resize", { exclusive = true })`)
	content := readCarbonfoxTerminalFile(t, root, "configs/rex/init.lua")
	var actual []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "--") {
			actual = append(actual, line)
		}
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("Rex executable configuration differs from agreed bindings\ngot %q\nwant %q", actual, expected)
	}
	for _, name := range []string{"config.ghostty", "config-carbonfox.ghostty"} {
		bindings := map[string]string{}
		for _, line := range strings.Split(readCarbonfoxTerminalFile(t, root, "configs/ghostty/"+name), "\n") {
			if !strings.HasPrefix(line, "keybind = ") {
				continue
			}
			key, action, ok := strings.Cut(strings.TrimPrefix(line, "keybind = "), "=")
			if !ok {
				t.Fatalf("invalid binding %q", line)
			}
			bindings[key] = action
			if strings.HasPrefix(action, "esc:") {
				t.Fatalf("%s forwards obsolete shortcut %q", name, line)
			}
		}
		for key, action := range ghostty {
			if bindings[key] != action {
				t.Errorf("%s %s = %q, want %q", name, key, bindings[key], action)
			}
		}
		if _, ok := bindings["super+k"]; ok {
			t.Errorf("%s invents a session-search equivalent", name)
		}
	}
}

func TestRexThemePreservesReviewedPalette(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "configs/rex/themes/Catppuccin Mocha Blue.ghostty"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, line := range strings.Split(string(data), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			got = append(got, line)
		}
	}
	palette := []string{"45475a", "f38ba8", "a6e3a1", "f9e2af", "89b4fa", "f5c2e7", "94e2d5", "bac2de", "585b70", "f7aec2", "c2ecbf", "fcd682", "aeccfc", "f398da", "b1eae1", "a6adc8"}
	var want []string
	for i, color := range palette {
		want = append(want, "palette = "+strconv.Itoa(i)+"=#"+color)
	}
	want = append(want, "background = #1e1e2e", "foreground = #cdd6f4", "cursor-color = #89b4fa", "cursor-text = #1e1e2e", "selection-background = #89b4fa", "selection-foreground = #1e1e2e")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Rex theme = %q, want reviewed palette %q", got, want)
	}
}

func TestRexPrerequisiteDetectsAppOrCLIWithoutInstalling(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join(repositoryRoot(t), "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, os              string
		app, command, present bool
	}{
		{"missing", "darwin", false, false, false},
		{"app-only", "darwin", true, false, true},
		{"cli-only", "darwin", false, true, true},
		{"linux-missing", "linux", false, false, false},
		{"linux-cli", "linux", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := deps.Options{ExtraTags: []string{"rex"}, OS: tc.os, Arch: "arm64", Home: t.TempDir(), AppLookup: func(name string) bool { return name == "Rex Beta.app" && tc.app }}
			report, err := deps.Check(*m, opts, func(name string) bool { return name == "rex" && tc.command }, func(string) bool { return false })
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Results) != 1 || report.Results[0].Present != tc.present {
				t.Fatalf("Rex presence = %+v, want %v", report.Results, tc.present)
			}
		})
	}
}
