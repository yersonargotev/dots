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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
	"github.com/yersonargotev/dots/internal/status"
	tagselectortui "github.com/yersonargotev/dots/internal/tui/tagselector"
)

func TestInstallTagSelectorCandidateStoreRejectsLatePreviewAndTransfersAcceptedLease(t *testing.T) {
	home := t.TempDir()
	sourceRoot := t.TempDir()
	source := filepath.Join(sourceRoot, "tool.conf")
	target := filepath.Join(home, ".tool.conf")
	if err := os.WriteFile(source, []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := plan.Plan{Actions: []plan.Action{{Source: "tool.conf", Target: target, Strategy: "copy", Status: plan.StatusCreate}}}
	opts := install.Options{Home: home, SourceRoot: sourceRoot}
	capturesA, err := install.CaptureManagedSources(p, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := install.ReleaseCapturedSources(capturesA); err != nil {
			t.Errorf("release first captures: %v", err)
		}
	})
	store := newInstallTagSelectorCandidateStore()
	t.Cleanup(func() {
		if err := store.close(); err != nil {
			t.Errorf("candidate store cleanup: %v", err)
		}
	})
	tokenA, ok, err := store.activate(1)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("candidate store rejected first preview")
	}
	candidateA := installTagSelectorCandidate{CapturedSources: capturesA, Preview: tagselectortui.Preview{SemanticDigest: "sha256:same", CandidateToken: tokenA}}
	stored, err := store.put(1, tokenA, candidateA)
	if err != nil {
		t.Fatal(err)
	}
	if !stored {
		t.Fatal("candidate store rejected current first preview")
	}

	tokenB, ok, err := store.activate(2)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("candidate store rejected second preview")
	}
	opts.CapturedSources = capturesA
	if err := install.ValidateManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), "released") {
		t.Fatalf("superseded candidate error = %v, want released authority", err)
	}

	capturesB, err := install.CaptureManagedSources(p, install.Options{Home: home, SourceRoot: sourceRoot})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := install.ReleaseCapturedSources(capturesB); err != nil {
			t.Errorf("release second captures: %v", err)
		}
	})
	candidateB := installTagSelectorCandidate{CapturedSources: capturesB, Preview: tagselectortui.Preview{SemanticDigest: "sha256:same", CandidateToken: tokenB}}
	stored, err = store.put(2, tokenB, candidateB)
	if err != nil {
		t.Fatal(err)
	}
	if !stored {
		t.Fatal("candidate store rejected current second preview")
	}

	lateCaptures, err := install.CaptureManagedSources(p, install.Options{Home: home, SourceRoot: sourceRoot})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := install.ReleaseCapturedSources(lateCaptures); err != nil {
			t.Errorf("release late captures: %v", err)
		}
	})
	lateA := installTagSelectorCandidate{CapturedSources: lateCaptures, Preview: tagselectortui.Preview{SemanticDigest: "sha256:same", CandidateToken: tokenA}}
	stored, err = store.put(1, tokenA, lateA)
	if err != nil {
		t.Fatal(err)
	}
	if stored {
		t.Fatal("candidate store accepted stale first preview after second preview")
	}
	opts.CapturedSources = lateCaptures
	if err := install.ValidateManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), "released") {
		t.Fatalf("late candidate error = %v, want released authority", err)
	}

	leased, ok := store.take(tokenB)
	if !ok {
		t.Fatal("candidate store did not transfer accepted second preview")
	}
	if err := store.close(); err != nil {
		t.Fatal(err)
	}
	opts.CapturedSources = leased.CapturedSources
	if err := install.ValidateManagedEntries(p, opts); err != nil {
		t.Fatalf("accepted lease was closed by store: %v", err)
	}
	if err := leased.releaseCapturedSources(); err != nil {
		t.Fatal(err)
	}
	if err := install.ValidateManagedEntries(p, opts); err == nil || !strings.Contains(err.Error(), "released") {
		t.Fatalf("released lease error = %v, want fail-closed authority", err)
	}
}

func TestInstallTagSelectorCandidateStoreSerializesLatePreviewAndCancellation(t *testing.T) {
	store := newInstallTagSelectorCandidateStore()
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := range 100 {
		requestID := uint64(i + 1)
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			token, ok, err := store.activate(requestID)
			if err != nil {
				t.Errorf("activate(%d): %v", requestID, err)
				return
			}
			if !ok {
				return
			}
			if _, err := store.put(requestID, token, installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: fmt.Sprintf("sha256:%d", i), CandidateToken: token}}); err != nil {
				t.Errorf("put(%d): %v", requestID, err)
			}
		}()
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		if err := store.close(); err != nil {
			t.Errorf("close candidate store: %v", err)
		}
	}()
	close(start)
	workers.Wait()
	if err := store.close(); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.activate(101); err != nil || ok {
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("candidate store reopened after concurrent cancellation")
	}
}

