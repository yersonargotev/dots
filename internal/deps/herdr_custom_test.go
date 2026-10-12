package deps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

func customDependency() manifest.Dependency {
	return manifest.Dependency{Name: "herdr-custom", Command: "herdr-custom", UserLocal: &manifest.UserLocalProvider{Recipe: "herdr-custom", Version: "v0.9.3-custom.3", Checksums: map[string]string{"darwin_arm64": "0a5125f2e0402dba3ebf509434836cc8e8f4b0902f6bbd2bf1259bdfa3f09389", "linux_amd64": "5bd815660f60ae38c9e2ce812107abd31478d8bba45ce06b3e7dda0f9acce60c"}}}
}

func TestHerdrCustomArtifactPlatformsAndProvider(t *testing.T) {
	for _, platform := range []struct{ os, arch, target string }{{"darwin", "arm64", "macos-aarch64"}, {"linux", "amd64", "linux-x86_64"}} {
		opts := Options{OS: platform.os, Arch: platform.arch, Home: t.TempDir()}
		a, ok, err := userLocalArtifact(customDependency(), opts)
		if err != nil || !ok {
			t.Fatalf("resolve: %v", err)
		}
		want := "https://github.com/yersonargotev/herdr-custom/releases/download/v0.9.3-custom.3/herdr-custom-0.9.3-custom.3-" + platform.target + ".tar.gz"
		if a.URL != want || a.Command != "herdr-custom" || a.Layout != userLocalLayoutBundle {
			t.Fatalf("artifact: %+v", a)
		}
		candidates, err := providerCandidates(customDependency(), opts, TierHomebrew, func(string) bool { return false })
		if err != nil || len(candidates) != 1 || candidates[0].UserLocal == nil {
			t.Fatalf("provider: %+v %v", candidates, err)
		}
	}
	for _, platform := range []struct{ os, arch string }{{"darwin", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}} {
		if _, _, err := userLocalArtifact(customDependency(), Options{OS: platform.os, Arch: platform.arch}); err == nil {
			t.Fatalf("accepted %+v", platform)
		}
	}
}

func customFixture(t *testing.T, missing, unsafe, badMode bool) ([]byte, UserLocalArtifact) {
	return customFixtureVersion(t, "v0.9.3-custom.3", missing, unsafe, badMode)
}

