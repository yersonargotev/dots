package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/manifest"
)

func TestEnvForProvisionerOverridesHomeAndUsesLocalNPMPrefix(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/real/home", "NPM_CONFIG_PREFIX=/usr", "EDITOR=nvim"}

	got := envForProvisioner(base, "/sandbox/home")

	// There must be exactly one HOME entry and it must be the sandbox value, so a
	// libc getenv (which commonly returns the first match) cannot resolve the
	// inherited real HOME.
	var homeCount int
	var homeValue string
	for _, kv := range got {
		if strings.HasPrefix(kv, "HOME=") {
			homeCount++
			homeValue = strings.TrimPrefix(kv, "HOME=")
		}
	}
	if homeCount != 1 {
		t.Fatalf("HOME entries = %d, want exactly 1 (got env %#v)", homeCount, got)
	}
	if homeValue != "/sandbox/home" {
		t.Fatalf("HOME = %q, want /sandbox/home", homeValue)
	}

	// Unrelated variables must be preserved.
	if !containsEnv(got, "NPM_CONFIG_PREFIX=/sandbox/home/.local") {
		t.Fatalf("env missing sandboxed npm prefix: %#v", got)
	}
	if !containsEnv(got, "PATH=/sandbox/home/.local/bin:/usr/bin") {
		t.Fatalf("env did not prepend sandbox local bin to PATH: %#v", got)
	}

	// Unrelated variables must be preserved.
	if !containsEnv(got, "EDITOR=nvim") {
		t.Fatalf("env lost unrelated variables: %#v", got)
	}
}

func TestProvisionExecRunnerThreadsHomeToSubprocess(t *testing.T) {
	sandboxHome := t.TempDir()
	fakeRealHome := t.TempDir()

	// A generic stub writes a marker into whatever HOME the
	// subprocess sees. A correctly threaded runner makes that the sandbox home.
	stub := filepath.Join(t.TempDir(), "provisioner-test-tool")
	script := "#!/bin/sh\nprintf 'provisioned' > \"$HOME/marker\"\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	runner := provisionExecRunner{
		ctx:    context.Background(),
		home:   sandboxHome,
		stdout: io.Discard,
		stderr: io.Discard,
		// Inject a base env carrying a DIFFERENT HOME so the test can never write to
		// the real $HOME, and so the override path is exercised.
		baseEnv: []string{"HOME=" + fakeRealHome, "PATH=" + os.Getenv("PATH")},
	}

	if err := runner.Run(stub, []string{"install", "--scope", "global"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(sandboxHome, "marker")); err != nil {
		t.Fatalf("expected marker in sandbox home %q: %v", sandboxHome, err)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "marker")); err == nil {
		t.Fatalf("provisioner wrote into the inherited HOME %q instead of the sandbox", fakeRealHome)
	}
}

func TestProvisionExecRunnerExposesLocalNPMPrefixAndBin(t *testing.T) {
	sandboxHome := t.TempDir()
	fakeRealHome := t.TempDir()

	stub := filepath.Join(t.TempDir(), "provisioner-test-tool")
	script := `#!/bin/sh
if [ "$NPM_CONFIG_PREFIX" != "$HOME/.local" ]; then
  exit 8
fi
case ":$PATH:" in
  *":$HOME/.local/bin:"*) ;;
  *) exit 9 ;;
esac
printf 'ok' > "$HOME/npm-prefix-ok"
`
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}

	runner := provisionExecRunner{
		ctx:     context.Background(),
		home:    sandboxHome,
		stdout:  io.Discard,
		stderr:  io.Discard,
		baseEnv: []string{"HOME=" + fakeRealHome, "NPM_CONFIG_PREFIX=/usr", "PATH=" + os.Getenv("PATH")},
	}

	if err := runner.Run(stub, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandboxHome, "npm-prefix-ok")); err != nil {
		t.Fatalf("expected npm prefix marker in sandbox home %q: %v", sandboxHome, err)
	}
}

