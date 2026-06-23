package upgrade

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

const (
	ChannelHomebrew    = "homebrew"
	ChannelRelease     = "release-artifact"
	ChannelDevelopment = "development"

	ActionHomebrewUpgrade = "homebrew-upgrade"
	ActionReplaceBinary   = "replace-binary"
	ActionAlreadyCurrent  = "already-current"
	ActionManualRebuild   = "manual-rebuild"
)

const defaultReleaseBaseURL = "https://github.com/yersonargotev/dots/releases/download"

type Runner interface {
	Run(ctx context.Context, executable string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, executable string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, executable, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return stdout.String(), fmt.Errorf("%s", msg)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

type Plan struct {
	Channel        string `json:"channel"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version,omitempty"`
	Action         string `json:"action"`
	Executable     string `json:"executable"`
	Artifact       string `json:"artifact,omitempty"`
	Checksum       string `json:"checksum,omitempty"`
}

type Options struct {
	CurrentVersion string
	Executable     string
	GOOS           string
	GOARCH         string
	ReleaseBaseURL string
	HTTPClient     *http.Client
	Runner         Runner
}

func (o Options) withDefaults() Options {
	if o.GOOS == "" {
		o.GOOS = runtime.GOOS
	}
	if o.GOARCH == "" {
		o.GOARCH = runtime.GOARCH
	}
	if o.ReleaseBaseURL == "" {
		o.ReleaseBaseURL = os.Getenv("DOTS_RELEASE_BASE_URL")
	}
	if o.ReleaseBaseURL == "" {
		o.ReleaseBaseURL = defaultReleaseBaseURL
	}
	if o.HTTPClient == nil {
		o.HTTPClient = http.DefaultClient
	}
	if o.Runner == nil {
		o.Runner = ExecRunner{}
	}
	return o
}

func Preview(ctx context.Context, opts Options) (Plan, error) {
	opts = opts.withDefaults()
	channel := detectChannel(ctx, opts)
	plan := Plan{Channel: channel, CurrentVersion: opts.CurrentVersion, Executable: opts.Executable}
	switch channel {
	case ChannelDevelopment:
		plan.Action = ActionManualRebuild
		return plan, nil
	case ChannelHomebrew:
		latest, err := homebrewLatestVersion(ctx, opts.Runner)
		if err != nil {
			return Plan{}, err
		}
		plan.Action = ActionHomebrewUpgrade
		plan.LatestVersion = latest
		return plan, nil
	default:
		asset, err := resolveLatestAsset(ctx, opts)
		if err != nil {
			return Plan{}, err
		}
		plan.LatestVersion = asset.Version
		plan.Artifact = asset.Name
		plan.Checksum = asset.Checksum
		if opts.CurrentVersion == asset.Version {
			plan.Action = ActionAlreadyCurrent
		} else {
			plan.Action = ActionReplaceBinary
		}
		return plan, nil
	}
}

func Execute(ctx context.Context, opts Options) (Plan, error) {
	opts = opts.withDefaults()
	plan, err := Preview(ctx, opts)
	if err != nil {
		return Plan{}, err
	}
	switch plan.Action {
	case ActionManualRebuild:
		return plan, fmt.Errorf("development/local dots build detected; rebuild manually from the Source of Truth before running dots upgrade")
	case ActionHomebrewUpgrade:
		if _, err := opts.Runner.Run(ctx, "brew", "update"); err != nil {
			return plan, fmt.Errorf("brew update: %w", err)
		}
		if _, err := opts.Runner.Run(ctx, "brew", "upgrade", "yersonargotev/tap/dots"); err != nil {
			return plan, fmt.Errorf("brew upgrade yersonargotev/tap/dots: %w", err)
		}
		return plan, nil
	case ActionAlreadyCurrent:
		return plan, nil
	case ActionReplaceBinary:
		if err := replaceReleaseArtifact(ctx, opts, plan); err != nil {
			return plan, err
		}
		return plan, nil
	default:
		return plan, fmt.Errorf("unknown upgrade action %q", plan.Action)
	}
}

func detectChannel(ctx context.Context, opts Options) string {
	if opts.CurrentVersion == "" || opts.CurrentVersion == "dev" {
		return ChannelDevelopment
	}
	if opts.Runner != nil {
		if _, err := opts.Runner.Run(ctx, "brew", "list", "--formula", "yersonargotev/tap/dots"); err == nil {
			return ChannelHomebrew
		}
	}
	return ChannelRelease
}

type homebrewInfo struct {
	Formulae []struct {
		Versions struct {
			Stable string `json:"stable"`
		} `json:"versions"`
	} `json:"formulae"`
}

func homebrewLatestVersion(ctx context.Context, runner Runner) (string, error) {
	out, err := runner.Run(ctx, "brew", "info", "--json=v2", "yersonargotev/tap/dots")
	if err != nil {
		return "", fmt.Errorf("brew info yersonargotev/tap/dots: %w", err)
	}
	var info homebrewInfo
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return "", fmt.Errorf("parse brew info yersonargotev/tap/dots: %w", err)
	}
	if len(info.Formulae) == 0 || info.Formulae[0].Versions.Stable == "" {
		return "", fmt.Errorf("brew info yersonargotev/tap/dots did not report a stable version")
	}
	stable := info.Formulae[0].Versions.Stable
	if strings.HasPrefix(stable, "v") {
		return stable, nil
	}
	return "v" + stable, nil
}

type asset struct {
	Name     string
	Version  string
	Checksum string
}

func resolveLatestAsset(ctx context.Context, opts Options) (asset, error) {
	assetBase := assetBaseURL(opts.ReleaseBaseURL)
	checksums, err := downloadText(ctx, opts.HTTPClient, assetBase+"/checksums.txt")
	if err != nil {
		return asset{}, err
	}
	return parseAsset(checksums, opts.GOOS, opts.GOARCH)
}

var artifactRE = regexp.MustCompile(`^dots_(v[^_]+)_([a-z0-9]+)_([a-z0-9]+)$`)

func parseAsset(checksums, goos, goarch string) (asset, error) {
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		m := artifactRE.FindStringSubmatch(fields[1])
		if len(m) != 4 {
			continue
		}
		if m[2] == goos && m[3] == goarch {
			return asset{Name: fields[1], Version: m[1], Checksum: fields[0]}, nil
		}
	}
	return asset{}, fmt.Errorf("checksums.txt does not contain a Release Artifact for %s/%s", goos, goarch)
}

