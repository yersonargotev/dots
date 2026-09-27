package batcache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

type fakeBatRunner struct {
	home            string
	buildCalls      int
	envs            [][]string
	dirs            []string
	calls           []string
	build           func(context.Context, string) error
	fail            map[string]error
	listOutput      string
	render          []byte
	listCalls       int
	failFinal       error
	renderCalls     int
	failFinalRender bool
}

func (r *fakeBatRunner) Run(ctx context.Context, _ string, args, env []string, _ []byte, dir string) ([]byte, []byte, error) {
	r.envs = append(r.envs, append([]string(nil), env...))
	r.dirs = append(r.dirs, dir)
	command := strings.Join(args, " ")
	r.calls = append(r.calls, command)
	if err := r.fail[command]; err != nil {
		return nil, nil, err
	}
	switch command {
	case "--config-dir":
		return []byte(filepath.Join(r.home, ".config", "bat") + "\n"), nil, nil
	case "--config-file":
		return []byte(filepath.Join(r.home, ".config", "bat", "config") + "\n"), nil, nil
	case "--cache-dir":
		return []byte(filepath.Join(r.home, ".cache", "bat") + "\n"), nil, nil
	case "cache --build":
		r.buildCalls++
		target := envValue(env, "BAT_CACHE_PATH")
		if r.build != nil {
			return nil, nil, r.build(ctx, target)
		}
		for _, name := range generatedFiles {
			if err := os.WriteFile(filepath.Join(target, name), []byte("generated-"+name), 0o600); err != nil {
				return nil, nil, err
			}
		}
		return nil, nil, nil
	case "--list-themes":
		r.listCalls++
		if r.listCalls == 2 && r.failFinal != nil {
			return nil, nil, r.failFinal
		}
		if r.listOutput != "" {
			return []byte(r.listOutput), nil, nil
		}
		return []byte("Carbonfox\nOther\n"), nil, nil
	case "--color=always --paging=never --style=plain --language=Go -":
		r.renderCalls++
		if r.renderCalls == 2 && r.failFinalRender {
			return []byte("plain output\n"), nil, nil
		}
		if r.render != nil {
			return r.render, nil, nil
		}
		return []byte("\x1b[38;2;220;220;220mrenderer\x1b[0m\n"), nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected bat args %q", args)
	}
}

