package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

type herdrRetirementFixture struct {
	home, source, state, file string
	before                    state.Metadata
	files                     map[string][]byte
}

func newHerdrRetirementFixture(t *testing.T, evidence string) herdrRetirementFixture {
	t.Helper()
	f := herdrRetirementFixture{home: t.TempDir(), source: t.TempDir(), state: t.TempDir(), files: map[string][]byte{}}
	f.state = filepath.Join(f.home, ".local", "state", "dots")
	f.file = filepath.Join(f.source, "dots.yaml")
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	for _, name := range []string{"herdr", "herdr-custom", "npx", "brew", "apt", "apt-get", "dnf", "pacman", "sudo", "git", "curl"} {
		writeHerdrIntegrationStub(t, filepath.Join(bin, name), "#!/bin/sh\nexit 97\n")
	}
	write := func(root, name, body string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Only the read-only repository-presence probe is permitted. A stale
	// selection must stop update before fetch or any other Git operation.
	writeHerdrIntegrationStub(t, filepath.Join(bin, "git"), "#!/bin/sh\nif [ \"$1\" = rev-parse ] && [ \"$2\" = --is-inside-work-tree ]; then echo true; exit 0; fi\nexit 97\n")
	write(f.source, "configs/base", "retained\n")
	write(f.source, "configs/custom.toml", "[ui]\nsidebar_width = 38\n")
	write(f.source, "configs/tabby.toml", "[tabby]\nenabled = true\n")
	write(f.source, "configs/official.toml", "[ui]\nsidebar_width = 26\n")
	// A temporary neutral selector creates exact ownership evidence using the
	// real installer. Then seed the historical name; never execute custom code.
	const historical = `version: 1
tags:
  base: {kind: surface, status: current}
  herdr: {kind: surface, status: current}
  fixture-custom: {kind: surface, status: current}
profiles:
  kept: {tags: [base, herdr]}
entries:
  - {source: configs/base, target: ~/.base, strategy: copy, tags: [base]}
  - {source: configs/official.toml, target: ~/.config/herdr/config.toml, strategy: copy, ownership: toml-subset, tags: [herdr]}
  - {source: configs/custom.toml, target: ~/.config/herdr/config.toml, strategy: copy, ownership: toml-subset, tags: [fixture-custom]}
  - {source: configs/tabby.toml, target: ~/.config/herdr/plugins/config/tabby.toml, strategy: copy, ownership: toml-subset, tags: [fixture-custom]}
`
	write(f.source, "dots.yaml", historical)
	if _, err := f.run(context.Background(), "", "install", "--profile", "kept", "--exclude-tag", "herdr", "--tag", "fixture-custom", "--yes", "--skip-deps"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(state.Path(f.state))
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte("fixture-custom"), []byte("herdr-custom"))
	if err := os.WriteFile(state.Path(f.state), data, 0600); err != nil {
		t.Fatal(err)
	}
	meta, err := state.Load(state.Path(f.state))
	if err != nil {
		t.Fatal(err)
	}
	meta.Provisioners = []state.ProvisionerRecord{{Tool: "herdr-custom", Executable: "herdr-custom", Tags: []string{"herdr-custom"}, Args: []string{"plugin", "install", "fixture/plugin", "--ref", "historical", "--yes"}, Status: "provisioned"}}
	if evidence == "unproven" {
		for i := range meta.Entries {
			if strings.Contains(meta.Entries[i].Target, "herdr") {
				meta.Entries[i].Contributions = nil
				meta.Entries[i].OwnedBytes = nil
				meta.Entries[i].Ownership = ""
			}
		}
	}
	if err := state.Save(state.Path(f.state), meta); err != nil {
		t.Fatal(err)
	}
	current := strings.ReplaceAll(historical, "  fixture-custom: {kind: surface, status: current}\n", "")
	lines := strings.Split(current, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if !strings.Contains(line, "tags: [fixture-custom]") {
			kept = append(kept, line)
		}
	}
	write(f.source, "dots.yaml", strings.Join(kept, "\n"))
	for _, name := range []string{"configs/custom.toml", "configs/tabby.toml"} {
		if err := os.Remove(filepath.Join(f.source, name)); err != nil {
			t.Fatal(err)
		}
	}
	if evidence == "modified" {
		write(f.home, ".config/herdr/config.toml", "# user changes\n[ui]\nsidebar_width = 57\n[user]\nkeep = true\n")
	}
	for name, body := range map[string]string{".local/bin/herdr-custom": "custom launcher", ".local/opt/herdr-custom/v0.9.5-custom.1/runtime": "custom bundle", ".local/share/herdr/session": "live session", ".config/herdr/history": "history", ".config/herdr/plugins/registry": "plugin effects"} {
		write(f.home, name, body)
	}
	if err := deps.SaveDependencyMetadata(deps.DependencyMetadataPath(f.state), deps.DependencyMetadata{Version: 1, Dependencies: []deps.DependencyRecord{{Dependency: "herdr-custom", Provider: "user-local", Path: filepath.Join(f.home, ".local/bin/herdr-custom"), InstalledAt: "historical"}}}); err != nil {
		t.Fatal(err)
	}
	f.before, err = state.Load(state.Path(f.state))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".config/herdr/config.toml", ".config/herdr/plugins/config/tabby.toml", ".local/bin/herdr-custom", ".local/opt/herdr-custom/v0.9.5-custom.1/runtime", ".local/share/herdr/session", ".config/herdr/history", ".config/herdr/plugins/registry"} {
		p := filepath.Join(f.home, name)
		f.files[p], err = os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
	}
	p := deps.DependencyMetadataPath(f.state)
	f.files[p], err = os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f herdrRetirementFixture) run(ctx context.Context, input string, args ...string) (string, error) {
	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetContext(ctx)
	common := []string{"--file", f.file, "--home", f.home}
	if args[0] != "deps" {
		common = append(common, "--source-root", f.source, "--state-root", f.state)
	}
	cmd.SetArgs(append(args, common...))
	err := cmd.Execute()
	return out.String(), err
}
func (f herdrRetirementFixture) preserved(t *testing.T) {
	t.Helper()
	for p, want := range f.files {
		got, err := os.ReadFile(p)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("preservation %s: %q %v", p, got, err)
		}
	}
}
func (f herdrRetirementFixture) oldSelection(t *testing.T) {
	t.Helper()
	m, err := state.Load(state.Path(f.state))
	if err != nil || !reflect.DeepEqual(m.InstalledSelection, f.before.InstalledSelection) {
		t.Fatalf("selection replaced prematurely: %+v %v", m.InstalledSelection, err)
	}
}

