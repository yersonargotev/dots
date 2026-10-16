package manifest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestHerdrCustomSelectionAndSharedPluginPins(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join(repositoryRoot(t), "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for name, profile := range m.Profiles {
		for _, tag := range profile.Tags {
			if tag == "herdr-custom" {
				t.Fatalf("profile %s selects custom implicitly", name)
			}
		}
	}
	pins := map[string]string{}
	for _, p := range m.Provisioners {
		if p.Tool == "herdr" {
			pins[p.Spec.Plugin] = p.Spec.Ref
		}
	}
	for _, platform := range []struct {
		os, arch string
		plugins  int
	}{{"darwin", "arm64", 3}, {"linux", "amd64", 2}} {
		t.Run(platform.os, func(t *testing.T) {
			custom := selectedsurface.EvaluateForPlatform(*m, []string{"herdr-custom"}, platform.os, platform.arch)
			if len(custom.Provisioners) != platform.plugins {
				t.Fatalf("plugins = %d, want %d", len(custom.Provisioners), platform.plugins)
			}
			for _, p := range custom.Provisioners {
				if p.Tool != "herdr-custom" || pins[p.Spec.Plugin] != p.Spec.Ref {
					t.Fatalf("custom plugin does not share official pin: %#v", p)
				}
				if platform.os == "linux" && p.Spec.Plugin == "yersonargotev/tabby" {
					t.Fatal("Linux selects unsupported Tabby")
				}
			}
			var runtimeFound bool
			for _, dep := range custom.Dependencies {
				if dep.Name == "herdr" || dep.Command == "herdr" || dep.Brew == "herdr" {
					t.Fatalf("custom depends on official: %#v", dep)
				}
				if dep.Name == "herdr-custom" {
					runtimeFound = true
					if dep.UserLocal == nil || dep.UserLocal.Recipe != "herdr-custom" || dep.UserLocal.Version != "v0.9.3-custom.5" || len(dep.UserLocal.Checksums) != 2 {
						t.Fatalf("unpinned custom runtime: %#v", dep)
					}
				}
			}
			if !runtimeFound {
				t.Fatal("custom runtime missing")
			}
			for _, tool := range []string{"lazygit", "fzf", "fd", "bat", "git", "Node LTS (fnm)", "Rust stable (rustup)"} {
				if !selectedDependencyNamed(custom, tool) {
					t.Errorf("custom misses %s", tool)
				}
			}
			if selectedDependencyNamed(custom, "macmon") {
				t.Fatal("custom implicitly selects metrics")
			}
			if platform.os == "linux" {
				if !selectedDependencyNamed(custom, "xdg-open") {
					t.Fatal("Linux missing URL opener")
				}
				for _, dep := range custom.Dependencies {
					if dep.Name == "pbcopy" || dep.Name == "open" {
						t.Fatalf("Linux selects macOS utility %s", dep.Name)
					}
					if dep.Name == "wl-copy" || dep.Name == "xclip" || dep.Name == "xsel" {
						if dep.RequirementValue() != manifest.DependencyRequirementOptional {
							t.Fatalf("clipboard alternative %s must be optional", dep.Name)
						}
					}
				}
			}
			official := selectedsurface.EvaluateForPlatform(*m, []string{"herdr"}, platform.os, platform.arch)
			for _, dep := range official.Dependencies {
				if dep.Name == "herdr-custom" {
					t.Fatal("official selects custom")
				}
			}
			for _, p := range official.Provisioners {
				if p.Tool != "herdr" {
					t.Fatalf("official selects %s", p.Tool)
				}
			}
		})
	}
}

