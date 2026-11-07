package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

type sourceSwitchFixture struct{ home, sourceRoot, stateRoot, manifest, target string }

func newSourceSwitchFixture(t *testing.T, first string) sourceSwitchFixture {
	t.Helper()
	f := sourceSwitchFixture{home: t.TempDir(), sourceRoot: t.TempDir(), stateRoot: t.TempDir()}
	t.Setenv("HOME", t.TempDir())
	writeCLISource(t, f.sourceRoot, "alpha.toml", "[ui]\nwidth = 38\n")
	writeCLISource(t, f.sourceRoot, "beta.toml", "[ui]\nwidth = 26\n")
	f.manifest = writeCLIManifest(t, f.home, `version: 1
tags:
  alpha:
    kind: surface
    status: current
    description: Alpha
    conflicts_with: [beta]
  beta:
    kind: surface
    status: current
    description: Beta
    conflicts_with: [alpha]
profiles:
  alpha:
    tags: [alpha]
  beta:
    tags: [beta]
entries:
  - source: alpha.toml
    target: ~/.config/sample/config.toml
    strategy: copy
    ownership: toml-subset
    tags: [alpha]
  - source: beta.toml
    target: ~/.config/sample/config.toml
    strategy: copy
    ownership: toml-subset
    tags: [beta]
`)
	f.target = filepath.Join(f.home, ".config/sample/config.toml")
	runInstallForRetirementTest(t, f.manifest, f.home, f.sourceRoot, f.stateRoot, first)
	return f
}
func (f sourceSwitchFixture) run(t *testing.T, command string, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	all := append([]string{command, "--file", f.manifest, "--home", f.home, "--source-root", f.sourceRoot, "--state-root", f.stateRoot}, args...)
	code := cli.Run(all, &out, &errOut)
	return code, out.String() + errOut.String()
}
func switchRead(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func switchWrite(t *testing.T, path string, b []byte) {
	t.Helper()
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
}

func TestInstallConflictingTOMLSourceSwitch(t *testing.T) {
	for _, first := range []string{"alpha", "beta"} {
		for _, mode := range []string{"exact", "edited", "desired", "skip"} {
			t.Run(first+"/"+mode, func(t *testing.T) {
				next := "alpha"
				if first == "alpha" {
					next = "beta"
				}
				f := newSourceSwitchFixture(t, first)
				before := switchRead(t, f.target)
				if mode == "edited" || mode == "skip" {
					before = bytes.ReplaceAll(before, []byte("width = 38"), []byte("width = 99"))
					before = bytes.ReplaceAll(before, []byte("width = 26"), []byte("width = 99"))
				}
				if mode == "desired" {
					before = switchRead(t, filepath.Join(f.sourceRoot, next+".toml"))
				}
				external := []byte("\n# personal note\n[personal]\nkeep = \"untouched\" # exact spacing\n")
				before = append(before, external...)
				switchWrite(t, f.target, before)
				metaBefore := switchRead(t, state.Path(f.stateRoot))
				args := []string{"--profile", next, "--yes", "--skip-deps", "--acknowledge-selection-change", "--output", "json"}
				code, out := f.run(t, "install", args...)
				if code != 0 {
					t.Fatalf("Skip did not reach Conflict Resolution: %d %s", code, out)
				}
				if !bytes.Equal(switchRead(t, f.target), before) || !bytes.Equal(switchRead(t, state.Path(f.stateRoot)), metaBefore) {
					t.Fatal("Skip changed target or prior authoritative metadata")
				}
				if !strings.Contains(out, "owned-source-conflict") || !strings.Contains(out, `"status": "conflict"`) {
					t.Fatalf("unresolved conflict missing: %s", out)
				}
				if mode == "skip" {
					return
				}
				code, out = f.run(t, "install", append(args, "--backup-and-replace")...)
				if code != 0 {
					t.Fatalf("Replace failed: %d %s", code, out)
				}
				got := switchRead(t, f.target)
				if !bytes.Contains(got, external) || !bytes.Contains(got, []byte(map[string]string{"alpha": "width = 38", "beta": "width = 26"}[next])) {
					t.Fatalf("incorrect target: %s", got)
				}
				retired := map[string]string{"alpha": "old = true", "beta": "next = true"}[first]
				if bytes.Contains(got, []byte(retired)) {
					t.Fatalf("retired owned path remains: %s", got)
				}
				meta, e := state.Load(state.Path(f.stateRoot))
				if e != nil {
					t.Fatal(e)
				}
				rec, ok := meta.FindByTarget(f.target)
				if !ok || len(rec.Contributions) != 1 || rec.Contributions[0].Source != next+".toml" || rec.PendingReconciliation != nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{next}) {
					t.Fatalf("incorrect committed metadata: %#v", meta)
				}
				bm, e := backups.Load(backups.Path(f.stateRoot))
				if e != nil {
					t.Fatal(e)
				}
				if len(bm.Sets) != 1 {
					t.Fatalf("Backup Sets = %d", len(bm.Sets))
				}
				b := switchRead(t, backups.FilePath(f.stateRoot, bm.Sets[0].ID, 1, f.target))
				if !bytes.Equal(b, before) {
					t.Fatalf("backup = %q want exact preimage %q", b, before)
				}
			})
		}
	}
}

