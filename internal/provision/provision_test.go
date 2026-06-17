package provision_test

import (
	"reflect"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
)

func TestRenderCommand(t *testing.T) {
	tests := []struct {
		name     string
		prov     manifest.Provisioner
		wantExec string
		wantArgs []string
	}{
		{
			name: "full spec renders every flag in deterministic order",
			prov: manifest.Provisioner{
				Tool: "gentle-ai",
				Spec: manifest.ProvisionerSpec{
					Scope:      "global",
					Channel:    "stable",
					Persona:    "neutral",
					SDDMode:    "strict",
					Agents:     []string{"codex", "claude"},
					Components: []string{"engram", "sdd"},
					Skills:     []string{"tdd", "go-testing"},
				},
			},
			wantExec: "gentle-ai",
			wantArgs: []string{
				"install",
				"--scope", "global",
				"--channel", "stable",
				"--persona", "neutral",
				"--sdd-mode", "strict",
				"--agents", "codex,claude",
				"--components", "engram,sdd",
				"--skills", "tdd,go-testing",
			},
		},
		{
			name: "partial spec omits unset flags",
			prov: manifest.Provisioner{
				Tool: "gentle-ai",
				Spec: manifest.ProvisionerSpec{
					Scope:  "global",
					Agents: []string{"codex"},
				},
			},
			wantExec: "gentle-ai",
			wantArgs: []string{"install", "--scope", "global", "--agents", "codex"},
		},
		{
			name: "persona only",
			prov: manifest.Provisioner{
				Tool: "gentle-ai",
				Spec: manifest.ProvisionerSpec{Persona: "gentleman"},
			},
			wantExec: "gentle-ai",
			wantArgs: []string{"install", "--persona", "gentleman"},
		},
		{
			name: "whitespace scalars and list entries are trimmed and dropped",
			prov: manifest.Provisioner{
				Tool: "gentle-ai",
				Spec: manifest.ProvisionerSpec{
					Scope:   "  global  ",
					Channel: "   ",
					Agents:  []string{" codex ", "", "  ", "claude"},
				},
			},
			wantExec: "gentle-ai",
			wantArgs: []string{"install", "--scope", "global", "--agents", "codex,claude"},
		},
		{
			name: "claude marketplace registration",
			prov: manifest.Provisioner{
				Tool: "claude",
				Spec: manifest.ProvisionerSpec{Marketplace: "ChromeDevTools/chrome-devtools-mcp"},
			},
			wantExec: "claude",
			wantArgs: []string{"plugin", "marketplace", "add", "ChromeDevTools/chrome-devtools-mcp"},
		},
		{
			name: "claude plugin install into user scope",
			prov: manifest.Provisioner{
				Tool: "claude",
				Spec: manifest.ProvisionerSpec{Plugin: "chrome-devtools-mcp", From: "chrome-devtools-plugins"},
			},
			wantExec: "claude",
			wantArgs: []string{"plugin", "install", "chrome-devtools-mcp@chrome-devtools-plugins", "--scope", "user"},
		},
		{
			name: "claude trims whitespace around marketplace source",
			prov: manifest.Provisioner{
				Tool: "claude",
				Spec: manifest.ProvisionerSpec{Marketplace: "  ChromeDevTools/chrome-devtools-mcp  "},
			},
			wantExec: "claude",
			wantArgs: []string{"plugin", "marketplace", "add", "ChromeDevTools/chrome-devtools-mcp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExec, gotArgs := provision.RenderCommand(tt.prov)
			if gotExec != tt.wantExec {
				t.Fatalf("RenderCommand() executable = %q, want %q", gotExec, tt.wantExec)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Fatalf("RenderCommand() args = %#v, want %#v", gotArgs, tt.wantArgs)
			}
		})
	}
}
