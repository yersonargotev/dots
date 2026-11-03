package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/selection"
	"github.com/yersonargotev/dots/internal/state"
)

// This is the argument shape emitted by v0.96.1's upgradeContinuationArgs,
// independently of the current caller. Binary replacement and brew have
// completed at this boundary; neither is invoked by the candidate.
func legacyUpgradeArgs(home, source, stateRoot, channel, intent string) []string {
	action, artifact, checksum := "homebrew-upgrade", "", ""
	if channel == "release-artifact" {
		action, artifact, checksum = "replace-binary", "dots_v0.97.0_test", "abc123"
	}
	return []string{"upgrade", "--continue", "--file", filepath.Join(source, "dots.yaml"),
		"--selection-source", intent, "--selection-profile", "core", "--selection-tag", "extra",
		"--source-root", source, "--home", home, "--state-root", stateRoot,
		"--yes", "--no-tui", "--output", "json",
		"--binary-channel", channel, "--binary-current-version", "v0.96.1",
		"--binary-latest-version", "v0.97.0", "--binary-action", action,
		"--binary-executable", "/tmp/sandbox-dots", "--binary-artifact", artifact, "--binary-checksum", checksum}
}

func withoutLegacyFlag(args []string, name string, value bool) []string {
	var result []string
	for i := 0; i < len(args); i++ {
		if args[i] == name {
			if value {
				i++
			}
			continue
		}
		result = append(result, args[i])
	}
	return result
}

