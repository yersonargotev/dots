package deps

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yersonargotev/dots/internal/manifest"
)

// UserLocalArtifact is the resolved, reviewed user-local install recipe for a
// Dependency on the current platform.
type UserLocalArtifact struct {
	Recipe        string `json:"recipe"`
	Version       string `json:"version"`
	Artifact      string `json:"artifact,omitempty"`
	URL           string `json:"url"`
	Digest        string `json:"digest,omitempty"`
	Checksum      string `json:"checksum"`
	Platform      string `json:"platform,omitempty"`
	Layout        string `json:"layout"`
	Command       string `json:"command"`
	Destination   string `json:"destination,omitempty"`
	InstalledPath string `json:"installed_path,omitempty"`
}

const (
	userLocalLayoutSingle = "single-binary"
	userLocalLayoutBundle = "bundle"
	maxArtifactBytes      = 512 << 20
	maxExtractedBytes     = 1 << 30
	maxArchiveFiles       = 10_000
)

type userLocalRecipe struct {
	archiveName func(version, goarch string) (string, bool)
	url         func(version, archive string) string
	layout      string
	command     string
	archiveType userLocalArchiveType
	binaryPath  func(archive, command string) string
	links       []string
}

type userLocalArchiveType string

const (
	userLocalArchiveTarGzip     userLocalArchiveType = "tar.gz"
	userLocalArchiveZip         userLocalArchiveType = "zip"
	userLocalArchiveRaw         userLocalArchiveType = "raw"
	userLocalArchiveByExtension userLocalArchiveType = "by-extension"
)

