// Package deps computes Dependency findings for a Profile: which external tools
// a workstation needs for its selected Managed Entries and Provisioners, whether
// they are present, OS-aware advisory guidance for installing the missing ones,
// and explicitly confirmed package-manager execution.
package deps

import (
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selection"
)

// Lookup reports whether a command is available on the workstation. It is the
// injectable boundary around PATH probing so dependency checks stay deterministic
// in tests.
type Lookup func(command string) bool

// FontLookup reports whether a font matching the declared glob is installed on
// the workstation. It is the injectable boundary around font-directory scanning,
// parallel to Lookup, so font dependency checks stay deterministic in tests.
type FontLookup func(match string) bool

// AppLookup reports whether a named macOS application bundle is installed.
// It is injected so tests never need to inspect the operator's Applications
// directories.
type AppLookup func(app string) bool

// CommandRunner executes a read-only tool probe and returns its combined output.
type CommandRunner func(command string, args ...string) (string, error)

// Options carries the resolved inputs needed to select Dependencies.
type Options struct {
	Profile   string
	Profiles  []string
	ExtraTags []string
	Selection *manifest.Selection
	OS        string
	Arch      string
	Home      string
	StateRoot string
	AppLookup AppLookup
}

// Result is the presence finding for a single declared Dependency.
type Result struct {
	Name        string `json:"name"`
	Requirement string `json:"requirement"`
	Command     string `json:"command"`
	Present     bool   `json:"present"`
	Warning     string `json:"warning,omitempty"`
	// ProbeDetail and Hint are advisory, human-prose strings with no stable
	// format (truncated probe output, English remediation text). They stay out of
	// the Agent Output Contract; the machine-meaningful signals are Present and
	// Warning.
	ProbeDetail string `json:"-"`
	Hint        string `json:"-"`
}