func TestInstallTagSelectorCandidateStoreUsesUIRequestOrder(t *testing.T) {
	store := newInstallTagSelectorCandidateStore()
	t.Cleanup(func() {
		if err := store.close(); err != nil {
			t.Errorf("candidate store cleanup: %v", err)
		}
	})

	tokenB, ok, err := store.activate(2)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("candidate store rejected newer request")
	}
	if _, ok, err := store.activate(1); err != nil || ok {
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("candidate store accepted an older request that arrived later")
	}
	candidateB := installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:b", CandidateToken: tokenB}}
	stored, err := store.put(2, tokenB, candidateB)
	if err != nil {
		t.Fatal(err)
	}
	if !stored {
		t.Fatal("candidate store rejected the newest request")
	}
	if _, ok := store.take(tokenB); !ok {
		t.Fatal("candidate store lost the newest request after scheduling inversion")
	}
}

func TestInstallTagSelectorCandidateStoreCleanupErrorsDeliveredOnce(t *testing.T) {
	t.Run("superseded", func(t *testing.T) {
		injected := errors.New("injected superseded release failure")
		store := newInstallTagSelectorCandidateStore()
		var releases int
		store.release = func(candidate installTagSelectorCandidate) error {
			releases++
			if candidate.Preview.SemanticDigest == "sha256:first" {
				return injected
			}
			return nil
		}

		token, ok, err := store.activate(1)
		if err != nil || !ok {
			t.Fatalf("activate first candidate = (%q, %t, %v)", token, ok, err)
		}
		if stored, err := store.put(1, token, installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:first"}}); err != nil || !stored {
			t.Fatalf("put first candidate = (%t, %v)", stored, err)
		}
		_, ok, err = store.activate(2)
		if !ok || !errors.Is(err, injected) || !strings.Contains(err.Error(), `release superseded tag selector candidate "selector-preview-1"`) {
			t.Fatalf("activate replacement = (%t, %v), want contextual wrapped cleanup failure", ok, err)
		}
		if err := store.close(); err != nil {
			t.Fatalf("close replayed superseded cleanup failure: %v", err)
		}
		if err := store.close(); err != nil {
			t.Fatalf("second close replayed superseded cleanup failure: %v", err)
		}
		if releases != 1 {
			t.Fatalf("release calls = %d, want 1", releases)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		injected := errors.New("injected rejected release failure")
		store := newInstallTagSelectorCandidateStore()
		var releases int
		store.release = func(installTagSelectorCandidate) error {
			releases++
			return injected
		}

		token, ok, err := store.activate(1)
		if err != nil || !ok {
			t.Fatalf("activate first candidate = (%q, %t, %v)", token, ok, err)
		}
		if _, ok, err := store.activate(2); err != nil || !ok {
			t.Fatalf("activate newer candidate = (%t, %v)", ok, err)
		}
		stored, err := store.put(1, token, installTagSelectorCandidate{})
		if stored || !errors.Is(err, injected) || !strings.Contains(err.Error(), `release rejected tag selector candidate "selector-preview-1"`) {
			t.Fatalf("put rejected candidate = (%t, %v), want contextual wrapped cleanup failure", stored, err)
		}
		if err := store.close(); err != nil {
			t.Fatalf("close replayed rejected cleanup failure: %v", err)
		}
		if err := store.close(); err != nil {
			t.Fatalf("second close replayed rejected cleanup failure: %v", err)
		}
		if releases != 1 {
			t.Fatalf("release calls = %d, want 1", releases)
		}
	})

	t.Run("replaced", func(t *testing.T) {
		injected := errors.New("injected replaced release failure")
		store := newInstallTagSelectorCandidateStore()
		releases := make(map[string]int)
		store.release = func(candidate installTagSelectorCandidate) error {
			digest := candidate.Preview.SemanticDigest
			releases[digest]++
			if digest == "sha256:first" {
				return injected
			}
			return nil
		}

		token, ok, err := store.activate(1)
		if err != nil || !ok {
			t.Fatalf("activate candidate = (%q, %t, %v)", token, ok, err)
		}
		if stored, err := store.put(1, token, installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:first"}}); err != nil || !stored {
			t.Fatalf("put first candidate = (%t, %v)", stored, err)
		}
		stored, err := store.put(1, token, installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:second"}})
		if !stored || !errors.Is(err, injected) || !strings.Contains(err.Error(), `release replaced tag selector candidate "selector-preview-1"`) {
			t.Fatalf("put replacement candidate = (%t, %v), want contextual wrapped cleanup failure", stored, err)
		}
		if err := store.close(); err != nil {
			t.Fatalf("close replayed replaced cleanup failure: %v", err)
		}
		if err := store.close(); err != nil {
			t.Fatalf("second close replayed replaced cleanup failure: %v", err)
		}
		if releases["sha256:first"] != 1 || releases["sha256:second"] != 1 {
			t.Fatalf("release calls = %v, want each candidate once", releases)
		}
	})

	t.Run("stored close", func(t *testing.T) {
		injected := errors.New("injected stored release failure")
		store := newInstallTagSelectorCandidateStore()
		var releases int
		store.release = func(installTagSelectorCandidate) error {
			releases++
			return injected
		}

		token, ok, err := store.activate(1)
		if err != nil || !ok {
			t.Fatalf("activate candidate = (%q, %t, %v)", token, ok, err)
		}
		if stored, err := store.put(1, token, installTagSelectorCandidate{}); err != nil || !stored {
			t.Fatalf("put candidate = (%t, %v)", stored, err)
		}
		err = store.close()
		if !errors.Is(err, injected) || !strings.Contains(err.Error(), `release stored tag selector candidate "selector-preview-1"`) {
			t.Fatalf("close error = %v, want contextual wrapped cleanup failure", err)
		}
		if err := store.close(); err != nil {
			t.Fatalf("second close replayed stored cleanup failure: %v", err)
		}
		if releases != 1 {
			t.Fatalf("release calls = %d, want 1", releases)
		}
	})
}

func TestInstallTagSelectorPreviewProviderReturnsSynchronousCleanupOnce(t *testing.T) {
	t.Run("superseded", func(t *testing.T) {
		injected := errors.New("injected synchronous supersede failure")
		store := newInstallTagSelectorCandidateStore()
		var releases int
		store.release = func(installTagSelectorCandidate) error {
			releases++
			return injected
		}
		lateErrors := make(chan error, 1)
		var builds int
		provider := &installTagSelectorPreviewProvider{
			store:     store,
			lateError: func(err error) { lateErrors <- err },
			build: func([]string) (installTagSelectorCandidate, error) {
				builds++
				return installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:candidate"}}, nil
			},
		}
		if _, err := provider.preview(1, []string{"first"}); err != nil {
			t.Fatal(err)
		}
		_, err := provider.preview(2, []string{"second"})
		if !errors.Is(err, injected) || !strings.Contains(err.Error(), `release superseded tag selector candidate "selector-preview-1"`) {
			t.Fatalf("preview error = %v, want synchronous contextual cleanup failure", err)
		}
		if err := provider.close(); err != nil {
			t.Fatalf("close replayed synchronous cleanup failure: %v", err)
		}
		if err := provider.close(); err != nil {
			t.Fatalf("second close replayed synchronous cleanup failure: %v", err)
		}
		select {
		case err := <-lateErrors:
			t.Fatalf("synchronous cleanup was also sent to late sink: %v", err)
		default:
		}
		if releases != 1 || builds != 1 {
			t.Fatalf("release calls = %d, builds = %d, want 1 and 1", releases, builds)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		injected := errors.New("injected synchronous reject failure")
		store := newInstallTagSelectorCandidateStore()
		var releases int
		store.release = func(installTagSelectorCandidate) error {
			releases++
			return injected
		}
		lateErrors := make(chan error, 1)
		provider := &installTagSelectorPreviewProvider{
			store:     store,
			lateError: func(err error) { lateErrors <- err },
			build: func([]string) (installTagSelectorCandidate, error) {
				if _, ok, err := store.activate(2); err != nil || !ok {
					t.Fatalf("supersede active request = (%t, %v)", ok, err)
				}
				return installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:rejected"}}, nil
			},
		}
		_, err := provider.preview(1, []string{"first"})
		if !errors.Is(err, injected) || !strings.Contains(err.Error(), `release rejected tag selector candidate "selector-preview-1"`) {
			t.Fatalf("preview error = %v, want synchronous contextual cleanup failure", err)
		}
		if err := provider.close(); err != nil {
			t.Fatalf("close replayed synchronous cleanup failure: %v", err)
		}
		if err := provider.close(); err != nil {
			t.Fatalf("second close replayed synchronous cleanup failure: %v", err)
		}
		select {
		case err := <-lateErrors:
			t.Fatalf("synchronous cleanup was also sent to late sink: %v", err)
		default:
		}
		if releases != 1 {
			t.Fatalf("release calls = %d, want 1", releases)
		}
	})
}

func TestInstallTagSelectorPreviewProviderReturnsStoredCloseCleanupOnce(t *testing.T) {
	injected := errors.New("injected stored close failure")
	store := newInstallTagSelectorCandidateStore()
	var releases int
	store.release = func(installTagSelectorCandidate) error {
		releases++
		return injected
	}
	lateErrors := make(chan error, 1)
	provider := &installTagSelectorPreviewProvider{
		store:     store,
		lateError: func(err error) { lateErrors <- err },
		build: func([]string) (installTagSelectorCandidate, error) {
			return installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:stored"}}, nil
		},
	}
	if _, err := provider.preview(1, []string{"stored"}); err != nil {
		t.Fatal(err)
	}
	err := provider.close()
	if !errors.Is(err, injected) || !strings.Contains(err.Error(), `release stored tag selector candidate "selector-preview-1"`) {
		t.Fatalf("close error = %v, want contextual wrapped cleanup failure", err)
	}
	if err := provider.close(); err != nil {
		t.Fatalf("second close replayed stored cleanup failure: %v", err)
	}
	select {
	case err := <-lateErrors:
		t.Fatalf("stored close cleanup was also sent to late sink: %v", err)
	default:
	}
	if releases != 1 {
		t.Fatalf("release calls = %d, want 1", releases)
	}
}

func TestInstallTagSelectorPreviewProviderSerializesCandidateBuilds(t *testing.T) {
	store := newInstallTagSelectorCandidateStore()
	t.Cleanup(func() {
		if err := store.close(); err != nil {
			t.Errorf("candidate store cleanup: %v", err)
		}
	})
	entered := make(chan string, 2)
	release := make(chan struct{})
	var statsMu sync.Mutex
	inFlight := 0
	maxInFlight := 0
	provider := &installTagSelectorPreviewProvider{
		store: store,
		build: func(tags []string) (installTagSelectorCandidate, error) {
			statsMu.Lock()
			inFlight++
			if inFlight > maxInFlight {
				maxInFlight = inFlight
			}
			statsMu.Unlock()
			entered <- tags[0]
			<-release
			statsMu.Lock()
			inFlight--
			statsMu.Unlock()
			return installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:" + tags[0]}}, nil
		},
	}

	type previewResult struct {
		preview tagselectortui.Preview
		err     error
	}
	first := make(chan previewResult, 1)
	second := make(chan previewResult, 1)
	go func() {
		preview, err := provider.preview(1, []string{"first"})
		first <- previewResult{preview: preview, err: err}
	}()
	if got := <-entered; got != "first" {
		t.Fatalf("first build = %q, want first", got)
	}
	go func() {
		preview, err := provider.preview(2, []string{"second"})
		second <- previewResult{preview: preview, err: err}
	}()
	select {
	case got := <-entered:
		t.Fatalf("build %q entered while the first capture-bearing build was blocked", got)
	case <-time.After(50 * time.Millisecond):
	}
	release <- struct{}{}
	if result := <-first; result.err != nil {
		t.Fatalf("first preview error = %v", result.err)
	}
	if got := <-entered; got != "second" {
		t.Fatalf("second build = %q, want second", got)
	}
	release <- struct{}{}
	if result := <-second; result.err != nil {
		t.Fatalf("second preview error = %v", result.err)
	}
	statsMu.Lock()
	defer statsMu.Unlock()
	if maxInFlight != 1 {
		t.Fatalf("maximum concurrent candidate builds = %d, want 1", maxInFlight)
	}
}

func TestInstallTagSelectorPreviewProviderClosesWithoutWaitingForActiveBuild(t *testing.T) {
	store := newInstallTagSelectorCandidateStore()
	var releaseCount atomic.Int32
	store.release = func(installTagSelectorCandidate) error {
		releaseCount.Add(1)
		return errors.New("injected late release failure")
	}
	entered := make(chan string, 2)
	release := make(chan struct{})
	lateErrors := make(chan error, 2)
	var lateErrorCount atomic.Int32
	var buildCount atomic.Int32
	provider := &installTagSelectorPreviewProvider{
		store: store,
		lateError: func(err error) {
			lateErrorCount.Add(1)
			lateErrors <- err
		},
		build: func(tags []string) (installTagSelectorCandidate, error) {
			buildCount.Add(1)
			entered <- tags[0]
			<-release
			return installTagSelectorCandidate{Preview: tagselectortui.Preview{SemanticDigest: "sha256:" + tags[0]}}, nil
		},
	}
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() {
		_, err := provider.preview(1, []string{"first"})
		first <- err
	}()
	if got := <-entered; got != "first" {
		t.Fatalf("first build = %q, want first", got)
	}
	go func() {
		_, err := provider.preview(2, []string{"second"})
		second <- err
	}()
	closed := make(chan error, 1)
	go func() { closed <- provider.close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("close preview provider: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("preview provider close waited for the active build")
	}
	release <- struct{}{}
	if err := <-first; err == nil || err.Error() != "tag selector preview provider is closed" {
		t.Fatalf("active preview error = %v, want generic closed response", err)
	}
	select {
	case err := <-lateErrors:
		if !strings.Contains(err.Error(), `release rejected tag selector candidate "selector-preview-1"`) || !strings.Contains(err.Error(), "injected late release failure") {
			t.Fatalf("late cleanup sink error = %v, want candidate and operation context", err)
		}
	case <-time.After(time.Second):
		t.Fatal("late release failure was not reported to the asynchronous cleanup sink")
	}
	if err := <-second; err == nil || err.Error() != "tag selector preview provider is closed" {
		t.Fatalf("queued preview error = %v, want generic closed response", err)
	}
	select {
	case got := <-entered:
		t.Fatalf("queued build %q entered after provider close", got)
	default:
	}
	if err := provider.close(); err != nil {
		t.Fatalf("second close replayed late cleanup failure: %v", err)
	}
	select {
	case err := <-lateErrors:
		t.Fatalf("late cleanup was delivered more than once: %v", err)
	default:
	}
	if got := releaseCount.Load(); got != 1 {
		t.Fatalf("release calls = %d, want 1", got)
	}
	if got := lateErrorCount.Load(); got != 1 {
		t.Fatalf("late sink calls = %d, want 1", got)
	}
	if got := buildCount.Load(); got != 1 {
		t.Fatalf("build calls = %d, want 1", got)
	}
}

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
			built, err := preview(1, []string{"one"})
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
		for _, want := range []string{"Forward-only selection preview", "Dry run complete; nothing was applied."} {
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

func TestInstallTagSelectorPreviewIncludesPreparedDependencies(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, []string{"one"})
		return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, err
	})
	stubDir := t.TempDir()
	t.Setenv("PATH", stubDir)

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--dry-run", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	for _, want := range []string{"Dependency install preview", "install one manually (required)", "Plan for tags only", "Dry run complete; nothing was applied."} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("selector preview missing %q\n%s", want, out.String())
		}
	}
	if _, err := os.Stat(state.Path(stateRoot)); !os.IsNotExist(err) {
		t.Fatalf("dry-run selector wrote Installation Metadata: %v", err)
	}
}

