package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/deps"
	"github.com/yersonargotev/dots/internal/manifest"
	"github.com/yersonargotev/dots/internal/state"
	"gopkg.in/yaml.v3"
)

func TestRepositoryHerdrCustomInstallsPinnedPluginsInSandbox(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	// Resolve only the two shell utilities the generated launcher actually needs
	// before restricting PATH. Every dependency and package manager is a stub.
	shellUtilities := map[string]string{}
	for _, command := range []string{"dirname", "readlink"} {
		path, err := exec.LookPath(command)
		if err != nil {
			t.Fatal(err)
		}
		shellUtilities[command] = path
	}
	for _, platform := range []struct{ os, arch, target string }{{"darwin", "arm64", "macos-aarch64"}, {"linux", "amd64", "linux-x86_64"}} {
		t.Run(platform.os, func(t *testing.T) {
			home, stateRoot, fakeRealHome, stubs := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
			bin := filepath.Join(home, ".local/bin")
			if err := os.MkdirAll(bin, 0755); err != nil {
				t.Fatal(err)
			}
			official := filepath.Join(bin, "herdr")
			sentinel := "#!/bin/sh\nexit 99\n"
			writeHerdrIntegrationStub(t, official, sentinel)
			for command, path := range shellUtilities {
				if err := os.Symlink(path, filepath.Join(stubs, command)); err != nil {
					t.Fatal(err)
				}
			}
			for _, command := range []string{"git", "python3", "fnm", "node", "go", "rustup", "rustc", "cargo", "curl", "tar", "pbcopy", "open", "lazygit", "fzf", "fd", "bat", "cc", "xdg-open", "wl-copy", "xclip", "xsel", "npx", "brew", "apt", "apt-get", "dnf", "pacman", "sudo"} {
				writeHerdrIntegrationStub(t, filepath.Join(stubs, command), "#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$HOME/external-command.log\"\nexit 0\n")
			}
			t.Setenv("HOME", home)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+stubs)
			for _, name := range []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "CARGO_HOME", "RUSTUP_HOME"} {
				t.Setenv(name, filepath.Join(fakeRealHome, name))
			}
			for _, name := range []string{"HERDR_SOCKET_PATH", "HERDR_SESSION", "HERDR_SESSION_NAME", "HERDR_CONFIG_PATH"} {
				t.Setenv(name, filepath.Join(fakeRealHome, name))
			}
			oldOS, oldArch := installHostOS, installHostArch
			installHostOS, installHostArch = platform.os, platform.arch
			t.Cleanup(func() { installHostOS, installHostArch = oldOS, oldArch })

			archiveName := "herdr-custom-0.9.3-custom.6-" + platform.target + ".tar.gz"
			data := herdrCustomCLIArchive(t, strings.TrimSuffix(archiveName, ".tar.gz"))
			sum := sha256.Sum256(data)
			checksum := hex.EncodeToString(sum[:])
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/"+archiveName {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			oldTransport := http.DefaultTransport
			http.DefaultTransport = herdrCustomCLIFixtureTransport{base: oldTransport, allowedURL: server.URL + "/" + archiveName}
			t.Cleanup(func() { http.DefaultTransport = oldTransport })
			artifact := deps.UserLocalArtifact{Recipe: "herdr-custom", Version: "v0.9.3-custom.6", Artifact: archiveName, URL: server.URL + "/" + archiveName, Checksum: checksum, Platform: platform.os + "_" + platform.arch, Layout: "bundle", Command: "herdr-custom", Destination: filepath.Join(home, ".local/opt/herdr-custom/v0.9.3-custom.6"), InstalledPath: filepath.Join(bin, "herdr-custom")}
			if err := deps.InstallUserLocal(home, deps.InstallAction{UserLocal: &artifact}); err != nil {
				t.Fatal(err)
			}
			m, err := manifest.LoadFile(filepath.Join(root, "dots.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			replaceChecksum := func(items []manifest.Dependency) {
				for i := range items {
					d := &items[i]
					if d.UserLocal != nil && d.UserLocal.Recipe == "herdr-custom" {
						d.UserLocal.Checksums[platform.os+"_"+platform.arch] = checksum
					}
				}
			}
			for i := range m.Dependencies {
				replaceChecksum(m.Dependencies[i].Dependencies)
			}
			for i := range m.Entries {
				replaceChecksum(m.Entries[i].Dependencies)
			}
			for i := range m.Provisioners {
				replaceChecksum(m.Provisioners[i].Dependencies)
			}
			encoded, err := yaml.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			manifestPath := filepath.Join(t.TempDir(), "dots.yaml")
			if err := os.WriteFile(manifestPath, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			command := NewRootCommand()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs([]string{"install", "--yes", "--tag", "herdr-custom", "--file", manifestPath, "--home", home, "--source-root", root, "--state-root", stateRoot})
			if err := command.Execute(); err != nil {
				t.Fatalf("install: %v\n%s", err, output.String())
			}
			log, err := os.ReadFile(filepath.Join(home, "custom-plugin.log"))
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"plugin install yersonargotev/herdr-tab-git --ref 8eb3a68dcf79797593c0eca1bacab2aaa23e05ec --yes"}
			if platform.os == "darwin" {
				want = append(want, "plugin install yersonargotev/tabby --ref 19031d668f0f8a79b58591db4ce8bc8f8846e329 --yes")
			}
			want = append(want, "plugin install rmarganti/herdr-pluck --ref d1eacb80956c3a23ab6f7428a9e83961fb86ba28 --yes")
			got := strings.Split(strings.TrimSpace(string(log)), "\n")
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("custom commands=%v want=%v", got, want)
			}
			meta, err := state.Load(state.Path(stateRoot))
			if err != nil {
				t.Fatal(err)
			}
			if len(meta.Provisioners) != len(want) {
				t.Fatalf("recorded provisioners=%d want=%d", len(meta.Provisioners), len(want))
			}
			config, err := os.ReadFile(filepath.Join(home, ".config/herdr/config.toml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range []string{"sidebar_width = 38", "sidebar_max_width = 44", `status_indicators = "dots"`, `token = "tab"`} {
				if !strings.Contains(string(config), value) {
					t.Fatalf("installed config missing %q", value)
				}
			}
			tabbyPath := filepath.Join(home, ".config/herdr/plugins/config/yersonargotev.tabby/config.toml")
			if _, err := os.Stat(tabbyPath); platform.os == "darwin" && err != nil || platform.os == "linux" && !os.IsNotExist(err) {
				t.Fatalf("Tabby config on %s: %v", platform.os, err)
			}
			after, err := os.ReadFile(official)
			if err != nil || string(after) != sentinel {
				t.Fatalf("official changed: %q %v", after, err)
			}
			if entries, err := os.ReadDir(fakeRealHome); err != nil || len(entries) != 0 {
				t.Fatalf("inherited roots mutated: %v %v", entries, err)
			}
			if log, err := os.ReadFile(filepath.Join(home, "external-command.log")); err == nil && len(log) > 0 {
				for _, line := range strings.Split(strings.TrimSpace(string(log)), "\n") {
					if line != filepath.Join(stubs, "git")+" -C "+root+" rev-parse --short HEAD" {
						t.Fatalf("unexpected external execution: %s", line)
					}
				}
			} else if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		})
	}
}

