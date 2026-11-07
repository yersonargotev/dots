package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

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
		for _, folder := range []string{"herdr"} {
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
