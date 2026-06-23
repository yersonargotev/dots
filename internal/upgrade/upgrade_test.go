package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type recordingRunner struct {
	brewInstalled bool
	calls         []string
	failAt        string
	outputs       map[string]string
}

func (r *recordingRunner) Run(_ context.Context, executable string, args ...string) (string, error) {
	call := executable + " " + strings.Join(args, " ")
	r.calls = append(r.calls, call)
	if call == r.failAt {
		return "", fmt.Errorf("boom")
	}
	if executable == "brew" && strings.Join(args, " ") == "list --formula yersonargotev/tap/dots" {
		if r.brewInstalled {
			return "", nil
		}
		return "", fmt.Errorf("not installed")
	}
	if r.outputs != nil {
		return r.outputs[call], nil
	}
	return "", nil
}

func TestPreviewRejectsDevelopmentBuilds(t *testing.T) {
	plan, err := Preview(context.Background(), Options{CurrentVersion: "dev", Runner: &recordingRunner{}})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if plan.Channel != ChannelDevelopment || plan.Action != ActionManualRebuild {
		t.Fatalf("plan = %+v, want development manual rebuild", plan)
	}
}

func TestPreviewHomebrewReportsStableLatestVersion(t *testing.T) {
	runner := &recordingRunner{brewInstalled: true, outputs: map[string]string{"brew info --json=v2 yersonargotev/tap/dots": `{"formulae":[{"versions":{"stable":"0.20.0"}}]}`}}
	plan, err := Preview(context.Background(), Options{CurrentVersion: "v0.19.0", Runner: runner})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if plan.Channel != ChannelHomebrew || plan.LatestVersion != "v0.20.0" || plan.Action != ActionHomebrewUpgrade {
		t.Fatalf("plan = %+v, want Homebrew dry-run preview with real latest version", plan)
	}
}

func TestExecuteRunsHomebrewUpdateThenFormulaUpgrade(t *testing.T) {
	runner := &recordingRunner{brewInstalled: true, outputs: map[string]string{"brew info --json=v2 yersonargotev/tap/dots": `{"formulae":[{"versions":{"stable":"0.19.0"}}]}`}}
	plan, err := Execute(context.Background(), Options{CurrentVersion: "v0.18.0", Runner: runner})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if plan.Channel != ChannelHomebrew || plan.Action != ActionHomebrewUpgrade || plan.LatestVersion != "v0.19.0" {
		t.Fatalf("plan = %+v, want homebrew upgrade", plan)
	}
	want := []string{
		"brew list --formula yersonargotev/tap/dots",
		"brew info --json=v2 yersonargotev/tap/dots",
		"brew update",
		"brew upgrade yersonargotev/tap/dots",
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestReleaseReplacementVerifiesChecksumAndPreservesOldBinary(t *testing.T) {
	body := []byte("new dots binary")
	sum := sha256.Sum256(body)
	checksum := hex.EncodeToString(sum[:])
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/checksums.txt":
			fmt.Fprintf(w, "%s  dots_v0.19.0_linux_amd64\n", checksum)
		case "/latest/dots_v0.19.0_linux_amd64":
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exe := filepath.Join(t.TempDir(), "dots")
	if err := os.WriteFile(exe, []byte("old dots binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	plan, err := Execute(context.Background(), Options{
		CurrentVersion: "v0.18.0",
		Executable:     exe,
		GOOS:           "linux",
		GOARCH:         "amd64",
		ReleaseBaseURL: server.URL,
		Runner:         &recordingRunner{},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if plan.Action != ActionReplaceBinary || plan.LatestVersion != "v0.19.0" {
		t.Fatalf("plan = %+v, want replacement to v0.19.0", plan)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("executable = %q, want new body", got)
	}
	old, err := os.ReadFile(exe + ".old")
	if err != nil {
		t.Fatalf("read old executable: %v", err)
	}
	if string(old) != "old dots binary" {
		t.Fatalf("old executable = %q, want preserved old body", old)
	}
	if _, err := os.Stat(exe + ".new"); !os.IsNotExist(err) {
		t.Fatalf("temporary .new file remains; stat err = %v", err)
	}
}

func TestReleaseReplacementRejectsChecksumMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/checksums.txt":
			fmt.Fprintln(w, "0000  dots_v0.19.0_linux_amd64")
		case "/latest/dots_v0.19.0_linux_amd64":
			_, _ = w.Write([]byte("new dots binary"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	exe := filepath.Join(t.TempDir(), "dots")
	if err := os.WriteFile(exe, []byte("old dots binary"), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}

	_, err := Execute(context.Background(), Options{
		CurrentVersion: "v0.18.0",
		Executable:     exe,
		GOOS:           "linux",
		GOARCH:         "amd64",
		ReleaseBaseURL: server.URL,
		Runner:         &recordingRunner{},
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Execute() error = %v, want checksum mismatch", err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(got) != "old dots binary" {
		t.Fatalf("executable mutated after checksum failure: %q", got)
	}
}

func TestDownloadToTempWritesReleaseArtifactToTemporaryFile(t *testing.T) {
	body := []byte("downloaded release artifact")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()

	path, err := downloadToTemp(context.Background(), server.Client(), server.URL+"/dots")
	if err != nil {
		t.Fatalf("downloadToTemp() error = %v", err)
	}
	defer os.Remove(path)
	if filepath.Base(path) == "dots.new" {
		t.Fatalf("download used final .new path instead of temporary artifact path: %s", path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read temporary artifact: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("temporary artifact = %q, want downloaded body", got)
	}
}
