package deps

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

func TestRollingClaudeSelectsStableVerifiedArtifactForSupportedPlatforms(t *testing.T) {
	platforms := []struct {
		os, arch, platform string
	}{
		{"darwin", "amd64", "darwin-x64"},
		{"darwin", "arm64", "darwin-arm64"},
		{"linux", "amd64", "linux-x64"},
		{"linux", "arm64", "linux-arm64"},
	}
	for _, tt := range platforms {
		t.Run(tt.os+"_"+tt.arch, func(t *testing.T) {
			server := claudeReleaseServer(t, "2.1.220", claudeManifest("2.1.220", tt.platform, strings.Repeat("b", 64)), nil)
			artifact, ok, err := rollingUserLocalArtifact(rollingClaudeDependency(), Options{OS: tt.os, Arch: tt.arch, Home: "/tmp/home", HTTPClient: server.Client(), RollingReleaseURL: server.URL})
			if err != nil {
				t.Fatalf("rollingUserLocalArtifact() error = %v", err)
			}
			if !ok || artifact.Version != "2.1.220" || artifact.Artifact != "claude" || artifact.Platform != tt.platform {
				t.Fatalf("artifact = %#v, want verified %s artifact", artifact, tt.platform)
			}
			if artifact.URL != server.URL+"/2.1.220/"+tt.platform+"/claude" || artifact.Digest != "sha256:"+strings.Repeat("b", 64) {
				t.Fatalf("artifact = %#v, want immutable URL and digest", artifact)
			}
			if artifact.Destination != "/tmp/home/.local/opt/claude/2.1.220" || artifact.InstalledPath != "/tmp/home/.local/bin/claude" {
				t.Fatalf("artifact paths = (%q, %q)", artifact.Destination, artifact.InstalledPath)
			}
		})
	}
}