func TestHerdrCustomThemeAndCompactSettings(t *testing.T) {
	root := repositoryRoot(t)
	m, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []struct{ file, tag string }{{"config.toml", ""}, {"config-adaptive.toml", "adaptive-theme"}, {"config-carbonfox.toml", "theme-carbonfox"}} {
		t.Run(variant.file, func(t *testing.T) {
			read := func(dir string) map[string]any {
				data, err := os.ReadFile(filepath.Join(root, "configs", dir, variant.file))
				if err != nil {
					t.Fatal(err)
				}
				var config map[string]any
				if err := toml.Unmarshal(data, &config); err != nil {
					t.Fatal(err)
				}
				return config
			}
			custom, official := read("herdr-custom"), read("herdr")
			for _, section := range []string{"theme", "terminal", "keys"} {
				if !reflect.DeepEqual(custom[section], official[section]) {
					t.Fatalf("custom changes shared %s", section)
				}
			}
			ui := custom["ui"].(map[string]any)
			for key, want := range map[string]int64{"sidebar_width": 38, "sidebar_min_width": 18, "sidebar_max_width": 44} {
				if ui[key] != want {
					t.Errorf("%s = %v, want %v", key, ui[key], want)
				}
			}
			if ui["status_indicators"] != "dots" || ui["tab_bar_position"] != "top" {
				t.Fatalf("unexpected custom UI: %#v", ui)
			}
			if custom["update"].(map[string]any)["version_check"] != false {
				t.Fatal("official update checks enabled")
			}
			sidebar := ui["sidebar"].(map[string]any)
			for name, wantGap := range map[string]int64{"spaces": 1, "agents": 1} {
				section := sidebar[name].(map[string]any)
				if section["row_gap"] != wantGap {
					t.Fatalf("%s row_gap = %v, want %d", name, section["row_gap"], wantGap)
				}
				for _, row := range section["rows"].([]any) {
					if len(row.([]any)) == 0 {
						t.Fatalf("%s has blank row", name)
					}
				}
			}
			spaces := sidebar["spaces"].(map[string]any)["rows"].([]any)
			if len(spaces) != 3 || len(spaces[0].([]any)) != 2 || len(spaces[1].([]any)) != 1 || len(spaces[2].([]any)) != 7 {
				t.Fatalf("Spaces must have separate tab, branch, and status rows: %#v", spaces)
			}
			if spaces[0].([]any)[0] != "state_icon" || spaces[0].([]any)[1].(map[string]any)["token"] != "$tab_name" {
				t.Fatal("Spaces must use active tab names")
			}
			nameStyle := spaces[0].([]any)[1].(map[string]any)
			if _, fixed := nameStyle["fg"]; fixed {
				t.Fatal("Space tab names must inherit native selection colors")
			}
			if _, rules := nameStyle["rules"]; rules {
				t.Fatal("Space tab names must not use label-specific selection rules")
			}
			agents := sidebar["agents"].(map[string]any)["rows"].([]any)
			first := agents[0].([]any)
			if first[0] != "state_icon" || first[1].(map[string]any)["token"] != "agent" || first[2] != "state_text" {
				t.Fatal("agent/state row changed")
			}
			if len(agents) != 2 || agents[1].([]any)[0].(map[string]any)["token"] != "tab" {
				t.Fatal("native tab context missing")
			}
			surface := selectedsurface.EvaluateForPlatform(*m, []string{"herdr-custom", variant.tag}, "linux", "amd64")
			found := false
			for _, e := range surface.Entries {
				if e.Entry.Target == "~/.config/herdr/config.toml" {
					found = true
					if e.Source != "configs/herdr-custom/"+variant.file || e.Entry.Ownership != "toml-subset" {
						t.Fatalf("wrong selected config: %#v", e)
					}
				}
			}
			if !found {
				t.Fatal("custom config not selected")
			}
		})
	}
}

func TestHerdrGitColorsMatchStarship(t *testing.T) {
	root := repositoryRoot(t)
	read := func(path string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, "configs", path))
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if err := toml.Unmarshal(data, &config); err != nil {
			t.Fatal(err)
		}
		return config
	}
	for _, variant := range []struct{ herdr, starship string }{
		{"config.toml", "starship.toml"},
		{"config-adaptive.toml", "starship.toml"},
		{"config-carbonfox.toml", "starship-carbonfox.toml"},
	} {
		prompt := read(filepath.Join("starship", variant.starship))
		palette := prompt["palettes"].(map[string]any)[prompt["palette"].(string)].(map[string]any)
		branch := prompt["git_branch"].(map[string]any)
		if branch["style"] != "bold fg:mauve" {
			t.Fatal("Herdr branch color and weight must follow the Starship style")
		}
		status := prompt["git_status"].(map[string]any)
		for _, folder := range []string{"herdr", "herdr-custom"} {
			t.Run(folder+"/"+variant.herdr, func(t *testing.T) {
				config := read(filepath.Join(folder, variant.herdr))
				rows := config["ui"].(map[string]any)["sidebar"].(map[string]any)["spaces"].(map[string]any)["rows"].([]any)
				row := append(append([]any{}, rows[1].([]any)...), rows[2].([]any)...)
				want := []struct{ token, role, module, format string }{
					{"gitbranch", "mauve", "", ""},
					{"gitstaged", "green", "staged", "[+${count}](fg:green) "},
					{"gitworkmodified", "yellow", "modified", "[!${count}](fg:yellow) "},
					{"gituntracked", "red", "untracked", "[?${count}](fg:red) "},
					{"gitforward", "teal", "ahead", "[⇡${count}](fg:teal) "},
					{"gitbackward", "peach", "behind", "[⇣${count}](fg:peach) "},
					{"gitdiverged", "mauve", "diverged", "[⇕⇡${ahead_count}⇣${behind_count}](fg:mauve) "},
					{"gitclean", "overlay0", "", ""},
				}
				if len(row) != len(want) {
					t.Fatalf("Git rows have %d tokens, want %d", len(row), len(want))
				}
				for i, item := range want {
					got := row[i].(map[string]any)
					if got["token"] != "$"+item.token || got["fg"] != palette[item.role] || got["dim"] != false {
						t.Errorf("Git token %d = %#v, want %s in %s", i, got, item.token, palette[item.role])
					}
					if (got["bold"] == true) != (i == 0) {
						t.Errorf("%s bold style differs from Starship", item.token)
					}
					if item.module != "" && status[item.module] != item.format {
						t.Errorf("Starship %s changed; update the plugin parity contract", item.module)
					}
				}
			})
		}
	}
}
