package manifest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestLoadFileAcceptsMinimalValidManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if got.Version != 1 {
		t.Fatalf("Version = %d, want 1", got.Version)
	}
	if len(got.Profiles) != 1 || len(got.Profiles["default"].Tags) != 1 || got.Profiles["default"].Tags[0] != "core" {
		t.Fatalf("Profiles = %#v, want default profile with core tag", got.Profiles)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("Entries len = %d, want 1", len(got.Entries))
	}
	entry := got.Entries[0]
	if entry.Source != "configs/zsh/zshrc" || entry.Target != "~/.zshrc" || entry.Strategy != "symlink" {
		t.Fatalf("Entry = %#v, want parsed source, target, and strategy", entry)
	}
}

func TestParseValidatesOptionalTagRegistry(t *testing.T) {
	valid := `version: 1
tags:
  core:
    description: shared baseline
    kind: surface
    status: current
  legacy-core:
    description: old baseline selector
    kind: surface
    status: legacy
    replaced_by: [core, agents]
  agents:
    description: retire Gentle AI state
    kind: surface
    status: current
profiles:
  default:
    description: default workstation
    status: current
    tags: [core]
dependencies:
  - tags: [core]
    dependencies: [{name: git}]
entries:
  - source: configs/zsh/zshrc
    source_overrides: {legacy-core: configs/zsh/legacy-zshrc}
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: claude
    tags: [agents]
    spec: {marketplace: example/tools}
`
	got, err := manifest.Parse([]byte(valid))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Profiles["default"].Description != "default workstation" || got.Profiles["default"].Status != "current" {
		t.Fatalf("Profile = %#v, want description and status", got.Profiles["default"])
	}
	if want := []string{"core", "agents"}; !reflect.DeepEqual([]string(got.Tags["legacy-core"].ReplacedBy), want) {
		t.Fatalf("legacy tag = %#v, want replacement", got.Tags["legacy-core"])
	}

	scalar := strings.Replace(valid, "replaced_by: [core, agents]", "replaced_by: core", 1)
	got, err = manifest.Parse([]byte(scalar))
	if err != nil {
		t.Fatalf("Parse() scalar replacement error = %v", err)
	}
	if want := []string{"core"}; !reflect.DeepEqual([]string(got.Tags["legacy-core"].ReplacedBy), want) {
		t.Fatalf("scalar legacy tag = %#v, want replacement %#v", got.Tags["legacy-core"], want)
	}

	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"profile reference", strings.Replace(valid, "tags: [core]\ndependencies:", "tags: [missing]\ndependencies:", 1), `profiles["default"].tags[0] tag "missing" is not declared`},
		{"dependency reference", strings.Replace(valid, "  - tags: [core]\n    dependencies", "  - tags: [missing]\n    dependencies", 1), `dependencies[0].tags[0] tag "missing" is not declared`},
		{"entry reference", strings.Replace(valid, "    tags: [core]\nprovisioners:", "    tags: [missing]\nprovisioners:", 1), `entries[0].tags[0] tag "missing" is not declared`},
		{"override key", strings.Replace(valid, "source_overrides: {legacy-core:", "source_overrides: {missing:", 1), `entries[0].source_overrides[0] tag "missing" is not declared`},
		{"provisioner reference", strings.Replace(valid, "    tags: [agents]\n    spec:", "    tags: [missing]\n    spec:", 1), `provisioners[0].tags[0] tag "missing" is not declared`},
		{"invalid kind", strings.Replace(valid, "kind: surface", "kind: command", 1), `tags["core"].kind must be one of surface, cleanup, compatibility`},
		{"legacy missing replacement", strings.Replace(valid, "    replaced_by: [core, agents]\n", "", 1), `tags["legacy-core"].replaced_by must contain at least one current tag`},
		{"empty replacement", strings.Replace(valid, "replaced_by: [core, agents]", `replaced_by: [core, ""]`, 1), `tags["legacy-core"].replaced_by[1] must not be empty`},
		{"duplicate replacement", strings.Replace(valid, "replaced_by: [core, agents]", "replaced_by: [core, core]", 1), `tags["legacy-core"].replaced_by[1] duplicates "core"`},
		{"replacement source current", strings.Replace(valid, "status: legacy\n    replaced_by", "status: current\n    replaced_by", 1), `tags["legacy-core"].replaced_by requires status legacy`},
		{"replacement target undeclared", strings.Replace(valid, "replaced_by: [core, agents]", "replaced_by: [missing]", 1), `tags["legacy-core"].replaced_by "missing" is not declared`},
		{"replacement target legacy chain", strings.Replace(valid, "    status: current\n  legacy-core:", "    status: legacy\n    replaced_by: agents\n  legacy-core:", 1), `tags["legacy-core"].replaced_by "core" must reference a current tag`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manifest.Parse([]byte(tt.content))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadPreviousFileProjectsRetiredProvisionerInventoryWithoutAcceptingItsDialect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dots.yaml")
	content := []byte(`version: 1
profiles:
  agents:
    tags: [agents]
entries:
  - source: configs/agents
    target: ~/.agents
    strategy: copy
    tags: [agents]
provisioners:
  - tool: gentle-ai
    tags: [agents]
    spec:
      action: install
      persona: senior-architect
      components: [engram]
    dependencies:
      - name: gentle-ai
      - name: engram
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	if _, err := manifest.LoadFile(path); err == nil || !strings.Contains(err.Error(), "field action not found") {
		t.Fatalf("LoadFile() error = %v, want retired dialect rejection", err)
	}
	got, err := manifest.LoadPreviousFile(path)
	if err != nil {
		t.Fatalf("LoadPreviousFile() error = %v", err)
	}
	if len(got.Provisioners) != 1 || got.Provisioners[0].Tool != "gentle-ai" {
		t.Fatalf("Provisioners = %#v, want retired tool inventory", got.Provisioners)
	}
	if !got.Provisioners[0].Spec.IsEmpty() {
		t.Fatalf("previous Provisioner spec = %#v, want discarded dialect", got.Provisioners[0].Spec)
	}
	wantDependencies := []manifest.Dependency{{Name: "gentle-ai"}, {Name: "engram"}}
	if !reflect.DeepEqual(got.Provisioners[0].Dependencies, wantDependencies) {
		t.Fatalf("Dependencies = %#v, want %#v", got.Provisioners[0].Dependencies, wantDependencies)
	}
}

func TestLoadFileRollingUserLocalAcceptsOnlyRecipe(t *testing.T) {
	base := `version: 1
profiles:
  default:
    tags: [agents]
entries:
  - source: configs/codex/config.toml
    target: ~/.codex/config.toml
    strategy: copy
    tags: [agents]
    dependencies:
      - name: codex
        command: codex
        rolling_user_local:
          recipe: codex
`
	path := filepath.Join(t.TempDir(), "dots.yaml")
	if err := os.WriteFile(path, []byte(base), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	policy := got.Entries[0].Dependencies[0].RollingUserLocal
	if policy == nil || policy.Recipe != "codex" {
		t.Fatalf("rolling policy = %#v", policy)
	}

	for _, field := range []string{"url", "checksum", "command", "installer"} {
		t.Run(field, func(t *testing.T) {
			badPath := filepath.Join(t.TempDir(), "dots.yaml")
			content := base + "          " + field + ": https://attacker.invalid/install\n"
			if err := os.WriteFile(badPath, []byte(content), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}
			if _, err := manifest.LoadFile(badPath); err == nil || !strings.Contains(err.Error(), "field "+field+" not found") {
				t.Fatalf("LoadFile() error = %v, want closed rolling policy rejection", err)
			}
		})
	}
}

func TestLoadFileParsesEntryOwnership(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/claude/settings.json
    target: ~/.claude/settings.json
    strategy: copy
    ownership: json-subset
    tags: [core]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if got.Entries[0].Ownership != "json-subset" {
		t.Fatalf("Entry.Ownership = %q, want json-subset", got.Entries[0].Ownership)
	}
}

func TestLoadFileParsesJSONCSubsetOwnership(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [desktop]
entries:
  - source: configs/zed/settings.json
    target: ~/.config/zed/settings.json
    strategy: copy
    ownership: jsonc-subset
    tags: [desktop]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if got.Entries[0].Ownership != "jsonc-subset" {
		t.Fatalf("Entry.Ownership = %q, want jsonc-subset", got.Entries[0].Ownership)
	}
}

func TestLoadFileParsesEntryDependencies(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - name: tmux
        brew: tmux
        apt: tmux
        dnf: tmux
        pacman: tmux
      - name: ripgrep
        requirement: optional
        command: rg
        brew: ripgrep
      - name: CascadiaCode Nerd Font
        brew_cask: font-cascadia-code-nf
        font_match: "CascadiaCodeNF*"
        font_fallback_matches:
          - "CaskaydiaCoveNerdFont*"
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	deps := got.Entries[0].Dependencies
	if len(deps) != 3 {
		t.Fatalf("Dependencies len = %d, want 3", len(deps))
	}
	if deps[0].Name != "tmux" || deps[0].Brew != "tmux" || deps[0].Apt != "tmux" || deps[0].Dnf != "tmux" || deps[0].Pacman != "tmux" {
		t.Fatalf("Dependencies[0] = %#v, want fully mapped tmux dependency", deps[0])
	}
	if deps[1].Name != "ripgrep" || deps[1].Requirement != "optional" || deps[1].Command != "rg" || deps[1].Brew != "ripgrep" {
		t.Fatalf("Dependencies[1] = %#v, want ripgrep with rg command", deps[1])
	}
	if deps[2].Name != "CascadiaCode Nerd Font" || deps[2].BrewCask != "font-cascadia-code-nf" || deps[2].FontMatch != "CascadiaCodeNF*" || !sameStrings(deps[2].FontFallbackMatches, []string{"CaskaydiaCoveNerdFont*"}) {
		t.Fatalf("Dependencies[2] = %#v, want Homebrew cask font dependency with fallback match", deps[2])
	}
}

func TestLoadFileRejectsProfileDependenciesWithMigrationGuidance(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  desktop:
    tags: [core, desktop]
    dependencies:
      - name: Desktop Nerd Font
        requirement: optional
        brew_cask: font-cascadia-code-nf
        font_match: "CascadiaCodeNF*"
        font_fallback_matches:
          - "CaskaydiaCoveNerdFont*"
entries:
  - source: configs/ghostty/config.ghostty
    target: ~/.config/ghostty/config.ghostty
    strategy: symlink
    tags: [desktop]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := manifest.LoadFile(path)
	if err == nil || !strings.Contains(err.Error(), "profiles[\"desktop\"].dependencies is retired") || !strings.Contains(err.Error(), "tag-scoped dependency set") {
		t.Fatalf("LoadFile() error = %v, want actionable retired profile dependency guidance", err)
	}

	got, err := manifest.LoadPreviousFile(path)
	if err != nil {
		t.Fatalf("LoadPreviousFile() error = %v", err)
	}
	deps := got.LegacyProfileDependencies("desktop")
	if len(deps) != 1 || deps[0].Name != "Desktop Nerd Font" || deps[0].Requirement != "optional" || deps[0].BrewCask != "font-cascadia-code-nf" || deps[0].FontMatch != "CascadiaCodeNF*" || !sameStrings(deps[0].FontFallbackMatches, []string{"CaskaydiaCoveNerdFont*"}) {
		t.Fatalf("legacy Profile dependencies = %#v, want desktop font dependency with fallback match", deps)
	}
}

func TestLoadFileParsesClaudeProvisioners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: claude
    tags: [desktop]
    os: [darwin, linux]
    spec:
      marketplace: ChromeDevTools/chrome-devtools-mcp
    dependencies:
      - name: claude
        command: claude
  - tool: claude
    tags: [desktop]
    spec:
      plugin: chrome-devtools-mcp
      from: chrome-devtools-plugins
    dependencies:
      - name: claude
        command: claude
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if len(got.Provisioners) != 2 {
		t.Fatalf("Provisioners len = %d, want 2", len(got.Provisioners))
	}

	market := got.Provisioners[0]
	if market.Tool != "claude" {
		t.Fatalf("Provisioner[0].Tool = %q, want claude", market.Tool)
	}
	if market.Spec.Marketplace != "ChromeDevTools/chrome-devtools-mcp" {
		t.Fatalf("Provisioner[0].Spec.Marketplace = %q, want ChromeDevTools/chrome-devtools-mcp", market.Spec.Marketplace)
	}
	if !sameStrings(market.Tags, []string{"desktop"}) {
		t.Fatalf("Provisioner[0].Tags = %#v, want [desktop]", market.Tags)
	}
	if market.Dependencies[0].Name != "claude" || market.Dependencies[0].Command != "claude" {
		t.Fatalf("Provisioner[0].Dependencies[0] = %#v, want claude command dependency", market.Dependencies[0])
	}

	plugin := got.Provisioners[1]
	if plugin.Spec.Plugin != "chrome-devtools-mcp" || plugin.Spec.From != "chrome-devtools-plugins" {
		t.Fatalf("Provisioner[1].Spec = %#v, want plugin chrome-devtools-mcp from chrome-devtools-plugins", plugin.Spec)
	}
}

func TestLoadFileParsesCodexProvisioner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: codex
    tags: [desktop]
    os: [darwin, linux]
    spec:
      mcp: chrome-devtools
      command: [npx, -y, chrome-devtools-mcp@latest, --no-performance-crux]
      env:
        CHROME_DEVTOOLS_MCP_NO_UPDATE_CHECKS: "1"
        CHROME_DEVTOOLS_MCP_NO_USAGE_STATISTICS: "1"
    dependencies:
      - name: codex
        command: codex
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if len(got.Provisioners) != 1 {
		t.Fatalf("Provisioners len = %d, want 1", len(got.Provisioners))
	}

	codex := got.Provisioners[0]
	if codex.Tool != "codex" {
		t.Fatalf("Provisioner[0].Tool = %q, want codex", codex.Tool)
	}
	if codex.Spec.MCP != "chrome-devtools" {
		t.Fatalf("Provisioner[0].Spec.MCP = %q, want chrome-devtools", codex.Spec.MCP)
	}
	wantCommand := []string{"npx", "-y", "chrome-devtools-mcp@latest", "--no-performance-crux"}
	if !sameStrings(codex.Spec.Command, wantCommand) {
		t.Fatalf("Provisioner[0].Spec.Command = %#v, want %#v", codex.Spec.Command, wantCommand)
	}
	if codex.Spec.Env["CHROME_DEVTOOLS_MCP_NO_UPDATE_CHECKS"] != "1" ||
		codex.Spec.Env["CHROME_DEVTOOLS_MCP_NO_USAGE_STATISTICS"] != "1" {
		t.Fatalf("Provisioner[0].Spec.Env = %#v, want both telemetry flags set to 1", codex.Spec.Env)
	}
	if codex.Dependencies[0].Name != "codex" || codex.Dependencies[0].Command != "codex" {
		t.Fatalf("Provisioner[0].Dependencies[0] = %#v, want codex command dependency", codex.Dependencies[0])
	}
}

func TestLoadFileParsesClaudeMCPProvisioner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: claude
    tags: [mobile]
    os: [darwin, linux]
    spec:
      mcp: dart
      command: [dart, mcp-server]
    dependencies:
      - name: dart
        command: dart
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if len(got.Provisioners) != 1 {
		t.Fatalf("Provisioners len = %d, want 1", len(got.Provisioners))
	}

	claude := got.Provisioners[0]
	if claude.Tool != "claude" {
		t.Fatalf("Provisioner[0].Tool = %q, want claude", claude.Tool)
	}
	if claude.Spec.MCP != "dart" {
		t.Fatalf("Provisioner[0].Spec.MCP = %q, want dart", claude.Spec.MCP)
	}
	if !sameStrings(claude.Spec.Command, []string{"dart", "mcp-server"}) {
		t.Fatalf("Provisioner[0].Spec.Command = %#v, want [dart mcp-server]", claude.Spec.Command)
	}
	if !hasDependency(claude.Dependencies, "dart") {
		t.Fatalf("Provisioner[0].Dependencies = %#v, want dart dependency", claude.Dependencies)
	}
}

func TestLoadFileParsesCodeGraphProvisioner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: codegraph
    tags: [agents]
    os: [darwin, linux]
    spec:
      scope: global
      agents: [codex, claude, antigravity, opencode]
      yes: true
    dependencies:
      - name: curl
        command: curl
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if len(got.Provisioners) != 1 {
		t.Fatalf("Provisioners len = %d, want 1", len(got.Provisioners))
	}
	codegraph := got.Provisioners[0]
	if codegraph.Tool != "codegraph" {
		t.Fatalf("Provisioner[0].Tool = %q, want codegraph", codegraph.Tool)
	}
	if codegraph.Spec.Scope != "global" || !codegraph.Spec.Yes {
		t.Fatalf("Provisioner[0].Spec scalar flags = %#v, want global/yes", codegraph.Spec)
	}
	if !sameStrings(codegraph.Spec.Agents, []string{"codex", "claude", "antigravity", "opencode"}) {
		t.Fatalf("Provisioner[0].Spec.Agents = %#v, want [codex claude antigravity opencode]", codegraph.Spec.Agents)
	}
	if codegraph.Dependencies[0].Name != "curl" || codegraph.Dependencies[0].Command != "curl" {
		t.Fatalf("Provisioner[0].Dependencies[0] = %#v, want curl command dependency", codegraph.Dependencies[0])
	}
}

func TestLoadFileParsesSkillsProvisioner(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: skills
    tags: [agents]
    spec:
      package: vercel-labs/agent-skills
      agents: [codex, claude-code, antigravity]
      skills: [web-design-guidelines]
      global: true
      copy: true
    dependencies:
      - name: npx
        command: npx
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	if len(got.Provisioners) != 1 {
		t.Fatalf("Provisioners len = %d, want 1", len(got.Provisioners))
	}
	skills := got.Provisioners[0]
	if skills.Tool != "skills" {
		t.Fatalf("Provisioner[0].Tool = %q, want skills", skills.Tool)
	}
	if skills.Spec.Package != "vercel-labs/agent-skills" {
		t.Fatalf("Provisioner[0].Spec.Package = %q, want vercel-labs/agent-skills", skills.Spec.Package)
	}
	if !sameStrings(skills.Spec.Agents, []string{"codex", "claude-code", "antigravity"}) {
		t.Fatalf("Provisioner[0].Spec.Agents = %#v, want [codex claude-code antigravity]", skills.Spec.Agents)
	}
	if !sameStrings(skills.Spec.Skills, []string{"web-design-guidelines"}) {
		t.Fatalf("Provisioner[0].Spec.Skills = %#v, want [web-design-guidelines]", skills.Spec.Skills)
	}
	if !skills.Spec.Global || !skills.Spec.Copy {
		t.Fatalf("Provisioner[0].Spec global/copy = %v/%v, want true/true", skills.Spec.Global, skills.Spec.Copy)
	}
	if skills.Dependencies[0].Name != "npx" || skills.Dependencies[0].Command != "npx" {
		t.Fatalf("Provisioner[0].Dependencies[0] = %#v, want npx command dependency", skills.Dependencies[0])
	}
}

func TestRepositoryManifestIncludesChromeDevToolsCodexProvisioner(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var codex *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool == "codex" && prov.Spec.MCP == "chrome-devtools" {
			codex = prov
		}
	}

	if codex == nil {
		t.Fatal("repository manifest missing codex MCP provisioner for chrome-devtools")
	}
	if !sameStrings(codex.Tags, []string{"codex-chrome-devtools"}) {
		t.Errorf("codex provisioner tags = %#v, want [codex-chrome-devtools]", codex.Tags)
	}
	if !sameStrings(codex.OS, []string{"darwin", "linux"}) {
		t.Errorf("codex provisioner OS = %#v, want [darwin linux]", codex.OS)
	}
	if !hasDependency(codex.Dependencies, "codex") {
		t.Errorf("codex provisioner missing codex dependency: %#v", codex.Dependencies)
	}
}

func TestRepositoryManifestIncludesDartFlutterMCPProvisioners(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	want := map[string]struct {
		tag     string
		command []string
	}{
		"claude": {tag: "claude-dart-mcp", command: []string{"dart", "mcp-server"}},
		"codex":  {tag: "codex-dart-mcp", command: []string{"dart", "mcp-server", "--force-roots-fallback"}},
	}
	for tool, wantProvisioner := range want {
		var prov *manifest.Provisioner
		for i := range got.Provisioners {
			candidate := &got.Provisioners[i]
			if candidate.Tool == tool && candidate.Spec.MCP == "dart" {
				prov = candidate
				break
			}
		}

		if prov == nil {
			t.Fatalf("repository manifest missing %s MCP provisioner for Dart and Flutter", tool)
		}
		if !sameStrings(prov.Tags, []string{wantProvisioner.tag}) {
			t.Errorf("%s MCP provisioner tags = %#v, want [%s]", tool, prov.Tags, wantProvisioner.tag)
		}
		if !sameStrings(prov.OS, []string{"darwin", "linux"}) {
			t.Errorf("%s MCP provisioner OS = %#v, want [darwin linux]", tool, prov.OS)
		}
		if !sameStrings(prov.Spec.Command, wantProvisioner.command) {
			t.Errorf("%s MCP command = %#v, want %#v", tool, prov.Spec.Command, wantProvisioner.command)
		}
		if !hasDependency(prov.Dependencies, tool) || !hasDependency(prov.Dependencies, "dart") {
			t.Errorf("%s MCP provisioner dependencies = %#v, want %s and dart", tool, prov.Dependencies, tool)
		}
		for _, dep := range prov.Dependencies {
			if dep.Name != "dart" {
				continue
			}
			if !strings.Contains(dep.ManualDebian, "Flutter SDK") || !strings.Contains(dep.ManualDebian, "dart --version") {
				t.Errorf("%s Dart dependency manual_debian = %q, want Ubuntu Flutter/Dart repair guidance", tool, dep.ManualDebian)
			}
		}
	}
}

func TestRepositoryManifestIncludesMobileAgentMCPConfigEntries(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	wantEntries := []struct {
		source string
		target string
		tag    string
	}{
		{source: "configs/antigravity/mobile-mcp-settings.json", target: "~/.gemini/antigravity-cli/settings.json", tag: "antigravity-dart-mcp"},
		{source: "configs/vscode/settings.json", target: "~/Library/Application Support/Code/User/settings.json", tag: "vscode-mobile"},
		{source: "configs/vscode/settings.json", target: "~/.config/Code/User/settings.json", tag: "vscode-mobile"},
	}
	for _, want := range wantEntries {
		source, target := want.source, want.target
		var entry *manifest.Entry
		for i := range got.Entries {
			candidate := &got.Entries[i]
			if candidate.Source == source && candidate.Target == target {
				entry = candidate
				break
			}
		}

		if entry == nil {
			t.Fatalf("repository manifest missing mobile MCP config entry %s -> %s", source, target)
		}
		if entry.Strategy != "copy" {
			t.Errorf("%s strategy = %q, want copy", source, entry.Strategy)
		}
		if entry.Ownership != "json-subset" {
			t.Errorf("%s ownership = %q, want json-subset", source, entry.Ownership)
		}
		if !sameStrings(entry.Tags, []string{want.tag}) {
			t.Errorf("%s tags = %#v, want [%s]", source, entry.Tags, want.tag)
		}
	}
}

func TestRepositoryManifestWebDependencySetIncludesPlaywrightCLI(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var dep manifest.Dependency
	for _, set := range got.Dependencies {
		if !hasString(set.Tags, "playwright") {
			continue
		}
		for _, candidate := range set.Dependencies {
			if candidate.Name == "Playwright CLI" {
				dep = candidate
			}
		}
	}
	if dep.Command != "playwright-cli" || dep.Brew != "playwright-cli" || !dep.LinuxHomebrew {
		t.Fatalf("Playwright CLI dependency = %#v, want command/brew playwright-cli with linux_homebrew", dep)
	}
}

func TestRepositoryManifestDeclaresAtomicDesktopCapabilities(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	capabilityTags := []string{"ghostty", "warp", "zed"}
	for _, name := range capabilityTags {
		tag, ok := got.Tags[name]
		if !ok {
			t.Errorf("repository manifest missing atomic Desktop Tag %q", name)
			continue
		}
		if tag.Kind != "surface" || tag.Status != "current" || strings.TrimSpace(tag.Description) == "" {
			t.Errorf("Tag %q = %#v, want described current surface", name, tag)
		}
	}

	legacy := got.Tags["desktop"]
	if legacy.Kind != "compatibility" || legacy.Status != "legacy" || !reflect.DeepEqual([]string(legacy.ReplacedBy), capabilityTags) {
		t.Fatalf("desktop Tag = %#v, want ordered compatibility alias for %#v", legacy, capabilityTags)
	}

	selection, err := manifest.ResolveSelection(*got, []string{"desktop"}, nil)
	if err != nil {
		t.Fatalf("ResolveSelection(desktop) error = %v", err)
	}
	if want := []string{"ghostty", "warp", "zed", "codexbar"}; !reflect.DeepEqual(selection.Tags, want) {
		t.Fatalf("desktop selection tags = %#v, want %#v", selection.Tags, want)
	}
	legacySelection, err := manifest.ResolveSelection(*got, nil, []string{"desktop"})
	if err != nil {
		t.Fatalf("ResolveSelection(--tag desktop) error = %v", err)
	}
	if !reflect.DeepEqual(legacySelection.Tags, capabilityTags) {
		t.Fatalf("legacy desktop selection tags = %#v, want %#v", legacySelection.Tags, capabilityTags)
	}

	for _, name := range capabilityTags {
		if dep := findDependency(selectedsurface.Evaluate(*got, []string{name}, "linux").Dependencies, "Desktop Nerd Font"); dep == nil {
			t.Errorf("Tag %q missing shared Desktop Nerd Font Dependency", name)
		}
	}

	tag, ok := got.Tags["codexbar"]
	if !ok || tag.Kind != "surface" || tag.Status != "current" {
		t.Fatalf("codexbar tag = %#v, present=%v, want current surface tag", tag, ok)
	}
	darwin := selectedsurface.Evaluate(*got, []string{"codexbar"}, "darwin")
	if len(darwin.Dependencies) != 1 {
		t.Fatalf("Darwin codexbar dependencies = %#v, want one dependency", darwin.Dependencies)
	}
	codexbar := darwin.Dependencies[0]
	if codexbar.Name != "CodexBar" || codexbar.DarwinApp != "CodexBar.app" || codexbar.BrewCask != "codexbar" {
		t.Fatalf("CodexBar dependency = %#v, want CodexBar.app detection and codexbar cask", codexbar)
	}

	linux := selectedsurface.Evaluate(*got, []string{"codexbar"}, "linux")
	if len(linux.Dependencies) != 0 {
		t.Fatalf("Linux codexbar dependencies = %#v, want none", linux.Dependencies)
	}
}

func TestRepositoryManifestDeclaresAtomicAgentCapabilities(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	capabilityTags := []string{"codex", "claude", "opencode", "antigravity", "copilot"}
	for _, name := range capabilityTags {
		tag, ok := got.Tags[name]
		if !ok {
			t.Errorf("repository manifest missing atomic Agent Tag %q", name)
			continue
		}
		if tag.Kind != "surface" || tag.Status != "current" || strings.TrimSpace(tag.Description) == "" {
			t.Errorf("Tag %q = %#v, want described current surface", name, tag)
		}
	}
	legacy := got.Tags["agents"]
	if legacy.Kind != "compatibility" || legacy.Status != "legacy" || !reflect.DeepEqual([]string(legacy.ReplacedBy), capabilityTags) {
		t.Fatalf("agents Tag = %#v, want ordered compatibility alias for %#v", legacy, capabilityTags)
	}
	if profile := got.Profiles["agents"]; !reflect.DeepEqual(profile.Tags, capabilityTags) {
		t.Fatalf("agents Profile Tags = %#v, want %#v", profile.Tags, capabilityTags)
	}

	wantAgents := map[string]struct {
		tag     string
		command string
		recipe  string
	}{
		"Codex":       {tag: "codex", command: "codex", recipe: "codex"},
		"Claude Code": {tag: "claude", command: "claude", recipe: "claude"},
		"OpenCode":    {tag: "opencode", command: "opencode", recipe: "opencode"},
		"Antigravity": {tag: "antigravity", command: "agy", recipe: "antigravity"},
		"Copilot CLI": {tag: "copilot", command: "copilot", recipe: "copilot"},
	}
	for name, want := range wantAgents {
		surface := selectedsurface.Evaluate(*got, []string{want.tag}, "linux")
		dep := findDependency(surface.Dependencies, name)
		if dep == nil {
			t.Errorf("Tag %q missing %s Dependency: %#v", want.tag, name, surface.Dependencies)
			continue
		}
		if dep.Command != want.command {
			t.Errorf("Tag %q dependency %s command = %q, want %q", want.tag, name, dep.Command, want.command)
		}
		if dep.RollingUserLocal == nil || dep.RollingUserLocal.Recipe != want.recipe {
			t.Errorf("Tag %q dependency %s rolling provider = %#v, want recipe %q", want.tag, name, dep.RollingUserLocal, want.recipe)
		}
	}
	for _, name := range []string{"claude", "copilot"} {
		if dep := findDependency(selectedsurface.Evaluate(*got, []string{name}, "linux").Dependencies, "jq"); dep == nil {
			t.Errorf("Tag %q missing shared jq Dependency", name)
		}
	}

	githubCLI := findDependency(selectedsurface.Evaluate(*got, []string{"gh"}, "linux").Dependencies, "GitHub CLI")
	if githubCLI == nil {
		t.Fatal("gh Tag missing GitHub CLI Dependency")
	}
	if githubCLI != nil && (githubCLI.Command != "gh" || githubCLI.Brew != "gh" || !githubCLI.LinuxHomebrew) {
		t.Errorf("GitHub CLI dependency = %#v, want command/brew gh with linux_homebrew", *githubCLI)
	}
	if dep := findDependency(selectedsurface.Evaluate(*got, []string{"jq"}, "linux").Dependencies, "jq"); dep == nil {
		t.Fatal("jq Tag missing jq Dependency")
	}

	opencodeEntry := findEntry(got.Entries, "~/.config/opencode/opencode.json")
	if opencodeEntry == nil {
		t.Fatal("agents profile missing native OpenCode Managed Entry")
	}
	if opencodeEntry.Source != "configs/opencode/opencode.json" || opencodeEntry.Ownership != "json-subset" || !sameStrings(opencodeEntry.Tags, []string{"opencode"}) {
		t.Errorf("OpenCode Managed Entry = %#v, want opencode-tagged configs/opencode/opencode.json with JSON Subset Ownership", *opencodeEntry)
	}

	zedSettings := findEntry(got.Entries, "~/.config/zed/settings.json")
	if zedSettings == nil {
		t.Fatal("desktop profile missing Zed settings Managed Entry")
	}
	if zedSettings.Source != "configs/zed/settings.json" || zedSettings.Strategy != "copy" || zedSettings.Ownership != "jsonc-subset" || !sameStrings(zedSettings.Tags, []string{"zed"}) {
		t.Errorf("Zed settings Managed Entry = %#v, want zed-tagged copy with JSONC Subset Ownership", *zedSettings)
	}

	selected, err := provision.Select(*got, provision.Options{Profile: "agents", OS: "darwin"})
	if err != nil {
		t.Fatalf("provision.Select(agents) error = %v", err)
	}
	if len(selected) != 0 {
		t.Errorf("agents selected legacy Provisioners = %#v, want none", selected)
	}

	wantWorkstation := append(append(append([]string(nil), got.Profiles["core"].Tags...), got.Profiles["desktop"].Tags[:3]...), capabilityTags...)
	if profile := got.Profiles["workstation"]; !reflect.DeepEqual(profile.Tags, wantWorkstation) {
		t.Fatalf("workstation Profile Tags = %#v, want core + Ghostty/Warp/Zed + Agent capability Tags %#v", profile.Tags, wantWorkstation)
	}

	for i, set := range got.Dependencies {
		if hasString(set.Tags, "desktop") || hasString(set.Tags, "agents") {
			t.Errorf("dependencies[%d] still selects a legacy broad Tag: %#v", i, set.Tags)
		}
	}
	for i, entry := range got.Entries {
		if hasString(entry.Tags, "desktop") || hasString(entry.Tags, "agents") {
			t.Errorf("entries[%d] %q still selects a legacy broad Tag: %#v", i, entry.Target, entry.Tags)
		}
	}
	for i, provisioner := range got.Provisioners {
		if hasString(provisioner.Tags, "desktop") || hasString(provisioner.Tags, "agents") {
			t.Errorf("provisioners[%d] %q still selects a legacy broad Tag: %#v", i, provisioner.Tool, provisioner.Tags)
		}
	}
}

func TestRepositoryManifestDeclaresAtomicWebAndMobileCapabilities(t *testing.T) {
	got, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	wantProfiles := map[string][]string{
		"web": {
			"playwright", "frontend-design", "vercel-web-skills",
			"claude-chrome-devtools", "codex-chrome-devtools", "opencode-chrome-devtools",
		},
		"mobile": {
			"dart-skills", "flutter-skills", "android-skills", "claude-dart-mcp",
			"codex-dart-mcp", "antigravity-dart-mcp", "vscode-mobile",
		},
	}
	for name, wantTags := range wantProfiles {
		for _, tagName := range wantTags {
			tag, ok := got.Tags[tagName]
			if !ok {
				t.Errorf("repository manifest missing atomic %s Tag %q", name, tagName)
				continue
			}
			if tag.Kind != "surface" || tag.Status != "current" || strings.TrimSpace(tag.Description) == "" {
				t.Errorf("Tag %q = %#v, want described current surface", tagName, tag)
			}
		}

		legacy := got.Tags[name]
		if legacy.Kind != "compatibility" || legacy.Status != "legacy" || !sameStrings(legacy.ReplacedBy, wantTags) {
			t.Errorf("%s Tag = %#v, want ordered compatibility alias for %#v", name, legacy, wantTags)
		}
		if profile := got.Profiles[name]; !sameStrings(profile.Tags, wantTags) {
			t.Errorf("%s Profile Tags = %#v, want %#v", name, profile.Tags, wantTags)
		}
		legacySelection, err := manifest.ResolveSelection(*got, nil, []string{name})
		if err != nil {
			t.Errorf("ResolveSelection(--tag %s) error = %v", name, err)
		} else if !sameStrings(legacySelection.Tags, wantTags) {
			t.Errorf("legacy %s selection Tags = %#v, want %#v", name, legacySelection.Tags, wantTags)
		}
	}

	for i, set := range got.Dependencies {
		if hasString(set.Tags, "web") || hasString(set.Tags, "mobile") {
			t.Errorf("dependencies[%d] still selects a legacy broad Tag: %#v", i, set.Tags)
		}
	}
	for i, entry := range got.Entries {
		if hasString(entry.Tags, "web") || hasString(entry.Tags, "mobile") {
			t.Errorf("entries[%d] %q still selects a legacy broad Tag: %#v", i, entry.Target, entry.Tags)
		}
	}
	for i, provisioner := range got.Provisioners {
		if hasString(provisioner.Tags, "web") || hasString(provisioner.Tags, "mobile") {
			t.Errorf("provisioners[%d] %q still selects a legacy broad Tag: %#v", i, provisioner.Tool, provisioner.Tags)
		}
	}

	for _, name := range []string{"adaptive-theme", "codegraph"} {
		tag := got.Tags[name]
		if tag.Kind != "surface" || tag.Status != "current" || len(tag.ReplacedBy) != 0 {
			t.Errorf("global Tag %q = %#v, want one current opt-in without replacements", name, tag)
		}
		for profileName, profile := range got.Profiles {
			if hasString(profile.Tags, name) {
				t.Errorf("Profile %q includes global opt-in Tag %q", profileName, name)
			}
		}
	}

	for _, entry := range selectedsurface.Evaluate(*got, []string{"adaptive-theme"}, "darwin").Entries {
		if entry.Entry.Target == "~/.config/herdr/config.toml" || entry.Entry.Target == "~/.config/zellij/config.kdl" {
			t.Errorf("adaptive-theme selected unrequested consumer %q", entry.Entry.Target)
		}
	}
	for _, entry := range selectedsurface.Evaluate(*got, []string{"codegraph"}, "linux").Entries {
		if entry.Entry.Target == "~/.codex/config.toml" {
			t.Errorf("codegraph selected the unrequested Codex consumer")
		}
	}

	wantOverrides := []struct {
		tags   []string
		osName string
		target string
		source string
	}{
		{tags: []string{"herdr", "adaptive-theme"}, osName: "darwin", target: "~/.config/herdr/config.toml", source: "configs/herdr/config-adaptive.toml"},
		{tags: []string{"codex", "codegraph"}, osName: "linux", target: "~/.codex/config.toml", source: "configs/codex/config-codegraph.toml"},
	}
	for _, want := range wantOverrides {
		found := false
		for _, entry := range selectedsurface.Evaluate(*got, want.tags, want.osName).Entries {
			if entry.Entry.Target != want.target {
				continue
			}
			found = true
			if entry.Source != want.source {
				t.Errorf("%q source = %q, want %q", want.target, entry.Source, want.source)
			}
		}
		if !found {
			t.Errorf("Tags %#v did not select consumer %q", want.tags, want.target)
		}
	}
}

func TestRepositoryAtomicAgentCompositionAndCodexSourceOverride(t *testing.T) {
	got, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		tags    []string
		target  string
		sources []string
	}{
		{name: "OpenCode baseline plus Chrome DevTools subset", tags: []string{"opencode", "opencode-chrome-devtools"}, target: "~/.config/opencode/opencode.json", sources: []string{"configs/opencode/opencode.json", "configs/opencode/mcp.json"}},
		{name: "Antigravity baseline plus Dart MCP subset", tags: []string{"antigravity", "antigravity-dart-mcp"}, target: "~/.gemini/antigravity-cli/settings.json", sources: []string{"configs/antigravity/settings.json", "configs/antigravity/mobile-mcp-settings.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sources []string
			for _, entry := range selectedsurface.Evaluate(*got, tt.tags, "linux").Entries {
				if entry.Entry.Target == tt.target {
					sources = append(sources, entry.Source)
				}
			}
			if !reflect.DeepEqual(sources, tt.sources) {
				t.Fatalf("selected sources for %q = %#v, want %#v", tt.target, sources, tt.sources)
			}
		})
	}

	for _, entry := range selectedsurface.Evaluate(*got, []string{"codex", "codegraph"}, "linux").Entries {
		if entry.Entry.Target == "~/.codex/config.toml" {
			if entry.Source != "configs/codex/config-codegraph.toml" || entry.OverrideTag != "codegraph" {
				t.Fatalf("Codex selected entry = %#v, want codegraph source override", entry)
			}
			return
		}
	}
	t.Fatal("Codex and codegraph Tags did not select the Codex Managed Entry")
}

func TestRepositoryManifestLinuxHomebrewReviewBoundary(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	dependencies := make(map[string][]manifest.Dependency)
	for _, depSet := range got.Dependencies {
		for _, dep := range depSet.Dependencies {
			dependencies[dep.Name] = append(dependencies[dep.Name], dep)
		}
	}
	for _, entry := range got.Entries {
		for _, dep := range entry.Dependencies {
			dependencies[dep.Name] = append(dependencies[dep.Name], dep)
		}
	}
	for _, provisioner := range got.Provisioners {
		for _, dep := range provisioner.Dependencies {
			dependencies[dep.Name] = append(dependencies[dep.Name], dep)
		}
	}

	for _, name := range []string{"ghostty", "zed"} {
		for _, dep := range dependencies[name] {
			if dep.LinuxHomebrew {
				t.Fatalf("%s dependency = %#v, want manual Linux handling without linux_homebrew", name, dep)
			}
		}
	}
	ghosttyManualFound := false
	ghosttyDarwinAppFound := false
	for _, dep := range dependencies["ghostty"] {
		if strings.Contains(dep.ManualDebian, "snap install ghostty --classic") && strings.Contains(dep.ManualDebian, "requires sudo") {
			ghosttyManualFound = true
		}
		if dep.DarwinApp == "Ghostty.app" && dep.BrewCask == "ghostty" && dep.Brew == "" {
			ghosttyDarwinAppFound = true
		}
	}
	if !ghosttyManualFound {
		t.Fatalf("ghostty dependency missing explicit Ubuntu manual guidance with snap sudo/interactivity note: %#v", dependencies["ghostty"])
	}
	if !ghosttyDarwinAppFound {
		t.Fatalf("ghostty dependency missing Darwin app-bundle detection: %#v", dependencies["ghostty"])
	}

	for _, name := range []string{"bat", "starship", "zellij", "atuin", "pnpm"} {
		if len(dependencies[name]) == 0 {
			t.Fatalf("repository manifest missing %s dependency", name)
		}
		for _, dep := range dependencies[name] {
			if !dep.LinuxHomebrew {
				t.Fatalf("%s dependency = %#v, want reviewed Linuxbrew opt-in", name, dep)
			}
			if (name == "bat" || name == "starship" || name == "zellij" || name == "atuin" || name == "pnpm") && dep.Apt != "" {
				t.Fatalf("%s dependency = %#v, want no Ubuntu apt package declaration", name, dep)
			}
			if name == "bat" || name == "starship" || name == "atuin" || name == "pnpm" {
				if dep.UserLocal == nil || dep.UserLocal.Recipe != name || dep.UserLocal.Version == "" {
					t.Fatalf("%s dependency = %#v, want reviewed user-local policy", name, dep)
				}
				if dep.UserLocal.Checksums["linux_amd64"] == "" || dep.UserLocal.Checksums["linux_arm64"] == "" {
					t.Fatalf("%s user_local checksums = %#v, want linux amd64 and arm64", name, dep.UserLocal.Checksums)
				}
			}
		}
	}
}

func TestRepositoryManifestIncludesPlaywrightCLISkillProvisioner(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var skills *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool == "skills" && prov.Spec.Package == "microsoft/playwright-cli" {
			skills = prov
		}
	}

	if skills == nil {
		t.Fatal("repository manifest missing skills provisioner for microsoft/playwright-cli")
	}
	if !sameStrings(skills.Tags, []string{"playwright"}) {
		t.Errorf("skills provisioner tags = %#v, want [playwright]", skills.Tags)
	}
	if !sameStrings(skills.Spec.Agents, []string{"codex", "claude-code", "antigravity", "opencode", "github-copilot"}) {
		t.Errorf("skills provisioner agents = %#v, want [codex claude-code antigravity opencode github-copilot]", skills.Spec.Agents)
	}
	if !sameStrings(skills.Spec.Skills, []string{"playwright-cli"}) {
		t.Errorf("skills provisioner skills = %#v, want [playwright-cli]", skills.Spec.Skills)
	}
	if !skills.Spec.Global || !skills.Spec.Copy {
		t.Errorf("skills provisioner global/copy = %v/%v, want true/true", skills.Spec.Global, skills.Spec.Copy)
	}
	if !hasDependency(skills.Dependencies, "npx") {
		t.Errorf("skills provisioner missing npx dependency: %#v", skills.Dependencies)
	}
}

func TestRepositoryManifestIncludesExternalSkillsProvisioner(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var skills *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool == "skills" && prov.Spec.Package == "vercel-labs/agent-skills" {
			skills = prov
		}
	}

	if skills == nil {
		t.Fatal("repository manifest missing skills provisioner for vercel-labs/agent-skills")
	}
	if !sameStrings(skills.Tags, []string{"vercel-web-skills"}) {
		t.Errorf("skills provisioner tags = %#v, want [vercel-web-skills]", skills.Tags)
	}
	if !sameStrings(skills.Spec.Agents, []string{"codex", "claude-code", "antigravity", "opencode", "github-copilot"}) {
		t.Errorf("skills provisioner agents = %#v, want [codex claude-code antigravity opencode github-copilot]", skills.Spec.Agents)
	}
	wantSkills := []string{
		"vercel-react-best-practices",
		"vercel-composition-patterns",
		"vercel-react-view-transitions",
		"web-design-guidelines",
	}
	if !sameStrings(skills.Spec.Skills, wantSkills) {
		t.Errorf("skills provisioner skills = %#v, want %#v", skills.Spec.Skills, wantSkills)
	}
	if !skills.Spec.Global {
		t.Errorf("skills provisioner global = false, want true")
	}
	if !hasDependency(skills.Dependencies, "npx") {
		t.Errorf("skills provisioner missing npx dependency: %#v", skills.Dependencies)
	}
}

func TestRepositoryManifestIncludesAnthropicFrontendDesignSkillProvisioner(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var skills *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool == "skills" && prov.Spec.Package == "anthropics/skills" {
			skills = prov
		}
	}

	if skills == nil {
		t.Fatal("repository manifest missing skills provisioner for anthropics/skills")
	}
	if !sameStrings(skills.Tags, []string{"frontend-design"}) {
		t.Errorf("skills provisioner tags = %#v, want [frontend-design]", skills.Tags)
	}
	if !sameStrings(skills.Spec.Agents, []string{"codex", "claude-code", "antigravity", "opencode", "github-copilot"}) {
		t.Errorf("skills provisioner agents = %#v, want [codex claude-code antigravity opencode github-copilot]", skills.Spec.Agents)
	}
	if !sameStrings(skills.Spec.Skills, []string{"frontend-design"}) {
		t.Errorf("skills provisioner skills = %#v, want [frontend-design]", skills.Spec.Skills)
	}
	if !skills.Spec.Global {
		t.Errorf("skills provisioner global = false, want true")
	}
	if !hasDependency(skills.Dependencies, "npx") {
		t.Errorf("skills provisioner missing npx dependency: %#v", skills.Dependencies)
	}
}

func TestRepositoryManifestMobileProfileIncludesMobileSkills(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	mobile, ok := got.Profiles["mobile"]
	if !ok {
		t.Fatal("repository manifest missing mobile profile")
	}
	wantMobileTags := []string{"dart-skills", "flutter-skills", "android-skills", "claude-dart-mcp", "codex-dart-mcp", "antigravity-dart-mcp", "vscode-mobile"}
	if !sameStrings(mobile.Tags, wantMobileTags) {
		t.Fatalf("mobile profile tags = %#v, want %#v", mobile.Tags, wantMobileTags)
	}

	mobileSkillPackages := []struct {
		name       string
		tag        string
		wantSkills []string
	}{
		{name: "dart-lang/skills", tag: "dart-skills"},
		{name: "flutter/skills", tag: "flutter-skills"},
		{name: "android/skills", tag: "android-skills", wantSkills: []string{"android-cli"}},
	}

	for _, pkg := range mobileSkillPackages {
		t.Run(pkg.name, func(t *testing.T) {
			var skills *manifest.Provisioner
			for i := range got.Provisioners {
				prov := &got.Provisioners[i]
				if prov.Tool == "skills" && prov.Spec.Package == pkg.name {
					skills = prov
				}
			}

			if skills == nil {
				t.Fatalf("repository manifest missing skills provisioner for %s", pkg.name)
			}
			if !sameStrings(skills.Tags, []string{pkg.tag}) {
				t.Errorf("skills provisioner tags = %#v, want [%s]", skills.Tags, pkg.tag)
			}
			if !sameStrings(skills.Spec.Agents, []string{"codex", "claude-code", "antigravity", "opencode", "github-copilot"}) {
				t.Errorf("skills provisioner agents = %#v, want [codex claude-code antigravity opencode github-copilot]", skills.Spec.Agents)
			}
			if !sameStrings(skills.Spec.Skills, pkg.wantSkills) {
				t.Errorf("skills provisioner skills = %#v, want %#v", skills.Spec.Skills, pkg.wantSkills)
			}
			if !skills.Spec.Global {
				t.Errorf("skills provisioner global = false, want true")
			}
			if !hasDependency(skills.Dependencies, "npx") {
				t.Errorf("skills provisioner missing npx dependency: %#v", skills.Dependencies)
			}
		})
	}
}

func TestRepositoryManifestDoesNotIncludeRetiredDelegationSkillProvisioner(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	for _, prov := range got.Provisioners {
		if prov.Tool == "skills" && prov.Spec.Package == "yersonargotev/dots/skills/delegation" {
			t.Fatal("repository manifest still includes the retired delegation skill provisioner")
		}
	}
}

func TestRepositoryManifestIncludesCodeGraphProvisioner(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var codegraph *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool == "codegraph" {
			codegraph = prov
		}
	}

	if codegraph == nil {
		t.Fatal("repository manifest missing codegraph provisioner")
	}
	if !hasString(codegraph.Tags, "codegraph") {
		t.Errorf("codegraph provisioner %#v missing codegraph tag", codegraph.Spec)
	}
	if hasString(codegraph.Tags, "agents") {
		t.Errorf("codegraph provisioner tags = %#v, want independent from agents tag", codegraph.Tags)
	}
	if !sameStrings(codegraph.OS, []string{"darwin", "linux"}) {
		t.Errorf("codegraph provisioner OS = %#v, want [darwin linux]", codegraph.OS)
	}
	if codegraph.Spec.Scope != "global" || !codegraph.Spec.Yes {
		t.Errorf("codegraph provisioner scalar flags = %#v, want global/yes", codegraph.Spec)
	}
	if !sameStrings(codegraph.Spec.Agents, []string{"codex", "claude", "antigravity", "opencode"}) {
		t.Errorf("codegraph provisioner agents = %#v, want [codex claude antigravity opencode]", codegraph.Spec.Agents)
	}
	if !hasDependency(codegraph.Dependencies, "curl") {
		t.Errorf("codegraph provisioner missing curl dependency: %#v", codegraph.Dependencies)
	}
}

func TestRepositoryManifestIncludesChromeDevToolsPluginProvisioners(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var market, plugin *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool != "claude" {
			continue
		}
		switch {
		case prov.Spec.Marketplace == "ChromeDevTools/chrome-devtools-mcp":
			market = prov
		case prov.Spec.Plugin == "chrome-devtools-mcp" && prov.Spec.From == "chrome-devtools-plugins":
			plugin = prov
		}
	}

	if market == nil {
		t.Fatal("repository manifest missing claude marketplace provisioner for chrome-devtools")
	}
	if plugin == nil {
		t.Fatal("repository manifest missing claude plugin provisioner for chrome-devtools-mcp")
	}

	for _, prov := range []*manifest.Provisioner{market, plugin} {
		if !sameStrings(prov.Tags, []string{"claude-chrome-devtools"}) {
			t.Errorf("claude provisioner tags = %#v, want [claude-chrome-devtools]", prov.Tags)
		}
		if !sameStrings(prov.OS, []string{"darwin", "linux"}) {
			t.Errorf("claude provisioner OS = %#v, want [darwin linux]", prov.OS)
		}
		if !hasDependency(prov.Dependencies, "claude") {
			t.Errorf("claude provisioner missing claude dependency: %#v", prov.Dependencies)
		}
	}
}

func TestRepositoryManifestMarksClaudeSettingsAsJSONSubsetOwned(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var settings *manifest.Entry
	for i := range got.Entries {
		entry := &got.Entries[i]
		if entry.Source == "configs/claude/settings.json" && entry.Target == "~/.claude/settings.json" {
			settings = entry
			break
		}
	}

	if settings == nil {
		t.Fatal("repository manifest missing Claude settings entry")
	}
	if settings.Strategy != "copy" {
		t.Fatalf("Claude settings strategy = %q, want copy", settings.Strategy)
	}
	if settings.Ownership != "json-subset" {
		t.Fatalf("Claude settings ownership = %q, want json-subset", settings.Ownership)
	}
}

func TestRepositoryManifestMarksCodexConfigAsTOMLSubsetOwned(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var settings *manifest.Entry
	for i := range got.Entries {
		entry := &got.Entries[i]
		if entry.Source == "configs/codex/config.toml" && entry.Target == "~/.codex/config.toml" {
			settings = entry
			break
		}
	}

	if settings == nil {
		t.Fatal("repository manifest missing Codex config entry")
	}
	if settings.Strategy != "copy" {
		t.Fatalf("Codex config strategy = %q, want copy", settings.Strategy)
	}
	if settings.Ownership != "toml-subset" {
		t.Fatalf("Codex config ownership = %q, want toml-subset", settings.Ownership)
	}
	if settings.SourceOverrides["codegraph"] != "configs/codex/config-codegraph.toml" {
		t.Fatalf("Codex config codegraph override = %q, want configs/codex/config-codegraph.toml", settings.SourceOverrides["codegraph"])
	}
}

func TestDependencyProbeTrimsWhitespace(t *testing.T) {
	tests := []struct {
		name string
		dep  manifest.Dependency
		want string
	}{
		{name: "defaults to name", dep: manifest.Dependency{Name: "tmux"}, want: "tmux"},
		{name: "command overrides name", dep: manifest.Dependency{Name: "ripgrep", Command: "rg"}, want: "rg"},
		{name: "trims padded command", dep: manifest.Dependency{Name: "ripgrep", Command: " rg "}, want: "rg"},
		{name: "trims padded name", dep: manifest.Dependency{Name: " neovim "}, want: "neovim"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dep.Probe(); got != tt.want {
				t.Fatalf("Probe() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDependencyProbesUsesCommandsWhenDeclared(t *testing.T) {
	dep := manifest.Dependency{Name: "Rust stable (rustup)", Commands: []string{" rustup ", "rustc", "cargo", "rustc"}}
	want := []string{"rustup", "rustc", "cargo"}
	if got := dep.Probes(); !sameStrings(got, want) {
		t.Fatalf("Probes() = %#v, want %#v", got, want)
	}
}

func TestDependencyIsFont(t *testing.T) {
	tests := []struct {
		name string
		dep  manifest.Dependency
		want bool
	}{
		{name: "command dependency is not a font", dep: manifest.Dependency{Name: "tmux"}, want: false},
		{name: "font_match marks a font dependency", dep: manifest.Dependency{Name: "CascadiaCode NF", FontMatch: "CascadiaCodeNF*"}, want: true},
		{name: "fallback match marks a font dependency", dep: manifest.Dependency{Name: "Desktop Nerd Font", FontFallbackMatches: []string{"CaskaydiaCoveNerdFont*"}}, want: true},
		{name: "blank font_match is not a font", dep: manifest.Dependency{Name: "tmux", FontMatch: "  "}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dep.IsFont(); got != tt.want {
				t.Fatalf("IsFont() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDependencyFontMatches(t *testing.T) {
	dep := manifest.Dependency{
		FontMatch:           " CascadiaCodeNF* ",
		FontFallbackMatches: []string{"", " CaskaydiaCoveNerdFont* ", "CascadiaCodeNF*"},
	}
	want := []string{"CascadiaCodeNF*", "CaskaydiaCoveNerdFont*"}
	if got := dep.FontMatches(); !sameStrings(got, want) {
		t.Fatalf("FontMatches() = %#v, want %#v", got, want)
	}
}

func TestLoadFileRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategey: symlink
    tags: [core]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := manifest.LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want error for unknown field")
	}
	if !strings.Contains(err.Error(), "strategey") {
		t.Fatalf("LoadFile() error = %q, want it to name the unknown field %q", err.Error(), "strategey")
	}
}

func TestLoadFileRejectsInvalidManifests(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "missing version",
			content: `profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: "version is required",
		},
		{
			name: "profile without tags",
			content: `version: 1
profiles:
  default:
    tags: []
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: `profiles["default"].tags is required`,
		},
		{
			name: "profile with empty tag",
			content: `version: 1
profiles:
  default:
    tags: ["", core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: `profiles["default"].tags[0] must not be empty`,
		},
		{
			name: "profile with whitespace-only tag",
			content: `version: 1
profiles:
  default:
    tags: ["  ", core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: `profiles["default"].tags[0] must not be empty`,
		},
		{
			name: "profile dependency without name",
			content: `version: 1
profiles:
  default:
    tags: [core]
    dependencies:
      - brew_cask: font-cascadia-code-nf
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`,
			want: `profiles["default"].dependencies is retired; move the dependencies to a tag-scoped dependency set under dependencies using the profile's tags`,
		},
		{
			name: "unsupported strategy",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: shell
    tags: [core]
`,
			want: "entries[0].strategy must be one of copy, symlink, template",
		},
		{
			name: "unsupported os filter",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: copy
    tags: [core]
    os: [windows]
`,
			want: "entries[0].os[0] must be one of darwin, linux",
		},
		{
			name: "entry with empty tag",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: copy
    tags: ["", core]
`,
			want: "entries[0].tags[0] must not be empty",
		},
		{
			name: "entry with whitespace-only tag",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: copy
    tags: ["  ", core]
`,
			want: "entries[0].tags[0] must not be empty",
		},
		{
			name: "dependency without name",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - brew: tmux
`,
			want: "entries[0].dependencies[0].name is required",
		},
		{
			name: "dependency with whitespace-only name",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - name: "  "
        brew: tmux
`,
			want: "entries[0].dependencies[0].name is required",
		},
		{
			name: "dependency with invalid requirement",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - name: tmux
        requirement: recommended
`,
			want: "entries[0].dependencies[0].requirement must be one of required, optional",
		},
		{
			name: "dependency with whitespace-only command",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/tmux/tmux.conf
    target: ~/.tmux.conf
    strategy: symlink
    tags: [core]
    dependencies:
      - name: tmux
        command: "  "
`,
			want: `entries[0].dependencies[0].command must not be empty`,
		},
		{
			name: "dependency with invalid Darwin app bundle name",
			content: `version: 1
profiles:
  default:
    tags: [desktop]
entries:
  - source: configs/ghostty/config.ghostty
    target: ~/.config/ghostty/config.ghostty
    strategy: symlink
    tags: [desktop]
    dependencies:
      - name: ghostty
        darwin_app: ../Ghostty.app
`,
			want: `entries[0].dependencies[0].darwin_app must be an .app bundle name without a path`,
		},
		{
			name: "dependency with whitespace-only manual guidance",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/ghostty/config.ghostty
    target: ~/.config/ghostty/config.ghostty
    strategy: symlink
    tags: [desktop]
    dependencies:
      - name: ghostty
        manual: "  "
`,
			want: `entries[0].dependencies[0].manual must not be empty`,
		},
		{
			name: "dependency with whitespace-only Debian manual guidance",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/ghostty/config.ghostty
    target: ~/.config/ghostty/config.ghostty
    strategy: symlink
    tags: [desktop]
    dependencies:
      - name: ghostty
        manual_debian: "  "
`,
			want: `entries[0].dependencies[0].manual_debian must not be empty`,
		},
		{
			name: "dependency with whitespace-only brew cask",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/fonts
    target: ~/.local/share/fonts
    strategy: copy
    tags: [core]
    dependencies:
      - name: CascadiaCode Nerd Font
        brew_cask: "  "
        font_match: "CascadiaCodeNF*"
`,
			want: `entries[0].dependencies[0].brew_cask must not be empty`,
		},
		{
			name: "dependency with ambiguous homebrew formula and cask",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/fonts
    target: ~/.local/share/fonts
    strategy: copy
    tags: [core]
    dependencies:
      - name: CascadiaCode Nerd Font
        brew: cascadia-code
        brew_cask: font-cascadia-code-nf
`,
			want: `entries[0].dependencies[0] must not set both brew and brew_cask`,
		},
		{
			name: "dependency with empty font fallback match",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/fonts
    target: ~/.local/share/fonts
    strategy: copy
    tags: [core]
    dependencies:
      - name: CascadiaCode Nerd Font
        font_match: "CascadiaCodeNF*"
        font_fallback_matches: ["  "]
`,
			want: `entries[0].dependencies[0].font_fallback_matches[0] must not be empty`,
		},
		{
			name: "missing entry target",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    strategy: copy
    tags: [core]
`,
			want: "entries[0].target is required",
		},
		{
			name: "unsupported target root",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim/lazy-lock.json
    target: nvim/lazy-lock.json
    target_root: arbitrary
    strategy: copy
    ownership: seeded
    tags: [core]
`,
			want: "entries[0].target_root must be xdg-state when set",
		},
		{
			name: "xdg state target traversal",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim/lazy-lock.json
    target: ../lazy-lock.json
    target_root: xdg-state
    strategy: copy
    ownership: seeded
    tags: [core]
`,
			want: "entries[0].target must be a confined relative path for target_root xdg-state",
		},
		{
			name: "xdg state target requires seeded ownership",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim/lazy-lock.json
    target: nvim/lazy-lock.json
    target_root: xdg-state
    strategy: copy
    tags: [core]
`,
			want: "entries[0].target_root xdg-state requires seeded ownership",
		},
		{
			name: "unsupported ownership",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/claude/settings.json
    target: ~/.claude/settings.json
    strategy: copy
    ownership: merge
    tags: [core]
`,
			want: "entries[0].ownership must be one of json-subset, jsonc-subset, toml-subset, marked-block, seeded",
		},
		{
			name: "json subset ownership on non-copy strategy",
			content: `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/claude/settings.json
    target: ~/.claude/settings.json
    strategy: symlink
    ownership: json-subset
    tags: [core]
`,
			want: "entries[0].ownership json-subset requires strategy copy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "dots.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			_, err := manifest.LoadFile(path)
			if err == nil {
				t.Fatal("LoadFile() error = nil, want validation error")
			}
			if err.Error() != tt.want {
				t.Fatalf("LoadFile() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestLoadFileRejectsInvalidProvisioners(t *testing.T) {
	const base = `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
`

	tests := []struct {
		name        string
		provisioner string
		want        string
	}{
		{
			name: "missing tool",
			provisioner: `  - tags: [core]
    spec:
      scope: global
`,
			want: "provisioners[0].tool is required",
		},
		{
			name: "tool not allowlisted",
			provisioner: `  - tool: bash
    tags: [core]
    spec:
      scope: global
`,
			want: "provisioners[0].tool must be one of claude, codegraph, codex, skills, zimfw",
		},
		{
			name: "retired gentle-ai tool",
			provisioner: `  - tool: gentle-ai
    tags: [agents]
    spec:
      scope: global
      agents: [codex]
`,
			want: "provisioners[0].tool must be one of claude, codegraph, codex, skills, zimfw",
		},
		{
			name: "claude spec sets neither marketplace nor plugin",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      from: chrome-devtools-plugins
`,
			want: "provisioners[0].spec must set exactly one of marketplace, plugin, or mcp for the claude tool",
		},
		{
			name: "claude spec sets both marketplace and plugin",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: ChromeDevTools/chrome-devtools-mcp
      plugin: chrome-devtools-mcp
`,
			want: "provisioners[0].spec must set exactly one of marketplace, plugin, or mcp for the claude tool",
		},
		{
			name: "claude plugin without from",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      plugin: chrome-devtools-mcp
`,
			want: "provisioners[0].spec.from is required when plugin is set",
		},
		{
			name: "claude marketplace with stray from",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: ChromeDevTools/chrome-devtools-mcp
      from: chrome-devtools-plugins
`,
			want: "provisioners[0].spec.from is only valid alongside plugin",
		},
		{
			name: "claude marketplace with command",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: ChromeDevTools/chrome-devtools-mcp
      command: [npx, chrome-devtools-mcp@latest]
`,
			want: "provisioners[0].spec.command is only valid when mcp is set",
		},
		{
			name: "claude plugin with env",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      plugin: chrome-devtools-mcp
      from: chrome-devtools-plugins
      env:
        CHROME_DEVTOOLS_MCP_NO_UPDATE_CHECKS: "1"
`,
			want: "provisioners[0].spec.env is only valid when mcp is set",
		},
		{
			name: "claude spec mixes marketplace and mcp",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: ChromeDevTools/chrome-devtools-mcp
      mcp: chrome-devtools
      command: [npx, chrome-devtools-mcp@latest]
`,
			want: "provisioners[0].spec must set exactly one of marketplace, plugin, or mcp for the claude tool",
		},
		{
			name: "codex spec without mcp name",
			provisioner: `  - tool: codex
    tags: [core]
    spec:
      command: [npx, chrome-devtools-mcp@latest]
`,
			want: "provisioners[0].spec.mcp is required for the codex tool",
		},
		{
			name: "codex spec with mcp but no command",
			provisioner: `  - tool: codex
    tags: [core]
    spec:
      mcp: chrome-devtools
`,
			want: "provisioners[0].spec.command is required when mcp is set",
		},
		{
			name: "codex spec mixes claude fields",
			provisioner: `  - tool: codex
    tags: [core]
    spec:
      mcp: chrome-devtools
      command: [npx, chrome-devtools-mcp@latest]
      from: chrome-devtools-plugins
`,
			want: "provisioners[0].spec must not set claude fields (marketplace, plugin, from) for the codex tool",
		},
		{
			name: "codegraph spec requires agents",
			provisioner: `  - tool: codegraph
    tags: [core]
    spec:
      scope: global
      yes: true
`,
			want: "provisioners[0].spec.agents is required for the codegraph tool",
		},
		{
			name: "codegraph spec rejects unsupported scope",
			provisioner: `  - tool: codegraph
    tags: [core]
    spec:
      scope: workspace
      agents: [codex]
      yes: true
`,
			want: "provisioners[0].spec.scope must be one of global, local for the codegraph tool",
		},
		{
			name: "codegraph spec requires yes",
			provisioner: `  - tool: codegraph
    tags: [core]
    spec:
      scope: global
      agents: [codex]
`,
			want: "provisioners[0].spec.yes must be true for the codegraph tool",
		},
		{
			name: "codegraph spec rejects unsupported agent target",
			provisioner: `  - tool: codegraph
    tags: [core]
    spec:
      scope: global
      agents: [claude-code]
      yes: true
`,
			want: "provisioners[0].spec.agents[0] must be one of antigravity, claude, codex, opencode for the codegraph tool",
		},
		{
			name: "skills spec without package",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      agents: [codex]
`,
			want: "provisioners[0].spec.package is required for the skills tool",
		},
		{
			name: "skills spec rejects flag-like package",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: --help
      global: true
`,
			want: "provisioners[0].spec.package must be a package reference, not a CLI flag",
		},
		{
			name:        "skills spec rejects package control characters",
			provisioner: "  - tool: skills\n    tags: [core]\n    spec:\n      package: \"vercel-labs/agent-skills\\u001f\"\n      global: true\n",
			want:        "provisioners[0].spec.package must not contain control characters",
		},
		{
			name: "skills spec rejects package outside allowlisted ref format",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: agent-skills
      global: true
`,
			want: "provisioners[0].spec.package must be an owner/repo package reference with optional path or @ref",
		},
		{
			name: "zimfw spec rejects mcp fields",
			provisioner: `  - tool: zimfw
    tags: [core]
    spec:
      yes: true
      mcp: codegraph
`,
			want: "provisioners[0].spec must not set MCP fields (mcp, command, env) for the zimfw tool",
		},
		{
			name: "skills spec requires global true when missing",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
`,
			want: "provisioners[0].spec.global must be true for the skills tool",
		},
		{
			name: "skills spec requires global true when false",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
      global: false
`,
			want: "provisioners[0].spec.global must be true for the skills tool",
		},
		{
			name: "skills spec rejects flag-like agent",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
      agents: [--global]
      global: true
`,
			want: "provisioners[0].spec.agents[0] must be data, not a CLI flag",
		},
		{
			name: "skills spec rejects flag-like skill",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
      skills: [--help]
      global: true
`,
			want: "provisioners[0].spec.skills[0] must be data, not a CLI flag",
		},
		{
			name: "skills spec mixes claude fields",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
      plugin: chrome-devtools-mcp
`,
			want: "provisioners[0].spec must not set claude fields (marketplace, plugin, from) for the skills tool",
		},
		{
			name: "skills spec mixes codex fields",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
      mcp: chrome-devtools
`,
			want: "provisioners[0].spec must not set MCP fields (mcp, command, env) for the skills tool",
		},
		{
			name: "missing spec",
			provisioner: `  - tool: claude
    tags: [core]
`,
			want: "provisioners[0].spec is required",
		},
		{
			name: "missing tags",
			provisioner: `  - tool: claude
    tags: []
    spec:
      marketplace: example/tools
`,
			want: "provisioners[0].tags is required",
		},
		{
			name: "empty tag value",
			provisioner: `  - tool: claude
    tags: ["", core]
    spec:
      marketplace: example/tools
`,
			want: "provisioners[0].tags[0] must not be empty",
		},
		{
			name: "unsupported os filter",
			provisioner: `  - tool: claude
    tags: [core]
    os: [windows]
    spec:
      marketplace: example/tools
`,
			want: "provisioners[0].os[0] must be one of darwin, linux",
		},
		{
			name: "empty agent value",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: example/tools
      agents: ["  "]
      global: true
`,
			want: "provisioners[0].spec.agents[0] must not be empty",
		},
		{
			name: "empty skill value",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: example/tools
      skills: ["  "]
      global: true
`,
			want: "provisioners[0].spec.skills[0] must not be empty",
		},
		{
			name: "dependency without name",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - brew: example/tools
`,
			want: "provisioners[0].dependencies[0].name is required",
		},
		{
			name: "dependency with whitespace-only manual guidance",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: ghostty
        manual: "  "
`,
			want: "provisioners[0].dependencies[0].manual must not be empty",
		},
		{
			name: "dependency with whitespace-only Debian manual guidance",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: ghostty
        manual_debian: "  "
`,
			want: "provisioners[0].dependencies[0].manual_debian must not be empty",
		},
		{
			name: "dependency with whitespace-only brew cask",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: CascadiaCode Nerd Font
        brew_cask: "  "
        font_match: "CascadiaCodeNF*"
`,
			want: "provisioners[0].dependencies[0].brew_cask must not be empty",
		},
		{
			name: "dependency with ambiguous homebrew formula and cask",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: CascadiaCode Nerd Font
        brew: cascadia-code
        brew_cask: font-cascadia-code-nf
`,
			want: "provisioners[0].dependencies[0] must not set both brew and brew_cask",
		},
		{
			name: "dependency with empty font fallback match",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: CascadiaCode Nerd Font
        font_match: "CascadiaCodeNF*"
        font_fallback_matches: ["  "]
`,
			want: "provisioners[0].dependencies[0].font_fallback_matches[0] must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "dots.yaml")
			if err := os.WriteFile(path, []byte(base+tt.provisioner), 0o600); err != nil {
				t.Fatalf("write manifest: %v", err)
			}

			_, err := manifest.LoadFile(path)
			if err == nil {
				t.Fatal("LoadFile() error = nil, want validation error")
			}
			if err.Error() != tt.want {
				t.Fatalf("LoadFile() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestRepositoryManifestIncludesMVPConfigurationSet(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	entriesByTarget := map[string]manifest.Entry{}
	for _, entry := range got.Entries {
		entriesByTarget[entry.Target] = entry
	}

	tests := []struct {
		name     string
		target   string
		source   string
		strategy string
		tag      string
		dep      string
	}{
		{name: "zsh", target: "~/.zshrc", source: "configs/zsh/loader.zsh", strategy: "copy", tag: "zsh", dep: "zsh"},
		{name: "git", target: "~/.gitconfig", source: "configs/git/loader.gitconfig", strategy: "copy", tag: "git", dep: "git"},
		{name: "starship", target: "~/.config/starship.toml", source: "configs/starship/starship.toml", strategy: "symlink", tag: "starship", dep: "starship"},
		{name: "tmux", target: "~/.tmux.conf", source: "configs/tmux/tmux.conf", strategy: "symlink", tag: "tmux", dep: "tmux"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := entriesByTarget[tt.target]
			if !ok {
				t.Fatalf("repository manifest missing MVP entry for target %q", tt.target)
			}
			if entry.Source != tt.source {
				t.Errorf("Source = %q, want %q", entry.Source, tt.source)
			}
			if entry.Strategy != tt.strategy {
				t.Errorf("Strategy = %q, want %q", entry.Strategy, tt.strategy)
			}
			if !sameStrings(entry.Tags, []string{tt.tag}) {
				t.Errorf("Tags = %#v, want %q", entry.Tags, tt.tag)
			}
			if !sameStrings(entry.OS, []string{"darwin", "linux"}) {
				t.Errorf("OS = %#v, want [darwin linux]", entry.OS)
			}
			if !hasDependency(entry.Dependencies, tt.dep) {
				t.Errorf("Dependencies = %#v, want %q", entry.Dependencies, tt.dep)
			}
		})
	}
	if got := entriesByTarget["~/.zshrc"].Ownership; got != "marked-block" {
		t.Errorf("zsh ownership = %q, want marked-block", got)
	}
	if got := entriesByTarget["~/.gitconfig"].Ownership; got != "marked-block" {
		t.Errorf("git ownership = %q, want marked-block", got)
	}
	atuin, ok := entriesByTarget["~/.config/atuin/config.toml"]
	if !ok || atuin.Source != "configs/atuin/config.toml" || atuin.Strategy != "copy" || atuin.Ownership != "toml-subset" {
		t.Errorf("Atuin config entry = %#v, want copy with TOML Subset Ownership", atuin)
	}
	bat, ok := entriesByTarget["~/.config/bat/config"]
	if !ok || bat.Source != "configs/bat/config" || bat.Strategy != "copy" || bat.Ownership != "" {
		t.Errorf("bat config entry = %#v, want copy with implicit Whole-Target Ownership", bat)
	}
	zellij, ok := entriesByTarget["~/.config/zellij/config.kdl"]
	if !ok || zellij.Source != "configs/zellij/config.kdl" || zellij.Strategy != "copy" || zellij.Ownership != "" {
		t.Errorf("Zellij config entry = %#v, want copy with implicit Whole-Target Ownership", zellij)
	}
	portable, ok := entriesByTarget["~/.config/dots/zsh/zshrc"]
	if !ok || portable.Source != "configs/zsh/zshrc" || portable.Strategy != "symlink" {
		t.Errorf("portable zsh entry = %#v, want managed symlink", portable)
	}
	portableGit, ok := entriesByTarget["~/.config/dots/git/gitconfig"]
	if !ok || portableGit.Source != "configs/git/gitconfig" || portableGit.Strategy != "symlink" {
		t.Errorf("portable git entry = %#v, want managed symlink", portableGit)
	}
}

func TestRepositoryManifestIncludesTuicrCoreConfiguration(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	if _, ok := got.Profiles["tuicr"]; ok {
		t.Fatal("repository manifest defines a dedicated tuicr Profile, want core ownership")
	}
	tuicrTag, ok := got.Tags["tuicr"]
	if !ok || tuicrTag.Kind != "surface" || tuicrTag.Status != "current" {
		t.Fatalf("repository manifest tuicr Tag = %#v, want current surface", tuicrTag)
	}

	var entry *manifest.Entry
	for i := range got.Entries {
		if got.Entries[i].Target == "~/.config/tuicr/config.toml" {
			entry = &got.Entries[i]
			break
		}
	}
	if entry == nil {
		t.Fatal("repository manifest missing tuicr Managed Entry")
	}
	if entry.Source != "configs/tuicr/config.toml" || entry.Strategy != "copy" || entry.Ownership != "toml-subset" {
		t.Fatalf("tuicr config entry = %#v, want copy with TOML Subset Ownership", *entry)
	}
	if !sameStrings(entry.Tags, []string{"tuicr"}) || !sameStrings(entry.OS, []string{"darwin", "linux"}) {
		t.Fatalf("tuicr config scope = tags %#v, OS %#v; want tuicr on darwin and linux", entry.Tags, entry.OS)
	}
	if len(entry.Dependencies) != 1 {
		t.Fatalf("tuicr dependencies = %#v, want one dependency", entry.Dependencies)
	}
	dependency := entry.Dependencies[0]
	if dependency.Name != "tuicr" || dependency.Command != "tuicr" || dependency.Brew != "agavra/tap/tuicr" || !dependency.LinuxHomebrew {
		t.Fatalf("tuicr dependency = %#v, want command tuicr through agavra/tap/tuicr with Linuxbrew enabled", dependency)
	}

	config, err := os.ReadFile(filepath.Join(root, entry.Source))
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", entry.Source, err)
	}
	if string(config) != "theme = \"catppuccin-mocha\"\n" {
		t.Fatalf("tuicr config = %q, want Catppuccin Mocha baseline", config)
	}

	for _, provisioner := range got.Provisioners {
		if provisioner.Spec.Package == "agavra/tuicr" {
			t.Fatalf("repository manifest includes tuicr skill Provisioner: %#v", provisioner)
		}
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func hasDependency(deps []manifest.Dependency, want string) bool {
	for _, dep := range deps {
		if dep.Name == want {
			return true
		}
	}
	return false
}

func findDependency(deps []manifest.Dependency, want string) *manifest.Dependency {
	for i := range deps {
		if deps[i].Name == want {
			return &deps[i]
		}
	}
	return nil
}

func findEntry(entries []manifest.Entry, target string) *manifest.Entry {
	for i := range entries {
		if entries[i].Target == target {
			return &entries[i]
		}
	}
	return nil
}

func TestRepositoryManagedConfigsExposeLocalExtensionPoints(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	tests := []struct {
		name     string
		path     string
		contains []string
	}{
		{
			name: "zsh has private local include",
			path: "configs/zsh/zshrc",
			contains: []string{
				"Machine-specific overrides and secrets",
				".zshrc.local",
			},
		},
		{
			name: "git has private local include",
			path: "configs/git/loader.gitconfig",
			contains: []string{
				"established machine-local extension",
				"path = ~/.gitconfig.local",
			},
		},
		{
			name: "tmux has private local include",
			path: "configs/tmux/tmux.conf",
			contains: []string{
				"Machine-specific overrides and secrets",
				"source-file ~/.tmux.conf.local",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(root, tt.path))
			if err != nil {
				t.Fatalf("ReadFile(%q) error = %v", tt.path, err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(string(content), want) {
					t.Fatalf("%s does not contain %q", tt.path, want)
				}
			}
		})
	}
}

func TestRepositoryManagedZshExposesClaudeProxyHarness(t *testing.T) {
	path := filepath.Clean(filepath.Join("..", "..", "configs", "zsh", "rc.d", "post", "60-ai.zsh"))
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}

	for _, want := range []string{
		"command -v claude >/dev/null 2>&1 && command -v cliproxyapi >/dev/null 2>&1",
		"alias claudex=",
		"ANTHROPIC_BASE_URL=http://127.0.0.1:8317",
		"ANTHROPIC_AUTH_TOKEN=sk-dummy",
		"CLAUDE_CODE_SUBAGENT_MODEL=gpt-5.6-sol",
		"CLAUDE_CODE_ALWAYS_ENABLE_EFFORT=1",
		"CLAUDE_CODE_MAX_TOOL_USE_CONCURRENCY=3",
		"ENABLE_TOOL_SEARCH=false",
		"claude --model gpt-5.6-sol",
	} {
		if !strings.Contains(string(content), want) {
			t.Errorf("%s missing %q", path, want)
		}
	}
}

func TestRepositoryGitConfigClassifiesPortableAndLocalConcerns(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	managedPath := filepath.Join(root, "configs/git/gitconfig")
	managedBytes, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", managedPath, err)
	}
	managed := string(managedBytes)

	for _, want := range []string{
		"defaultBranch = main",
		"rebase = false",
		"conflictStyle = diff3",
	} {
		if !strings.Contains(managed, want) {
			t.Fatalf("managed gitconfig missing portable/local boundary %q:\n%s", want, managed)
		}
	}
	if strings.Contains(managed, "gitconfig.local") {
		t.Fatalf("portable gitconfig includes the local extension directly:\n%s", managed)
	}

	loaderPath := filepath.Join(root, "configs/git/loader.gitconfig")
	loaderBytes, err := os.ReadFile(loaderPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", loaderPath, err)
	}
	loader := string(loaderBytes)
	portableIndex := strings.Index(loader, "path = ~/.config/dots/git/gitconfig")
	localIndex := strings.Index(loader, "path = ~/.gitconfig.local")
	if portableIndex < 0 || localIndex < 0 || portableIndex >= localIndex {
		t.Fatalf("git loader include order is not portable then local:\n%s", loader)
	}

	normalizedManaged := strings.ToLower(managed)
	for _, forbidden := range []string{
		"[user]",
		"signingkey",
		"gpgsign",
		"[credential]",
		"credential.helper",
		"[credential ",
		"includeif \"gitdir:~/",
		"includeif.gitdir:",
		"/users/",
		"/home/",
		"~/documents/",
	} {
		if strings.Contains(normalizedManaged, forbidden) {
			t.Fatalf("managed gitconfig contains local/private concern %q:\n%s", forbidden, managed)
		}
	}

	docPath := filepath.Join(root, "configs/git/README.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", docPath, err)
	}
	doc := string(docBytes)
	for _, want := range []string{
		"Portable",
		"Local/private",
		"Generated",
		"Machine-specific",
		"gitconfig.local.example",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("git config documentation missing %q:\n%s", want, doc)
		}
	}

	examplePath := filepath.Join(root, "configs/git/gitconfig.local.example")
	exampleBytes, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", examplePath, err)
	}
	example := string(exampleBytes)
	for _, want := range []string{
		"[user]",
		"name =",
		"email =",
		"signingKey =",
		"[credential]",
		"[includeIf \"gitdir:~/work/\"]",
	} {
		if !strings.Contains(example, want) {
			t.Fatalf("gitconfig.local.example missing %q:\n%s", want, example)
		}
	}
	for _, line := range strings.Split(example, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		normalized := strings.ToLower(strings.ReplaceAll(trimmed, " ", ""))
		if strings.Contains(normalized, "helper=store") || strings.Contains(normalized, "credential.helper=store") {
			t.Fatalf("gitconfig.local.example contains uncommented plaintext credential store guidance: %q", line)
		}
	}
}

func TestRepositoryStarshipConfigClassifiesPortablePromptSafely(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	managedPath := filepath.Join(root, "configs/starship/starship.toml")
	managedBytes, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", managedPath, err)
	}
	managed := string(managedBytes)

	if strings.TrimSpace(managed) == "" {
		t.Fatalf("managed starship config is empty")
	}

	for _, want := range []string{
		`palette = "catppuccin_mocha"`,
		"[palettes.catppuccin_mocha]",
		"format = ",
		"[character]",
		"[cmd_duration]",
		"[time]",
	} {
		if !strings.Contains(managed, want) {
			t.Fatalf("managed starship config missing portable prompt segment %q:\n%s", want, managed)
		}
	}

	// Guard against committing machine-specific paths or private values. The
	// [username] module renders Starship's own $user variable at runtime, so a
	// literal username must never appear in the tracked file.
	normalizedManaged := strings.ToLower(managed)
	for _, forbidden := range []string{
		"/users/",
		"/home/",
		"argote",
		"yerson",
		"[custom",
		"env_var",
		"password",
		"secret",
		"token",
		"api_key",
		"localhost",
	} {
		if strings.Contains(normalizedManaged, forbidden) {
			t.Fatalf("managed starship config contains machine-specific/private concern %q:\n%s", forbidden, managed)
		}
	}

	docPath := filepath.Join(root, "configs/starship/README.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", docPath, err)
	}
	doc := string(docBytes)
	for _, want := range []string{
		"Portable",
		"Machine-specific",
		"Private",
		"Nerd Font",
		// Starship has no native include; the README must say so honestly
		// instead of documenting a broken local-override file.
		"no native include",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("starship config documentation missing %q:\n%s", want, doc)
		}
	}

	// Starship has no include mechanism, so the repository must NOT ship a
	// local-override file that Starship would silently ignore.
	for _, stray := range []string{
		"configs/starship/starship.local.toml",
		"configs/starship/starship.local.example",
		"configs/starship/starship.toml.local",
	} {
		if _, err := os.Stat(filepath.Join(root, stray)); err == nil {
			t.Fatalf("repository ships a broken Starship local-override file Starship cannot read: %s", stray)
		}
	}
}

