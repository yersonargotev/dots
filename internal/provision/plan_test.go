package provision_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
)

func manifestWithProvisioners(provs ...manifest.Provisioner) manifest.Manifest {
	return manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {Tags: []string{"core"}},
			"desktop": {Tags: []string{"core", "desktop"}},
		},
		Entries: []manifest.Entry{{
			Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"},
		}},
		Provisioners: provs,
	}
}

func TestSelect(t *testing.T) {
	coreProv := manifest.Provisioner{
		Tool: "zimfw", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Yes: true},
	}
	desktopProv := manifest.Provisioner{
		Tool: "claude", Tags: []string{"desktop"},
		Spec: manifest.ProvisionerSpec{Marketplace: "example/tools"},
	}
	linuxOnlyProv := manifest.Provisioner{
		Tool: "zimfw", Tags: []string{"core"}, OS: []string{"linux"},
		Spec: manifest.ProvisionerSpec{Yes: true},
	}

	tests := []struct {
		name    string
		m       manifest.Manifest
		opts    provision.Options
		want    []manifest.Provisioner
		wantErr bool
	}{
		{
			name: "selects provisioner sharing a profile tag",
			m:    manifestWithProvisioners(coreProv, desktopProv),
			opts: provision.Options{Profile: "default", OS: "darwin"},
			want: []manifest.Provisioner{coreProv},
		},
		{
			name: "desktop profile selects both shared tags in manifest order",
			m:    manifestWithProvisioners(coreProv, desktopProv),
			opts: provision.Options{Profile: "desktop", OS: "darwin"},
			want: []manifest.Provisioner{coreProv, desktopProv},
		},
		{
			name: "skips provisioner excluded by OS filter",
			m:    manifestWithProvisioners(linuxOnlyProv),
			opts: provision.Options{Profile: "default", OS: "darwin"},
			want: nil,
		},
		{
			name: "includes provisioner with OS filter when it matches",
			m:    manifestWithProvisioners(linuxOnlyProv),
			opts: provision.Options{Profile: "default", OS: "linux"},
			want: []manifest.Provisioner{linuxOnlyProv},
		},
		{
			name:    "unknown profile is an error",
			m:       manifestWithProvisioners(coreProv),
			opts:    provision.Options{Profile: "ghost", OS: "darwin"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := provision.Select(tt.m, tt.opts)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Select() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Select() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPlanResolvesSelectedProvisioners(t *testing.T) {
	prov := manifest.Provisioner{
		Tool: "claude", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Marketplace: "example/tools"},
	}
	m := manifestWithProvisioners(prov)

	p, err := provision.Build(m, provision.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if p.Profile != "default" {
		t.Fatalf("Plan.Profile = %q, want default", p.Profile)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("len(Plan.Steps) = %d, want 1", len(p.Steps))
	}
	step := p.Steps[0]
	if step.Tool != "claude" || step.Executable != "claude" {
		t.Fatalf("Step tool/executable = %q/%q, want claude", step.Tool, step.Executable)
	}
	wantArgs := []string{"plugin", "marketplace", "add", "example/tools"}
	if !reflect.DeepEqual(step.Args, wantArgs) {
		t.Fatalf("Step.Args = %#v, want %#v", step.Args, wantArgs)
	}
	if !reflect.DeepEqual(step.Targets, []string{"~/.claude", "~/.claude.json"}) {
		t.Fatalf("Step.Targets = %#v, want [~/.claude ~/.claude.json]", step.Targets)
	}
}

func TestBuildProfileAndEquivalentExplicitTagsProduceSameSteps(t *testing.T) {
	core := manifest.Provisioner{Tool: "zimfw", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Yes: true}}
	desktop := manifest.Provisioner{Tool: "claude", Tags: []string{"desktop"}, OS: []string{"darwin"}, Spec: manifest.ProvisionerSpec{Marketplace: "example/tools"}}
	linux := manifest.Provisioner{Tool: "codex", Tags: []string{"desktop"}, OS: []string{"linux"}, Spec: manifest.ProvisionerSpec{MCP: "example", Command: []string{"npx"}}}
	m := manifestWithProvisioners(core, desktop, linux)
	m.Profiles["workstation"] = manifest.Profile{Tags: []string{"core", "desktop"}}
	profileSelection := manifest.Selection{Profile: "workstation", Profiles: []string{"workstation"}, Tags: []string{"core", "desktop"}}
	explicitTagSelection := manifest.Selection{Tags: []string{"core", "desktop"}}

	fromProfile, err := provision.Build(m, provision.Options{Selection: &profileSelection, OS: "darwin"})
	if err != nil {
		t.Fatalf("Build() from Profile error = %v", err)
	}
	fromTags, err := provision.Build(m, provision.Options{Selection: &explicitTagSelection, OS: "darwin"})
	if err != nil {
		t.Fatalf("Build() from explicit Tags error = %v", err)
	}
	if !reflect.DeepEqual(fromProfile.Steps, fromTags.Steps) {
		t.Fatalf("Profile steps = %#v, explicit Tag steps = %#v", fromProfile.Steps, fromTags.Steps)
	}
}

func TestPlanResolvesClaudeProvisioner(t *testing.T) {
	market := manifest.Provisioner{
		Tool: "claude", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Marketplace: "ChromeDevTools/chrome-devtools-mcp"},
	}
	plugin := manifest.Provisioner{
		Tool: "claude", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Plugin: "chrome-devtools-mcp", From: "chrome-devtools-plugins"},
	}
	m := manifestWithProvisioners(market, plugin)

	p, err := provision.Build(m, provision.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Steps) != 2 {
		t.Fatalf("len(Plan.Steps) = %d, want 2", len(p.Steps))
	}

	marketStep := p.Steps[0]
	if marketStep.Executable != "claude" {
		t.Fatalf("market step executable = %q, want claude", marketStep.Executable)
	}
	if !reflect.DeepEqual(marketStep.Args, []string{"plugin", "marketplace", "add", "ChromeDevTools/chrome-devtools-mcp"}) {
		t.Fatalf("market step args = %#v", marketStep.Args)
	}
	if !reflect.DeepEqual(marketStep.Targets, []string{"~/.claude", "~/.claude.json"}) {
		t.Fatalf("market step targets = %#v, want [~/.claude ~/.claude.json]", marketStep.Targets)
	}

	pluginStep := p.Steps[1]
	if !reflect.DeepEqual(pluginStep.Args, []string{"plugin", "install", "chrome-devtools-mcp@chrome-devtools-plugins", "--scope", "user"}) {
		t.Fatalf("plugin step args = %#v", pluginStep.Args)
	}
}