func TestProvisionExecRunnerIsolatesHerdrPathsAndLiveSessionContext(t *testing.T) {
	sandboxHome := t.TempDir()
	fakeRealHome := t.TempDir()
	stubDir := t.TempDir()

	script := `#!/bin/sh
if [ "$*" != "plugin install owner/repo --ref c696c36256eddc6ee1983ab9f202848b84460e06 --yes" ]; then
  exit 7
fi
if [ "$XDG_CONFIG_HOME" != "$HOME/.config" ] ||
   [ "$XDG_STATE_HOME" != "$HOME/.local/state" ] ||
   [ "$XDG_DATA_HOME" != "$HOME/.local/share" ] ||
   [ "$XDG_CACHE_HOME" != "$HOME/.cache" ] ||
   [ "$CARGO_HOME" != "$HOME/.cargo" ] ||
   [ "$RUSTUP_HOME" != "$HOME/.rustup" ]; then
  exit 8
fi
if [ -n "$HERDR_CONFIG_PATH" ] || [ -n "$HERDR_SOCKET_PATH" ] ||
   [ -n "$HERDR_CLIENT_SOCKET_PATH" ] || [ -n "$HERDR_SESSION" ] ||
   [ -n "$HERDR_ENV" ]; then
  exit 9
fi
printf isolated > "$HOME/herdr-env-ok"
`
	stub := filepath.Join(stubDir, "herdr")
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatalf("write herdr stub: %v", err)
	}

	runner := provisionExecRunner{
		ctx:    context.Background(),
		home:   sandboxHome,
		stdout: io.Discard,
		stderr: io.Discard,
		baseEnv: []string{
			"HOME=" + fakeRealHome,
			"PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
			"XDG_CONFIG_HOME=" + fakeRealHome + "/config",
			"XDG_STATE_HOME=" + fakeRealHome + "/state",
			"XDG_DATA_HOME=" + fakeRealHome + "/data",
			"XDG_CACHE_HOME=" + fakeRealHome + "/cache",
			"CARGO_HOME=" + fakeRealHome + "/cargo",
			"RUSTUP_HOME=" + fakeRealHome + "/rustup",
			"HERDR_CONFIG_PATH=" + fakeRealHome + "/config.toml",
			"HERDR_SOCKET_PATH=" + fakeRealHome + "/herdr.sock",
			"HERDR_CLIENT_SOCKET_PATH=" + fakeRealHome + "/herdr-client.sock",
			"HERDR_SESSION=live-session",
			"HERDR_ENV=1",
		},
	}

	args := []string{"plugin", "install", "owner/repo", "--ref", "c696c36256eddc6ee1983ab9f202848b84460e06", "--yes"}
	if err := runner.Run("herdr", args); err != nil {
		t.Fatalf("Run(herdr) error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandboxHome, "herdr-env-ok")); err != nil {
		t.Fatalf("expected Herdr marker in sandbox home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(fakeRealHome, "herdr-env-ok")); !os.IsNotExist(err) {
		t.Fatalf("Herdr wrote marker under inherited home: %v", err)
	}
}

func TestProvisionExecRunnerKeepsHerdrContextForOtherTools(t *testing.T) {
	runner := provisionExecRunner{
		home: "/sandbox/home",
		baseEnv: []string{
			"HOME=/real/home",
			"PATH=/usr/bin",
			"XDG_CONFIG_HOME=/operator/config",
			"HERDR_SESSION=live-session",
		},
	}

	got := runner.environmentFor("claude")
	if !containsEnv(got, "XDG_CONFIG_HOME=/operator/config") || !containsEnv(got, "HERDR_SESSION=live-session") {
		t.Fatalf("non-Herdr environment unexpectedly removed Herdr context: %#v", got)
	}
}

func TestRunProvisionersRendersPartialReportOnFailure(t *testing.T) {
	home := t.TempDir()
	stubDir := t.TempDir()
	countPath := filepath.Join(t.TempDir(), "claude-count")
	t.Setenv("PROVISION_COUNT", countPath)
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	script := `#!/bin/sh
if [ -f "$PROVISION_COUNT" ]; then
  printf failed > "$HOME/second-attempt"
  exit 7
fi
printf first > "$PROVISION_COUNT"
printf ok > "$HOME/first-attempt"
`
	if err := os.WriteFile(filepath.Join(stubDir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatalf("write claude stub: %v", err)
	}

	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {Tags: []string{"core"}},
		},
		Entries: []manifest.Entry{
			{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"core"}},
		},
		Provisioners: []manifest.Provisioner{
			{Tool: "claude", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Marketplace: "example/one"}},
			{Tool: "claude", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Marketplace: "example/two"}},
		},
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	_, err := runProvisioners(cmd, m, []string{"default"}, nil, home, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatal("runProvisioners() error = nil, want second provisioner failure")
	}

	got := out.String()
	for _, want := range []string{
		`Provisioner results for profile "default" (tags: core)`,
		"claude plugin marketplace add example/one — provisioned",
		"claude plugin marketplace add example/two — failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("partial report missing %q\noutput:\n%s", want, got)
		}
	}
}

