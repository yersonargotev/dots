package deps

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yersonargotev/dots/internal/manifest"
)

func transactionFixture(t *testing.T, recipe string) (UserLocalInstaller, InstallAction, *int) {
	t.Helper()
	home, state := t.TempDir(), t.TempDir()
	a := UserLocalArtifact{Recipe: recipe, Version: "v1.2.3", Platform: "linux_amd64", Command: recipe, Layout: userLocalRecipes[recipe].layout}
	a.Artifact = recipe + ".tar.gz"
	files := map[string]string{}
	switch recipe {
	case "uv":
		a.Artifact = "uv-x86_64-unknown-linux-gnu.tar.gz"
		files["uv-x86_64-unknown-linux-gnu/uv"] = "#!/bin/sh\nexit 0\n"
		files["uv-x86_64-unknown-linux-gnu/uvx"] = "#!/bin/sh\nexit 0\n"
		files["uv-x86_64-unknown-linux-gnu/empty"] = ""
	case "starship":
		files["starship"] = "#!/bin/sh\nexit 0\n"
	case "codex":
		a.Version = "rust-v1.2.3"
		files["bin/codex"] = "#!/bin/sh\nexit 0\n"
	case "claude":
		a.Artifact = "claude"
	default:
		t.Fatal(recipe)
	}
	data := []byte("#!/bin/sh\nexit 0\n")
	if recipe != "claude" {
		data = tarGz(t, files)
	}
	sum := sha256.Sum256(data)
	a.Checksum = hex.EncodeToString(sum[:])
	if recipe == "codex" || recipe == "claude" {
		a.Digest = "sha256:" + a.Checksum
	}
	a.URL = "https://example.invalid/" + a.Artifact
	a.Destination, a.InstalledPath = userLocalPaths(home, a.Recipe, a.Version, a.Layout, a.Command)
	action := InstallAction{Dependency: recipe, Requirement: manifest.DependencyRequirementRequired, Status: InstallActionStatusInstallable, Probe: a.Command, Probes: []string{a.Command}, Provider: TierUserLocal, UserLocal: &a}
	calls := new(int)
	installer := UserLocalInstaller{Context: context.Background(), Home: home, StateRoot: state, Download: func(url, checksum string) ([]byte, error) {
		*calls++
		if url != a.URL || checksum != a.Checksum {
			t.Errorf("artifact authority changed: %s %s", url, checksum)
		}
		return data, nil
	}}
	return installer, action, calls
}

func loadTransactionOps(t *testing.T, state string) []journalOperation {
	t.Helper()
	root, err := openReceiptRoot(state, false)
	if err != nil {
		t.Fatal(err)
	}
	ops, err := readJournal(root)
	closeErr := root.file.Close()
	if err := errors.Join(err, closeErr); err != nil {
		t.Fatal(err)
	}
	return ops
}

func TestUserLocalTransactionFinalFailureRecoversEntireSurface(t *testing.T) {
	for _, recipe := range []string{"starship", "uv", "codex", "claude"} {
		t.Run(recipe, func(t *testing.T) {
			i, action, downloads := transactionFixture(t, recipe)
			prior := DependencyMetadata{Version: 1, Dependencies: []DependencyRecord{{Dependency: "prior", Provider: "user-local", Path: "/preserve", InstalledAt: "legacy"}}}
			if err := SaveDependencyMetadata(DependencyMetadataPath(i.StateRoot), prior); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(DependencyMetadataPath(i.StateRoot))
			if err != nil {
				t.Fatal(err)
			}
			i.Fault = func(point string) error {
				if point == "final:before" {
					return errors.New("receipt unavailable")
				}
				return nil
			}
			for attempt := 0; attempt < 2; attempt++ {
				if attempt == 0 {
					err = i.RunUserLocal(action, nil)
				} else {
					_, err = i.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)})
				}
				if err == nil {
					t.Fatal("false success before receipt")
				}
				meta, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot))
				if err != nil || len(meta.Dependencies) != 1 {
					t.Fatalf("prior receipt damaged: %+v %v", meta, err)
				}
			}
			if _, err := os.Stat(action.UserLocal.InstalledPath); err != nil {
				t.Fatal(err)
			}
			if recipe == "uv" {
				if _, err := os.Stat(filepath.Join(i.Home, ".local/bin/uvx")); err != nil {
					t.Fatal(err)
				}
			}
			i.Fault = nil
			i.Download = func(string, string) ([]byte, error) { return nil, errors.New("recovery attempted download") }
			actions, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)})
			if err != nil || len(actions) != 1 {
				t.Fatalf("recover: %+v %v", actions, err)
			}
			meta, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot))
			if err != nil || len(meta.Dependencies) != 2 {
				t.Fatalf("final receipt: %+v %v", meta, err)
			}
			var got DependencyRecord
			for _, r := range meta.Dependencies {
				if r.Dependency == recipe {
					got = r
				}
			}
			if got.Version != action.UserLocal.Version || got.URL != action.UserLocal.URL || got.Checksum != action.UserLocal.Checksum || got.Platform != action.UserLocal.Platform || got.OperationID == "" {
				t.Fatalf("wrong artifact receipt %+v", got)
			}
			want := 1
			if action.UserLocal.Layout == userLocalLayoutBundle {
				want = 1 + len(userLocalRecipes[recipe].links)
			}
			if len(got.Publications) != want {
				t.Fatalf("surface %+v", got.Publications)
			}
			actions, err = i.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)})
			if err != nil || len(actions) != 0 {
				t.Fatalf("second recovery not noop: %+v %v", actions, err)
			}
			if *downloads != 1 {
				t.Fatalf("downloads %d", *downloads)
			}
			after, err := os.ReadFile(DependencyMetadataPath(i.StateRoot))
			if err != nil || string(before) != string(after) {
				t.Fatal("legacy bytes rewritten")
			}
		})
	}
}

