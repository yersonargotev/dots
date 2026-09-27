package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yersonargotev/dots/internal/batcache"
	"github.com/yersonargotev/dots/internal/install"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/provision"
	"github.com/yersonargotev/dots/internal/state"
)

func TestBatCacheReceiptSaveFailureRerunsConvergently(t *testing.T) {
	fixture := newCarbonfoxFixture(t, runtime.GOOS)
	config := []byte("--theme=\"Carbonfox\"\n")
	theme := []byte("<plist>Carbonfox</plist>\n")
	writeBatCacheLifecycleFile(t, filepath.Join(fixture.home, ".config", "bat", "config"), config)
	writeBatCacheLifecycleFile(t, filepath.Join(fixture.home, ".config", "bat", "themes", "Carbonfox.tmTheme"), theme)

	m := manifest.Manifest{Provisioners: []manifest.Provisioner{{
		Tool: "bat", Tags: []string{"bat", "zsh"}, RequiredTags: []string{carbonfoxTag},
		Spec: manifest.ProvisionerSpec{Cache: "build"},
	}}}
	selection := manifest.Selection{Tags: []string{"bat", carbonfoxTag}}
	opts := provision.Options{Selection: &selection, OS: runtime.GOOS, Arch: runtime.GOARCH}
	input := &batcache.Input{Config: config, Theme: theme}
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	badStateRoot := filepath.Join(t.TempDir(), "state-file")
	if err := os.WriteFile(badStateRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runProvisionersWithOptionsEnvironmentAndBatInput(cmd, m, opts, fixture.home, badStateRoot, fixture.repositoryRoot, os.Environ(), input)
	if err == nil {
		t.Fatal("runProvisioners() error = nil, want receipt-save failure")
	}
	for _, name := range []string{"metadata.yaml", "syntaxes.bin", "themes.bin"} {
		if data, readErr := os.ReadFile(filepath.Join(fixture.home, ".cache", "bat", name)); readErr != nil || len(data) == 0 {
			t.Fatalf("native cache %s after receipt failure = %q, %v", name, data, readErr)
		}
	}

	goodStateRoot := t.TempDir()
	if _, err := runProvisionersWithOptionsEnvironmentAndBatInput(cmd, m, opts, fixture.home, goodStateRoot, fixture.repositoryRoot, os.Environ(), input); err != nil {
		t.Fatalf("rerun provisioners: %v", err)
	}
	metadata, err := state.Load(state.Path(goodStateRoot))
	if err != nil {
		t.Fatal(err)
	}
	record, ok := metadata.FindProvisioner("", "bat", "bat", []string{"cache", "--build"})
	if !ok || record.Status != "provisioned" {
		t.Fatalf("bat provisioner receipt = %#v, found=%t", record, ok)
	}
}

func TestBatCacheTerminalSelectionFailurePreservesAuthorityAndReruns(t *testing.T) {
	fixture := newCarbonfoxFixture(t, "linux")
	fault := errors.New("injected terminal selection failure")
	originalCommit := commitInstallationMetadata
	commitInstallationMetadata = func(install.MetadataCommit, state.InstalledSelection) error { return fault }
	t.Cleanup(func() { commitInstallationMetadata = originalCommit })

	args := []string{
		"install", "--yes", "--skip-deps", "--output", "json",
		"--file", fixture.manifestPath, "--home", fixture.home,
		"--source-root", fixture.repositoryRoot, "--state-root", fixture.stateRoot,
		"--tag", "bat", "--tag", carbonfoxTag,
	}
	output := fixture.run(t, ExitError, args...)
	if !strings.Contains(output, fault.Error()) {
		t.Fatalf("terminal failure output = %s", output)
	}
	failed := fixture.metadata(t)
	if failed.InstalledSelection != nil {
		t.Fatalf("InstalledSelection after terminal failure = %#v", failed.InstalledSelection)
	}
	if receipt, ok := failed.FindProvisioner("", "bat", "bat", []string{"cache", "--build"}); !ok || receipt.Status != "provisioned" {
		t.Fatalf("bat receipt after terminal failure = %#v, found=%t", receipt, ok)
	}

	commitInstallationMetadata = originalCommit
	fixture.install(t, false, "bat", carbonfoxTag)
	converged := fixture.metadata(t)
	if converged.InstalledSelection == nil || !strings.Contains(strings.Join(converged.InstalledSelection.ResolvedTags, ","), carbonfoxTag) {
		t.Fatalf("InstalledSelection after rerun = %#v", converged.InstalledSelection)
	}
}

func writeBatCacheLifecycleFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
