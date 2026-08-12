package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/catalog"
)

func TestCatalogCommandsRenderManifestOnlyViews(t *testing.T) {
	manifestPath := writeCatalogManifest(t)

	tests := []struct {
		name  string
		args  []string
		want  []string
		avoid []string
	}{
		{
			name:  "compact summary hides legacy",
			args:  []string{"catalog", "--file", manifestPath, "--os", "all"},
			want:  []string{"Catalog (OS: all; metadata: declared)", "- core (current)", "- theme (surface, current, declared)", "Hidden legacy items:"},
			avoid: []string{"- old (legacy)"},
		},
		{
			name:  "profile list focuses profiles and includes legacy on request",
			args:  []string{"catalog", "profiles", "--file", manifestPath, "--os", "all", "--all"},
			want:  []string{"Profiles (OS: all; metadata: declared)", "- core (current)", "- old (legacy)"},
			avoid: []string{"Tags:"},
		},
		{
			name:  "profile map renders a compact tag-oriented surface",
			args:  []string{"catalog", "map", "desktop", "--file", manifestPath, "--os", "all"},
			want:  []string{"Profile map \"desktop\" (OS: all)", "desktop", "├─ core — Core configuration", "└─ theme — Theme configuration", "1 entry · 1 dependency · 1 provisioner", "Total: 1 entry · 1 unique dependency · 1 provisioner"},
			avoid: []string{"must-not-leak", "Source overrides:", "Excluded surfaces:"},
		},
		{
			name:  "tag list focuses current tags",
			args:  []string{"catalog", "tags", "--file", manifestPath, "--os", "all"},
			want:  []string{"Tags (OS: all; metadata: declared)", "- theme (surface, current, declared)", "Hidden legacy tags: 1"},
			avoid: []string{"Profiles:", "- old (surface, legacy, declared)"},
		},
		{
			name: "legacy tag detail remains directly addressable",
			args: []string{"catalog", "tag", "old", "--file", manifestPath, "--os", "all"},
			want: []string{"Tag \"old\"", "Status: legacy", "Replaced by: core"},
		},
		{
			name:  "tag detail renders surfaces and exclusions",
			args:  []string{"catalog", "tag", "theme", "--file", manifestPath, "--os", "linux"},
			want:  []string{"Tag \"theme\"", "Dependency sets:", "Entries:", "Source overrides:", "Provisioners:", "Behaviors:", "Excluded surfaces:", "not applicable to linux"},
			avoid: []string{"must-not-leak"},
		},
		{
			name:  "profile comparison renders a directional delta",
			args:  []string{"catalog", "compare", "core", "desktop", "--file", manifestPath, "--os", "all"},
			want:  []string{"Profile comparison: core -> desktop", "Added:", "+ tag theme", "+ dependency theme-tool", "+ entry adaptive -> ~/.example (copy)", "+ provisioner codex (mcp: demo)", "Removed:", "Shared:"},
			avoid: []string{"must-not-leak"},
		},
		{
			name:  "why traces a dependency to its profile tag",
			args:  []string{"catalog", "why", "desktop", "theme-tool", "--file", manifestPath, "--os", "all"},
			want:  []string{"Why profile \"desktop\" selects \"theme-tool\" (OS: all)", "desktop", "dependency theme-tool", "selected by tags: theme", "declared by dependency_set tags=theme"},
			avoid: []string{"must-not-leak", "Installed Selection", "Drift"},
		},
		{
			name:  "why explains the winning entry source override",
			args:  []string{"catalog", "why", "desktop", "adaptive", "--file", manifestPath, "--os", "all"},
			want:  []string{"entry ~/.example", "selected by tags: theme", "adaptive -> ~/.example (copy; source override: theme)"},
			avoid: []string{"must-not-leak"},
		},
		{
			name:  "why matches a provisioner identity",
			args:  []string{"catalog", "why", "desktop", "demo", "--file", manifestPath, "--os", "all"},
			want:  []string{"provisioner demo", "selected by tags: theme", "codex mcp: demo"},
			avoid: []string{"must-not-leak"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("output missing %q:\n%s", want, out.String())
				}
			}
			for _, avoid := range tt.avoid {
				if strings.Contains(out.String(), avoid) {
					t.Fatalf("output unexpectedly contains %q:\n%s", avoid, out.String())
				}
			}
		})
	}
}

