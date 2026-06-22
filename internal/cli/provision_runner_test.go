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

func TestEnvWithHomeOverridesExistingHome(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/real/home", "EDITOR=nvim"}

	got := envWithHome(base, "/sandbox/home")

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
	if !containsEnv(got, "PATH=/usr/bin") || !containsEnv(got, "EDITOR=nvim") {
		t.Fatalf("env lost unrelated variables: %#v", got)
	}
}

func TestProvisionExecRunnerThreadsHomeToSubprocess(t *testing.T) {
	sandboxHome := t.TempDir()
	fakeRealHome := t.TempDir()

	// A stub standing in for gentle-ai: it writes a marker into whatever HOME the
	// subprocess sees. A correctly threaded runner makes that the sandbox home.
	stub := filepath.Join(t.TempDir(), "gentle-ai")
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

func TestRunProvisionersRendersPartialReportOnFailure(t *testing.T) {
	home := t.TempDir()
	stubDir := t.TempDir()
	countPath := filepath.Join(t.TempDir(), "gentle-ai-count")
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
	if err := os.WriteFile(filepath.Join(stubDir, "gentle-ai"), []byte(script), 0o755); err != nil {
		t.Fatalf("write gentle-ai stub: %v", err)
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
			{Tool: "gentle-ai", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Scope: "global", Agents: []string{"codex"}}},
			{Tool: "gentle-ai", Tags: []string{"core"}, Spec: manifest.ProvisionerSpec{Scope: "global", Agents: []string{"claude"}}},
		},
	}

	cmd := NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := runProvisioners(cmd, m, "default", nil, home)
	if err == nil {
		t.Fatal("runProvisioners() error = nil, want second provisioner failure")
	}

	got := out.String()
	for _, want := range []string{
		`Provisioner results for profile "default"`,
		"gentle-ai install --scope global --agents codex — provisioned",
		"gentle-ai install --scope global --agents claude — failed",
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

	if err := runProvisioners(cmd, m, "default", nil, home); err != nil {
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
	if _, err := os.ReadFile(filepath.Join(home, ".codex", "AGENTS.md")); err != nil {
		t.Fatalf("skills-only Codex provisioner did not write Codex overlay: %v", err)
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

	if err := runProvisioners(cmd, m, "default", []string{"codegraph"}, home); err != nil {
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
	}
}

func TestSelectedProvisionersAffectCodex(t *testing.T) {
	tests := []struct {
		name     string
		selected []manifest.Provisioner
		want     bool
	}{
		{
			name: "skills provisioner targeting codex",
			selected: []manifest.Provisioner{
				{Tool: "skills", Spec: manifest.ProvisionerSpec{Agents: []string{"codex"}}},
			},
			want: true,
		},
		{
			name: "skills provisioner targeting claude only",
			selected: []manifest.Provisioner{
				{Tool: "skills", Spec: manifest.ProvisionerSpec{Agents: []string{"claude"}}},
			},
			want: false,
		},
		{
			name: "gentle-ai provisioner targeting codex",
			selected: []manifest.Provisioner{
				{Tool: "gentle-ai", Spec: manifest.ProvisionerSpec{Agents: []string{"codex"}}},
			},
			want: true,
		},
		{
			name: "codex provisioner",
			selected: []manifest.Provisioner{
				{Tool: "codex"},
			},
			want: true,
		},
		{
			name: "no codex-affecting provisioners",
			selected: []manifest.Provisioner{
				{Tool: "claude"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectedProvisionersAffectCodex(tt.selected); got != tt.want {
				t.Fatalf("selectedProvisionersAffectCodex() = %v, want %v", got, tt.want)
			}
		})
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
