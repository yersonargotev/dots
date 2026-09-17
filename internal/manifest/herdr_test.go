package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

const herdrTestCommit = "c696c36256eddc6ee1983ab9f202848b84460e06"

func manifestWithHerdrSpec(spec manifest.ProvisionerSpec) manifest.Manifest {
	return manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {Tags: []string{"herdr"}},
		},
		Entries: []manifest.Entry{{
			Source: "configs/herdr/config.toml", Target: "~/.config/herdr/config.toml",
			Strategy: "copy", Tags: []string{"herdr"},
		}},
		Provisioners: []manifest.Provisioner{{
			Tool: "herdr", Tags: []string{"herdr"}, OS: []string{"darwin"}, Spec: spec,
		}},
	}
}

func TestParseAcceptsPinnedHerdrPlugin(t *testing.T) {
	data := []byte(`version: 1
profiles:
  default:
    tags: [herdr]
entries:
  - source: configs/herdr/config.toml
    target: ~/.config/herdr/config.toml
    strategy: copy
    tags: [herdr]
provisioners:
  - tool: herdr
    tags: [herdr]
    os: [darwin]
    arch: [arm64]
    spec:
      plugin: szrenwei/herdr-space-tab-metadata
      ref: c696c36256eddc6ee1983ab9f202848b84460e06
`)

	got, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(got.Provisioners) != 1 || got.Provisioners[0].Spec.Ref != herdrTestCommit || len(got.Provisioners[0].Arch) != 1 || got.Provisioners[0].Arch[0] != "arm64" {
		t.Fatalf("parsed Herdr provisioner = %#v", got.Provisioners)
	}
}

