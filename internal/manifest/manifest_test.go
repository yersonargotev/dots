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

func TestLoadFileParsesProfileDependencies(t *testing.T) {
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

	got, err := manifest.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}

	deps := got.Profiles["desktop"].Dependencies
	if len(deps) != 1 {
		t.Fatalf("Profile dependencies len = %d, want 1 (%#v)", len(deps), deps)
	}
	if deps[0].Name != "Desktop Nerd Font" || deps[0].Requirement != "optional" || deps[0].BrewCask != "font-cascadia-code-nf" || deps[0].FontMatch != "CascadiaCodeNF*" || !sameStrings(deps[0].FontFallbackMatches, []string{"CaskaydiaCoveNerdFont*"}) {
		t.Fatalf("Profile dependency = %#v, want desktop font dependency with fallback match", deps[0])
	}
}

func TestLoadFileParsesProvisioners(t *testing.T) {
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
  - tool: gentle-ai
    tags: [core]
    os: [darwin, linux]
    spec:
      action: install
      scope: global
      channel: stable
      persona: neutral
      preset: custom
      sdd-mode: strict
      agents: [codex]
      components: [engram]
      skills: [tdd]
    dependencies:
      - name: gentle-ai
        brew: gentleman-programming/tap/gentle-ai
      - name: engram
        requirement: optional
        brew: gentleman-programming/tap/engram
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
	prov := got.Provisioners[0]
	if prov.Tool != "gentle-ai" {
		t.Fatalf("Provisioner.Tool = %q, want gentle-ai", prov.Tool)
	}
	if !sameStrings(prov.Tags, []string{"core"}) {
		t.Fatalf("Provisioner.Tags = %#v, want [core]", prov.Tags)
	}
	if !sameStrings(prov.OS, []string{"darwin", "linux"}) {
		t.Fatalf("Provisioner.OS = %#v, want [darwin linux]", prov.OS)
	}
	if prov.Spec.Action != "install" || prov.Spec.Scope != "global" || prov.Spec.Channel != "stable" || prov.Spec.Persona != "neutral" || prov.Spec.Preset != "custom" || prov.Spec.SDDMode != "strict" || prov.Spec.Yes {
		t.Fatalf("Provisioner.Spec scalar flags = %#v, want install/global/stable/neutral/custom/strict/no yes", prov.Spec)
	}
	if !sameStrings(prov.Spec.Agents, []string{"codex"}) {
		t.Fatalf("Provisioner.Spec.Agents = %#v, want [codex]", prov.Spec.Agents)
	}
	if !sameStrings(prov.Spec.Components, []string{"engram"}) {
		t.Fatalf("Provisioner.Spec.Components = %#v, want [engram]", prov.Spec.Components)
	}
	if !sameStrings(prov.Spec.Skills, []string{"tdd"}) {
		t.Fatalf("Provisioner.Spec.Skills = %#v, want [tdd]", prov.Spec.Skills)
	}
	if len(prov.Dependencies) != 2 {
		t.Fatalf("Provisioner.Dependencies len = %d, want 2", len(prov.Dependencies))
	}
	if prov.Dependencies[0].Name != "gentle-ai" || prov.Dependencies[0].Brew != "gentleman-programming/tap/gentle-ai" {
		t.Fatalf("Provisioner.Dependencies[0] = %#v, want gentle-ai brew dependency", prov.Dependencies[0])
	}
	if prov.Dependencies[1].Name != "engram" || prov.Dependencies[1].Requirement != "optional" || prov.Dependencies[1].Brew != "gentleman-programming/tap/engram" {
		t.Fatalf("Provisioner.Dependencies[1] = %#v, want optional engram brew dependency", prov.Dependencies[1])
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
	if !hasString(codex.Tags, "web") {
		t.Errorf("codex provisioner %#v missing web tag", codex.Spec)
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

	want := map[string][]string{
		"claude": {"dart", "mcp-server"},
		"codex":  {"dart", "mcp-server", "--force-roots-fallback"},
	}
	for tool, wantCommand := range want {
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
		if !hasString(prov.Tags, "mobile") {
			t.Errorf("%s MCP provisioner %#v missing mobile tag", tool, prov.Spec)
		}
		if !sameStrings(prov.OS, []string{"darwin", "linux"}) {
			t.Errorf("%s MCP provisioner OS = %#v, want [darwin linux]", tool, prov.OS)
		}
		if !sameStrings(prov.Spec.Command, wantCommand) {
			t.Errorf("%s MCP command = %#v, want %#v", tool, prov.Spec.Command, wantCommand)
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
	}{
		{source: "configs/antigravity/mobile-mcp-settings.json", target: "~/.gemini/antigravity-cli/settings.json"},
		{source: "configs/vscode/settings.json", target: "~/Library/Application Support/Code/User/settings.json"},
		{source: "configs/vscode/settings.json", target: "~/.config/Code/User/settings.json"},
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
		if !hasString(entry.Tags, "mobile") {
			t.Errorf("%s tags = %#v, want mobile", source, entry.Tags)
		}
	}
}

func TestRepositoryManifestWebProfileIncludesPlaywrightCLI(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	web := got.Profiles["web"]
	if !hasDependency(web.Dependencies, "Playwright CLI") {
		t.Fatalf("web profile dependencies = %#v, want Playwright CLI dependency", web.Dependencies)
	}

	var dep manifest.Dependency
	for _, candidate := range web.Dependencies {
		if candidate.Name == "Playwright CLI" {
			dep = candidate
			break
		}
	}
	if dep.Command != "playwright-cli" || dep.Brew != "playwright-cli" || !dep.LinuxHomebrew {
		t.Fatalf("Playwright CLI dependency = %#v, want command/brew playwright-cli with linux_homebrew", dep)
	}
}

func TestRepositoryManifestAgentsProfileIncludesGitHubCLI(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var agents *manifest.DependencySet
	for i := range got.Dependencies {
		candidate := &got.Dependencies[i]
		if hasString(candidate.Tags, "agents") {
			agents = candidate
			break
		}
	}
	if agents == nil {
		t.Fatal("repository manifest missing agents dependency set")
	}
	if !sameStrings(agents.OS, []string{"darwin", "linux"}) {
		t.Fatalf("agents dependency set OS = %#v, want [darwin linux]", agents.OS)
	}

	var dep *manifest.Dependency
	for i := range agents.Dependencies {
		candidate := &agents.Dependencies[i]
		if candidate.Name == "GitHub CLI" {
			dep = candidate
			break
		}
	}
	if dep == nil {
		t.Fatalf("agents dependency set missing GitHub CLI: %#v", agents.Dependencies)
	}
	if dep.Command != "gh" || dep.Brew != "gh" || !dep.LinuxHomebrew {
		t.Fatalf("GitHub CLI dependency = %#v, want command/brew gh with linux_homebrew", *dep)
	}
	if dep.Apt != "" || dep.Dnf != "" || dep.Pacman != "" {
		t.Fatalf("GitHub CLI dependency = %#v, want Homebrew/manual Linux handling until distro setup is modeled", *dep)
	}
}

func TestRepositoryManifestLinuxHomebrewReviewBoundary(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	dependencies := make(map[string][]manifest.Dependency)
	for _, profile := range got.Profiles {
		for _, dep := range profile.Dependencies {
			dependencies[dep.Name] = append(dependencies[dep.Name], dep)
		}
	}
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
	for _, dep := range dependencies["ghostty"] {
		if strings.Contains(dep.ManualDebian, "snap install ghostty --classic") && strings.Contains(dep.ManualDebian, "requires sudo") {
			ghosttyManualFound = true
		}
	}
	if !ghosttyManualFound {
		t.Fatalf("ghostty dependency missing explicit Ubuntu manual guidance with snap sudo/interactivity note: %#v", dependencies["ghostty"])
	}

	for _, name := range []string{"bat", "starship", "zellij", "atuin", "pnpm", "gentle-ai", "engram"} {
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
	if !hasString(skills.Tags, "web") {
		t.Errorf("skills provisioner %#v missing web tag", skills.Spec)
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
	if !hasString(skills.Tags, "web") {
		t.Errorf("skills provisioner %#v missing web tag", skills.Spec)
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
	if !hasString(skills.Tags, "web") {
		t.Errorf("skills provisioner %#v missing web tag", skills.Spec)
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
	if !sameStrings(mobile.Tags, []string{"mobile"}) {
		t.Fatalf("mobile profile tags = %#v, want [mobile]", mobile.Tags)
	}

	mobileSkillPackages := []struct {
		name       string
		wantSkills []string
	}{
		{name: "dart-lang/skills"},
		{name: "flutter/skills"},
		{name: "android/skills", wantSkills: []string{"android-cli"}},
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
			if !hasString(skills.Tags, "mobile") {
				t.Errorf("skills provisioner %#v missing mobile tag", skills.Spec)
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

func TestRepositoryManifestIncludesMattPocockEngineeringSkillsProvisioner(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var skills *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool == "skills" && prov.Spec.Package == "mattpocock/skills/skills/engineering" {
			skills = prov
		}
	}

	if skills == nil {
		t.Fatal("repository manifest missing skills provisioner for mattpocock/skills/skills/engineering")
	}
	if !hasString(skills.Tags, "agents") {
		t.Errorf("skills provisioner %#v missing agents tag", skills.Spec)
	}
	if !sameStrings(skills.Spec.Agents, []string{"codex", "claude-code", "antigravity", "opencode", "github-copilot"}) {
		t.Errorf("skills provisioner agents = %#v, want [codex claude-code antigravity opencode github-copilot]", skills.Spec.Agents)
	}
	wantSkills := []string{"ask-matt", "codebase-design", "diagnosing-bugs", "domain-modeling", "grill-with-docs", "implement", "improve-codebase-architecture", "prototype", "resolving-merge-conflicts", "setup-matt-pocock-skills", "tdd", "to-issues", "to-prd", "triage"}
	if !sameStrings(skills.Spec.Skills, wantSkills) {
		t.Errorf("skills provisioner skills = %#v, want %#v", skills.Spec.Skills, wantSkills)
	}
	if !skills.Spec.Global || !skills.Spec.Copy {
		t.Errorf("skills provisioner global/copy = %v/%v, want true/true", skills.Spec.Global, skills.Spec.Copy)
	}
	if !hasDependency(skills.Dependencies, "npx") {
		t.Errorf("skills provisioner missing npx dependency: %#v", skills.Dependencies)
	}
}

func TestRepositoryManifestIncludesMattPocockReviewSkillProvisioner(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var skills *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool == "skills" && prov.Spec.Package == "mattpocock/skills" {
			skills = prov
		}
	}

	if skills == nil {
		t.Fatal("repository manifest missing skills provisioner for mattpocock/skills review bundle")
	}
	if !hasString(skills.Tags, "agents") {
		t.Errorf("skills provisioner %#v missing agents tag", skills.Spec)
	}
	if !sameStrings(skills.Spec.Agents, []string{"codex", "claude-code", "antigravity", "opencode", "github-copilot"}) {
		t.Errorf("skills provisioner agents = %#v, want [codex claude-code antigravity opencode github-copilot]", skills.Spec.Agents)
	}
	if !sameStrings(skills.Spec.Skills, []string{"grilling", "loop-me", "review", "writing-great-skills"}) {
		t.Errorf("skills provisioner skills = %#v, want [grilling loop-me review writing-great-skills]", skills.Spec.Skills)
	}
	if !skills.Spec.Global || !skills.Spec.Copy {
		t.Errorf("skills provisioner global/copy = %v/%v, want true/true", skills.Spec.Global, skills.Spec.Copy)
	}
	if !hasDependency(skills.Dependencies, "npx") {
		t.Errorf("skills provisioner missing npx dependency: %#v", skills.Dependencies)
	}
}

func TestRepositoryManifestIncludesGentleAICleanupBeforeBasicInstall(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	var cleanup, sddInstall, codexInstall, claudeInstall, antigravityInstall, opencodeInstall, copilotInstall, personaInstall *manifest.Provisioner
	for i := range got.Provisioners {
		prov := &got.Provisioners[i]
		if prov.Tool != "gentle-ai" {
			continue
		}
		switch {
		case cleanup == nil && prov.Spec.Action == "uninstall":
			cleanup = prov
		case sddInstall == nil && sameStrings(prov.Spec.Components, []string{"sdd"}):
			sddInstall = prov
		case personaInstall == nil && sameStrings(prov.Spec.Components, []string{"persona"}):
			personaInstall = prov
		case codexInstall == nil && sameStrings(prov.Spec.Agents, []string{"codex"}):
			codexInstall = prov
		case claudeInstall == nil && sameStrings(prov.Spec.Agents, []string{"claude-code"}):
			claudeInstall = prov
		case antigravityInstall == nil && sameStrings(prov.Spec.Agents, []string{"antigravity"}):
			antigravityInstall = prov
		case opencodeInstall == nil && sameStrings(prov.Spec.Agents, []string{"opencode"}):
			opencodeInstall = prov
		case copilotInstall == nil && sameStrings(prov.Spec.Agents, []string{"vscode-copilot"}):
			copilotInstall = prov
		}
	}

	if cleanup == nil {
		t.Fatal("repository manifest missing gentle-ai uninstall cleanup provisioner")
	}
	if sddInstall == nil {
		t.Fatal("repository manifest missing opt-in gentle-ai SDD install provisioner")
	}
	if codexInstall == nil {
		t.Fatal("repository manifest missing gentle-ai codex basic install provisioner")
	}
	if claudeInstall == nil {
		t.Fatal("repository manifest missing gentle-ai claude basic install provisioner")
	}
	if antigravityInstall == nil {
		t.Fatal("repository manifest missing gentle-ai antigravity basic install provisioner")
	}
	if opencodeInstall == nil {
		t.Fatal("repository manifest missing gentle-ai opencode basic install provisioner")
	}
	if copilotInstall == nil {
		t.Fatal("repository manifest missing gentle-ai vscode-copilot basic install provisioner")
	}
	for _, prov := range []*manifest.Provisioner{cleanup, codexInstall, claudeInstall, antigravityInstall, opencodeInstall, copilotInstall} {
		if !sameStrings(prov.Tags, []string{"agents"}) {
			t.Fatalf("gentle-ai provisioner tags = %#v, want [agents] so desktop installs do not apply SDD/gentle-dev agent setup", prov.Tags)
		}
	}
	if cleanup.Spec.Yes != true {
		t.Fatalf("gentle-ai cleanup yes = %v, want true", cleanup.Spec.Yes)
	}
	if !sameStrings(cleanup.Spec.Agents, []string{"codex", "claude-code", "opencode", "antigravity", "vscode-copilot"}) {
		t.Fatalf("gentle-ai cleanup agents = %#v, want [codex claude-code opencode antigravity vscode-copilot]", cleanup.Spec.Agents)
	}
	if !sameStrings(cleanup.Spec.Components, []string{"sdd", "persona"}) {
		t.Fatalf("gentle-ai cleanup components = %#v, want [sdd persona]", cleanup.Spec.Components)
	}
	if !sameStrings(sddInstall.Tags, []string{"sdd"}) {
		t.Fatalf("gentle-ai SDD install tags = %#v, want [sdd] so agents profile only installs SDD with --tag sdd", sddInstall.Tags)
	}
	if personaInstall != nil {
		t.Fatalf("repository manifest must not install gentle-ai persona; found tags %#v spec %#v", personaInstall.Tags, personaInstall.Spec)
	}
	if !sameStrings(sddInstall.Spec.Agents, []string{"codex", "claude-code", "opencode", "antigravity", "vscode-copilot"}) {
		t.Fatalf("gentle-ai SDD install agents = %#v, want [codex claude-code opencode antigravity vscode-copilot]", sddInstall.Spec.Agents)
	}
	if sddInstall.Spec.SDDMode != "multi" {
		t.Fatalf("gentle-ai SDD install mode = %q, want multi", sddInstall.Spec.SDDMode)
	}
	if codexInstall.Spec.Preset != "custom" {
		t.Fatalf("gentle-ai codex install preset = %q, want custom", codexInstall.Spec.Preset)
	}
	if !sameStrings(codexInstall.Spec.Components, []string{"engram", "context7"}) {
		t.Fatalf("gentle-ai codex install components = %#v, want [engram context7]", codexInstall.Spec.Components)
	}
	if hasString(codexInstall.Spec.Components, "permissions") {
		t.Fatalf("gentle-ai codex install components = %#v, must not include permissions because it installs gentle-dev", codexInstall.Spec.Components)
	}
	if claudeInstall.Spec.Preset != "custom" {
		t.Fatalf("gentle-ai claude install preset = %q, want custom", claudeInstall.Spec.Preset)
	}
	if !sameStrings(claudeInstall.Spec.Components, []string{"engram", "context7", "permissions"}) {
		t.Fatalf("gentle-ai claude install components = %#v, want [engram context7 permissions]", claudeInstall.Spec.Components)
	}
	if !sameStrings(antigravityInstall.Spec.Components, []string{"engram", "context7"}) {
		t.Fatalf("gentle-ai antigravity install components = %#v, want [engram context7]", antigravityInstall.Spec.Components)
	}
	if hasString(antigravityInstall.Spec.Components, "sdd") || hasString(antigravityInstall.Spec.Components, "permissions") {
		t.Fatalf("gentle-ai antigravity install components = %#v, must not include sdd or permissions", antigravityInstall.Spec.Components)
	}
	if !sameStrings(opencodeInstall.Spec.Components, []string{"engram", "context7"}) {
		t.Fatalf("gentle-ai opencode install components = %#v, want [engram context7]", opencodeInstall.Spec.Components)
	}
	if hasString(opencodeInstall.Spec.Components, "sdd") || hasString(opencodeInstall.Spec.Components, "permissions") {
		t.Fatalf("gentle-ai opencode install components = %#v, must not include sdd or permissions", opencodeInstall.Spec.Components)
	}
	if !sameStrings(copilotInstall.Spec.Components, []string{"engram", "context7"}) {
		t.Fatalf("gentle-ai vscode-copilot install components = %#v, want [engram context7]", copilotInstall.Spec.Components)
	}
	if hasString(copilotInstall.Spec.Components, "sdd") || hasString(copilotInstall.Spec.Components, "permissions") {
		t.Fatalf("gentle-ai vscode-copilot install components = %#v, must not include sdd or permissions", copilotInstall.Spec.Components)
	}
	for i := range got.Provisioners {
		if &got.Provisioners[i] == cleanup {
			break
		}
		if &got.Provisioners[i] == sddInstall || &got.Provisioners[i] == codexInstall || &got.Provisioners[i] == claudeInstall || &got.Provisioners[i] == antigravityInstall || &got.Provisioners[i] == opencodeInstall || &got.Provisioners[i] == copilotInstall {
			t.Fatal("gentle-ai install provisioner appears before cleanup")
		}
	}
}

func TestRepositoryManifestDesktopProfileDoesNotSelectGentleAIProvisioners(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	manifestPath := filepath.Join(root, "dots.yaml")

	got, err := manifest.LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile(%q) error = %v", manifestPath, err)
	}

	desktop, err := provision.Build(*got, provision.Options{Profile: "desktop", OS: "darwin"})
	if err != nil {
		t.Fatalf("provision.Build(desktop) error = %v", err)
	}
	for _, step := range desktop.Steps {
		if step.Tool == "gentle-ai" {
			t.Fatalf("desktop profile selected gentle-ai provisioner args %#v; desktop must not install SDD or gentle-dev agent setup", step.Args)
		}
	}

	agents, err := provision.Build(*got, provision.Options{Profile: "agents", OS: "darwin"})
	if err != nil {
		t.Fatalf("provision.Build(agents) error = %v", err)
	}
	if len(agents.Steps) == 0 {
		t.Fatal("agents profile selected no provisioners, want gentle-ai setup there")
	}
	foundGentleAI := false
	for _, step := range agents.Steps {
		if step.Tool == "gentle-ai" {
			foundGentleAI = true
			break
		}
	}
	if !foundGentleAI {
		t.Fatal("agents profile did not select gentle-ai provisioners")
	}
	wantSDDArgs := []string{
		"install",
		"--scope", "global",
		"--channel", "stable",
		"--preset", "custom",
		"--sdd-mode", "multi",
		"--agents", "codex,claude-code,opencode,antigravity,vscode-copilot",
		"--components", "sdd",
	}
	for _, step := range agents.Steps {
		if step.Tool == "gentle-ai" && sameStrings(step.Args, wantSDDArgs) {
			t.Fatalf("agents profile selected SDD install args %#v; SDD must require --tag sdd", step.Args)
		}
	}

	agentsWithSDD, err := provision.Build(*got, provision.Options{Profile: "agents", ExtraTags: []string{"sdd"}, OS: "darwin"})
	if err != nil {
		t.Fatalf("provision.Build(agents --tag sdd) error = %v", err)
	}
	foundSDDInstall := false
	for _, step := range agentsWithSDD.Steps {
		if step.Tool == "gentle-ai" && sameStrings(step.Args, wantSDDArgs) {
			foundSDDInstall = true
			break
		}
	}
	if !foundSDDInstall {
		t.Fatal("agents profile with --tag sdd did not select the Gentle-AI SDD install provisioner")
	}

	wantPersonaArgs := []string{
		"install",
		"--scope", "global",
		"--channel", "stable",
		"--persona", "neutral",
		"--preset", "custom",
		"--agents", "codex,claude-code,opencode,antigravity,vscode-copilot",
		"--components", "persona",
	}
	for _, step := range agents.Steps {
		if step.Tool == "gentle-ai" && sameStrings(step.Args, wantPersonaArgs) {
			t.Fatalf("agents profile selected persona install args %#v; persona must require --tag persona", step.Args)
		}
	}

	agentsWithPersona, err := provision.Build(*got, provision.Options{Profile: "agents", ExtraTags: []string{"persona"}, OS: "darwin"})
	if err != nil {
		t.Fatalf("provision.Build(agents --tag persona) error = %v", err)
	}
	for _, step := range agentsWithPersona.Steps {
		if step.Tool == "gentle-ai" && sameStrings(step.Args, wantPersonaArgs) {
			t.Fatalf("agents profile with --tag persona selected persona install args %#v; repository manifest must not install persona", step.Args)
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
		if !hasString(prov.Tags, "web") {
			t.Errorf("claude provisioner %#v missing web tag", prov.Spec)
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
			want: `profiles["default"].dependencies[0].name is required`,
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
			want: "entries[0].ownership must be one of json-subset, toml-subset",
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
			want: "provisioners[0].tool must be one of claude, codegraph, codex, gentle-ai, skills, zimfw",
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
			name: "claude spec mixes gentle-ai flags",
			provisioner: `  - tool: claude
    tags: [core]
    spec:
      marketplace: ChromeDevTools/chrome-devtools-mcp
      persona: neutral
`,
			want: "provisioners[0].spec must not set gentle-ai install flags for the claude tool",
		},
		{
			name: "gentle-ai spec mixes claude fields",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      plugin: chrome-devtools-mcp
`,
			want: "provisioners[0].spec must not set claude fields (marketplace, plugin, from) for the gentle-ai tool",
		},
		{
			name: "gentle-ai spec mixes mcp fields",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      mcp: chrome-devtools
`,
			want: "provisioners[0].spec must not set MCP fields (mcp, command, env) for the gentle-ai tool",
		},
		{
			name: "gentle-ai spec mixes skills fields",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      package: vercel-labs/agent-skills
`,
			want: "provisioners[0].spec must not set skills.sh fields (package, global, copy) for the gentle-ai tool",
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
			name: "codex spec mixes gentle-ai flags",
			provisioner: `  - tool: codex
    tags: [core]
    spec:
      mcp: chrome-devtools
      command: [npx, chrome-devtools-mcp@latest]
      persona: neutral
`,
			want: "provisioners[0].spec must not set gentle-ai install flags for the codex tool",
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
			name: "zimfw spec requires yes",
			provisioner: `  - tool: zimfw
    tags: [core]
    spec:
      action: install
`,
			want: "provisioners[0].spec must not set gentle-ai fields (action, scope, channel, persona, preset, sdd-mode, agents, components, skills) for the zimfw tool",
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
			name: "skills spec mixes gentle-ai scalar fields",
			provisioner: `  - tool: skills
    tags: [core]
    spec:
      package: vercel-labs/agent-skills
      persona: neutral
`,
			want: "provisioners[0].spec must not set gentle-ai fields (action, scope, channel, persona, preset, sdd-mode, components, yes) for the skills tool",
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
			provisioner: `  - tool: gentle-ai
    tags: [core]
`,
			want: "provisioners[0].spec is required",
		},
		{
			name: "missing tags",
			provisioner: `  - tool: gentle-ai
    tags: []
    spec:
      scope: global
`,
			want: "provisioners[0].tags is required",
		},
		{
			name: "empty tag value",
			provisioner: `  - tool: gentle-ai
    tags: ["", core]
    spec:
      scope: global
`,
			want: "provisioners[0].tags[0] must not be empty",
		},
		{
			name: "unsupported os filter",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    os: [windows]
    spec:
      scope: global
`,
			want: "provisioners[0].os[0] must be one of darwin, linux",
		},
		{
			name: "unsupported persona",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      persona: senior-architect
`,
			want: "provisioners[0].spec.persona must be one of gentleman, neutral",
		},
		{
			name: "unsupported gentle-ai action",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      action: sync
`,
			want: "provisioners[0].spec.action must be one of install, uninstall",
		},
		{
			name: "yes without uninstall action",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      preset: custom
      yes: true
`,
			want: "provisioners[0].spec.yes is only valid when action is uninstall",
		},
		{
			name: "uninstall action rejects install-only fields",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      action: uninstall
      scope: global
      agents: [codex]
      components: [sdd]
      yes: true
`,
			want: "provisioners[0].spec uninstall action must not set install-only fields (scope, channel, persona, preset, sdd-mode, skills)",
		},
		{
			name: "whitespace-only agents list is missing spec",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      agents: ["  "]
`,
			want: "provisioners[0].spec is required",
		},
		{
			name: "empty agent value",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      agents: ["  "]
`,
			want: "provisioners[0].spec.agents[0] must not be empty",
		},
		{
			name: "empty component value",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      components: ["  "]
`,
			want: "provisioners[0].spec.components[0] must not be empty",
		},
		{
			name: "empty skill value",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
      skills: ["  "]
`,
			want: "provisioners[0].spec.skills[0] must not be empty",
		},
		{
			name: "dependency without name",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
    dependencies:
      - brew: gentleman-programming/tap/gentle-ai
`,
			want: "provisioners[0].dependencies[0].name is required",
		},
		{
			name: "dependency with whitespace-only manual guidance",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
    dependencies:
      - name: ghostty
        manual: "  "
`,
			want: "provisioners[0].dependencies[0].manual must not be empty",
		},
		{
			name: "dependency with whitespace-only Debian manual guidance",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
    dependencies:
      - name: ghostty
        manual_debian: "  "
`,
			want: "provisioners[0].dependencies[0].manual_debian must not be empty",
		},
		{
			name: "dependency with whitespace-only brew cask",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
    dependencies:
      - name: CascadiaCode Nerd Font
        brew_cask: "  "
        font_match: "CascadiaCodeNF*"
`,
			want: "provisioners[0].dependencies[0].brew_cask must not be empty",
		},
		{
			name: "dependency with ambiguous homebrew formula and cask",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
    dependencies:
      - name: CascadiaCode Nerd Font
        brew: cascadia-code
        brew_cask: font-cascadia-code-nf
`,
			want: "provisioners[0].dependencies[0] must not set both brew and brew_cask",
		},
		{
			name: "dependency with empty font fallback match",
			provisioner: `  - tool: gentle-ai
    tags: [core]
    spec:
      scope: global
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
		dep      string
	}{
		{name: "zsh", target: "~/.zshrc", source: "configs/zsh/zshrc", strategy: "symlink", dep: "zsh"},
		{name: "git", target: "~/.gitconfig", source: "configs/git/gitconfig", strategy: "symlink", dep: "git"},
		{name: "starship", target: "~/.config/starship.toml", source: "configs/starship/starship.toml", strategy: "symlink", dep: "starship"},
		{name: "tmux", target: "~/.tmux.conf", source: "configs/tmux/tmux.conf", strategy: "symlink", dep: "tmux"},
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
			if !hasString(entry.Tags, "core") {
				t.Errorf("Tags = %#v, want core tag", entry.Tags)
			}
			if !sameStrings(entry.OS, []string{"darwin", "linux"}) {
				t.Errorf("OS = %#v, want [darwin linux]", entry.OS)
			}
			if !hasDependency(entry.Dependencies, tt.dep) {
				t.Errorf("Dependencies = %#v, want %q", entry.Dependencies, tt.dep)
			}
		})
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
			path: "configs/git/gitconfig",
			contains: []string{
				"Machine-specific identity belongs in ~/.gitconfig.local",
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
		"path = ~/.gitconfig.local",
	} {
		if !strings.Contains(managed, want) {
			t.Fatalf("managed gitconfig missing portable/local boundary %q:\n%s", want, managed)
		}
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
			if entry.Strategy != "symlink" {
				t.Errorf("Strategy = %q, want symlink", entry.Strategy)
			}
			if !hasString(entry.Tags, "core") {
				t.Errorf("Tags = %#v, want core tag", entry.Tags)
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

	p, err := plan.Build(*got, plan.Options{
		Profile:    "core",
		OS:         "darwin",
		SourceRoot: root,
		Home:       t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Derive the expected action count from the loaded manifest instead of a
	// hardcoded literal: every managed entry whose tags intersect the profile
	// and that passes the OS filter must produce exactly one action. This is the
	// same selection plan.Build applies, so adding or removing a dots.yaml entry
	// never forces a magic-number bump here.
	profile, ok := got.Profiles["core"]
	if !ok {
		t.Fatal("manifest missing profile \"default\"")
	}
	wantActions := 0
	for _, entry := range got.Entries {
		if manifest.SharesTag(entry.Tags, profile.Tags) && manifest.MatchesOS(entry.OS, "darwin") {
			wantActions++
		}
	}
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

	entry, ok := entriesByTarget["~/.config/nvim"]
	if !ok {
		t.Fatal("repository manifest missing Neovim entry for target ~/.config/nvim")
	}
	if entry.Source != "configs/nvim" {
		t.Errorf("Source = %q, want %q", entry.Source, "configs/nvim")
	}
	if entry.Strategy != "symlink" {
		t.Errorf("Strategy = %q, want symlink", entry.Strategy)
	}
	if !hasString(entry.Tags, "core") {
		t.Errorf("Tags = %#v, want core tag", entry.Tags)
	}
	if !sameStrings(entry.OS, []string{"darwin", "linux"}) {
		t.Errorf("OS = %#v, want [darwin linux]", entry.OS)
	}
	if !hasDependency(entry.Dependencies, "neovim") {
		t.Errorf("Dependencies = %#v, want neovim dependency", entry.Dependencies)
	}

	// The source directory must exist in the repository.
	sourcePath := filepath.Join(root, entry.Source)
	info, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatalf("source %q does not exist: %v", sourcePath, err)
	}
	if !info.IsDir() {
		t.Fatalf("source %q is not a directory, want directory for symlink entry", sourcePath)
	}
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

	var core *manifest.DependencySet
	for i := range got.Dependencies {
		candidate := &got.Dependencies[i]
		if hasString(candidate.Tags, "core") {
			core = candidate
			break
		}
	}
	if core == nil {
		t.Fatal("repository manifest missing core dependency set")
	}
	if !sameStrings(core.OS, []string{"darwin", "linux"}) {
		t.Fatalf("core dependency set OS = %#v, want [darwin linux]", core.OS)
	}

	want := map[string]struct {
		command   string
		commands  []string
		brew      string
		apt       string
		dnf       string
		pacman    string
		toolchain string
		linuxBrew bool
	}{
		"Node LTS (fnm)":       {commands: []string{"fnm", "node"}, brew: "fnm", toolchain: manifest.DependencyToolchainNodeLTSFNM, linuxBrew: true},
		"Rust stable (rustup)": {commands: []string{"rustup", "rustc", "cargo"}, brew: "rustup", toolchain: manifest.DependencyToolchainRustStableRustup, linuxBrew: true},
		"go":                   {command: "go", brew: "go", linuxBrew: true},
		"uv":                   {command: "uv", brew: "uv", linuxBrew: true},
		"pnpm":                 {command: "pnpm", brew: "pnpm", linuxBrew: true},
		"bun":                  {command: "bun", brew: "bun", linuxBrew: true},
		"fzf":                  {command: "fzf", brew: "fzf", apt: "fzf", dnf: "fzf", pacman: "fzf"},
		"zoxide":               {command: "zoxide", brew: "zoxide", apt: "zoxide", dnf: "zoxide", pacman: "zoxide"},
		"lazygit":              {command: "lazygit", brew: "lazygit", dnf: "lazygit", pacman: "lazygit", linuxBrew: true},
		"eza":                  {command: "eza", brew: "eza", apt: "eza", dnf: "eza", pacman: "eza"},
		"ripgrep":              {command: "rg", brew: "ripgrep", apt: "ripgrep", dnf: "ripgrep", pacman: "ripgrep"},
		"delta":                {command: "delta", brew: "git-delta", dnf: "git-delta", pacman: "git-delta", linuxBrew: true},
		"unzip":                {command: "unzip", brew: "unzip", apt: "unzip", dnf: "unzip", pacman: "unzip"},
		"fd":                   {command: "fd", brew: "fd", dnf: "fd-find", pacman: "fd", linuxBrew: true},
	}
	for name, wantDep := range want {
		var dep *manifest.Dependency
		for i := range core.Dependencies {
			candidate := &core.Dependencies[i]
			if candidate.Name == name {
				dep = candidate
				break
			}
		}
		if dep == nil {
			t.Fatalf("core dependency set missing %q: %#v", name, core.Dependencies)
		}
		if dep.Command != wantDep.command || dep.Brew != wantDep.brew || dep.Apt != wantDep.apt || dep.Dnf != wantDep.dnf || dep.Pacman != wantDep.pacman || dep.Toolchain != wantDep.toolchain || dep.LinuxHomebrew != wantDep.linuxBrew || !sameStrings(dep.Commands, wantDep.commands) {
			t.Fatalf("%s dependency = %#v, want command %q, commands %#v, brew %q, apt %q, dnf %q, pacman %q, toolchain %q, linux_homebrew %v", name, *dep, wantDep.command, wantDep.commands, wantDep.brew, wantDep.apt, wantDep.dnf, wantDep.pacman, wantDep.toolchain, wantDep.linuxBrew)
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
	adaptiveText := string(adaptiveConfig)
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
