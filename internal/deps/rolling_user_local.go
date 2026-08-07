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
	codexRollingRecipe  = "codex"
	codexReleasesAPIURL = "https://api.github.com/repos/openai/codex/releases/latest"
	maxReleaseMetadata  = 4 << 20
)

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

func rollingUserLocalArtifact(dep manifest.Dependency, opts Options) (UserLocalArtifact, bool, error) {
	if dep.RollingUserLocal == nil {
		return UserLocalArtifact{}, false, nil
	}
	if dep.UserLocal != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q cannot declare both user_local and rolling_user_local", dep.Name)
	}
	recipe := strings.TrimSpace(dep.RollingUserLocal.Recipe)
	if recipe != codexRollingRecipe {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q declares unsupported rolling_user_local recipe %q", dep.Name, recipe)
	}
	artifactName, platform, ok := codexPlatformArtifact(opts.OS, opts.Arch)
	if !ok {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q rolling_user_local recipe %q does not support %s/%s", dep.Name, recipe, opts.OS, opts.Arch)
	}
	if resolved, ok := opts.ResolvedUserLocal[dep.Name]; ok {
		if resolved.Recipe != recipe || resolved.Version == "" || resolved.URL == "" || resolved.Checksum == "" || resolved.Artifact != artifactName || resolved.Platform != platform {
			return UserLocalArtifact{}, false, fmt.Errorf("dependency %q has incomplete resolved rolling artifact", dep.Name)
		}
		return resolved, true, nil
	}

	releaseURL := strings.TrimSpace(opts.RollingReleaseURL)
	if releaseURL == "" {
		releaseURL = codexReleasesAPIURL
	}
	releases, err := fetchGitHubReleases(opts.HTTPClient, releaseURL)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q: %w", recipe, err)
	}
	release, err := latestStableRelease(releases)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q: %w", recipe, err)
	}
	asset, err := uniqueReleaseAsset(release, artifactName)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q: %w", recipe, err)
	}
	digest, err := verifiedSHA256Digest(asset.Digest)
	if err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q asset %q: %w", recipe, artifactName, err)
	}
	if err := validateRollingArtifactURL(asset.BrowserDownloadURL, releaseURL, release.TagName, artifactName); err != nil {
		return UserLocalArtifact{}, false, fmt.Errorf("resolve rolling user-local recipe %q asset %q: %w", recipe, artifactName, err)
	}

	destination, installedPath := userLocalPaths(opts.Home, recipe, release.TagName, userLocalLayoutBundle, "codex")
	return UserLocalArtifact{
		Recipe:        recipe,
		Version:       release.TagName,
		Artifact:      artifactName,
		URL:           asset.BrowserDownloadURL,
		Digest:        "sha256:" + digest,
		Checksum:      digest,
		Platform:      platform,
		Layout:        userLocalLayoutBundle,
		Command:       "codex",
		Destination:   destination,
		InstalledPath: installedPath,
	}, true, nil
}

func codexPlatformArtifact(goos, goarch string) (artifact, platform string, ok bool) {
	targets := map[string]string{
		"darwin_amd64": "x86_64-apple-darwin",
		"darwin_arm64": "aarch64-apple-darwin",
		"linux_amd64":  "x86_64-unknown-linux-musl",
		"linux_arm64":  "aarch64-unknown-linux-musl",
	}
	platform = platformKey(goos, goarch)
	target, ok := targets[platform]
	if !ok {
		return "", platform, false
	}
	return "codex-package-" + target + ".tar.gz", platform, true
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

func validateRollingArtifactURL(raw, metadataURL, version, artifact string) error {
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
		wantPath := path.Join("/openai/codex/releases/download", version, artifact)
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