func TestUpgradeV0961ExplicitContinuationPreservesCompleteRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home, stateRoot := filepath.Join(t.TempDir(), "home with spaces"), t.TempDir()
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	origin, source := newContinuationRepo(t)
	const customFile = "custom selection.yaml"
	writeCustom := func(root string) {
		data, err := os.ReadFile(filepath.Join(root, "dots.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), "profiles:\n", "profiles:\n  work:\n    tags: [extra]\n", 1))
		if err := os.WriteFile(filepath.Join(root, customFile), data, 0600); err != nil {
			t.Fatal(err)
		}
		runContinuationGit(t, root, "add", "-A")
		runContinuationGit(t, root, "commit", "-m", "custom selection manifest")
	}
	writeCustom(origin)
	runContinuationGit(t, source, "pull", "--ff-only")
	previous := state.InstalledSelection{Profiles: []string{"core"}, ResolvedTags: []string{"core"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
		t.Fatal(err)
	}
	advanceContinuationRepo(t, origin)
	writeCustom(origin)
	args := legacyUpgradeArgs(home, source, stateRoot, "homebrew", "explicit")
	for _, name := range []string{"--selection-profile", "--selection-tag", "--file"} {
		args = withoutLegacyFlag(args, name, true)
	}
	args = append(args, "--selection-profile", "work", "--selection-profile", "core", "--selection-tag", "core", "--selection-tag", "extra", "--file", filepath.Join(source, customFile))
	var out, errOut bytes.Buffer
	if code := Run(args, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d\n%s\n%s", code, &out, &errOut)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"work", "core"}) ||
		!reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{"core", "extra"}) ||
		!reflect.DeepEqual(meta.InstalledSelection.ResolvedTags, []string{"extra", "core", "new"}) {
		t.Fatalf("complete explicit intent lost: %#v", meta.InstalledSelection)
	}
	for _, name := range []string{".core", ".extra", ".new"} {
		if _, err := os.Readlink(filepath.Join(home, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	if !strings.Contains(out.String(), `"source": "explicit"`) {
		t.Fatalf("explicit provenance lost: %s", &out)
	}
	// Malformed legacy requests still carry a shell-safe, complete recovery.
	args = append(args, "--selection-previous", "")
	out.Reset()
	errOut.Reset()
	if code := Run(args, &out, &errOut); code != 1 {
		t.Fatalf("empty identity exit=%d", code)
	}
	if !strings.Contains(out.String(), "--profile work --profile core --tag core --tag extra") ||
		!strings.Contains(out.String(), "--file '"+filepath.Join(source, customFile)+"'") ||
		!strings.Contains(out.String(), "--home '"+home+"'") {
		t.Fatalf("incomplete recovery: %s", &out)
	}
}

func TestUpgradeV0961InteractiveReductionRequiresNewConfirmation(t *testing.T) {
	for _, response := range []string{"no\n", "", "yes\n"} {
		t.Run(response, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			home, stateRoot := t.TempDir(), t.TempDir()
			origin, source := newContinuationRepo(t)
			previous := state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"extra", "core"}, ResolvedTags: []string{"core", "extra"}}
			if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
				t.Fatal(err)
			}
			advanceContinuationRepo(t, origin)
			args := legacyUpgradeArgs(home, source, stateRoot, "release-artifact", "explicit")
			args = withoutLegacyFlag(withoutLegacyFlag(args, "--yes", false), "--output", true)
			args = append(args, "--acknowledge-selection-change")
			cmd := NewRootCommand()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetIn(strings.NewReader(response))
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("interactive error=%v\n%s", err, &out)
			}
			if !strings.Contains(out.String(), "Apply this Installed Selection reduction?") {
				t.Fatalf("fresh prompt absent: %s", &out)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if response == "yes\n" {
				if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{"extra"}) {
					t.Fatalf("confirmed selection=%#v", meta.InstalledSelection)
				}
				if _, err := os.Readlink(filepath.Join(home, ".new")); err != nil {
					t.Fatal(err)
				}
			} else {
				if !reflect.DeepEqual(meta.InstalledSelection, &previous) {
					t.Fatal("denial/cancellation changed selection")
				}
				if _, err := os.Lstat(filepath.Join(home, ".core")); !os.IsNotExist(err) {
					t.Fatal("denial/cancellation mutated home")
				}
				data, err := os.ReadFile(filepath.Join(source, "dots.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(data), "configs/new") {
					t.Fatal("denial/cancellation refreshed repository")
				}
			}
		})
	}
}

func TestUpgradeV0961RejectsUnrecognizedContinuationBeforeEffects(t *testing.T) {
	tests := []struct {
		name   string
		remove string
		append []string
	}{
		{"missing binary outcome", "--binary-action", nil},
		{"different old version", "--binary-current-version", []string{"--binary-current-version", "v0.95.0"}},
		{"unknown channel", "--binary-channel", []string{"--binary-channel", "other"}},
		{"wrong action", "--binary-action", []string{"--binary-action", "already-current"}},
		{"empty executable", "--binary-executable", []string{"--binary-executable", ""}},
		{"JSON without confirmation", "", []string{"--yes=false"}},
		{"empty release digest", "--binary-checksum", []string{"--binary-checksum", ""}},
		{"missing release digest", "--binary-checksum", nil},
		{"empty identity", "", []string{"--selection-previous", ""}},
		{"unknown source", "--selection-source", []string{"--selection-source", "other"}},
		{"inherited migration", "--selection-source", []string{"--selection-source", "migration"}},
		{"legacy exclusion", "", []string{"--selection-exclude-tag", "core"}},
		{"mixed public selection", "", []string{"--profile", "core"}},
		{"dry run", "", []string{"--dry-run"}},
		{"positional argument", "", []string{"unexpected"}},
		{"inconsistent recorded intent", "--selection-tag", []string{"--selection-tag", "core"}},
		{"current invalid identity", "", []string{"--selection-previous", "invalid"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			home, stateRoot := t.TempDir(), t.TempDir()
			origin, source := newContinuationRepo(t)
			previous := state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"extra"}, ResolvedTags: []string{"core", "extra"}}
			if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
				t.Fatal(err)
			}
			advanceContinuationRepo(t, origin)
			args := legacyUpgradeArgs(home, source, stateRoot, "release-artifact", "recorded")
			if tt.remove != "" {
				args = withoutLegacyFlag(args, tt.remove, true)
			}
			args = append(args, tt.append...)
			var out, errOut bytes.Buffer
			if code := Run(args, &out, &errOut); code != 1 {
				t.Fatalf("exit=%d want rejection\n%s\n%s", code, &out, &errOut)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(meta.InstalledSelection, &previous) {
				t.Fatal("rejected continuation changed selection")
			}
			files, err := os.ReadDir(home)
			if err != nil || len(files) != 0 {
				t.Fatalf("home changed: %v %v", files, err)
			}
			cmd := exec.Command("git", "diff", "--exit-code")
			cmd.Dir = source
			if err := cmd.Run(); err != nil {
				t.Fatal("rejected continuation dirtied source")
			}
			data, err := os.ReadFile(filepath.Join(source, "dots.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "configs/new") {
				t.Fatal("rejected continuation refreshed repository")
			}
		})
	}
}

