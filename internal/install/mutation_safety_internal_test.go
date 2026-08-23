package install

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/state"
)

func TestSourceCaptureAuthoritiesAreDetachedAndDeterministic(t *testing.T) {
	captures := map[SourceCaptureKey]CapturedSource{
		{Target: "/home/.z", Source: "z"}: {Content: []byte("z"), ContentPresent: true, Mode: 0o640, ModePresent: true, IdentityFingerprint: "z-id", IdentityPresent: true},
		{Target: "/home/.a", Source: "b"}: {Content: []byte("b"), ContentPresent: true},
		{Target: "/home/.a", Source: "a"}: {Content: []byte{}, ContentPresent: true},
	}

	got := SourceCaptureAuthorities(captures)
	wantKeys := []SourceCaptureKey{
		{Target: "/home/.a", Source: "a"},
		{Target: "/home/.a", Source: "b"},
		{Target: "/home/.z", Source: "z"},
	}
	for i, want := range wantKeys {
		if got[i].Target != want.Target || got[i].Source != want.Source {
			t.Fatalf("authority[%d] = %s/%s, want %s/%s", i, got[i].Target, got[i].Source, want.Target, want.Source)
		}
	}
	captures[SourceCaptureKey{Target: "/home/.z", Source: "z"}] = CapturedSource{Content: []byte("changed"), ContentPresent: true}
	if string(got[2].Content) != "z" {
		t.Fatalf("authority content changed through source map: %q", got[2].Content)
	}
}

func TestValidateCapturedSourceJoinsAuthorityAndReleaseErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	expected := CapturedSource{IdentityPresent: true, identity: &capturedFileIdentity{}}
	current := CapturedSource{identity: &capturedFileIdentity{file: file}}
	err = validateCapturedSource(expected, current, "test source", func(component string) error {
		return fmt.Errorf("test source changed %s", component)
	})
	if err == nil {
		t.Fatal("validateCapturedSource() error = nil, want joined authority and release failures")
	}
	for _, want := range []string{"captured identity was released", "close captured identity"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("validateCapturedSource() error = %v, want %q", err, want)
		}
	}
}

func TestMetadataCommitTransitionComparesExactInstalledSelection(t *testing.T) {
	tests := []struct {
		name     string
		current  *state.InstalledSelection
		expected *state.InstalledSelection
		wantOK   bool
	}{
		{name: "nil matches nil", wantOK: true},
		{name: "nil differs from explicit empty", expected: &state.InstalledSelection{}},
		{name: "explicit empty differs from nil", current: &state.InstalledSelection{}},
		{
			name:     "provenance differs",
			current:  &state.InstalledSelection{Profiles: []string{"core"}, Provenance: state.Provenance{SourceRevision: "S1"}},
			expected: &state.InstalledSelection{Profiles: []string{"core"}, Provenance: state.Provenance{SourceRevision: "S0"}},
		},
		{
			name:     "nonempty exact match",
			current:  &state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"git"}, ResolvedTags: []string{"core", "git"}},
			expected: &state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"git"}, ResolvedTags: []string{"core", "git"}},
			wantOK:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			sourceRoot := t.TempDir()
			stateRoot := t.TempDir()
			if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: test.current}); err != nil {
				t.Fatal(err)
			}
			commit := MetadataCommit{opts: Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}}
			updated := &state.InstalledSelection{Profiles: []string{"desktop"}, ResolvedTags: []string{"desktop"}}
			err := commit.CommitTransition(test.expected, updated)
			if test.wantOK && err != nil {
				t.Fatalf("CommitTransition() error = %v", err)
			}
			if !test.wantOK && (err == nil || !strings.Contains(err.Error(), "installed selection changed concurrently")) {
				t.Fatalf("CommitTransition() error = %v, want exact CAS rejection", err)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			want := test.current
			if test.wantOK {
				want = updated
			}
			if !reflect.DeepEqual(meta.InstalledSelection, want) {
				t.Fatalf("InstalledSelection = %#v, want %#v", meta.InstalledSelection, want)
			}
		})
	}
}

func TestCapturedCopySourcePreservesExplicitEmptyBytesAndMode(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "empty.conf")
	target := filepath.Join(home, ".empty.conf")
	if err := os.WriteFile(source, []byte{}, 0o640); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "empty.conf", Target: target, Strategy: "copy", Status: plan.StatusCreate}}}
	opts := Options{Home: home, SourceRoot: sourceRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatalf("CaptureManagedSources() error = %v", err)
	}
	releaseCapturedSourcesForTest(t, captures)
	captured := captures[SourceCaptureKey{Target: target, Source: "empty.conf"}]
	if !captured.ContentPresent || captured.Content == nil || len(captured.Content) != 0 || !captured.ModePresent {
		t.Fatalf("captured source = %#v, want explicitly present empty bytes and mode", captured)
	}
	opts.CapturedSources = captures
	if err := ValidateManagedEntries(p, opts); err != nil {
		t.Fatalf("ValidateManagedEntries() error = %v", err)
	}
	if _, err := ApplyManagedEntries(p, opts); err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, []byte{}) {
		t.Fatalf("target content = %q, %v; want explicit empty", got, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("target mode = %o, want 0640", info.Mode().Perm())
	}
}

