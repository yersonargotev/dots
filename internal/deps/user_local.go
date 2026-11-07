package deps

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
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
	"golang.org/x/sys/unix"
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
	"pnpm-standalone": {
		archiveName: func(version, goarch string) (string, bool) { return "", false },
		url:         func(version, archive string) string { return "" },
		layout:      userLocalLayoutBundle, command: "pnpm", archiveType: userLocalArchiveTarGzip,
		binaryPath: func(_ string, command string) string { return command },
		links:      []string{"pnpm"},
	},
	"gh": {
		archiveName: func(version, goarch string) (string, bool) { return "", false },
		url:         func(version, archive string) string { return "" },
		layout:      userLocalLayoutBundle, command: "gh", archiveType: userLocalArchiveTarGzip,
		binaryPath: func(archive, command string) string {
			return strings.TrimSuffix(archive, ".tar.gz") + "/bin/" + command
		},
		links: []string{"gh"},
	},
	// Receipt validation only: retired custom artifacts can never be installed or recovered.
	"herdr-custom": {layout: userLocalLayoutBundle, command: "herdr-custom", archiveType: userLocalArchiveTarGzip, binaryPath: func(archive, _ string) string { return strings.TrimSuffix(archive, ".tar.gz") + "/dots/herdr-custom" }, links: []string{"herdr-custom"}},
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
	if err := dep.RetirementError(); err != nil {
		return UserLocalArtifact{}, false, err
	}
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

// InstallUserLocal preserves the direct recipe entrypoint while using the same
// pending/final lifecycle as CLI orchestration.
func InstallUserLocal(home string, action InstallAction) error {
	if err := retiredActionError(action); err != nil {
		return err
	}
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		home = resolved
	}
	if action.UserLocal == nil {
		return errors.New("missing user-local artifact")
	}
	a := *action.UserLocal
	action.UserLocal = &a
	if a.Platform == "" {
		a.Platform = platformKey(runtime.GOOS, runtime.GOARCH)
	}
	a.Destination, a.InstalledPath = userLocalPaths(home, a.Recipe, a.Version, a.Layout, a.Command)
	if action.Dependency == "" {
		action.Dependency = a.Recipe
	}
	if action.Probe == "" && len(action.Probes) == 0 {
		action.Probe = a.Command
		action.Probes = []string{a.Command}
	}
	action.Provider = TierUserLocal
	action.Status = InstallActionStatusInstallable
	if action.Requirement == "" {
		action.Requirement = manifest.DependencyRequirementRequired
	}
	installer := UserLocalInstaller{Home: home, StateRoot: filepath.Join(home, ".local", "state", "dots"), Download: downloadUserLocalArtifact}
	return installer.RunUserLocal(action, func() bool {
		recipe, ok := userLocalRecipes[a.Recipe]
		if !ok {
			return false
		}
		for _, command := range recipe.links {
			info, err := os.Stat(filepath.Join(home, ".local", "bin", command))
			if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
				return false
			}
		}
		return true
	})
}

func stageUserLocalArtifactAt(data []byte, archive string, recipe userLocalRecipe, root *receiptRoot, check func() error) error {
	commands := map[string]bool{}
	for _, command := range recipe.links {
		commands[recipe.binaryPath(archive, command)] = true
	}
	write := func(name string, mode os.FileMode, reader io.Reader) error {
		if commands[filepath.Clean(name)] {
			mode |= 0755
		}
		if check != nil {
			if err := check(); err != nil {
				return err
			}
			reader = &receiptContextReader{reader: reader, check: check}
		}
		return writeExtractedFile(root, name, mode, reader, commands[filepath.Clean(name)])
	}
	if recipe.archiveType == userLocalArchiveRaw {
		return write(recipe.binaryPath(archive, recipe.command), 0755, bytes.NewReader(data))
	}
	typ := recipe.archiveType
	if typ == userLocalArchiveByExtension {
		switch {
		case strings.HasSuffix(archive, ".tar.gz"):
			typ = userLocalArchiveTarGzip
		case strings.HasSuffix(archive, ".zip"):
			typ = userLocalArchiveZip
		default:
			return fmt.Errorf("unsupported user-local archive %q", archive)
		}
	}
	return extractArchiveWith(data, typ, write, check)
}

