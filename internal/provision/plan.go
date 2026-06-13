package provision

import (
	"fmt"

	"github.com/yersonargotev/dots/internal/manifest"
)

// Options carries the resolved inputs needed to select and plan Provisioners.
type Options struct {
	Profile string
	OS      string
}

// Step is a single planned Provisioner invocation: the exact resolved command
// plus the HOME-relative roots the tool will affect, shown so the user can judge
// the blast radius before confirming.
type Step struct {
	Tool       string
	Executable string
	Args       []string
	Targets    []string
}

// Plan is the preview of Provisioner steps the installer would run for a
// Profile, in manifest order.
type Plan struct {
	Profile string
	Steps   []Step
}

// Select gathers the Provisioners that belong to the Profile (their tags
// intersect the Profile's tags) and pass the OS filter, preserving manifest
// order. It mirrors the Entry selection used by deps, plan, and status.
func Select(m manifest.Manifest, opts Options) ([]manifest.Provisioner, error) {
	profile, ok := m.Profiles[opts.Profile]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", opts.Profile)
	}

	var selected []manifest.Provisioner
	for _, prov := range m.Provisioners {
		if !sharesTag(prov.Tags, profile.Tags) {
			continue
		}
		if !matchesOS(prov.OS, opts.OS) {
			continue
		}
		selected = append(selected, prov)
	}
	return selected, nil
}

// Build resolves every selected Provisioner into its exact command and the
// roots it affects. It performs no I/O and never invokes the tool, so it is safe
// to render in a dry-run. It mirrors plan.Build for Managed Entries.
func Build(m manifest.Manifest, opts Options) (Plan, error) {
	selected, err := Select(m, opts)
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{Profile: opts.Profile}
	for _, prov := range selected {
		executable, args := RenderCommand(prov)
		plan.Steps = append(plan.Steps, Step{
			Tool:       prov.Tool,
			Executable: executable,
			Args:       args,
			Targets:    managedRoots(prov),
		})
	}
	return plan, nil
}

// managedRoots returns the well-known HOME-relative roots an allowlisted tool
// manages, used as the advisory blast radius in the plan. gentle-ai owns its own
// state under ~/.gentle-ai, the Claude agent layer under ~/.claude, and selected
// agent-specific configuration roots.
func managedRoots(prov manifest.Provisioner) []string {
	switch prov.Tool {
	case "gentle-ai":
		roots := []string{"~/.claude"}
		if includes(prov.Spec.Agents, "codex") {
			roots = append(roots, "~/.codex")
		}
		return append(roots, "~/.gentle-ai")
	default:
		return nil
	}
}

func includes(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sharesTag(provTags, profileTags []string) bool {
	for _, pt := range provTags {
		for _, prt := range profileTags {
			if pt == prt {
				return true
			}
		}
	}
	return false
}

func matchesOS(provOS []string, current string) bool {
	if len(provOS) == 0 {
		return true
	}
	for _, osName := range provOS {
		if osName == current {
			return true
		}
	}
	return false
}
