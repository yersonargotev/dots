package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

func TestInstallRecordsAuthoritativeSelectionAfterSuccessfulApply(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  core:
    tags: [core, shared]
  agents:
    tags: [agents, shared]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes", "--skip-deps",
		"--profile", "core", "--profile", "agents",
		"--tag", "shared", "--tag", "adaptive-theme", "--tag", "adaptive-theme",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\noutput:\n%s", err, out.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.Version != state.CurrentVersion {
		t.Fatalf("metadata version = %d, want %d", meta.Version, state.CurrentVersion)
	}
	if meta.InstalledSelection == nil {
		t.Fatal("InstalledSelection = nil after successful install")
	}
	got := meta.InstalledSelection
	if want := []string{"core", "agents"}; !reflect.DeepEqual(got.Profiles, want) {
		t.Fatalf("Profiles = %#v, want %#v", got.Profiles, want)
	}
	if want := []string{"shared", "adaptive-theme"}; !reflect.DeepEqual(got.ExtraTags, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got.ExtraTags, want)
	}
	if want := []string{"core", "shared", "agents", "adaptive-theme"}; !reflect.DeepEqual(got.ResolvedTags, want) {
		t.Fatalf("ResolvedTags = %#v, want %#v", got.ResolvedTags, want)
	}
	if got.Provenance.SourceRoot != sourceRoot {
		t.Fatalf("SourceRoot = %q, want %q", got.Provenance.SourceRoot, sourceRoot)
	}
	if got.Provenance.RecordedAt == "" {
		t.Fatal("RecordedAt is empty")
	}
	if len(meta.Entries) != 1 {
		t.Fatalf("Entries = %#v, want the Managed Entry inventory preserved", meta.Entries)
	}
}