func TestUserLocalTransactionExternalChangesFailClosed(t *testing.T) {
	for _, change := range []string{"command-file", "command-link", "bundle-file", "bundle-link", "same-bytes-new-inode", "missing-published", "empty-file"} {
		t.Run(change, func(t *testing.T) {
			i, action, _ := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == "final:before" {
					return errors.New("stop")
				}
				return nil
			}
			if err := i.RunUserLocal(action, nil); err == nil {
				t.Fatal("expected pending")
			}
			path := action.UserLocal.InstalledPath
			switch change {
			case "command-file":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("foreign"), 0755); err != nil {
					t.Fatal(err)
				}
			case "command-link":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("foreign", path); err != nil {
					t.Fatal(err)
				}
			case "bundle-file":
				path = filepath.Join(action.UserLocal.Destination, "uv-x86_64-unknown-linux-gnu/uvx")
				if err := os.WriteFile(path, []byte("foreign"), 0755); err != nil {
					t.Fatal(err)
				}
			case "bundle-link":
				path = action.UserLocal.Destination
				if err := os.Rename(path, path+"-saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+"-saved", path); err != nil {
					t.Fatal(err)
				}
			case "same-bytes-new-inode":
				path = filepath.Join(action.UserLocal.Destination, "uv-x86_64-unknown-linux-gnu/uvx")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0755); err != nil {
					t.Fatal(err)
				}
			case "missing-published":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "empty-file":
				path = filepath.Join(action.UserLocal.Destination, "uv-x86_64-unknown-linux-gnu/empty")
				if err := os.WriteFile(path, []byte("foreign"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			root, err := openReceiptRoot(i.Home, false)
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(i.Home, path)
			if err != nil {
				t.Fatal(err)
			}
			before, beforeErr := root.evidence(rel, false)
			i.Fault = nil
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)}); err == nil {
				t.Fatal("adopted external state")
			}
			after, afterErr := root.evidence(rel, false)
			if err := root.file.Close(); err != nil {
				t.Fatal(err)
			}
			if (beforeErr == nil) != (afterErr == nil) || !sameEvidence(before, after) {
				t.Fatal("external state changed")
			}
			ops := loadTransactionOps(t, i.StateRoot)
			if len(ops) != 1 || ops[0].Complete {
				t.Fatal("pending evidence lost")
			}
		})
	}
}

func TestUserLocalTransactionPartialPublicationAndCapturedRetry(t *testing.T) {
	for _, stop := range []string{"after-captured", "after-unit-0", "after-unit-1", "after-final", "final:dir-sync", "complete:before"} {
		t.Run(stop, func(t *testing.T) {
			i, action, _ := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == stop {
					return errors.New("interrupted")
				}
				return nil
			}
			if err := i.RunUserLocal(action, nil); err == nil {
				t.Fatal("expected interruption")
			}
			if stop != "after-captured" {
				i.Download = func(string, string) ([]byte, error) { return nil, errors.New("download after ready") }
			}
			i.Fault = nil
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)}); err != nil {
				t.Fatal(err)
			}
			ops := loadTransactionOps(t, i.StateRoot)
			if !ops[0].Complete {
				t.Fatal("not complete")
			}
			for _, command := range []string{"uv", "uvx"} {
				if _, err := os.Stat(filepath.Join(i.Home, ".local/bin", command)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestUserLocalTransactionFaultsPreventFalseSuccess(t *testing.T) {
	for _, point := range []string{"captured:before", "captured:write", "captured:sync", "captured:link", "captured:dir-sync", "ready:write", "ready:sync", "ready:link", "ready:dir-sync", "before-unit-0", "unit-0:write", "final:write", "final:sync", "final:link", "complete:write", "complete:sync", "complete:dir-sync"} {
		t.Run(point, func(t *testing.T) {
			i, action, _ := transactionFixture(t, "starship")
			i.Fault = func(actual string) error {
				if actual == point {
					return errors.New("injected")
				}
				return nil
			}
			if err := i.RunUserLocal(action, nil); err == nil {
				t.Fatal("false success")
			}
			if strings.HasPrefix(point, "captured:") || strings.HasPrefix(point, "ready:") || point == "before-unit-0" {
				if _, err := os.Lstat(action.UserLocal.InstalledPath); !os.IsNotExist(err) {
					t.Fatalf("published early: %v", err)
				}
			}
		})
	}
	i, action, _ := transactionFixture(t, "starship")
	i.write = func(f *os.File, data []byte) (int, error) { return f.Write(data[:len(data)/2]) }
	if err := i.RunUserLocal(action, nil); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write: %v", err)
	}
}

func TestUserLocalTransactionPostFinalEditBeforeAcknowledgement(t *testing.T) {
	i, action, _ := transactionFixture(t, "uv")
	i.Fault = func(point string) error {
		if point == "after-final" {
			return errors.New("lost final acknowledgement")
		}
		return nil
	}
	if err := i.RunUserLocal(action, nil); err == nil {
		t.Fatal("expected error")
	}
	if err := os.Remove(action.UserLocal.InstalledPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("external", action.UserLocal.InstalledPath); err != nil {
		t.Fatal(err)
	}
	i.Fault = nil
	if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)}); err == nil {
		t.Fatal("acknowledged foreign state")
	}
	meta, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot))
	if err != nil || len(meta.Dependencies) != 0 {
		t.Fatalf("false receipt %+v %v", meta, err)
	}
}

func TestUserLocalTransactionFreshProcessRecovery(t *testing.T) {
	if os.Getenv("DOTS_RECEIPT_TEST_HELPER") == "1" {
		i := UserLocalInstaller{Home: os.Getenv("DOTS_RECEIPT_HOME"), StateRoot: os.Getenv("DOTS_RECEIPT_STATE"), Download: func(string, string) ([]byte, error) { return nil, errors.New("fresh process downloaded") }}
		actions, err := i.RecoverUserLocal([]UserLocalSelection{{Dependency: "uv", Kind: "fixed", Recipe: "uv", Platform: "linux_amd64", Requirement: manifest.DependencyRequirementRequired, Probes: "uv"}})
		if err != nil || len(actions) != 1 {
			t.Fatalf("fresh recovery %+v %v", actions, err)
		}
		return
	}
	i, action, _ := transactionFixture(t, "uv")
	i.Fault = func(point string) error {
		if point == "final:before" {
			return errors.New("receipt failure")
		}
		return nil
	}
	if err := i.RunUserLocal(action, nil); err == nil {
		t.Fatal("expected error")
	}
	command := exec.Command(os.Args[0], "-test.run=^TestUserLocalTransactionFreshProcessRecovery$", "-test.v")
	command.Env = append(os.Environ(), "DOTS_RECEIPT_TEST_HELPER=1", "DOTS_RECEIPT_HOME="+i.Home, "DOTS_RECEIPT_STATE="+i.StateRoot, "PATH="+filepath.Join(i.Home, ".local/bin"))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fresh process %v\n%s", err, output)
	}
	if !loadTransactionOps(t, i.StateRoot)[0].Complete {
		t.Fatal("fresh process did not finalize")
	}
}

func TestUserLocalTransactionConcurrentReceiptsPreserveLegacy(t *testing.T) {
	i, a, _ := transactionFixture(t, "uv")
	j, b, _ := transactionFixture(t, "starship")
	j.Home, j.StateRoot = i.Home, i.StateRoot
	b.UserLocal.Destination, b.UserLocal.InstalledPath = userLocalPaths(j.Home, b.UserLocal.Recipe, b.UserLocal.Version, b.UserLocal.Layout, b.UserLocal.Command)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, pair := range []struct {
		i UserLocalInstaller
		a InstallAction
	}{{i, a}, {j, b}} {
		wg.Add(1)
		go func(i UserLocalInstaller, a InstallAction) { defer wg.Done(); results <- i.RunUserLocal(a, nil) }(pair.i, pair.a)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	meta, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot))
	if err != nil || len(meta.Dependencies) != 2 {
		t.Fatalf("lost concurrent receipt %+v %v", meta, err)
	}
}