func TestRollingClaudeFailsClosedForInvalidMetadata(t *testing.T) {
	tests := []struct {
		name, stable string
		manifest     any
		want         string
	}{
		{"prerelease channel", "2.1.221-beta.1", claudeManifest("2.1.221-beta.1", "linux-x64", strings.Repeat("a", 64)), "malformed version"},
		{"malformed channel", "latest", nil, "malformed version"},
		{"version mismatch", "2.1.220", claudeManifest("2.1.219", "linux-x64", strings.Repeat("a", 64)), "does not match"},
		{"missing platform", "2.1.220", claudeManifest("2.1.220", "darwin-x64", strings.Repeat("a", 64)), "no claude artifact"},
		{"missing checksum", "2.1.220", claudeManifest("2.1.220", "linux-x64", ""), "malformed"},
		{"malformed checksum", "2.1.220", claudeManifest("2.1.220", "linux-x64", "not-a-digest"), "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := claudeReleaseServer(t, tt.stable, tt.manifest, nil)
			_, _, err := rollingUserLocalArtifact(rollingClaudeDependency(), Options{OS: "linux", Arch: "amd64", HTTPClient: server.Client(), RollingReleaseURL: server.URL})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRollingClaudeRawArtifactRequiresMatchingChecksum(t *testing.T) {
	body := []byte("#!/bin/sh\nprintf 'claude fixture\\n'\n")
	sum := sha256.Sum256(body)
	server := claudeReleaseServer(t, "2.1.220", claudeManifest("2.1.220", "linux-x64", hex.EncodeToString(sum[:])), body)
	artifact, _, err := rollingUserLocalArtifact(rollingClaudeDependency(), Options{OS: "linux", Arch: "amd64", Home: t.TempDir(), HTTPClient: server.Client(), RollingReleaseURL: server.URL})
	if err != nil {
		t.Fatalf("resolve artifact: %v", err)
	}
	artifact.Checksum = strings.Repeat("0", 64)
	err = InstallUserLocal(t.TempDir(), InstallAction{UserLocal: &artifact})
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("InstallUserLocal() error = %v, want checksum mismatch", err)
	}
}

func TestRollingClaudeInstallsRawExecutableAndLink(t *testing.T) {
	body := []byte("#!/bin/sh\nprintf 'claude fixture\\n'\n")
	sum := sha256.Sum256(body)
	home := t.TempDir()
	server := claudeReleaseServer(t, "2.1.220", claudeManifest("2.1.220", "linux-x64", hex.EncodeToString(sum[:])), body)
	artifact, _, err := rollingUserLocalArtifact(rollingClaudeDependency(), Options{OS: "linux", Arch: "amd64", Home: home, HTTPClient: server.Client(), RollingReleaseURL: server.URL})
	if err != nil {
		t.Fatalf("resolve artifact: %v", err)
	}
	if err := InstallUserLocal(home, InstallAction{UserLocal: &artifact}); err != nil {
		t.Fatalf("InstallUserLocal() error = %v", err)
	}
	target, err := os.Readlink(filepath.Join(home, ".local", "bin", "claude"))
	if err != nil || target != filepath.Join(home, ".local", "opt", "claude", "2.1.220", "claude") {
		t.Fatalf("claude link = (%q, %v)", target, err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("claude executable = (%v, %v)", info, err)
	}
}

func TestRollingCodexSelectsStableOfficialArtifactForSupportedPlatforms(t *testing.T) {
	platforms := []struct {
		os, arch, asset string
	}{
		{"darwin", "amd64", "codex-package-x86_64-apple-darwin.tar.gz"},
		{"darwin", "arm64", "codex-package-aarch64-apple-darwin.tar.gz"},
		{"linux", "amd64", "codex-package-x86_64-unknown-linux-musl.tar.gz"},
		{"linux", "arm64", "codex-package-aarch64-unknown-linux-musl.tar.gz"},
	}

	for _, tt := range platforms {
		t.Run(tt.os+"_"+tt.arch, func(t *testing.T) {
			server := rollingReleaseServer(t, []githubRelease{
				{TagName: "rust-v0.148.0-alpha.1", Prerelease: true, Assets: releaseAssets("http://unused.invalid")},
				{TagName: "rust-v0.147.0", Assets: releaseAssets("SERVER")},
			})
			artifact, ok, err := rollingUserLocalArtifact(rollingCodexDependency(), Options{OS: tt.os, Arch: tt.arch, Home: "/tmp/home", HTTPClient: server.Client(), RollingReleaseURL: server.URL + "/releases"})
			if err != nil {
				t.Fatalf("rollingUserLocalArtifact() error = %v", err)
			}
			if !ok || artifact.Version != "rust-v0.147.0" || artifact.Artifact != tt.asset || artifact.Platform != tt.os+"_"+tt.arch {
				t.Fatalf("artifact = %#v, want stable %s artifact", artifact, tt.asset)
			}
			if artifact.Digest != "sha256:"+strings.Repeat("a", 64) || artifact.Checksum != strings.Repeat("a", 64) {
				t.Fatalf("artifact digest = (%q, %q), want verified sha256", artifact.Digest, artifact.Checksum)
			}
			if artifact.Destination != "/tmp/home/.local/opt/codex/rust-v0.147.0" {
				t.Fatalf("destination = %q", artifact.Destination)
			}
		})
	}
}

func TestRollingGitHubRecipesSelectStableVerifiedArtifactsForSupportedPlatforms(t *testing.T) {
	recipes := []struct {
		name, command, version string
		assets                 map[string]string
	}{
		{"opencode", "opencode", "v1.2.3", opencodePlatformAssets},
		{"antigravity", "agy", "1.1.0", antigravityPlatformAssets},
		{"copilot", "copilot", "v1.0.0", copilotPlatformAssets},
	}
	platforms := []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"}
	for _, recipe := range recipes {
		t.Run(recipe.name, func(t *testing.T) {
			for _, platform := range platforms {
				t.Run(platform, func(t *testing.T) {
					assetName := recipe.assets[platform]
					goos, goarch, _ := strings.Cut(platform, "_")
					server := rollingReleaseServer(t, []githubRelease{
						{TagName: recipe.version + "-beta.1", Prerelease: true, Assets: releaseAssetsFor("SERVER", assetName)},
						{TagName: recipe.version, Assets: releaseAssetsFor("SERVER", assetName)},
					})
					artifact, ok, err := rollingUserLocalArtifact(rollingDependency(recipe.name, recipe.command), Options{
						OS: goos, Arch: goarch, Home: "/tmp/home", HTTPClient: server.Client(), RollingReleaseURL: server.URL + "/releases",
					})
					if err != nil {
						t.Fatalf("rollingUserLocalArtifact() error = %v", err)
					}
					if !ok || artifact.Version != recipe.version || artifact.Artifact != assetName || artifact.Platform != platform {
						t.Fatalf("artifact = %#v, want stable %s artifact", artifact, assetName)
					}
					if artifact.Command != recipe.command || artifact.Digest != "sha256:"+strings.Repeat("a", 64) {
						t.Fatalf("artifact = %#v, want verified %s command", artifact, recipe.command)
					}
				})
			}
		})
	}
}

func TestRollingOpenCodeInstallsTarAndZipArtifacts(t *testing.T) {
	tests := []struct {
		name, archive string
		data          func(*testing.T, map[string]string) []byte
	}{
		{"linux tar", "opencode-linux-x64.tar.gz", tarGz},
		{"macOS zip", "opencode-darwin-arm64.zip", zipFiles},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			data := tt.data(t, map[string]string{"opencode": "#!/bin/sh\nprintf 'opencode fixture\\n'\n"})
			oldDownload := downloadUserLocalArtifact
			downloadUserLocalArtifact = func(string, string) ([]byte, error) { return data, nil }
			t.Cleanup(func() { downloadUserLocalArtifact = oldDownload })

			artifact := UserLocalArtifact{Recipe: "opencode", Version: "v1.2.3", URL: "https://example.invalid/" + tt.archive, Checksum: "fixture", Layout: userLocalLayoutBundle, Command: "opencode"}
			if err := InstallUserLocal(home, InstallAction{UserLocal: &artifact}); err != nil {
				t.Fatalf("InstallUserLocal() error = %v", err)
			}
			want := filepath.Join(home, ".local", "opt", "opencode", "v1.2.3", "opencode")
			if target, err := os.Readlink(filepath.Join(home, ".local", "bin", "opencode")); err != nil || target != want {
				t.Fatalf("opencode link = (%q, %v), want %q", target, err, want)
			}
		})
	}
}

func TestRollingAntigravityInstallsVendorBinaryAsAgy(t *testing.T) {
	home := t.TempDir()
	data := tarGz(t, map[string]string{"antigravity": "#!/bin/sh\nprintf 'antigravity fixture\\n'\n"})
	oldDownload := downloadUserLocalArtifact
	downloadUserLocalArtifact = func(string, string) ([]byte, error) { return data, nil }
	t.Cleanup(func() { downloadUserLocalArtifact = oldDownload })

	artifact := UserLocalArtifact{Recipe: "antigravity", Version: "1.1.0", URL: "https://example.invalid/agy_cli_linux_x64.tar.gz", Checksum: "fixture", Layout: userLocalLayoutBundle, Command: "agy"}
	if err := InstallUserLocal(home, InstallAction{UserLocal: &artifact}); err != nil {
		t.Fatalf("InstallUserLocal() error = %v", err)
	}
	want := filepath.Join(home, ".local", "opt", "antigravity", "1.1.0", "antigravity")
	if target, err := os.Readlink(filepath.Join(home, ".local", "bin", "agy")); err != nil || target != want {
		t.Fatalf("agy link = (%q, %v), want %q", target, err, want)
	}
	if _, err := os.Lstat(filepath.Join(home, ".local", "bin", "antigravity")); !os.IsNotExist(err) {
		t.Fatalf("unexpected antigravity command link: %v", err)
	}
}

func TestRollingGitHubRecipesFailClosedForInvalidMetadata(t *testing.T) {
	recipes := []struct {
		name, command, asset string
	}{
		{"opencode", "opencode", "opencode-linux-x64.tar.gz"},
		{"antigravity", "agy", "agy_cli_linux_x64.tar.gz"},
		{"copilot", "copilot", "copilot-linux-x64.tar.gz"},
	}
	for _, recipe := range recipes {
		t.Run(recipe.name, func(t *testing.T) {
			tests := []struct {
				name     string
				releases []githubRelease
				want     string
			}{
				{"no stable release", []githubRelease{{TagName: "v2.0.0-beta.1", Prerelease: true}}, "no stable release"},
				{"missing platform asset", []githubRelease{{TagName: "v1.0.0"}}, "has no asset"},
				{"missing digest", []githubRelease{{TagName: "v1.0.0", Assets: []githubReleaseAsset{{Name: recipe.asset, BrowserDownloadURL: "SERVER/" + recipe.asset}}}}, "must use sha256"},
				{"duplicate asset", []githubRelease{{TagName: "v1.0.0", Assets: releaseAssetsFor("SERVER", recipe.asset, recipe.asset)}}, "2 assets"},
				{"unexpected platform asset", []githubRelease{{TagName: "v1.0.0", Assets: releaseAssetsFor("SERVER", "unexpected-"+recipe.asset)}}, "has no asset"},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					server := rollingReleaseServer(t, tt.releases)
					_, _, err := rollingUserLocalArtifact(rollingDependency(recipe.name, recipe.command), Options{OS: "linux", Arch: "amd64", HTTPClient: server.Client(), RollingReleaseURL: server.URL + "/releases"})
					if err == nil || !strings.Contains(err.Error(), tt.want) {
						t.Fatalf("error = %v, want containing %q", err, tt.want)
					}
				})
			}
		})
	}
}