func TestInstallTagOnlySelectionAppliesAndRecordsWithoutProfile(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
tags:
  zsh:
    description: Zsh configuration
    kind: surface
    status: current
profiles:
  workstation:
    tags: [zsh]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [zsh]
`)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--skip-deps", "--output", "json", "--tag", "zsh",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitOK, out.String(), errOut.String())
	}
	if _, err := os.Readlink(filepath.Join(home, ".zshrc")); err != nil {
		t.Fatalf("Tag-only install did not apply Managed Entry: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if meta.InstalledSelection == nil {
		t.Fatal("InstalledSelection = nil")
	}
	if len(meta.InstalledSelection.Profiles) != 0 {
		t.Fatalf("Profiles = %#v, want none", meta.InstalledSelection.Profiles)
	}
	if got, want := meta.InstalledSelection.ExtraTags, []string{"zsh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got, want)
	}
	if got, want := meta.InstalledSelection.ResolvedTags, []string{"zsh"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolvedTags = %#v, want %#v", got, want)
	}

	dryHome := t.TempDir()
	dryStateRoot := filepath.Join(t.TempDir(), "state")
	cmd := cli.NewRootCommand()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--dry-run", "--skip-deps", "--tag", "zsh",
		"--file", manifestPath, "--home", dryHome, "--source-root", sourceRoot, "--state-root", dryStateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("human Tag-only dry run error = %v\noutput:\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Plan for tags only (tags: zsh)") {
		t.Fatalf("human Tag-only output missing selection:\n%s", out.String())
	}
	if _, err := os.Stat(dryStateRoot); !os.IsNotExist(err) {
		t.Fatalf("dry run created state root: %v", err)
	}
}

func TestRepositoryAtomicCoreTagsInstallIndependentlyInTemporaryHome(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repositoryRoot, "dots.yaml")

	for _, tag := range []string{"zsh", "node", "jq"} {
		t.Run(tag, func(t *testing.T) {
			home := t.TempDir()
			stateRoot := t.TempDir()
			fakeRealHome := t.TempDir()
			t.Setenv("HOME", fakeRealHome)
			stubDir := t.TempDir()
			writeManifestDependencyStubs(t, stubDir)
			t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

			var out, errOut bytes.Buffer
			code := cli.Run([]string{
				"install", "--yes", "--output", "json", "--tag", tag,
				"--file", manifestPath, "--home", home, "--source-root", repositoryRoot, "--state-root", stateRoot,
			}, &out, &errOut)
			if code != cli.ExitOK {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitOK, out.String(), errOut.String())
			}

			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatalf("load Installation Metadata: %v", err)
			}
			if meta.InstalledSelection == nil {
				t.Fatal("Installed Selection = nil")
			}
			if got := meta.InstalledSelection.Profiles; len(got) != 0 {
				t.Fatalf("Profiles = %#v, want none", got)
			}
			if got := meta.InstalledSelection.ExtraTags; !reflect.DeepEqual(got, []string{tag}) {
				t.Fatalf("explicit extra Tags = %#v, want %q", got, tag)
			}
			if got := meta.InstalledSelection.ResolvedTags; !reflect.DeepEqual(got, []string{tag}) {
				t.Fatalf("resolved Tags = %#v, want %q", got, tag)
			}

			if tag == "zsh" {
				for _, target := range []string{".zshrc", ".zshenv", filepath.Join(".config", "dots", "zsh", "zshrc")} {
					if _, err := os.Lstat(filepath.Join(home, target)); err != nil {
						t.Errorf("zsh Tag did not install %s: %v", target, err)
					}
				}
				if _, err := os.Lstat(filepath.Join(home, ".zimrc")); !os.IsNotExist(err) {
					t.Errorf("zsh Tag installed the independent Zim configuration: %v", err)
				}
			} else if len(meta.Entries) != 0 {
				t.Errorf("Dependency-only Tag %q recorded Managed Entries: %#v", tag, meta.Entries)
			}

			if entries, err := os.ReadDir(fakeRealHome); err != nil || len(entries) != 0 {
				t.Fatalf("install touched inherited HOME %q: entries=%v err=%v", fakeRealHome, entries, err)
			}
		})
	}
}

func TestRepositoryAtomicDesktopAndAgentTagsInstallIndependentlyInTemporaryHome(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(repositoryRoot, "dots.yaml")
	tests := []struct {
		tag      string
		present  []string
		excluded []string
	}{
		{
			tag:      "ghostty",
			present:  []string{filepath.Join(".config", "ghostty", "config.ghostty")},
			excluded: []string{filepath.Join(".warp", "settings.toml"), filepath.Join(".config", "zed", "settings.json")},
		},
		{
			tag:      "codex",
			present:  []string{filepath.Join(".codex", "config.toml")},
			excluded: []string{filepath.Join(".claude", "settings.json"), filepath.Join(".config", "opencode", "opencode.json")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			home := t.TempDir()
			stateRoot := t.TempDir()
			fakeRealHome := t.TempDir()
			t.Setenv("HOME", fakeRealHome)

			var out, errOut bytes.Buffer
			code := cli.Run([]string{
				"install", "--yes", "--skip-deps", "--output", "json", "--tag", tt.tag,
				"--file", manifestPath, "--home", home, "--source-root", repositoryRoot, "--state-root", stateRoot,
			}, &out, &errOut)
			if code != cli.ExitOK {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitOK, out.String(), errOut.String())
			}

			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatalf("load Installation Metadata: %v", err)
			}
			if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{tt.tag}) || !reflect.DeepEqual(meta.InstalledSelection.ResolvedTags, []string{tt.tag}) {
				t.Fatalf("Installed Selection = %#v, want only Tag %q", meta.InstalledSelection, tt.tag)
			}
			for _, target := range tt.present {
				if _, err := os.Lstat(filepath.Join(home, target)); err != nil {
					t.Errorf("Tag %q did not install %s: %v", tt.tag, target, err)
				}
			}
			for _, target := range tt.excluded {
				if _, err := os.Lstat(filepath.Join(home, target)); !os.IsNotExist(err) {
					t.Errorf("Tag %q installed unrelated target %s: %v", tt.tag, target, err)
				}
			}
			if entries, err := os.ReadDir(fakeRealHome); err != nil || len(entries) != 0 {
				t.Fatalf("install touched inherited HOME %q: entries=%v err=%v", fakeRealHome, entries, err)
			}
		})
	}
}

func TestInstallLegacyTagNormalizesBeforeApplyAndReportsJSONEvidence(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	writeCLISource(t, sourceRoot, "configs/zsh", "zsh\n")
	writeCLISource(t, sourceRoot, "configs/starship", "starship\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
tags:
  zsh: {description: Zsh, kind: surface, status: current}
  starship: {description: Starship, kind: surface, status: current}
  core:
    description: Legacy core
    kind: compatibility
    status: legacy
    replaced_by: [zsh, starship]
profiles:
  shell:
    tags: [zsh, starship]
entries:
  - {source: configs/zsh, target: ~/.zshrc, strategy: symlink, tags: [zsh]}
  - {source: configs/starship, target: ~/.config/starship.toml, strategy: symlink, tags: [starship]}
`)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--skip-deps", "--output", "json",
		"--tag", "core", "--tag", "zsh", "--tag", "core",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitOK, out.String(), errOut.String())
	}
	for _, target := range []string{".zshrc", filepath.Join(".config", "starship.toml")} {
		if _, err := os.Readlink(filepath.Join(home, target)); err != nil {
			t.Fatalf("normalized legacy install did not apply %s: %v", target, err)
		}
	}
	for _, want := range []string{`"tag_migrations"`, `"legacy_tag": "core"`, `"replacement_tags": [`, `"zsh"`, `"starship"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("JSON output missing %q:\n%s", want, out.String())
		}
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if meta.InstalledSelection == nil {
		t.Fatal("InstalledSelection = nil")
	}
	if got, want := meta.InstalledSelection.ExtraTags, []string{"zsh", "starship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got, want)
	}
	if got, want := meta.InstalledSelection.ResolvedTags, []string{"zsh", "starship"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolvedTags = %#v, want %#v", got, want)
	}
}

func TestInstallFailurePreservesPreviousInstalledSelection(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nexit 7\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	previous := state.InstalledSelection{
		Profiles:     []string{"old"},
		ExtraTags:    []string{"old-extra"},
		ResolvedTags: []string{"old", "old-extra"},
		Provenance:   state.Provenance{SourceRoot: "/old/source", RecordedAt: "2026-01-02T03:04:05Z"},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		Entries:            []state.Record{},
		InstalledSelection: &previous,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  new:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--profile", "new",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want failing provisioner\noutput:\n%s", out.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.Version != state.CurrentVersion {
		t.Fatalf("metadata version = %d, want preserved v%d", meta.Version, state.CurrentVersion)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
	if len(meta.Entries) != 1 {
		t.Fatalf("Entries = %#v, want partial Managed Entry inventory retained", meta.Entries)
	}
	if len(meta.Entries[0].Contributions) != 0 {
		t.Fatalf("Contributions = %#v, want no exact evidence from failed install", meta.Entries[0].Contributions)
	}
	if len(meta.Provisioners) != 1 || meta.Provisioners[0].Status != "failed" {
		t.Fatalf("Provisioners = %#v, want failed inventory retained", meta.Provisioners)
	}
}

func TestInstallYesSelectionReductionRequiresDedicatedAcknowledgement(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	previous := state.InstalledSelection{
		Profiles:     []string{"old"},
		ExtraTags:    []string{"old-extra"},
		ResolvedTags: []string{"old", "old-extra"},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		InstalledSelection: &previous,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}
	writeCLISource(t, sourceRoot, "configs/new", "new\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  new:
    tags: [new]
entries:
  - source: configs/new
    target: ~/.new
    strategy: symlink
    tags: [new]
`)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--skip-deps", "--output", "json", "--profile", "new",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitError {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitError, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "selection-change-acknowledgement-required") {
		t.Fatalf("JSON error missing acknowledgement code:\n%s", out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".new")); !os.IsNotExist(err) {
		t.Fatalf("selection rejection applied Managed Configuration: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
}

func TestInstallRejectsRetiredCodexDelegationSelectors(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	manifestPath := filepath.Join(repositoryRoot, "dots.yaml")

	for _, tc := range []struct {
		name  string
		flag  string
		value string
		want  string
	}{
		{name: "profile", flag: "--profile", value: "codex-delegation", want: `profile "codex-delegation" not found`},
		{name: "surface tag", flag: "--tag", value: "codex-delegation", want: `tag "codex-delegation" is not declared`},
		{name: "cleanup tag", flag: "--tag", value: "without-codex-delegation", want: `tag "without-codex-delegation" is not declared`},
		{name: "legacy surface tag", flag: "--tag", value: "codex-spark-delegation", want: `tag "codex-spark-delegation" is not declared`},
		{name: "legacy cleanup tag", flag: "--tag", value: "without-codex-spark-delegation", want: `tag "without-codex-spark-delegation" is not declared`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			args := []string{
				"install", "--dry-run", tc.flag, tc.value,
				"--file", manifestPath,
				"--home", t.TempDir(),
				"--source-root", repositoryRoot,
				"--state-root", t.TempDir(),
			}
			if tc.flag == "--tag" {
				args = append(args, "--profile", "core")
			}
			code := cli.Run(args, &out, &errOut)
			if code != cli.ExitError {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitError, out.String(), errOut.String())
			}
			if !strings.Contains(errOut.String(), tc.want) {
				t.Fatalf("stderr missing %q:\n%s", tc.want, errOut.String())
			}
		})
	}
}

