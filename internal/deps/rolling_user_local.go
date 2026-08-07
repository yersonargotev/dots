package deps

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/yersonargotev/dots/internal/manifest"
)

const (
	codexRollingRecipe        = "codex"
	codexReleasesAPIURL       = "https://api.github.com/repos/openai/codex/releases/latest"
	opencodeRollingRecipe     = "opencode"
	opencodeReleasesAPIURL    = "https://api.github.com/repos/anomalyco/opencode/releases/latest"
	antigravityRollingRecipe  = "antigravity"
	antigravityReleasesAPIURL = "https://api.github.com/repos/google-antigravity/antigravity-cli/releases/latest"
	copilotRollingRecipe      = "copilot"
	copilotReleasesAPIURL     = "https://api.github.com/repos/github/copilot-cli/releases/latest"
	claudeRollingRecipe       = "claude"
	claudeReleaseBaseURL      = "https://downloads.claude.ai/claude-code-releases"
	maxReleaseMetadata        = 4 << 20
)

type claudeReleaseManifest struct {
	Version   string                            `json:"version"`
	Platforms map[string]claudePlatformArtifact `json:"platforms"`
}

type claudePlatformArtifact struct {
	Binary   string `json:"binary"`
	Checksum string `json:"checksum"`
}

type githubRelease struct {
	TagName    string               `json:"tag_name"`
	Draft      bool                 `json:"draft"`
	Prerelease bool                 `json:"prerelease"`
	Assets     []githubReleaseAsset `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type githubRollingSpec struct {
	recipe         string
	repository     string
	releasesURL    string
	command        string
	platformAssets map[string]string
}

var (
	codexPlatformAssets = map[string]string{
		"darwin_amd64": "codex-package-x86_64-apple-darwin.tar.gz",
		"darwin_arm64": "codex-package-aarch64-apple-darwin.tar.gz",
		"linux_amd64":  "codex-package-x86_64-unknown-linux-musl.tar.gz",
		"linux_arm64":  "codex-package-aarch64-unknown-linux-musl.tar.gz",
	}
	opencodePlatformAssets = map[string]string{
		"darwin_amd64": "opencode-darwin-x64.zip",
		"darwin_arm64": "opencode-darwin-arm64.zip",
		"linux_amd64":  "opencode-linux-x64.tar.gz",
		"linux_arm64":  "opencode-linux-arm64.tar.gz",
	}
	antigravityPlatformAssets = map[string]string{
		"darwin_amd64": "agy_cli_mac_x64.tar.gz",
		"darwin_arm64": "agy_cli_mac_arm64.tar.gz",
		"linux_amd64":  "agy_cli_linux_x64.tar.gz",
		"linux_arm64":  "agy_cli_linux_arm64.tar.gz",
	}
	copilotPlatformAssets = map[string]string{
		"darwin_amd64": "copilot-darwin-x64.tar.gz",
		"darwin_arm64": "copilot-darwin-arm64.tar.gz",
		"linux_amd64":  "copilot-linux-x64.tar.gz",
		"linux_arm64":  "copilot-linux-arm64.tar.gz",
	}
	githubRollingSpecs = map[string]githubRollingSpec{
		codexRollingRecipe: {
			recipe: codexRollingRecipe, repository: "openai/codex", releasesURL: codexReleasesAPIURL,
			command: "codex", platformAssets: codexPlatformAssets,
		},
		opencodeRollingRecipe: {
			recipe: opencodeRollingRecipe, repository: "anomalyco/opencode", releasesURL: opencodeReleasesAPIURL,
			command: "opencode", platformAssets: opencodePlatformAssets,
		},
		antigravityRollingRecipe: {
			recipe: antigravityRollingRecipe, repository: "google-antigravity/antigravity-cli", releasesURL: antigravityReleasesAPIURL,
			command: "agy", platformAssets: antigravityPlatformAssets,
		},
		copilotRollingRecipe: {
			recipe: copilotRollingRecipe, repository: "github/copilot-cli", releasesURL: copilotReleasesAPIURL,
			command: "copilot", platformAssets: copilotPlatformAssets,
		},
	}
)

func rollingUserLocalArtifact(dep manifest.Dependency, opts Options) (UserLocalArtifact, bool, error) {
	if dep.RollingUserLocal == nil {
		return UserLocalArtifact{}, false, nil
	}
	if dep.UserLocal != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q cannot declare both user_local and rolling_user_local", dep.Name)
	}
	recipe := strings.TrimSpace(dep.RollingUserLocal.Recipe)
	if spec, ok := githubRollingSpecs[recipe]; ok {
		return rollingGitHubArtifact(dep, opts, spec)
	}
	switch recipe {
	case claudeRollingRecipe:
		return rollingClaudeArtifact(dep, opts)
	default:
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q declares unsupported rolling_user_local recipe %q", dep.Name, recipe)
	}
}

func rollingGitHubArtifact(dep manifest.Dependency, opts Options, spec githubRollingSpec) (UserLocalArtifact, bool, error) {
	if strings.TrimSpace(dep.Command) != spec.command || len(dep.Commands) != 0 {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q rolling_user_local recipe %q requires command %q", dep.Name, spec.recipe, spec.command)
	}
	platform := platformKey(opts.OS, opts.Arch)
	artifactName, ok := spec.platformAssets[platform]
	if !ok {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q rolling_user_local recipe %q does not support %s/%s", dep.Name, spec.recipe, opts.OS, opts.Arch)
	}
	if resolved, ok := opts.ResolvedUserLocal[dep.Name]; ok {
		if resolved.Recipe != spec.recipe || resolved.Version == "" || resolved.URL == "" || resolved.Checksum == "" || resolved.Artifact != artifactName || resolved.Platform != platform {
			return UserLocalArtifact{}, false, fmt.Errorf("dependency %q has incomplete resolved rolling artifact", dep.Name)
		}
		return resolved, true, nil
	}

	releaseURL := strings.TrimSpace(opts.RollingReleaseURL)
	if releaseURL == "" {
		releaseURL = spec.releasesURL
	}
	releases, err := fetchGitHubReleases(opts.HTTPClient, releaseURL)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q: %w", spec.recipe, err)
	}
	release, err := latestStableRelease(releases)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q: %w", spec.recipe, err)
	}
	asset, err := uniqueReleaseAsset(release, artifactName)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q: %w", spec.recipe, err)
	}
	digest, err := verifiedSHA256Digest(asset.Digest)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q asset %q: %w", spec.recipe, artifactName, err)
	}
	if err := validateRollingArtifactURL(asset.BrowserDownloadURL, releaseURL, spec.repository, release.TagName, artifactName); err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q asset %q: %w", spec.recipe, artifactName, err)
	}

	destination, installedPath := userLocalPaths(opts.Home, spec.recipe, release.TagName, userLocalLayoutBundle, spec.command)
	return UserLocalArtifact{
		Recipe: spec.recipe, Version: release.TagName, Artifact: artifactName, URL: asset.BrowserDownloadURL,
		Digest: "sha256:" + digest, Checksum: digest, Platform: platform,
		Layout: userLocalLayoutBundle, Command: spec.command, Destination: destination, InstalledPath: installedPath,
	}, true, nil
}

func rollingClaudeArtifact(dep manifest.Dependency, opts Options) (UserLocalArtifact, bool, error) {
	platform, ok := claudePlatform(opts.OS, opts.Arch)
	if !ok {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q rolling_user_local recipe %q does not support %s/%s", dep.Name, claudeRollingRecipe, opts.OS, opts.Arch)
	}
	if resolved, ok := opts.ResolvedUserLocal[dep.Name]; ok {
		if resolved.Recipe != claudeRollingRecipe || resolved.Version == "" || resolved.URL == "" || resolved.Checksum == "" || resolved.Artifact != "claude" || resolved.Platform != platform {
			return UserLocalArtifact{}, false, fmt.Errorf("dependency %q has incomplete resolved rolling artifact", dep.Name)
		}
		return resolved, true, nil
	}

	baseURL := strings.TrimRight(strings.TrimSpace(opts.RollingReleaseURL), "/")
	if baseURL == "" {
		baseURL = claudeReleaseBaseURL
	}
	versionBytes, err := fetchReleaseMetadata(opts.HTTPClient, baseURL+"/stable")
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q stable channel: %w", claudeRollingRecipe, err)
	}
	version := strings.TrimSpace(string(versionBytes))
	if !stableNumericVersion(version) {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q: official stable channel returned malformed version", claudeRollingRecipe)
	}
	manifestBytes, err := fetchReleaseMetadata(opts.HTTPClient, baseURL+"/"+version+"/manifest.json")
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q integrity metadata: %w", claudeRollingRecipe, err)
	}
	var release claudeReleaseManifest
	if err := json.Unmarshal(manifestBytes, &release); err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q integrity metadata: parse: %w", claudeRollingRecipe, err)
	}
	if release.Version != version {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q integrity metadata version does not match stable channel", claudeRollingRecipe)
	}
	platformArtifact, ok := release.Platforms[platform]
	if !ok || platformArtifact.Binary != "claude" {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q integrity metadata has no claude artifact for %s", claudeRollingRecipe, platform)
	}
	digest, err := verifiedSHA256Digest("sha256:" + platformArtifact.Checksum)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q artifact %q: %w", claudeRollingRecipe, platform, err)
	}
	artifactURL := baseURL + "/" + version + "/" + platform + "/claude"
	if err := validateClaudeArtifactURL(artifactURL, baseURL, version, platform); err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q artifact %q: %w", claudeRollingRecipe, platform, err)
	}
	destination, installedPath := userLocalPaths(opts.Home, claudeRollingRecipe, version, userLocalLayoutBundle, "claude")
	return UserLocalArtifact{
		Recipe: claudeRollingRecipe, Version: version, Artifact: "claude", URL: artifactURL,
		Digest: "sha256:" + digest, Checksum: digest, Platform: platform,
		Layout: userLocalLayoutBundle, Command: "claude", Destination: destination, InstalledPath: installedPath,
	}, true, nil
}

func claudePlatform(goos, goarch string) (string, bool) {
	platforms := map[string]string{
		"darwin_amd64": "darwin-x64",
		"darwin_arm64": "darwin-arm64",
		"linux_amd64":  "linux-x64",
		"linux_arm64":  "linux-arm64",
	}
	platform, ok := platforms[platformKey(goos, goarch)]
	return platform, ok
}

func stableNumericVersion(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, char := range part {
			if char < '0' || char > '9' {
				return false
			}
		}
	}
	return true
}

func fetchReleaseMetadata(client *http.Client, endpoint string) ([]byte, error) {
	origin, err := url.Parse(endpoint)
	if err != nil || origin.Host == "" || (origin.Scheme != "https" && origin.Scheme != "http") {
		return nil, errors.New("release metadata URL is malformed")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	boundedClient := *client
	priorRedirectCheck := client.CheckRedirect
	boundedClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != origin.Scheme || req.URL.Host != origin.Host {
			return fmt.Errorf("redirect outside release metadata origin: %s", req.URL.Host)
		}
		if priorRedirectCheck != nil {
			return priorRedirectCheck(req, via)
		}
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create release metadata request: %w", err)
	}
	resp, err := boundedClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download release metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download release metadata: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseMetadata+1))
	if err != nil {
		return nil, fmt.Errorf("read release metadata: %w", err)
	}
	if len(data) > maxReleaseMetadata {
		return nil, errors.New("release metadata exceeds size limit")
	}
	return data, nil
}

func validateClaudeArtifactURL(raw, base, version, platform string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("official artifact URL is malformed")
	}
	b, err := url.Parse(base)
	if err != nil || b.Host == "" {
		return errors.New("official metadata base URL is malformed")
	}
	if u.Scheme != b.Scheme || u.Host != b.Host {
		return errors.New("official artifact URL is outside the release metadata origin")
	}
	if b.Host == "downloads.claude.ai" && u.Scheme != "https" {
		return errors.New("official artifact URL must use https")
	}
	wantPath := path.Join("/", b.EscapedPath(), version, platform, "claude")
	if u.EscapedPath() != wantPath {
		return errors.New("official artifact URL is not immutable for the resolved release")
	}
	return nil
}

func fetchGitHubReleases(client *http.Client, endpoint string) ([]githubRelease, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create release metadata request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download release metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download release metadata: %s", resp.Status)
	}
	limited := io.LimitReader(resp.Body, maxReleaseMetadata+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read release metadata: %w", err)
	}
	if len(data) > maxReleaseMetadata {
		return nil, errors.New("release metadata exceeds size limit")
	}
	var releases []githubRelease
	if err := json.Unmarshal(data, &releases); err == nil {
		return releases, nil
	}
	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return nil, fmt.Errorf("parse release metadata: %w", err)
	}
	return []githubRelease{release}, nil
}

func latestStableRelease(releases []githubRelease) (githubRelease, error) {
	for _, release := range releases {
		if !release.Draft && !release.Prerelease && strings.TrimSpace(release.TagName) != "" {
			return release, nil
		}
	}
	return githubRelease{}, errors.New("official metadata contains no stable release")
}

func uniqueReleaseAsset(release githubRelease, name string) (githubReleaseAsset, error) {
	var matches []githubReleaseAsset
	for _, asset := range release.Assets {
		if asset.Name == name {
			matches = append(matches, asset)
		}
	}
	if len(matches) == 0 {
		return githubReleaseAsset{}, fmt.Errorf("stable release %q has no asset %q", release.TagName, name)
	}
	if len(matches) != 1 {
		return githubReleaseAsset{}, fmt.Errorf("stable release %q has %d assets named %q", release.TagName, len(matches), name)
	}
	return matches[0], nil
}

func verifiedSHA256Digest(digest string) (string, error) {
	algorithm, value, ok := strings.Cut(strings.TrimSpace(digest), ":")
	if !ok || !strings.EqualFold(algorithm, "sha256") {
		return "", errors.New("official asset digest must use sha256")
	}
	value = strings.ToLower(strings.TrimSpace(value))
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("official asset digest is malformed")
	}
	return value, nil
}

func validateRollingArtifactURL(raw, metadataURL, repository, version, artifact string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("official asset URL is malformed")
	}
	metadata, err := url.Parse(metadataURL)
	if err != nil {
		return errors.New("release metadata URL is malformed")
	}
	if metadata.Host == "api.github.com" {
		if u.Scheme != "https" || u.Host != "github.com" {
			return errors.New("official asset URL is outside github.com")
		}
		wantPath := path.Join("/", repository, "releases/download", version, artifact)
		if u.EscapedPath() != wantPath {
			return errors.New("official asset URL is not immutable for the resolved release")
		}
		return nil
	}
	if u.Scheme != metadata.Scheme || u.Host != metadata.Host {
		return errors.New("fixture asset URL is outside the release metadata origin")
	}
	return nil
}