func customFixtureVersion(t *testing.T, version string, missing, unsafe, badMode bool) ([]byte, UserLocalArtifact) {
	t.Helper()
	dep := customDependency()
	dep.UserLocal.Version = version
	a, _, err := herdrCustomArtifact(dep, Options{OS: "linux", Arch: "amd64", Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	runtime := "#!/bin/sh\nprintf '%s\\n' \"$HERDR_CONFIG_PATH\" \"$@\"\n"
	sum := sha256.Sum256([]byte(runtime))
	root := strings.TrimSuffix(a.Artifact, ".tar.gz")
	files := map[string]string{root + "/bin/herdr-custom": "#!/bin/sh\nexec \"$(dirname \"$0\")/../libexec/herdr\" \"$@\"\n", root + "/libexec/herdr": runtime, root + "/provenance.json": fmt.Sprintf(`{"binary_sha256":%q}`, hex.EncodeToString(sum[:]))}
	if missing {
		delete(files, root+"/libexec/herdr")
	}
	if unsafe {
		files["../../escape"] = "bad"
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		mode := int64(0755)
		if badMode {
			mode = 0644
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), a
}

func fakeCustomDownload(t *testing.T, data []byte) {
	t.Helper()
	old := downloadUserLocalArtifact
	downloadUserLocalArtifact = func(string, string) ([]byte, error) { return data, nil }
	t.Cleanup(func() { downloadUserLocalArtifact = old })
}

func TestHerdrCustomInstallPreservesOfficialAndUsesDotsConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home with spaces")
	bin := filepath.Join(home, ".local/bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	official := filepath.Join(bin, "herdr")
	if err := os.WriteFile(official, []byte("official sentinel"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	data, a := customFixture(t, false, false, false)
	fakeCustomDownload(t, data)
	if err := InstallUserLocal(home, InstallAction{UserLocal: &a}); err != nil {
		t.Fatal(err)
	}
	dep := customDependency()
	opts := Options{OS: "linux", Arch: "amd64", Home: home}
	if !DependencyPresent(dep, opts, func(string) bool { return true }, nil) {
		t.Fatal("integrated dependency not present")
	}
	for _, tc := range []struct{ extra, want string }{{"", filepath.Join(home, ".config/herdr/config.toml")}, {"XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"), filepath.Join(home, ".config/herdr/config.toml")}, {"HERDR_CONFIG_PATH=/explicit/custom.toml", "/explicit/custom.toml"}} {
		cmd := exec.Command(filepath.Join(bin, "herdr-custom"), "argument with spaces", "literal;$value")
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home}
		if tc.extra != "" {
			cmd.Env = append(cmd.Env, tc.extra)
		}
		out, err := cmd.CombinedOutput()
		want := tc.want + "\nargument with spaces\nliteral;$value\n"
		if err != nil || string(out) != want {
			t.Fatalf("launcher=%q %v want %q", out, err, want)
		}
	}
	if err := InstallUserLocal(home, InstallAction{UserLocal: &a}); err != nil {
		t.Fatalf("idempotence: %v", err)
	}
	if got, _ := os.ReadFile(official); string(got) != "official sentinel" {
		t.Fatal("official changed")
	}
	// A valid local installation must not hide a preceding external PATH command.
	shadow := t.TempDir()
	if err := os.WriteFile(filepath.Join(shadow, "herdr-custom"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shadow+string(os.PathListSeparator)+os.Getenv("PATH"))
	if DependencyPresent(dep, opts, func(string) bool { return true }, nil) {
		t.Fatal("PATH shadow counted as integrated")
	}
	if _, err := actionFor(dep, opts, TierHomebrew, func(string) bool { return true }); err == nil || !strings.Contains(err.Error(), "shadow") {
		t.Fatalf("shadow planning=%v", err)
	}
}

func TestHerdrCustomRejectsUnmanagedCommandBeforeDownload(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local/bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(bin, "herdr-custom")
	original := []byte("#!/bin/sh\n# manually installed release\n")
	if err := os.WriteFile(path, original, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, a := customFixture(t, false, false, false)
	old := downloadUserLocalArtifact
	downloadUserLocalArtifact = func(string, string) ([]byte, error) { t.Fatal("download before conflict validation"); return nil, nil }
	t.Cleanup(func() { downloadUserLocalArtifact = old })
	if err := InstallUserLocal(home, InstallAction{UserLocal: &a}); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("conflict=%v", err)
	}
	if DependencyPresent(customDependency(), Options{OS: "linux", Arch: "amd64", Home: home}, func(string) bool { return true }, nil) {
		t.Fatal("plain release mistaken for integrated installation")
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, original) {
		t.Fatal("unmanaged command changed")
	}
}

func TestHerdrCustomRejectsInvalidBundleBeforeCommit(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		missing, unsafe, badMode bool
	}{{"missing-runtime", true, false, false}, {"unsafe-archive", false, true, false}, {"not-executable", false, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			data, a := customFixture(t, tc.missing, tc.unsafe, tc.badMode)
			fakeCustomDownload(t, data)
			if err := InstallUserLocal(home, InstallAction{UserLocal: &a}); err == nil {
				t.Fatal("accepted invalid bundle")
			}
			if _, err := os.Lstat(filepath.Join(home, ".local/bin/herdr-custom")); !os.IsNotExist(err) {
				t.Fatalf("committed public link: %v", err)
			}
		})
	}
}

func TestHerdrCustomUpgradePreservesPriorUntilValidated(t *testing.T) {
	for _, failure := range []string{"", "checksum", "missing-runtime"} {
		t.Run("upgrade-"+failure, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".local/bin")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			oldData, old := customFixture(t, false, false, false)
			fakeCustomDownload(t, oldData)
			if err := InstallUserLocal(home, InstallAction{UserLocal: &old}); err != nil {
				t.Fatal(err)
			}
			public := filepath.Join(bin, "herdr-custom")
			oldTarget, err := os.Readlink(public)
			if err != nil {
				t.Fatal(err)
			}
			oldRuntime := filepath.Join(filepath.Dir(filepath.Dir(oldTarget)), "libexec/herdr")
			oldBytes, err := os.ReadFile(oldRuntime)
			if err != nil {
				t.Fatal(err)
			}
			data, next := customFixtureVersion(t, "v0.9.3-custom.4", failure == "missing-runtime", false, false)
			dep := customDependency()
			dep.UserLocal.Version = next.Version
			opts := Options{OS: "linux", Arch: "amd64", Home: home}
			if DependencyPresent(dep, opts, func(string) bool { return true }, nil) {
				t.Fatal("old version satisfies new pin")
			}
			action, err := actionFor(dep, opts, TierHomebrew, func(string) bool { return true })
			if err != nil || action.UserLocal == nil {
				t.Fatalf("upgrade plan %+v %v", action, err)
			}
			downloadUserLocalArtifact = func(string, string) ([]byte, error) {
				if failure == "checksum" {
					return nil, fmt.Errorf("verify user-local artifact: sha256 mismatch")
				}
				return data, nil
			}
			err = InstallUserLocal(home, action)
			gotTarget, linkErr := os.Readlink(public)
			if linkErr != nil {
				t.Fatal(linkErr)
			}
			if failure != "" {
				if err == nil || gotTarget != oldTarget {
					t.Fatalf("failed upgrade changed link: %s %v", gotTarget, err)
				}
			} else {
				if err != nil || gotTarget == oldTarget {
					t.Fatalf("upgrade failed: %s %v", gotTarget, err)
				}
				if !DependencyPresent(dep, opts, func(string) bool { return true }, nil) {
					t.Fatal("new version not detected")
				}
			}
			if got, err := os.ReadFile(oldRuntime); err != nil || !bytes.Equal(got, oldBytes) {
				t.Fatalf("prior runtime changed: %v", err)
			}
		})
	}
}

