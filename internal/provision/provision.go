// Package provision drives allowlisted external agent-configuration tools
// (initially gentle-ai) declared in the Install Manifest. dots versions the
// declarative invocation — the tool plus its flag spec — and renders it into an
// exact, resolved command. It never versions the intellectual layer the tool
// regenerates (skills, agents, personas, CLAUDE.md).
package provision

import (
	"sort"
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

const skillsCLIPackage = "skills@1.5.12"

// RenderCommand resolves a Provisioner into the exact executable and argv that
// would run it. It is PURE: it performs no I/O and never invokes the tool, so it
// is safe to render in a dry-run. The tool name is the binary name, enforced by
// the manifest allowlist before this function is reached.
func RenderCommand(p manifest.Provisioner) (executable string, args []string) {
	switch p.Tool {
	case "claude":
		return p.Tool, renderClaudeArgs(p.Spec)
	case "codex":
		return p.Tool, renderCodexArgs(p.Spec)
	case "skills":
		return "npx", renderSkillsArgs(p.Spec)
	default:
		return p.Tool, renderGentleAIArgs(p.Spec)
	}
}

// renderGentleAIArgs renders the selected gentle-ai action plus its flags in a
// deterministic order; unset scalar flags and empty list flags are omitted, and
// list values are comma-joined. The action defaults to install for existing
// manifests that only declare install flags.
func renderGentleAIArgs(spec manifest.ProvisionerSpec) []string {
	action := strings.TrimSpace(spec.Action)
	if action == "" {
		action = "install"
	}
	args := []string{action}
	args = appendScalarFlag(args, "--scope", spec.Scope)
	args = appendScalarFlag(args, "--channel", spec.Channel)
	args = appendScalarFlag(args, "--persona", spec.Persona)
	args = appendScalarFlag(args, "--preset", spec.Preset)
	args = appendScalarFlag(args, "--sdd-mode", spec.SDDMode)
	args = appendListFlag(args, "--agents", spec.Agents)
	args = appendListFlag(args, "--components", spec.Components)
	args = appendListFlag(args, "--skills", spec.Skills)
	if spec.Yes {
		args = append(args, "--yes")
	}
	return args
}

// renderClaudeArgs renders one idempotent `claude` invocation. A marketplace
// spec registers a plugin marketplace from its source; otherwise a plugin spec
// installs `<plugin>@<from>` into the user scope. Validation guarantees exactly
// one shape is set before this is reached.
func renderClaudeArgs(spec manifest.ProvisionerSpec) []string {
	if marketplace := strings.TrimSpace(spec.Marketplace); marketplace != "" {
		return []string{"plugin", "marketplace", "add", marketplace}
	}
	ref := strings.TrimSpace(spec.Plugin) + "@" + strings.TrimSpace(spec.From)
	return []string{"plugin", "install", ref, "--scope", "user"}
}

// renderCodexArgs renders one idempotent `codex mcp add` invocation: the MCP
// server name, any environment flags in sorted-key order, and the launch command
// after the `--` separator. Sorting env keys keeps the rendered command
// deterministic. Validation guarantees MCP and Command are set before this is
// reached.
func renderCodexArgs(spec manifest.ProvisionerSpec) []string {
	args := []string{"mcp", "add", strings.TrimSpace(spec.MCP)}
	keys := make([]string, 0, len(spec.Env))
	for key := range spec.Env {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", strings.TrimSpace(key)+"="+strings.TrimSpace(spec.Env[key]))
	}
	args = append(args, "--")
	return append(args, cleanList(spec.Command)...)
}

// renderSkillsArgs renders one allowlisted skills.sh install through a pinned
// npm CLI package, with deterministic repeated flags for selected agents and
// skill names. Validation guarantees Package is set before this is reached.
func renderSkillsArgs(spec manifest.ProvisionerSpec) []string {
	args := []string{"--yes", skillsCLIPackage, "add", strings.TrimSpace(spec.Package)}
	for _, agent := range cleanList(spec.Agents) {
		args = append(args, "--agent", agent)
	}
	for _, skill := range cleanList(spec.Skills) {
		args = append(args, "--skill", skill)
	}
	if spec.Global {
		args = append(args, "--global")
	}
	if spec.Copy {
		args = append(args, "--copy")
	}
	return args
}

func appendScalarFlag(args []string, flag, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return args
	}
	return append(args, flag, value)
}

func appendListFlag(args []string, flag string, values []string) []string {
	joined := joinNonEmpty(values)
	if joined == "" {
		return args
	}
	return append(args, flag, joined)
}

func joinNonEmpty(values []string) string {
	return strings.Join(cleanList(values), ",")
}

// cleanList trims surrounding whitespace from each value and drops the entries
// that are empty afterwards, preserving order.
func cleanList(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
