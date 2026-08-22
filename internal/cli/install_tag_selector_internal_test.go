package cli

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
	tagselectortui "github.com/yersonargotev/dots/internal/tui/tagselector"
)

func TestInstallTagSelectorRouting(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	base := []string{"install", "--dry-run", "--skip-deps", "--file", manifestPath, "--source-root", sourceRoot, "--home", home, "--state-root", stateRoot}

	t.Run("interactive terminal previews without applying", func(t *testing.T) {
		called := 0
		setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, initial []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
			called++
			if len(initial) != 0 {
				t.Fatalf("initial Tags = %v, want empty without Installed Selection", initial)
			}
			built, err := preview([]string{"one"})
			return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, err
		})
		cmd := NewRootCommand()
		var out bytes.Buffer
		cmd.SetIn(strings.NewReader(""))
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(base)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v\n%s", err, out.String())
		}
		if called != 1 {
			t.Fatalf("selector calls = %d, want 1", called)
		}
		for _, want := range []string{"Forward-only selection preview", "Selection preview only; nothing was applied."} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("output missing %q\n%s", want, out.String())
			}
		}
		if _, err := os.Stat(state.Path(stateRoot)); !os.IsNotExist(err) {
			t.Fatalf("Installation Metadata exists after preview-only selector: %v", err)
		}
	})

	t.Run("bypass modes reject missing selection", func(t *testing.T) {
		tests := []struct {
			name     string
			terminal bool
			prefix   []string
			extra    []string
		}{
			{name: "non-terminal", terminal: false},
			{name: "JSON", terminal: true, prefix: []string{"--output", "json"}},
			{name: "confirmed", terminal: true, extra: []string{"--yes"}},
			{name: "text prompt", terminal: true, extra: []string{"--no-tui"}},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				called := 0
				setInstallTagSelectorTestHooks(t, tt.terminal, func(io.Reader, io.Writer, tagselectortui.BrowseData, []string, tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
					called++
					return tagselectortui.Result{}, nil
				})
				cmd := NewRootCommand()
				cmd.SetArgs(append(append(append([]string{}, tt.prefix...), base...), tt.extra...))
				err := cmd.Execute()
				if !errors.Is(err, selection.ErrSelectionRequired) {
					t.Fatalf("Execute() error = %v, want selection required", err)
				}
				if called != 0 {
					t.Fatalf("selector calls = %d, want 0", called)
				}
			})
		}
	})

	t.Run("explicit selection bypasses selector", func(t *testing.T) {
		called := 0
		setInstallTagSelectorTestHooks(t, true, func(io.Reader, io.Writer, tagselectortui.BrowseData, []string, tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
			called++
			return tagselectortui.Result{}, nil
		})
		cmd := NewRootCommand()
		cmd.SetArgs(append(append([]string{}, base...), "--profile", "default"))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if called != 0 {
			t.Fatalf("selector calls = %d, want 0", called)
		}
	})
}

func TestInstallTagSelectorCancelIsSuccessfulAndNonMutating(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	setInstallTagSelectorTestHooks(t, true, func(io.Reader, io.Writer, tagselectortui.BrowseData, []string, tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		return tagselectortui.Result{}, tagselectortui.ErrCanceled
	})
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--source-root", sourceRoot, "--home", home, "--state-root", stateRoot})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "Tag selection canceled; nothing was applied.") {
		t.Fatalf("output missing cancellation guarantee\n%s", out.String())
	}
	if _, err := os.Stat(state.Path(stateRoot)); !os.IsNotExist(err) {
		t.Fatalf("Installation Metadata exists after cancellation: %v", err)
	}
}