func TestActivateUsesCapturedInputsAndConfinedNativeCache(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "bat"), []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := []byte("--theme=\"Carbonfox\"\n")
	theme := []byte("<plist>carbonfox</plist>\n")
	writeTestFile(t, filepath.Join(home, configRelative), config)
	writeTestFile(t, filepath.Join(home, themeRelative), theme)
	writeTestFile(t, filepath.Join(home, ".config", "bat", "themes", "User.tmTheme"), []byte("user-theme"))
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(outside, "Linked.tmTheme"), []byte("linked-theme"))
	if err := os.Symlink(filepath.Join(outside, "Linked.tmTheme"), filepath.Join(home, ".config", "bat", "themes", "Linked.tmTheme")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, ".cache", "bat", "keep.me"), []byte("unknown"))

	runner := &fakeBatRunner{home: home}
	receipt, err := Activate(context.Background(), Input{
		Home: home, Config: config, Theme: theme, Runner: runner,
		BaseEnv: []string{
			"PATH=" + bin,
			"HOME=/operator",
			"BAT_CACHE_PATH=/outside/cache",
			"BAT_CONFIG_DIR=/outside/config",
			"XDG_CACHE_HOME=/outside/xdg-cache",
			"XDG_CONFIG_HOME=/outside/xdg-config",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.buildCalls != 1 {
		t.Fatalf("build calls = %d, want 1", runner.buildCalls)
	}
	for _, dir := range runner.dirs {
		if dir == "" || !strings.HasPrefix(filepath.Base(dir), "dots-bat-cache-") {
			t.Fatalf("native working directory = %q, want private stage", dir)
		}
	}
	if receipt.CacheDir != filepath.Join(home, ".cache", "bat") || len(receipt.Hashes) != 3 {
		t.Fatalf("receipt = %+v", receipt)
	}
	for _, name := range generatedFiles {
		if got, err := os.ReadFile(filepath.Join(home, ".cache", "bat", name)); err != nil || string(got) != "generated-"+name {
			t.Fatalf("published %s = %q, %v", name, got, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(home, ".cache", "bat", "keep.me")); err != nil || string(got) != "unknown" {
		t.Fatalf("unknown cache file = %q, %v", got, err)
	}
	for _, env := range runner.envs {
		for _, item := range env {
			if strings.HasPrefix(item, "XDG_") || strings.Contains(item, "/outside/") {
				t.Fatalf("native environment retained hostile override %q", item)
			}
			if strings.HasPrefix(item, "BAT_") && !strings.Contains(item, runner.dirs[0]) {
				t.Fatalf("native environment has unconfined bat path %q", item)
			}
		}
	}
}

func TestActivateRejectsInstalledInputMismatchBeforeBuild(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "bat"), []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(home, configRelative), []byte("changed"))
	writeTestFile(t, filepath.Join(home, themeRelative), []byte("theme"))
	runner := &fakeBatRunner{home: home}
	_, err := Activate(context.Background(), Input{Home: home, Config: []byte("expected"), Theme: []byte("theme"), Runner: runner, BaseEnv: []string{"PATH=" + bin}})
	if err == nil || !strings.Contains(err.Error(), "differs from captured") {
		t.Fatalf("Activate() error = %v", err)
	}
	if runner.buildCalls != 0 {
		t.Fatalf("build calls = %d, want 0", runner.buildCalls)
	}
}

func TestActivateRejectsInvalidNativeOutputWithoutPublishing(t *testing.T) {
	tests := map[string]func(string) error{
		"missing": func(target string) error {
			return os.WriteFile(filepath.Join(target, generatedFiles[0]), []byte("generated"), 0o600)
		},
		"empty": func(target string) error {
			for _, name := range generatedFiles {
				data := []byte("generated")
				if name == "themes.bin" {
					data = nil
				}
				if err := os.WriteFile(filepath.Join(target, name), data, 0o600); err != nil {
					return err
				}
			}
			return nil
		},
		"symlink": func(target string) error {
			for _, name := range generatedFiles[:2] {
				if err := os.WriteFile(filepath.Join(target, name), []byte("generated"), 0o600); err != nil {
					return err
				}
			}
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
				return err
			}
			return os.Symlink(outside, filepath.Join(target, "themes.bin"))
		},
		"special": func(target string) error {
			for _, name := range generatedFiles[:2] {
				if err := os.WriteFile(filepath.Join(target, name), []byte("generated"), 0o600); err != nil {
					return err
				}
			}
			return os.Mkdir(filepath.Join(target, "themes.bin"), 0o700)
		},
		"extra": func(target string) error {
			if err := writeGeneratedOutput(target); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(target, "unexpected.bin"), []byte("unexpected"), 0o600)
		},
	}
	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			home, input := activationFixture(t)
			old := []byte("old-cache")
			for _, generated := range generatedFiles {
				writeTestFile(t, filepath.Join(home, cacheRelative, generated), old)
			}
			runner := &fakeBatRunner{home: home, build: func(_ context.Context, target string) error { return build(target) }}
			input.Runner = runner
			if receipt, err := Activate(context.Background(), input); err == nil || len(receipt.Hashes) != 0 {
				t.Fatalf("Activate() = %+v, %v; want failed empty receipt", receipt, err)
			}
			for _, generated := range generatedFiles {
				got, err := os.ReadFile(filepath.Join(home, cacheRelative, generated))
				if err != nil || string(got) != string(old) {
					t.Fatalf("preexisting %s changed to %q, %v", generated, got, err)
				}
			}
		})
	}
}

func TestActivateRejectsSpecialCustomSource(t *testing.T) {
	home, input := activationFixture(t)
	fifo := filepath.Join(home, ".config", "bat", "themes", "special.tmTheme")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	input.Runner = &fakeBatRunner{home: home}
	if _, err := Activate(context.Background(), input); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("Activate() error = %v", err)
	}
	assertNoGeneratedCache(t, home)
}