func TestRepositoryTmuxConfigClassifiesPortableConfigSafely(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))

	managedPath := filepath.Join(root, "configs/tmux/tmux.conf")
	managedBytes, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", managedPath, err)
	}
	managed := string(managedBytes)

	if strings.TrimSpace(managed) == "" {
		t.Fatalf("managed tmux config is empty")
	}

	for _, want := range []string{
		"set -g @plugin 'tmux-plugins/tpm'",
		"set -g @plugin 'catppuccin/tmux#v2.3.0'",
		`.config/dots/theme.sh`,
		`dots_catppuccin_flavor`,
		`set -g @catppuccin_flavor "latte"`,
		`set -g @catppuccin_flavor "mocha"`,
		`@dots_catppuccin_status_icon_fg`,
		`@catppuccin_status_directory_icon_fg`,
		`#{E:@dots_catppuccin_status_icon_fg}`,
		`set -g @catppuccin_reset "true"`,
		`#{@thm_text}`,
		"set -g prefix C-a",
		"set -g mode-keys vi",
		"set -g status-position top",
		`set -as terminal-features ",*:RGB"`,
		`set-environment -g COLORTERM "truecolor"`,
		"display-popup",
		"source-file ~/.tmux.conf.local",
	} {
		if !strings.Contains(managed, want) {
			t.Fatalf("managed tmux config missing portable segment %q:\n%s", want, managed)
		}
	}

	// Guard against committing active machine-specific shell wiring or private
	// values. Comments may explain why those concerns are excluded, so only
	// non-comment configuration lines are checked for shell commands.
	for _, line := range strings.Split(managed, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		for _, forbidden := range []string{
			"default-command",
			"default-shell",
		} {
			if strings.Contains(trimmed, forbidden) {
				t.Fatalf("managed tmux config contains active machine-specific shell setting %q:\n%s", line, managed)
			}
		}
	}

	activeTmuxConfig := make([]string, 0)
	for _, line := range strings.Split(managed, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		activeTmuxConfig = append(activeTmuxConfig, trimmed)
	}
	normalizedManaged := strings.ToLower(strings.Join(activeTmuxConfig, "\n"))
	for _, forbidden := range []string{
		"/users/",
		"/bin/zsh",
		"argote",
		"yerson",
		"password",
		"secret",
		"token",
		"api_key",
	} {
		if strings.Contains(normalizedManaged, forbidden) {
			t.Fatalf("managed tmux config contains machine-specific/private concern %q:\n%s", forbidden, managed)
		}
	}

	tmuxDir := filepath.Join(root, "configs/tmux")
	if err := filepath.WalkDir(tmuxDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == "plugins" || d.Name() == ".tmux" {
			t.Fatalf("repository ships generated tmux plugin state under %s", path)
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir(%q) error = %v", tmuxDir, err)
	}

	docPath := filepath.Join(root, "configs/tmux/README.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", docPath, err)
	}
	doc := string(docBytes)
	for _, want := range []string{
		"Portable",
		"Machine-specific",
		"Generated",
		"Private",
		"Tmux Plugin Manager",
		"~/.tmux.conf.local",
		"Sandbox validation",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("tmux config documentation missing %q:\n%s", want, doc)
		}
	}
}