type realTransactionRunner struct{ installer *UserLocalInstaller }

func (r realTransactionRunner) Run(string, []string) error {
	return errors.New("unexpected package manager")
}
func (r realTransactionRunner) RunUserLocal(action InstallAction, present func() bool) error {
	return r.installer.RunUserLocal(action, present)
}
func (r realTransactionRunner) RecoverUserLocal(selected []UserLocalSelection) ([]InstallAction, error) {
	return r.installer.RecoverUserLocal(selected)
}

func transactionManifest(action InstallAction) manifest.Manifest {
	a := action.UserLocal
	dep := manifest.Dependency{Name: action.Dependency, Command: a.Command, UserLocal: &manifest.UserLocalProvider{Recipe: a.Recipe, Version: a.Version, Checksum: a.Checksum}}
	if a.Digest != "" {
		dep.UserLocal = nil
		dep.RollingUserLocal = &manifest.RollingUserLocalProvider{Recipe: a.Recipe}
	}
	return manifest.Manifest{Version: 1, Profiles: map[string]manifest.Profile{"default": {Tags: []string{"test"}}}, Entries: []manifest.Entry{{Source: "fixture", Target: "fixture", Tags: []string{"test"}, Dependencies: []manifest.Dependency{dep}}}}
}

func TestUserLocalTransactionSharedInstallRecoversBeforePATHSkip(t *testing.T) {
	for _, recipe := range []string{"uv", "codex"} {
		t.Run(recipe, func(t *testing.T) {
			i, action, _ := transactionFixture(t, recipe)
			opts := Options{Profile: "default", OS: "linux", Arch: "amd64", Home: i.Home, StateRoot: i.StateRoot}
			m := transactionManifest(action)
			// The command was satisfied at preview time. The execution must still retain
			// its declaration and detect an operation that appeared after that preview.
			look := func(string) bool { return true }
			prepared, err := PrepareInstall(m, opts, look, func(string) bool { return false }, TierGeneric)
			if err != nil || len(prepared.Plan.Actions) != 0 {
				t.Fatalf("preview %+v %v", prepared, err)
			}
			i.Fault = func(point string) error {
				if point == "final:before" {
					return errors.New("receipt failure")
				}
				return nil
			}
			if err := i.RunUserLocal(action, nil); err == nil {
				t.Fatal("expected receipt failure")
			}
			// Read-only planning reads captured authority without resolving latest.
			opsBefore := loadTransactionOps(t, i.StateRoot)
			opts.RollingReleaseURL = "https://example.invalid/must-not-resolve"
			dry, err := InstallDryRun(m, opts, look, func(string) bool { return false }, TierGeneric)
			if err != nil || len(dry.Items) != 1 || dry.Items[0].UserLocal.URL != action.UserLocal.URL {
				t.Fatalf("read-only recovery preview %+v %v", dry, err)
			}
			if !sameReceipt(opsBefore, loadTransactionOps(t, i.StateRoot)) {
				t.Fatal("dry run mutated journal")
			}
			i.Fault = nil
			i.Download = func(string, string) ([]byte, error) { return nil, errors.New("unexpected download") }
			report, err := InstallPrepared(prepared, opts, look, func(string) bool { return false }, realTransactionRunner{&i})
			if err != nil || len(report.Items) != 1 {
				t.Fatalf("execution omitted recovery %+v %v", report, err)
			}
			report, err = Install(m, opts, look, func(string) bool { return false }, TierGeneric, realTransactionRunner{&i})
			if err != nil || len(report.Items) != 0 {
				t.Fatalf("finalized retry not noop %+v %v", report, err)
			}
		})
	}
}

func TestUserLocalTransactionRecoveryScopeAndDeclaration(t *testing.T) {
	i, action, _ := transactionFixture(t, "uv")
	i.Fault = func(point string) error {
		if point == "final:before" {
			return errors.New("receipt failure")
		}
		return nil
	}
	if err := i.RunUserLocal(action, nil); err == nil {
		t.Fatal("expected failure")
	}
	i.Fault = nil
	if actions, err := i.RecoverUserLocal([]UserLocalSelection{{Dependency: "unselected"}}); err != nil || len(actions) != 0 {
		t.Fatalf("unselected recovery %+v %v", actions, err)
	}
	other := i
	other.Home = t.TempDir()
	if actions, err := other.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)}); err != nil || len(actions) != 0 {
		t.Fatalf("other home recovery %+v %v", actions, err)
	}
	other = i
	other.StateRoot = t.TempDir()
	if actions, err := other.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)}); err != nil || len(actions) != 0 {
		t.Fatalf("other state recovery %+v %v", actions, err)
	}
	changed := artifactSelection(action)
	changed.Recipe = "starship"
	if _, err := i.RecoverUserLocal([]UserLocalSelection{changed}); err == nil {
		t.Fatal("conflicting declaration accepted")
	}
	if loadTransactionOps(t, i.StateRoot)[0].Complete {
		t.Fatal("scoped checks mutated pending")
	}
}

func TestUserLocalTransactionCorruptionFailsClosed(t *testing.T) {
	for _, phase := range []string{"captured", "ready", "unit-0", "final", "complete"} {
		t.Run(phase, func(t *testing.T) {
			i, action, _ := transactionFixture(t, "uv")
			if err := i.RunUserLocal(action, nil); err != nil {
				t.Fatal(err)
			}
			op := loadTransactionOps(t, i.StateRoot)[0]
			path := filepath.Join(i.StateRoot, operationPath(op.Captured.ID, phase))
			if err := os.WriteFile(path, []byte("{corrupt"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot)); err == nil {
				t.Fatal("corrupt receipt accepted")
			}
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)}); err == nil {
				t.Fatal("corrupt operation adopted")
			}
			if _, err := os.Stat(action.UserLocal.InstalledPath); err != nil {
				t.Fatal("corruption changed public bytes")
			}
		})
	}
}

func TestUserLocalTransactionExternalInterleavingsAndCancellation(t *testing.T) {
	for _, point := range []string{"after-captured", "before-unit-0", "before-unit-1", "final:before"} {
		t.Run(point, func(t *testing.T) {
			i, action, _ := transactionFixture(t, "uv")
			path := action.UserLocal.InstalledPath
			i.Fault = func(actual string) error {
				if actual != point {
					return nil
				}
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
				return os.Symlink("foreign", path)
			}
			if err := i.RunUserLocal(action, nil); err == nil {
				t.Fatal("external interleaving adopted")
			}
			got, err := os.Readlink(path)
			if err != nil || got != "foreign" {
				t.Fatalf("foreign link changed %q %v", got, err)
			}
		})
	}
	i, action, _ := transactionFixture(t, "uv")
	ctx, cancel := context.WithCancel(context.Background())
	i.Context = ctx
	i.Fault = func(point string) error {
		if point == "after-unit-0" {
			cancel()
		}
		return nil
	}
	if err := i.RunUserLocal(action, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation %v", err)
	}
	i.Context = context.Background()
	i.Fault = nil
	if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(action)}); err != nil {
		t.Fatal(err)
	}
}