func TestInstallTagSelectorAcceptedAdditionAppliesAndPersistsExplicitTags(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, initial []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		if len(initial) != 0 {
			t.Fatalf("initial Tags = %v, want empty without Installed Selection", initial)
		}
		built, err := preview(1, []string{"one"})
		return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, err
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--skip-deps", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	if _, err := os.Readlink(filepath.Join(home, ".one")); err != nil {
		t.Fatalf("accepted Tag selection did not apply Managed Entry: %v\n%s", err, out.String())
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
	if got, want := meta.InstalledSelection.ExtraTags, []string{"one"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtraTags = %#v, want %#v", got, want)
	}
	if got, want := meta.InstalledSelection.ResolvedTags, []string{"one"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolvedTags = %#v, want %#v", got, want)
	}
	for _, want := range []string{
		"Tag selection applied: tags=one",
		"Historical retirement: not run (Tag selector path)",
		"Managed Entry results:",
		"created    " + filepath.Join(home, ".one"),
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("final result missing %q\n%s", want, out.String())
		}
	}
}

func TestInstallTagSelectorProfilePresetResultIsPersistedAsExplicitCurrentTags(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, browse tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		var preset []string
		for _, profile := range browse.Profiles {
			if profile.Name == "default" {
				preset = append([]string(nil), profile.Tags...)
			}
		}
		if !reflect.DeepEqual(preset, []string{"one"}) {
			t.Fatalf("default preset Tags = %#v, want one", preset)
		}
		built, err := preview(1, preset)
		return tagselectortui.Result{Tags: preset, Preview: built}, err
	})

	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"install", "--skip-deps", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || len(meta.InstalledSelection.Profiles) != 0 ||
		!reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{"one"}) ||
		!reflect.DeepEqual(meta.InstalledSelection.ResolvedTags, []string{"one"}) {
		t.Fatalf("Installed Selection = %#v, want flattened explicit current Tag intent", meta.InstalledSelection)
	}
}