func TestPlanResolvesCodexProvisioner(t *testing.T) {
	codex := manifest.Provisioner{
		Tool: "codex", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{
			MCP:     "chrome-devtools",
			Command: []string{"npx", "-y", "chrome-devtools-mcp@latest"},
			Env:     map[string]string{"CHROME_DEVTOOLS_MCP_NO_UPDATE_CHECKS": "1"},
		},
	}
	m := manifestWithProvisioners(codex)

	p, err := provision.Build(m, provision.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("len(Plan.Steps) = %d, want 1", len(p.Steps))
	}

	step := p.Steps[0]
	if step.Executable != "codex" {
		t.Fatalf("codex step executable = %q, want codex", step.Executable)
	}
	wantArgs := []string{
		"mcp", "add", "chrome-devtools",
		"--env", "CHROME_DEVTOOLS_MCP_NO_UPDATE_CHECKS=1",
		"--", "npx", "-y", "chrome-devtools-mcp@latest",
	}
	if !reflect.DeepEqual(step.Args, wantArgs) {
		t.Fatalf("codex step args = %#v, want %#v", step.Args, wantArgs)
	}
	if !reflect.DeepEqual(step.Targets, []string{"~/.codex"}) {
		t.Fatalf("codex step targets = %#v, want [~/.codex]", step.Targets)
	}
}

