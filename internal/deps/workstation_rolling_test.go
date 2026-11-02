package deps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/selectedsurface"
)

func TestWorkstationRollingProvidersSelectStablePlatformArtifacts(t *testing.T) {
	for _, tt := range []struct {
		recipe, command, version string
		amd64, arm64             string
	}{
		{"uv", "uv", "0.13.6", "uv-x86_64-unknown-linux-gnu.tar.gz", "uv-aarch64-unknown-linux-gnu.tar.gz"},
		{"pnpm-standalone", "pnpm", "v12.16.1", "pnpm-linux-x64.tar.gz", "pnpm-linux-arm64.tar.gz"},
		{"bun", "bun", "bun-v1.4.4", "bun-linux-x64.zip", "bun-linux-aarch64.zip"},
		{"gh", "gh", "v2.103.3", "gh_2.103.3_linux_amd64.tar.gz", "gh_2.103.3_linux_arm64.tar.gz"},
	} {
		for arch, asset := range map[string]string{"amd64": tt.amd64, "arm64": tt.arm64} {
			t.Run(tt.recipe+"/"+arch, func(t *testing.T) {
				server := rollingReleaseServer(t, []githubRelease{
					{TagName: "newer-draft", Draft: true},
					{TagName: "newer-beta", Prerelease: true},
					{TagName: tt.version, Assets: releaseAssetsFor("SERVER", asset)},
				})
				defer server.Close()
				report, err := Plan(rollingManifest(rollingDependency(tt.recipe, tt.command)), Options{
					Profile: "default", OS: "linux", Arch: arch, Home: t.TempDir(), HTTPClient: server.Client(), RollingReleaseURL: server.URL,
				}, func(string) bool { return false }, nil, TierGeneric)
				if err != nil || len(report.Actions) != 1 {
					t.Fatalf("Plan = %+v, %v", report, err)
				}
				a := report.Actions[0]
				if a.Provider != TierUserLocal || a.UserLocal == nil {
					t.Fatalf("missing verified user-local action: %+v", a)
				}
				if got := a.UserLocal; got.Version != tt.version || got.Artifact != asset || got.Platform != "linux_"+arch || got.Command != tt.command || got.Digest != "sha256:"+strings.Repeat("a", 64) {
					t.Fatalf("wrong stable platform artifact: %+v", got)
				}
			})
		}
	}
}