func TestHerdrCustomUpgradeRejectsUnconfinedOrUnverifiedPrior(t *testing.T) {
	for _, change := range []string{"outside-link", "marker-symlink", "empty-hash", "runtime-changed", "directory-symlink"} {
		t.Run(change, func(t *testing.T) {
			home := t.TempDir()
			bin := filepath.Join(home, ".local/bin")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			data, a := customFixture(t, false, false, false)
			fakeCustomDownload(t, data)
			if err := InstallUserLocal(home, InstallAction{UserLocal: &a}); err != nil {
				t.Fatal(err)
			}
			public := filepath.Join(bin, "herdr-custom")
			target, err := os.Readlink(public)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Dir(filepath.Dir(target))
			marker := filepath.Join(root, "dots/integration.json")
			switch change {
			case "outside-link":
				if err := os.Remove(public); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(t.TempDir(), "herdr-custom"), public); err != nil {
					t.Fatal(err)
				}
			case "marker-symlink":
				data, err := os.ReadFile(marker)
				if err != nil {
					t.Fatal(err)
				}
				external := filepath.Join(t.TempDir(), "marker")
				if err := os.WriteFile(external, data, 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(marker); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(external, marker); err != nil {
					t.Fatal(err)
				}
			case "empty-hash":
				if err := os.WriteFile(marker, []byte(`{"archive_sha256":"","original_launcher_sha256":"","runtime_sha256":""}`), 0644); err != nil {
					t.Fatal(err)
				}
			case "runtime-changed":
				if err := os.WriteFile(filepath.Join(root, "libexec/herdr"), []byte("changed"), 0755); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				moved := filepath.Join(t.TempDir(), "bundle")
				if err := os.Rename(root, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, root); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Readlink(public)
			if err != nil {
				t.Fatal(err)
			}
			_, next := customFixtureVersion(t, "v0.9.3-custom.4", false, false, false)
			if err := InstallUserLocal(home, InstallAction{UserLocal: &next}); err == nil {
				t.Fatal("accepted invalid prior integration")
			}
			after, err := os.Readlink(public)
			if err != nil || before != after {
				t.Fatal("conflict changed public link")
			}
		})
	}
}
