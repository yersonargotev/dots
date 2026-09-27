package batcache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActivateRejectsCyclicCustomSourceBeforeBuild(t *testing.T) {
	home, input := activationFixture(t)
	themes := filepath.Join(home, ".config", "bat", "themes")
	first := filepath.Join(themes, "cycle-a.tmTheme")
	second := filepath.Join(themes, "cycle-b.tmTheme")
	if err := os.Symlink(filepath.Base(second), first); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(first), second); err != nil {
		t.Fatal(err)
	}

	runner := &fakeBatRunner{home: home}
	input.Runner = runner
	receipt, err := Activate(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "snapshot bat custom themes") || !receiptIsEmpty(receipt) {
		t.Fatalf("Activate() = %+v, %v; want cyclic custom-source snapshot failure", receipt, err)
	}
	if runner.buildCalls != 0 {
		t.Fatalf("build calls = %d, want 0", runner.buildCalls)
	}
	assertNoGeneratedCache(t, home)
}

func TestActivateRejectsUnreadableCustomSourcesBeforeBuild(t *testing.T) {
	t.Run("missing symlink target", func(t *testing.T) {
		home, input := activationFixture(t)
		link := filepath.Join(home, ".config", "bat", "themes", "Unreadable.tmTheme")
		if err := os.Symlink(filepath.Join(t.TempDir(), "missing.tmTheme"), link); err != nil {
			t.Fatal(err)
		}

		runner := &fakeBatRunner{home: home}
		input.Runner = runner
		receipt, err := Activate(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "snapshot bat custom themes") || !receiptIsEmpty(receipt) {
			t.Fatalf("Activate() = %+v, %v; want unreadable custom-source snapshot failure", receipt, err)
		}
		if runner.buildCalls != 0 {
			t.Fatalf("build calls = %d, want 0", runner.buildCalls)
		}
		assertNoGeneratedCache(t, home)
	})

	t.Run("permission denied", func(t *testing.T) {
		home, input := activationFixture(t)
		path := filepath.Join(home, ".config", "bat", "themes", "Unreadable.tmTheme")
		writeTestFile(t, path, []byte("private theme"))
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
		if file, err := os.Open(path); err == nil {
			_ = file.Close()
			t.Skip("current user can read mode-000 files")
		}

		runner := &fakeBatRunner{home: home}
		input.Runner = runner
		receipt, err := Activate(context.Background(), input)
		if err == nil || !strings.Contains(err.Error(), "snapshot bat custom themes") || !receiptIsEmpty(receipt) {
			t.Fatalf("Activate() = %+v, %v; want unreadable custom-source snapshot failure", receipt, err)
		}
		if runner.buildCalls != 0 {
			t.Fatalf("build calls = %d, want 0", runner.buildCalls)
		}
		assertNoGeneratedCache(t, home)
	})
}

func TestActivateRejectsCustomSourceDisappearingDuringNativeBuild(t *testing.T) {
	home, input := activationFixture(t)
	path := filepath.Join(home, ".config", "bat", "themes", "Transient.tmTheme")
	writeTestFile(t, path, []byte("transient theme"))
	runner := &fakeBatRunner{home: home, build: func(_ context.Context, target string) error {
		if err := writeGeneratedOutput(target); err != nil {
			return err
		}
		return os.Remove(path)
	}}
	input.Runner = runner

	receipt, err := Activate(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "reopen custom bat input") || !receiptIsEmpty(receipt) {
		t.Fatalf("Activate() = %+v, %v; want disappearing custom-source failure", receipt, err)
	}
	if runner.buildCalls != 1 {
		t.Fatalf("build calls = %d, want 1", runner.buildCalls)
	}
	assertNoGeneratedCache(t, home)
}

func TestActivateStagesReadableOutsideHomeSymlinkAsRegularFile(t *testing.T) {
	home, input := activationFixture(t)
	outside := filepath.Join(t.TempDir(), "Outside.tmTheme")
	want := []byte("outside custom theme\n")
	writeTestFile(t, outside, want)
	if err := os.Chmod(outside, 0o400); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(home, ".config", "bat", "themes", "Linked.tmTheme")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	base := &fakeBatRunner{home: home}
	input.Runner = &stageInspectRunner{
		CommandRunner: base,
		inspect: func(sourceStage string) error {
			staged := filepath.Join(sourceStage, "themes", "Linked.tmTheme")
			info, err := os.Lstat(staged)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return errors.New("linked custom theme was not staged as a regular file")
			}
			got, err := os.ReadFile(staged)
			if err != nil {
				return err
			}
			if string(got) != string(want) {
				return errors.New("linked custom theme bytes did not reach the stage")
			}
			return nil
		},
	}

	if _, err := Activate(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if base.buildCalls != 1 {
		t.Fatalf("build calls = %d, want 1", base.buildCalls)
	}
	after, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("outside source changed: bytes=%q before=%v after=%v", got, before, after)
	}
}

type stageInspectRunner struct {
	CommandRunner
	inspect func(string) error
}

func receiptIsEmpty(receipt Receipt) bool {
	return receipt.Executable == "" && receipt.CacheDir == "" && len(receipt.Hashes) == 0
}

func (r *stageInspectRunner) Run(ctx context.Context, executable string, args, env []string, stdin []byte, dir string) ([]byte, []byte, error) {
	if strings.Join(args, " ") == "cache --build" {
		if err := r.inspect(envValue(env, "BAT_CONFIG_DIR")); err != nil {
			return nil, nil, err
		}
	}
	return r.CommandRunner.Run(ctx, executable, args, env, stdin, dir)
}
