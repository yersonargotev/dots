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
// is safe to render in a dry-run. Flags are emitted in a deterministic order;
// unset scalar flags and empty list flags are omitted, and list values are
// comma-joined. The tool name is the binary name (gentle-ai), enforced by the
// manifest allowlist before this function is reached.
func RenderCommand(p manifest.Provisioner) (executable string, args []string) {
	args = []string{"install"}
	args = appendScalarFlag(args, "--scope", p.Spec.Scope)
	args = appendScalarFlag(args, "--channel", p.Spec.Channel)
	args = appendScalarFlag(args, "--persona", p.Spec.Persona)
	args = appendScalarFlag(args, "--sdd-mode", p.Spec.SDDMode)
	args = appendListFlag(args, "--agents", p.Spec.Agents)
	args = appendListFlag(args, "--components", p.Spec.Components)
	args = appendListFlag(args, "--skills", p.Spec.Skills)
	return p.Tool, args
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