func TestUpgradeV0961RequiresFreshReductionAcknowledgement(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home, stateRoot := t.TempDir(), t.TempDir()
	origin, source := newContinuationRepo(t)
	previous := state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"extra"}, ResolvedTags: []string{"core", "extra"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
		t.Fatal(err)
	}
	// Authority advances after the older caller's acknowledgement. The carried
	// request now removes an extra Tag, even though its effective surface is equal.
	previous.ExtraTags = []string{"extra", "core"}
	previous.Provenance.RecordedAt = "new authority before continuation"
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
		t.Fatal(err)
	}
	advanceContinuationRepo(t, origin)
	args := append(legacyUpgradeArgs(home, source, stateRoot, "release-artifact", "explicit"), "--acknowledge-selection-change")
	var out, errOut bytes.Buffer
	if code := Run(args, &out, &errOut); code != 1 {
		t.Fatalf("exit=%d, want 1\n%s\n%s", code, &out, &errOut)
	}
	var result struct {
		Status string `json:"status"`
		Data   struct {
			Code        string           `json:"code"`
			Change      selection.Change `json:"selection_change"`
			Remediation struct {
				Command string `json:"recommended_command"`
			} `json:"remediation"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "error" || result.Data.Code != "selection-change-acknowledgement-required" || result.Data.Change.AcknowledgementAccepted {
		t.Fatalf("unexpected acknowledgement result: %s", &out)
	}
	for _, want := range []string{"dots upgrade", "--profile core", "--tag extra", "--file " + filepath.Join(source, "dots.yaml"), "--home " + home, "--source-root " + source, "--state-root " + stateRoot, "--yes", "--acknowledge-selection-change", "--no-tui", "--output json"} {
		if !strings.Contains(result.Data.Remediation.Command, want) {
			t.Errorf("recovery %q missing %q", result.Data.Remediation.Command, want)
		}
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta.InstalledSelection, &previous) {
		t.Fatal("denial changed fresh authority")
	}
	if _, err := os.Lstat(filepath.Join(home, ".core")); !os.IsNotExist(err) {
		t.Fatalf("denial mutated home: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(source, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "configs/new") {
		t.Fatal("denial refreshed repository")
	}
	// A new public invocation supplies acknowledgement against the current
	// snapshot; it must not repeat the legacy internal protocol.
	retry := []string{"update", "--profile", "core", "--tag", "extra", "--yes", "--acknowledge-selection-change", "--output", "json",
		"--file", filepath.Join(source, "dots.yaml"), "--home", home, "--source-root", source, "--state-root", stateRoot}
	out.Reset()
	errOut.Reset()
	if code := Run(retry, &out, &errOut); code != 0 {
		t.Fatalf("retry exit=%d\n%s\n%s", code, &out, &errOut)
	}
	meta, err = state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{"extra"}) {
		t.Fatalf("retry selection=%#v", meta.InstalledSelection)
	}
}

func TestUpgradeV0961RecordedContinuationRefreshesConfiguration(t *testing.T) {
	for _, channel := range []string{"release-artifact", "homebrew"} {
		t.Run(channel, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			home, stateRoot := t.TempDir(), t.TempDir()
			origin, source := newContinuationRepo(t)
			previous := state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"extra"}, ResolvedTags: []string{"core", "extra"}}
			if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
				t.Fatal(err)
			}
			advanceContinuationRepo(t, origin)
			var out, errOut bytes.Buffer
			code := Run(legacyUpgradeArgs(home, source, stateRoot, channel, "recorded"), &out, &errOut)
			if code != 0 {
				t.Fatalf("exit=%d\n%s\n%s", code, &out, &errOut)
			}
			for _, name := range []string{".core", ".extra", ".new"} {
				if _, err := os.Readlink(filepath.Join(home, name)); err != nil {
					t.Fatalf("missing Managed Entry %s: %v", name, err)
				}
			}
			var result struct {
				Status string `json:"status"`
				Data   struct {
					Selection selection.Report `json:"selection"`
					Binary    struct {
						Channel string `json:"channel"`
					} `json:"binary"`
				} `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Status != "ok" || result.Data.Selection.Source != selection.SourceRecorded || result.Data.Binary.Channel != channel {
				t.Fatalf("unexpected result: %s", &out)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.Profiles, []string{"core"}) ||
				!reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{"extra"}) ||
				!reflect.DeepEqual(meta.InstalledSelection.ResolvedTags, []string{"core", "new", "extra"}) {
				t.Fatalf("terminal selection = %#v", meta.InstalledSelection)
			}
		})
	}
}