var userLocalRecipes = map[string]userLocalRecipe{
	"codex": {
		archiveName: func(version, goarch string) (string, bool) { return "", false },
		url:         func(version, archive string) string { return "" },
		layout:      userLocalLayoutBundle,
		command:     "codex",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(_ string, command string) string { return "bin/" + command },
		links:       []string{"codex"},
	},
	"claude": {
		archiveName: func(version, goarch string) (string, bool) { return "", false },
		url:         func(version, archive string) string { return "" },
		layout:      userLocalLayoutBundle,
		command:     "claude",
		archiveType: userLocalArchiveRaw,
		binaryPath:  func(_ string, command string) string { return command },
		links:       []string{"claude"},
	},
	"opencode": {
		archiveName: func(version, goarch string) (string, bool) { return "", false },
		url:         func(version, archive string) string { return "" },
		layout:      userLocalLayoutBundle,
		command:     "opencode",
		archiveType: userLocalArchiveByExtension,
		binaryPath:  func(_ string, command string) string { return command },
		links:       []string{"opencode"},
	},
	"antigravity": {
		archiveName: func(version, goarch string) (string, bool) { return "", false },
		url:         func(version, archive string) string { return "" },
		layout:      userLocalLayoutBundle,
		command:     "agy",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(_ string, _ string) string { return "antigravity" },
		links:       []string{"agy"},
	},
	"copilot": {
		archiveName: func(version, goarch string) (string, bool) { return "", false },
		url:         func(version, archive string) string { return "" },
		layout:      userLocalLayoutBundle,
		command:     "copilot",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(_ string, command string) string { return command },
		links:       []string{"copilot"},
	},
	"uv": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64":
				return "uv-x86_64-unknown-linux-gnu.tar.gz", true
			case "arm64":
				return "uv-aarch64-unknown-linux-gnu.tar.gz", true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/astral-sh/uv/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutBundle,
		command:     "uv",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(archive, command string) string { return strings.TrimSuffix(archive, ".tar.gz") + "/" + command },
		links:       []string{"uv", "uvx"},
	},
	"pnpm": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64":
				return fmt.Sprintf("linux-x64-%s.tgz", version), true
			case "arm64":
				return fmt.Sprintf("linux-arm64-%s.tgz", version), true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			packageName := strings.TrimSuffix(archive, "-"+version+".tgz")
			return fmt.Sprintf("https://registry.npmjs.org/@pnpm/%s/-/%s", packageName, archive)
		},
		layout:      userLocalLayoutSingle,
		command:     "pnpm",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(_ string, command string) string { return "package/" + command },
		links:       []string{"pnpm"},
	},
	"nvim": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64":
				return "nvim-linux-x86_64.tar.gz", true
			case "arm64":
				return "nvim-linux-arm64.tar.gz", true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/neovim/neovim/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutBundle,
		command:     "nvim",
		archiveType: userLocalArchiveTarGzip,
		binaryPath: func(archive, command string) string {
			return strings.TrimSuffix(archive, ".tar.gz") + "/bin/" + command
		},
		links: []string{"nvim"},
	},
	"gentle-ai": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64", "arm64":
				return fmt.Sprintf("gentle-ai_%s_linux_%s.tar.gz", strings.TrimPrefix(version, "v"), goarch), true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/Gentleman-Programming/gentle-ai/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutSingle,
		command:     "gentle-ai",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(_ string, command string) string { return command },
		links:       []string{"gentle-ai"},
	},
	"engram": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64", "arm64":
				return fmt.Sprintf("engram_%s_linux_%s.tar.gz", strings.TrimPrefix(version, "v"), goarch), true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/Gentleman-Programming/engram/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutSingle,
		command:     "engram",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(_ string, command string) string { return command },
		links:       []string{"engram"},
	},
	"bun": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64":
				return "bun-linux-x64.zip", true
			case "arm64":
				return "bun-linux-aarch64.zip", true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/oven-sh/bun/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutBundle,
		command:     "bun",
		archiveType: userLocalArchiveZip,
		binaryPath:  func(archive, command string) string { return strings.TrimSuffix(archive, ".zip") + "/" + command },
		links:       []string{"bun"},
	},
	"bat": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64":
				return fmt.Sprintf("bat-%s-x86_64-unknown-linux-gnu.tar.gz", version), true
			case "arm64":
				return fmt.Sprintf("bat-%s-aarch64-unknown-linux-gnu.tar.gz", version), true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/sharkdp/bat/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutSingle,
		command:     "bat",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(archive, command string) string { return strings.TrimSuffix(archive, ".tar.gz") + "/" + command },
		links:       []string{"bat"},
	},
	"atuin": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64":
				return "atuin-x86_64-unknown-linux-gnu.tar.gz", true
			case "arm64":
				return "atuin-aarch64-unknown-linux-gnu.tar.gz", true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/atuinsh/atuin/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutBundle,
		command:     "atuin",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(archive, command string) string { return strings.TrimSuffix(archive, ".tar.gz") + "/" + command },
		links:       []string{"atuin"},
	},
	"starship": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64":
				return "starship-x86_64-unknown-linux-gnu.tar.gz", true
			case "arm64":
				return "starship-aarch64-unknown-linux-musl.tar.gz", true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/starship/starship/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutSingle,
		command:     "starship",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(_ string, command string) string { return command },
		links:       []string{"starship"},
	},
	"zellij": {
		archiveName: func(version, goarch string) (string, bool) {
			switch goarch {
			case "amd64":
				return "zellij-x86_64-unknown-linux-musl.tar.gz", true
			case "arm64":
				return "zellij-aarch64-unknown-linux-musl.tar.gz", true
			default:
				return "", false
			}
		},
		url: func(version, archive string) string {
			return fmt.Sprintf("https://github.com/zellij-org/zellij/releases/download/%s/%s", version, archive)
		},
		layout:      userLocalLayoutSingle,
		command:     "zellij",
		archiveType: userLocalArchiveTarGzip,
		binaryPath:  func(_ string, command string) string { return command },
		links:       []string{"zellij"},
	},
}