func replaceReleaseArtifact(ctx context.Context, opts Options, plan Plan) error {
	if opts.Executable == "" {
		return fmt.Errorf("cannot replace dots binary: executable path is unknown")
	}
	tmpPath, err := downloadToTemp(ctx, opts.HTTPClient, assetBaseURL(opts.ReleaseBaseURL)+"/"+plan.Artifact)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)
	actual, err := sha256File(tmpPath)
	if err != nil {
		return err
	}
	if actual != plan.Checksum {
		return fmt.Errorf("checksum mismatch for %s", plan.Artifact)
	}
	newPath := opts.Executable + ".new"
	oldPath := opts.Executable + ".old"
	if err := copyFile(tmpPath, newPath, 0o755); err != nil {
		return err
	}
	_ = os.Remove(oldPath)
	if err := os.Rename(opts.Executable, oldPath); err != nil {
		_ = os.Remove(newPath)
		return fmt.Errorf("preserve previous binary as %s: %w", oldPath, err)
	}
	if err := os.Rename(newPath, opts.Executable); err != nil {
		_ = os.Rename(oldPath, opts.Executable)
		return fmt.Errorf("activate new binary: %w", err)
	}
	return nil
}

func assetBaseURL(releaseBaseURL string) string {
	base := strings.TrimRight(releaseBaseURL, "/")
	if base == defaultReleaseBaseURL {
		return "https://github.com/yersonargotev/dots/releases/latest/download"
	}
	return base + "/latest"
}

func downloadToTemp(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "dots-upgrade-*")
	if err != nil {
		return "", fmt.Errorf("create temporary release artifact: %w", err)
	}
	tmpPath := tmp.Name()
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("write temporary release artifact: %w", err)
	}
	return tmpPath, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("write %s: %w", target, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", target, err)
	}
	if err := os.Chmod(target, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", target, err)
	}
	return nil
}

func downloadText(ctx context.Context, client *http.Client, url string) (string, error) {
	b, err := downloadBytes(ctx, client, url)
	return string(b), err
}

func downloadBytes(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