func TestCatalogDetailCompletionUsesManifestNames(t *testing.T) {
	manifestPath := writeCatalogManifest(t)
	root := NewRootCommand()
	profileCmd, _, err := root.Find([]string{"catalog", "profile"})
	if err != nil {
		t.Fatalf("find profile: %v", err)
	}
	if err := profileCmd.InheritedFlags().Set("file", manifestPath); err != nil {
		t.Fatalf("set profile file: %v", err)
	}
	if err := profileCmd.InheritedFlags().Set("os", "all"); err != nil {
		t.Fatalf("set profile os: %v", err)
	}
	profiles, directive := profileCmd.ValidArgsFunction(profileCmd, nil, "")
	if !reflect.DeepEqual(profiles, []string{"core", "desktop", "old"}) {
		t.Fatalf("profile completion = %#v", profiles)
	}

	compareCmd, _, err := root.Find([]string{"catalog", "compare"})
	if err != nil {
		t.Fatalf("find compare: %v", err)
	}
	if err := compareCmd.InheritedFlags().Set("file", manifestPath); err != nil {
		t.Fatalf("set compare file: %v", err)
	}
	profiles, directive = compareCmd.ValidArgsFunction(compareCmd, []string{"core"}, "")
	if !reflect.DeepEqual(profiles, []string{"desktop", "old"}) {
		t.Fatalf("comparison second-profile completion = %#v", profiles)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("comparison completion directive = %v", directive)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("profile completion directive = %v", directive)
	}

	mapCmd, _, err := root.Find([]string{"catalog", "map"})
	if err != nil {
		t.Fatalf("find map: %v", err)
	}
	if err := mapCmd.InheritedFlags().Set("file", manifestPath); err != nil {
		t.Fatalf("set map file: %v", err)
	}
	profiles, directive = mapCmd.ValidArgsFunction(mapCmd, nil, "d")
	if !reflect.DeepEqual(profiles, []string{"desktop"}) {
		t.Fatalf("map completion = %#v", profiles)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("map completion directive = %v", directive)
	}

	tagCmd, _, err := root.Find([]string{"catalog", "tag"})
	if err != nil {
		t.Fatalf("find tag: %v", err)
	}
	if err := tagCmd.InheritedFlags().Set("file", manifestPath); err != nil {
		t.Fatalf("set tag file: %v", err)
	}
	if err := tagCmd.InheritedFlags().Set("os", "all"); err != nil {
		t.Fatalf("set tag os: %v", err)
	}
	tags, directive := tagCmd.ValidArgsFunction(tagCmd, nil, "t")
	if !reflect.DeepEqual(tags, []string{"theme"}) {
		t.Fatalf("tag completion = %#v", tags)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("tag completion directive = %v", directive)
	}

	whyCmd, _, err := root.Find([]string{"catalog", "why"})
	if err != nil {
		t.Fatalf("find why: %v", err)
	}
	if err := whyCmd.InheritedFlags().Set("file", manifestPath); err != nil {
		t.Fatalf("set why file: %v", err)
	}
	if err := whyCmd.InheritedFlags().Set("os", "all"); err != nil {
		t.Fatalf("set why os: %v", err)
	}
	profiles, directive = whyCmd.ValidArgsFunction(whyCmd, nil, "d")
	if !reflect.DeepEqual(profiles, []string{"desktop"}) {
		t.Fatalf("why profile completion = %#v", profiles)
	}
	items, directive := whyCmd.ValidArgsFunction(whyCmd, []string{"desktop"}, "")
	wantItems := []string{"theme-tool", "~/.example", "adaptive", "codex", "demo"}
	if !reflect.DeepEqual(items, wantItems) {
		t.Fatalf("why item completion = %#v, want %#v", items, wantItems)
	}
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Fatalf("why completion directive = %v", directive)
	}
}