func TestInstallTagSelectorRejectsAcceptedCandidateWhenSourceIdentityChanges(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, []string{"one"})
		if err != nil {
			return tagselectortui.Result{}, err
		}
		replacement := filepath.Join(sourceRoot, "config", "replacement")
		if err := os.WriteFile(replacement, []byte("changed\n"), 0o600); err != nil {
			t.Fatalf("write replacement source: %v", err)
		}
		if err := os.Rename(replacement, filepath.Join(sourceRoot, "config", "one")); err != nil {
			t.Fatalf("replace source identity: %v", err)
		}
		return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, nil
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--skip-deps", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "source") || !strings.Contains(err.Error(), "changed identity") {
		t.Fatalf("Execute() error = %v, want stale source identity rejection\n%s", err, out.String())
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".one")); !os.IsNotExist(statErr) {
		t.Fatalf("Managed Entry changed after stale candidate: %v", statErr)
	}
	if _, statErr := os.Stat(state.Path(stateRoot)); !os.IsNotExist(statErr) {
		t.Fatalf("Installation Metadata changed after stale candidate: %v", statErr)
	}
}

func TestInstallTagSelectorRejectsManifestAndMetadataStalenessBeforeMutation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, manifestPath, stateRoot string)
	}{
		{
			name: "Install Manifest",
			mutate: func(t *testing.T, manifestPath, _ string) {
				data, err := os.ReadFile(manifestPath)
				if err != nil {
					t.Fatal(err)
				}
				data = bytes.Replace(data, []byte("One selectable capability."), []byte("Changed selectable capability."), 1)
				if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "Installation Metadata",
			mutate: func(t *testing.T, _ string, stateRoot string) {
				concurrent := &state.InstalledSelection{Profiles: []string{}, ExtraTags: []string{"two"}, ResolvedTags: []string{"two"}}
				if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{}, InstalledSelection: concurrent}); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
			setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
				built, err := preview(1, []string{"one"})
				if err != nil {
					return tagselectortui.Result{}, err
				}
				tc.mutate(t, manifestPath, stateRoot)
				return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, nil
			})

			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{
				"install", "--skip-deps", "--file", manifestPath,
				"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
			})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "candidate is stale") || !strings.Contains(err.Error(), tc.name) {
				t.Fatalf("Execute() error = %v, want %s staleness rejection\n%s", err, tc.name, out.String())
			}
			if _, statErr := os.Lstat(filepath.Join(home, ".one")); !os.IsNotExist(statErr) {
				t.Fatalf("Managed Entry changed after stale candidate: %v", statErr)
			}
		})
	}
}