func TestInstallTagSelectorDefaultRepositoryIsByteStable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		runner installTagSelectorRunnerFunc
	}{
		{
			name: "completed preview",
			runner: func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
				built, err := preview([]string{"one"})
				return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, err
			},
		},
		{
			name: "canceled",
			runner: func(io.Reader, io.Writer, tagselectortui.BrowseData, []string, tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
				return tagselectortui.Result{}, tagselectortui.ErrCanceled
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, sourceRoot := writeTagSelectorGitRepository(t)
			t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".xdg-state"))
			before := fingerprintTree(t, home)
			setInstallTagSelectorTestHooks(t, true, tc.runner)

			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"install", "--home", home})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\n%s", err, out.String())
			}
			if after := fingerprintTree(t, home); after != before {
				t.Fatalf("home fingerprint changed after selector path: before %s, after %s", before, after)
			}
			if _, err := os.Stat(filepath.Join(home, ".one")); !os.IsNotExist(err) {
				t.Fatalf("Managed Entry exists after selector path: %v", err)
			}
			if _, err := os.Stat(state.Path(defaultStateRoot(home))); !os.IsNotExist(err) {
				t.Fatalf("Installation Metadata exists after selector path: %v", err)
			}
			if !strings.HasPrefix(sourceRoot, home+string(filepath.Separator)) {
				t.Fatalf("test Installed Repository %q is outside temporary home %q", sourceRoot, home)
			}
		})
	}
}

func TestInstallTagSelectorMissingDefaultRepositoryDoesNotBootstrap(t *testing.T) {
	home := t.TempDir()
	sourceRoot := defaultSourceRoot(home)
	called := 0
	setInstallTagSelectorTestHooks(t, true, func(io.Reader, io.Writer, tagselectortui.BrowseData, []string, tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		called++
		return tagselectortui.Result{}, nil
	})

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"install", "--home", home})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "Installed Repository not found") {
		t.Fatalf("Execute() error = %v, want missing Installed Repository guidance", err)
	}
	if called != 0 {
		t.Fatalf("selector calls = %d, want zero before manifest is available", called)
	}
	if _, statErr := os.Stat(sourceRoot); !os.IsNotExist(statErr) {
		t.Fatalf("selector path bootstrapped Installed Repository: %v", statErr)
	}
}

func TestInstallTagSelectorPreviewFailureIsNonMutating(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		_, err := preview([]string{"unknown"})
		return tagselectortui.Result{}, err
	})

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"install", "--file", manifestPath, "--source-root", sourceRoot, "--home", home, "--state-root", stateRoot})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `tag "unknown" is not declared`) {
		t.Fatalf("Execute() error = %v, want preview failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(home, ".one")); !os.IsNotExist(statErr) {
		t.Fatalf("Managed Entry exists after preview failure: %v", statErr)
	}
	if _, statErr := os.Stat(state.Path(stateRoot)); !os.IsNotExist(statErr) {
		t.Fatalf("Installation Metadata exists after preview failure: %v", statErr)
	}
}

func setInstallTagSelectorTestHooks(t *testing.T, terminal bool, runner func(io.Reader, io.Writer, tagselectortui.BrowseData, []string, tagselectortui.PreviewFunc) (tagselectortui.Result, error)) {
	t.Helper()
	previousTerminal := installTagSelectorTerminal
	previousRunner := installTagSelectorRunner
	installTagSelectorTerminal = func(io.Reader, io.Writer) bool { return terminal }
	installTagSelectorRunner = runner
	t.Cleanup(func() {
		installTagSelectorTerminal = previousTerminal
		installTagSelectorRunner = previousRunner
	})
}

func writeTagSelectorTestManifest(t *testing.T) (manifestPath, sourceRoot, home, stateRoot string) {
	t.Helper()
	home = t.TempDir()
	sourceRoot = t.TempDir()
	stateRoot = t.TempDir()
	configDir := filepath.Join(sourceRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create selector source directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "one"), []byte("one\n"), 0o600); err != nil {
		t.Fatalf("write selector source: %v", err)
	}
	manifestPath = filepath.Join(sourceRoot, "dots.yaml")
	data := []byte(`version: 1
tags:
  one:
    description: One selectable capability.
    kind: surface
    status: current
profiles:
  default:
    tags: [one]
dependencies:
  - tags: [one]
    dependencies:
      - name: one
        command: one
        manual: install one manually
entries:
  - source: config/one
    target: ~/.one
    strategy: symlink
    tags: [one]
`)
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatalf("write selector manifest: %v", err)
	}
	return manifestPath, sourceRoot, home, stateRoot
}