func TestRepositoryZellijConfigClassifiesPortableConfigSafely(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	entriesByTarget := map[string]manifest.Entry{}
	for _, entry := range got.Entries {
		entriesByTarget[entry.Target] = entry
	}

	tests := []struct {
		name   string
		target string
		source string
	}{
		{
			name:   "config",
			target: "~/.config/zellij/config.kdl",
			source: "configs/zellij/config.kdl",
		},
		{
			name:   "default layout",
			target: "~/.config/zellij/layouts/default.kdl",
			source: "configs/zellij/layouts/default.kdl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := entriesByTarget[tt.target]
			if !ok {
				t.Fatalf("repository manifest missing Zellij entry for target %q", tt.target)
			}
			if entry.Source != tt.source {
				t.Errorf("Source = %q, want %q", entry.Source, tt.source)
			}
			wantStrategy := "symlink"
			if tt.name == "config" {
				wantStrategy = "copy"
			}
			if entry.Strategy != wantStrategy {
				t.Errorf("Strategy = %q, want %s", entry.Strategy, wantStrategy)
			}
			if !sameStrings(entry.Tags, []string{"zellij"}) {
				t.Errorf("Tags = %#v, want zellij tag", entry.Tags)
			}
			if !sameStrings(entry.OS, []string{"darwin", "linux"}) {
				t.Errorf("OS = %#v, want [darwin linux]", entry.OS)
			}
			if !hasDependency(entry.Dependencies, "zellij") {
				t.Errorf("Dependencies = %#v, want zellij", entry.Dependencies)
			}
		})
	}

	configPath := filepath.Join(root, "configs/zellij/config.kdl")
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", configPath, err)
	}
	config := string(configBytes)

	if strings.TrimSpace(config) == "" {
		t.Fatalf("managed Zellij config is empty")
	}

	for _, want := range []string{
		`keybinds clear-defaults=true`,
		`bind "Ctrl g" { SwitchToMode "normal"; }`,
		`bind "Ctrl g" { SwitchToMode "locked"; }`,
		`theme "catppuccin-mocha"`,
		`default_mode "locked"`,
		`mouse_mode true`,
		`scroll_buffer_size 10000`,
		`default_layout "default"`,
		`scrollback_editor "nvim"`,
		`plugins {`,
		`session-manager location="zellij:session-manager"`,
		`plugin-manager location="zellij:plugin-manager"`,
		`pane_frames {`,
		`rounded_corners true`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("managed Zellij config missing portable segment %q:\n%s", want, config)
		}
	}

	for _, notWant := range []string{`theme_light "catppuccin-latte"`, `theme_dark "catppuccin-mocha"`} {
		if strings.Contains(config, notWant) {
			t.Fatalf("default Zellij config contains untagged adaptive segment %q:\n%s", notWant, config)
		}
	}

	layoutPath := filepath.Join(root, "configs/zellij/layouts/default.kdl")
	layoutBytes, err := os.ReadFile(layoutPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", layoutPath, err)
	}
	layout := string(layoutBytes)
	for _, want := range []string{
		`layout {`,
		`default_tab_template {`,
		`plugin location="file:~/.config/zellij/plugins/zjstatus.wasm"`,
		`datetime_timezone "America/Bogota"`,
	} {
		if !strings.Contains(layout, want) {
			t.Fatalf("managed Zellij layout missing portable segment %q:\n%s", want, layout)
		}
	}

	for name, content := range map[string]string{
		"config": config,
		"layout": layout,
	} {
		normalized := strings.ToLower(content)
		for _, forbidden := range []string{
			"/users/",
			"/home/",
			"argote",
			"yerson",
			"password",
			"secret",
			"token",
			"api_key",
			"hostname",
		} {
			if strings.Contains(normalized, forbidden) {
				t.Fatalf("managed Zellij %s contains machine-specific/private concern %q:\n%s", name, forbidden, content)
			}
		}
	}

	zellijDir := filepath.Join(root, "configs/zellij")
	if err := filepath.WalkDir(zellijDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "plugins", "cache", "logs", "sessions":
				t.Fatalf("repository ships generated Zellij runtime state under %s", path)
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		for _, forbiddenSuffix := range []string{".wasm", ".bak", ".log"} {
			if strings.HasSuffix(name, forbiddenSuffix) {
				t.Fatalf("repository ships generated Zellij runtime file %s", path)
			}
		}
		for _, forbiddenName := range []string{"plugexit", "plugin-manager"} {
			if name == forbiddenName {
				t.Fatalf("repository ships generated Zellij runtime file %s", path)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("WalkDir(%q) error = %v", zellijDir, err)
	}

	docPath := filepath.Join(root, "configs/zellij/README.md")
	docBytes, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", docPath, err)
	}
	doc := string(docBytes)
	for _, want := range []string{
		"Portable",
		"Machine-specific",
		"Generated",
		"Private",
		"ZELLIJ_CONFIG_FILE",
		"ZELLIJ_CONFIG_DIR",
		"zellij --config",
		"zellij --config-dir",
		"zjstatus",
		"manual prerequisite",
		"America/Bogota",
		"Sandbox validation",
		"adaptive-theme",
		"config-adaptive.kdl",
	} {
		if !strings.Contains(doc, want) {
			t.Fatalf("Zellij config documentation missing %q:\n%s", want, doc)
		}
	}
}