func TestInstallTagSelectorOrdinaryReductionRequiresDistinctConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name          string
		answer        string
		acknowledged  bool
		wantTags      []string
		wantTwoExists bool
	}{
		{name: "decline", answer: "n\n", wantTags: []string{"one", "two"}, wantTwoExists: true},
		{name: "accept", answer: "y\n", wantTags: []string{"one"}},
		{name: "accepted in selector is not prompted twice", acknowledged: true, wantTags: []string{"one"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
			runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "one", "two")
			setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, initial []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
				if want := []string{"one", "two"}; !reflect.DeepEqual(initial, want) {
					t.Fatalf("initial Tags = %#v, want %#v", initial, want)
				}
				built, err := preview(1, []string{"one"})
				return tagselectortui.Result{Tags: []string{"one"}, Preview: built, AcknowledgementAccepted: tc.acknowledged}, err
			})

			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetIn(strings.NewReader(tc.answer))
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{
				"install", "--skip-deps", "--file", manifestPath,
				"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\n%s", err, out.String())
			}
			if !tc.acknowledged && !strings.Contains(out.String(), "Apply this Installed Selection reduction? This is separate from Conflict Resolution.") {
				t.Fatalf("output missing distinct reduction confirmation\n%s", out.String())
			}
			if tc.acknowledged && strings.Contains(out.String(), "Apply this Installed Selection reduction?") {
				t.Fatalf("selector acknowledgement was prompted twice\n%s", out.String())
			}
			_, twoErr := os.Lstat(filepath.Join(home, ".two"))
			if tc.wantTwoExists && twoErr != nil {
				t.Fatalf("declined reduction changed .two: %v", twoErr)
			}
			if !tc.wantTwoExists && !os.IsNotExist(twoErr) {
				t.Fatalf("accepted reduction retained .two: %v", twoErr)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.ExtraTags, tc.wantTags) {
				t.Fatalf("Installed Selection = %#v, want explicit Tags %#v", meta.InstalledSelection, tc.wantTags)
			}
		})
	}
}

