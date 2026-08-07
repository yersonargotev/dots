package deps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadAndVerifyRejectsDigestMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("untrusted artifact"))
	}))
	defer server.Close()

	_, err := downloadAndVerify(server.URL, strings.Repeat("0", 64))
	if err == nil || !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("downloadAndVerify() error = %v, want digest mismatch", err)
	}
}

func TestInstallUserLocalRejectsUnsafeArchiveBeforeLinkReplacement(t *testing.T) {
	home := t.TempDir()
	data := tarGz(t, map[string]string{"../../escaped": "malicious\n", "bin/codex": "#!/bin/sh\n"})
	sum := sha256.Sum256(data)
	oldDownload := downloadUserLocalArtifact
	downloadUserLocalArtifact = func(url, checksum string) ([]byte, error) {
		if checksum != hex.EncodeToString(sum[:]) {
			t.Fatalf("checksum = %q", checksum)
		}
		return data, nil
	}
	t.Cleanup(func() { downloadUserLocalArtifact = oldDownload })

	action := InstallAction{UserLocal: &UserLocalArtifact{Recipe: "codex", Version: "rust-v1.0.0", URL: "https://example.invalid/codex.tar.gz", Checksum: hex.EncodeToString(sum[:]), Layout: userLocalLayoutBundle, Command: "codex"}}
	err := InstallUserLocal(home, action)
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("InstallUserLocal() error = %v, want unsafe archive rejection", err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".local", "bin", "codex")); !os.IsNotExist(err) {
		t.Fatalf("codex link created after unsafe archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "escaped")); !os.IsNotExist(err) {
		t.Fatalf("archive escaped selected home: %v", err)
	}
}

func TestInstallUserLocalInstallsNeovimBundleInSandboxHome(t *testing.T) {
	home := t.TempDir()
	artifactData := tarGz(t, map[string]string{
		"nvim-linux-x86_64/bin/nvim":          "#!/bin/sh\necho nvim\n",
		"nvim-linux-x86_64/share/nvim/readme": "runtime files\n",
	})

	oldDownload := downloadUserLocalArtifact
	downloadUserLocalArtifact = func(url, checksum string) ([]byte, error) {
		if url != "https://github.com/neovim/neovim/releases/download/v0.12.3/nvim-linux-x86_64.tar.gz" {
			t.Fatalf("download url = %q, want Neovim linux amd64 artifact", url)
		}
		return artifactData, nil
	}
	t.Cleanup(func() { downloadUserLocalArtifact = oldDownload })

	action := InstallAction{UserLocal: &UserLocalArtifact{
		Recipe:   "nvim",
		Version:  "v0.12.3",
		URL:      "https://github.com/neovim/neovim/releases/download/v0.12.3/nvim-linux-x86_64.tar.gz",
		Checksum: "test-checksum",
		Layout:   userLocalLayoutBundle,
		Command:  "nvim",
	}}

	if err := InstallUserLocal(home, action); err != nil {
		t.Fatalf("InstallUserLocal() error = %v", err)
	}

	binary := filepath.Join(home, ".local", "opt", "nvim", "v0.12.3", "nvim-linux-x86_64", "bin", "nvim")
	if info, err := os.Stat(binary); err != nil || info.Mode()&0o111 == 0 {
		t.Fatalf("installed binary stat = (%v, %v), want executable under sandbox opt", info, err)
	}
	shim := filepath.Join(home, ".local", "bin", "nvim")
	if target, err := os.Readlink(shim); err != nil || target != binary {
		t.Fatalf("nvim shim = (%q, %v), want symlink to %q", target, err, binary)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "opt", "nvim", "v0.12.3", "nvim-linux-x86_64", "share", "nvim", "readme")); err != nil {
		t.Fatalf("runtime file missing from bundle: %v", err)
	}
}

func TestRecordDependencyInstallationUsesSandboxStateAndHome(t *testing.T) {
	home := t.TempDir()
	stateRoot := t.TempDir()
	action := InstallAction{
		Dependency: "neovim",
		UserLocal: &UserLocalArtifact{
			Recipe:   "nvim",
			Version:  "v0.12.3",
			Checksum: "c441b547142860bf01bcce39e36cbed185c41112813e15443b16e5237750724d",
			Command:  "nvim",
		},
	}

	if err := RecordDependencyInstallation(stateRoot, home, action); err != nil {
		t.Fatalf("RecordDependencyInstallation() error = %v", err)
	}

	meta, err := LoadDependencyMetadata(DependencyMetadataPath(stateRoot))
	if err != nil {
		t.Fatalf("LoadDependencyMetadata() error = %v", err)
	}
	if len(meta.Dependencies) != 1 {
		t.Fatalf("dependencies = %#v, want one record", meta.Dependencies)
	}
	record := meta.Dependencies[0]
	if record.Dependency != "neovim" || record.Provider != string(TierUserLocal) || record.Path != filepath.Join(home, ".local", "bin", "nvim") {
		t.Fatalf("dependency record = %#v, want sandboxed home/state record", record)
	}
}

func tarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		hdr := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}