func herdrCustomCLIArchive(t *testing.T, root string) []byte {
	t.Helper()
	runtime := `#!/bin/sh
set -eu
[ "$HERDR_CONFIG_PATH" = "$HOME/.config/herdr/config.toml" ]
[ "$XDG_CONFIG_HOME" = "$HOME/.config" ]
[ "$XDG_STATE_HOME" = "$HOME/.local/state" ]
[ "$XDG_DATA_HOME" = "$HOME/.local/share" ]
[ "$XDG_CACHE_HOME" = "$HOME/.cache" ]
[ "$CARGO_HOME" = "$HOME/.cargo" ]
[ "$RUSTUP_HOME" = "$HOME/.rustup" ]
[ -z "${HERDR_SOCKET_PATH:-}" ]
[ -z "${HERDR_SESSION:-}" ]
[ -z "${HERDR_SESSION_NAME:-}" ]
printf '%s\n' "$*" >> "$HOME/custom-plugin.log"
`
	sum := sha256.Sum256([]byte(runtime))
	files := map[string]string{root + "/bin/herdr-custom": "#!/bin/sh\nexec \"$(dirname \"$0\")/../libexec/herdr\" \"$@\"\n", root + "/libexec/herdr": runtime, root + "/provenance.json": fmt.Sprintf(`{"binary_sha256":%q}`, hex.EncodeToString(sum[:]))}
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// Fail closed if an install regression tries to download outside the fixture.
type herdrCustomCLIFixtureTransport struct {
	base       http.RoundTripper
	allowedURL string
}

func (f herdrCustomCLIFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL.String() != f.allowedURL {
		return nil, fmt.Errorf("test forbids external HTTP request: %s", request.URL)
	}
	return f.base.RoundTrip(request)
}