func TestInstallConflictingTOMLSourceSwitchMissingEvidence(t *testing.T) {
	for _, first := range []string{"alpha", "beta"} {
		for _, kind := range []string{"record", "contributions", "owned-bytes", "legacy", "contradictory", "hash", "target-symlink", "target-directory", "target-missing"} {
			t.Run(first+"/"+kind, func(t *testing.T) {
				f := newSourceSwitchFixture(t, first)
				next := "alpha"
				if first == "alpha" {
					next = "beta"
				}
				meta, e := state.Load(state.Path(f.stateRoot))
				if e != nil {
					t.Fatal(e)
				}
				switch kind {
				case "record":
					meta.Entries = nil
				case "contributions", "legacy":
					meta.Entries[0].Contributions = nil
				case "owned-bytes":
					meta.Entries[0].Contributions[0].OwnedBytes = nil
					meta.Entries[0].OwnedBytes = nil
				case "contradictory":
					meta.Entries[0].OwnedBytes = []byte("wrong = true\n")
				case "hash":
					meta.Entries[0].Hash = "wrong"
					meta.Entries[0].Contributions[0].Hash = "wrong"
				case "target-symlink", "target-directory", "target-missing":
					if e := os.Remove(f.target); e != nil {
						t.Fatal(e)
					}
					if kind == "target-symlink" {
						if e := os.Symlink(filepath.Join(f.sourceRoot, first+".toml"), f.target); e != nil {
							t.Fatal(e)
						}
					}
					if kind == "target-directory" {
						if e := os.Mkdir(f.target, 0700); e != nil {
							t.Fatal(e)
						}
					}
				}
				if e := state.Save(state.Path(f.stateRoot), meta); e != nil {
					t.Fatal(e)
				}
				before := switchRead(t, state.Path(f.stateRoot))
				code, out := f.run(t, "install", "--profile", next, "--yes", "--skip-deps", "--acknowledge-selection-change", "--backup-and-replace")
				if code != 1 {
					t.Fatalf("unsafe %s allowed: %d %s", kind, code, out)
				}
				if !bytes.Equal(before, switchRead(t, state.Path(f.stateRoot))) {
					t.Fatal("blocked apply changed metadata")
				}
			})
		}
	}
}

