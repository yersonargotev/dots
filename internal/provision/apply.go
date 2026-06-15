package provision

import (
	"fmt"
	"strings"

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
	Tool       string
	Executable string
	Args       []string
	Status     RunStatus
	// Missing holds the probes of absent dependencies when Status is
	// RunStatusMissingDeps.
	Missing []string
}

// Report records the outcome of an Apply run. A Report is always returned, even
// on failure, so the caller can show exactly how far provisioning got and never
// leaves the run half-applied without an account of it.
type Report struct {
	Profile string
	Items   []RunItem
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

	report := Report{Profile: opts.Profile}
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
			match := strings.TrimSpace(dep.FontMatch)
			if !fontLook(match) {
				missing = append(missing, match)
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