func TestUserLocalTransactionParentAndSourceSubstitution(t *testing.T) {
	for _, kind := range []string{"bin-parent", "staged-source"} {
		t.Run(kind, func(t *testing.T) {
			i, action, _ := transactionFixture(t, "uv")
			outside := t.TempDir()
			i.Fault = func(point string) error {
				if point != "before-unit-0" {
					return nil
				}
				if kind == "bin-parent" {
					bin := filepath.Join(i.Home, ".local/bin")
					if err := os.Rename(bin, bin+"-saved"); err != nil {
						return err
					}
					return os.Symlink(outside, bin)
				}
				ops := loadTransactionOps(t, i.StateRoot)
				source := filepath.Join(i.Home, ops[0].Ready.Units[0].Source, "uv-x86_64-unknown-linux-gnu/uvx")
				return os.WriteFile(source, []byte("foreign source"), 0755)
			}
			if err := i.RunUserLocal(action, nil); err == nil {
				t.Fatal("substituted authority accepted")
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatal("escaped selected home")
			}
			if kind == "bin-parent" {
				if target, err := os.Readlink(filepath.Join(i.Home, ".local/bin")); err != nil || target != outside {
					t.Fatal("external parent changed")
				}
			}
		})
	}
}

func TestUserLocalTransactionOptionalFailureIsAnExecutionError(t *testing.T) {
	i, action, _ := transactionFixture(t, "uv")
	action.Requirement = manifest.DependencyRequirementOptional
	i.Fault = func(point string) error {
		if point == "final:before" {
			return errors.New("receipt failure")
		}
		return nil
	}
	prepared := PreparedInstall{Plan: PlanReport{Actions: []InstallAction{action}, selected: []UserLocalSelection{artifactSelection(action)}}}
	report, err := InstallPrepared(prepared, Options{OS: "linux", Home: i.Home, StateRoot: i.StateRoot}, func(string) bool { return false }, func(string) bool { return false }, realTransactionRunner{&i})
	if err == nil || len(report.Items) != 1 || report.Items[0].Status != InstallStatusFailed {
		t.Fatalf("optional receipt error swallowed: %+v %v", report, err)
	}
}

func TestUserLocalTransactionExternalCapabilityDoesNotCreateReceipt(t *testing.T) {
	i, action, downloads := transactionFixture(t, "codex")
	opts := Options{Profile: "default", OS: "linux", Arch: "amd64", Home: i.Home, StateRoot: i.StateRoot, RollingReleaseURL: "https://example.invalid/no-resolution"}
	report, err := Install(transactionManifest(action), opts, func(string) bool { return true }, func(string) bool { return false }, TierGeneric, realTransactionRunner{&i})
	if err != nil || len(report.Items) != 0 || *downloads != 0 {
		t.Fatalf("external capability mutated %+v %v downloads=%d", report, err, *downloads)
	}
	entries, err := os.ReadDir(i.StateRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "dependencies.lock" {
			t.Fatal("invented receipt for external command")
		}
	}
}

// This opt-in helper creates persistent sandbox state for the built-binary
// manual gate. CI skips it; normal lifecycle tests above own their temp roots.
func TestUserLocalReceiptBinaryFixture(t *testing.T) {
	root := os.Getenv("DOTS_RECEIPT_MANUAL_ROOT")
	if root == "" {
		t.Skip("manual sandbox fixture helper")
	}
	i, action, _ := transactionFixture(t, "codex")
	i.Home = filepath.Join(root, "home")
	i.StateRoot = filepath.Join(root, "state")
	if err := os.MkdirAll(i.Home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(i.StateRoot, 0700); err != nil {
		t.Fatal(err)
	}
	a := action.UserLocal
	a.Platform = platformKey(runtime.GOOS, runtime.GOARCH)
	a.Destination, a.InstalledPath = userLocalPaths(i.Home, a.Recipe, a.Version, a.Layout, a.Command)
	prior := DependencyMetadata{Version: 1, Dependencies: []DependencyRecord{{Dependency: "prior", Provider: "user-local", Path: filepath.Join(i.Home, ".local/bin/prior"), InstalledAt: "2026-01-01T00:00:00Z"}}}
	if err := SaveDependencyMetadata(DependencyMetadataPath(i.StateRoot), prior); err != nil {
		t.Fatal(err)
	}
	i.Fault = func(point string) error {
		if point == "final:before" {
			return errors.New("manual receipt failure fixture")
		}
		return nil
	}
	if err := i.RunUserLocal(action, nil); err == nil {
		t.Fatal("fixture unexpectedly finalized")
	}
	body := `version: 1
profiles:
  default:
    tags: [agents]
entries:
  - source: fixture.txt
    target: ~/.config/receipt-manual.txt
    strategy: copy
    tags: [agents]
    dependencies:
      - name: codex
        command: codex
        rolling_user_local:
          recipe: codex
`
	if err := os.WriteFile(filepath.Join(root, "dots.yaml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixture.txt"), []byte("sandbox configuration\n"), 0600); err != nil {
		t.Fatal(err)
	}
}

func (r realTransactionRunner) WithUserLocalBoundary(selected []UserLocalSelection, consume func([]InstallAction) error) error {
	return r.installer.WithUserLocalBoundary(selected, consume)
}

func TestUserLocalTransactionReDurabilizesVisiblePhases(t *testing.T) {
	for _, phase := range []string{"authority", "captured", "ready", "final", "complete"} {
		t.Run(phase, func(t *testing.T) {
			i, a, downloads := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == phase+":dir-sync" {
					return errors.New("directory sync failed")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("expected sync error")
			}
			i.Fault = func(point string) error {
				if point == "replay:operation-dir-sync" || (phase == "authority" && strings.HasPrefix(point, "replay:authority-") && strings.HasSuffix(point, ":file-sync")) {
					return errors.New("replay sync failed")
				}
				return nil
			}
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); err == nil {
				t.Fatal("relied on non-durable visible phase")
			}
			if phase == "authority" || phase == "captured" || phase == "ready" {
				if _, err := os.Lstat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
					t.Fatal("published before durable authority")
				}
			}
			if phase == "authority" {
				if *downloads != 0 {
					t.Fatal("downloaded before canonical authority replay sync")
				}
				ops := loadTransactionOps(t, i.StateRoot)
				if ops[0].CapturedProjection || ops[0].Ready != nil {
					t.Fatal("repaired or staged before authority replay sync")
				}
			}
			i.Fault = nil
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUserLocalTransactionCreationAndLockSyncFailures(t *testing.T) {
	for _, point := range []string{"scaffold:ancestor-sync", "mkdir:bin:parent-sync", "lock:.dots-dependencies.lock:sync", "lock:.dots-dependencies.lock:parent-sync", "lock:dependencies.lock:sync", "lock:dependencies.lock:parent-sync"} {
		t.Run(point, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			i.Fault = func(actual string) error {
				if actual == point {
					return errors.New("durability failure")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("ignored scaffold durability failure")
			}
			if _, err := os.Lstat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
				t.Fatal("published after scaffold failure")
			}
		})
	}
}

func TestUserLocalTransactionHeldLockSubstitution(t *testing.T) {
	for _, scope := range []string{"home", "state"} {
		t.Run(scope, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			name := ".dots-dependencies.lock"
			path := filepath.Join(i.Home, ".local", name)
			if scope == "state" {
				name = "dependencies.lock"
				path = filepath.Join(i.StateRoot, name)
			}
			i.Fault = func(point string) error {
				if point != "lock:"+name+":held" {
					return nil
				}
				if err := os.Rename(path, path+"-held"); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("external lock"), 0600)
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("captured replacement instead of held lock")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "external lock" {
				t.Fatal("foreign lock modified")
			}
		})
	}
}

func TestUserLocalTransactionLatePublicAndPrivateSubstitution(t *testing.T) {
	for _, scope := range []string{"source", "destination"} {
		t.Run(scope, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "starship")
			point := "source:before-consume"
			if scope == "destination" {
				point = "destination:before-consume"
			}
			path := ""
			i.Fault = func(actual string) error {
				if actual != point {
					return nil
				}
				path = a.UserLocal.InstalledPath
				if scope == "source" {
					ops := loadTransactionOps(t, i.StateRoot)
					path = filepath.Join(i.Home, ops[0].Ready.Units[0].Source)
					if err := os.Remove(path); err != nil {
						return err
					}
				}
				return os.WriteFile(path, []byte("external"), 0755)
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("late replacement accepted")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != "external" {
				t.Fatal("foreign bytes overwritten")
			}
		})
	}
}