func TestWorkstationRollingArtifactsInstallCompleteBundlesAndRecoverReceipts(t *testing.T) {
	for _, tt := range []struct {
		recipe, command, version, asset string
		files                           map[string]string
	}{
		{"uv", "uv", "0.13.6", "uv-x86_64-unknown-linux-gnu.tar.gz", map[string]string{"uv-x86_64-unknown-linux-gnu/uv": "uv fixture\n", "uv-x86_64-unknown-linux-gnu/uvx": "uvx fixture\n"}},
		{"pnpm-standalone", "pnpm", "v12.16.1", "pnpm-linux-x64.tar.gz", map[string]string{"pnpm": "pnpm fixture\n", "dist/node-gyp-bin/node-gyp": "node-gyp fixture\n"}},
		{"bun", "bun", "bun-v1.4.4", "bun-linux-x64.zip", map[string]string{"bun-linux-x64/bun": "bun fixture\n"}},
		{"gh", "gh", "v2.103.3", "gh_2.103.3_linux_amd64.tar.gz", map[string]string{"gh_2.103.3_linux_amd64/bin/gh": "gh fixture\n"}},
	} {
		for _, scenario := range []string{"success", "receipt failure", "digest mismatch", "missing command", "unsafe archive"} {
			t.Run(tt.recipe+"/"+scenario, func(t *testing.T) {
				home, stateRoot := t.TempDir(), t.TempDir()
				files := tt.files
				if scenario == "missing command" {
					files = map[string]string{"readme": "no executable"}
					if tt.recipe == "uv" {
						files["uv-x86_64-unknown-linux-gnu/uv"] = "uv without uvx\n"
					}
				}
				if scenario == "unsafe archive" {
					files = map[string]string{"../../escaped": "unsafe"}
				}
				var data []byte
				if strings.HasSuffix(tt.asset, ".zip") {
					data = zipFiles(t, files)
				} else {
					data = tarGz(t, files)
				}
				sum := sha256.Sum256(data)
				digest := hex.EncodeToString(sum[:])
				if scenario == "digest mismatch" {
					digest = strings.Repeat("0", 64)
				}
				var server *httptest.Server
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/"+tt.asset {
						if _, err := w.Write(data); err != nil {
							t.Error(err)
						}
						return
					}
					if err := json.NewEncoder(w).Encode(githubRelease{TagName: tt.version, Assets: []githubReleaseAsset{{Name: tt.asset, BrowserDownloadURL: server.URL + "/" + tt.asset, Digest: "sha256:" + digest}}}); err != nil {
						t.Error(err)
					}
				}))
				defer server.Close()
				dep := rollingDependency(tt.recipe, tt.command)
				report, err := Plan(rollingManifest(dep), Options{Profile: "default", OS: "linux", Arch: "amd64", Home: home, HTTPClient: server.Client(), RollingReleaseURL: server.URL}, func(string) bool { return false }, nil, TierGeneric)
				if err != nil || len(report.Actions) != 1 {
					t.Fatalf("Plan = %+v, %v", report, err)
				}
				action := report.Actions[0]
				installer := UserLocalInstaller{Home: home, StateRoot: stateRoot, Download: downloadAndVerify}
				if scenario == "receipt failure" {
					installer.Fault = func(point string) error {
						if point == "final:before" {
							return errors.New("injected receipt failure")
						}
						return nil
					}
				}
				err = installer.RunUserLocal(action, nil)
				if scenario != "success" && scenario != "receipt failure" {
					if err == nil {
						t.Fatal("unverified or incomplete artifact was accepted")
					}
					if _, err := os.Lstat(action.UserLocal.InstalledPath); !os.IsNotExist(err) {
						t.Fatalf("failed artifact published command: %v", err)
					}
					meta, err := LoadDependencyMetadata(DependencyMetadataPath(stateRoot))
					if err != nil || len(meta.Dependencies) != 0 {
						t.Fatalf("failed artifact acknowledged receipt: %+v, %v", meta, err)
					}
					return
				}
				if scenario == "receipt failure" {
					if err == nil {
						t.Fatal("receipt failure reported success")
					}
					meta, loadErr := LoadDependencyMetadata(DependencyMetadataPath(stateRoot))
					if loadErr != nil || len(meta.Dependencies) != 0 {
						t.Fatalf("uncommitted receipt visible: %+v, %v", meta, loadErr)
					}
					installer.Fault = nil
					installer.Download = func(string, string) ([]byte, error) { return nil, errors.New("recovery must not download again") }
					server.Close()
					selection := []UserLocalSelection{{Dependency: tt.recipe, Kind: "rolling", Recipe: tt.recipe, Platform: "linux_amd64", Requirement: manifest.DependencyRequirementRequired, Probes: tt.command}}
					for attempt := 0; attempt < 2; attempt++ {
						if _, err := installer.RecoverUserLocal(selection); err != nil {
							t.Fatalf("recover captured rolling artifact: %v", err)
						}
					}
				} else if err != nil {
					t.Fatal(err)
				}
				for path, want := range tt.files {
					got, err := os.ReadFile(filepath.Join(action.UserLocal.Destination, path))
					if err != nil || string(got) != want {
						t.Fatalf("bundle lost %s: %q, %v", path, got, err)
					}
				}
				commands := []string{tt.command}
				if tt.command == "uv" {
					commands = append(commands, "uvx")
				}
				for _, command := range commands {
					info, err := os.Stat(filepath.Join(home, ".local/bin", command))
					if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
						t.Fatalf("missing executable %s: %v, %v", command, info, err)
					}
				}
				if tt.command == "pnpm" {
					binary, err := filepath.EvalSymlinks(action.UserLocal.InstalledPath)
					if err != nil {
						t.Fatal(err)
					}
					got, err := os.ReadFile(filepath.Join(filepath.Dir(binary), "dist/node-gyp-bin/node-gyp"))
					if err != nil || string(got) != "node-gyp fixture\n" {
						t.Fatalf("pnpm auxiliary payload unavailable beside executable: %q, %v", got, err)
					}
				}
				meta, err := LoadDependencyMetadata(DependencyMetadataPath(stateRoot))
				if err != nil || len(meta.Dependencies) != 1 {
					t.Fatalf("resolved receipt missing: %+v, %v", meta, err)
				}
				r := meta.Dependencies[0]
				if r.Provider != "user-local" || r.Version != tt.version || r.Artifact != tt.asset || r.Digest != "sha256:"+digest || r.Checksum != digest || r.Platform != "linux_amd64" || r.URL != action.UserLocal.URL || r.Path != action.UserLocal.InstalledPath || len(r.Publications) != 1+len(commands) {
					t.Fatalf("receipt does not match complete published artifact: %+v", r)
				}
			})
		}
	}
}