func TestInstallAcknowledgedDelegationReductionRetiresOwnedBlocksAndReportsCopiedSkill(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	previous := state.InstalledSelection{
		Profiles:     []string{"codex-delegation"},
		ResolvedTags: []string{"codex-delegation"},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		InstalledSelection: &previous,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	codexInstructions := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(codexInstructions), 0o755); err != nil {
		t.Fatalf("mkdir Codex instructions: %v", err)
	}
	if err := os.WriteFile(codexInstructions, []byte("user before\n<!-- dots:delegation -->\nowned\n<!-- /dots:delegation -->\nuser after\n"), 0o600); err != nil {
		t.Fatalf("write Codex instructions: %v", err)
	}
	copiedSkill := filepath.Join(home, ".agents", "skills", "delegation")
	if err := os.MkdirAll(copiedSkill, 0o755); err != nil {
		t.Fatalf("write copied skill: %v", err)
	}

	writeCLISource(t, sourceRoot, "configs/core", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/core
    target: ~/.core
    strategy: symlink
    tags: [core]
`)

	var out, errOut bytes.Buffer
	code := cli.Run([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--output", "json", "--profile", "core",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	}, &out, &errOut)
	if code != cli.ExitOK {
		t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, cli.ExitOK, out.String(), errOut.String())
	}
	if got, err := os.ReadFile(codexInstructions); err != nil || string(got) != "user before\n\nuser after\n" {
		t.Fatalf("Codex instructions = %q, %v; want only user content", got, err)
	}
	if _, err := os.Stat(copiedSkill); err != nil {
		t.Fatalf("copied skill was removed: %v", err)
	}
	for _, want := range []string{`"retirement"`, `~/.codex/AGENTS.md delegation blocks`, `~/.agents/skills/delegation`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("JSON output missing %q:\n%s", want, out.String())
		}
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load updated metadata: %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"core"}) {
		t.Fatalf("InstalledSelection = %#v, want core after terminal success", meta.InstalledSelection)
	}
}

func TestInstallPostProvisionerConvergenceFailurePreservesPreviousSelection(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "codegraph"), "#!/bin/sh\nmkdir -p \"$HOME/.codex/AGENTS.md\"\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	previous := state.InstalledSelection{
		Profiles:     []string{"old"},
		ResolvedTags: []string{"old"},
		Provenance:   state.Provenance{RecordedAt: "2026-01-02T03:04:05Z"},
	}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		Entries:            []state.Record{},
		InstalledSelection: &previous,
	}); err != nil {
		t.Fatalf("seed metadata: %v", err)
	}

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  new:
    tags: [core, codegraph]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
provisioners:
  - tool: codegraph
    tags: [codegraph]
    spec:
      scope: global
      agents: [codex]
      yes: true
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--profile", "new",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want post-Provisioner convergence failure\noutput:\n%s", out.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
	if len(meta.Provisioners) != 1 || meta.Provisioners[0].Status != "provisioned" {
		t.Fatalf("Provisioners = %#v, want successful inventory before convergence failure", meta.Provisioners)
	}
	if len(meta.Entries) != 1 || len(meta.Entries[0].Contributions) != 0 {
		t.Fatalf("Entries = %#v, want partial inventory without exact evidence", meta.Entries)
	}
}

func TestInstallHistoricalRetirementFailurePreservesPreviousSelection(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("HOME", t.TempDir())

	previous := state.InstalledSelection{Profiles: []string{"old"}, ResolvedTags: []string{"old"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version:            state.CurrentVersion,
		InstalledSelection: &previous,
		Provisioners: []state.ProvisionerRecord{{
			Profile: "agents", Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global"}, Status: "provisioned",
		}},
	}); err != nil {
		t.Fatalf("seed historical metadata: %v", err)
	}
	instructions := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(instructions), 0o755); err != nil {
		t.Fatal(err)
	}
	malformed := "before\n<!-- gentle-ai:trigger-rules -->\nunclosed\n"
	if err := os.WriteFile(instructions, []byte(malformed), 0o600); err != nil {
		t.Fatal(err)
	}
	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  new:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes", "--acknowledge-selection-change", "--skip-deps", "--profile", "new",
		"--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("Execute() error = nil, want malformed historical retirement failure\noutput:\n%s", out.String())
	}

	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(*meta.InstalledSelection, previous) {
		t.Fatalf("InstalledSelection = %#v, want previous %#v", meta.InstalledSelection, previous)
	}
	if len(meta.Entries) != 1 || len(meta.Entries[0].Contributions) != 0 {
		t.Fatalf("Entries = %#v, want partial inventory without exact evidence", meta.Entries)
	}
	got, err := os.ReadFile(instructions)
	if err != nil || string(got) != malformed {
		t.Fatalf("malformed instructions changed: %q, %v", got, err)
	}
}

func TestInstallWithoutExplicitProfileMutatesNothing(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HOME", t.TempDir())

	writeCLISource(t, sourceRoot, "configs/zsh/zshrc", "managed\n")
	manifestPath := writeCLIManifest(t, home, `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/zsh/zshrc
    target: ~/.zshrc
    strategy: symlink
    tags: [core]
    dependencies:
      - name: must-not-run
        command: definitely-missing-profile-guard-probe
provisioners:
  - tool: claude
    tags: [core]
    spec:
      marketplace: example/tools
`)

	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--yes",
		"--file", manifestPath,
		"--home", home,
		"--source-root", sourceRoot,
		"--state-root", stateRoot,
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "selection required") {
		t.Fatalf("Execute() error = %v, want explicit selection guidance\noutput:\n%s", err, out.String())
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatalf("target mutated without Profile; lstat err = %v", err)
	}
	if _, err := os.Stat(stateRoot); !os.IsNotExist(err) {
		t.Fatalf("state root created without Profile; stat err = %v", err)
	}
}