func TestUserLocalTransactionCrossStateBoundarySerializesAbsence(t *testing.T) {
	i, a, _ := transactionFixture(t, "uv")
	j, b, _ := transactionFixture(t, "starship")
	j.Home = i.Home
	b.UserLocal.Destination, b.UserLocal.InstalledPath = userLocalPaths(j.Home, b.UserLocal.Recipe, b.UserLocal.Version, b.UserLocal.Layout, b.UserLocal.Command)
	contended, finished := make(chan struct{}), make(chan error, 1)
	var once sync.Once
	j.Fault = func(point string) error {
		if point == "lock:.dots-dependencies.lock:contended" {
			once.Do(func() { close(contended) })
		}
		return nil
	}
	i.Fault = func(point string) error {
		if point != "after-recovery-read" {
			return nil
		}
		go func() { finished <- j.RunUserLocal(b, nil) }()
		select {
		case <-contended:
		case <-time.After(2 * time.Second):
			return errors.New("second root did not contend on held home lock")
		}
		return nil
	}
	err := i.WithUserLocalBoundary([]UserLocalSelection{artifactSelection(a)}, func(actions []InstallAction) error {
		if len(actions) != 0 {
			t.Fatal("unexpected initial pending")
		}
		select {
		case err := <-finished:
			t.Fatalf("second root escaped held lock: %v", err)
		default:
		}
		if _, err := os.Stat(b.UserLocal.InstalledPath); !os.IsNotExist(err) {
			t.Fatal("published after negative check before presence decision")
		}
		return i.RunUserLocal(a, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{i.StateRoot, j.StateRoot} {
		meta, err := LoadDependencyMetadata(DependencyMetadataPath(state))
		if err != nil || len(meta.Dependencies) != 1 {
			t.Fatalf("cross-root receipt %+v %v", meta, err)
		}
	}
}

func TestUserLocalReceiptChainConflicts(t *testing.T) {
	id := func(n byte) string { return strings.Repeat(string(n), 32) }
	op := func(n byte, prior string, complete bool) journalOperation {
		return journalOperation{Captured: capturedReceipt{ID: id(n), Ordinal: uint64(n), Previous: prior, Home: "/selected", Action: InstallAction{Dependency: "uv"}}, Complete: complete}
	}
	tests := map[string][]journalOperation{
		"missing-prior":       {op('a', id('b'), false)},
		"divergent-successor": {op('a', "", true), op('b', id('a'), true), op('c', id('a'), false)},
		"multiple-pending":    {op('a', "", false), op('b', "", false)},
		"cycle":               {op('a', id('b'), true), op('b', id('a'), true)},
	}
	for name, ops := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateReceiptChains(ops); err == nil {
				t.Fatal("conflicting receipt chain accepted")
			}
		})
	}
}

func TestUserLocalTransactionCancellationAtPreparationGates(t *testing.T) {
	for _, point := range []string{"before-stage", "after-download", "extract", "before-unit-0"} {
		t.Run(point, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			i.Context = ctx
			i.Fault = func(actual string) error {
				if actual == point {
					cancel()
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation ignored: %v", err)
			}
			if _, err := os.Lstat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
				t.Fatal("published after cancellation")
			}
			if point == "before-stage" {
				matches, err := filepath.Glob(filepath.Join(i.Home, ".local/.dots-deps-*"))
				if err != nil || len(matches) != 0 {
					t.Fatal("staged after cancellation")
				}
			}
		})
	}
}