func TestHerdrRetirementHistoricalDiagnosticsAndRetainedTransition(t *testing.T) {
	for _, evidence := range []string{"exact", "modified", "unproven"} {
		t.Run(evidence, func(t *testing.T) {
			f := newHerdrRetirementFixture(t, evidence)
			before, err := os.ReadFile(state.Path(f.state))
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range [][]string{{"plan"}, {"status"}, {"doctor"}, {"deps", "check"}, {"deps", "plan"}, {"update", "--dry-run"}} {
				for _, format := range [][]string{nil, {"--output", "json"}} {
					args := append(append([]string(nil), command...), format...)
					out, err := f.run(context.Background(), "", args...)
					if !errors.Is(err, manifest.ErrHerdrCustomRetired) {
						t.Fatalf("%v: %v %s", args, err, out)
					}
					f.preserved(t)
					f.oldSelection(t)
					after, err := os.ReadFile(state.Path(f.state))
					if err != nil || !bytes.Equal(before, after) {
						t.Fatal("read/error path rewrote metadata")
					}
				}
			}
			retained := []string{"--profile", "kept", "--exclude-tag", "herdr"}
			out, err := f.run(context.Background(), "", append([]string{"plan", "--output", "json"}, retained...)...)
			if err != nil {
				t.Fatalf("retained plan: %v %s", err, out)
			}
			for _, want := range []string{"manifest-evolution-report-only", "retained-external-state", "herdr-custom"} {
				if !strings.Contains(out, want) {
					t.Fatalf("report missing %s: %s", want, out)
				}
			}
			if strings.Contains(out, `"effective_tags": [`+"\n"+`      "herdr"`) {
				t.Fatal("official selected")
			}
			if _, err := f.run(context.Background(), "", append([]string{"install", "--dry-run", "--skip-deps"}, retained...)...); err != nil {
				t.Fatal(err)
			}
			if _, err := f.run(context.Background(), "", append([]string{"install", "--yes", "--skip-deps"}, retained...)...); err == nil || !strings.Contains(err.Error(), "selection-change-acknowledgement-required") {
				t.Fatalf("missing acknowledgement: %v", err)
			}
			if out, err := f.run(context.Background(), "n\n", append([]string{"install", "--skip-deps"}, retained...)...); err != nil || !strings.Contains(out, "declined") {
				t.Fatalf("decline: %v %s", err, out)
			}
			f.oldSelection(t)
			f.preserved(t)
			original := commitInstallationMetadata
			commitInstallationMetadata = func(install.MetadataCommit, *state.InstalledSelection, state.InstalledSelection) error {
				return errors.New("injected terminal failure")
			}
			_, err = f.run(context.Background(), "", append([]string{"install", "--yes", "--acknowledge-selection-change", "--skip-deps"}, retained...)...)
			commitInstallationMetadata = original
			if err == nil || !strings.Contains(err.Error(), "injected terminal failure") {
				t.Fatalf("terminal failure: %v", err)
			}
			f.oldSelection(t)
			f.preserved(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := f.run(ctx, "", append([]string{"install", "--yes", "--acknowledge-selection-change", "--skip-deps"}, retained...)...); err == nil {
				t.Fatal("cancellation accepted")
			}
			f.oldSelection(t)
			f.preserved(t)
			for attempt := 0; attempt < 2; attempt++ {
				if out, err := f.run(context.Background(), "", append([]string{"install", "--yes", "--acknowledge-selection-change", "--skip-deps"}, retained...)...); err != nil {
					t.Fatalf("transition: %v %s", err, out)
				}
				f.preserved(t)
				m, err := state.Load(state.Path(f.state))
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(m.InstalledSelection.Profiles, []string{"kept"}) || len(m.InstalledSelection.ExtraTags) != 0 || !reflect.DeepEqual(m.InstalledSelection.ExcludedTags, []string{"herdr"}) || !reflect.DeepEqual(m.InstalledSelection.ResolvedTags, []string{"base"}) {
					t.Fatalf("silent substitution/intent loss: %+v", m.InstalledSelection)
				}
				for _, old := range f.before.Entries {
					if strings.Contains(old.Target, "herdr") {
						got, ok := m.FindByTarget(old.Target)
						if !ok || !reflect.DeepEqual(old, got) {
							t.Fatalf("historical evidence changed: %+v", got)
						}
					}
				}
				if !reflect.DeepEqual(m.Provisioners, f.before.Provisioners) {
					t.Fatal("historical provisioner receipt changed")
				}
			}
		})
	}
}

func TestHerdrRetirementRawInstallPlanSelectionRejected(t *testing.T) {
	_, err := plan.Build(manifest.Manifest{}, plan.Options{Selection: &manifest.Selection{Tags: []string{"herdr-custom"}}, Home: t.TempDir(), SourceRoot: t.TempDir()})
	if !errors.Is(err, manifest.ErrHerdrCustomRetired) {
		t.Fatal(err)
	}
}

func TestHerdrRetirementRepositoryCompletion(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := runCompletion(t, "install", "--file", filepath.Join(root, "dots.yaml"), "--tag", "herdr")
	if strings.Contains(out, "herdr-custom") || !strings.Contains(out, "herdr-metrics") {
		t.Fatalf("completion: %s", out)
	}
}

func TestHerdrRetirementDoesNotInventExternalDependencyEvidence(t *testing.T) {
	f := newHerdrRetirementFixture(t, "exact")
	if err := os.Remove(deps.DependencyMetadataPath(f.state)); err != nil {
		t.Fatal(err)
	}
	out, err := f.run(context.Background(), "", "plan", "--profile", "kept", "--exclude-tag", "herdr", "--output", "json")
	if err != nil {
		t.Fatalf("plan: %v %s", err, out)
	}
	if strings.Contains(out, `"scope": "dependency"`) {
		t.Fatalf("selection alone invented dependency installation: %s", out)
	}
}

func TestHerdrRetirementCompletionRejectsOldTagOnlyManifest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "dots.yaml")
	old := `version: 1
tags:
  herdr-custom: {kind: surface, status: current}
profiles:
  old: {tags: [herdr-custom]}
entries: []
`
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	out, errOut := runCompletion(t, "install", "--file", path, "--tag", "herdr")
	assertCompletion(t, out, errOut, nil, []string{"herdr-custom"})
}
