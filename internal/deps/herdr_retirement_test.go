package deps

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

type retirementRunner struct{ calls int }

func (r *retirementRunner) Run(string, []string) error {
	r.calls++
	return errors.New("retired runner invoked")
}

type retirementBoundaryRunner struct {
	*UserLocalInstaller
	retirementRunner
}

func TestHerdrRetirementRejectsPreparedAndDirectActionsBeforeIO(t *testing.T) {
	actions := []InstallAction{
		{Dependency: "herdr-custom"}, {Probe: "herdr-custom"}, {Probes: []string{"other", "herdr-custom"}},
		{Executable: "herdr-custom"}, {UserLocal: &UserLocalArtifact{Recipe: "herdr-custom"}},
		{UserLocal: &UserLocalArtifact{Command: "herdr-custom"}},
		{Dependency: "retained", Status: InstallActionStatusInstallable, Executable: "manager", Bootstrap: []Command{{Executable: "herdr-custom"}}},
	}
	for _, a := range actions {
		home, state := filepath.Join(t.TempDir(), "absent-home"), filepath.Join(t.TempDir(), "absent-state")
		i := &UserLocalInstaller{Home: home, StateRoot: state, Download: func(string, string) ([]byte, error) { t.Fatal("download invoked"); return nil, nil }}
		for _, run := range []func() error{
			func() error { return InstallUserLocal(home, a) },
			func() error {
				return i.RunUserLocal(a, func() bool { t.Fatal("presence bypassed retirement"); return true })
			},
			func() error {
				r := &retirementRunner{}
				_, err := InstallPrepared(PreparedInstall{Plan: PlanReport{Actions: []InstallAction{a}}}, Options{}, nil, nil, r)
				if r.calls != 0 {
					t.Fatal("runner called")
				}
				return err
			},
		} {
			if err := run(); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
				t.Fatalf("action %+v: %v", a, err)
			}
		}
		for _, p := range []string{home, state} {
			if _, err := os.Lstat(p); !os.IsNotExist(err) {
				t.Fatalf("denial created %s: %v", p, err)
			}
		}
	}
}

func TestHerdrRetirementRejectsSelectedDependencyIdentity(t *testing.T) {
	for _, d := range []manifest.Dependency{
		{Name: "herdr-custom", Command: "other"}, {Name: "other", Command: "herdr-custom"},
		{Name: "other", Commands: []string{"herdr-custom"}},
		{Name: "other", UserLocal: &manifest.UserLocalProvider{Recipe: "herdr-custom"}},
		{Name: "other", RollingUserLocal: &manifest.RollingUserLocalProvider{Recipe: "herdr-custom"}},
	} {
		m := manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Tags: []string{"kept"}, Dependencies: []manifest.Dependency{d}}}}
		for _, tags := range [][]string{{"kept"}, {"herdr-custom"}} {
			opts := Options{OS: "linux", Arch: "amd64", Selection: &manifest.Selection{Tags: tags}}
			if _, err := Plan(m, opts, func(string) bool { t.Fatal("probe before retirement"); return true }, nil, TierGeneric); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
				t.Fatalf("Plan: %v", err)
			}
			if _, err := Check(m, opts, nil, nil); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
				t.Fatalf("Check: %v", err)
			}
		}
	}
}