func TestCapturedCopySourceRejectsContentAndModeChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) error
		want   string
	}{
		{name: "content", mutate: func(path string) error { return os.WriteFile(path, []byte("changed\n"), 0o600) }, want: "changed content"},
		{name: "mode", mutate: func(path string) error { return os.Chmod(path, 0o640) }, want: "changed mode"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			sourceRoot := t.TempDir()
			source := filepath.Join(sourceRoot, "tool.conf")
			target := filepath.Join(home, ".tool.conf")
			if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			p := plan.Plan{Actions: []plan.Action{{Source: "tool.conf", Target: target, Strategy: "copy", Status: plan.StatusCreate}}}
			opts := Options{Home: home, SourceRoot: sourceRoot}
			captures, err := CaptureManagedSources(p, opts)
			if err != nil {
				t.Fatal(err)
			}
			releaseCapturedSourcesForTest(t, captures)
			opts.CapturedSources = captures
			if err := test.mutate(source); err != nil {
				t.Fatal(err)
			}
			if err := ValidateManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateManagedEntries() error = %v, want %q", err, test.want)
			}
			if _, err := ApplyManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ApplyManagedEntries() error = %v, want %q", err, test.want)
			}
			if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("target lstat error = %v, want no mutation", err)
			}
		})
	}
}

func TestReleasedCapturedSourceFailsClosedAndReleaseIsIdempotent(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "tool.conf")
	target := filepath.Join(home, ".tool.conf")
	if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "tool.conf", Target: target, Strategy: "copy", Status: plan.StatusCreate}}}
	opts := Options{Home: home, SourceRoot: sourceRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReleaseCapturedSources(captures); err != nil {
		t.Fatalf("ReleaseCapturedSources() error = %v", err)
	}
	if err := ReleaseCapturedSources(captures); err != nil {
		t.Fatalf("second ReleaseCapturedSources() error = %v, want idempotent release", err)
	}
	opts.CapturedSources = captures
	if err := ValidateManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), "released") {
		t.Fatalf("ValidateManagedEntries() error = %v, want released authority rejection", err)
	}
}

func TestCapturedAdoptRejectsTargetOrSourceSubstitution(t *testing.T) {
	tests := []struct {
		name       string
		substitute func(target, source string) error
		want       string
	}{
		{
			name: "target",
			substitute: func(target, _ string) error {
				if err := os.Remove(target); err != nil {
					return err
				}
				return os.WriteFile(target, []byte("local\n"), 0o640)
			},
			want: "adopt target changed identity",
		},
		{
			name: "source",
			substitute: func(_, source string) error {
				if err := os.Remove(source); err != nil {
					return err
				}
				return os.WriteFile(source, []byte("managed\n"), 0o600)
			},
			want: "adopt source changed identity",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			sourceRoot := t.TempDir()
			source := filepath.Join(sourceRoot, "gitconfig")
			target := filepath.Join(home, ".gitconfig")
			if err := os.WriteFile(source, []byte("managed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("local\n"), 0o640); err != nil {
				t.Fatal(err)
			}
			p := plan.Plan{Actions: []plan.Action{{Source: "gitconfig", Target: target, Strategy: "copy", Status: plan.StatusConflict}}}
			opts := Options{Home: home, SourceRoot: sourceRoot, ConflictDecisions: map[string]ConflictDecision{target: DecisionAdopt}}
			snapshots, err := CaptureAdoptSnapshots(p, opts)
			if err != nil {
				t.Fatalf("CaptureAdoptSnapshots() error = %v", err)
			}
			releaseAdoptSnapshotsForTest(t, snapshots)
			opts.AdoptSnapshots = snapshots
			if err := test.substitute(target, source); err != nil {
				t.Fatal(err)
			}
			if _, err := ApplyManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ApplyManagedEntries() error = %v, want %q", err, test.want)
			}
			got, err := os.ReadFile(source)
			if err != nil || string(got) != "managed\n" {
				t.Fatalf("source = %q, %v; want unchanged managed content", got, err)
			}
		})
	}
}

