package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/deps/pkgmgr"
)

const packageManagerSetupManifest = `version: 1
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
        command: starship
        brew: starship
`

func withDarwinInstallHost(t *testing.T, detector pkgmgr.Detector, runner pkgmgr.Runner) {
	t.Helper()
	oldOS, oldArch := installHostOS, installHostArch
	oldDetector, oldRunner := packageManagerDetector, packageManagerSetupRunner
	installHostOS, installHostArch = "darwin", "arm64"
	packageManagerDetector = detector
	packageManagerSetupRunner = runner
	t.Cleanup(func() {
		installHostOS, installHostArch = oldOS, oldArch
		packageManagerDetector = oldDetector
		packageManagerSetupRunner = oldRunner
	})
}

func writeInstallFixture(t *testing.T, manifestText string) (home, sourceRoot, stateRoot, manifestPath string) {
	t.Helper()
	home = t.TempDir()
	sourceRoot = t.TempDir()
	stateRoot = t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceRoot, "configs", "zsh"), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "configs", "zsh", "zshrc"), []byte("managed\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	manifestPath = filepath.Join(home, "dots.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestText), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return home, sourceRoot, stateRoot, manifestPath
}

func runInstallCommand(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if stdin != "" {
		cmd.SetIn(strings.NewReader(stdin))
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

type recordingPkgMgrRunner struct {
	executable string
	args       []string
	onRun      func() error
}

func (r *recordingPkgMgrRunner) Run(ctx context.Context, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	r.executable = executable
	r.args = append([]string(nil), args...)
	if r.onRun != nil {
		return r.onRun()
	}
	return nil
}

func TestInstallDryRunJSONReportsHomebrewPackageManagerSetupSeparately(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	home, sourceRoot, _, manifestPath := writeInstallFixture(t, packageManagerSetupManifest)
	withDarwinInstallHost(t, pkgmgr.Detector{
		LookPath: func(command string) (string, error) { return "", os.ErrNotExist },
		Exists:   func(path string) bool { return false },
	}, &recordingPkgMgrRunner{})

	out, err := runInstallCommand(t, "", "install", "--dry-run", "--output", "json", "--file", manifestPath, "--home", home, "--source-root", sourceRoot)
	if err != nil {
		t.Fatalf("install --dry-run error = %v\n%s", err, out)
	}
	var env struct {
		Data struct {
			PackageManagerSetup *struct {
				Manager string `json:"manager"`
				Status  string `json:"status"`
				Command struct {
					Display string `json:"display"`
				} `json:"command"`
				Dependencies []string `json:"dependencies"`
			} `json:"package_manager_setup"`
			Dependencies struct {
				Preview struct {
					Items []struct {
						Dependency string `json:"dependency"`
					} `json:"items"`
				} `json:"preview"`
			} `json:"dependencies"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	setup := env.Data.PackageManagerSetup
	if setup == nil || setup.Manager != "homebrew" || setup.Status != "would-offer" || setup.Command.Display != pkgmgr.OfficialHomebrewInstallerCommand || !reflect.DeepEqual(setup.Dependencies, []string{"starship"}) {
		t.Fatalf("package_manager_setup = %#v\n%s", setup, out)
	}
	if len(env.Data.Dependencies.Preview.Items) != 1 {
		t.Fatalf("dependency preview should stay under dependencies, got %#v", env.Data.Dependencies)
	}
}

func TestInstallYesDoesNotRunHomebrewPackageManagerSetup(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	home, sourceRoot, stateRoot, manifestPath := writeInstallFixture(t, packageManagerSetupManifest)
	runner := &recordingPkgMgrRunner{}
	withDarwinInstallHost(t, pkgmgr.Detector{
		LookPath: func(command string) (string, error) { return "", os.ErrNotExist },
		Exists:   func(path string) bool { return false },
	}, runner)

	out, err := runInstallCommand(t, "", "install", "--yes", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if err == nil {
		t.Fatalf("install --yes error = nil, want interactive setup error\n%s", out)
	}
	if runner.executable != "" {
		t.Fatalf("--yes must not run setup, runner = %#v", runner)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(statErr) {
		t.Fatalf("--yes setup block must not write managed config; lstat=%v", statErr)
	}
}

func TestInstallYesJSONReportsHomebrewPackageManagerSetupGate(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	home, sourceRoot, stateRoot, manifestPath := writeInstallFixture(t, packageManagerSetupManifest)
	runner := &recordingPkgMgrRunner{}
	withDarwinInstallHost(t, pkgmgr.Detector{
		LookPath: func(command string) (string, error) { return "", os.ErrNotExist },
		Exists:   func(path string) bool { return false },
	}, runner)

	var out, errOut bytes.Buffer
	code := Run([]string{"install", "--yes", "--output", "json", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}, &out, &errOut)
	if code != ExitError {
		t.Fatalf("exit code = %d, want %d\nstderr:\n%s\nstdout:\n%s", code, ExitError, errOut.String(), out.String())
	}
	if errOut.Len() != 0 {
		t.Fatalf("Machine Output Mode must keep stderr clean, got:\n%s", errOut.String())
	}
	if runner.executable != "" {
		t.Fatalf("--yes JSON must not run setup, runner = %#v", runner)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(statErr) {
		t.Fatalf("--yes setup gate must not write managed config; lstat=%v", statErr)
	}

	var env struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			PackageManagerSetup *struct {
				Manager string `json:"manager"`
				Status  string `json:"status"`
				Command struct {
					Display string `json:"display"`
				} `json:"command"`
			} `json:"package_manager_setup"`
			Dependencies struct {
				Preview *struct {
					Items []struct {
						Dependency string `json:"dependency"`
					} `json:"items"`
				} `json:"preview"`
			} `json:"dependencies"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, out.String())
	}
	setup := env.Data.PackageManagerSetup
	if env.Status != "error" || !strings.Contains(env.Error, "requires interactive confirmation") {
		t.Fatalf("error envelope = %#v\n%s", env, out.String())
	}
	if setup == nil || setup.Manager != "homebrew" || setup.Status != "unavailable" || setup.Command.Display != pkgmgr.OfficialHomebrewInstallerCommand {
		t.Fatalf("package_manager_setup = %#v\n%s", setup, out.String())
	}
	if env.Data.Dependencies.Preview == nil || len(env.Data.Dependencies.Preview.Items) != 1 {
		t.Fatalf("dependencies preview missing from gate envelope: %#v\n%s", env.Data.Dependencies, out.String())
	}
}

func TestInstallDecliningHomebrewSetupAbortsBeforeManagedConfiguration(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	home, sourceRoot, stateRoot, manifestPath := writeInstallFixture(t, packageManagerSetupManifest)
	withDarwinInstallHost(t, pkgmgr.Detector{
		LookPath: func(command string) (string, error) { return "", os.ErrNotExist },
		Exists:   func(path string) bool { return false },
	}, &recordingPkgMgrRunner{})

	out, err := runInstallCommand(t, "n\n", "install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if err != nil {
		t.Fatalf("install decline error = %v\n%s", err, out)
	}
	if !strings.Contains(out, pkgmgr.OfficialHomebrewInstallerCommand) || !strings.Contains(out, "Package Manager Setup declined") {
		t.Fatalf("decline output missing setup details:\n%s", out)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(statErr) {
		t.Fatalf("declined setup must not write managed config; lstat=%v", statErr)
	}
}

func TestInstallAcceptingHomebrewSetupUsesPrefixBrewForCurrentRun(t *testing.T) {
	home, sourceRoot, stateRoot, manifestPath := writeInstallFixture(t, packageManagerSetupManifest)
	binDir := t.TempDir()
	brewPath := filepath.Join(t.TempDir(), "brew")
	probe := filepath.Join(binDir, "starship")
	t.Setenv("PATH", binDir)

	runner := &recordingPkgMgrRunner{onRun: func() error {
		if err := os.MkdirAll(filepath.Dir(brewPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(brewPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$BREW_ARGS\"\nprintf '#!/bin/sh\\n' > \"$STARSHIP_PROBE\"\n/bin/chmod +x \"$STARSHIP_PROBE\"\n"), 0o755)
	}}
	t.Setenv("BREW_ARGS", filepath.Join(binDir, "brew-args"))
	t.Setenv("STARSHIP_PROBE", probe)
	withDarwinInstallHost(t, pkgmgr.Detector{
		LookPath: func(command string) (string, error) { return "", os.ErrNotExist },
		Exists: func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		},
		Prefixes: []string{brewPath},
	}, runner)

	out, err := runInstallCommand(t, "y\ny\n", "install", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if err != nil {
		t.Fatalf("install accept error = %v\n%s", err, out)
	}
	if runner.executable != "/bin/bash" || !reflect.DeepEqual(runner.args, []string{"-c", `$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)`}) {
		t.Fatalf("setup runner = %#v", runner)
	}
	args, err := os.ReadFile(filepath.Join(binDir, "brew-args"))
	if err != nil || string(args) != "install\nstarship\n" {
		t.Fatalf("brew args = %q, err=%v\noutput:\n%s", string(args), err, out)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("managed config not written after setup+deps: %v\n%s", err, out)
	}
	if !strings.Contains(out, "not on PATH") {
		t.Fatalf("output should include PATH guidance for prefix brew:\n%s", out)
	}
}