func TestUserLocalPrivateModesWithRestrictiveUmask(t *testing.T) {
	if os.Getenv("DOTS_PRIVATE_MODE_HELPER") == "1" {
		i, a, _ := transactionFixture(t, "starship")
		i.Home = os.Getenv("DOTS_MODE_HOME")
		i.StateRoot = os.Getenv("DOTS_MODE_STATE")
		a.UserLocal.Destination, a.UserLocal.InstalledPath = userLocalPaths(i.Home, a.UserLocal.Recipe, a.UserLocal.Version, a.UserLocal.Layout, a.UserLocal.Command)
		old := unix.Umask(0777)
		defer unix.Umask(old)
		if err := i.RunUserLocal(a, nil); err != nil {
			t.Fatal(err)
		}
		for _, base := range []string{filepath.Join(i.StateRoot, receiptJournal), filepath.Join(i.Home, ".local")} {
			err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if strings.Contains(path, ".dots-deps-") || strings.Contains(path, receiptJournal) {
					info, err := entry.Info()
					if err != nil {
						return err
					}
					if entry.Type()&os.ModeSymlink != 0 {
						return nil
					}
					if entry.IsDir() {
						if info.Mode().Perm() != 0700 {
							return fmt.Errorf("private directory mode %s %o", path, info.Mode().Perm())
						}
					} else if strings.Contains(path, receiptJournal) && info.Mode().Perm() != 0600 {
						return fmt.Errorf("private receipt mode %o", info.Mode().Perm())
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	home, state := t.TempDir(), t.TempDir()
	for _, path := range []string{".local/bin", ".local/opt/starship"} {
		if err := os.MkdirAll(filepath.Join(home, path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(home, path), 0755); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command(os.Args[0], "-test.run=^TestUserLocalPrivateModesWithRestrictiveUmask$", "-test.v")
	command.Env = append(os.Environ(), "DOTS_PRIVATE_MODE_HELPER=1", "DOTS_MODE_HOME="+home, "DOTS_MODE_STATE="+state)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("restrictive umask %v\n%s", err, output)
	}
	for _, path := range []string{".local/bin", ".local/opt/starship"} {
		info, err := os.Stat(filepath.Join(home, path))
		if err != nil || info.Mode().Perm() != 0755 {
			t.Fatal("public directory mode changed")
		}
	}
}

func TestUserLocalDownloadCancellationClosesRequest(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(stopped)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := downloadAndVerifyContext(ctx, server.URL, strings.Repeat("0", 64)); result <- err }()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("HTTP cancellation %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("download ignored cancellation")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("request resource not closed")
	}
}

func TestUserLocalTransactionLateParentAndHeldLockChange(t *testing.T) {
	for _, kind := range []string{"parent", "home-lock", "state-lock"} {
		t.Run(kind, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "starship")
			point := "source:before-consume"
			path := filepath.Join(i.Home, ".local/bin")
			if kind == "home-lock" {
				point = "after-captured"
				path = filepath.Join(i.Home, ".local/.dots-dependencies.lock")
			}
			if kind == "state-lock" {
				point = "after-captured"
				path = filepath.Join(i.StateRoot, "dependencies.lock")
			}
			i.Fault = func(actual string) error {
				if actual != point {
					return nil
				}
				if err := os.Rename(path, path+"-saved"); err != nil {
					return err
				}
				if kind == "parent" {
					return os.Mkdir(path, 0755)
				}
				return os.WriteFile(path, []byte("external"), 0600)
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("changed parent/lock authority accepted")
			}
			if _, err := os.Stat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
				t.Fatal("published with changed authority")
			}
		})
	}
}

func TestUserLocalTransactionPreservesExistingDirectoryModes(t *testing.T) {
	i, a, _ := transactionFixture(t, "starship")
	bin := filepath.Join(i.Home, ".local/bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bin, 0750); err != nil {
		t.Fatal(err)
	}
	if err := i.RunUserLocal(a, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(bin)
	if err != nil || info.Mode().Perm() != 0750 {
		t.Fatal("existing public permissions normalized")
	}
}

func TestUserLocalTransactionSourceParentRelocationAtConsume(t *testing.T) {
	i, a, _ := transactionFixture(t, "starship")
	foreign := ""
	i.Fault = func(point string) error {
		if point != "source:before-consume" {
			return nil
		}
		ops := loadTransactionOps(t, i.StateRoot)
		source := filepath.Join(i.Home, ops[0].Ready.Units[0].Source)
		stage := filepath.Join(i.Home, ".local/.dots-deps-"+ops[0].Captured.ID)
		if err := os.Rename(stage, stage+"-moved"); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
			return err
		}
		foreign = source
		return os.WriteFile(source, []byte("foreign stage"), 0755)
	}
	if err := i.RunUserLocal(a, nil); err == nil {
		t.Fatal("relocated private source parent accepted")
	}
	data, err := os.ReadFile(foreign)
	if err != nil || string(data) != "foreign stage" {
		t.Fatal("foreign stage changed")
	}
	if _, err := os.Stat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
		t.Fatal("published from relocated source")
	}
}

func TestUserLocalPrivateModeCorruptionFailsReadOnlyAndMutating(t *testing.T) {
	for _, kind := range []string{"journal", "operation", "sticky-operation", "special-record"} {
		t.Run(kind, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == "final:before" {
					return errors.New("pending")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("expected pending")
			}
			op := loadTransactionOps(t, i.StateRoot)[0]
			path := filepath.Join(i.StateRoot, receiptJournal)
			mode := os.FileMode(0755)
			if kind != "journal" {
				path = filepath.Join(path, op.Captured.ID)
			}
			if kind == "sticky-operation" {
				mode = 0700 | os.ModeSticky
			}
			if kind == "special-record" {
				path = filepath.Join(path, "ready.json")
				mode = 0600 | os.ModeSetuid
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot)); err == nil {
				t.Fatal("read accepted contradictory private mode")
			}
			i.Fault = nil
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); err == nil {
				t.Fatal("mutation accepted contradictory private mode")
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != mode.Perm() || info.Mode()&(os.ModeSetuid|os.ModeSticky) != mode&(os.ModeSetuid|os.ModeSticky) {
				t.Fatal("corrupt private mode rewritten")
			}
		})
	}
}