func userLocalArtifact(dep manifest.Dependency, opts Options) (UserLocalArtifact, bool, error) {
	if dep.RollingUserLocal != nil {
		return rollingUserLocalArtifact(dep, opts)
	}
	if opts.OS != "linux" || dep.UserLocal == nil {
		return UserLocalArtifact{}, false, nil
	}
	recipeName := strings.TrimSpace(dep.UserLocal.Recipe)
	if recipeName == "" {
		recipeName = strings.TrimSpace(dep.Name)
	}
	recipe, ok := userLocalRecipes[recipeName]
	if !ok {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q declares unsupported user_local recipe %q", dep.Name, recipeName)
	}
	version := strings.TrimSpace(dep.UserLocal.Version)
	if version == "" {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q user_local.version is required", dep.Name)
	}
	arch := opts.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	archive, ok := recipe.archiveName(version, arch)
	if !ok {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q user_local recipe %q does not support %s/%s", dep.Name, recipeName, opts.OS, arch)
	}
	checksum := dep.UserLocal.Checksums[platformKey(opts.OS, arch)]
	if checksum == "" {
		checksum = strings.TrimSpace(dep.UserLocal.Checksum)
	}
	if checksum == "" {
		return UserLocalArtifact{}, false, fmt.Errorf("dependency %q user_local checksum is required for %s", dep.Name, platformKey(opts.OS, arch))
	}
	destination, installedPath := userLocalPaths(opts.Home, recipeName, version, recipe.layout, recipe.command)
	return UserLocalArtifact{Recipe: recipeName, Version: version, Artifact: archive, URL: recipe.url(version, archive), Checksum: checksum, Platform: platformKey(opts.OS, arch), Layout: recipe.layout, Command: recipe.command, Destination: destination, InstalledPath: installedPath}, true, nil
}

func platformKey(goos, goarch string) string { return goos + "_" + goarch }

func (a UserLocalArtifact) Hint() string {
	if a.Recipe == "" {
		return "user-local install"
	}
	target := a.Destination
	installedPath := a.InstalledPath
	if target == "" {
		target, installedPath = userLocalPaths("", a.Recipe, a.Version, a.Layout, a.Command)
	}
	if a.Digest != "" {
		return fmt.Sprintf("user-local install %s %s artifact %s for %s (%s) to %s with link at %s", a.Recipe, a.Version, a.Artifact, a.Platform, a.Digest, target, installedPath)
	}
	if a.Layout == userLocalLayoutBundle {
		return fmt.Sprintf("user-local install %s %s to %s with link at %s", a.Recipe, a.Version, target, installedPath)
	}
	return fmt.Sprintf("user-local install %s %s to %s", a.Recipe, a.Version, target)
}

func userLocalPaths(home, recipe, version, layout, command string) (destination, installedPath string) {
	if strings.TrimSpace(home) == "" {
		installedPath = filepath.Join("~", ".local", "bin", command)
		if layout == userLocalLayoutBundle {
			return filepath.Join("~", ".local", "opt", recipe, version), installedPath
		}
		return installedPath, installedPath
	}
	installedPath = filepath.Join(home, ".local", "bin", command)
	if layout == userLocalLayoutBundle {
		return filepath.Join(home, ".local", "opt", recipe, version), installedPath
	}
	return installedPath, installedPath
}

func (a UserLocalArtifact) archiveName() string {
	_, name := filepath.Split(a.URL)
	return name
}

func InstallUserLocal(home string, action InstallAction) error {
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
	}
	artifact := action.UserLocal
	if artifact == nil {
		return errors.New("missing user-local artifact")
	}
	recipe, ok := userLocalRecipes[artifact.Recipe]
	if !ok {
		return fmt.Errorf("unknown user-local recipe %q", artifact.Recipe)
	}

	data, err := downloadUserLocalArtifact(artifact.URL, artifact.Checksum)
	if err != nil {
		return err
	}
	archive := artifact.archiveName()
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("create user-local bin directory: %w", err)
	}

	installedPath := filepath.Join(binDir, artifact.Command)
	if artifact.Layout == userLocalLayoutBundle {
		optDir := filepath.Join(home, ".local", "opt", artifact.Recipe, artifact.Version)
		parent := filepath.Dir(optDir)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create user-local opt parent: %w", err)
		}
		staging, err := os.MkdirTemp(parent, ".dots-"+artifact.Recipe+"-*")
		if err != nil {
			return fmt.Errorf("create user-local staging directory: %w", err)
		}
		defer os.RemoveAll(staging)
		if err := stageUserLocalArtifact(data, archive, recipe, staging); err != nil {
			return err
		}
		for _, link := range recipe.links {
			if err := ensureExecutable(filepath.Join(staging, recipe.binaryPath(archive, link))); err != nil {
				return err
			}
		}
		if err := replaceUserLocalBundle(staging, optDir); err != nil {
			return err
		}
		for _, link := range recipe.links {
			target := filepath.Join(optDir, recipe.binaryPath(archive, link))
			if err := replaceSymlink(target, filepath.Join(binDir, link)); err != nil {
				return err
			}
		}
	} else {
		tmpDir, err := os.MkdirTemp("", "dots-user-local-*")
		if err != nil {
			return fmt.Errorf("create extraction directory: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		if err := extractArchive(data, recipe.archiveType, tmpDir); err != nil {
			return err
		}
		source := filepath.Join(tmpDir, recipe.binaryPath(archive, artifact.Command))
		if err := ensureExecutable(source); err != nil {
			return err
		}
		if err := copyFile(source, installedPath, 0o755); err != nil {
			return err
		}
	}

	return nil
}

