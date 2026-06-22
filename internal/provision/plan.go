package provision

import (
	"fmt"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/profilesel"
)

// Options carries the resolved inputs needed to select and plan Provisioners.
type Options struct {
	Profile   string
	ExtraTags []string
	OS        string
}

// Step is a single planned Provisioner invocation: the exact resolved command
// plus the HOME-relative roots the tool will affect, shown so the user can judge
// the blast radius before confirming.
type Step struct {
	Tool       string   `json:"tool"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	Targets    []string `json:"targets"`
}

// Plan is the preview of Provisioner steps the installer would run for a
// Profile, in manifest order.
type Plan struct {
	Profile string `json:"profile"`
	Steps   []Step `json:"steps"`
}

// Select gathers the Provisioners that belong to the Profile (their tags
// intersect the Profile's tags) and pass the OS filter, preserving manifest
// order. It mirrors the Entry selection used by deps, plan, and status.
func Select(m manifest.Manifest, opts Options) ([]manifest.Provisioner, error) {
	indices, err := selectedIndices(m, opts.Profile, opts.OS, opts.ExtraTags)
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
func selectedIndices(m manifest.Manifest, profileName, os string, extraTags []string) (map[int]bool, error) {
	profile, ok := m.Profiles[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", profileName)
	}

	tags := manifest.SelectionTags(profile, extraTags)

	indices := make(map[int]bool)
	for i, prov := range m.Provisioners {
		if manifest.SharesTag(prov.Tags, tags) && manifest.MatchesOS(prov.OS, os) {
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
		return selectedIndices(m, name, os, nil)
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
// state under ~/.gentle-ai plus the selected agent-specific configuration roots:
// ~/.claude for claude-code, ~/.codex for codex, ~/.config/opencode for opencode,
// ~/.gemini for antigravity, and VS Code user config for vscode-copilot. claude writes marketplace and plugin state under
// ~/.claude and the user MCP/plugin registry in ~/.claude.json. codex records MCP
// servers in ~/.codex/config.toml, under ~/.codex. codegraph writes its own
// installed versions and shim under ~/.codegraph and ~/.local/bin, plus MCP
// config and instructions for the selected agents. skills.sh installs global
// skills under the user-level agent skill directories selected by its --agent
// flags. For skills@1.5.12, codex and antigravity share ~/.agents/skills;
// claude-code uses ~/.claude/skills.
func managedRoots(prov manifest.Provisioner) []string {
	switch prov.Tool {
	case "gentle-ai":
		var roots []string
		if includes(prov.Spec.Agents, "claude-code") {
			roots = append(roots, "~/.claude")
		}
		if includes(prov.Spec.Agents, "codex") {
			roots = append(roots, "~/.codex")
		}
		if includes(prov.Spec.Agents, "opencode") {
			roots = append(roots, "~/.config/opencode")
		}
		if includes(prov.Spec.Agents, "antigravity") {
			roots = append(roots, "~/.gemini")
		}
		if includes(prov.Spec.Agents, "vscode-copilot") {
			roots = append(roots, "~/Library/Application Support/Code/User")
		}
		return append(roots, "~/.gentle-ai")
	case "claude":
		return []string{"~/.claude", "~/.claude.json"}
	case "codex":
		return []string{"~/.codex"}
	case "codegraph":
		return codeGraphRoots(prov.Spec.Agents)
	case "skills":
		return skillsRoots(prov.Spec.Agents)
	default:
		return nil
	}
}

func codeGraphRoots(agents []string) []string {
	roots := []string{"~/.codegraph", "~/.local/bin"}
	seen := map[string]bool{
		"~/.codegraph": true,
		"~/.local/bin": true,
	}
	cleanAgents := cleanAgentList(agents)
	if len(cleanAgents) == 0 {
		return append(roots, "~/.codex", "~/.claude", "~/.claude.json", "~/.config/opencode", "~/.gemini")
	}
	add := func(root string) {
		if seen[root] {
			return
		}
		seen[root] = true
		roots = append(roots, root)
	}
	for _, agent := range cleanAgents {
		switch agent {
		case "codex":
			add("~/.codex")
		case "claude":
			add("~/.claude")
			add("~/.claude.json")
		case "opencode":
			add("~/.config/opencode")
		case "antigravity":
			add("~/.gemini")
		}
	}
	return roots
}

func skillsRoots(agents []string) []string {
	cleanAgents := cleanAgentList(agents)
	if len(cleanAgents) == 0 {
		return []string{"~/.agents/skills"}
	}
	roots := make([]string, 0, len(cleanAgents))
	seen := map[string]bool{}
	for _, agent := range cleanAgents {
		root := "~/.agents/skills"
		if agent == "claude-code" {
			root = "~/.claude/skills"
		}
		if seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
}

func cleanAgentList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func includes(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