func TestUserLocalReceiptCloseFailures(t *testing.T) {
	for _, phase := range []string{"captured", "ready", "final", "complete"} {
		t.Run(phase, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == phase+":close" {
					return errors.New("close failed")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("ignored close error")
			}
			meta, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot))
			if err != nil || len(meta.Dependencies) != 0 {
				t.Fatalf("false final receipt %+v %v", meta, err)
			}
			i.Fault = nil
			if phase == "captured" {
				if err := i.RunUserLocal(a, nil); err != nil {
					t.Fatal(err)
				}
			} else if phase == "ready" {
				if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); err == nil {
					t.Fatal("adopted ambiguous preparation after close failure")
				}
				if _, err := os.Stat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
					t.Fatal("published ambiguous preparation")
				}
			} else {
				if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestUserLocalOnDiskReceiptChainConflictsPreserveState(t *testing.T) {
	for _, kind := range []string{"missing-prior", "divergent-successor", "multiple-pending", "cycle"} {
		t.Run(kind, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "starship")
			prior := DependencyMetadata{Version: 1, Dependencies: []DependencyRecord{{Dependency: "prior", Provider: "external", Path: "/preserved", InstalledAt: "legacy"}}}
			if err := SaveDependencyMetadata(DependencyMetadataPath(i.StateRoot), prior); err != nil {
				t.Fatal(err)
			}
			legacy, err := os.ReadFile(DependencyMetadataPath(i.StateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if err := i.RunUserLocal(a, nil); err != nil {
				t.Fatal(err)
			}
			original := loadTransactionOps(t, i.StateRoot)[0]
			before, err := os.ReadFile(a.UserLocal.InstalledPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(i.StateRoot, receiptJournal)); err != nil {
				t.Fatal(err)
			}
			type link struct {
				id, previous string
				complete     bool
			}
			id := func(c string) string { return strings.Repeat(c, 32) }
			graph := []link{{id("a"), id("b"), false}}
			switch kind {
			case "divergent-successor":
				graph = []link{{id("a"), "", true}, {id("b"), id("a"), false}, {id("c"), id("a"), false}}
			case "multiple-pending":
				graph = []link{{id("a"), "", false}, {id("b"), "", false}}
			case "cycle":
				graph = []link{{id("a"), id("b"), true}, {id("b"), id("a"), true}}
			}
			for index, node := range graph {
				c := original.Captured
				c.Ordinal = uint64(index + 1)
				c.ID = node.id
				c.Previous = node.previous
				dir := filepath.Join(i.StateRoot, receiptJournal, c.ID)
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				write := func(name string, value any) {
					t.Helper()
					data, err := json.Marshal(value)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0600); err != nil {
						t.Fatal(err)
					}
				}
				write("authority-"+receiptDigest(c), c)
				write("captured", c)
				if node.complete {
					ready := *original.Ready
					ready.ID = c.ID
					ready.Captured = receiptDigest(c)
					ready.Parents = map[string]PublicationEvidence{}
					for path, evidence := range original.Ready.Parents {
						ready.Parents[strings.ReplaceAll(path, original.Captured.ID, c.ID)] = evidence
					}
					ready.Units = append([]publicationUnit(nil), original.Ready.Units...)
					for n, unit := range unitPaths(c) {
						ready.Units[n].Source = unit.Source
					}
					write("ready", ready)
					for n, unit := range ready.Units {
						write(fmt.Sprintf("unit-%d", n), receiptMarker{ID: c.ID, Digest: receiptDigest(unit)})
					}
					final := finalReceipt{ID: c.ID, Ready: receiptDigest(ready), Record: recordForOperation(c, ready, original.Final.Record.InstalledAt)}
					write("final", final)
					write("complete", receiptMarker{ID: c.ID, Digest: receiptDigest(final)})
				}
			}
			if _, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot)); err == nil {
				t.Fatal("read accepted conflicting on-disk graph")
			}
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); err == nil {
				t.Fatal("recovery accepted conflicting on-disk graph")
			}
			after, err := os.ReadFile(a.UserLocal.InstalledPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("public bytes changed")
			}
			after, err = os.ReadFile(DependencyMetadataPath(i.StateRoot))
			if err != nil || !bytes.Equal(legacy, after) {
				t.Fatal("legacy bytes changed")
			}
		})
	}
}

func TestUserLocalDirectHerdrWrapperRecoversPendingReceipt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", filepath.Join(home, ".local/bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	data, a := customFixture(t, false, false, false)
	fakeCustomDownload(t, data)
	a.Destination, a.InstalledPath = userLocalPaths(home, a.Recipe, a.Version, a.Layout, a.Command)
	action := InstallAction{Dependency: "herdr-custom", Requirement: manifest.DependencyRequirementRequired, Status: InstallActionStatusInstallable, Provider: TierUserLocal, Probe: "herdr-custom", Probes: []string{"herdr-custom"}, UserLocal: &a}
	installer := UserLocalInstaller{Home: home, StateRoot: filepath.Join(home, ".local/state/dots"), Download: func(string, string) ([]byte, error) { return data, nil }, Fault: func(point string) error {
		if point == "final:before" {
			return errors.New("receipt unavailable")
		}
		return nil
	}}
	if err := installer.RunUserLocal(action, nil); err == nil {
		t.Fatal("expected pending receipt")
	}
	downloadUserLocalArtifact = func(string, string) ([]byte, error) { return nil, errors.New("direct retry redownloaded") }
	if err := InstallUserLocal(home, action); err != nil {
		t.Fatal(err)
	}
	meta, err := LoadDependencyMetadata(DependencyMetadataPath(installer.StateRoot))
	if err != nil || len(meta.Dependencies) != 1 {
		t.Fatalf("direct retry skipped receipt recovery %+v %v", meta, err)
	}
	if err := InstallUserLocal(home, action); err != nil {
		t.Fatal(err)
	}
}

func TestUserLocalTransactionPostPublicationParentChange(t *testing.T) {
	for _, point := range []string{"destination:after-publish", "after-unit-0"} {
		t.Run(point, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "starship")
			bin := filepath.Join(i.Home, ".local/bin")
			i.Fault = func(actual string) error {
				if actual != point {
					return nil
				}
				if err := os.Rename(bin, bin+"-moved"); err != nil {
					return err
				}
				if err := os.Mkdir(bin, 0755); err != nil {
					return err
				}
				return os.Link(filepath.Join(bin+"-moved", "starship"), filepath.Join(bin, "starship"))
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("post-publication parent change accepted")
			}
			ops := loadTransactionOps(t, i.StateRoot)
			if ops[0].Progress[0] || ops[0].Complete {
				t.Fatal("progress committed under changed parent authority")
			}
			if _, err := os.Stat(filepath.Join(bin, "starship")); err != nil {
				t.Fatal("external replacement removed")
			}
		})
	}
}

func TestUserLocalSemanticAuthorityCorruptionFailsBeforeDownload(t *testing.T) {
	for _, which := range []string{"projection", "canonical", "missing", "multiple"} {
		t.Run(which, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == "after-captured" {
					return errors.New("interrupted")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("expected captured interruption")
			}
			op := loadTransactionOps(t, i.StateRoot)[0]
			canonical := filepath.Join(i.StateRoot, operationPath(op.Captured.ID, "authority-"+receiptDigest(op.Captured)))
			path := filepath.Join(i.StateRoot, operationPath(op.Captured.ID, "captured"))
			corrupted := op.Captured
			artifact := *corrupted.Action.UserLocal
			corrupted.Action.UserLocal = &artifact
			artifact.URL = "https://example.invalid/substituted"
			artifact.Checksum = strings.Repeat("a", 64)
			data, err := json.Marshal(corrupted)
			if err != nil {
				t.Fatal(err)
			}
			switch which {
			case "canonical":
				path = canonical
			case "missing":
				if err := os.Remove(canonical); err != nil {
					t.Fatal(err)
				}
			case "multiple":
				path = filepath.Join(i.StateRoot, operationPath(op.Captured.ID, "authority-"+receiptDigest(corrupted)))
			}
			if which != "missing" {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			i.Fault = nil
			i.Download = func(string, string) ([]byte, error) {
				t.Error("downloaded corrupted authority")
				return nil, errors.New("unexpected download")
			}
			if _, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot)); err == nil {
				t.Fatal("read accepted semantic authority corruption")
			}
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); err == nil {
				t.Fatal("recovery accepted semantic authority corruption")
			}
			if _, err := os.Stat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
				t.Fatal("published substituted artifact")
			}
		})
	}
}