func replaceUserLocalBundle(staging, destination string) error {
	if _, err := os.Lstat(destination); os.IsNotExist(err) {
		if err := os.Rename(staging, destination); err != nil {
			return fmt.Errorf("promote user-local bundle: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect user-local bundle: %w", err)
	}

	backup, err := os.MkdirTemp(filepath.Dir(destination), ".dots-previous-"+filepath.Base(destination)+"-*")
	if err != nil {
		return fmt.Errorf("reserve previous user-local bundle path: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("prepare previous user-local bundle path: %w", err)
	}
	if err := os.Rename(destination, backup); err != nil {
		return fmt.Errorf("preserve previous user-local bundle: %w", err)
	}
	if err := os.Rename(staging, destination); err != nil {
		if rollbackErr := os.Rename(backup, destination); rollbackErr != nil {
			return fmt.Errorf("promote user-local bundle: %v (restore previous bundle: %w)", err, rollbackErr)
		}
		return fmt.Errorf("promote user-local bundle: %w", err)
	}
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("remove previous user-local bundle: %w", err)
	}
	return nil
}

func stageUserLocalArtifact(data []byte, archive string, recipe userLocalRecipe, dest string) error {
	if recipe.archiveType != userLocalArchiveRaw {
		archiveType := recipe.archiveType
		if archiveType == userLocalArchiveByExtension {
			switch {
			case strings.HasSuffix(archive, ".tar.gz"):
				archiveType = userLocalArchiveTarGzip
			case strings.HasSuffix(archive, ".zip"):
				archiveType = userLocalArchiveZip
			default:
				return fmt.Errorf("unsupported user-local archive %q", archive)
			}
		}
		return extractArchive(data, archiveType, dest)
	}
	path := filepath.Join(dest, recipe.binaryPath(archive, recipe.command))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create raw artifact directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o755); err != nil {
		return fmt.Errorf("write raw executable artifact: %w", err)
	}
	return nil
}

var downloadUserLocalArtifact = downloadAndVerify

func downloadAndVerify(url, checksum string) ([]byte, error) {
	initial, err := parseArtifactURL(url)
	if err != nil {
		return nil, fmt.Errorf("download user-local artifact: %w", err)
	}
	client := http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			if redirectAllowed(initial, req.URL) {
				return nil
			}
			return fmt.Errorf("redirect outside allowed artifact hosts: %s", req.URL.Host)
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download user-local artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download user-local artifact: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxArtifactBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read user-local artifact: %w", err)
	}
	if len(data) > maxArtifactBytes {
		return nil, errors.New("read user-local artifact: artifact exceeds size limit")
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	fields := strings.Fields(checksum)
	if len(fields) == 0 {
		return nil, errors.New("verify user-local artifact: missing sha256 checksum")
	}
	want := strings.ToLower(strings.TrimSpace(fields[0]))
	if got != want {
		return nil, fmt.Errorf("verify user-local artifact: sha256 %s, want %s", got, want)
	}
	return data, nil
}

func parseArtifactURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("artifact URL is malformed")
	}
	return u, nil
}

