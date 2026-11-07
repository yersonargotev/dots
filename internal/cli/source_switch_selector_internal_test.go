package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
)

func TestTagSelectorOwnedSourceSwitchConflictDecisions(t *testing.T) {
	for _, first := range []string{"alpha", "beta"} {
		for _, decision := range []string{"s", "r", "a", "cancel"} {
			t.Run(first+"/"+decision, func(t *testing.T) {
				home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
				t.Setenv("HOME", t.TempDir())
				for name, content := range map[string]string{"alpha.toml": "width=38\n", "beta.toml": "width=26\n"} {
					if err := os.WriteFile(filepath.Join(sourceRoot, name), []byte(content), 0600); err != nil {
						t.Fatal(err)
					}
				}
				manifestPath := filepath.Join(sourceRoot, "dots.yaml")
				body := `version: 1
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
  base:
    tags: [alpha]
entries:
  - source: alpha.toml
    target: ~/.shared.toml
    strategy: copy
    ownership: toml-subset
    tags: [alpha]
  - source: beta.toml
    target: ~/.shared.toml
    strategy: copy
    ownership: toml-subset
    tags: [beta]
`
				next := "alpha"
				if first == "alpha" {
					next = "beta"
				}
				if decision == "cancel" {
					body += "provisioners:\n  - tool: claude\n    tags: [" + next + "]\n    spec:\n      marketplace: example/tools\n    dependencies:\n      - name: claude\n"
					bin := t.TempDir()
					writeSelectorExecutable(t, filepath.Join(bin, "claude"), "#!/bin/sh\nexit 0\n")
					t.Setenv("PATH", bin)
				}
				if err := os.WriteFile(manifestPath, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
				runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, first)
				target := filepath.Join(home, ".shared.toml")
				targetBefore, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				metaBefore, err := os.ReadFile(state.Path(stateRoot))
				if err != nil {
					t.Fatal(err)
				}
				m, err := manifest.LoadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				meta, err := state.Load(state.Path(stateRoot))
				if err != nil {
					t.Fatal(err)
				}
				paths, err := resolvePaths(home, sourceRoot, stateRoot)
				if err != nil {
					t.Fatal(err)
				}
				opts := installTagSelectorOptions{SkipDependencies: true, NoTUI: true, ManifestPath: manifestPath, RequestedStateRoot: stateRoot}
				candidate, err := buildInstallTagSelectorCandidate(*m, meta, paths, sourceRoot, nil, installHostOS, []string{next}, opts)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := candidate.releaseCapturedSources(); err != nil {
						t.Error(err)
					}
				})
				cmd := NewRootCommand()
				cmd.SetContext(context.Background())
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&out)
				input := decision
				if decision == "cancel" {
					input = "r"
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					cmd.SetContext(ctx)
					desired, err := os.ReadFile(filepath.Join(sourceRoot, next+".toml"))
					if err != nil {
						t.Fatal(err)
					}
					writer := &cancelAfterSourceSwitchWriter{out: &out, target: target, desired: desired, cancel: cancel}
					cmd.SetOut(writer)
					cmd.SetErr(writer)
				}
				cmd.SetIn(strings.NewReader(input + "\n"))
				err = applyInstallTagSelectorCandidate(cmd, candidate, opts, true)
				if decision == "cancel" {
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("post-apply cancellation=%v %s", err, out.String())
					}
					after, e := state.Load(state.Path(stateRoot))
					if e != nil {
						t.Fatal(e)
					}
					if after.InstalledSelection.ExtraTags[0] != first || after.Entries[0].Contributions[0].Source != first+".toml" || after.Entries[0].PendingReconciliation == nil {
						t.Fatal("cancellation committed selection/evidence or lost receipt")
					}
					return
				}
				if decision == "a" {
					if err == nil || !strings.Contains(err.Error(), "Skip or Replace") {
						t.Fatalf("adopt accepted: %v %s", err, out.String())
					}
				} else if err != nil {
					t.Fatalf("apply: %v %s", err, out.String())
				}
				got, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				final, err := os.ReadFile(state.Path(stateRoot))
				if err != nil {
					t.Fatal(err)
				}
				if decision != "r" {
					if !bytes.Equal(got, targetBefore) || !bytes.Equal(final, metaBefore) {
						t.Fatal("unresolved selector changed target/metadata")
					}
				} else {
					meta, err := state.Load(state.Path(stateRoot))
					if err != nil {
						t.Fatal(err)
					}
					if meta.InstalledSelection.ExtraTags[0] != next || meta.Entries[0].Contributions[0].Source != next+".toml" {
						t.Fatal("selector did not commit selected contribution")
					}
				}
			})
		}
	}
}

type cancelAfterSourceSwitchWriter struct {
	out     *bytes.Buffer
	target  string
	desired []byte
	cancel  context.CancelFunc
}

func (w *cancelAfterSourceSwitchWriter) Write(b []byte) (int, error) {
	n, err := w.out.Write(b)
	live, readErr := os.ReadFile(w.target)
	if readErr == nil && bytes.Equal(live, w.desired) {
		w.cancel()
	}
	return n, err
}