func TestActivateRejectsSourceChangeAndCancellationBeforePublishing(t *testing.T) {
	for name, mutate := range map[string]func(string) error{
		"file content": func(themes string) error {
			return os.WriteFile(filepath.Join(themes, "User.tmTheme"), []byte("after"), 0o600)
		},
		"directory inventory": func(themes string) error {
			return os.WriteFile(filepath.Join(themes, "Added.tmTheme"), []byte("added"), 0o600)
		},
	} {
		t.Run(name, func(t *testing.T) {
			home, input := activationFixture(t)
			themes := filepath.Join(home, ".config", "bat", "themes")
			writeTestFile(t, filepath.Join(themes, "User.tmTheme"), []byte("before"))
			runner := &fakeBatRunner{home: home, build: func(_ context.Context, target string) error {
				if err := writeGeneratedOutput(target); err != nil {
					return err
				}
				return mutate(themes)
			}}
			input.Runner = runner
			if _, err := Activate(context.Background(), input); err == nil || !strings.Contains(err.Error(), "changed during cache build") {
				t.Fatalf("Activate() error = %v", err)
			}
			assertNoGeneratedCache(t, home)
		})
	}

	t.Run("canceled", func(t *testing.T) {
		home, input := activationFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		runner := &fakeBatRunner{home: home, build: func(ctx context.Context, _ string) error { return ctx.Err() }}
		input.Runner = runner
		if _, err := Activate(ctx, input); !errors.Is(err, context.Canceled) {
			t.Fatalf("Activate() error = %v, want context canceled", err)
		}
		assertNoGeneratedCache(t, home)
	})
}

func TestActivateConfinesHostileSelectedHomePaths(t *testing.T) {
	home, input := activationFixture(t)
	if err := os.RemoveAll(filepath.Join(home, ".cache")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(home, ".cache")); err != nil {
		t.Fatal(err)
	}
	input.Runner = &fakeBatRunner{home: home}
	if _, err := Activate(context.Background(), input); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Activate() error = %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside directory entries = %v, %v", entries, err)
	}
}

func TestActivateFinalNativeProofFailureKeepsPublishedRuntimeState(t *testing.T) {
	for name, runnerFor := range map[string]func(string) *fakeBatRunner{
		"inventory": func(home string) *fakeBatRunner {
			return &fakeBatRunner{home: home, failFinal: errors.New("final inventory fault")}
		},
		"render": func(home string) *fakeBatRunner { return &fakeBatRunner{home: home, failFinalRender: true} },
	} {
		t.Run(name, func(t *testing.T) {
			home, input := activationFixture(t)
			input.Runner = runnerFor(home)
			receipt, err := Activate(context.Background(), input)
			if err == nil || len(receipt.Hashes) != 0 {
				t.Fatalf("Activate() = %+v, %v; want failed empty receipt", receipt, err)
			}
			for _, generated := range generatedFiles {
				if data, readErr := os.ReadFile(filepath.Join(home, cacheRelative, generated)); readErr != nil || len(data) == 0 {
					t.Fatalf("published runtime state %s = %q, %v", generated, data, readErr)
				}
			}
		})
	}
}

func TestActivateReplacesKnownCacheSymlinkWithoutWritingItsTarget(t *testing.T) {
	home, input := activationFixture(t)
	outside := filepath.Join(t.TempDir(), "outside-cache")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, cacheRelative), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, cacheRelative, "themes.bin")); err != nil {
		t.Fatal(err)
	}
	input.Runner = &fakeBatRunner{home: home}
	if _, err := Activate(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(home, cacheRelative, "themes.bin"))
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("published themes.bin mode = %v, %v", info, err)
	}
	if data, err := os.ReadFile(outside); err != nil || string(data) != "outside" {
		t.Fatalf("outside symlink target = %q, %v", data, err)
	}
}