func TestRepositoryTmuxConfigResetsGeneratedCatppuccinStateBeforeReload(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	managedPath := filepath.Join(root, "configs/tmux/tmux.conf")
	managedBytes, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", managedPath, err)
	}
	managed := string(managedBytes)

	resetLine := `set -g @catppuccin_reset "true"`
	resetIndex := strings.Index(managed, resetLine)
	if resetIndex == -1 {
		t.Fatalf("managed tmux config missing Catppuccin reset line %q:\n%s", resetLine, managed)
	}

	firstLoadLine := `run "$HOME/.tmux/plugins/tmux/catppuccin.tmux"`
	firstLoadIndex := strings.Index(managed[resetIndex:], firstLoadLine)
	if firstLoadIndex == -1 {
		t.Fatalf("managed tmux config does not load Catppuccin after reset:\n%s", managed)
	}
	firstLoadIndex += resetIndex

	for _, generated := range []string{
		`set -gu @catppuccin_directory_color`,
		`set -gu @catppuccin_status_directory_icon_bg`,
		`set -gu @catppuccin_status_directory_text_fg`,
		`set -gu @catppuccin_status_uptime_text_bg`,
		`set -gu @cpu_low_bg_color`,
		`set -gu @ram_medium_bg_color`,
	} {
		generatedIndex := strings.Index(managed, generated)
		if generatedIndex == -1 {
			t.Fatalf("managed tmux config missing generated module reset %q", generated)
		}
		if generatedIndex < firstLoadIndex {
			t.Fatalf("generated module reset %q occurs before Catppuccin reset load", generated)
		}
	}

	customLine := `set -g @catppuccin_status_directory_icon_fg`
	customIndex := strings.Index(managed, customLine)
	if customIndex == -1 {
		t.Fatalf("managed tmux config missing custom Catppuccin option %q", customLine)
	}
	if customIndex < firstLoadIndex {
		t.Fatalf("custom Catppuccin options are set before reset load; reset would discard them")
	}

	secondLoadIndex := strings.Index(managed[firstLoadIndex+len(firstLoadLine):], firstLoadLine)
	if secondLoadIndex == -1 {
		t.Fatalf("managed tmux config does not reload Catppuccin after custom options:\n%s", managed)
	}
	secondLoadIndex += firstLoadIndex + len(firstLoadLine)
	if secondLoadIndex < customIndex {
		t.Fatalf("Catppuccin reload occurs before custom options; generated modules may ignore dots options")
	}

	for _, preserved := range []string{
		`set -g @catppuccin_window_status_style "rounded"`,
		`set -g @catppuccin_status_background "default"`,
		`set -g @catppuccin_status_directory_icon_fg`,
		`set -g @catppuccin_status_uptime_icon_fg`,
	} {
		if !strings.Contains(managed, preserved) {
			t.Fatalf("managed tmux config missing custom Catppuccin option %q", preserved)
		}
	}
}

