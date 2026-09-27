package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/batcache"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/state"
)

type failingBatBuild struct{ err error }

func (f failingBatBuild) Run(ctx context.Context, executable string, args, env []string, stdin []byte, dir string) ([]byte, []byte, error) {
	if len(args) > 0 && args[0] == "cache" {
		return nil, nil, f.err
	}
	return (batcache.ExecRunner{}).Run(ctx, executable, args, env, stdin, dir)
}

func TestBatCacheOperationAndFailedReceiptSaveErrorsAreJoined(t *testing.T) {
	fixture := newCarbonfoxFixture(t, runtime.GOOS)
	config, theme := []byte("--theme=Carbonfox\n"), []byte("<plist>Carbonfox</plist>\n")
	writeBatCacheLifecycleFile(t, filepath.Join(fixture.home, ".config/bat/config"), config)
	writeBatCacheLifecycleFile(t, filepath.Join(fixture.home, ".config/bat/themes/Carbonfox.tmTheme"), theme)
	m := manifest.Manifest{Provisioners: []manifest.Provisioner{{Tool: "bat", Tags: []string{"bat", "zsh"}, RequiredTags: []string{carbonfoxTag}, Spec: manifest.ProvisionerSpec{Cache: "build"}}}}
	selected := manifest.Selection{Tags: []string{"bat", carbonfoxTag}}
	opts := provision.Options{Selection: &selected, OS: runtime.GOOS, Arch: runtime.GOARCH}
	fault := errors.New("injected native bat build failure")
	input := &batcache.Input{Config: config, Theme: theme, Runner: failingBatBuild{fault}}
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	badRoot := filepath.Join(t.TempDir(), "state-file")
	if err := os.WriteFile(badRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := runProvisionersWithOptionsEnvironmentAndBatInput(cmd, m, opts, fixture.home, badRoot, fixture.repositoryRoot, os.Environ(), input)
	if !errors.Is(err, fault) || !strings.Contains(err.Error(), badRoot) {
		t.Fatalf("joined operation/receipt error = %v", err)
	}
	if len(report.Items) != 1 || report.Items[0].Status != provision.RunStatusFailed {
		t.Fatalf("failed operation report = %#v", report)
	}
	if _, err := os.Stat(filepath.Join(fixture.home, ".cache/bat/themes.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed build published cache: %v", err)
	}
	goodRoot := t.TempDir()
	if _, err := runProvisionersWithOptionsEnvironmentAndBatInput(cmd, m, opts, fixture.home, goodRoot, fixture.repositoryRoot, os.Environ(), input); !errors.Is(err, fault) {
		t.Fatalf("persist failed receipt: %v", err)
	}
	metadata, err := state.Load(state.Path(goodRoot))
	if err != nil {
		t.Fatal(err)
	}
	if rec, ok := metadata.FindProvisioner("", "bat", "bat", []string{"cache", "--build"}); !ok || rec.Status != "failed" {
		t.Fatalf("failed receipt = %#v, found=%v", rec, ok)
	}
	if metadata.InstalledSelection != nil {
		t.Fatal("failed cache operation committed selection")
	}
	input.Runner = nil
	if _, err := runProvisionersWithOptionsEnvironmentAndBatInput(cmd, m, opts, fixture.home, goodRoot, fixture.repositoryRoot, os.Environ(), input); err != nil {
		t.Fatalf("retry: %v", err)
	}
	metadata, err = state.Load(state.Path(goodRoot))
	if err != nil {
		t.Fatal(err)
	}
	if rec, ok := metadata.FindProvisioner("", "bat", "bat", []string{"cache", "--build"}); !ok || rec.Status != "provisioned" {
		t.Fatalf("converged receipt = %#v, found=%v", rec, ok)
	}
}