func TestUserLocalAuthorityAndProjectionPersistenceRecovery(t *testing.T) {
	for _, point := range []string{"authority:write", "authority:sync", "authority:close", "authority:link", "authority:dir-sync", "captured:write", "captured:sync", "captured:close", "captured:link", "captured:dir-sync"} {
		t.Run(point, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "starship")
			i.Fault = func(actual string) error {
				if actual == point {
					return errors.New("persist failure")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("ignored authority persistence failure")
			}
			if _, err := os.Stat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
				t.Fatal("published before complete authority")
			}
			i.Fault = nil
			if err := i.RunUserLocal(a, nil); err != nil {
				t.Fatalf("exact captured retry failed: %v", err)
			}
			if !loadTransactionOps(t, i.StateRoot)[0].Complete {
				t.Fatal("retry did not finalize")
			}
		})
	}
}

func TestUserLocalPendingRecoveryUsesPhysicalHomeAndStateIdentity(t *testing.T) {
	i, a, _ := transactionFixture(t, "uv")
	actualHome, actualState := i.Home, i.StateRoot
	aliases := t.TempDir()
	i.Home = filepath.Join(aliases, "home-alias")
	i.StateRoot = filepath.Join(aliases, "state-alias")
	if err := os.Symlink(actualHome, i.Home); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actualState, i.StateRoot); err != nil {
		t.Fatal(err)
	}
	a.UserLocal.Destination, a.UserLocal.InstalledPath = userLocalPaths(i.Home, a.UserLocal.Recipe, a.UserLocal.Version, a.UserLocal.Layout, a.UserLocal.Command)
	i.Fault = func(point string) error {
		if point == "final:before" {
			return errors.New("receipt failed")
		}
		return nil
	}
	if err := i.RunUserLocal(a, nil); err == nil {
		t.Fatal("expected pending")
	}
	i.Home, i.StateRoot = actualHome, actualState
	i.Fault = nil
	i.Download = func(string, string) ([]byte, error) { return nil, errors.New("alias retry downloaded") }
	opts := Options{Profile: "default", OS: "linux", Arch: "amd64", Home: actualHome, StateRoot: actualState}
	prepared, err := PrepareInstall(transactionManifest(a), opts, func(string) bool { return true }, func(string) bool { return false }, TierGeneric)
	if err != nil || len(prepared.Plan.Actions) != 1 {
		t.Fatalf("alias plan skipped pending %+v %v", prepared, err)
	}
	report, err := InstallPrepared(prepared, opts, func(string) bool { return true }, func(string) bool { return false }, realTransactionRunner{&i})
	if err != nil || len(report.Items) != 1 {
		t.Fatalf("alias execution skipped pending %+v %v", report, err)
	}
	if !loadTransactionOps(t, actualState)[0].Complete {
		t.Fatal("alias recovery did not finalize")
	}
}

func TestUserLocalDirectPresenceRequiresExecutableFile(t *testing.T) {
	for _, kind := range []string{"file", "directory", "executable"} {
		t.Run(kind, func(t *testing.T) {
			i, a, downloads := transactionFixture(t, "starship")
			oldDownload := downloadUserLocalArtifact
			downloadUserLocalArtifact = func(string, string) ([]byte, error) {
				*downloads++
				return nil, errors.New("unexpected direct download")
			}
			t.Cleanup(func() { downloadUserLocalArtifact = oldDownload })
			bin := filepath.Join(i.Home, ".local/bin")
			if err := os.MkdirAll(bin, 0755); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(bin, "starship")
			if kind == "directory" {
				if err := os.Mkdir(target, 0755); err != nil {
					t.Fatal(err)
				}
			} else {
				mode := os.FileMode(0644)
				if kind == "executable" {
					mode = 0755
				}
				if err := os.WriteFile(target, []byte("external"), mode); err != nil {
					t.Fatal(err)
				}
			}
			err := InstallUserLocal(i.Home, a)
			if kind == "executable" && err != nil {
				t.Fatal(err)
			}
			if kind != "executable" && err == nil {
				t.Fatal("non-command satisfied dependency")
			}
			if *downloads != 0 {
				t.Fatal("replaced external state")
			}
			meta, err := LoadDependencyMetadata(DependencyMetadataPath(filepath.Join(i.Home, ".local/state/dots")))
			if err != nil || len(meta.Dependencies) != 0 {
				t.Fatalf("invented receipt %+v %v", meta, err)
			}
		})
	}
}

func TestUserLocalCapturedAliasRetargetFailsPlanningAndExecution(t *testing.T) {
	for _, scope := range []string{"home", "state"} {
		t.Run(scope, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			actualHome, actualState := i.Home, i.StateRoot
			aliases := t.TempDir()
			homeAlias, stateAlias := filepath.Join(aliases, "home"), filepath.Join(aliases, "state")
			if err := os.Symlink(actualHome, homeAlias); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(actualState, stateAlias); err != nil {
				t.Fatal(err)
			}
			i.Home, i.StateRoot = homeAlias, stateAlias
			a.UserLocal.Destination, a.UserLocal.InstalledPath = userLocalPaths(i.Home, a.UserLocal.Recipe, a.UserLocal.Version, a.UserLocal.Layout, a.UserLocal.Command)
			i.Fault = func(point string) error {
				if point == "final:before" {
					return errors.New("receipt failure")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("expected pending")
			}
			i.Home, i.StateRoot = actualHome, actualState
			i.Fault = nil
			i.Download = func(string, string) ([]byte, error) {
				t.Error("download after alias retarget")
				return nil, errors.New("unexpected download")
			}
			selected := homeAlias
			if scope == "state" {
				selected = stateAlias
			}
			if err := os.Remove(selected); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(t.TempDir(), selected); err != nil {
				t.Fatal(err)
			}
			roots := []*receiptRoot{}
			for _, path := range []string{actualHome, actualState} {
				root, err := openReceiptRoot(path, false)
				if err != nil {
					t.Fatal(err)
				}
				roots = append(roots, root)
			}
			before := []PublicationEvidence{}
			for _, root := range roots {
				node, err := root.evidence(".", false)
				if err != nil {
					t.Fatal(err)
				}
				before = append(before, node)
			}
			opts := Options{Profile: "default", OS: "linux", Arch: "amd64", Home: actualHome, StateRoot: actualState}
			if _, err := PrepareInstall(transactionManifest(a), opts, func(string) bool { return true }, func(string) bool { return false }, TierGeneric); err == nil {
				t.Fatal("planning accepted captured alias retarget")
			}
			prepared := PreparedInstall{Plan: PlanReport{selected: []UserLocalSelection{artifactSelection(a)}}}
			if _, err := InstallPrepared(prepared, opts, func(string) bool { return true }, func(string) bool { return false }, realTransactionRunner{&i}); err == nil {
				t.Fatal("execution accepted captured alias retarget")
			}
			for n, root := range roots {
				after, err := root.evidence(".", false)
				closeErr := root.file.Close()
				if err := errors.Join(err, closeErr); err != nil {
					t.Fatal(err)
				}
				if !sameEvidence(before[n], after) {
					t.Fatal("alias rejection changed published/journal bytes")
				}
			}
		})
	}
}
