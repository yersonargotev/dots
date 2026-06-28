package pkgmgr_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/deps/pkgmgr"
	"github.com/yersonargotev/dots/internal/manifest"
)

func TestDetectHomebrewFindsPATHBeforePrefixes(t *testing.T) {
	d := pkgmgr.Detector{
		LookPath: func(command string) (string, error) { return "/tmp/bin/brew", nil },
		Exists:   func(path string) bool { return true },
		Prefixes: []string{"/opt/homebrew/bin/brew"},
	}.DetectHomebrew()
	if !d.Found || d.Path != "/tmp/bin/brew" || d.NeedsPATH {
		t.Fatalf("DetectHomebrew() = %#v", d)
	}
}

func TestDetectHomebrewFindsExpectedPrefixAndReportsPATHGuidance(t *testing.T) {
	d := pkgmgr.Detector{
		LookPath: func(command string) (string, error) { return "", execErr{} },
		Exists:   func(path string) bool { return path == "/opt/homebrew/bin/brew" },
		Prefixes: []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"},
	}.DetectHomebrew()
	if !d.Found || d.Path != "/opt/homebrew/bin/brew" || !d.NeedsPATH || d.PATHGuidance == "" {
		t.Fatalf("DetectHomebrew() = %#v", d)
	}
}

type execErr struct{}

func (execErr) Error() string { return "missing" }

func TestHomebrewInstallerCommandIsOfficialCommand(t *testing.T) {
	cmd := pkgmgr.HomebrewInstallerCommand()
	if cmd.Display != `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"` {
		t.Fatalf("Display = %q", cmd.Display)
	}
	if cmd.Executable != "/bin/sh" || !reflect.DeepEqual(cmd.Args, []string{"-c", pkgmgr.OfficialHomebrewInstallerCommand}) {
		t.Fatalf("command = %#v", cmd)
	}
}

func TestHomebrewSetupNeedRequiresDarwinMissingBrewAndRequiredHomebrewDependency(t *testing.T) {
	preview := deps.InstallDryRunReport{Profile: "default", Tier: deps.TierHomebrew, Items: []deps.InstallPreview{{
		Dependency: "starship", Requirement: manifest.DependencyRequirementRequired, Status: deps.InstallPreviewManual,
		Candidates: []deps.ProviderCandidate{{Provider: deps.TierHomebrew, Package: "starship"}},
	}}}
	report := pkgmgr.HomebrewSetupNeed("darwin", preview, pkgmgr.HomebrewDetection{})
	if report.Status != pkgmgr.StatusWouldOffer || len(report.Dependencies) != 1 || report.Command.Display == "" {
		t.Fatalf("HomebrewSetupNeed() = %#v", report)
	}
	if got := pkgmgr.HomebrewSetupNeed("linux", preview, pkgmgr.HomebrewDetection{}); got.Status != pkgmgr.StatusNotNeeded {
		t.Fatalf("linux setup = %#v, want not-needed", got)
	}
	preview.Items[0].Requirement = manifest.DependencyRequirementOptional
	if got := pkgmgr.HomebrewSetupNeed("darwin", preview, pkgmgr.HomebrewDetection{}); got.Status != pkgmgr.StatusNotNeeded {
		t.Fatalf("optional setup = %#v, want not-needed", got)
	}
}