func TestReleasedAdoptSnapshotFailsClosedAndReleaseIsIdempotent(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "gitconfig")
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(source, []byte("managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("local\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "gitconfig", Target: target, Strategy: "copy", Status: plan.StatusConflict}}}
	opts := Options{Home: home, SourceRoot: sourceRoot, ConflictDecisions: map[string]ConflictDecision{target: DecisionAdopt}}
	snapshots, err := CaptureAdoptSnapshots(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	releaseAdoptSnapshotsForTest(t, snapshots)
	if err := ReleaseAdoptSnapshots(snapshots); err != nil {
		t.Fatalf("ReleaseAdoptSnapshots() error = %v", err)
	}
	if err := ReleaseAdoptSnapshots(snapshots); err != nil {
		t.Fatalf("second ReleaseAdoptSnapshots() error = %v, want idempotent release", err)
	}
	opts.AdoptSnapshots = snapshots
	if err := ValidateManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), "released") {
		t.Fatalf("ValidateManagedEntries() error = %v, want released authority rejection", err)
	}
}

func TestCapturedAdoptWritesReviewedBytesAndRecordsTerminalEvidence(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "gitconfig")
	target := filepath.Join(home, ".gitconfig")
	if err := os.WriteFile(source, []byte("managed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("local\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	action := plan.Action{Source: "gitconfig", Target: target, Strategy: "copy", Status: plan.StatusConflict, Contributions: []plan.Contribution{{Source: "gitconfig"}}}
	p := plan.Plan{Actions: []plan.Action{action}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot, ConflictDecisions: map[string]ConflictDecision{target: DecisionAdopt}}
	snapshots, err := CaptureAdoptSnapshots(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	releaseAdoptSnapshotsForTest(t, snapshots)
	opts.AdoptSnapshots = snapshots
	commit, err := ApplyManagedEntries(p, opts)
	if err != nil {
		t.Fatalf("ApplyManagedEntries() error = %v", err)
	}
	if err := commit.Commit(nil); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != "local\n" {
		t.Fatalf("source = %q, %v; want reviewed adopted bytes", got, err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	record, ok := meta.FindByTarget(target)
	if !ok || record.Hash != state.HashBytes([]byte("local\n")) || len(record.Contributions) != 1 || record.Contributions[0].Hash != state.HashBytes([]byte("local\n")) {
		t.Fatalf("terminal evidence = %#v, want adopted content hashes", record)
	}
}

func TestCapturedSymlinkSourceRejectsEscapeBeforeCreate(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "tool.conf")
	target := filepath.Join(home, ".tool.conf")
	outside := filepath.Join(t.TempDir(), "outside.conf")
	if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "tool.conf", Target: target, Strategy: "symlink", Status: plan.StatusCreate}}}
	opts := Options{Home: home, SourceRoot: sourceRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, source); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyManagedEntries(p, opts); err == nil {
		t.Fatal("ApplyManagedEntries() accepted a captured symlink source escaping Source of Truth")
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target lstat = %v, want absent", err)
	}
}

