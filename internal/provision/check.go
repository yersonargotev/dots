package provision

import (
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
)

// Readiness is the read-only diagnostic for one selected Provisioner: its exact
// resolved command and which declared dependencies are absent. It never runs the
// tool.
type Readiness struct {
	Tool       string   `json:"tool"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	// Missing holds the probes of absent dependencies, in declared order. An
	// empty Missing means the Provisioner is ready to run.
	Missing []string `json:"missing"`
}

// CheckReport is the readiness report for a Profile's selected Provisioners, in
// manifest order. It mirrors deps.CheckReport for Managed Entry dependencies.
type CheckReport struct {
	Profile  string      `json:"profile,omitempty"`
	Profiles []string    `json:"profiles,omitempty"`
	Tags     []string    `json:"tags,omitempty"`
	Items    []Readiness `json:"items"`
}

// HasFindings reports whether any selected Provisioner is missing a dependency
// and is therefore not ready to run.
func (r CheckReport) HasFindings() bool {
	for _, item := range r.Items {
		if len(item.Missing) > 0 {
			return true
		}
	}
	return false
}

// Check reports each selected Provisioner's resolved command and dependency
// readiness without invoking any tool. It is the read-only diagnostic surfaced
// by dots doctor.
func Check(m manifest.Manifest, opts Options, look deps.Lookup, fontLook deps.FontLookup) (CheckReport, error) {
	selected, err := Select(m, opts)
	if err != nil {
		return CheckReport{}, err
	}

	selection, _ := resolveOptionsSelection(m, opts)
	report := CheckReport{Profile: selection.Profile, Profiles: selection.Profiles, Tags: selection.Tags}
	for _, prov := range selected {
		executable, args := RenderCommand(prov)
		report.Items = append(report.Items, Readiness{
			Tool:       prov.Tool,
			Executable: executable,
			Args:       args,
			Missing:    missingDependencies(prov, opts, look, fontLook),
		})
	}
	return report, nil
}