func writeTagSelectorGitRepository(t *testing.T) (home, sourceRoot string) {
	t.Helper()
	home = t.TempDir()
	sourceRoot = defaultSourceRoot(home)
	if err := os.MkdirAll(filepath.Join(sourceRoot, "config"), 0o755); err != nil {
		t.Fatalf("create Installed Repository: %v", err)
	}
	manifestData := []byte(`version: 1
tags:
  one:
    description: One selectable capability.
    kind: surface
    status: current
profiles:
  default:
    tags: [one]
entries:
  - source: config/one
    target: ~/.one
    strategy: symlink
    tags: [one]
`)
	if err := os.WriteFile(filepath.Join(sourceRoot, "dots.yaml"), manifestData, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	managedSource := filepath.Join(sourceRoot, "config", "one")
	if err := os.WriteFile(managedSource, []byte("committed\n"), 0o600); err != nil {
		t.Fatalf("write managed source: %v", err)
	}
	runGit(t, sourceRoot, "init", "-b", "main")
	runGit(t, sourceRoot, "config", "user.name", "Dots Tests")
	runGit(t, sourceRoot, "config", "user.email", "dots-tests@example.invalid")
	runGit(t, sourceRoot, "add", ".")
	runGit(t, sourceRoot, "commit", "-m", "test: seed selector repository")
	if err := os.WriteFile(managedSource, []byte("stashed\n"), 0o600); err != nil {
		t.Fatalf("write stashed source: %v", err)
	}
	runGit(t, sourceRoot, "stash", "push", "-m", "selector-safety")
	if err := os.WriteFile(managedSource, []byte("working tree\n"), 0o600); err != nil {
		t.Fatalf("write dirty source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "untracked"), []byte("preserve me\n"), 0o600); err != nil {
		t.Fatalf("write untracked source: %v", err)
	}
	return home, sourceRoot
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-C", directory}, args...)
	output, err := exec.Command("git", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func fingerprintTree(t *testing.T, root string) string {
	t.Helper()
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", filepath.ToSlash(relative), info.Mode())
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			fmt.Fprintf(hash, "%s\x00", target)
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		t.Fatalf("fingerprint %s: %v", root, err)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

func TestBuildInstallTagSelectorPreviewUsesSharedReconciliationReport(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	m := manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"old": {Kind: "surface", Status: "current"},
			"new": {Kind: "surface", Status: "current"},
		},
		Profiles: map[string]manifest.Profile{},
		Dependencies: []manifest.DependencySet{
			{Tags: []string{"old"}, Dependencies: []manifest.Dependency{{Name: "old-tool", Command: "old-tool"}}},
			{Tags: []string{"new"}, Dependencies: []manifest.Dependency{{Name: "new-tool", Command: "new-tool"}}},
		},
	}
	meta := state.Metadata{InstalledSelection: &state.InstalledSelection{ExtraTags: []string{"old"}, ResolvedTags: []string{"old"}}}
	tags := []string{"new"}
	preview, err := buildInstallTagSelectorPreview(m, meta, resolvedPaths{
		Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot, XDGStateHome: filepath.Join(home, ".local", "state"),
	}, sourceRoot, nil, "linux", tags)
	if err != nil {
		t.Fatalf("buildInstallTagSelectorPreview() error = %v", err)
	}
	if preview.ForwardOnly {
		t.Fatal("Preview.ForwardOnly = true, want shared reconciliation preview")
	}
	for _, want := range []string{"Selection reconciliation:", "retained-external-state", "old-tool", "create", "new-tool"} {
		if !strings.Contains(preview.Text, want) {
			t.Fatalf("Preview.Text missing %q\n%s", want, preview.Text)
		}
	}
	if !strings.HasPrefix(preview.SemanticDigest, "sha256:") {
		t.Fatalf("Preview.SemanticDigest = %q, want sha256 digest", preview.SemanticDigest)
	}

	originalText, originalDigest := preview.Text, preview.SemanticDigest
	tags[0] = "old"
	meta.InstalledSelection.ExtraTags[0] = "new"
	if preview.Text != originalText || preview.SemanticDigest != originalDigest {
		t.Fatal("opaque Preview changed after caller mutated input slices")
	}
}

func TestBuildInstallTagSelectorPreviewWithoutInstalledSelectionIsForwardOnly(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	m := manifest.Manifest{
		Tags:     map[string]manifest.Tag{"new": {Kind: "surface", Status: "current"}},
		Profiles: map[string]manifest.Profile{},
		Dependencies: []manifest.DependencySet{{
			Tags: []string{"new"}, Dependencies: []manifest.Dependency{{Name: "new-tool", Command: "new-tool"}},
		}},
	}
	meta := state.Metadata{Entries: []state.Record{{Target: filepath.Join(home, ".historical"), Source: "historical", Tags: []string{"old"}}}}
	preview, err := buildInstallTagSelectorPreview(m, meta, resolvedPaths{
		Home: home, SourceRoot: sourceRoot, StateRoot: t.TempDir(), XDGStateHome: filepath.Join(home, ".local", "state"),
	}, sourceRoot, nil, "linux", []string{"new"})
	if err != nil {
		t.Fatalf("buildInstallTagSelectorPreview() error = %v", err)
	}
	if !preview.ForwardOnly {
		t.Fatal("Preview.ForwardOnly = false, want forward-only without Installed Selection")
	}
	if !strings.Contains(preview.Text, "No Installed Selection is recorded") || !strings.Contains(preview.Text, "no retirement is authorized") {
		t.Fatalf("Preview.Text missing forward-only authority notice\n%s", preview.Text)
	}
	if strings.Contains(preview.Text, "Selection reconciliation:") {
		t.Fatalf("Preview.Text fabricates a Selection Reconciliation Plan\n%s", preview.Text)
	}
}

func TestBuildInstallTagSelectorCatalogGroupsCurrentTagsAndReportsComponents(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	m := manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"shared":      {Description: "shared capability", Kind: "surface", Status: "current"},
			"core-a":      {Description: "core capability", Kind: "surface", Status: "current"},
			"desktop-b":   {Description: "desktop capability", Kind: "surface", Status: "current"},
			"agents-c":    {Description: "agent capability", Kind: "surface", Status: "current"},
			"darwin-only": {Description: "platform capability", Kind: "surface", Status: "current"},
			"global":      {Description: "global capability", Kind: "surface", Status: "current"},
			"legacy":      {Description: "legacy alias", Kind: "compatibility", Status: "legacy", ReplacedBy: manifest.ReplacementTags{"core-a"}},
			"legacy-agents": {
				Description: "legacy agent alias", Kind: "compatibility", Status: "legacy", ReplacedBy: manifest.ReplacementTags{"agents-c"},
			},
		},
		Profiles: map[string]manifest.Profile{
			"core":        {Description: "core preset", Tags: []string{"shared", "core-a"}},
			"desktop":     {Description: "desktop preset", Tags: []string{"desktop-b", "shared"}},
			"agents":      {Description: "agents preset", Tags: []string{"agents-c"}},
			"workstation": {Description: "combined preset", Tags: []string{"core-a", "agents-c"}},
		},
		Dependencies: []manifest.DependencySet{
			{
				Tags:         []string{"desktop-b"},
				Dependencies: []manifest.Dependency{{Name: "desktop-tool", Command: "desktop-tool"}},
			},
			{
				Tags:         []string{"darwin-only"},
				OS:           []string{"darwin"},
				Dependencies: []manifest.Dependency{{Name: "darwin-tool", Command: "darwin-tool"}},
			},
		},
		Entries: []manifest.Entry{
			{Source: "configs/core", Target: "~/.core", Strategy: "copy", Tags: []string{"core-a"}},
			{Source: "configs/darwin", Target: "~/.darwin", Strategy: "copy", Tags: []string{"darwin-only"}, OS: []string{"darwin"}},
		},
		Provisioners: []manifest.Provisioner{
			{Tool: "zimfw", Tags: []string{"agents-c"}},
			{Tool: "zimfw", Tags: []string{"darwin-only"}, OS: []string{"darwin"}},
		},
	}
	meta := state.Metadata{Provisioners: []state.ProvisionerRecord{
		{
			Profile: "desktop", Profiles: []string{"desktop"}, Tags: []string{"desktop-b"},
			Tool: "zimfw", Executable: "zsh", Args: provisionCommandArgs(t, m.Provisioners[0]), Status: string(provision.RunStatusFailed), LastRunAt: "2026-08-22T12:00:00Z",
		},
		{
			Profile: "workstation", Profiles: []string{"workstation"}, Tags: []string{"agents-c"},
			Tool: "zimfw", Executable: "zsh", Args: provisionCommandArgs(t, m.Provisioners[0]), Status: string(provision.RunStatusFailed), LastRunAt: "2026-08-20T12:00:00Z",
		},
		{
			Profile: "agents", Profiles: []string{"agents"}, Tags: []string{"legacy-agents"},
			Tool: "zimfw", Executable: "zsh", Args: provisionCommandArgs(t, m.Provisioners[0]), Status: string(provision.RunStatusProvisioned), LastRunAt: "2026-08-21T12:00:00Z",
		},
	}}

	got, err := buildInstallTagSelectorBrowseData(m, meta, tagSelectorBrowseOptions{
		OS: "linux", Arch: "amd64", SourceReadRoot: sourceRoot, Home: home, StateRoot: stateRoot,
		XDGStateHome: filepath.Join(home, ".local", "state"),
		Lookup:       func(command string) bool { return command == "desktop-tool" },
		FontLookup:   func(string) bool { return false },
		AppLookup:    func(string) bool { return false },
	})
	if err != nil {
		t.Fatalf("buildInstallTagSelectorBrowseData() error = %v", err)
	}

	var names, groups []string
	byName := map[string]tagselectortui.Tag{}
	for _, tag := range got.Tags {
		names = append(names, tag.Name)
		groups = append(groups, tag.Group)
		byName[tag.Name] = tag
	}
	if want := []string{"shared", "core-a", "desktop-b", "agents-c", "darwin-only", "global"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("Tag order = %v, want %v", names, want)
	}
	if want := []string{"Core", "Core", "Desktop", "Agents", "Global", "Global"}; !reflect.DeepEqual(groups, want) {
		t.Fatalf("Tag groups = %v, want %v", groups, want)
	}
	if _, exists := byName["legacy"]; exists {
		t.Fatal("legacy Tag should be hidden from selector browse data")
	}
	if want := []string{"core", "desktop"}; !reflect.DeepEqual(byName["shared"].Profiles, want) {
		t.Fatalf("shared Profiles = %v, want %v", byName["shared"].Profiles, want)
	}
	if byName["core-a"].State != tagselectortui.StateMissing {
		t.Fatalf("core-a State = %q, want missing", byName["core-a"].State)
	}
	if byName["desktop-b"].State != tagselectortui.StateAligned || !byName["desktop-b"].ExternalEffectsPresent {
		t.Fatalf("desktop-b = %+v, want aligned with retained external evidence", byName["desktop-b"])
	}
	if byName["agents-c"].State != tagselectortui.StateAligned || !byName["agents-c"].ExternalEffectsPresent {
		t.Fatalf("agents-c = %+v, want aligned with retained external evidence", byName["agents-c"])
	}
	if byName["darwin-only"].State != tagselectortui.StateNotApplicable {
		t.Fatalf("darwin-only State = %q, want not-applicable", byName["darwin-only"].State)
	}
	if want := []string{"darwin-tool"}; !reflect.DeepEqual(byName["darwin-only"].Dependencies, want) {
		t.Fatalf("darwin-only Dependencies = %v, want portable details %v", byName["darwin-only"].Dependencies, want)
	}
	if want := []string{"zimfw"}; !reflect.DeepEqual(byName["darwin-only"].Provisioners, want) {
		t.Fatalf("darwin-only Provisioners = %v, want portable details %v", byName["darwin-only"].Provisioners, want)
	}
	for _, want := range []tagselectortui.Component{
		{Kind: "Managed Entry", Name: "~/.darwin", State: tagselectortui.StateNotApplicable, Detail: "not applicable to linux"},
		{Kind: "Dependency", Name: "darwin-tool", State: tagselectortui.StateNotApplicable, Detail: "not applicable to linux"},
		{Kind: "Provisioner", Name: "zimfw", State: tagselectortui.StateNotApplicable, Detail: "not applicable to linux"},
	} {
		if !containsTagSelectorComponent(byName["darwin-only"].Components, want) {
			t.Fatalf("darwin-only Components missing %+v: %+v", want, byName["darwin-only"].Components)
		}
	}
	if byName["global"].State != tagselectortui.StateNotApplicable {
		t.Fatalf("global State = %q, want not-applicable without an applicable surface", byName["global"].State)
	}
	if got.Profiles[0].Name != "core" || got.Profiles[len(got.Profiles)-1].Name != "workstation" {
		t.Fatalf("Profile order = %+v, want conceptual Profiles before remaining Profiles", got.Profiles)
	}
}

func containsTagSelectorComponent(components []tagselectortui.Component, want tagselectortui.Component) bool {
	for _, component := range components {
		if component == want {
			return true
		}
	}
	return false
}

func provisionCommandArgs(t *testing.T, declaration manifest.Provisioner) []string {
	t.Helper()
	_, args := provision.RenderCommand(declaration)
	return args
}

func TestFindTagSelectorProvisionerRecordUsesLatestEvidence(t *testing.T) {
	m := manifest.Manifest{Tags: map[string]manifest.Tag{"core": {Kind: "surface", Status: "current"}}}
	step := provision.Step{Tool: "zimfw", Executable: "zsh", Args: []string{"-c", "zimfw install"}}
	tests := []struct {
		name        string
		records     []state.ProvisionerRecord
		wantProfile string
	}{
		{
			name: "latest valid timestamp",
			records: []state.ProvisionerRecord{
				{Profile: "old", Tags: []string{"core"}, Tool: step.Tool, Executable: step.Executable, Args: step.Args, LastRunAt: "2026-08-20T12:00:00Z"},
				{Profile: "new", Tags: []string{"core"}, Tool: step.Tool, Executable: step.Executable, Args: step.Args, LastRunAt: "2026-08-21T12:00:00Z"},
			},
			wantProfile: "new",
		},
		{
			name: "valid timestamp beats missing timestamp",
			records: []state.ProvisionerRecord{
				{Profile: "dated", Tags: []string{"core"}, Tool: step.Tool, Executable: step.Executable, Args: step.Args, LastRunAt: "2026-08-20T12:00:00Z"},
				{Profile: "undated", Tags: []string{"core"}, Tool: step.Tool, Executable: step.Executable, Args: step.Args},
			},
			wantProfile: "dated",
		},
		{
			name: "later record breaks timestamp tie",
			records: []state.ProvisionerRecord{
				{Profile: "first", Tags: []string{"core"}, Tool: step.Tool, Executable: step.Executable, Args: step.Args},
				{Profile: "last", Tags: []string{"core"}, Tool: step.Tool, Executable: step.Executable, Args: step.Args},
			},
			wantProfile: "last",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := findTagSelectorProvisionerRecord(m, tt.records, "core", step)
			if !ok {
				t.Fatal("findTagSelectorProvisionerRecord() found no record")
			}
			if got.Profile != tt.wantProfile {
				t.Fatalf("Profile = %q, want %q", got.Profile, tt.wantProfile)
			}
		})
	}
}