func TestCapturedSymlinkCompensatesSourceSubstitution(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "tool.conf")
	target := filepath.Join(home, ".tool.conf")
	if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	action := plan.Action{Source: "tool.conf", Target: target, Strategy: "symlink", Status: plan.StatusCreate}
	p := plan.Plan{Actions: []plan.Action{action}}
	opts := Options{Home: home, SourceRoot: sourceRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures
	err = createCapturedSymlinkWithHook(action, source, opts, func() error {
		moved := source + ".reviewed"
		if err := os.Rename(source, moved); err != nil {
			return err
		}
		return os.WriteFile(source, []byte("substituted\n"), 0o600)
	})
	if err == nil || !strings.Contains(err.Error(), "different source identity") {
		t.Fatalf("createCapturedSymlinkWithHook() error = %v, want identity rejection", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target lstat = %v, want compensated link removal", err)
	}
}

func TestCapturedSymlinkCompensationPreservesExternalReplacement(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "tool.conf")
	target := filepath.Join(home, ".tool.conf")
	if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	action := plan.Action{Source: "tool.conf", Target: target, Strategy: "symlink", Status: plan.StatusCreate}
	p := plan.Plan{Actions: []plan.Action{action}}
	opts := Options{Home: home, SourceRoot: sourceRoot}
	captures, err := CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures
	err = createCapturedSymlinkWithHook(action, source, opts, func() error {
		if err := os.Remove(target); err != nil {
			return err
		}
		return os.WriteFile(target, []byte("external\n"), 0o600)
	})
	if err == nil || !strings.Contains(err.Error(), "refusing removal") {
		t.Fatalf("createCapturedSymlinkWithHook() error = %v, want conditional compensation refusal", err)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil || string(got) != "external\n" {
		t.Fatalf("target = %q, %v; want external replacement preserved", got, readErr)
	}
}

func TestOrdinaryCreateFailureDoesNotClaimOwnershipAndExternalEditFailsClosed(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	firstSource := filepath.Join(sourceRoot, "first.conf")
	secondSource := filepath.Join(sourceRoot, "second.conf")
	for path, content := range map[string]string{firstSource: "first\n", secondSource: "second\n"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	firstTarget := filepath.Join(home, ".first.conf")
	secondTarget := filepath.Join(home, ".second.conf")
	p := plan.Plan{Actions: []plan.Action{
		{Source: "first.conf", Target: firstTarget, Strategy: "copy", Status: plan.StatusCreate},
		{Source: "second.conf", Target: secondTarget, Strategy: "copy", Status: plan.StatusCreate},
	}}
	opts := Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}
	injected := errors.New("injected second action failure")
	_, err := applyManagedEntriesWithApply(p, opts, func(action plan.Action, source string, applyOpts Options) (managedActionResult, error) {
		if action.Target == secondTarget {
			return managedActionResult{}, injected
		}
		return applyManagedAction(action, source, applyOpts)
	})
	if !errors.Is(err, injected) {
		t.Fatalf("applyManagedEntriesWithApply() error = %v, want injected failure", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if _, owned := meta.FindByTarget(firstTarget); owned {
		t.Fatalf("metadata = %#v, must not claim ordinary create after later failure", meta)
	}
	if err := os.WriteFile(firstTarget, []byte("external\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("ValidateManagedEntries() error = %v, want stale create rejection", err)
	}
	got, err := os.ReadFile(firstTarget)
	if err != nil || string(got) != "external\n" {
		t.Fatalf("first target = %q, %v; want external edit preserved", got, err)
	}
}

func TestCapturedCopyCreateRejectsParentSymlinkSubstitution(t *testing.T) {
	home := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(home, ".config")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "tool.conf")
	err := writeNewFileWithModeAtRoot(home, target, []byte("reviewed\n"), 0o600, func() error {
		if err := os.Rename(parent, parent+".reviewed"); err != nil {
			return err
		}
		return os.Symlink(outside, parent)
	})
	if err == nil {
		t.Fatal("writeNewFileWithModeAtRoot() accepted a parent symlink escape")
	}
	if _, err := os.Lstat(filepath.Join(outside, "tool.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside target lstat = %v, want no escaped write", err)
	}
}

func TestCapturedSymlinkVerificationRejectsParentEscape(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(home, ".config")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceRoot, "tool.conf")
	if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(parent, "tool.conf")
	action := plan.Action{Source: "tool.conf", Target: target, Strategy: "symlink", Status: plan.StatusCreate}
	opts := Options{Home: home, SourceRoot: sourceRoot}
	captures, err := CaptureManagedSources(plan.Plan{Actions: []plan.Action{action}}, opts)
	if err != nil {
		t.Fatal(err)
	}
	releaseCapturedSourcesForTest(t, captures)
	opts.CapturedSources = captures
	err = createCapturedSymlinkWithHook(action, source, opts, func() error {
		if err := os.Rename(parent, parent+".reviewed"); err != nil {
			return err
		}
		return os.Symlink(outside, parent)
	})
	if err == nil {
		t.Fatal("createCapturedSymlinkWithHook() accepted a parent symlink escape during verification")
	}
	if _, err := os.Lstat(filepath.Join(outside, "tool.conf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside target lstat = %v, want no escaped verification or compensation", err)
	}
}

func releaseCapturedSourcesForTest(t *testing.T, captures map[SourceCaptureKey]CapturedSource) {
	t.Helper()
	t.Cleanup(func() {
		if err := ReleaseCapturedSources(captures); err != nil {
			t.Errorf("ReleaseCapturedSources() error = %v", err)
		}
	})
}

func releaseAdoptSnapshotsForTest(t *testing.T, snapshots map[string]AdoptSnapshot) {
	t.Helper()
	t.Cleanup(func() {
		if err := ReleaseAdoptSnapshots(snapshots); err != nil {
			t.Errorf("ReleaseAdoptSnapshots() error = %v", err)
		}
	})
}

func TestMetadataCommitTransitionPreservesConcurrentSelection(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	stateRoot := t.TempDir()
	s1 := &state.InstalledSelection{Profiles: []string{"S1"}, ResolvedTags: []string{"S1"}}
	s2 := &state.InstalledSelection{Profiles: []string{"S2"}, ResolvedTags: []string{"S2"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: s2}); err != nil {
		t.Fatal(err)
	}
	commit := MetadataCommit{opts: Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot}}
	err := commit.CommitTransition(s1, &state.InstalledSelection{Profiles: []string{"S3"}})
	if err == nil || !strings.Contains(err.Error(), "installed selection changed concurrently") {
		t.Fatalf("CommitTransition() error = %v, want concurrent transition rejection", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta.InstalledSelection, s2) {
		t.Fatalf("InstalledSelection = %#v, want concurrent S2 %#v", meta.InstalledSelection, s2)
	}
}