func TestConflictingTOMLSourceSwitchPlanningIsReadOnly(t *testing.T) {
	f := newSourceSwitchFixture(t, "alpha")
	before := switchRead(t, f.target)
	meta := switchRead(t, state.Path(f.stateRoot))
	for _, command := range []string{"plan", "install"} {
		args := []string{"--profile", "beta", "--output", "json"}
		want := 2
		if command == "install" {
			args = append(args, "--dry-run", "--skip-deps")
			want = 0
		}
		code, out := f.run(t, command, args...)
		if code != want || !strings.Contains(out, `"reason": "owned-source-conflict"`) {
			t.Fatalf("%s: %d %s", command, code, out)
		}
		if !bytes.Equal(before, switchRead(t, f.target)) || !bytes.Equal(meta, switchRead(t, state.Path(f.stateRoot))) {
			t.Fatal("preview mutated state")
		}
	}

}

func TestConflictingTOMLSourceSwitchReportsAgree(t *testing.T) {
	f := newSourceSwitchFixture(t, "alpha")
	var expected any
	for _, command := range []string{"plan", "dry-run", "apply"} {
		verb := "install"
		args := []string{"--profile", "beta", "--output", "json"}
		if command == "plan" {
			verb = "plan"
		} else {
			args = append(args, "--skip-deps", "--yes", "--acknowledge-selection-change")
			if command == "dry-run" {
				args = append(args, "--dry-run")
			} else {
				args = append(args, "--backup-and-replace")
			}
		}
		code, out := f.run(t, verb, args...)
		want := 0
		if command == "plan" {
			want = 2
		}
		if code != want {
			t.Fatalf("%s %d %s", command, code, out)
		}
		var envelope struct{ Data map[string]any }
		if err := json.Unmarshal([]byte(out), &envelope); err != nil {
			t.Fatal(err)
		}
		data := envelope.Data
		if command != "plan" {
			data = data["plan"].(map[string]any)
		}
		if expected == nil {
			expected = data["selection_reconciliation"]
		} else if !reflect.DeepEqual(expected, data["selection_reconciliation"]) {
			t.Fatalf("%s report differs", command)
		}
	}
}

func TestConflictingTOMLSourceSwitchFailureAndReceiptRetry(t *testing.T) {
	for _, first := range []string{"alpha", "beta"} {
		for _, receipt := range []string{"matching", "missing", "mismatched"} {
			t.Run(first+"/"+receipt, func(t *testing.T) {
				f := newSourceSwitchFixture(t, first)
				next := "alpha"
				if first == "alpha" {
					next = "beta"
				}
				manifest := string(switchRead(t, f.manifest)) + `provisioners:
  - tool: claude
    tags: [` + next + `]
    spec:
      marketplace: example/tools
    dependencies:
      - name: claude
`
				switchWrite(t, f.manifest, []byte(manifest))
				bin := t.TempDir()
				stub := filepath.Join(bin, "claude")
				writeExecStub(t, stub, "#!/bin/sh\nexit 17\n")
				t.Setenv("PATH", bin)
				args := []string{"--profile", next, "--yes", "--skip-deps", "--acknowledge-selection-change", "--output", "json", "--backup-and-replace"}
				code, out := f.run(t, "install", args...)
				if code != 1 {
					t.Fatalf("failure=%d %s", code, out)
				}
				meta, err := state.Load(state.Path(f.stateRoot))
				if err != nil {
					t.Fatal(err)
				}
				rec := meta.Entries[0]
				if rec.PendingReconciliation == nil || rec.Contributions[0].Source != first+".toml" || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{first}) {
					t.Fatalf("partial run destroyed authority: %#v", meta)
				}
				if receipt == "missing" {
					meta.Entries[0].PendingReconciliation = nil
				}
				if receipt == "mismatched" {
					meta.Entries[0].PendingReconciliation.TargetHash = "not-the-result"
				}
				if err := state.Save(state.Path(f.stateRoot), meta); err != nil {
					t.Fatal(err)
				}
				writeExecStub(t, stub, "#!/bin/sh\nexit 0\n")
				// A rerun WITHOUT Replace must succeed only for an exact receipt.
				code, out = f.run(t, "install", args[:len(args)-1]...)
				if code != 0 {
					t.Fatalf("retry=%d %s", code, out)
				}
				final, err := state.Load(state.Path(f.stateRoot))
				if err != nil {
					t.Fatal(err)
				}
				want := first
				if receipt == "matching" {
					want = next
				}
				if !reflect.DeepEqual(final.InstalledSelection.Profiles, []string{want}) || final.Entries[0].Contributions[0].Source != want+".toml" {
					t.Fatalf("retry fabricated authority (%s): %#v", receipt, final)
				}
				if receipt == "matching" && final.Entries[0].PendingReconciliation != nil {
					t.Fatal("receipt not cleared")
				}
			})
		}
	}
}