func TestTagSelectorComponentStateProjection(t *testing.T) {
	t.Run("managed entries", func(t *testing.T) {
		tests := []struct {
			input status.State
			want  tagselectortui.State
		}{
			{status.StateOK, tagselectortui.StateAligned},
			{status.StateMissing, tagselectortui.StateMissing},
			{status.StateDrifted, tagselectortui.StateDrift},
			{status.StateConflict, tagselectortui.StateConflict},
			{status.StateSkipped, tagselectortui.StateNotApplicable},
			{status.StateUnsupported, tagselectortui.StateConflict},
		}
		for _, tt := range tests {
			if got := tagSelectorManagedEntryState(tt.input); got != tt.want {
				t.Fatalf("tagSelectorManagedEntryState(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})

	t.Run("dependencies", func(t *testing.T) {
		tests := []struct {
			input deps.Result
			want  tagselectortui.State
		}{
			{deps.Result{Present: true}, tagselectortui.StateAligned},
			{deps.Result{Present: false}, tagselectortui.StateMissing},
			{deps.Result{Present: true, Warning: "degraded"}, tagselectortui.StateDrift},
		}
		for _, tt := range tests {
			if got := tagSelectorDependencyState(tt.input); got != tt.want {
				t.Fatalf("tagSelectorDependencyState(%+v) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})

	t.Run("provisioners", func(t *testing.T) {
		tests := []struct {
			input provision.StatusState
			want  tagselectortui.State
		}{
			{provision.StatusStateProvisioned, tagselectortui.StateAligned},
			{provision.StatusStatePending, tagselectortui.StateMissing},
			{provision.StatusStateMissingDependencies, tagselectortui.StateMissing},
			{provision.StatusStateFailed, tagselectortui.StateConflict},
		}
		for _, tt := range tests {
			if got := tagSelectorProvisionerState(tt.input); got != tt.want {
				t.Fatalf("tagSelectorProvisionerState(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	})
}

func TestAggregateTagSelectorStateUsesDeterministicPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		components []tagselectortui.Component
		want       tagselectortui.State
	}{
		{name: "no components", want: tagselectortui.StateNotApplicable},
		{name: "not applicable only", components: []tagselectortui.Component{{State: tagselectortui.StateNotApplicable}}, want: tagselectortui.StateNotApplicable},
		{name: "aligned beats excluded", components: []tagselectortui.Component{{State: tagselectortui.StateNotApplicable}, {State: tagselectortui.StateAligned}}, want: tagselectortui.StateAligned},
		{name: "missing beats aligned", components: []tagselectortui.Component{{State: tagselectortui.StateAligned}, {State: tagselectortui.StateMissing}}, want: tagselectortui.StateMissing},
		{name: "drift beats missing", components: []tagselectortui.Component{{State: tagselectortui.StateMissing}, {State: tagselectortui.StateDrift}}, want: tagselectortui.StateDrift},
		{name: "conflict beats drift", components: []tagselectortui.Component{{State: tagselectortui.StateDrift}, {State: tagselectortui.StateConflict}}, want: tagselectortui.StateConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aggregateTagSelectorState(tt.components); got != tt.want {
				t.Fatalf("aggregateTagSelectorState() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveInstallTagSelectorInitialUsesOnlyAuthoritativeIntent(t *testing.T) {
	m := manifest.Manifest{
		Tags: map[string]manifest.Tag{
			"current-a": {Kind: "surface", Status: "current"},
			"current-b": {Kind: "surface", Status: "current"},
			"legacy":    {Kind: "compatibility", Status: "legacy", ReplacedBy: manifest.ReplacementTags{"current-a", "current-b"}},
		},
		Profiles: map[string]manifest.Profile{"core": {Tags: []string{"current-a"}}},
	}

	t.Run("no Installed Selection starts empty", func(t *testing.T) {
		got, err := resolveInstallTagSelectorInitial(m, nil)
		if err != nil {
			t.Fatalf("resolveInstallTagSelectorInitial() error = %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("initial Tags = %v, want empty", got)
		}
	})

	t.Run("declared legacy alias normalizes", func(t *testing.T) {
		installed := &state.InstalledSelection{ExtraTags: []string{"legacy"}, ResolvedTags: []string{"legacy"}}
		got, err := resolveInstallTagSelectorInitial(m, installed)
		if err != nil {
			t.Fatalf("resolveInstallTagSelectorInitial() error = %v", err)
		}
		if want := []string{"current-a", "current-b"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("initial Tags = %v, want %v", got, want)
		}
	})

	t.Run("missing Profile fails closed", func(t *testing.T) {
		installed := &state.InstalledSelection{Profiles: []string{"removed"}, ResolvedTags: []string{"current-a"}}
		if _, err := resolveInstallTagSelectorInitial(m, installed); err == nil {
			t.Fatal("resolveInstallTagSelectorInitial() error = nil, want stale Profile error")
		}
	})

	t.Run("missing extra Tag fails closed", func(t *testing.T) {
		installed := &state.InstalledSelection{ExtraTags: []string{"removed"}, ResolvedTags: []string{"current-a"}}
		if _, err := resolveInstallTagSelectorInitial(m, installed); err == nil {
			t.Fatal("resolveInstallTagSelectorInitial() error = nil, want stale extra Tag error")
		}
	})
}