// CheckReport is the Dependency presence report for a Profile.
type CheckReport struct {
	Profile   string            `json:"profile,omitempty"`
	Profiles  []string          `json:"profiles,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Selection *selection.Report `json:"selection,omitempty"`
	Results   []Result          `json:"results"`
}

// HasFindings reports whether any declared Dependency is absent from the
// workstation, or present but degraded by a probe warning (such as a broken Git
// toolchain). Both are concerns doctor surfaces and the caller should act on.
func (r CheckReport) HasFindings() bool {
	for _, res := range r.Results {
		if !res.Present || res.Warning != "" {
			return true
		}
	}
	return false
}

// Check reports which Dependencies declared by the Profile's selected Managed
// Entries and Provisioners are present on the workstation. Dependencies are
// deduplicated by name in first-declared order.
func Check(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup) (CheckReport, error) {
	return CheckWithToolProbes(m, opts, look, fontLook, nil)
}

// CheckWithToolProbes reports Dependency presence and, for selected tools whose
// PATH presence is not enough, runs read-only executable probes.
func CheckWithToolProbes(m manifest.Manifest, opts Options, look Lookup, fontLook FontLookup, run CommandRunner) (CheckReport, error) {
	selected, err := selectDependencies(m, opts)
	if err != nil {
		return CheckReport{}, err
	}

	selection, _ := resolveOptionsSelection(m, opts)
	report := CheckReport{Profile: selection.Profile, Profiles: selection.Profiles, Tags: selection.Tags}
	for _, dep := range selected {
		result := checkResult(dep, opts, look, fontLook)
		probeToolchain(&result, opts, run)
		report.Results = append(report.Results, result)
	}
	return report, nil
}

// DependencyPresent reports whether any declared detection mode satisfies a
// Dependency in the selected environment.
func DependencyPresent(dep manifest.Dependency, opts Options, look Lookup, fontLook FontLookup) bool {
	if dep.IsFont() {
		return fontPresent(dep.FontMatches(), fontLook)
	}
	return commandsPresent(dep.Probes(), look) || darwinAppPresent(dep.DarwinApp, opts)
}

func checkResult(dep manifest.Dependency, opts Options, look Lookup, fontLook FontLookup) Result {
	if dep.IsFont() {
		// A font has no executable on PATH; detect it as an installed asset
		// by scanning the workstation font directories for any compatible file
		// pattern, starting with the primary match.
		matches := dep.FontMatches()
		return Result{Name: dep.Name, Requirement: dep.RequirementValue(), Command: fontProbeLabel(matches), Present: fontPresent(matches, fontLook)}
	}
	probes := dep.Probes()
	return Result{Name: dep.Name, Requirement: dep.RequirementValue(), Command: probeLabel(probes), Present: commandsPresent(probes, look) || darwinAppPresent(dep.DarwinApp, opts)}
}

func darwinAppPresent(app string, opts Options) bool {
	app = strings.TrimSpace(app)
	return opts.OS == "darwin" && app != "" && opts.AppLookup != nil && opts.AppLookup(app)
}

const maxProbeDetailLen = 240

func probeToolchain(result *Result, opts Options, run CommandRunner) {
	if run == nil || !result.Present {
		return
	}

	switch result.Command {
	case "git":
		probeGitToolchain(result, opts, run)
	case "claude":
		probeClaudeCode(result, run)
	case "tmux":
		probeTmuxToolchain(result, run)
	}
}

func probeGitToolchain(result *Result, opts Options, run CommandRunner) {
	output, err := run("git", "--version")
	if err == nil {
		return
	}

	result.Warning = "git resolved on PATH but `git --version` failed"
	result.ProbeDetail = probeDetail(output, err)
	if opts.OS == "darwin" && strings.Contains(output, "xcrun: error: invalid active developer path") {
		result.Hint = "Repair Xcode Command Line Tools with `xcode-select --install` or reinstall them, then rerun `dots doctor`."
	}
}

func probeClaudeCode(result *Result, run CommandRunner) {
	output, err := run("claude", "--version")
	if err == nil {
		return
	}

	result.Warning = "claude resolved on PATH but `claude --version` failed"
	result.ProbeDetail = probeDetail(output, err)
	result.Hint = "Repair Claude Code's native binary install from its package directory with `node install.cjs`, then verify with `claude --version` and rerun `dots doctor`."
}

func probeTmuxToolchain(result *Result, run CommandRunner) {
	output, err := run("tmux", "-V")
	if err != nil {
		return
	}

	version := tmuxVersion(output)
	if version == "3.7" || version == "3.7a" {
		result.Warning = "tmux " + version + " has a known synchronized-update redraw regression"
		result.ProbeDetail = probeDetail(output, nil)
		result.Hint = "Upgrade tmux to 3.7b or newer, then stop old servers with `tmux kill-server` so new sessions use the fixed binary."
	}
}

func tmuxVersion(output string) string {
	fields := strings.Fields(output)
	if len(fields) < 2 || fields[0] != "tmux" {
		return ""
	}
	return fields[1]
}

func probeDetail(output string, err error) string {
	source := output
	if strings.TrimSpace(source) == "" && err != nil {
		source = err.Error()
	}
	detail := strings.Join(strings.Fields(source), " ")
	if len(detail) <= maxProbeDetailLen {
		return detail
	}
	return detail[:maxProbeDetailLen-len("...")] + "..."
}

func commandsPresent(commands []string, look Lookup) bool {
	for _, command := range commands {
		if !look(command) {
			return false
		}
	}
	return true
}

func probeLabel(probes []string) string {
	return strings.Join(probes, ", ")
}

func fontPresent(matches []string, fontLook FontLookup) bool {
	for _, match := range matches {
		if fontLook(match) {
			return true
		}
	}
	return false
}

func fontProbeLabel(matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	return strings.Join(matches, ", ")
}

// selectDependencies gathers the Dependencies declared directly by the Profile,
// tag-scoped Dependency Sets, then every Managed Entry and Provisioner that
// belongs to the Profile and passes the OS filter, deduplicated by name in
// first-declared order.

func selectDependencies(m manifest.Manifest, opts Options) ([]manifest.Dependency, error) {
	selection, err := resolveOptionsSelection(m, opts)
	if err != nil {
		return nil, err
	}

	var selected []manifest.Dependency
	seen := make(map[string]bool)
	addDependencies := func(deps []manifest.Dependency) {
		for _, dep := range deps {
			// Normalize the name so padded and unpadded declarations of the same
			// dependency deduplicate and render consistently with Probe().
			dep.Name = strings.TrimSpace(dep.Name)
			if seen[dep.Name] {
				if dep.IsRequired() {
					for i := range selected {
						if selected[i].Name == dep.Name && !selected[i].IsRequired() {
							selected[i].Requirement = manifest.DependencyRequirementRequired
							break
						}
					}
				}
				continue
			}
			seen[dep.Name] = true
			selected = append(selected, dep)
		}
	}

	for _, profileName := range selection.Profiles {
		addDependencies(m.Profiles[profileName].Dependencies)
	}
	tags := selection.Tags
	for _, set := range m.Dependencies {
		if !manifest.SharesTag(set.Tags, tags) {
			continue
		}
		if !manifest.MatchesOS(set.OS, opts.OS) {
			continue
		}
		addDependencies(set.Dependencies)
	}
	for _, entry := range m.Entries {
		if !manifest.SharesTag(entry.Tags, tags) {
			continue
		}
		if !manifest.MatchesOS(entry.OS, opts.OS) {
			continue
		}
		addDependencies(entry.Dependencies)
	}
	for _, prov := range m.Provisioners {
		if !manifest.SharesTag(prov.Tags, tags) {
			continue
		}
		if !manifest.MatchesOS(prov.OS, opts.OS) {
			continue
		}
		addDependencies(prov.Dependencies)
	}
	return selected, nil
}

func resolveOptionsSelection(m manifest.Manifest, opts Options) (manifest.Selection, error) {
	if opts.Selection != nil {
		return *opts.Selection, nil
	}
	return manifest.ResolveSelection(m, manifest.SelectedProfileNames(opts.Profile, opts.Profiles), opts.ExtraTags)
}