func TestUpgradeV0961FailurePreservesSelectionAndRetryConverges(t *testing.T) {
	for _, failure := range []string{"repository", "configuration", "provisioner", "cancellation", "terminal authority"} {
		t.Run(failure, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			home, stateRoot := t.TempDir(), t.TempDir()
			origin, source := newContinuationRepo(t)
			previous := state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"extra"}, ResolvedTags: []string{"core", "extra"}}
			if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
				t.Fatal(err)
			}
			advanceContinuationRepo(t, origin)
			stubs := t.TempDir()
			t.Setenv("PATH", stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
			for _, tool := range []string{"brew", "npx", "codex", "claude"} {
				writeSelectorExecutable(t, filepath.Join(stubs, tool), "#!/bin/sh\nexit 91\n")
			}
			expected := previous
			switch failure {
			case "repository":
				runContinuationGit(t, source, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing-origin"))
			case "configuration":
				if err := os.Remove(filepath.Join(origin, "configs/core")); err != nil {
					t.Fatal(err)
				}
				runContinuationGit(t, origin, "add", "-A")
				runContinuationGit(t, origin, "commit", "-m", "missing configuration source")
			case "provisioner", "cancellation", "terminal authority":
				data, err := os.ReadFile(filepath.Join(origin, "dots.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, []byte("provisioners:\n  - tool: claude\n    tags: [core]\n    spec:\n      marketplace: example/tools\n    dependencies:\n      - name: claude\n")...)
				if err := os.WriteFile(filepath.Join(origin, "dots.yaml"), data, 0600); err != nil {
					t.Fatal(err)
				}
				runContinuationGit(t, origin, "add", "-A")
				runContinuationGit(t, origin, "commit", "-m", "add stubbed provisioner")
				if failure == "terminal authority" {
					expected.Provenance.RecordedAt = "concurrent terminal authority"
					concurrentPath := filepath.Join(t.TempDir(), "concurrent.json")
					if err := state.Save(concurrentPath, state.Metadata{Version: state.CurrentVersion, InstalledSelection: &expected}); err != nil {
						t.Fatal(err)
					}
					writeSelectorExecutable(t, filepath.Join(stubs, "claude"), "#!/bin/sh\ncp "+shellQuoteIfNeeded(concurrentPath)+" "+shellQuoteIfNeeded(state.Path(stateRoot))+"\n")
				}
			}
			args := legacyUpgradeArgs(home, source, stateRoot, "release-artifact", "recorded")
			var out, errOut bytes.Buffer
			code := 1
			if failure == "cancellation" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				cmd := NewRootCommand()
				cmd.SetContext(ctx)
				cmd.SetOut(&out)
				cmd.SetErr(&errOut)
				cmd.SetArgs(args)
				if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "context canceled") {
					t.Fatalf("cancellation error=%v\n%s", err, &out)
				}
			} else {
				code = Run(args, &out, &errOut)
			}
			if code != 1 {
				t.Fatalf("failure exit=%d\n%s\n%s", code, &out, &errOut)
			}
			if failure == "terminal authority" && !strings.Contains(out.String(), "changed concurrently") {
				t.Fatalf("wrong concurrency failure: %s", &out)
			}
			if strings.Contains(out.String(), "--profile core") || strings.Contains(out.String(), "--tag extra") {
				t.Fatalf("recorded recovery converted intent to explicit: %s", &out)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(meta.InstalledSelection, &expected) {
				t.Fatalf("failure overwrote authority: %#v", meta.InstalledSelection)
			}
			if failure == "repository" || failure == "configuration" {
				if _, err := os.Lstat(filepath.Join(home, ".core")); !os.IsNotExist(err) {
					t.Fatalf("early failure mutated home: %v", err)
				}
			}
			switch failure {
			case "repository":
				runContinuationGit(t, source, "remote", "set-url", "origin", origin)
			case "configuration":
				if err := os.WriteFile(filepath.Join(origin, "configs/core"), []byte("core\n"), 0600); err != nil {
					t.Fatal(err)
				}
				runContinuationGit(t, origin, "add", "-A")
				runContinuationGit(t, origin, "commit", "-m", "restore configuration source")
			default:
				writeSelectorExecutable(t, filepath.Join(stubs, "claude"), "#!/bin/sh\nexit 0\n")
			}
			out.Reset()
			errOut.Reset()
			if code := Run(args, &out, &errOut); code != 0 {
				t.Fatalf("retry exit=%d\n%s\n%s", code, &out, &errOut)
			}
			meta, err = state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.ResolvedTags, []string{"core", "new", "extra"}) {
				t.Fatalf("retry selection=%#v", meta.InstalledSelection)
			}
			for _, target := range []string{".core", ".extra", ".new"} {
				if _, err := os.Readlink(filepath.Join(home, target)); err != nil {
					t.Fatalf("retry missing %s: %v", target, err)
				}
			}
		})
	}
}

