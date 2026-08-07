package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedZshPathExposesHomebrewRustupProxies(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is not available")
	}

	fakeBin := t.TempDir()
	formulaPrefix := filepath.Join(t.TempDir(), "rustup")
	proxyBin := filepath.Join(formulaPrefix, "bin")
	if err := os.MkdirAll(proxyBin, 0o755); err != nil {
		t.Fatalf("create fake rustup proxy bin: %v", err)
	}
	for _, command := range []string{"rustc", "cargo"} {
		if err := os.WriteFile(filepath.Join(proxyBin, command), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatalf("write fake %s proxy: %v", command, err)
		}
	}
	brew := "#!/bin/sh\nprintf '%s\\n' " + shellSingleQuote(formulaPrefix) + "\n"
	if err := os.WriteFile(filepath.Join(fakeBin, "brew"), []byte(brew), 0o700); err != nil {
		t.Fatalf("write fake brew: %v", err)
	}

	config := filepath.Join("..", "..", "configs", "zsh", "rc.d", "post", "30-path.zsh")
	cmd := exec.Command(zsh, "-c", `source "$1"; command -v rustc; command -v cargo`, "zsh", config)
	cmd.Env = []string{
		"HOME=" + t.TempDir(),
		"PATH=" + fakeBin + string(os.PathListSeparator) + "/usr/bin:/bin",
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("source managed Zsh PATH config: %v\n%s", err, output)
	}
	for _, command := range []string{"rustc", "cargo"} {
		want := filepath.Join(proxyBin, command)
		if !strings.Contains(string(output), want) {
			t.Fatalf("managed Zsh PATH did not expose %s proxy %q:\n%s", command, want, output)
		}
	}
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