func TestRepositoryTmuxCatppuccinFlavorCondition(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	managedPath := filepath.Join(root, "configs/tmux/tmux.conf")
	managedBytes, err := os.ReadFile(managedPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", managedPath, err)
	}

	condition, ok := tmuxIfShellCondition(string(managedBytes), "@catppuccin_flavor")
	if !ok {
		t.Fatalf("managed tmux config missing @catppuccin_flavor if-shell")
	}

	tests := []struct {
		name         string
		uname        string
		defaults     string
		defaultsExit int
		marker       bool
		wantLatte    bool
	}{
		{name: "darwin light with opt-in marker", uname: "Darwin", defaults: "", defaultsExit: 1, marker: true, wantLatte: true},
		{name: "darwin light without marker", uname: "Darwin", defaults: "", defaultsExit: 1, marker: false, wantLatte: false},
		{name: "darwin dark with marker", uname: "Darwin", defaults: "Dark", marker: true, wantLatte: false},
		{name: "darwin unknown with marker", uname: "Darwin", defaults: "Blue", marker: true, wantLatte: false},
		{name: "linux with marker", uname: "Linux", defaults: "", marker: true, wantLatte: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf '%s\\n' '"+tt.uname+"'\n")
			writeDefaultsStub(t, filepath.Join(bin, "defaults"), tt.defaults, tt.defaultsExit)

			home := t.TempDir()
			installAdaptiveThemeHelperForCondition(t, root, home, tt.marker)

			gotLatte := shellConditionSucceeds(t, condition, bin, home)
			if gotLatte != tt.wantLatte {
				t.Fatalf("condition selected latte = %v, want %v; condition: %s", gotLatte, tt.wantLatte, condition)
			}
		})
	}

	t.Run("darwin missing defaults falls back to mocha", func(t *testing.T) {
		bin := t.TempDir()
		writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf '%s\\n' 'Darwin'\n")

		home := t.TempDir()
		installAdaptiveThemeHelperForCondition(t, root, home, true)

		gotLatte := shellConditionSucceeds(t, condition, bin, home)
		if gotLatte {
			t.Fatalf("condition selected latte when defaults(1) was unavailable; condition: %s", condition)
		}
	})
}

