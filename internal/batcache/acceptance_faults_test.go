package batcache

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type nativePathOutputRunner struct {
	base    *fakeBatRunner
	command string
	output  string
}

func (r *nativePathOutputRunner) Run(ctx context.Context, executable string, args, env []string, stdin []byte, dir string) ([]byte, []byte, error) {
	if strings.Join(args, " ") == r.command {
		return []byte(r.output), nil, nil
	}
	return r.base.Run(ctx, executable, args, env, stdin, dir)
}

func TestActivateRejectsInvalidNativePathOutputBeforeBuild(t *testing.T) {
	tests := []struct {
		name    string
		command string
		output  func(string) string
	}{
		{
			name:    "relative config directory",
			command: "--config-dir",
			output:  func(string) string { return ".config/bat\n" },
		},
		{
			name:    "multiline config file",
			command: "--config-file",
			output: func(home string) string {
				path := filepath.Join(home, configRelative)
				return path + "\n" + path + "\n"
			},
		},
		{
			name:    "cache directory outside selected home",
			command: "--cache-dir",
			output: func(home string) string {
				return filepath.Join(filepath.Dir(home), "outside-home", "bat") + "\n"
			},
		},
		{
			name:    "config file inconsistent with config directory",
			command: "--config-file",
			output: func(home string) string {
				return filepath.Join(home, ".config", "other-bat", "config") + "\n"
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home, input := activationFixture(t)
			previous := seedPreviousGeneratedCache(t, home)
			base := &fakeBatRunner{home: home}
			input.Runner = &nativePathOutputRunner{
				base:    base,
				command: test.command,
				output:  test.output(home),
			}

			receipt, err := Activate(context.Background(), input)
			if err == nil || !strings.Contains(err.Error(), test.command) || !strings.Contains(err.Error(), "want selected-home path") {
				t.Fatalf("Activate() error = %v, want invalid %s selected-home path error", err, test.command)
			}
			if receipt.Executable != "" || receipt.CacheDir != "" || len(receipt.Hashes) != 0 {
				t.Fatalf("Activate() receipt = %+v, want empty receipt", receipt)
			}
			if base.buildCalls != 0 {
				t.Fatalf("build calls = %d, want 0", base.buildCalls)
			}
			assertGeneratedCacheEquals(t, home, previous)
		})
	}
}

func TestActivateStagedNativeProofFailurePreservesPreviousCache(t *testing.T) {
	renderCommand := "--color=always --paging=never --style=plain --language=Go -"
	tests := []struct {
		name        string
		command     string
		errorDetail string
	}{
		{name: "list themes", command: "--list-themes", errorDetail: "list bat themes"},
		{name: "render", command: renderCommand, errorDetail: "render bat Carbonfox sample"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home, input := activationFixture(t)
			previous := seedPreviousGeneratedCache(t, home)
			fault := errors.New("injected staged native proof fault")
			runner := &fakeBatRunner{home: home, fail: map[string]error{test.command: fault}}
			input.Runner = runner

			receipt, err := Activate(context.Background(), input)
			if !errors.Is(err, fault) || !strings.Contains(err.Error(), "validate staged bat Carbonfox cache") || !strings.Contains(err.Error(), test.errorDetail) {
				t.Fatalf("Activate() error = %v, want staged %s failure", err, test.name)
			}
			if receipt.Executable != "" || receipt.CacheDir != "" || len(receipt.Hashes) != 0 {
				t.Fatalf("Activate() receipt = %+v, want empty receipt", receipt)
			}
			if runner.buildCalls != 1 {
				t.Fatalf("build calls = %d, want 1", runner.buildCalls)
			}
			if got := countCommand(runner.calls, test.command); got != 1 {
				t.Fatalf("%s calls = %d, want one staged proof call", test.command, got)
			}
			if test.command == "--list-themes" && countCommand(runner.calls, renderCommand) != 0 {
				t.Fatalf("render called after staged theme inventory failure: %v", runner.calls)
			}
			assertGeneratedCacheEquals(t, home, previous)
		})
	}
}

func seedPreviousGeneratedCache(t *testing.T, home string) map[string][]byte {
	t.Helper()
	previous := make(map[string][]byte, len(generatedFiles))
	for _, name := range generatedFiles {
		data := []byte("previous-" + name)
		writeTestFile(t, filepath.Join(home, cacheRelative, name), data)
		previous[name] = data
	}
	return previous
}

func assertGeneratedCacheEquals(t *testing.T, home string, want map[string][]byte) {
	t.Helper()
	for _, name := range generatedFiles {
		got, err := os.ReadFile(filepath.Join(home, cacheRelative, name))
		if err != nil || string(got) != string(want[name]) {
			t.Fatalf("preexisting %s = %q, %v; want %q", name, got, err, want[name])
		}
	}
}

func countCommand(calls []string, command string) int {
	count := 0
	for _, call := range calls {
		if call == command {
			count++
		}
	}
	return count
}
