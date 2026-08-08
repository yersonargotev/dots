// Package provision drives allowlisted external configuration tools declared in
// the Install Manifest. dots versions the
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
const codeGraphInstallScript = `set -eu
if ! command -v codegraph >/dev/null 2>&1; then
	curl -fsSL https://raw.githubusercontent.com/colbymchenry/codegraph/main/install.sh | sh
fi
export PATH="$HOME/.local/bin:$PATH"
codegraph install --target "$1" --location "$2" --yes`
const zimfwInstallScript = `set -e
ZDOTDIR="${ZDOTDIR:-${HOME}}"
ZIM_HOME="${ZIM_HOME:-${ZDOTDIR}/.zim}"
ZIM_CONFIG_FILE="${ZIM_CONFIG_FILE:-${ZDOTDIR}/.zimrc}"
export ZDOTDIR ZIM_HOME ZIM_CONFIG_FILE

mkdir -p "${ZIM_HOME}"
if [[ ! -e "${ZIM_HOME}/zimfw.zsh" ]]; then
	curl -fsSL -o "${ZIM_HOME}/zimfw.zsh" https://github.com/zimfw/zimfw/releases/latest/download/zimfw.zsh
fi

source "${ZIM_HOME}/zimfw.zsh" init -q`

// RenderCommand resolves a Provisioner into the exact executable and argv that
// would run it. It is PURE: it performs no I/O and never invokes the tool, so it
// is safe to render in a dry-run. The tool name is the binary name, enforced by
// the manifest allowlist before this function is reached.
func RenderCommand(p manifest.Provisioner) (executable string, args []string) {
	switch p.Tool {
	case "claude":
		return p.Tool, renderClaudeArgs(p.Spec)
	case "codegraph":
		return "sh", renderCodeGraphArgs(p.Spec)
	case "codex":
		return p.Tool, renderCodexArgs(p.Spec)
	case "skills":
		return "npx", renderSkillsArgs(p.Spec)
	case "zimfw":
		return "zsh", []string{"-c", zimfwInstallScript}
	default:
		return "", nil
	}
}

// renderCodeGraphArgs renders one non-interactive CodeGraph installer run. The
// fixed shell script follows CodeGraph's official bootstrap path when the binary
// is absent, then runs the installed CLI to wire MCP config for selected agents.
func renderCodeGraphArgs(spec manifest.ProvisionerSpec) []string {
	target := joinNonEmpty(spec.Agents)
	location := strings.TrimSpace(spec.Scope)
	if location == "" {
		location = "global"
	}
	return []string{"-c", codeGraphInstallScript, "codegraph-install", target, location}
}

// renderClaudeArgs renders one idempotent `claude` invocation. A marketplace
// spec registers a plugin marketplace from its source, a plugin spec installs
// `<plugin>@<from>` into the user scope, and an MCP spec registers a stdio MCP
// server. Validation guarantees exactly one shape is set before this is reached.
func renderClaudeArgs(spec manifest.ProvisionerSpec) []string {
	if marketplace := strings.TrimSpace(spec.Marketplace); marketplace != "" {
		return []string{"plugin", "marketplace", "add", marketplace}
	}
	if mcp := strings.TrimSpace(spec.MCP); mcp != "" {
		args := []string{"mcp", "add", "--transport", "stdio", mcp, "--"}
		return append(args, cleanList(spec.Command)...)
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