func tmuxIfShellCondition(config, marker string) (string, bool) {
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "if-shell '") || !strings.Contains(line, marker) {
			continue
		}
		rest := strings.TrimPrefix(line, "if-shell '")
		end := strings.Index(rest, "'")
		if end == -1 {
			return "", false
		}
		return rest[:end], true
	}
	return "", false
}

func shellConditionSucceeds(t *testing.T, condition, path, home string) bool {
	t.Helper()
	cmd := exec.Command("sh", "-c", condition)
	cmd.Env = append(os.Environ(), "PATH="+path, "HOME="+home)
	err := cmd.Run()
	return err == nil
}

func installAdaptiveThemeHelperForCondition(t *testing.T, root, home string, marker bool) {
	t.Helper()
	dir := filepath.Join(home, ".config", "dots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", dir, err)
	}
	helper, err := os.ReadFile(filepath.Join(root, "configs", "dots", "theme.sh"))
	if err != nil {
		t.Fatalf("read theme helper: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "theme.sh"), helper, 0o755); err != nil {
		t.Fatalf("write theme helper: %v", err)
	}
	if marker {
		if err := os.WriteFile(filepath.Join(dir, "adaptive-theme"), []byte("adaptive-theme\n"), 0o644); err != nil {
			t.Fatalf("write adaptive marker: %v", err)
		}
	}
}

