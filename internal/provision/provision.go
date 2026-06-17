// Package provision drives allowlisted external agent-configuration tools
// (initially gentle-ai) declared in the Install Manifest. dots versions the
// declarative invocation — the tool plus its flag spec — and renders it into an
// exact, resolved command. It never versions the intellectual layer the tool
// regenerates (skills, agents, personas, CLAUDE.md).
package provision

import (
	"strings"

	"github.com/yersonargotev/dots/internal/manifest"
)

// RenderCommand resolves a Provisioner into the exact executable and argv that
// would run it. It is PURE: it performs no I/O and never invokes the tool, so it
// is safe to render in a dry-run. The tool name is the binary name, enforced by
// the manifest allowlist before this function is reached.
func RenderCommand(p manifest.Provisioner) (executable string, args []string) {
	if p.Tool == "claude" {
		return p.Tool, renderClaudeArgs(p.Spec)
	}
	return p.Tool, renderGentleAIArgs(p.Spec)
}

// renderGentleAIArgs renders `install` plus gentle-ai's flags in a deterministic
// order; unset scalar flags and empty list flags are omitted, and list values
// are comma-joined.
func renderGentleAIArgs(spec manifest.ProvisionerSpec) []string {
	args := []string{"install"}
	args = appendScalarFlag(args, "--scope", spec.Scope)
	args = appendScalarFlag(args, "--channel", spec.Channel)
	args = appendScalarFlag(args, "--persona", spec.Persona)
	args = appendScalarFlag(args, "--sdd-mode", spec.SDDMode)
	args = appendListFlag(args, "--agents", spec.Agents)
	args = appendListFlag(args, "--components", spec.Components)
	args = appendListFlag(args, "--skills", spec.Skills)
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
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, ",")
}
