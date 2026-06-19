package provision

import (
	"fmt"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/profilesel"
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
	indices, err := selectedIndices(m, opts.Profile, opts.OS)
	if err != nil {
		return nil, err
	}

	var selected []manifest.Provisioner
	for i, prov := range m.Provisioners {
		if indices[i] {
			selected = append(selected, prov)
		}
	}
	return selected, nil
}

// selectedIndices returns the set of m.Provisioners positions a profile would
// select on the given OS. Working in index space lets callers reason about
// provisioner identity across profiles (which provisioner, not just how many)
// without depending on struct equality, and keeps Select and SkippedProvisioners
// filtering through one tag/OS rule.
func selectedIndices(m manifest.Manifest, profileName, os string) (map[int]bool, error) {
	profile, ok := m.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}

	indices := make(map[int]bool)
	for i, prov := range m.Provisioners {
		if manifest.SharesTag(prov.Tags, profile.Tags) && manifest.MatchesOS(prov.OS, os) {
			indices[i] = true
		}
	}
	return indices, nil
}

// SkippedProvisioners reports whether the active profile omits provisioners that
// another profile would select on this OS, and which single profile best recovers
// them. It is a thin adapter over profilesel.Skipped, injecting the provisioner
// index selection; plan.SkippedEntries is its file-entry twin over the same
// shared math. It is PURE: no I/O, safe in a dry-run, and mirrors the tag/OS
// scoping used by Select.
func SkippedProvisioners(m manifest.Manifest, opts Options) (profilesel.Hint, bool, error) {
	return profilesel.Skipped(m.Profiles, opts.Profile, opts.OS, func(name, os string) (map[int]bool, error) {
		return selectedIndices(m, name, os)
	})
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
// agent-specific configuration roots. claude writes marketplace and plugin state
// under ~/.claude and the user MCP/plugin registry in ~/.claude.json. codex
// records MCP servers in ~/.codex/config.toml, under ~/.codex.
func managedRoots(prov manifest.Provisioner) []string {
	switch prov.Tool {
	case "gentle-ai":
		roots := []string{"~/.claude"}
		if includes(prov.Spec.Agents, "codex") {
			roots = append(roots, "~/.codex")
		}
		if includes(prov.Spec.Agents, "opencode") {
			roots = append(roots, "~/.config/opencode")
		}
		return append(roots, "~/.gentle-ai")
	case "claude":
		return []string{"~/.claude", "~/.claude.json"}
	case "codex":
		return []string{"~/.codex"}
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