func writeDefaultsStub(t *testing.T, path, output string, exitCode int) {
	t.Helper()
	script := "#!/bin/sh\n"
	if output != "" {
		script += "printf '%s\\n' '" + output + "'\n"
	}
	if exitCode != 0 {
		script += "exit 1\n"
	}
	writeExecutable(t, path, script)
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func TestRepositoryManifestPlansMVPConfigurationSetSafely(t *testing.T) {
	root, err := filepath.Abs(filepath.Clean(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	home := t.TempDir()
	p, err := plan.Build(*got, plan.Options{
		Profile:      "core",
		OS:           "darwin",
		SourceRoot:   root,
		Home:         home,
		XDGStateHome: filepath.Join(home, ".local", "state"),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Derive the expected action count from the Selected Surface instead of
	// repeating its Tag and OS traversal in this command-level plan test.
	profile, ok := got.Profiles["core"]
	if !ok {
		t.Fatal("manifest missing profile \"default\"")
	}
	wantActions := len(selectedsurface.Evaluate(*got, profile.Tags, "darwin").Entries)
	if wantActions == 0 {
		t.Fatal("no managed entries apply to profile \"default\" on darwin; derived plan is empty")
	}
	if len(p.Actions) != wantActions {
		t.Fatalf("len(Actions) = %d, want %d (derived from dots.yaml)", len(p.Actions), wantActions)
	}
	for _, action := range p.Actions {
		if action.Status != plan.StatusCreate {
			t.Fatalf("Action for %s has Status = %q, want %q", action.Target, action.Status, plan.StatusCreate)
		}
	}
}

func TestRepositoryManifestSourcesExist(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	for i, entry := range got.Entries {
		sources := []string{entry.Source}
		for _, override := range entry.SourceOverrides {
			sources = append(sources, override)
		}
		for _, source := range sources {
			sourcePath := filepath.Join(root, source)
			info, err := os.Stat(sourcePath)
			if err != nil {
				t.Fatalf("entries[%d].source %q does not exist at %s: %v", i, source, sourcePath, err)
			}
			// For symlink strategy a directory source is valid: dots creates a directory
			// symlink at the target pointing at the source directory. For copy and
			// template strategies the source must be a regular file because those
			// strategies operate on file content.
			if info.IsDir() && entry.Strategy != "symlink" {
				t.Fatalf("entries[%d].source %q points to a directory, want a file for strategy %q", i, source, entry.Strategy)
			}
		}
	}
}

func TestRepositoryManifestRemainingSymlinksMatchAuditedClassifications(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	got, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	// These classifications are the dated evidence inventory in
	// docs/application-writable-target-research.md. They deliberately describe
	// observed ordinary use rather than claiming that a target is immutable.
	const (
		readUnderOrdinaryUse   = "read-under-ordinary-use"
		explicitOperatorOutput = "explicit-operator-output"
		conditionalInitializer = "conditional-initializer"
	)
	want := map[string]string{
		"configs/zsh/zshrc\x00~/.config/dots/zsh/zshrc":                                               readUnderOrdinaryUse,
		"configs/zsh/zimrc\x00~/.zimrc":                                                               readUnderOrdinaryUse,
		"configs/zsh/zshenv\x00~/.zshenv":                                                             readUnderOrdinaryUse,
		"configs/git/gitconfig\x00~/.config/dots/git/gitconfig":                                       readUnderOrdinaryUse,
		"configs/dots/theme.sh\x00~/.config/dots/theme.sh":                                            readUnderOrdinaryUse,
		"configs/dots/adaptive-theme\x00~/.config/dots/adaptive-theme":                                readUnderOrdinaryUse,
		"configs/starship/starship.toml\x00~/.config/starship.toml":                                   explicitOperatorOutput,
		"configs/tmux/tmux.conf\x00~/.tmux.conf":                                                      readUnderOrdinaryUse,
		"configs/zellij/layouts/default.kdl\x00~/.config/zellij/layouts/default.kdl":                  explicitOperatorOutput,
		"configs/ghostty/config.ghostty\x00~/.config/ghostty/config.ghostty":                          conditionalInitializer,
		"configs/ghostty/adaptive/adaptive-theme.ghostty\x00~/.config/ghostty/adaptive-theme.ghostty": readUnderOrdinaryUse,
		"configs/atuin/themes/catppuccin-mocha.toml\x00~/.config/atuin/themes/catppuccin-mocha.toml":  readUnderOrdinaryUse,
		"configs/nvim\x00~/.config/dots/nvim":                                                         readUnderOrdinaryUse,
		"configs/zed/themes/catppuccin-blue.json\x00~/.config/zed/themes/catppuccin-blue.json":        explicitOperatorOutput,
	}

	found := make(map[string]string)
	for _, entry := range got.Entries {
		if entry.Strategy != "symlink" {
			continue
		}
		key := entry.Source + "\x00" + entry.Target
		classification, ok := want[key]
		if !ok {
			t.Errorf("unaudited symlink Managed Entry %q -> %q", entry.Source, entry.Target)
			continue
		}
		found[key] = classification
	}
	if !reflect.DeepEqual(found, want) {
		t.Fatalf("audited symlinks = %#v, want %#v", found, want)
	}

	for _, writer := range []struct{ source, target string }{
		{"configs/zsh/loader.zsh", "~/.zshrc"},
		{"configs/git/loader.gitconfig", "~/.gitconfig"},
		{"configs/herdr/config.toml", "~/.config/herdr/config.toml"},
		{"configs/zellij/config.kdl", "~/.config/zellij/config.kdl"},
		{"configs/atuin/config.toml", "~/.config/atuin/config.toml"},
		{"configs/bat/config", "~/.config/bat/config"},
		{"configs/nvim/lazy-lock.json", "nvim/lazy-lock.json"},
		{"configs/zed/settings.json", "~/.config/zed/settings.json"},
		{"configs/zed/keymap.json", "~/.config/zed/keymap.json"},
	} {
		for _, entry := range got.Entries {
			if entry.Source == writer.source && entry.Target == writer.target && entry.Strategy == "symlink" {
				t.Errorf("Application-Writable Target %q -> %q must not be a symlink", writer.source, writer.target)
			}
		}
	}
}

func TestRepositoryManifestNeovimEntry(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	entriesByTarget := map[string]manifest.Entry{}
	for _, entry := range got.Entries {
		entriesByTarget[entry.Target] = entry
	}

	entry, ok := entriesByTarget["~/.config/nvim/init.lua"]
	if !ok {
		t.Fatal("repository manifest missing Neovim loader entry")
	}
	if entry.Source != "configs/nvim/loader.lua" {
		t.Errorf("Source = %q, want %q", entry.Source, "configs/nvim/loader.lua")
	}
	if entry.Strategy != "copy" {
		t.Errorf("Strategy = %q, want copy", entry.Strategy)
	}
	if !sameStrings(entry.Tags, []string{"neovim"}) {
		t.Errorf("Tags = %#v, want neovim tag", entry.Tags)
	}
	if !sameStrings(entry.OS, []string{"darwin", "linux"}) {
		t.Errorf("OS = %#v, want [darwin linux]", entry.OS)
	}
	if !hasDependency(entry.Dependencies, "neovim") {
		t.Errorf("Dependencies = %#v, want neovim dependency", entry.Dependencies)
	}
	managed, ok := entriesByTarget["~/.config/dots/nvim"]
	if !ok || managed.Source != "configs/nvim" || managed.Strategy != "symlink" {
		t.Fatalf("managed Neovim entry = %#v, want configs/nvim symlink", managed)
	}
	lock, ok := entriesByTarget["nvim/lazy-lock.json"]
	if !ok || lock.TargetRoot != "xdg-state" || lock.Ownership != "seeded" || lock.Strategy != "copy" {
		t.Fatalf("Neovim lock entry = %#v, want xdg-state seeded copy", lock)
	}

	// The separately Managed Configuration directory must exist in the repository.
	sourcePath := filepath.Join(root, managed.Source)
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("source %q does not exist: %v", sourcePath, err)
	}
	if !info.IsDir() {
		t.Fatalf("source %q is not a directory, want managed Neovim directory", sourcePath)
	}
}

func TestRepositoryManifestZedKeymapIsSeededRuntimeState(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	got, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	for _, entry := range got.Entries {
		if entry.Target != "~/.config/zed/keymap.json" {
			continue
		}
		if entry.Source != "configs/zed/keymap.json" || entry.Strategy != "copy" || entry.Ownership != "seeded" {
			t.Fatalf("Zed keymap entry = %#v, want seeded copy from configs/zed/keymap.json", entry)
		}
		return
	}
	t.Fatal("repository manifest missing Zed keymap entry")
}

func TestDirectorySourceSymlinkStrategyPassesValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim
    target: ~/.config/nvim
    strategy: symlink
    tags: [core]
    os: [darwin, linux]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v; directory source with symlink strategy should pass validation", err)
	}
	if got.Entries[0].Source != "configs/nvim" {
		t.Fatalf("Entry source = %q, want configs/nvim", got.Entries[0].Source)
	}
}

func TestDirectorySourceCopyStrategyIsAcceptedByManifestValidation(t *testing.T) {
	// The manifest itself accepts any source string; the copy strategy rejects
	// directory sources at plan/install time, not at manifest validation time.
	// This test documents the contract: manifest.LoadFile does not reject a copy
	// entry whose source happens to be a directory path — that is caught later.
	dir := t.TempDir()
	path := filepath.Join(dir, "dots.yaml")
	content := []byte(`version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/nvim
    target: ~/.config/nvim
    strategy: copy
    tags: [core]
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	_, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v; manifest validation does not inspect source type", err)
	}
}

func TestRepositoryManifestIncludesCoreDevelopmentBaselineDependencies(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	want := map[string]struct {
		tag       string
		command   string
		commands  []string
		brew      string
		apt       string
		dnf       string
		pacman    string
		toolchain string
		linuxBrew bool
	}{
		"Node LTS (fnm)":       {tag: "node", commands: []string{"fnm", "node"}, brew: "fnm", toolchain: manifest.DependencyToolchainNodeLTSFNM, linuxBrew: true},
		"Rust stable (rustup)": {tag: "rust", commands: []string{"rustup", "rustc", "cargo"}, brew: "rustup", toolchain: manifest.DependencyToolchainRustStableRustup, linuxBrew: true},
		"go":                   {tag: "go", command: "go", brew: "go", linuxBrew: true},
		"uv":                   {tag: "uv", command: "uv", brew: "uv", linuxBrew: true},
		"pnpm":                 {tag: "pnpm", command: "pnpm", brew: "pnpm", linuxBrew: true},
		"bun":                  {tag: "bun", command: "bun", brew: "bun", linuxBrew: true},
		"fzf":                  {tag: "fzf", command: "fzf", brew: "fzf", apt: "fzf", dnf: "fzf", pacman: "fzf"},
		"zoxide":               {tag: "zoxide", command: "zoxide", brew: "zoxide", apt: "zoxide", dnf: "zoxide", pacman: "zoxide"},
		"lazygit":              {tag: "lazygit", command: "lazygit", brew: "lazygit", dnf: "lazygit", pacman: "lazygit", linuxBrew: true},
		"eza":                  {tag: "eza", command: "eza", brew: "eza", apt: "eza", dnf: "eza", pacman: "eza"},
		"ripgrep":              {tag: "ripgrep", command: "rg", brew: "ripgrep", apt: "ripgrep", dnf: "ripgrep", pacman: "ripgrep"},
		"delta":                {tag: "delta", command: "delta", brew: "git-delta", dnf: "git-delta", pacman: "git-delta", linuxBrew: true},
		"unzip":                {tag: "node", command: "unzip", brew: "unzip", apt: "unzip", dnf: "unzip", pacman: "unzip"},
		"fd":                   {tag: "fd", command: "fd", brew: "fd", dnf: "fd-find", pacman: "fd", linuxBrew: true},
		"GitHub CLI":           {tag: "gh", command: "gh", brew: "gh", linuxBrew: true},
		"jq":                   {tag: "jq", command: "jq", brew: "jq", apt: "jq", dnf: "jq", pacman: "jq"},
	}
	for name, wantDep := range want {
		for _, osName := range []string{"darwin", "linux"} {
			dep := findDependency(selectedsurface.Evaluate(*got, []string{wantDep.tag}, osName).Dependencies, name)
			if dep == nil {
				t.Fatalf("Tag %q missing %q Dependency on %s", wantDep.tag, name, osName)
			}
			if dep.Command != wantDep.command || dep.Brew != wantDep.brew || dep.Apt != wantDep.apt || dep.Dnf != wantDep.dnf || dep.Pacman != wantDep.pacman || dep.Toolchain != wantDep.toolchain || dep.LinuxHomebrew != wantDep.linuxBrew || !sameStrings(dep.Commands, wantDep.commands) {
				t.Fatalf("%s dependency = %#v, want command %q, commands %#v, brew %q, apt %q, dnf %q, pacman %q, toolchain %q, linux_homebrew %v", name, *dep, wantDep.command, wantDep.commands, wantDep.brew, wantDep.apt, wantDep.dnf, wantDep.pacman, wantDep.toolchain, wantDep.linuxBrew)
			}
		}
	}
}

func TestRepositoryManifestDeclaresAtomicCoreCapabilitySelection(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	got, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	wantTags := []string{
		"zsh", "zimfw", "git", "starship", "tmux", "herdr", "zellij", "atuin",
		"neovim", "tuicr", "bat", "node", "rust", "go", "uv", "pnpm", "bun",
		"fzf", "zoxide", "lazygit", "eza", "ripgrep", "delta", "fd", "gh", "jq",
	}
	for _, name := range wantTags {
		tag, ok := got.Tags[name]
		if !ok {
			t.Errorf("repository manifest missing atomic Core Tag %q", name)
			continue
		}
		if tag.Kind != "surface" || tag.Status != "current" || strings.TrimSpace(tag.Description) == "" {
			t.Errorf("Tag %q = %#v, want described current surface", name, tag)
		}
	}

	legacy, ok := got.Tags["core"]
	if !ok {
		t.Fatal("repository manifest missing legacy core alias")
	}
	if legacy.Kind != "compatibility" || legacy.Status != "legacy" || !reflect.DeepEqual([]string(legacy.ReplacedBy), wantTags) {
		t.Fatalf("core Tag = %#v, want ordered compatibility alias for %#v", legacy, wantTags)
	}
	if profile := got.Profiles["core"]; !reflect.DeepEqual(profile.Tags, wantTags) {
		t.Fatalf("core Profile Tags = %#v, want %#v", profile.Tags, wantTags)
	}
	wantWorkstation := append(append([]string(nil), wantTags...),
		"ghostty", "warp", "zed", "codex", "claude", "opencode", "antigravity", "copilot")
	if profile := got.Profiles["workstation"]; !reflect.DeepEqual(profile.Tags, wantWorkstation) {
		t.Fatalf("workstation Profile Tags = %#v, want %#v", profile.Tags, wantWorkstation)
	}

	for i, set := range got.Dependencies {
		if hasString(set.Tags, "core") {
			t.Errorf("dependencies[%d] still selects the legacy core Tag: %#v", i, set.Tags)
		}
	}
	for i, entry := range got.Entries {
		if hasString(entry.Tags, "core") {
			t.Errorf("entries[%d] %q still selects the legacy core Tag", i, entry.Target)
		}
	}
	for i, provisioner := range got.Provisioners {
		if hasString(provisioner.Tags, "core") {
			t.Errorf("provisioners[%d] %q still selects the legacy core Tag", i, provisioner.Tool)
		}
	}
}

func TestRepositoryNeovimColorschemeUsesSharedAdaptiveHelper(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	path := filepath.Join(root, "configs/nvim/lua/plugins/colorscheme.lua")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	config := string(data)
	for _, want := range []string{
		`/.config/dots/theme.sh`,
		`dots_catppuccin_flavor`,
		`catppuccin-" .. flavour`,
	} {
		if !strings.Contains(config, want) {
			t.Fatalf("Neovim colorscheme config missing %q:\n%s", want, config)
		}
	}
	if strings.Contains(config, `defaults read -g AppleInterfaceStyle`) {
		t.Fatalf("Neovim colorscheme duplicates macOS defaults probing instead of using shared helper:\n%s", config)
	}
}

func TestDotsThemeHelperAdaptiveMatrix(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	tests := []struct {
		name       string
		uname      string
		defaults   string
		exitCode   int
		marker     bool
		wantFlavor string
	}{
		{name: "tag present macOS light", uname: "Darwin", defaults: "", exitCode: 1, marker: true, wantFlavor: "latte"},
		{name: "tag absent macOS light", uname: "Darwin", defaults: "", exitCode: 1, marker: false, wantFlavor: "mocha"},
		{name: "tag present macOS dark", uname: "Darwin", defaults: "Dark", marker: true, wantFlavor: "mocha"},
		{name: "tag present macOS unknown", uname: "Darwin", defaults: "Blue", marker: true, wantFlavor: "mocha"},
		{name: "tag present Linux", uname: "Linux", defaults: "", marker: true, wantFlavor: "mocha"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bin := t.TempDir()
			writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf '%s\n' '"+tt.uname+"'\n")
			writeDefaultsStub(t, filepath.Join(bin, "defaults"), tt.defaults, tt.exitCode)
			home := t.TempDir()
			installAdaptiveThemeHelperForCondition(t, root, home, tt.marker)

			cmd := exec.Command("sh", "-c", `. "$HOME/.config/dots/theme.sh" && dots_catppuccin_flavor`)
			cmd.Env = append(os.Environ(), "PATH="+bin, "HOME="+home)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("dots_catppuccin_flavor error = %v", err)
			}
			if got := strings.TrimSpace(string(out)); got != tt.wantFlavor {
				t.Fatalf("flavor = %q, want %q", got, tt.wantFlavor)
			}
		})
	}
}