func TestActivateJoinsLockAndStageCleanupFailures(t *testing.T) {
	fault := errors.New("injected lifecycle fault")
	tests := map[string]func(*operationHooks){
		"lock acquire": func(ops *operationHooks) {
			ops.flock = func(_ int, how int) error {
				if how&unix.LOCK_EX != 0 {
					return fault
				}
				return nil
			}
		},
		"lock release": func(ops *operationHooks) {
			ops.flock = func(fd int, how int) error {
				if how&unix.LOCK_UN != 0 {
					_ = unix.Flock(fd, how)
					return fault
				}
				return unix.Flock(fd, how)
			}
		},
		"stage cleanup": func(ops *operationHooks) {
			ops.cleanupStage = func(dir *os.File) error {
				if err := removeDirectoryContents(dir); err != nil {
					return err
				}
				return fault
			}
		},
	}
	for name, inject := range tests {
		t.Run(name, func(t *testing.T) {
			home, input := activationFixture(t)
			input.Runner = &fakeBatRunner{home: home}
			input.testOps = defaultOperationHooks()
			inject(input.testOps)
			receipt, err := Activate(context.Background(), input)
			if !errors.Is(err, fault) || len(receipt.Hashes) != 0 {
				t.Fatalf("Activate() = %+v, %v; want joined lifecycle fault and empty receipt", receipt, err)
			}
		})
	}
}

func TestActivateStagePathSwapDoesNotDeleteSubstitute(t *testing.T) {
	home, input := activationFixture(t)
	input.Runner = &fakeBatRunner{home: home}
	input.testOps = defaultOperationHooks()
	var moved, substitute string
	input.testOps.beforeStageCleanup = func(stage string) error {
		moved = stage + ".moved"
		substitute = filepath.Join(stage, "do-not-delete")
		if err := os.Rename(stage, moved); err != nil {
			return err
		}
		if err := os.Mkdir(stage, 0o700); err != nil {
			return err
		}
		return os.WriteFile(substitute, []byte("outside-stage"), 0o600)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(moved)
		if substitute != "" {
			_ = os.RemoveAll(filepath.Dir(substitute))
		}
	})
	receipt, err := Activate(context.Background(), input)
	if err != nil || len(receipt.Hashes) != len(generatedFiles) {
		t.Fatalf("Activate() = %+v, %v; descriptor-rooted cleanup should succeed", receipt, err)
	}
	if data, readErr := os.ReadFile(substitute); readErr != nil || string(data) != "outside-stage" {
		t.Fatalf("substituted stage content = %q, %v; cleanup must not delete it", data, readErr)
	}
	entries, readErr := os.ReadDir(moved)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("descriptor-rooted original stage entries = %v, %v; want emptied retained directory", entries, readErr)
	}
}

func TestActivateFailsWhenPinnedStageIsNotEmptyAfterCleanup(t *testing.T) {
	home, input := activationFixture(t)
	input.Runner = &fakeBatRunner{home: home}
	input.testOps = defaultOperationHooks()
	input.testOps.cleanupStage = func(*os.File) error { return nil }
	receipt, err := Activate(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "cleanup left") || len(receipt.Hashes) != 0 {
		t.Fatalf("Activate() = %+v, %v; want nonempty-stage cleanup failure", receipt, err)
	}
	for _, stage := range input.Runner.(*fakeBatRunner).dirs {
		_ = os.RemoveAll(stage)
	}
}

func TestActivateReportsPublisherFaults(t *testing.T) {
	fault := errors.New("injected publisher fault")
	for _, point := range []string{"create", "write", "sync", "close", "rename", "remove", "open-dir", "dir-sync", "dir-close"} {
		t.Run(point, func(t *testing.T) {
			home, input := activationFixture(t)
			input.Runner = &fakeBatRunner{home: home}
			input.testOps = defaultOperationHooks()
			input.testOps.publish = faultPublishOperations{base: realPublishOperations{}, point: point, err: fault}
			receipt, err := Activate(context.Background(), input)
			want := fault
			if point == "write" {
				want = io.ErrShortWrite
			}
			if !errors.Is(err, want) || len(receipt.Hashes) != 0 {
				t.Fatalf("Activate() = %+v, %v; want publisher fault and empty receipt", receipt, err)
			}
		})
	}
}

