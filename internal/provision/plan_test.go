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
		Tool: "gentle-ai", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Scope: "global"},
	}
	desktopProv := manifest.Provisioner{
		Tool: "gentle-ai", Tags: []string{"desktop"},
		Spec: manifest.ProvisionerSpec{Scope: "global", Agents: []string{"codex"}},
	}
	linuxOnlyProv := manifest.Provisioner{
		Tool: "gentle-ai", Tags: []string{"core"}, OS: []string{"linux"},
		Spec: manifest.ProvisionerSpec{Persona: "neutral"},
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
		Tool: "gentle-ai", Tags: []string{"core"},
		Spec: manifest.ProvisionerSpec{Scope: "global", Agents: []string{"codex", "opencode"}},
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
	if step.Tool != "gentle-ai" || step.Executable != "gentle-ai" {
		t.Fatalf("Step tool/executable = %q/%q, want gentle-ai", step.Tool, step.Executable)
	}
	wantArgs := []string{"install", "--scope", "global", "--agents", "codex,opencode"}
	if !reflect.DeepEqual(step.Args, wantArgs) {
		t.Fatalf("Step.Args = %#v, want %#v", step.Args, wantArgs)
	}
	if !reflect.DeepEqual(step.Targets, []string{"~/.claude", "~/.codex", "~/.config/opencode", "~/.gentle-ai"}) {
		t.Fatalf("Step.Targets = %#v, want [~/.claude ~/.codex ~/.config/opencode ~/.gentle-ai]", step.Targets)
	}
}

func TestPlanEmptyWhenNoProvisionerSelected(t *testing.T) {
	prov := manifest.Provisioner{
		Tool: "gentle-ai", Tags: []string{"desktop"},
		Spec: manifest.ProvisionerSpec{Scope: "global"},
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