func TestRepositoryHerdrConfigSupportsAdaptiveThemeOverride(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var herdrEntry *manifest.Entry
	for i := range got.Entries {
		if got.Entries[i].Target == "~/.config/herdr/config.toml" {
			herdrEntry = &got.Entries[i]
			break
		}
	}
	if herdrEntry == nil {
		t.Fatalf("manifest missing Herdr managed entry")
	}
	if herdrEntry.Strategy != "copy" || herdrEntry.Ownership != "toml-subset" {
		t.Fatalf("Herdr config entry = %#v, want copy with TOML Subset Ownership", *herdrEntry)
	}
	if got := herdrEntry.SourceOverrides["adaptive-theme"]; got != "configs/herdr/config-adaptive.toml" {
		t.Fatalf("Herdr adaptive source override = %q, want configs/herdr/config-adaptive.toml", got)
	}

	defaultConfig, err := os.ReadFile(filepath.Join(root, "configs/herdr/config.toml"))
	if err != nil {
		t.Fatalf("read default Herdr config: %v", err)
	}
	for _, notWant := range []string{
		`auto_switch = true`,
		`light_name = "catppuccin-latte"`,
	} {
		if strings.Contains(string(defaultConfig), notWant) {
			t.Fatalf("default Herdr config contains opt-in-only setting %q:\n%s", notWant, defaultConfig)
		}
	}

	adaptiveConfig, err := os.ReadFile(filepath.Join(root, "configs/herdr/config-adaptive.toml"))
	if err != nil {
		t.Fatalf("read adaptive Herdr config: %v", err)
	}
	for _, want := range []string{
		`name = "catppuccin"`,
		`auto_switch = true`,
		`dark_name = "catppuccin"`,
		`light_name = "catppuccin-latte"`,
	} {
		if !strings.Contains(string(adaptiveConfig), want) {
			t.Fatalf("adaptive Herdr config missing %q:\n%s", want, adaptiveConfig)
		}
	}

	defaultText := string(defaultConfig)
	for section, want := range map[string]string{
		"spaces": `[ui.sidebar.spaces]
row_gap = 1
rows = [
  ["state_icon", "workspace"],
  ["branch", "git_status"],
]`,
		"agents": `[ui.sidebar.agents]
row_gap = 1
rows = [
  ["state_icon", { token = "agent", fg = "#cba6f7", bold = true }, "state_text"],
  [{ token = "workspace", fg = "#6c7086" }, "tab"],
]`,
	} {
		if got := herdrConfigSection(t, defaultText, "[ui.sidebar."+section+"]"); got != want {
			t.Fatalf("default Herdr %s sidebar section =:\n%s\nwant:\n%s", section, got, want)
		}
	}
	for _, want := range []string{
		`previous_workspace = "prefix+alt+k"`,
		`next_workspace = "prefix+alt+j"`,
		`previous_agent = "prefix+alt+h"`,
		`next_agent = "prefix+alt+l"`,
		`mouse_capture = true`,
		`previous_tab = ["prefix+p", "ctrl+alt+["]`,
		`next_tab = ["prefix+n", "ctrl+alt+]"]`,
		`focus_pane_left = ["prefix+h", "ctrl+alt+h"]`,
		`focus_pane_down = ["prefix+j", "ctrl+alt+j"]`,
		`focus_pane_up = ["prefix+k", "ctrl+alt+k"]`,
		`focus_pane_right = ["prefix+l", "ctrl+alt+l"]`,
	} {
		if !strings.Contains(defaultText, want) {
			t.Fatalf("default Herdr config missing workspace shortcut %q:\n%s", want, defaultText)
		}
	}
	if strings.Contains(defaultText, `cmd+`) {
		t.Fatalf("default Herdr config contains unreliable Command shortcut:\n%s", defaultText)
	}

	adaptiveText := string(adaptiveConfig)
	if strings.Contains(adaptiveText, `cmd+`) {
		t.Fatalf("adaptive Herdr config contains unreliable Command shortcut:\n%s", adaptiveText)
	}
	if got, want := herdrConfigWithoutTheme(t, adaptiveText), herdrConfigWithoutTheme(t, defaultText); got != want {
		t.Fatalf("adaptive Herdr config non-theme sections drifted from default; got:\n%s\nwant:\n%s", got, want)
	}
}

func herdrConfigWithoutTheme(t *testing.T, config string) string {
	t.Helper()
	start := strings.Index(config, "onboarding =")
	if start == -1 {
		t.Fatalf("Herdr config missing onboarding setting:\n%s", config)
	}
	body := config[start:]
	themeStart := strings.Index(body, "[theme]")
	if themeStart == -1 {
		t.Fatalf("Herdr config missing [theme] section:\n%s", config)
	}
	afterThemeHeader := body[themeStart+len("[theme]"):]
	nextSection := strings.Index(afterThemeHeader, "\n[")
	if nextSection == -1 {
		t.Fatalf("Herdr config missing non-theme section after [theme]:\n%s", config)
	}
	themeEnd := themeStart + len("[theme]") + nextSection + 1
	return body[:themeStart] + body[themeEnd:]
}

func herdrConfigSection(t *testing.T, config, header string) string {
	t.Helper()
	start := strings.Index(config, header)
	if start == -1 {
		t.Fatalf("Herdr config missing %s section:\n%s", header, config)
	}
	section := config[start:]
	if nextSection := strings.Index(section[len(header):], "\n["); nextSection != -1 {
		section = section[:len(header)+nextSection]
	}
	return strings.TrimSpace(section)
}

func TestRepositoryGhosttyConfigDoesNotForwardObsoleteHerdrCommandShortcuts(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	configPath := filepath.Join(root, "configs/ghostty/config.ghostty")

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read Ghostty config: %v", err)
	}
	config := string(configBytes)

	for _, notWant := range []string{
		`keybind = cmd+j=unbind`,
		`keybind = cmd+k=unbind`,
		`keybind = cmd+j=scroll_to_selection`,
		`keybind = cmd+k=clear_screen`,
		`keybind = super+j=scroll_to_selection`,
		`keybind = super+k=clear_screen`,
	} {
		if strings.Contains(config, notWant) {
			t.Fatalf("Ghostty config consumes Herdr workspace shortcut %q:\n%s", notWant, config)
		}
	}
}

func TestAgentStatuslinePalettesUseAdaptiveHelper(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, script := range []string{
		"configs/claude/statusline-command.sh",
		"configs/copilot/statusline-command.sh",
	} {
		t.Run(script, func(t *testing.T) {
			bin := t.TempDir()
			writeExecutable(t, filepath.Join(bin, "uname"), "#!/bin/sh\nprintf '%s\n' 'Darwin'\n")
			writeDefaultsStub(t, filepath.Join(bin, "defaults"), "", 1)
			writeExecutable(t, filepath.Join(bin, "jq"), `#!/bin/sh
case "$*" in
  *model*) printf 'opus\n' ;;
esac
`)
			home := t.TempDir()
			installAdaptiveThemeHelperForCondition(t, root, home, true)

			cmd := exec.Command("bash", filepath.Join(root, script))
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOME="+home)
			cmd.Stdin = strings.NewReader(`{"model":{"display_name":"opus"}}`)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("%s error = %v", script, err)
			}
			if !strings.Contains(string(out), "\033[38;2;92;95;119m") {
				t.Fatalf("%s did not use Latte subtext color with adaptive marker; output %q", script, out)
			}
		})
	}
}

func TestResolveSelectionComposesProfilesInOrder(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"core":   {Tags: []string{"core"}},
		"agents": {Tags: []string{"agents", "core"}},
		"web":    {Tags: []string{"web"}},
	}}

	got, err := manifest.ResolveSelection(m, []string{"core", "agents", "web", "agents"}, []string{"web", "sdd"})
	if err != nil {
		t.Fatalf("ResolveSelection() error = %v", err)
	}
	if !reflect.DeepEqual(got.Profiles, []string{"core", "agents", "web"}) {
		t.Fatalf("Profiles = %#v", got.Profiles)
	}
	if !reflect.DeepEqual(got.Tags, []string{"core", "agents", "web", "sdd"}) {
		t.Fatalf("Tags = %#v", got.Tags)
	}
}

func TestNormalizeTagsExpandsLegacyAliasesInDeclaredOrder(t *testing.T) {
	m := manifest.Manifest{Tags: map[string]manifest.Tag{
		"core":       {Kind: "surface", Status: "current"},
		"agents":     {Kind: "surface", Status: "current"},
		"web":        {Kind: "surface", Status: "current"},
		"legacy":     {Kind: "compatibility", Status: "legacy", ReplacedBy: manifest.ReplacementTags{"core", "agents"}},
		"legacy-web": {Kind: "compatibility", Status: "legacy", ReplacedBy: manifest.ReplacementTags{"agents", "web"}},
	}}

	tags, replacements, err := manifest.NormalizeTags(m, []string{"legacy", "agents", "legacy", "legacy-web", "core"})
	if err != nil {
		t.Fatalf("NormalizeTags() error = %v", err)
	}
	if want := []string{"core", "agents", "web"}; !reflect.DeepEqual(tags, want) {
		t.Fatalf("tags = %#v, want %#v", tags, want)
	}
	wantReplacements := []manifest.TagReplacement{
		{LegacyTag: "legacy", ReplacementTags: []string{"core", "agents"}},
		{LegacyTag: "legacy-web", ReplacementTags: []string{"agents", "web"}},
	}
	if !reflect.DeepEqual(replacements, wantReplacements) {
		t.Fatalf("replacements = %#v, want %#v", replacements, wantReplacements)
	}

	if _, _, err := manifest.NormalizeTags(m, []string{"missing"}); err == nil || err.Error() != `tag "missing" is not declared` {
		t.Fatalf("NormalizeTags(missing) error = %v", err)
	}
}

func TestResolveSelectionAllowsTagsWithoutInferringDefaultAndNormalizesAliases(t *testing.T) {
	m := manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"core":   {Kind: "surface", Status: "current"},
			"agents": {Kind: "surface", Status: "current"},
			"legacy": {Kind: "compatibility", Status: "legacy", ReplacedBy: manifest.ReplacementTags{"core", "agents"}},
		},
		Profiles: map[string]manifest.Profile{"default": {Tags: []string{"core"}}},
	}

	got, err := manifest.ResolveSelection(m, nil, []string{"legacy", "core"})
	if err != nil {
		t.Fatalf("ResolveSelection() error = %v", err)
	}
	if len(got.Profiles) != 0 || got.Profile != "" {
		t.Fatalf("Profiles = %#v, want no inferred default", got.Profiles)
	}
	if want := []string{"core", "agents"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("Tags = %#v, want %#v", got.Tags, want)
	}
	if want := []manifest.TagReplacement{{LegacyTag: "legacy", ReplacementTags: []string{"core", "agents"}}}; !reflect.DeepEqual(got.Replacements, want) {
		t.Fatalf("Replacements = %#v, want %#v", got.Replacements, want)
	}
}

func TestResolveReadOnlySelectionAllowsTagsWithoutInferringDefault(t *testing.T) {
	m := manifest.Manifest{Profiles: map[string]manifest.Profile{
		"default": {Tags: []string{"default"}},
	}}

	got, err := manifest.ResolveReadOnlySelection(m, nil, []string{"web", "web"})
	if err != nil {
		t.Fatalf("ResolveReadOnlySelection() error = %v", err)
	}
	if got.Profile != "" || len(got.Profiles) != 0 {
		t.Fatalf("selection = %+v, want no inferred Profile", got)
	}
	if want := []string{"web"}; !reflect.DeepEqual(got.Tags, want) {
		t.Fatalf("Tags = %#v, want %#v", got.Tags, want)
	}
}

func TestResolveSelectionRejectsUndeclaredExtraTagWhenRegistryExists(t *testing.T) {
	m := manifest.Manifest{
		Tags:     map[string]manifest.Tag{"core": {Kind: "surface", Status: "current"}},
		Profiles: map[string]manifest.Profile{"core": {Tags: []string{"core"}}},
	}
	if _, err := manifest.ResolveSelection(m, []string{"core"}, []string{"retired"}); err == nil || !strings.Contains(err.Error(), `tag "retired" is not declared`) {
		t.Fatalf("ResolveSelection() error = %v, want undeclared Tag rejection", err)
	}
}

func TestRepositoryManifestRequiresExplicitProfileSelection(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	got, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	if _, ok := got.Profiles["default"]; ok {
		t.Fatal("repository manifest must not define default profile")
	}
	if _, err := manifest.ResolveSelection(*got, nil, nil); err == nil {
		t.Fatal("ResolveSelection() error = nil, want explicit profile error")
	}
}