func TestInstallTagSelectorAcceptedBlockedReductionStopsBeforeMutation(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorMixedFixture(t)
	stubDir := t.TempDir()
	writeSelectorExecutable(t, filepath.Join(stubDir, "claude"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "base", "retired")
	previous := *loadSelectorSelection(t, stateRoot)
	if err := os.WriteFile(filepath.Join(home, ".shared.json"), []byte("{\"base\":true,\"retired\":\"operator-change\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		tags := []string{"base", "next"}
		built, err := preview(1, tags)
		return tagselectortui.Result{Tags: tags, Preview: built, AcknowledgementAccepted: true}, err
	})

	out, err := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if err == nil || !strings.Contains(err.Error(), "accepted Tag selector reduction is blocked") {
		t.Fatalf("Execute() error = %v, want blocked reduction rejection\n%s", err, out)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".next")); !os.IsNotExist(statErr) {
		t.Fatalf("blocked reduction created next Managed Entry: %v", statErr)
	}
	if got := loadSelectorSelection(t, stateRoot); !reflect.DeepEqual(*got, previous) {
		t.Fatalf("Installed Selection = %#v, want previous %#v", got, previous)
	}
}

func TestInstallTagSelectorEmptyResultAlwaysRequiresLiteralClear(t *testing.T) {
	for _, tc := range []struct {
		name         string
		seed         string
		answer       string
		wantTags     []string
		wantOne      bool
		wantMetadata bool
	}{
		{name: "mismatch preserves prior authority", seed: "one", answer: "CLEAR\n", wantTags: []string{"one"}, wantOne: true, wantMetadata: true},
		{name: "clear succeeds", seed: "one", answer: "clear\n", wantTags: []string{}, wantMetadata: true},
		{name: "nil authority still prompts and records empty", answer: "clear\n", wantTags: []string{}, wantMetadata: true},
		{name: "already empty still prompts", seed: "empty", answer: "clear\n", wantTags: []string{}, wantMetadata: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
			switch tc.seed {
			case "one":
				runExplicitTagSelectorSeed(t, manifestPath, sourceRoot, home, stateRoot, "one")
			case "empty":
				empty := &state.InstalledSelection{Profiles: []string{}, ExtraTags: []string{}, ResolvedTags: []string{}}
				if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{}, InstalledSelection: empty}); err != nil {
					t.Fatalf("seed explicit empty Installed Selection: %v", err)
				}
			}
			setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
				built, err := preview(1, []string{})
				return tagselectortui.Result{Tags: []string{}, Preview: built}, err
			})

			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetIn(strings.NewReader(tc.answer))
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{
				"install", "--skip-deps", "--file", manifestPath,
				"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\n%s", err, out.String())
			}
			if !strings.Contains(out.String(), "Type clear to remove every selected Managed Entry from dots management:") {
				t.Fatalf("output missing literal clear prompt\n%s", out.String())
			}
			_, oneErr := os.Lstat(filepath.Join(home, ".one"))
			if tc.wantOne && oneErr != nil {
				t.Fatalf("clear mismatch changed .one: %v", oneErr)
			}
			if !tc.wantOne && tc.seed == "one" && !os.IsNotExist(oneErr) {
				t.Fatalf("successful clear retained .one: %v", oneErr)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			matchesTags := meta.InstalledSelection != nil && reflect.DeepEqual(meta.InstalledSelection.ExtraTags, tc.wantTags)
			if len(tc.wantTags) == 0 {
				matchesTags = meta.InstalledSelection != nil && len(meta.InstalledSelection.ExtraTags) == 0 && len(meta.InstalledSelection.ResolvedTags) == 0
			}
			if tc.wantMetadata && !matchesTags {
				t.Fatalf("Installed Selection = %#v, want explicit Tags %#v", meta.InstalledSelection, tc.wantTags)
			}
		})
	}
}

