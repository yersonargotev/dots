package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func seedDesktopNerdFont(t *testing.T, home string) {
	t.Helper()
	for _, rel := range []string{filepath.Join("Library", "Fonts"), filepath.Join(".local", "share", "fonts")} {
		dir := filepath.Join(home, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir font dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "CascadiaCodeNF-Regular.ttf"), []byte("fake font"), 0o600); err != nil {
			t.Fatalf("write fake font: %v", err)
		}
	}
}

const installDepsManifest = `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: starship
        command: definitely-missing-starship-probe
        brew: starship
`

func TestInstallAbortsBeforeFilesWhenRequiredDependencyIsManual(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: definitely-missing-manual
        command: definitely-missing-manual-probe
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil, want dependency gate failure\noutput:\n%s", out.String())
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(statErr) {
		t.Fatalf("dependency failure must not write target; lstat err = %v", statErr)
	}
	got := out.String()
	if !strings.Contains(got, `Dependency install preview for profile "default"`) || !strings.Contains(got, `manual`) {
		t.Fatalf("output missing dependency preview before abort:\n%s", got)
	}
}

func TestInstallRunsDependenciesBeforeManagedConfiguration(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	binDir := t.TempDir()
	probe := filepath.Join(binDir, "definitely-missing-starship-probe")
	brew := filepath.Join(binDir, "brew")
	writeExecStub(t, brew, "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$BREW_ARGS\"\nprintf '#!/bin/sh\\n' > \"$STARSHIP_PROBE\"\nchmod +x \"$STARSHIP_PROBE\"\n")
	t.Setenv("BREW_ARGS", filepath.Join(binDir, "brew-args"))
	t.Setenv("STARSHIP_PROBE", probe)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, installDepsManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("install did not write managed target after dependency success: %v", err)
	}
	args, err := os.ReadFile(filepath.Join(binDir, "brew-args"))
	if err != nil {
		t.Fatalf("dependency provider did not run: %v", err)
	}
	if string(args) != "install\nstarship\n" {
		t.Fatalf("brew args = %q, want install/starship", string(args))
	}
	got := out.String()
	depIdx := strings.Index(got, "Dependency install preview")
	planIdx := strings.Index(got, "Plan for profile")
	if depIdx < 0 || planIdx < 0 || depIdx > planIdx {
		t.Fatalf("dependency actions must render before file plan:\n%s", got)
	}
}

func TestInstallDryRunJSONIncludesDependencyPreviewAndInstallPlan(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, installDepsManifest)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--dry-run", "--output", "json", "--file", manifestPath, "--home", home, "--source-root", sourceRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	var env struct {
		Status string `json:"status"`
		Data   struct {
			Dependencies struct {
				Preview struct {
					Items []struct {
						Dependency string `json:"dependency"`
					} `json:"items"`
				} `json:"preview"`
			} `json:"dependencies"`
			Plan struct {
				Actions []struct {
					Target string `json:"target"`
				} `json:"actions"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, out.String())
	}
	if env.Status != "ok" || len(env.Data.Dependencies.Preview.Items) != 1 || env.Data.Dependencies.Preview.Items[0].Dependency != "starship" {
		t.Fatalf("dependency preview missing from JSON envelope: %#v\n%s", env, out.String())
	}
	if len(env.Data.Plan.Actions) != 1 {
		t.Fatalf("install plan missing from JSON envelope: %#v\n%s", env, out.String())
	}
}