func TestConflictingTOMLSourceSwitchInteractiveAuthority(t *testing.T) {
	for _, mode := range []string{"decline", "skip", "replace", "input-error", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			f := newSourceSwitchFixture(t, "alpha")
			before := switchRead(t, f.target)
			metadata := switchRead(t, state.Path(f.stateRoot))
			cmd := cli.NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			input := "y\ns\n"
			if mode == "decline" {
				input = "n\n"
			}
			if mode == "replace" {
				input = "y\nr\ny\n"
			}
			if mode == "input-error" {
				input = "y\n"
			}
			cmd.SetIn(&sourceSwitchPromptInput{text: input})
			if mode == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				cmd.SetContext(ctx)
			}
			cmd.SetArgs([]string{"install", "--profile", "beta", "--no-tui", "--skip-deps", "--file", f.manifest, "--home", f.home, "--source-root", f.sourceRoot, "--state-root", f.stateRoot})
			err := cmd.Execute()
			if mode == "replace" {
				if err != nil {
					t.Fatalf("replace: %v %s", err, out.String())
				}
				if bytes.Equal(before, switchRead(t, f.target)) {
					t.Fatal("Replace did not apply")
				}
				return
			}
			if mode == "input-error" && err == nil {
				t.Fatalf("input error accepted: %s", out.String())
			}
			meta, e := state.Load(state.Path(f.stateRoot))
			if e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"alpha"}) {
				t.Fatal("non-success replaced selection")
			}
			if mode != "canceled" && (!bytes.Equal(before, switchRead(t, f.target)) || !bytes.Equal(metadata, switchRead(t, state.Path(f.stateRoot)))) {
				t.Fatal("non-apply changed state")
			}
		})
	}
}

func TestConflictingTOMLSourceSwitchReportGoldens(t *testing.T) {
	f := newSourceSwitchFixture(t, "alpha")
	for _, format := range []string{"text", "json"} {
		args := []string{"--profile", "beta"}
		if format == "json" {
			args = append(args, "--output", "json")
		}
		code, out := f.run(t, "plan", args...)
		if code != 2 {
			t.Fatalf("%d %s", code, out)
		}
		out = strings.ReplaceAll(out, f.home, "<home>")
		path := filepath.Join("testdata", "source_switch_"+format+".golden")
		if os.Getenv("DOTS_593_GENERATE_GOLDENS") == "1" {
			switchWrite(t, path, []byte(out))
		}
		want := switchRead(t, path)
		if string(want) != out {
			t.Fatalf("%s golden differs:\n%s", format, out)
		}
	}
}

// A terminal supplies each response after the previous prompt. Keep that
// boundary when distinct prompt readers buffer input.
type sourceSwitchPromptInput struct{ text string }

func (r *sourceSwitchPromptInput) Read(p []byte) (int, error) {
	if r.text == "" {
		return 0, io.EOF
	}
	end := strings.IndexByte(r.text, '\n') + 1
	if end == 0 {
		end = len(r.text)
	}
	n := copy(p, r.text[:end])
	r.text = r.text[n:]
	return n, nil
}
