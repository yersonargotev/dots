package provision

import (
	"fmt"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

// RunStatus describes the outcome of attempting one Provisioner.
type RunStatus string

const (
	// RunStatusProvisioned means the tool ran to completion successfully.
	RunStatusProvisioned RunStatus = "provisioned"
	// RunStatusMissingDeps means one or more declared dependencies were absent,
	// so the tool was deliberately not invoked.
	RunStatusMissingDeps RunStatus = "missing-dependencies"
	// RunStatusFailed means the tool was invoked but exited with an error.
	RunStatusFailed RunStatus = "failed"
)

// RunItem is the outcome of one attempted Provisioner.
type RunItem struct {
	Tool       string    `json:"tool"`
	Executable string    `json:"executable"`
	Args       []string  `json:"args"`
	Status     RunStatus `json:"status"`
	// Missing holds the probes of absent dependencies when Status is
	// RunStatusMissingDeps.
	Missing []string `json:"missing,omitempty"`
}

// Report records the outcome of an Apply run. A Report is always returned, even
// on failure, so the caller can show exactly how far provisioning got and never
// leaves the run half-applied without an account of it.
type Report struct {
	Profile  string    `json:"profile,omitempty"`
	Profiles []string  `json:"profiles,omitempty"`
	Tags     []string  `json:"tags,omitempty"`
	Items    []RunItem `json:"items"`
}

// Apply runs every selected Provisioner in manifest order via the deps Runner
// boundary. Each Provisioner's declared dependencies are probed first; if any
// are absent the tool is not invoked and the run stops with a clear failure.
// HOME threading for sandboxing is the concrete Runner's responsibility, not
// this orchestration. Apply stops at the first failing Provisioner and returns
// the partial Report alongside the error.
func Apply(m manifest.Manifest, opts Options, look deps.Lookup, fontLook deps.FontLookup, runner deps.Runner) (Report, error) {
	selected, err := Select(m, opts)
	if err != nil {
		return Report{}, err
	}

	selection, _ := manifest.ResolveSelection(m, manifest.SelectedProfileNames(opts.Profile, opts.Profiles), opts.ExtraTags)
	report := Report{Profile: selection.Profile, Profiles: selection.Profiles, Tags: selection.Tags}
	for _, prov := range selected {
		executable, args := RenderCommand(prov)

		if missing := missingDependencies(prov, look, fontLook); len(missing) > 0 {
			report.Items = append(report.Items, RunItem{
				Tool:       prov.Tool,
				Executable: executable,
				Args:       args,
				Status:     RunStatusMissingDeps,
				Missing:    missing,
			})
			return report, fmt.Errorf("provisioner %q is missing dependencies: %v", prov.Tool, missing)
		}

		if err := runner.Run(executable, args); err != nil {
			report.Items = append(report.Items, RunItem{
				Tool:       prov.Tool,
				Executable: executable,
				Args:       args,
				Status:     RunStatusFailed,
			})
			return report, fmt.Errorf("provisioner %q failed: %w", prov.Tool, err)
		}

		report.Items = append(report.Items, RunItem{
			Tool:       prov.Tool,
			Executable: executable,
			Args:       args,
			Status:     RunStatusProvisioned,
		})
	}
	return report, nil
}

// missingDependencies returns the probes of the Provisioner's declared
// dependencies that are absent on the workstation, in declared order.
func missingDependencies(prov manifest.Provisioner, look deps.Lookup, fontLook deps.FontLookup) []string {
	var missing []string
	for _, dep := range prov.Dependencies {
		if dep.IsFont() {
			matches := dep.FontMatches()
			if !fontDependencyPresent(matches, fontLook) {
				missing = append(missing, fontDependencyLabel(matches))
			}
			continue
		}
		probe := dep.Probe()
		if !look(probe) {
			missing = append(missing, probe)
		}
	}
	return missing
}

func fontDependencyPresent(matches []string, fontLook deps.FontLookup) bool {
	for _, match := range matches {
		if fontLook(match) {
			return true
		}
	}
	return false
}

func fontDependencyLabel(matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}