func TestActivatePinsCacheDirectoryAcrossAncestorSwap(t *testing.T) {
	home, input := activationFixture(t)
	outside := t.TempDir()
	input.Runner = &fakeBatRunner{home: home}
	input.testOps = defaultOperationHooks()
	input.testOps.publish = &swapPublishOperations{
		base: realPublishOperations{},
		home: home, outside: outside,
	}
	receipt, err := Activate(context.Background(), input)
	if err == nil || len(receipt.Hashes) != 0 {
		t.Fatalf("Activate() = %+v, %v; want final confined inventory failure", receipt, err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("outside cache entries = %v, %v", entries, readErr)
	}
	oldCache := filepath.Join(home, ".cache", "bat-pinned")
	if data, readErr := os.ReadFile(filepath.Join(oldCache, generatedFiles[0])); readErr != nil || len(data) == 0 {
		t.Fatalf("pinned cache publication = %q, %v", data, readErr)
	}
}

func TestActivateNativeWriterInterleavingsUseFinalInventoryAsLinearization(t *testing.T) {
	for _, phase := range []string{"before", "between", "after"} {
		t.Run(phase, func(t *testing.T) {
			home, input := activationFixture(t)
			for _, name := range generatedFiles {
				writeTestFile(t, filepath.Join(home, cacheRelative, name), []byte("native-before"))
			}
			input.Runner = &fakeBatRunner{home: home}
			input.testOps = defaultOperationHooks()
			if phase != "before" {
				input.testOps.publish = &interleavingPublishOperations{base: realPublishOperations{}, phase: phase}
			}
			receipt, err := Activate(context.Background(), input)
			if phase == "before" {
				if err != nil || len(receipt.Hashes) != len(generatedFiles) {
					t.Fatalf("Activate() = %+v, %v; prior native state should be replaced", receipt, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "does not match generated output") || len(receipt.Hashes) != 0 {
				t.Fatalf("Activate() = %+v, %v; want final inventory failure", receipt, err)
			}
			data, readErr := os.ReadFile(filepath.Join(home, cacheRelative, "metadata.yaml"))
			if readErr != nil || string(data) != "native-interleaving" {
				t.Fatalf("interleaved native state = %q, %v; it must not be rolled back", data, readErr)
			}
		})
	}
}

func TestActivateSerializesConcurrentCacheActivation(t *testing.T) {
	home, input := activationFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	firstRunner := &fakeBatRunner{home: home, build: func(_ context.Context, target string) error {
		close(entered)
		<-release
		return writeGeneratedOutput(target)
	}}
	input.Runner = firstRunner
	firstDone := make(chan error, 1)
	go func() {
		_, err := Activate(context.Background(), input)
		firstDone <- err
	}()
	<-entered
	second := input
	second.Runner = &fakeBatRunner{home: home}
	if _, err := Activate(context.Background(), second); err == nil || !strings.Contains(err.Error(), "acquire bat cache operation lock") {
		t.Fatalf("concurrent Activate() error = %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Activate() error = %v", err)
	}
}

func writeTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func envValue(env []string, name string) string {
	for _, item := range env {
		if strings.HasPrefix(item, name+"=") {
			return strings.TrimPrefix(item, name+"=")
		}
	}
	return ""
}

func activationFixture(t *testing.T) (string, Input) {
	t.Helper()
	home := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "bat"), []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := []byte("--theme=\"Carbonfox\"\n")
	theme := []byte("<plist>carbonfox</plist>\n")
	writeTestFile(t, filepath.Join(home, configRelative), config)
	writeTestFile(t, filepath.Join(home, themeRelative), theme)
	return home, Input{Home: home, Config: config, Theme: theme, BaseEnv: []string{"PATH=" + bin}}
}

func writeGeneratedOutput(target string) error {
	for _, name := range generatedFiles {
		if err := os.WriteFile(filepath.Join(target, name), []byte("generated-"+name), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func assertNoGeneratedCache(t *testing.T, home string) {
	t.Helper()
	for _, name := range generatedFiles {
		if _, err := os.Lstat(filepath.Join(home, cacheRelative, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("generated cache %s exists after failed activation: %v", name, err)
		}
	}
}

type faultPublishOperations struct {
	base  publishOperations
	point string
	err   error
}

func (o faultPublishOperations) create(dir *os.File, name string) (syncWriteCloser, error) {
	if o.point == "create" {
		return nil, o.err
	}
	file, err := o.base.create(dir, name)
	if err != nil {
		return nil, err
	}
	return &faultPublishFile{syncWriteCloser: file, point: o.point, err: o.err}, nil
}

func (o faultPublishOperations) remove(dir *os.File, name string) error {
	err := o.base.remove(dir, name)
	if o.point == "remove" {
		return errors.Join(err, o.err)
	}
	return err
}

func (o faultPublishOperations) rename(dir *os.File, oldName, newName string) error {
	if o.point == "rename" || o.point == "remove" {
		return o.err
	}
	return o.base.rename(dir, oldName, newName)
}

func (o faultPublishOperations) openDir(dir *os.File) (syncCloser, error) {
	if o.point == "open-dir" {
		return nil, o.err
	}
	opened, err := o.base.openDir(dir)
	if err != nil {
		return nil, err
	}
	return &faultPublishDir{syncCloser: opened, point: o.point, err: o.err}, nil
}

type faultPublishFile struct {
	syncWriteCloser
	point string
	err   error
}

func (f *faultPublishFile) Write(data []byte) (int, error) {
	if f.point == "write" {
		if len(data) == 0 {
			return 0, f.err
		}
		return len(data) - 1, nil
	}
	return f.syncWriteCloser.Write(data)
}

func (f *faultPublishFile) Sync() error {
	if f.point == "sync" {
		return f.err
	}
	return f.syncWriteCloser.Sync()
}

func (f *faultPublishFile) Close() error {
	err := f.syncWriteCloser.Close()
	if f.point == "close" {
		return errors.Join(err, f.err)
	}
	return err
}

type faultPublishDir struct {
	syncCloser
	point string
	err   error
}

func (d *faultPublishDir) Sync() error {
	if d.point == "dir-sync" {
		return d.err
	}
	return d.syncCloser.Sync()
}

func (d *faultPublishDir) Close() error {
	err := d.syncCloser.Close()
	if d.point == "dir-close" {
		return errors.Join(err, d.err)
	}
	return err
}

var _ io.Writer = (*faultPublishFile)(nil)

type swapPublishOperations struct {
	base    publishOperations
	home    string
	outside string
	swapped bool
}

func (o *swapPublishOperations) create(dir *os.File, name string) (syncWriteCloser, error) {
	return o.base.create(dir, name)
}
func (o *swapPublishOperations) remove(dir *os.File, name string) error {
	return o.base.remove(dir, name)
}
func (o *swapPublishOperations) rename(dir *os.File, oldName, newName string) error {
	if !o.swapped {
		o.swapped = true
		cache := filepath.Join(o.home, ".cache", "bat")
		pinned := filepath.Join(o.home, ".cache", "bat-pinned")
		if err := os.Rename(cache, pinned); err != nil {
			return err
		}
		if err := os.Symlink(o.outside, cache); err != nil {
			return err
		}
	}
	return o.base.rename(dir, oldName, newName)
}
func (o *swapPublishOperations) openDir(dir *os.File) (syncCloser, error) {
	return o.base.openDir(dir)
}

type interleavingPublishOperations struct {
	base    publishOperations
	phase   string
	renames int
}

func (o *interleavingPublishOperations) create(dir *os.File, name string) (syncWriteCloser, error) {
	return o.base.create(dir, name)
}
func (o *interleavingPublishOperations) remove(dir *os.File, name string) error {
	return o.base.remove(dir, name)
}
func (o *interleavingPublishOperations) rename(dir *os.File, oldName, newName string) error {
	if err := o.base.rename(dir, oldName, newName); err != nil {
		return err
	}
	o.renames++
	if (o.phase == "between" && o.renames == 1) || (o.phase == "after" && o.renames == len(generatedFiles)) {
		fd, err := unix.Openat(int(dir.Fd()), "metadata.yaml", unix.O_WRONLY|unix.O_TRUNC|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(fd), "metadata.yaml")
		_, writeErr := file.Write([]byte("native-interleaving"))
		syncErr := file.Sync()
		closeErr := file.Close()
		return errors.Join(writeErr, syncErr, closeErr)
	}
	return nil
}
func (o *interleavingPublishOperations) openDir(dir *os.File) (syncCloser, error) {
	return o.base.openDir(dir)
}