func TestWorkstationRollingPoliciesPreserveHomebrewAndExistingCommands(t *testing.T) {
	m, err := manifest.LoadFile(filepath.Join("..", "..", "dots.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected metadata lookup: %s", r.URL)
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	for _, dep := range selectedsurface.Evaluate(*m, []string{"uv", "pnpm", "bun", "gh"}, "linux").Dependencies {
		t.Run(dep.Name, func(t *testing.T) {
			wantRecipe := dep.Command
			if dep.Command == "pnpm" {
				wantRecipe = "pnpm-standalone"
			}
			if dep.UserLocal != nil || dep.RollingUserLocal == nil || dep.RollingUserLocal.Recipe != wantRecipe {
				t.Fatalf("manifest does not select latest stable for %s: %+v", dep.Name, dep)
			}
			for _, goos := range []string{"linux", "darwin"} {
				opts := Options{Profile: "default", OS: goos, Arch: "arm64", Home: t.TempDir(), HTTPClient: server.Client(), RollingReleaseURL: server.URL}
				report, err := Plan(rollingManifest(dep), opts, func(command string) bool { return command == "brew" }, nil, TierHomebrew)
				if err != nil || len(report.Actions) != 1 || report.Actions[0].Provider != TierHomebrew || report.Actions[0].Package != dep.Brew || report.Actions[0].UserLocal != nil {
					t.Fatalf("%s Homebrew policy changed: %+v, %v", goos, report, err)
				}
				report, err = Plan(rollingManifest(dep), opts, func(command string) bool { return command == dep.Command }, nil, TierGeneric)
				if err != nil || len(report.Actions) != 0 {
					t.Fatalf("existing %s command did not satisfy dependency: %+v, %v", goos, report, err)
				}
			}
		})
	}
}

func TestWorkstationRollingProvidersRejectUnverifiableMetadata(t *testing.T) {
	for _, tt := range []struct{ recipe, command, asset string }{
		{"uv", "uv", "uv-x86_64-unknown-linux-gnu.tar.gz"},
		{"pnpm-standalone", "pnpm", "pnpm-linux-x64.tar.gz"},
		{"bun", "bun", "bun-linux-x64.zip"},
		{"gh", "gh", "gh_1.2.3_linux_amd64.tar.gz"},
	} {
		for _, failure := range []string{"no stable release", "missing asset", "missing digest", "duplicate asset"} {
			t.Run(tt.recipe+"/"+failure, func(t *testing.T) {
				release := githubRelease{TagName: "v1.2.3", Assets: releaseAssetsFor("SERVER", tt.asset)}
				switch failure {
				case "no stable release":
					release.Prerelease = true
				case "missing asset":
					release.Assets = nil
				case "missing digest":
					release.Assets[0].Digest = ""
				case "duplicate asset":
					release.Assets = append(release.Assets, release.Assets[0])
				}
				server := rollingReleaseServer(t, []githubRelease{release})
				defer server.Close()
				_, err := Plan(rollingManifest(rollingDependency(tt.recipe, tt.command)), Options{Profile: "default", OS: "linux", Arch: "amd64", Home: t.TempDir(), HTTPClient: server.Client(), RollingReleaseURL: server.URL}, func(string) bool { return false }, nil, TierGeneric)
				if err == nil {
					t.Fatal("unverifiable metadata was accepted")
				}
			})
		}
	}
}

type workstationMetadataTransport func(*http.Request) (*http.Response, error)

func (f workstationMetadataTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestWorkstationRollingProvidersRequireOfficialImmutableURLs(t *testing.T) {
	for _, tt := range []struct{ recipe, command, repository, version, asset string }{
		{"uv", "uv", "astral-sh/uv", "0.13.6", "uv-x86_64-unknown-linux-gnu.tar.gz"},
		{"pnpm-standalone", "pnpm", "pnpm/pnpm", "v12.16.1", "pnpm-linux-x64.tar.gz"},
		{"bun", "bun", "oven-sh/bun", "bun-v1.4.4", "bun-linux-x64.zip"},
		{"gh", "gh", "cli/cli", "v2.103.3", "gh_2.103.3_linux_amd64.tar.gz"},
	} {
		for _, scenario := range []string{"valid", "wrong repository", "wrong version", "wrong asset", "wrong host", "insecure URL", "malformed digest"} {
			t.Run(tt.recipe+"/"+scenario, func(t *testing.T) {
				url := "https://github.com/" + tt.repository + "/releases/download/" + tt.version + "/" + tt.asset
				digest := "sha256:" + strings.Repeat("a", 64)
				switch scenario {
				case "wrong repository":
					url = strings.Replace(url, "/"+tt.repository+"/", "/untrusted/fork/", 1)
				case "wrong version":
					url = strings.Replace(url, "/"+tt.version+"/", "/other-version/", 1)
				case "wrong asset":
					url += ".unexpected"
				case "wrong host":
					url = strings.Replace(url, "github.com", "untrusted.invalid", 1)
				case "insecure URL":
					url = strings.Replace(url, "https:", "http:", 1)
				case "malformed digest":
					digest = "sha256:invalid"
				}
				body, err := json.Marshal(githubRelease{TagName: tt.version, Assets: []githubReleaseAsset{{Name: tt.asset, BrowserDownloadURL: url, Digest: digest}}})
				if err != nil {
					t.Fatal(err)
				}
				client := &http.Client{Transport: workstationMetadataTransport(func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != "https://api.github.com/repos/"+tt.repository+"/releases/latest" {
						t.Errorf("unexpected metadata authority: %s", r.URL)
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
				})}
				opts := Options{Profile: "default", OS: "linux", Arch: "amd64", Home: t.TempDir(), HTTPClient: client}
				m := rollingManifest(rollingDependency(tt.recipe, tt.command))
				report, err := Plan(m, opts, func(string) bool { return false }, nil, TierGeneric)
				if scenario != "valid" {
					if err == nil {
						t.Fatal("untrusted official artifact identity accepted")
					}
					return
				}
				if err != nil || len(report.Actions) != 1 || report.Actions[0].UserLocal.URL != url {
					t.Fatalf("official immutable identity rejected: %+v, %v", report, err)
				}
				// A prepared version-qualified gh asset must remain pinned too.
				opts.ResolvedUserLocal = map[string]UserLocalArtifact{tt.recipe: *report.Actions[0].UserLocal}
				opts.HTTPClient = &http.Client{Transport: workstationMetadataTransport(func(*http.Request) (*http.Response, error) {
					return nil, errors.New("prepared artifact must not resolve latest again")
				})}
				report, err = Plan(m, opts, func(string) bool { return false }, nil, TierGeneric)
				if err != nil || len(report.Actions) != 1 || report.Actions[0].UserLocal.URL != url {
					t.Fatalf("prepared artifact changed: %+v, %v", report, err)
				}
			})
		}
	}
}