func TestCatalogJSONAndErrorsUseTheOutputContract(t *testing.T) {
	manifestPath := writeCatalogManifest(t)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"catalog", "profile", "core", "--file", manifestPath, "--os", "all", "--output", "json"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("Run() exit code = %d, stderr = %s", code, stderr.String())
	}
	var result envelope
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, stdout.String())
	}
	if result.SchemaVersion != schemaVersion || result.Command != "catalog profile" || result.Status != statusOK {
		t.Fatalf("envelope = %#v", result)
	}
	data, ok := result.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", result.Data)
	}
	if _, ok := data["profile"].(map[string]any); !ok {
		t.Fatalf("profile detail missing from data: %#v", data)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"catalog", "why", "desktop", "theme-tool", "--file", manifestPath, "--os", "all", "--output", "json"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("why Run() exit code = %d, stderr = %s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode why JSON: %v\n%s", err, stdout.String())
	}
	if result.Command != "catalog why" || result.Status != statusOK {
		t.Fatalf("why envelope = %#v", result)
	}
	data, ok = result.Data.(map[string]any)
	if !ok {
		t.Fatalf("why data = %#v", result.Data)
	}
	why, ok := data["why"].(map[string]any)
	if !ok || why["profile"] != "desktop" || why["query"] != "theme-tool" {
		t.Fatalf("why explanation missing from data: %#v", data)
	}
	var stableWhy struct {
		Data struct {
			Why catalog.Why `json:"why"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &stableWhy); err != nil {
		t.Fatalf("decode stable why payload: %v", err)
	}
	wantWhy := catalog.Why{
		Profile:      "desktop",
		Query:        "theme-tool",
		ResolvedTags: []string{"core", "theme"},
		Matches: []catalog.WhyMatch{{
			Type:             "dependency",
			Identity:         "theme-tool",
			ContributingTags: []string{"theme"},
			Dependency: &catalog.WhyDependency{
				Name: "theme-tool",
				Declarations: []catalog.Dependency{{
					Name:        "theme-tool",
					Requirement: "required",
					Probes:      []string{"theme-tool"},
					Origin:      catalog.Origin{Type: "dependency_set", Tags: []string{"theme"}},
				}},
			},
		}},
	}
	if !reflect.DeepEqual(stableWhy.Data.Why, wantWhy) {
		t.Fatalf("why payload = %#v, want %#v", stableWhy.Data.Why, wantWhy)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"catalog", "compare", "core", "desktop", "--file", manifestPath, "--os", "all", "--output", "json"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("comparison Run() exit code = %d, stderr = %s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode comparison JSON: %v\n%s", err, stdout.String())
	}
	if result.Command != "catalog compare" || result.Status != statusOK {
		t.Fatalf("comparison envelope = %#v", result)
	}
	data, ok = result.Data.(map[string]any)
	if !ok {
		t.Fatalf("comparison data = %#v", result.Data)
	}
	if _, ok := data["comparison"].(map[string]any); !ok {
		t.Fatalf("comparison missing from data: %#v", data)
	}
	if strings.Contains(stdout.String(), "must-not-leak") {
		t.Fatalf("JSON leaked provisioner environment value: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"catalog", "map", "desktop", "--file", manifestPath, "--os", "all", "--output", "json"}, &stdout, &stderr); code != ExitOK {
		t.Fatalf("map Run() exit code = %d, stderr = %s", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode map JSON: %v\n%s", err, stdout.String())
	}
	if result.Command != "catalog map" || result.Status != statusOK {
		t.Fatalf("map envelope = %#v", result)
	}
	data, ok = result.Data.(map[string]any)
	if !ok {
		t.Fatalf("map data = %#v", result.Data)
	}
	profileMap, ok := data["map"].(map[string]any)
	if !ok || profileMap["profile"] != "desktop" {
		t.Fatalf("map missing from data: %#v", data)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"catalog", "--file", manifestPath, "--os", "windows", "--output", "json"}, &stdout, &stderr); code != ExitError {
		t.Fatalf("invalid OS exit code = %d", code)
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode error JSON: %v\n%s", err, stdout.String())
	}
	if result.Command != "catalog" || result.Status != statusError || !strings.Contains(result.Error, "catalog OS \"windows\" is invalid") {
		t.Fatalf("error envelope = %#v", result)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"catalog", "profile", "missing", "--file", manifestPath, "--output", "json"}, &stdout, &stderr); code != ExitError {
		t.Fatalf("unknown profile exit code = %d", code)
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode unknown-profile JSON: %v\n%s", err, stdout.String())
	}
	if result.Command != "catalog profile" || result.Status != statusError || result.Error != `profile "missing" not found` {
		t.Fatalf("unknown-profile envelope = %#v", result)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"catalog", "why", "desktop", "missing", "--file", manifestPath, "--os", "linux", "--output", "json"}, &stdout, &stderr); code != ExitError {
		t.Fatalf("unknown why query exit code = %d", code)
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode why error JSON: %v\n%s", err, stdout.String())
	}
	if result.Command != "catalog why" || result.Status != statusError || result.Error != `profile "desktop" does not select an item matching "missing" for OS linux` {
		t.Fatalf("unknown why-query envelope = %#v", result)
	}
}

func TestCatalogDetailsRequireExactlyOneName(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"catalog", "profile"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "accepts 1 arg(s), received 0") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestCatalogWhyRequiresProfileAndQuery(t *testing.T) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"catalog", "why", "desktop"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "accepts 2 arg(s), received 1") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestCatalogUsesAnExplicitSourceRootWithoutHomeOrStateFlags(t *testing.T) {
	sourceRoot := t.TempDir()
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	writeCatalogManifestAt(t, manifestPath)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"catalog", "--source-root", sourceRoot, "--os", "all"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Catalog (OS: all; metadata: declared)") {
		t.Fatalf("catalog did not load default manifest from explicit source root:\n%s", out.String())
	}
}

func TestCatalogUsesTheDefaultInstalledRepositoryConvention(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sourceRoot := filepath.Join(home, ".local", "share", "dots")
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("mkdir source root: %v", err)
	}
	writeCatalogManifestAt(t, manifestPath)

	var out, errOut bytes.Buffer
	if code := Run([]string{"catalog", "--os", "all"}, &out, &errOut); code != ExitOK {
		t.Fatalf("Run() exit code = %d\nstderr:\n%s\nstdout:\n%s", code, errOut.String(), out.String())
	}
	if !strings.Contains(out.String(), "Catalog (OS: all; metadata: declared)") {
		t.Fatalf("catalog did not load the default Installed Repository:\n%s", out.String())
	}
}

func writeCatalogManifest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dots.yaml")
	writeCatalogManifestAt(t, path)
	return path
}

func writeCatalogManifestAt(t *testing.T, path string) {
	t.Helper()
	contents := `version: 1
tags:
  core:
    description: Core configuration
    kind: surface
    status: current
  theme:
    description: Theme configuration
    kind: surface
    status: current
  old:
    description: Compatibility profile
    kind: surface
    status: legacy
    replaced_by: core
profiles:
  core:
    description: Core profile
    tags: [core]
  desktop:
    description: Desktop profile
    tags: [core, theme]
  old:
    status: legacy
    tags: [old]
dependencies:
  - tags: [theme]
    dependencies:
      - name: theme-tool
entries:
  - source: base
    source_overrides:
      theme: adaptive
    target: ~/.example
    strategy: copy
    tags: [theme]
    os: [darwin]
provisioners:
  - tool: codex
    tags: [theme]
    os: [darwin]
    spec:
      mcp: demo
      command: [demo, serve]
      env:
        SECRET: must-not-leak
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
