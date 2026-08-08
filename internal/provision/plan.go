package provision

import (
	"strings"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/profilesel"
)

// Options carries the resolved inputs needed to select and plan Provisioners.
type Options struct {
	Profile   string
	Profiles  []string
	ExtraTags []string
	Selection *manifest.Selection
	OS        string
	AppLookup deps.AppLookup
}

// Step is a single planned Provisioner invocation: the exact resolved command
// plus the HOME-relative roots the tool will affect, shown so the user can judge
// the blast radius before confirming.
type Step struct {
	Tool        string   `json:"tool"`
	Executable  string   `json:"executable"`
	Args        []string `json:"args"`
	Targets     []string `json:"targets"`
	GlobalTools []string `json:"global_tools,omitempty"`
}

// Plan is the preview of Provisioner steps the installer would run for a
// Profile, in manifest order.
type Plan struct {
	Profile  string   `json:"profile,omitempty"`
	Profiles []string `json:"profiles,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Steps    []Step   `json:"steps"`
}

// Select gathers the Provisioners that belong to the Profile (their tags
// intersect the Profile's tags) and pass the OS filter, preserving manifest
// order. It mirrors the Entry selection used by deps, plan, and status.
func Select(m manifest.Manifest, opts Options) ([]manifest.Provisioner, error) {
	selection, err := resolveOptionsSelection(m, opts)
	if err != nil {
		return nil, err
	}
	indices := selectedIndicesForSelection(m, selection, opts.OS)

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
func selectedIndices(m manifest.Manifest, profileNames []string, os string, extraTags []string) (map[int]bool, error) {
	selection, err := manifest.ResolveSelection(m, profileNames, extraTags)
	if err != nil {
		return nil, err
	}
	return selectedIndicesForSelection(m, selection, os), nil
}

func selectedIndicesForSelection(m manifest.Manifest, selection manifest.Selection, os string) map[int]bool {
	indices := make(map[int]bool)
	for i, prov := range m.Provisioners {
		if manifest.SharesTag(prov.Tags, selection.Tags) && manifest.MatchesOS(prov.OS, os) {
			indices[i] = true
		}
	}
	return indices
}

// SkippedProvisioners reports whether the active profile omits provisioners that
// another profile would select on this OS, and which single profile best recovers
// them. It is a thin adapter over profilesel.Skipped, injecting the provisioner
// index selection; plan.SkippedEntries is its file-entry twin over the same
// shared math. It is PURE: no I/O, safe in a dry-run, and mirrors the tag/OS
// scoping used by Select.
func SkippedProvisioners(m manifest.Manifest, opts Options) (profilesel.Hint, bool, error) {
	active := selectionLabel(opts.Profile, opts.Profiles)
	if len(opts.Profiles) > 1 {
		return profilesel.SkippedSelection(m.Profiles, opts.Profiles, opts.OS, func(name, os string) (map[int]bool, error) {
			return selectedIndices(m, []string{name}, os, nil)
		})
	}
	return profilesel.Skipped(m.Profiles, active, opts.OS, func(name, os string) (map[int]bool, error) {
		return selectedIndices(m, []string{name}, os, nil)
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

	selection, _ := resolveOptionsSelection(m, opts)
	plan := Plan{Profile: selection.Profile, Profiles: selection.Profiles, Tags: selection.Tags}
	for _, prov := range selected {
		executable, args := RenderCommand(prov)
		plan.Steps = append(plan.Steps, Step{
			Tool:        prov.Tool,
			Executable:  executable,
			Args:        args,
			Targets:     managedRoots(prov),
			GlobalTools: globalTools(prov),
		})
	}
	return plan, nil
}

func resolveOptionsSelection(m manifest.Manifest, opts Options) (manifest.Selection, error) {
	if opts.Selection != nil {
		return *opts.Selection, nil
	}
	return manifest.ResolveSelection(m, manifest.SelectedProfileNames(opts.Profile, opts.Profiles), opts.ExtraTags)
}

// managedRoots returns the well-known HOME-relative roots an allowlisted tool
// manages, used as the advisory blast radius in the plan. claude writes marketplace and plugin state under
// ~/.claude and the user MCP/plugin registry in ~/.claude.json. codex records MCP
// servers in ~/.codex/config.toml, under ~/.codex. codegraph writes its own
// installed versions and shim under ~/.codegraph and ~/.local/bin, plus MCP
// config and instructions for the selected agents. skills.sh installs global
// skills under the user-level agent skill directories selected by its --agent
// flags. zimfw provisions the generated runtime under ~/.zim while ~/.zimrc
// remains a Managed Entry. Keep these roots aligned with the pinned skills@1.5.12
// agent registry: claude-code writes to ~/.claude/skills; codex, antigravity,
// opencode, and github-copilot write to ~/.agents/skills.
func managedRoots(prov manifest.Provisioner) []string {
	switch prov.Tool {
	case "claude":
		return []string{"~/.claude", "~/.claude.json"}
	case "codex":
		return []string{"~/.codex"}
	case "codegraph":
		return codeGraphRoots(prov.Spec.Agents)
	case "skills":
		return skillsRoots(prov.Spec.Agents)
	case "zimfw":
		return []string{"~/.zim"}
	default:
		return nil
	}
}

func globalTools(prov manifest.Provisioner) []string {
	switch prov.Tool {
	case "codegraph":
		return []string{"codegraph (~/.local/bin)"}
	}
	return nil
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
		root := skillsRoot(agent)
		if seen[root] {
			continue
		}
		seen[root] = true
		roots = append(roots, root)
	}
	return roots
}

func skillsRoot(agent string) string {
	switch agent {
	case "claude-code":
		return "~/.claude/skills"
	default:
		return "~/.agents/skills"
	}
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

func selectionLabel(profile string, profiles []string) string {
	if len(profiles) > 0 {
		return profiles[0]
	}
	if profile != "" {
		return profile
	}
	return "default"
}
