package deps

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

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

func rollingCodexDependency() manifest.Dependency {
	return manifest.Dependency{Name: "codex", Command: "codex", RollingUserLocal: &manifest.RollingUserLocalProvider{Recipe: "codex"}}
}

func rollingCodexManifest() manifest.Manifest {
	return manifest.Manifest{Profiles: map[string]manifest.Profile{"default": {Tags: []string{"agents"}}}, Entries: []manifest.Entry{{Source: "source", Target: "target", Tags: []string{"agents"}, Dependencies: []manifest.Dependency{rollingCodexDependency()}}}}
}

func lookupSetInternal(commands ...string) Lookup {
	present := map[string]bool{}
	for _, command := range commands {
		present[command] = true
	}
	return func(command string) bool { return present[command] }
}