func TestProvisionerArchitectureFilterValidation(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run("accepts "+arch, func(t *testing.T) {
			m := manifestWithHerdrSpec(manifest.ProvisionerSpec{Plugin: "owner/repo", Ref: herdrTestCommit})
			m.Provisioners[0].Arch = []string{arch}
			if err := m.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
	for _, arch := range []string{"x86_64", "aarch64", "ARM64", ""} {
		t.Run("rejects "+arch, func(t *testing.T) {
			m := manifestWithHerdrSpec(manifest.ProvisionerSpec{Plugin: "owner/repo", Ref: herdrTestCommit})
			m.Provisioners[0].Arch = []string{arch}
			err := m.Validate()
			if err == nil || !strings.Contains(err.Error(), ".arch[0] must be one of amd64, arm64") {
				t.Fatalf("Validate() error = %v, want architecture rejection", err)
			}
		})
	}
}

func TestLoadPreviousFilePreservesAndValidatesProvisionerArchitecture(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dots.yaml")
	data := []byte(`version: 1
profiles:
  default:
    tags: [herdr]
entries:
  - source: configs/herdr/config.toml
    target: ~/.config/herdr/config.toml
    strategy: copy
    tags: [herdr]
provisioners:
  - tool: retired-tool
    tags: [herdr]
    os: [darwin]
    arch: [arm64]
    spec:
      retired_field: ignored
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write previous manifest: %v", err)
	}

	previous, err := manifest.LoadPreviousFile(path)
	if err != nil {
		t.Fatalf("LoadPreviousFile() error = %v", err)
	}
	if len(previous.Provisioners) != 1 || !manifest.MatchesArch(previous.Provisioners[0].Arch, "arm64") || manifest.MatchesArch(previous.Provisioners[0].Arch, "amd64") {
		t.Fatalf("previous Provisioners = %#v, want preserved arm64 filter", previous.Provisioners)
	}

	invalid := strings.Replace(string(data), "arch: [arm64]", "arch: [x86_64]", 1)
	if err := os.WriteFile(path, []byte(invalid), 0o600); err != nil {
		t.Fatalf("rewrite previous manifest: %v", err)
	}
	if _, err := manifest.LoadPreviousFile(path); err == nil || !strings.Contains(err.Error(), ".arch[0] must be one of amd64, arm64") {
		t.Fatalf("LoadPreviousFile() error = %v, want architecture rejection", err)
	}
}

func TestHerdrProvisionerRejectsInvalidPluginAndRef(t *testing.T) {
	tests := []struct {
		name string
		spec manifest.ProvisionerSpec
		want string
	}{
		{name: "missing plugin", spec: manifest.ProvisionerSpec{Ref: herdrTestCommit}, want: ".spec.plugin is required"},
		{name: "extra path", spec: manifest.ProvisionerSpec{Plugin: "owner/repo/subdir", Ref: herdrTestCommit}, want: "owner/repo reference"},
		{name: "URL", spec: manifest.ProvisionerSpec{Plugin: "https://github.com/owner/repo", Ref: herdrTestCommit}, want: "owner/repo reference"},
		{name: "flag shaped", spec: manifest.ProvisionerSpec{Plugin: "--help/repo", Ref: herdrTestCommit}, want: "owner/repo reference"},
		{name: "embedded whitespace", spec: manifest.ProvisionerSpec{Plugin: "owner name/repo", Ref: herdrTestCommit}, want: "owner/repo reference"},
		{name: "missing ref", spec: manifest.ProvisionerSpec{Plugin: "owner/repo"}, want: "full 40-character hexadecimal commit"},
		{name: "branch ref", spec: manifest.ProvisionerSpec{Plugin: "owner/repo", Ref: "main"}, want: "full 40-character hexadecimal commit"},
		{name: "short commit", spec: manifest.ProvisionerSpec{Plugin: "owner/repo", Ref: strings.Repeat("a", 39)}, want: "full 40-character hexadecimal commit"},
		{name: "non hexadecimal commit", spec: manifest.ProvisionerSpec{Plugin: "owner/repo", Ref: strings.Repeat("g", 40)}, want: "full 40-character hexadecimal commit"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manifestWithHerdrSpec(tt.spec).Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestHerdrProvisionerRejectsOtherDialectFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*manifest.ProvisionerSpec)
	}{
		{name: "scope", mutate: func(s *manifest.ProvisionerSpec) { s.Scope = "global" }},
		{name: "agents", mutate: func(s *manifest.ProvisionerSpec) { s.Agents = []string{"codex"} }},
		{name: "skills", mutate: func(s *manifest.ProvisionerSpec) { s.Skills = []string{"example"} }},
		{name: "yes", mutate: func(s *manifest.ProvisionerSpec) { s.Yes = true }},
		{name: "marketplace", mutate: func(s *manifest.ProvisionerSpec) { s.Marketplace = "owner/marketplace" }},
		{name: "from", mutate: func(s *manifest.ProvisionerSpec) { s.From = "marketplace" }},
		{name: "mcp", mutate: func(s *manifest.ProvisionerSpec) { s.MCP = "example" }},
		{name: "command", mutate: func(s *manifest.ProvisionerSpec) { s.Command = []string{"example"} }},
		{name: "env", mutate: func(s *manifest.ProvisionerSpec) { s.Env = map[string]string{"KEY": "value"} }},
		{name: "package", mutate: func(s *manifest.ProvisionerSpec) { s.Package = "owner/package" }},
		{name: "global", mutate: func(s *manifest.ProvisionerSpec) { s.Global = true }},
		{name: "copy", mutate: func(s *manifest.ProvisionerSpec) { s.Copy = true }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := manifest.ProvisionerSpec{Plugin: "owner/repo", Ref: herdrTestCommit}
			tt.mutate(&spec)
			if err := manifestWithHerdrSpec(spec).Validate(); err == nil {
				t.Fatal("Validate() error = nil, want cross-dialect rejection")
			}
		})
	}
}

func TestOtherProvisionerDialectsRejectHerdrRef(t *testing.T) {
	provisioners := []manifest.Provisioner{
		{Tool: "claude", Spec: manifest.ProvisionerSpec{Marketplace: "owner/repo", Ref: herdrTestCommit}},
		{Tool: "codex", Spec: manifest.ProvisionerSpec{MCP: "server", Command: []string{"server"}, Ref: herdrTestCommit}},
		{Tool: "codegraph", Spec: manifest.ProvisionerSpec{Agents: []string{"codex"}, Yes: true, Ref: herdrTestCommit}},
		{Tool: "skills", Spec: manifest.ProvisionerSpec{Package: "owner/repo", Global: true, Ref: herdrTestCommit}},
		{Tool: "zimfw", Spec: manifest.ProvisionerSpec{Yes: true, Ref: herdrTestCommit}},
	}

	for _, provisioner := range provisioners {
		t.Run(provisioner.Tool, func(t *testing.T) {
			m := manifestWithHerdrSpec(provisioner.Spec)
			provisioner.Tags = []string{"herdr"}
			m.Provisioners[0] = provisioner
			err := m.Validate()
			if err == nil || !strings.Contains(err.Error(), ".spec.ref is only valid for the herdr tool") {
				t.Fatalf("Validate() error = %v, want Herdr ref rejection", err)
			}
		})
	}
}
