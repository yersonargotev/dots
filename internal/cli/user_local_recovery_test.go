package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
	installstate "github.com/yersonargotev/dots/internal/state"
)

func TestUserLocalReceiptRecoveryAcrossCLICommands(t *testing.T) {
	for _, verb := range []string{"deps", "install", "update", "upgrade"} {
		t.Run(verb, func(t *testing.T) {
			home, source, state := t.TempDir(), t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			bin := filepath.Join(home, ".local/bin")
			if err := os.MkdirAll(bin, 0755); err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(source, "dots.yaml")
			body := `version: 1
profiles:
  default:
    tags: [agents]
entries:
  - source: fixture.txt
    target: ~/.config/receipt-fixture.txt
    strategy: copy
    tags: [agents]
    dependencies:
      - name: codex
        command: codex
        rolling_user_local:
          recipe: codex
`
			if err := os.WriteFile(manifestPath, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "fixture.txt"), []byte("sandbox configuration\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if verb == "update" || verb == "upgrade" {
				origin := t.TempDir()
				runGit(t, origin, "init", "--bare", "--initial-branch=main")
				runGit(t, source, "init", "--initial-branch=main")
				runGit(t, source, "add", "dots.yaml", "fixture.txt")
				runGit(t, source, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "fixture")
				runGit(t, source, "remote", "add", "origin", origin)
				runGit(t, source, "push", "--set-upstream", "origin", "main")
			}
			execute := func(args ...string) error {
				cmd := NewRootCommand()
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&out)
				cmd.SetArgs(args)
				err := cmd.Execute()
				if err != nil {
					t.Log(out.String())
				}
				return err
			}
			common := []string{"--yes", "--profile", "default", "--file", manifestPath, "--home", home, "--state-root", state}
			if verb == "update" || verb == "upgrade" {
				external := t.TempDir()
				if err := os.WriteFile(filepath.Join(external, "codex"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", external+string(os.PathListSeparator)+"/usr/bin:/bin")
				args := append([]string{"install"}, common...)
				args = append(args, "--source-root", source)
				if err := execute(args...); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
			asset, binary := codexFixtureNames(t, runtime.GOOS, runtime.GOARCH)
			data := codexFixtureArchive(t, binary)
			sum := sha256.Sum256(data)
			checksum := hex.EncodeToString(sum[:])
			artifact := deps.UserLocalArtifact{Recipe: "codex", Version: "rust-v8.7.6", Artifact: asset, URL: "https://example.invalid/" + asset, Digest: "sha256:" + checksum, Checksum: checksum, Platform: runtime.GOOS + "_" + runtime.GOARCH, Layout: "bundle", Command: "codex"}
			artifact.Destination = filepath.Join(home, ".local/opt/codex", artifact.Version)
			artifact.InstalledPath = filepath.Join(bin, "codex")
			action := deps.InstallAction{Dependency: "codex", Requirement: manifest.DependencyRequirementRequired, Status: deps.InstallActionStatusInstallable, Probe: "codex", Probes: []string{"codex"}, Provider: deps.TierUserLocal, UserLocal: &artifact}
			installer := deps.UserLocalInstaller{Context: context.Background(), Home: home, StateRoot: state, Download: func(string, string) ([]byte, error) { return data, nil }, Fault: func(point string) error {
				if point == "final:before" {
					return errors.New("receipt persistence failure")
				}
				return nil
			}}
			if err := installer.RunUserLocal(action, nil); err == nil {
				t.Fatal("expected receipt failure")
			}
			args := []string{verb}
			if verb == "deps" {
				args = append(args, "install")
			}
			args = append(args, common...)
			if verb != "deps" {
				args = append(args, "--source-root", source)
			}
			if verb == "upgrade" {
				installed, err := installstate.Load(installstate.Path(state))
				if err != nil {
					t.Fatal(err)
				}
				fingerprint, err := installedSelectionFingerprint(installed.InstalledSelection)
				if err != nil {
					t.Fatal(err)
				}
				args = append(args, "--continue", "--selection-source", "recorded", "--selection-profile", "default", "--selection-previous", fingerprint)
			}
			if err := execute(args...); err != nil {
				t.Fatalf("%s recovery: %v", verb, err)
			}
			meta, err := deps.LoadDependencyMetadata(deps.DependencyMetadataPath(state))
			if err != nil || len(meta.Dependencies) != 1 {
				t.Fatalf("receipt %+v %v", meta, err)
			}
			if got := meta.Dependencies[0]; got.Version != artifact.Version || got.Checksum != checksum || got.URL != artifact.URL || got.OperationID == "" {
				t.Fatalf("wrong captured receipt %+v", got)
			}
			if verb != "deps" {
				if content, err := os.ReadFile(filepath.Join(home, ".config/receipt-fixture.txt")); err != nil || !strings.Contains(string(content), "sandbox") {
					t.Fatalf("managed configuration %q %v", content, err)
				}
			}
		})
	}
}
