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
	Recipe   string `json:"recipe"`
	Version  string `json:"version"`
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
	Layout   string `json:"layout"`
	Command  string `json:"command"`
}

const (
	userLocalLayoutSingle = "single-binary"
	userLocalLayoutBundle = "bundle"
)

type userLocalRecipe struct {
	archiveName func(version, goarch string) (string, bool)
	url         func(version, archive string) string
	layout      string
	command     string
	archiveType string
	binaryPath  func(archive, command string) string
	links       []string
}

var userLocalRecipes = map[string]userLocalRecipe{
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
		archiveType: "tar.gz",
		binaryPath:  func(archive, command string) string { return strings.TrimSuffix(archive, ".tar.gz") + "/" + command },
		links:       []string{"uv", "uvx"},
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
		archiveType: "zip",
		binaryPath:  func(archive, command string) string { return strings.TrimSuffix(archive, ".zip") + "/" + command },
		links:       []string{"bun"},
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
		archiveType: "tar.gz",
		binaryPath:  func(_ string, command string) string { return command },
		links:       []string{"zellij"},
	},
}

func userLocalArtifact(dep manifest.Dependency, opts Options) (UserLocalArtifact, bool, error) {
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
	return UserLocalArtifact{Recipe: recipeName, Version: version, URL: recipe.url(version, archive), Checksum: checksum, Layout: recipe.layout, Command: recipe.command}, true, nil
}

func platformKey(goos, goarch string) string { return goos + "_" + goarch }

func (a UserLocalArtifact) Hint() string {
	if a.Recipe == "" {
		return "user-local install"
	}
	target := "~/.local/bin/" + a.Command
	if a.Layout == userLocalLayoutBundle {
		target = "~/.local/opt/" + a.Recipe + " with shim in ~/.local/bin"
	}
	return fmt.Sprintf("user-local install %s %s to %s", a.Recipe, a.Version, target)
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

	data, err := downloadAndVerify(artifact.URL, artifact.Checksum)
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
		if err := os.RemoveAll(optDir); err != nil {
			return fmt.Errorf("prepare user-local opt directory: %w", err)
		}
		if err := os.MkdirAll(optDir, 0o755); err != nil {
			return fmt.Errorf("create user-local opt directory: %w", err)
		}
		if err := extractArchive(data, recipe.archiveType, optDir); err != nil {
			return err
		}
		for _, link := range recipe.links {
			target := filepath.Join(optDir, recipe.binaryPath(archive, link))
			if err := ensureExecutable(target); err != nil {
				return err
			}
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

func downloadAndVerify(url, checksum string) ([]byte, error) {
	client := http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download user-local artifact: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download user-local artifact: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read user-local artifact: %w", err)
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

func extractArchive(data []byte, typ, dest string) error {
	switch typ {
	case "tar.gz":
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
	case "zip":
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return fmt.Errorf("open zip artifact: %w", err)
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
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
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive path %q", name)
	}
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
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace user-local symlink: %w", err)
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("create user-local symlink: %w", err)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open extracted executable: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("create user-local executable: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy user-local executable: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close user-local executable: %w", closeErr)
	}
	return nil
}