func TestPlanResolvesCodeGraphProvisioner(t *testing.T) {
	codegraph := manifest.Provisioner{
		Tool: "codegraph", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{
			Scope:  "global",
			Agents: []string{"codex", "claude", "antigravity", "opencode"},
			Yes:    true,
		},
	}
	m := manifestWithProvisioners(codegraph)

	p, err := provision.Build(m, provision.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("len(Plan.Steps) = %d, want 1", len(p.Steps))
	}

	step := p.Steps[0]
	if step.Executable != "sh" {
		t.Fatalf("codegraph step executable = %q, want sh", step.Executable)
	}
	wantArgs := []string{
		"-c",
		"set -eu\nif ! command -v codegraph >/dev/null 2>&1; then\n\tcurl -fsSL https://raw.githubusercontent.com/colbymchenry/codegraph/main/install.sh | sh\nfi\nexport PATH=\"$HOME/.local/bin:$PATH\"\ncodegraph install --target \"$1\" --location \"$2\" --yes",
		"codegraph-install",
		"codex,claude,antigravity,opencode",
		"global",
	}
	if !reflect.DeepEqual(step.Args, wantArgs) {
		t.Fatalf("codegraph args = %#v, want %#v", step.Args, wantArgs)
	}
	wantTargets := []string{"~/.codegraph", "~/.local/bin", "~/.codex", "~/.claude", "~/.claude.json", "~/.gemini", "~/.config/opencode"}
	if !reflect.DeepEqual(step.Targets, wantTargets) {
		t.Fatalf("codegraph targets = %#v, want %#v", step.Targets, wantTargets)
	}
	if !reflect.DeepEqual(step.GlobalTools, []string{"codegraph (~/.local/bin)"}) {
		t.Fatalf("codegraph global tools = %#v, want local bin disclosure", step.GlobalTools)
	}
}

func TestPlanResolvesZimFWProvisioner(t *testing.T) {
	zimfw := manifest.Provisioner{
		Tool: "zimfw", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Yes: true},
	}
	m := manifestWithProvisioners(zimfw)

	p, err := provision.Build(m, provision.Options{Profile: "default", OS: "linux"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("len(Plan.Steps) = %d, want 1", len(p.Steps))
	}

	step := p.Steps[0]
	if step.Tool != "zimfw" || step.Executable != "zsh" {
		t.Fatalf("zimfw step tool/executable = %q/%q, want zimfw/zsh", step.Tool, step.Executable)
	}
	if !reflect.DeepEqual(step.Targets, []string{"~/.zim"}) {
		t.Fatalf("zimfw targets = %#v, want [~/.zim]", step.Targets)
	}
}

func TestPlanResolvesSkillsProvisioner(t *testing.T) {
	skills := manifest.Provisioner{
		Tool: "skills", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{
			Package: "vercel-labs/agent-skills",
			Agents:  []string{"codex", "claude-code", "antigravity", "opencode", "github-copilot"},
			Skills:  []string{"web-design-guidelines"},
			Global:  true,
		},
	}
	m := manifestWithProvisioners(skills)

	p, err := provision.Build(m, provision.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(p.Steps) != 1 {
		t.Fatalf("len(Plan.Steps) = %d, want 1", len(p.Steps))
	}

	step := p.Steps[0]
	if step.Tool != "skills" || step.Executable != "npx" {
		t.Fatalf("skills step tool/executable = %q/%q, want skills/npx", step.Tool, step.Executable)
	}
	wantArgs := []string{
		"--yes", "skills@1.5.12", "add", "vercel-labs/agent-skills",
		"--agent", "codex",
		"--agent", "claude-code",
		"--agent", "antigravity",
		"--agent", "opencode",
		"--agent", "github-copilot",
		"--skill", "web-design-guidelines",
		"--global",
	}
	if !reflect.DeepEqual(step.Args, wantArgs) {
		t.Fatalf("skills step args = %#v, want %#v", step.Args, wantArgs)
	}
	wantTargets := []string{"~/.agents/skills", "~/.claude/skills"}
	if !reflect.DeepEqual(step.Targets, wantTargets) {
		t.Fatalf("skills step targets = %#v, want %#v", step.Targets, wantTargets)
	}
}

func TestPlanEmptyWhenNoProvisionerSelected(t *testing.T) {
	prov := manifest.Provisioner{
		Tool: "claude", Tags: []string{"desktop"},
		Spec: manifest.ProvisionerSpec{Marketplace: "example/tools"},
	}
	m := manifestWithProvisioners(prov)

	p, err := provision.Build(m, provision.Options{Profile: "default", OS: "darwin"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if p.Profile != "default" {
		t.Fatalf("Plan.Profile = %q, want default", p.Profile)
	}
	if len(p.Steps) != 0 {
		t.Fatalf("len(Plan.Steps) = %d, want 0", len(p.Steps))
	}
}