var downloadUserLocalArtifact = downloadAndVerify

func downloadAndVerify(url, checksum string) ([]byte, error) {
	return downloadAndVerifyContext(context.Background(), url, checksum)
}
func downloadAndVerifyContext(ctx context.Context, url, checksum string) (data []byte, resultErr error) {
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
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download user-local artifact: %w", err)
	}
	defer func() { resultErr = errors.Join(resultErr, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download user-local artifact: %s", resp.Status)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxArtifactBytes+1))
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

func extractArchive(data []byte, typ userLocalArchiveType, dest string) (resultErr error) {
	root, err := openReceiptRoot(dest, true)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, root.file.Close()) }()
	return extractArchiveWith(data, typ, func(name string, mode os.FileMode, reader io.Reader) error {
		return writeExtractedFile(root, name, mode, reader)
	})
}

func extractArchiveWith(data []byte, typ userLocalArchiveType, write func(string, os.FileMode, io.Reader) error, checks ...func() error) (resultErr error) {
	check := func() error {
		if len(checks) > 0 && checks[0] != nil {
			return checks[0]()
		}
		return nil
	}
	files := 0
	var extracted int64
	checkSize := func(size int64) error {
		files++
		extracted += size
		if files > maxArchiveFiles || size < 0 || extracted > maxExtractedBytes {
			return errors.New("archive exceeds extraction limits")
		}
		return nil
	}
	switch typ {
	case userLocalArchiveTarGzip:
		gz, err := gzip.NewReader(&receiptContextReader{reader: bytes.NewReader(data), check: check})
		if err != nil {
			return fmt.Errorf("open tar.gz artifact: %w", err)
		}
		defer func() { resultErr = errors.Join(resultErr, gz.Close()) }()
		tr := tar.NewReader(gz)
		for {
			if err := check(); err != nil {
				return err
			}
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
			if err := checkSize(h.Size); err != nil {
				return err
			}
			if err := write(h.Name, os.FileMode(h.Mode)&0777, tr); err != nil {
				return err
			}
		}
	case userLocalArchiveZip:
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return fmt.Errorf("open zip artifact: %w", err)
		}
		for _, f := range zr.File {
			if err := check(); err != nil {
				return err
			}
			if f.FileInfo().IsDir() {
				continue
			}
			if err := checkSize(int64(f.UncompressedSize64)); err != nil {
				return err
			}
			reader, err := f.Open()
			if err != nil {
				return err
			}
			writeErr := write(f.Name, f.Mode()&0777, reader)
			closeErr := reader.Close()
			if err := errors.Join(writeErr, closeErr); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported user-local archive type %q", typ)
	}
}

func writeExtractedFile(root *receiptRoot, name string, mode os.FileMode, reader io.Reader, exact ...bool) (resultErr error) {
	joined, err := safeJoin(root.path, name)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root.path, joined)
	if err != nil {
		return err
	}
	parent, base, err := root.parent(relative, true)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	fd, err := unix.Openat(int(parent.Fd()), base, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(mode.Perm()))
	if err != nil {
		return fmt.Errorf("create extracted file %s: %w", name, err)
	}
	file := os.NewFile(uintptr(fd), base)
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	if len(exact) > 0 && exact[0] {
		if err := file.Chmod(mode.Perm()); err != nil {
			return err
		}
	}
	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("write extracted file %s: %w", name, err)
	}
	return nil
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

type receiptContextReader struct {
	reader io.Reader
	check  func() error
}

func (r *receiptContextReader) Read(data []byte) (int, error) {
	if r.check != nil {
		if err := r.check(); err != nil {
			return 0, err
		}
	}
	return r.reader.Read(data)
}
