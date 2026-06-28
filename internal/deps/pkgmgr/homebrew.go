package pkgmgr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

const OfficialHomebrewInstallerCommand = `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
const homebrewInstallerScript = `$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)`

var macOSHomebrewPrefixes = []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"}

type Command struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
	Display    string   `json:"display"`
}

func HomebrewInstallerCommand() Command {
	return Command{Executable: "/bin/bash", Args: []string{"-c", homebrewInstallerScript}, Display: OfficialHomebrewInstallerCommand}
}

type HomebrewDetection struct {
	Found        bool   `json:"found"`
	Path         string `json:"path,omitempty"`
	NeedsPATH    bool   `json:"needs_path,omitempty"`
	PATHGuidance string `json:"path_guidance,omitempty"`
}

type Detector struct {
	LookPath func(string) (string, error)
	Exists   func(string) bool
	Prefixes []string
}

func (d Detector) DetectHomebrew() HomebrewDetection {
	lookPath := d.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	if path, err := lookPath("brew"); err == nil && path != "" {
		return HomebrewDetection{Found: true, Path: path}
	}
	prefixes := d.Prefixes
	if len(prefixes) == 0 {
		prefixes = macOSHomebrewPrefixes
	}
	exists := d.Exists
	if exists == nil {
		exists = func(path string) bool {
			info, err := os.Stat(path)
			return err == nil && !info.IsDir()
		}
	}
	for _, path := range prefixes {
		if exists(path) {
			return HomebrewDetection{Found: true, Path: path, NeedsPATH: true, PATHGuidance: fmt.Sprintf("Homebrew was found at %s but is not on PATH; add its bin directory to PATH for future shells.", path)}
		}
	}
	return HomebrewDetection{}
}

type Status string

const (
	StatusNotNeeded   Status = "not-needed"
	StatusWouldOffer  Status = "would-offer"
	StatusDeclined    Status = "declined"
	StatusInstalled   Status = "installed"
	StatusUnavailable Status = "unavailable"
	StatusFailed      Status = "failed"
)

type Report struct {
	Manager      string            `json:"manager"`
	Status       Status            `json:"status"`
	Command      Command           `json:"command,omitempty"`
	Detection    HomebrewDetection `json:"detection,omitempty"`
	Reason       string            `json:"reason,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
}

func HomebrewSetupNeed(goos string, preview deps.InstallDryRunReport, detection HomebrewDetection) Report {
	report := Report{Manager: "homebrew", Status: StatusNotNeeded, Detection: detection}
	if goos != "darwin" || detection.Found {
		return report
	}
	for _, item := range preview.Items {
		if requirementValue(item.Requirement) != manifest.DependencyRequirementRequired {
			continue
		}
		if needsHomebrew(item) {
			report.Dependencies = append(report.Dependencies, item.Dependency)
		}
	}
	if len(report.Dependencies) == 0 {
		return report
	}
	report.Status = StatusWouldOffer
	report.Command = HomebrewInstallerCommand()
	report.Reason = "selected required Dependencies need Homebrew, but brew is not available"
	return report
}

func requirementValue(requirement string) string {
	if requirement == "" {
		return manifest.DependencyRequirementRequired
	}
	return requirement
}

func needsHomebrew(item deps.InstallPreview) bool {
	if item.Provider == deps.TierHomebrew || item.Executable == "brew" {
		return true
	}
	for _, candidate := range item.Candidates {
		if candidate.Provider == deps.TierHomebrew {
			return true
		}
	}
	return false
}

type Runner interface {
	Run(ctx context.Context, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, executable string, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func RunHomebrewSetup(ctx context.Context, runner Runner, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	if runner == nil {
		return errors.New("homebrew setup runner unavailable")
	}
	cmd := HomebrewInstallerCommand()
	return runner.Run(ctx, cmd.Executable, append([]string(nil), cmd.Args...), stdin, stdout, stderr)
}
