package provision

import (
	"fmt"
	"sort"

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
		if sharesTag(prov.Tags, profile.Tags) && matchesOS(prov.OS, os) {
			indices[i] = true
		}
	}
	return indices, nil
}

// SkippedHint describes provisioners the active profile omits that some other
// profile would select on this OS, so the CLI can nudge the user toward the
// fuller profile instead of silently dropping them.
type SkippedHint struct {
	// Profile is the active profile being installed.
	Profile string
	// Count is how many of the skipped provisioners SuggestedProfile would
	// recover. It is intentionally the suggested profile's coverage, not the
	// union of omissions across every profile, so the "run --profile S to
	// include them" nudge is always exact and never promises more than S
	// delivers. In the nested-profile model dots uses (e.g. desktop ⊇ default)
	// the most complete profile recovers everything, so Count equals the total
	// the active profile omits.
	Count int
	// SuggestedProfile is the other profile that covers the most skipped
	// provisioners — the single most complete profile worth recommending.
	SuggestedProfile string
}

// SkippedProvisioners reports whether the active profile omits provisioners that
// another profile would select on this OS, and which single profile best
// recovers them. The second return is false (with a zero hint) when nothing is
// skipped — either the active profile already selects everything, or the OS
// filter excludes the extras for every profile so switching profiles would not
// recover them. The reported Count is the suggested profile's coverage of the
// skipped set, so the caller's remediation message stays accurate even when no
// single profile is a superset of every omission. It is PURE: no I/O, safe in a
// dry-run, and mirrors the tag/OS scoping used by Select.
func SkippedProvisioners(m manifest.Manifest, opts Options) (SkippedHint, bool, error) {
	active, err := selectedIndices(m, opts.Profile, opts.OS)
	if err != nil {
		return SkippedHint{}, false, err
	}

	others := make([]string, 0, len(m.Profiles))
	for name := range m.Profiles {
		if name != opts.Profile {
			others = append(others, name)
		}
	}
	// Sort so the suggested profile is deterministic on ties (first by name).
	sort.Strings(others)

	selections := make(map[string]map[int]bool, len(others))
	skipped := make(map[int]bool)
	for _, name := range others {
		sel, err := selectedIndices(m, name, opts.OS)
		if err != nil {
			return SkippedHint{}, false, err
		}
		selections[name] = sel
		for i := range sel {
			if !active[i] {
				skipped[i] = true
			}
		}
	}

	if len(skipped) == 0 {
		return SkippedHint{}, false, nil
	}

	var (
		suggested string
		best      int
	)
	for _, name := range others {
		covered := 0
		for i := range selections[name] {
			if skipped[i] {
				covered++
			}
		}
		if covered > best {
			best = covered
			suggested = name
		}
	}

	// best is the suggested profile's coverage of the skipped set, and is >= 1
	// whenever skipped is non-empty: every skipped index came from some other
	// profile's selection, so that profile covers it. Reporting best (not
	// len(skipped)) keeps "run --profile S to include them" exact.
	return SkippedHint{Profile: opts.Profile, Count: best, SuggestedProfile: suggested}, true, nil
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