type legacyConfirmationErrorReader struct{ err error }

func (r legacyConfirmationErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestUpgradeV0961ConfirmationReadFailurePreservesState(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home, stateRoot := t.TempDir(), t.TempDir()
	origin, source := newContinuationRepo(t)
	previous := state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"extra", "core"}, ResolvedTags: []string{"core", "extra"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
		t.Fatal(err)
	}
	advanceContinuationRepo(t, origin)
	args := legacyUpgradeArgs(home, source, stateRoot, "homebrew", "explicit")
	args = withoutLegacyFlag(withoutLegacyFlag(args, "--yes", false), "--output", true)
	readErr := errors.New("confirmation input unavailable")
	cmd := NewRootCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(legacyConfirmationErrorReader{readErr})
	cmd.SetArgs(args)
	if err := cmd.Execute(); !errors.Is(err, readErr) {
		t.Fatalf("confirmation error=%v", err)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta.InstalledSelection, &previous) {
		t.Fatal("confirmation failure changed authority")
	}
	files, err := os.ReadDir(home)
	if err != nil || len(files) != 0 {
		t.Fatalf("confirmation failure changed home: %v %v", files, err)
	}
	data, err := os.ReadFile(filepath.Join(source, "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "configs/new") {
		t.Fatal("confirmation failure refreshed repository")
	}
}

func TestUpgradeMalformedContinuationRecoveryPreservesPublicRequest(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(map[bool]string{false: "public only", true: "mixed intent"}[mixed], func(t *testing.T) {
			home, source, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
			args := []string{"upgrade", "--continue", "--profile", "workstation", "--profile", "mobile", "--tag", "extra", "--exclude-tag", "official",
				"--file", filepath.Join(source, "custom manifest.yaml"), "--home", home, "--source-root", source, "--state-root", stateRoot, "--yes", "--no-tui", "--output", "json"}
			if mixed {
				args = append(args, "--selection-source", "recorded", "--selection-profile", "other", "--selection-tag", "internal-extra")
			}
			var out, errOut bytes.Buffer
			if code := Run(args, &out, &errOut); code != 1 {
				t.Fatalf("exit=%d\n%s\n%s", code, &out, &errOut)
			}
			if mixed {
				if strings.Contains(out.String(), "restart with dots upgrade") {
					t.Fatalf("ambiguous executable recovery: %s", &out)
				}
				for _, value := range []string{"mixed public and internal selection", "workstation", "mobile", "extra", "official", "other", "internal-extra"} {
					if !strings.Contains(out.String(), value) {
						t.Errorf("ambiguous diagnostic lost %q: %s", value, &out)
					}
				}
			} else {
				for _, value := range []string{"--profile workstation --profile mobile --tag extra --exclude-tag official", "--file '" + filepath.Join(source, "custom manifest.yaml") + "'", "--home " + home, "--source-root " + source, "--state-root " + stateRoot, "--yes", "--no-tui", "--output json"} {
					if !strings.Contains(out.String(), value) {
						t.Errorf("public recovery lost %q: %s", value, &out)
					}
				}
			}
			for _, root := range []string{home, source, stateRoot} {
				files, err := os.ReadDir(root)
				if err != nil || len(files) != 0 {
					t.Fatalf("malformed request mutated %s: %v %v", root, files, err)
				}
			}
		})
	}
}