func TestRollingGitHubRecipesRejectMismatchedDependencyCommands(t *testing.T) {
	recipes := []struct {
		name, command, asset string
	}{
		{"opencode", "opencode", "opencode-linux-x64.tar.gz"},
		{"antigravity", "agy", "agy_cli_linux_x64.tar.gz"},
		{"copilot", "copilot", "copilot-linux-x64.tar.gz"},
	}
	for _, recipe := range recipes {
		t.Run(recipe.name, func(t *testing.T) {
			dep := rollingDependency(recipe.name, "wrong-command")
			resolved := UserLocalArtifact{Recipe: recipe.name, Version: "v1.0.0", Artifact: recipe.asset, URL: "https://example.invalid/asset", Checksum: strings.Repeat("a", 64), Platform: "linux_amd64"}
			_, _, err := rollingUserLocalArtifact(dep, Options{OS: "linux", Arch: "amd64", ResolvedUserLocal: map[string]UserLocalArtifact{recipe.name: resolved}})
			if err == nil || !strings.Contains(err.Error(), "requires command") || !strings.Contains(err.Error(), recipe.command) {
				t.Fatalf("error = %v, want required command %q", err, recipe.command)
			}
		})
	}
}

func TestInstallRollingBundleReplacesPreexistingVersionWithVerifiedArtifact(t *testing.T) {
	home := t.TempDir()
	optDir := filepath.Join(home, ".local", "opt", "antigravity", "1.1.0")
	if err := os.MkdirAll(optDir, 0o755); err != nil {
		t.Fatalf("mkdir preexisting bundle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(optDir, "antigravity"), []byte("stale bytes\n"), 0o755); err != nil {
		t.Fatalf("write preexisting binary: %v", err)
	}
	data := tarGz(t, map[string]string{"antigravity": "verified bytes\n"})
	oldDownload := downloadUserLocalArtifact
	downloadUserLocalArtifact = func(string, string) ([]byte, error) { return data, nil }
	t.Cleanup(func() { downloadUserLocalArtifact = oldDownload })

	artifact := UserLocalArtifact{Recipe: "antigravity", Version: "1.1.0", URL: "https://example.invalid/agy_cli_linux_x64.tar.gz", Checksum: "verified", Layout: userLocalLayoutBundle, Command: "agy"}
	if err := InstallUserLocal(home, InstallAction{UserLocal: &artifact}); err != nil {
		t.Fatalf("InstallUserLocal() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(optDir, "antigravity"))
	if err != nil || string(got) != "verified bytes\n" {
		t.Fatalf("installed binary = (%q, %v), want verified artifact bytes", got, err)
	}
}

func TestInstallRollingBundlePreservesPreexistingVersionWhenVerifiedArchiveIsInvalid(t *testing.T) {
	home := t.TempDir()
	optDir := filepath.Join(home, ".local", "opt", "antigravity", "1.1.0")
	if err := os.MkdirAll(optDir, 0o755); err != nil {
		t.Fatalf("mkdir preexisting bundle: %v", err)
	}
	oldBinary := filepath.Join(optDir, "antigravity")
	if err := os.WriteFile(oldBinary, []byte("previous bytes\n"), 0o755); err != nil {
		t.Fatalf("write preexisting binary: %v", err)
	}
	data := tarGz(t, map[string]string{"unexpected": "verified but invalid layout\n"})
	oldDownload := downloadUserLocalArtifact
	downloadUserLocalArtifact = func(string, string) ([]byte, error) { return data, nil }
	t.Cleanup(func() { downloadUserLocalArtifact = oldDownload })

	artifact := UserLocalArtifact{Recipe: "antigravity", Version: "1.1.0", URL: "https://example.invalid/agy_cli_linux_x64.tar.gz", Checksum: "verified", Layout: userLocalLayoutBundle, Command: "agy"}
	if err := InstallUserLocal(home, InstallAction{UserLocal: &artifact}); err == nil || !strings.Contains(err.Error(), "stat extracted executable") {
		t.Fatalf("InstallUserLocal() error = %v, want invalid layout rejection", err)
	}
	got, err := os.ReadFile(oldBinary)
	if err != nil || string(got) != "previous bytes\n" {
		t.Fatalf("preserved binary = (%q, %v), want previous bytes", got, err)
	}
}

func TestRollingCodexFailsClosedForInvalidMetadata(t *testing.T) {
	tests := []struct {
		name     string
		releases []githubRelease
		want     string
	}{
		{"no stable release", []githubRelease{{TagName: "rust-v1.0.0-alpha", Prerelease: true}}, "no stable release"},
		{"missing platform asset", []githubRelease{{TagName: "rust-v1.0.0"}}, "has no asset"},
		{"missing digest", []githubRelease{{TagName: "rust-v1.0.0", Assets: []githubReleaseAsset{{Name: "codex-package-x86_64-unknown-linux-musl.tar.gz", BrowserDownloadURL: "SERVER/asset"}}}}, "must use sha256"},
		{"duplicate asset", []githubRelease{{TagName: "rust-v1.0.0", Assets: []githubReleaseAsset{
			{Name: "codex-package-x86_64-unknown-linux-musl.tar.gz"},
			{Name: "codex-package-x86_64-unknown-linux-musl.tar.gz"},
		}}}, "2 assets"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := rollingReleaseServer(t, tt.releases)
			_, _, err := rollingUserLocalArtifact(rollingCodexDependency(), Options{OS: "linux", Arch: "amd64", HTTPClient: server.Client(), RollingReleaseURL: server.URL + "/releases"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestPlanSkipsRollingResolutionWhenCodexIsPresent(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	m := rollingCodexManifest()
	report, err := Plan(m, Options{Profile: "default", OS: "linux", Arch: "amd64", HTTPClient: server.Client(), RollingReleaseURL: server.URL}, lookupSetInternal("codex"), func(string) bool { return false }, TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 0 || requests.Load() != 0 {
		t.Fatalf("report/actions = %#v, requests = %d; want no resolution", report.Actions, requests.Load())
	}
}

func TestPlanSkipsRollingResolutionWhenClaudeIsPresent(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	m := manifest.Manifest{Profiles: map[string]manifest.Profile{"default": {Tags: []string{"agents"}}}, Entries: []manifest.Entry{{Source: "source", Target: "target", Tags: []string{"agents"}, Dependencies: []manifest.Dependency{rollingClaudeDependency()}}}}
	report, err := Plan(m, Options{Profile: "default", OS: "linux", Arch: "amd64", HTTPClient: server.Client(), RollingReleaseURL: server.URL}, lookupSetInternal("claude"), func(string) bool { return false }, TierDebian)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(report.Actions) != 0 || requests.Load() != 0 {
		t.Fatalf("report/actions = %#v, requests = %d; want no resolution", report.Actions, requests.Load())
	}
}

func TestPlanSkipsRollingGitHubResolutionForEachPresentCommand(t *testing.T) {
	dependencies := []manifest.Dependency{
		rollingDependency("opencode", "opencode"),
		rollingDependency("antigravity", "agy"),
		rollingDependency("copilot", "copilot"),
	}
	for _, dep := range dependencies {
		t.Run(dep.Name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Error(w, "unexpected", http.StatusInternalServerError)
			}))
			defer server.Close()

			report, err := Plan(rollingManifest(dep), Options{Profile: "default", OS: "linux", Arch: "amd64", HTTPClient: server.Client(), RollingReleaseURL: server.URL}, lookupSetInternal(dep.Command), func(string) bool { return false }, TierDebian)
			if err != nil {
				t.Fatalf("Plan() error = %v", err)
			}
			if len(report.Actions) != 0 || requests.Load() != 0 {
				t.Fatalf("report/actions = %#v, requests = %d; want no resolution", report.Actions, requests.Load())
			}
		})
	}
}

func rollingReleaseServer(t *testing.T, releases []githubRelease) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := range releases {
			for j := range releases[i].Assets {
				releases[i].Assets[j].BrowserDownloadURL = strings.ReplaceAll(releases[i].Assets[j].BrowserDownloadURL, "SERVER", server.URL)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(releases); err != nil {
			t.Errorf("encode releases: %v", err)
		}
	}))
	return server
}

func claudeReleaseServer(t *testing.T, stable string, releaseManifest any, artifact []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/stable":
			_, _ = w.Write([]byte(stable))
		case r.URL.Path == "/"+stable+"/manifest.json":
			if releaseManifest == nil {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(releaseManifest)
		case strings.HasSuffix(r.URL.Path, "/claude") && artifact != nil:
			_, _ = w.Write(artifact)
		default:
			http.NotFound(w, r)
		}
	}))
}

