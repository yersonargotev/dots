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
	previewIdx := strings.Index(got, "Dependency install preview")
	installIdx := strings.Index(got, "Dependency install for profile")
	planIdx := strings.Index(got, "Plan for profile")
	if previewIdx < 0 || installIdx < 0 || planIdx < 0 || previewIdx > installIdx || installIdx > planIdx {
		t.Fatalf("dependency actions must render before file plan:\n%s", got)
	}
}

func TestInstallSkipDepsPreservesConfigOnlyBehavior(t *testing.T) {
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
	cmd.SetArgs([]string{"install", "--skip-deps", "--yes", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("skip-deps install did not write managed target: %v", err)
	}
	if strings.Contains(out.String(), "Dependency install preview") {
		t.Fatalf("skip-deps should bypass dependency preview/provisioning:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Dependency provisioning skipped (--skip-deps).") {
		t.Fatalf("skip-deps should make the bypass visible:\n%s", out.String())
	}
}

func TestInstallReportsOptionalInstallableDependencyWithoutPromptingOrBlocking(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	binDir := t.TempDir()
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	brew := filepath.Join(binDir, "brew")
	argsLog := filepath.Join(binDir, "brew-args")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\nprintf '%s\n' \"$@\" > "+argsLog+"\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
      - name: definitely-missing-optional
        requirement: optional
        command: definitely-missing-optional-probe
        brew: optional-tool
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want optional dependency to be non-blocking\noutput:\n%s", err, out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("optional installable dependency must not block managed target write: %v", err)
	}
	if _, err := os.Stat(argsLog); !os.IsNotExist(err) {
		t.Fatalf("optional dependency should not run package manager without --yes; stat err = %v", err)
	}
	got := out.String()
	for _, unwanted := range []string{"Proceed with dependency installation?", "Dependency installation cancelled."} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("optional-only install should not contain %q:\n%s", unwanted, got)
		}
	}
	for _, want := range []string{"would-install", "unresolved", "optional", "Plan for profile"} {
		if !strings.Contains(got, want) {
			t.Fatalf("optional-only install output missing %q:\n%s", want, got)
		}
	}
}

func TestInstallReportsOptionalDependencyWithoutBlockingManagedConfiguration(t *testing.T) {
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
      - name: definitely-missing-optional
        requirement: optional
        command: definitely-missing-optional-probe
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--yes", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want optional dependency to be non-blocking\noutput:\n%s", err, out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("optional dependency must not block managed target write: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "manual") || !strings.Contains(got, "optional") || !strings.Contains(got, "definitely-missing-optional") {
		t.Fatalf("output should report unresolved optional dependency:\n%s", got)
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
						Dependency  string `json:"dependency"`
						Requirement string `json:"requirement"`
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
	if env.Status != "ok" || len(env.Data.Dependencies.Preview.Items) != 1 || env.Data.Dependencies.Preview.Items[0].Dependency != "starship" || env.Data.Dependencies.Preview.Items[0].Requirement != "required" {
		t.Fatalf("dependency preview missing from JSON envelope: %#v\n%s", env, out.String())
	}
	if len(env.Data.Plan.Actions) != 1 {
		t.Fatalf("install plan missing from JSON envelope: %#v\n%s", env, out.String())
	}
}

func TestInstallJSONDependencyFailureIncludesResultAndInstallPlan(t *testing.T) {
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

	var out, errOut bytes.Buffer
	code := cli.Run([]string{"install", "--yes", "--output", "json", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}, &out, &errOut)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d\nstderr:\n%s\nstdout:\n%s", code, cli.ExitError, errOut.String(), out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("Machine Output Mode must keep stderr clean, got:\n%s", errOut.String())
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(statErr) {
		t.Fatalf("dependency failure must not write target; lstat err = %v", statErr)
	}

	var env struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Dependencies struct {
				Result struct {
					Items []struct {
						Dependency  string `json:"dependency"`
						Requirement string `json:"requirement"`
						Status      string `json:"status"`
					} `json:"items"`
				} `json:"result"`
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
	if env.Status != "error" || env.Error == "" {
		t.Fatalf("error envelope missing status/error: %#v\n%s", env, out.String())
	}
	if len(env.Data.Dependencies.Result.Items) != 1 || env.Data.Dependencies.Result.Items[0].Status != "manual" || env.Data.Dependencies.Result.Items[0].Requirement != "required" {
		t.Fatalf("dependency result missing from error envelope: %#v\n%s", env.Data.Dependencies.Result, out.String())
	}
	if len(env.Data.Plan.Actions) != 1 {
		t.Fatalf("install plan missing from error envelope: %#v\n%s", env.Data.Plan, out.String())
	}
}