// These journals are constructed as historical evidence, never by downloading
// or preparing retired custom release artifacts. The generic capture supplies
// real filesystem identities; all custom bytes below are inert fixture sentinels.
func historicalCustomJournal(t *testing.T, phase string) (UserLocalInstaller, InstallAction) {
	t.Helper()
	i, a, downloads := transactionFixture(t, "starship")
	i.Fault = func(point string) error {
		if point == "after-captured" {
			return errors.New("fixture capture stop")
		}
		return nil
	}
	if err := i.RunUserLocal(a, nil); err == nil {
		t.Fatal("capture did not stop")
	}
	if *downloads != 0 {
		t.Fatal("capture downloaded")
	}
	c := loadTransactionOps(t, i.StateRoot)[0].Captured
	artifact := *a.UserLocal
	artifact.Recipe = "herdr-custom"
	artifact.Command = "herdr-custom"
	artifact.Version = "v0.9.5-custom.1"
	artifact.Layout = userLocalLayoutBundle
	artifact.Artifact = "herdr-custom-0.9.5-custom.1-linux-x86_64.tar.gz"
	artifact.URL = "https://example.invalid/" + artifact.Artifact
	artifact.Destination, artifact.InstalledPath = userLocalPaths(i.Home, artifact.Recipe, artifact.Version, artifact.Layout, artifact.Command)
	a.Dependency = "herdr-custom"
	a.Probe = "herdr-custom"
	a.Probes = []string{"herdr-custom"}
	a.UserLocal = &artifact
	c.Action = a
	c.Selection = artifactSelection(a)
	i.Fault = nil
	s, err := i.session()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if s != nil {
			if err := s.close(); err != nil {
				t.Error(err)
			}
		}
	})
	for _, name := range authorityParents(artifact) {
		if strings.HasSuffix(name, ".lock") {
			c.Parents[name], err = fileIdentity(s.homeLock)
		} else {
			if err = os.MkdirAll(filepath.Join(i.Home, name), 0755); err != nil {
				t.Fatal(err)
			}
			c.Parents[name], err = identityAt(s.home, name)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.RemoveAll(filepath.Join(i.StateRoot, receiptJournal, c.ID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(i.StateRoot, receiptJournal, c.ID), 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, value any) {
		t.Helper()
		if err := i.writeReceipt(s.state, operationPath(c.ID, name), value); err != nil {
			t.Fatal(err)
		}
	}
	write("authority-"+receiptDigest(c), c)
	if phase != "canonical-only" {
		write("captured", c)
	}
	if phase != "captured" && phase != "canonical-only" {
		stage := filepath.Join(".local", ".dots-deps-"+c.ID)
		for _, dir := range []string{stage, filepath.Join(stage, "payload"), filepath.Join(stage, "links")} {
			if err := os.MkdirAll(filepath.Join(i.Home, dir), 0700); err != nil {
				t.Fatal(err)
			}
		}
		launcher := filepath.Join(i.Home, stage, "payload", strings.TrimSuffix(artifact.Artifact, ".tar.gz"), "dots", "herdr-custom")
		if err := os.MkdirAll(filepath.Dir(launcher), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(launcher, []byte("historical inert launcher"), 0755); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(i.Home, stage, "links", "herdr-custom")
		target := filepath.Join(artifact.Destination, userLocalRecipes[artifact.Recipe].binaryPath(artifact.Artifact, artifact.Command))
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		ready := readyReceipt{ID: c.ID, Captured: receiptDigest(c), Units: unitPaths(c), Parents: map[string]PublicationEvidence{}}
		for _, name := range sourceParentPaths(c) {
			ready.Parents[name], err = identityAt(s.home, name)
			if err != nil {
				t.Fatal(err)
			}
		}
		for n, u := range ready.Units {
			ready.Units[n].Evidence, err = s.home.evidence(u.Source, true)
			if err != nil {
				t.Fatal(err)
			}
		}
		write("ready", ready)
		if phase != "ready" {
			count := 1
			if phase == "final" || phase == "complete" {
				count = len(ready.Units)
			}
			for n, u := range ready.Units[:count] {
				if err := os.Rename(filepath.Join(i.Home, u.Source), filepath.Join(i.Home, u.Target)); err != nil {
					t.Fatal(err)
				}
				write(fmt.Sprintf("unit-%d", n), receiptMarker{ID: c.ID, Digest: receiptDigest(u)})
			}
		}
		if phase == "final" || phase == "complete" {
			final := finalReceipt{ID: c.ID, Ready: receiptDigest(ready), Record: recordForOperation(c, ready, "2026-01-01T00:00:00Z")}
			write("final", final)
			if phase == "complete" {
				write("complete", receiptMarker{ID: c.ID, Digest: receiptDigest(final)})
			}
		}
	}
	// Release setup locks before exercising read-only and mutating boundaries.
	if err := s.close(); err != nil {
		t.Fatal(err)
	}
	s = nil
	loadTransactionOps(t, i.StateRoot)
	i.Download = func(string, string) ([]byte, error) { t.Fatal("retired journal downloaded"); return nil, nil }
	return i, a
}

func retirementTree(t *testing.T, root string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		value := info.Mode().String()
		if info.Mode()&os.ModeSymlink != 0 {
			v, err := os.Readlink(p)
			if err != nil {
				return err
			}
			value += v
		} else if !d.IsDir() {
			v, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			value += string(v)
		}
		got[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestHerdrRetirementPreservesPendingJournalsAtEveryPhase(t *testing.T) {
	for _, phase := range []string{"canonical-only", "captured", "ready", "partial-published", "final"} {
		t.Run(phase, func(t *testing.T) {
			i, a := historicalCustomJournal(t, phase)
			homeBefore, stateBefore := retirementTree(t, i.Home), retirementTree(t, i.StateRoot)
			for attempt := 0; attempt < 2; attempt++ {
				meta, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot))
				if err != nil || len(meta.Dependencies) != 0 {
					t.Fatalf("pending load: %+v %v", meta, err)
				}
				if _, err := pendingSelections(i.Home, i.StateRoot, nil); !errors.Is(err, manifest.ErrHerdrCustomRetired) || !strings.Contains(err.Error(), "manual remediation") {
					t.Fatalf("preview: %v", err)
				}
				if _, err := i.RecoverUserLocal(nil); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
					t.Fatalf("empty recovery: %v", err)
				}
				if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
					t.Fatalf("recovery: %v", err)
				}

				retained := InstallAction{Dependency: "starship", Provider: TierUserLocal, Status: InstallActionStatusInstallable, UserLocal: &UserLocalArtifact{Recipe: "starship", Command: "starship"}}
				if err := i.RunUserLocal(retained, nil); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
					t.Fatalf("retained run skipped custom journal: %v", err)
				}
				if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(retained)}); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
					t.Fatalf("retained provider skipped custom journal: %v", err)
				}
				r := &retirementBoundaryRunner{UserLocalInstaller: &i}
				_, err = InstallPrepared(PreparedInstall{}, Options{Home: i.Home, StateRoot: i.StateRoot}, nil, nil, r)
				if !errors.Is(err, manifest.ErrHerdrCustomRetired) || r.calls != 0 {
					t.Fatalf("provider-free prepared install: %v calls=%d", err, r.calls)
				}
				if !reflect.DeepEqual(homeBefore, retirementTree(t, i.Home)) || !reflect.DeepEqual(stateBefore, retirementTree(t, i.StateRoot)) {
					t.Fatal("pending diagnosis/retry changed evidence or external state")
				}
			}
			alias := filepath.Join(t.TempDir(), "home-alias")
			if err := os.Symlink(i.Home, alias); err != nil {
				t.Fatal(err)
			}
			if _, err := pendingSelections(alias, i.StateRoot, nil); !errors.Is(err, manifest.ErrHerdrCustomRetired) {
				t.Fatalf("home alias bypassed retirement: %v", err)
			}
			// Another selected home sharing this state root is outside this operation.
			if _, err := pendingSelections(t.TempDir(), i.StateRoot, nil); err != nil {
				t.Fatalf("unrelated home blocked: %v", err)
			}
		})
	}
}

func TestHerdrRetirementLoadsCompletedReceiptsWithoutRecovery(t *testing.T) {
	i, _ := historicalCustomJournal(t, "complete")
	before := retirementTree(t, i.StateRoot)
	meta, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot))
	if err != nil || len(meta.Dependencies) != 1 || meta.Dependencies[0].Dependency != "herdr-custom" {
		t.Fatalf("completed: %+v %v", meta, err)
	}
	if _, err := i.RecoverUserLocal(nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, retirementTree(t, i.StateRoot)) {
		t.Fatal("completed read rewrote history")
	}
}