func claudeManifest(version, platform, checksum string) map[string]any {
	return map[string]any{
		"version": version,
		"platforms": map[string]any{
			platform: map[string]any{"binary": "claude", "checksum": checksum},
		},
	}
}

func releaseAssets(base string) []githubReleaseAsset {
	names := []string{
		"codex-package-x86_64-apple-darwin.tar.gz",
		"codex-package-aarch64-apple-darwin.tar.gz",
		"codex-package-x86_64-unknown-linux-musl.tar.gz",
		"codex-package-aarch64-unknown-linux-musl.tar.gz",
	}
	assets := make([]githubReleaseAsset, 0, len(names))
	for _, name := range names {
		assets = append(assets, githubReleaseAsset{Name: name, BrowserDownloadURL: base + "/" + name, Digest: "sha256:" + strings.Repeat("a", 64)})
	}
	return assets
}

func releaseAssetsFor(base string, names ...string) []githubReleaseAsset {
	assets := make([]githubReleaseAsset, 0, len(names))
	for _, name := range names {
		assets = append(assets, githubReleaseAsset{Name: name, BrowserDownloadURL: base + "/" + name, Digest: "sha256:" + strings.Repeat("a", 64)})
	}
	return assets
}

func zipFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func rollingDependency(name, command string) manifest.Dependency {
	return manifest.Dependency{Name: name, Command: command, RollingUserLocal: &manifest.RollingUserLocalProvider{Recipe: name}}
}

func rollingCodexDependency() manifest.Dependency {
	return manifest.Dependency{Name: "codex", Command: "codex", RollingUserLocal: &manifest.RollingUserLocalProvider{Recipe: "codex"}}
}

func rollingClaudeDependency() manifest.Dependency {
	return manifest.Dependency{Name: "claude", Command: "claude", RollingUserLocal: &manifest.RollingUserLocalProvider{Recipe: "claude"}}
}

func rollingCodexManifest() manifest.Manifest {
	return rollingManifest(rollingCodexDependency())
}

func rollingManifest(dependencies ...manifest.Dependency) manifest.Manifest {
	return manifest.Manifest{Profiles: map[string]manifest.Profile{"default": {Tags: []string{"agents"}}}, Entries: []manifest.Entry{{Source: "source", Target: "target", Tags: []string{"agents"}, Dependencies: dependencies}}}
}

func lookupSetInternal(commands ...string) Lookup {
	present := map[string]bool{}
	for _, command := range commands {
		present[command] = true
	}
	return func(command string) bool { return present[command] }
}
