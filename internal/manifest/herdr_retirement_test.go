package manifest_test

import (
	"errors"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHerdrRetirementRejectsNamesWithoutNormalization(t *testing.T) {
	for _, registry := range []map[string]manifest.Tag{nil, {}, {"herdr-custom": {Status: "legacy", ReplacedBy: []string{"herdr"}}, "herdr": {Status: "current"}}} {
		m := manifest.Manifest{Tags: registry, Profiles: map[string]manifest.Profile{"old": {Tags: []string{"herdr-custom"}}}}
		if _, _, err := manifest.NormalizeTags(m, []string{"herdr-custom"}); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
			t.Fatal(err)
		}
		if _, err := manifest.ResolveSelection(m, nil, []string{"herdr-custom"}); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
			t.Fatal(err)
		}
		if _, err := manifest.ResolveReadOnlySelection(m, []string{"old"}, nil); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
			t.Fatal(err)
		}
		if err := manifest.ValidateSelection(m, manifest.Selection{Tags: []string{"herdr-custom"}}, "linux", "arm64"); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
			t.Fatal(err)
		}
	}
}

func TestHerdrRetirementKeepsHistoricalManifestReadable(t *testing.T) {
	const old = `version: 1
tags:
  herdr-custom: {kind: surface, status: current}
profiles:
  old: {tags: [herdr-custom]}
entries:
  - source: configs/herdr-custom/config.toml
    target: ~/.config/herdr/config.toml
    strategy: copy
    ownership: toml-subset
    tags: [herdr-custom]
    dependencies:
      - name: herdr-custom
        command: herdr-custom
        user_local:
          recipe: herdr-custom
          version: v0.9.5-custom.1
          checksums:
            darwin_arm64: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
            linux_amd64: bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
provisioners:
  - tool: herdr-custom
    tags: [herdr-custom]
    spec:
      plugin: yersonargotev/herdr-tab-git
      ref: 21b2baa278f571fafa7a1247851a17754fbeedf0
`
	path := filepath.Join(t.TempDir(), "dots.yaml")
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.LoadFile(path); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
		t.Fatalf("current: %v", err)
	}
	previous, err := manifest.LoadPreviousFile(path)
	if err != nil || len(previous.Provisioners) != 1 || len(previous.Entries[0].Dependencies) != 1 {
		t.Fatalf("historical: %+v %v", previous, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != old {
		t.Fatalf("history changed: %s %v", got, err)
	}
}

func TestRepositoryHerdrRetirementPreservesOfficialSurface(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join(repositoryRoot(t), "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Tags["herdr-custom"]; ok {
		t.Fatal("custom still offered")
	}
	for _, e := range m.Entries {
		for _, tag := range e.Tags {
			if tag == "herdr-custom" {
				t.Fatal("custom entry remains")
			}
		}
	}
	for _, platform := range []struct {
		os, arch string
		plugins  int
	}{{"darwin", "arm64", 3}, {"darwin", "amd64", 1}, {"linux", "amd64", 0}, {"linux", "arm64", 0}} {
		s := selectedsurface.EvaluateForPlatform(*m, []string{"herdr"}, platform.os, platform.arch)
		if len(s.Provisioners) != platform.plugins {
			t.Fatalf("%s/%s plugins=%d", platform.os, platform.arch, len(s.Provisioners))
		}
		for _, p := range s.Provisioners {
			if p.Tool != "herdr" {
				t.Fatalf("wrong retained tool: %+v", p)
			}
		}
		for _, d := range s.Dependencies {
			if err := d.RetirementError(); err != nil {
				t.Fatal(err)
			}
			if d.Name == "macmon" {
				t.Fatal("metrics became implicit")
			}
		}
	}
}

func TestHerdrRetirementRejectsInertCurrentDeclarations(t *testing.T) {
	const base = `version: 1
profiles:
  core: {tags: [base]}
entries:
  - {source: configs/base, target: ~/.base, strategy: copy, tags: [base]}
`
	for _, text := range []string{
		base + "tags:\n  base: {kind: surface, status: current}\n  herdr-custom: {kind: surface, status: current}\n",
		strings.Replace(base, "core: {tags: [base]}", "core: {tags: [herdr-custom]}", 1),
		strings.Replace(base, "strategy: copy, tags: [base]", "strategy: copy, tags: [herdr-custom]", 1),
		base + "dependencies:\n  - tags: [herdr-custom]\n    dependencies: [{name: benign}]\n",
	} {
		if _, err := manifest.Parse([]byte(text)); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
			t.Fatalf("offered retired name: %v", err)
		}
		path := filepath.Join(t.TempDir(), "dots.yaml")
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := manifest.LoadPreviousFile(path); err != nil {
			t.Fatalf("inert historical inventory: %v", err)
		}
	}
}