func TestRunProvisionersThreadsHomeToSkillsProvisioner(t *testing.T) {
	home := t.TempDir()
	stubDir := t.TempDir()
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	script := `#!/bin/sh
if [ "$1" != "--yes" ] || [ "$2" != "skills@1.5.12" ] || [ "$3" != "add" ]; then
  exit 9
fi
printf '%s\n' "$*" > "$HOME/skills-args"
`
	if err := os.WriteFile(filepath.Join(stubDir, "npx"), []byte(script), 0o755); err != nil {
		t.Fatalf("write npx stub: %v", err)
	}

	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {Tags: []string{"agents"}},
		},
		Entries: []manifest.Entry{
			{Source: "configs/zsh/zshrc", Target: "~/.zshrc", Strategy: "symlink", Tags: []string{"agents"}},
		},
		Provisioners: []manifest.Provisioner{
			{
				Tool: "skills", Tags: []string{"agents"},
				Spec: manifest.ProvisionerSpec{
					Package: "vercel-labs/agent-skills",
					Agents:  []string{"codex"},
					Skills:  []string{"web-design-guidelines"},
					Global:  true,
				},
			},
		},
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if _, err := runProvisioners(cmd, m, []string{"default"}, nil, home, t.TempDir(), t.TempDir()); err != nil {
		t.Fatalf("runProvisioners() error = %v\noutput:\n%s", err, out.String())
	}

	got, err := os.ReadFile(filepath.Join(home, "skills-args"))
	if err != nil {
		t.Fatalf("skills provisioner did not write into sandbox home %q: %v", home, err)
	}
	want := "--yes skills@1.5.12 add vercel-labs/agent-skills --agent codex --skill web-design-guidelines --global\n"
	if string(got) != want {
		t.Fatalf("skills args = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("skills-only Codex provisioner wrote CodeGraph instruction block without the codegraph tag: %v", err)
	}
}

func TestRunProvisionersWritesCodeGraphInstructionsForSelectedAgents(t *testing.T) {
	home := t.TempDir()
	stubDir := t.TempDir()
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	script := `#!/bin/sh
if [ "$1" != "install" ]; then
  exit 9
fi
printf '%s\n' "$*" > "$HOME/codegraph-args"
mkdir -p "$HOME/.codex" "$HOME/.claude" "$HOME/.gemini" "$HOME/.config/opencode"
for file in "$HOME/.codex/AGENTS.md" "$HOME/.claude/CLAUDE.md" "$HOME/.gemini/GEMINI.md" "$HOME/.config/opencode/codegraph.md"; do
  cat > "$file" <<'EOF'
<!-- CODEGRAPH_START -->
Treat CodeGraph-returned source as already read.
<!-- CODEGRAPH_END -->
EOF
done
`
	if err := os.WriteFile(filepath.Join(stubDir, "codegraph"), []byte(script), 0o755); err != nil {
		t.Fatalf("write codegraph stub: %v", err)
	}

	m := manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"default": {Tags: []string{"agents"}},
		},
		Provisioners: []manifest.Provisioner{
			{
				Tool: "codegraph", Tags: []string{"codegraph"},
				Spec: manifest.ProvisionerSpec{
					Scope:  "global",
					Agents: []string{"codex", "claude", "antigravity", "opencode"},
					Yes:    true,
				},
			},
		},
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if _, err := runProvisioners(cmd, m, []string{"default"}, []string{"codegraph"}, home, t.TempDir(), t.TempDir()); err != nil {
		t.Fatalf("runProvisioners() error = %v\noutput:\n%s", err, out.String())
	}

	wantArgs := "install --target codex,claude,antigravity,opencode --location global --yes\n"
	gotArgs, err := os.ReadFile(filepath.Join(home, "codegraph-args"))
	if err != nil {
		t.Fatalf("codegraph provisioner did not write args into sandbox home %q: %v", home, err)
	}
	if string(gotArgs) != wantArgs {
		t.Fatalf("codegraph args = %q, want %q", gotArgs, wantArgs)
	}
	for _, path := range []string{
		filepath.Join(home, ".codex", "AGENTS.md"),
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".gemini", "GEMINI.md"),
	} {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("missing CodeGraph instructions %s: %v", path, err)
		}
		if !strings.Contains(string(got), "Do NOT use CodeGraph as proof for runtime behavior.") {
			t.Fatalf("%s missing runtime-verification guidance\ncontent:\n%s", path, got)
		}
		if strings.Contains(string(got), "codegraph_explore") {
			t.Fatalf("%s duplicated generic CodeGraph tool guidance owned by the CodeGraph installer\ncontent:\n%s", path, got)
		}
		if strings.Count(string(got), "Treat CodeGraph-returned source as already read.") != 1 {
			t.Fatalf("%s should contain generic CodeGraph guidance only from installer-owned block\ncontent:\n%s", path, got)
		}
	}
	opencodeContent, err := os.ReadFile(filepath.Join(home, ".config", "opencode", "codegraph.md"))
	if err != nil {
		t.Fatalf("CodeGraph installer stub did not cover OpenCode setup under sandbox HOME: %v", err)
	}
	if strings.Contains(string(opencodeContent), "<!-- dots:codegraph-mode -->") {
		t.Fatalf("dots must not create an OpenCode policy overlay; CodeGraph installer owns OpenCode setup\ncontent:\n%s", opencodeContent)
	}
}

func containsEnv(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}