func TestInstallTagSelectorAcceptedSelectionUsesExistingConflictResolution(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	target := filepath.Join(home, ".one")
	if err := os.WriteFile(target, []byte("local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, []string{"one"})
		return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, err
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader("r\r"))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--skip-deps", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	if _, err := os.Readlink(target); err != nil {
		t.Fatalf("existing Conflict Resolution did not replace target: %v", err)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 1 {
		t.Fatalf("Backup Sets = %#v, error = %v, want one protected replacement", backupMeta.Sets, err)
	}
}

func TestInstallTagSelectorAdoptUsesPostConflictSnapshots(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	target := filepath.Join(home, ".one")
	if err := os.WriteFile(target, []byte("adopted-local\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, []string{"one"})
		return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, err
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetIn(strings.NewReader("a\r"))
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--skip-deps", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	gotSource, err := os.ReadFile(filepath.Join(sourceRoot, "config", "one"))
	if err != nil || string(gotSource) != "adopted-local\n" {
		t.Fatalf("adopted Source of Truth = %q, %v", gotSource, err)
	}
	if _, err := os.Readlink(target); err != nil {
		t.Fatalf("adopt did not install managed symlink: %v", err)
	}
}

func TestInstallTagSelectorCommitFailureThenPublicRerunConverges(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	runner := func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, []string{"one"})
		return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, err
	}
	setInstallTagSelectorTestHooks(t, true, runner)
	originalCommit := commitSelectorInstallationMetadata
	commitSelectorInstallationMetadata = func(install.MetadataCommit, *state.InstalledSelection, state.InstalledSelection) error {
		return errors.New("injected selector terminal commit failure")
	}
	t.Cleanup(func() { commitSelectorInstallationMetadata = originalCommit })

	run := func() error {
		cmd := NewRootCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{
			"install", "--skip-deps", "--file", manifestPath,
			"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
		})
		return cmd.Execute()
	}
	if err := run(); err == nil || !strings.Contains(err.Error(), "injected selector terminal commit failure") {
		t.Fatalf("first Execute() error = %v, want injected commit failure", err)
	}
	partial, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if partial.InstalledSelection != nil {
		t.Fatalf("Installed Selection = %#v after failed commit, want prior nil authority", partial.InstalledSelection)
	}
	if _, err := os.Readlink(filepath.Join(home, ".one")); err != nil {
		t.Fatalf("Managed Entry was not staged before commit failure: %v", err)
	}

	commitSelectorInstallationMetadata = originalCommit
	if err := run(); err != nil {
		t.Fatalf("public rerun did not converge: %v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{"one"}) {
		t.Fatalf("Installed Selection = %#v, want converged explicit Tag one", meta.InstalledSelection)
	}
	if len(meta.Entries) != 1 || len(meta.Entries[0].Contributions) == 0 {
		t.Fatalf("Installation Metadata = %#v, want terminal contribution evidence", meta.Entries)
	}
}

func TestInstallTagSelectorLeavesHistoricalRetirementEvidenceUntouched(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	if err := state.Save(state.Path(stateRoot), state.Metadata{
		Version: state.CurrentVersion,
		Entries: []state.Record{},
		Provisioners: []state.ProvisionerRecord{{
			Profile: "agents", Tool: "gentle-ai", Executable: "gentle-ai", Args: []string{"install", "--scope", "global"}, Status: "provisioned",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	instructions := filepath.Join(home, ".codex", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(instructions), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "before\n<!-- gentle-ai:trigger-rules -->\nretired\n<!-- /gentle-ai:trigger-rules -->\nafter\n"
	if err := os.WriteFile(instructions, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, []string{"one"})
		return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, err
	})

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"install", "--skip-deps", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out.String())
	}
	got, err := os.ReadFile(instructions)
	if err != nil || string(got) != legacy {
		t.Fatalf("selector path changed historical state: %q, %v", got, err)
	}
	if !strings.Contains(out.String(), "Historical retirement: not run (Tag selector path)") {
		t.Fatalf("final result does not disclose historical-retirement exclusion\n%s", out.String())
	}
}

func TestInstallTagSelectorSkipsAcceptedProvisionerWhenDependencyIsMissing(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorProvisionerFixture(t)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifestData = bytes.Replace(manifestData, []byte("      - name: claude\n"), []byte("      - name: selector-missing-tool\n"), 1)
	if err := os.WriteFile(manifestPath, manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	stubDir := t.TempDir()
	marker := filepath.Join(stubDir, "provisioner-invoked")
	writeSelectorExecutable(t, filepath.Join(stubDir, "claude"), fmt.Sprintf("#!/bin/sh\n: > %q\n", marker))
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	setInstallTagSelectorTestHooks(t, true, acceptedSelectorTags("two"))

	out, err := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if err == nil || !strings.Contains(err.Error(), `provisioner "claude" is missing dependencies`) {
		t.Fatalf("Execute() error = %v, want missing Provisioner Dependency gate\n%s", err, out)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("Provisioner ran despite missing Dependency: %v", statErr)
	}
	if !strings.Contains(out, "missing-dependencies") || !strings.Contains(out, "selector-missing-tool") {
		t.Fatalf("Provisioner result missing dependency finding\n%s", out)
	}
}

func runExplicitTagSelectorSeed(t *testing.T, manifestPath, sourceRoot, home, stateRoot string, tags ...string) {
	t.Helper()
	args := []string{
		"install", "--yes", "--skip-deps", "--file", manifestPath,
		"--source-root", sourceRoot, "--home", home, "--state-root", stateRoot,
	}
	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}
	cmd := NewRootCommand()
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("seed explicit Installed Selection: %v", err)
	}
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
		name          string
		expectApplied bool
		runner        installTagSelectorRunnerFunc
	}{
		{
			name:          "completed selection applies from current checkout",
			expectApplied: true,
			runner: func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
				built, err := preview(1, []string{"one"})
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
			beforeSource := fingerprintTree(t, sourceRoot)
			beforeHome := fingerprintTree(t, home)
			setInstallTagSelectorTestHooks(t, true, tc.runner)

			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"install", "--home", home})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v\n%s", err, out.String())
			}
			if afterSource := fingerprintTree(t, sourceRoot); afterSource != beforeSource {
				t.Fatalf("Installed Repository changed after selector path: before %s, after %s", beforeSource, afterSource)
			}
			if tc.expectApplied {
				if _, err := os.Readlink(filepath.Join(home, ".one")); err != nil {
					t.Fatalf("Managed Entry missing after accepted selector path: %v", err)
				}
				meta, err := state.Load(state.Path(defaultStateRoot(home)))
				if err != nil || meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{"one"}) {
					t.Fatalf("Installed Selection = %#v, error = %v, want explicit Tag one", meta.InstalledSelection, err)
				}
			} else {
				if afterHome := fingerprintTree(t, home); afterHome != beforeHome {
					t.Fatalf("home fingerprint changed after canceled selector: before %s, after %s", beforeHome, afterHome)
				}
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
		_, err := preview(1, []string{"unknown"})
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
	if err := os.WriteFile(filepath.Join(configDir, "two"), []byte("two\n"), 0o600); err != nil {
		t.Fatalf("write second selector source: %v", err)
	}
	manifestPath = filepath.Join(sourceRoot, "dots.yaml")
	data := []byte(`version: 1
tags:
  one:
    description: One selectable capability.
    kind: surface
    status: current
  two:
    description: Second selectable capability.
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
  - source: config/two
    target: ~/.two
    strategy: symlink
    tags: [two]
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

func TestInstallTagSelectorRejectsAcceptedPreviewWithDifferentCandidateToken(t *testing.T) {
	manifestPath, sourceRoot, home, stateRoot := writeTagSelectorTestManifest(t)
	setInstallTagSelectorTestHooks(t, true, func(_ io.Reader, _ io.Writer, _ tagselectortui.BrowseData, _ []string, preview tagselectortui.PreviewFunc) (tagselectortui.Result, error) {
		built, err := preview(1, []string{"one"})
		if err != nil {
			return tagselectortui.Result{}, err
		}
		built.CandidateToken = "selector-preview-forged"
		return tagselectortui.Result{Tags: []string{"one"}, Preview: built}, nil
	})

	out, err := executeAcceptedSelector(t, "", manifestPath, sourceRoot, home, stateRoot)
	if err == nil || !strings.Contains(err.Error(), "accepted preview does not match its immutable candidate") {
		t.Fatalf("Execute() error = %v, want candidate-token rejection\n%s", err, out)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".one")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("forged candidate token mutated Managed Entry: %v", statErr)
	}
	meta, loadErr := state.Load(state.Path(stateRoot))
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if meta.InstalledSelection != nil {
		t.Fatalf("forged candidate token committed Installed Selection: %#v", meta.InstalledSelection)
	}
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
	candidate, err := buildInstallTagSelectorCandidate(m, meta, resolvedPaths{
		Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot, XDGStateHome: filepath.Join(home, ".local", "state"),
	}, sourceRoot, nil, "linux", tags, installTagSelectorOptions{SkipDependencies: true})
	if err != nil {
		t.Fatalf("buildInstallTagSelectorCandidate() error = %v", err)
	}
	t.Cleanup(func() {
		if err := candidate.releaseCapturedSources(); err != nil {
			t.Errorf("release selector candidate: %v", err)
		}
	})
	preview := candidate.Preview
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
	candidate, err := buildInstallTagSelectorCandidate(m, meta, resolvedPaths{
		Home: home, SourceRoot: sourceRoot, StateRoot: t.TempDir(), XDGStateHome: filepath.Join(home, ".local", "state"),
	}, sourceRoot, nil, "linux", []string{"new"}, installTagSelectorOptions{SkipDependencies: true})
	if err != nil {
		t.Fatalf("buildInstallTagSelectorCandidate() error = %v", err)
	}
	t.Cleanup(func() {
		if err := candidate.releaseCapturedSources(); err != nil {
			t.Errorf("release selector candidate: %v", err)
		}
	})
	preview := candidate.Preview
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