func TestUpgradeV0961FreshSnapshotIsCheckedAtManagedEntryMutation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	home, stateRoot := t.TempDir(), t.TempDir()
	origin, source := newContinuationRepo(t)
	previous := state.InstalledSelection{Profiles: []string{"core"}, ExtraTags: []string{"extra"}, ResolvedTags: []string{"core", "extra"}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, InstalledSelection: &previous}); err != nil {
		t.Fatal(err)
	}
	advanceContinuationRepo(t, origin)
	concurrent := previous
	concurrent.Provenance.RecordedAt = "authority changed during repository fetch"
	concurrentPath := filepath.Join(t.TempDir(), "concurrent.json")
	if err := state.Save(concurrentPath, state.Metadata{Version: state.CurrentVersion, InstalledSelection: &concurrent}); err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	stubs := t.TempDir()
	writeSelectorExecutable(t, filepath.Join(stubs, "git"), "#!/bin/sh\nif [ \"$1\" = fetch ]; then cp "+shellQuoteIfNeeded(concurrentPath)+" "+shellQuoteIfNeeded(state.Path(stateRoot))+"; fi\nexec "+shellQuoteIfNeeded(realGit)+" \"$@\"\n")
	t.Setenv("PATH", stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out, errOut bytes.Buffer
	if code := Run(legacyUpgradeArgs(home, source, stateRoot, "homebrew", "recorded"), &out, &errOut); code != 1 || !strings.Contains(out.String(), "changed concurrently") {
		t.Fatalf("exit=%d\n%s\n%s", code, &out, &errOut)
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(meta.InstalledSelection, &concurrent) {
		t.Fatal("mutation overwrote newer authority")
	}
	files, err := os.ReadDir(home)
	if err != nil || len(files) != 0 {
		t.Fatalf("mutation used stale authority: %v %v", files, err)
	}
}

func TestUpgradeV0961NeverInfersAuthorityFromHistoricalInventory(t *testing.T) {
	for _, sourceKind := range []string{"recorded", "explicit"} {
		t.Run(sourceKind, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			home, stateRoot := t.TempDir(), t.TempDir()
			origin, source := newContinuationRepo(t)
			previous := state.Metadata{Version: 2, Entries: []state.Record{{Target: filepath.Join(home, ".historical"), Source: "configs/core", Strategy: "symlink", Profiles: []string{"core"}, Tags: []string{"core"}}}}
			if err := state.Save(state.Path(stateRoot), previous); err != nil {
				t.Fatal(err)
			}
			advanceContinuationRepo(t, origin)
			var out, errOut bytes.Buffer
			code := Run(legacyUpgradeArgs(home, source, stateRoot, "release-artifact", sourceKind), &out, &errOut)
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if sourceKind == "recorded" {
				if code != 1 || !strings.Contains(out.String(), "has no recorded selection") {
					t.Fatalf("exit=%d\n%s\n%s", code, &out, &errOut)
				}
				if !reflect.DeepEqual(meta, previous) {
					t.Fatalf("historical inventory promoted: %#v", meta)
				}
				files, err := os.ReadDir(home)
				if err != nil || len(files) != 0 {
					t.Fatalf("missing authority changed home: %v %v", files, err)
				}
			} else if code != 0 || meta.InstalledSelection == nil || !reflect.DeepEqual(meta.InstalledSelection.ExtraTags, []string{"extra"}) {
				t.Fatalf("explicit intent failed without previous authority: exit=%d\n%s\n%s", code, &out, &errOut)
			}
		})
	}
}