func redirectAllowed(initial, redirect *url.URL) bool {
	if redirect.Scheme != initial.Scheme {
		return false
	}
	if redirect.Host == initial.Host {
		return true
	}
	if initial.Scheme != "https" || initial.Host != "github.com" || redirect.Scheme != "https" {
		return false
	}
	switch redirect.Host {
	case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	default:
		return false
	}
}

func extractArchive(data []byte, typ userLocalArchiveType, dest string) error {
	files := 0
	var extracted int64
	switch typ {
	case userLocalArchiveTarGzip:
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("open tar.gz artifact: %w", err)
		}
		defer gz.Close()
		tr := tar.NewReader(gz)
		for {
			h, err := tr.Next()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("read tar.gz artifact: %w", err)
			}
			if h.Typeflag != tar.TypeReg {
				continue
			}
			files++
			extracted += h.Size
			if files > maxArchiveFiles || h.Size < 0 || extracted > maxExtractedBytes {
				return errors.New("extract tar.gz artifact: archive exceeds extraction limits")
			}
			path, err := safeJoin(dest, h.Name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create extraction directory: %w", err)
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o777)
			if err != nil {
				return fmt.Errorf("create extracted file: %w", err)
			}
			_, copyErr := io.Copy(out, tr)
			closeErr := out.Close()
			if copyErr != nil {
				return fmt.Errorf("extract file: %w", copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close extracted file: %w", closeErr)
			}
		}
	case userLocalArchiveZip:
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return fmt.Errorf("open zip artifact: %w", err)
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			files++
			extracted += int64(f.UncompressedSize64)
			if files > maxArchiveFiles || extracted > maxExtractedBytes {
				return errors.New("extract zip artifact: archive exceeds extraction limits")
			}
			path, err := safeJoin(dest, f.Name)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create extraction directory: %w", err)
			}
			r, err := f.Open()
			if err != nil {
				return fmt.Errorf("open zipped file: %w", err)
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode()&0o777)
			if err != nil {
				r.Close()
				return fmt.Errorf("create extracted file: %w", err)
			}
			_, copyErr := io.Copy(out, r)
			closeInErr := r.Close()
			closeOutErr := out.Close()
			if copyErr != nil {
				return fmt.Errorf("extract file: %w", copyErr)
			}
			if closeInErr != nil {
				return fmt.Errorf("close zipped file: %w", closeInErr)
			}
			if closeOutErr != nil {
				return fmt.Errorf("close extracted file: %w", closeOutErr)
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported user-local archive type %q", typ)
	}
}

func safeJoin(root, name string) (string, error) {
	if name == "" || !filepath.IsLocal(name) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	clean := filepath.Clean(name)
	path := filepath.Join(root, clean)
	if !strings.HasPrefix(path, filepath.Clean(root)+string(filepath.Separator)) && path != filepath.Clean(root) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
	return path, nil
}

func ensureExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat extracted executable %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("extracted executable %s is a directory", path)
	}
	return os.Chmod(path, info.Mode()|0o755)
}

func replaceSymlink(target, link string) error {
	tmp, err := os.CreateTemp(filepath.Dir(link), ".dots-link-*")
	if err != nil {
		return fmt.Errorf("create temporary user-local link: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary user-local link: %w", err)
	}
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("prepare temporary user-local link: %w", err)
	}
	defer os.Remove(tmpPath)
	if err := os.Symlink(target, tmpPath); err != nil {
		return fmt.Errorf("create user-local symlink: %w", err)
	}
	if err := os.Rename(tmpPath, link); err != nil {
		return fmt.Errorf("replace user-local symlink: %w", err)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open extracted executable: %w", err)
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dst), ".dots-executable-*")
	if err != nil {
		return fmt.Errorf("create temporary user-local executable: %w", err)
	}
	tmpPath := out.Name()
	defer os.Remove(tmpPath)
	if err := out.Chmod(mode); err != nil {
		out.Close()
		return fmt.Errorf("set temporary user-local executable mode: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy user-local executable: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close user-local executable: %w", closeErr)
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("replace user-local executable: %w", err)
	}
	return nil
}
