package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/cli"
)

func TestApplicationWritableTargetsKeepInstalledRepositoryCleanAcrossLifecycle(t *testing.T) {
	requireGitCLI(t)
	repositoryRoot, err := filepath.Abs(filepath.Clean(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	repositoryFixture := t.TempDir()
	sourceRoot := filepath.Join(repositoryFixture, "installed-repository")
	runGit(t, "", "clone", "--local", repositoryRoot, sourceRoot)
	gitIdentity(t, sourceRoot)
	// CI checks out the repository at a detached commit. Give the Installed
	// Repository a complete local upstream so update exercises the same branch
	// discovery path on developer machines and detached CI runners.
	runGit(t, sourceRoot, "switch", "-C", "main")
	origin := filepath.Join(repositoryFixture, "origin.git")
	runGit(t, "", "clone", "--bare", "--local", sourceRoot, origin)
	runGit(t, sourceRoot, "remote", "set-url", "origin", origin)
	runGit(t, sourceRoot, "fetch", "origin", "main")
	runGit(t, sourceRoot, "remote", "set-head", "origin", "main")
	runGit(t, sourceRoot, "branch", "--set-upstream-to=origin/main", "main")

	home := t.TempDir()
	stateRoot := filepath.Join(home, ".local", "state", "dots")
	xdgStateHome := filepath.Join(home, ".local", "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", xdgStateHome)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))

	// The real core Manifest selects zimfw. Stub only its executable surfaces so
	// the lifecycle remains local; keep the real git executable for repository
	// cleanliness and update verification.
	stubDir := t.TempDir()
	writeExecStub(t, filepath.Join(stubDir, "zsh"), "#!/bin/sh\nexit 0\n")
	writeExecStub(t, filepath.Join(stubDir, "curl"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	selection := []string{"--profile", "core", "--profile", "desktop"}
	common := append(append([]string{}, selection...), "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	runIssue393CLI(t, 0, append([]string{"install", "--yes", "--skip-deps"}, common...)...)
	assertIssue393RepositoryClean(t, sourceRoot, "install")

	targets := map[string]string{
		"zed-settings": filepath.Join(home, ".config", "zed", "settings.json"),
		"zed-keymap":   filepath.Join(home, ".config", "zed", "keymap.json"),
		"lazy-lock":    filepath.Join(xdgStateHome, "nvim", "lazy-lock.json"),
		"zsh":          filepath.Join(home, ".zshrc"),
		"git":          filepath.Join(home, ".gitconfig"),
		"atuin":        filepath.Join(home, ".config", "atuin", "config.toml"),
		"herdr":        filepath.Join(home, ".config", "herdr", "config.toml"),
		"bat":          filepath.Join(home, ".config", "bat", "config"),
		"zellij":       filepath.Join(home, ".config", "zellij", "config.kdl"),
	}

	zedSettings, err := os.ReadFile(targets["zed-settings"])
	if err != nil {
		t.Fatalf("read Zed settings: %v", err)
	}
	lastBrace := bytes.LastIndexByte(zedSettings, '}')
	if lastBrace < 0 {
		t.Fatal("Zed settings has no closing object brace")
	}
	zedSettings = append(append(append([]byte{}, zedSettings[:lastBrace]...), []byte("  \"writer_extension\": true,\n")...), zedSettings[lastBrace:]...)
	writeIssue393Target(t, targets["zed-settings"], zedSettings)
	assertIssue393AlignedAndClean(t, common, sourceRoot, "Zed settings writer")

	appendIssue393Target(t, targets["zed-keymap"], "\n// persisted by the Zed keymap editor\n")
	assertIssue393AlignedAndClean(t, common, sourceRoot, "Zed keymap writer")
	lazyLock, err := os.ReadFile(targets["lazy-lock"])
	if err != nil {
		t.Fatalf("read lazy.nvim lockfile: %v", err)
	}
	lazyLock = bytes.Replace(lazyLock, []byte(`"commit": "`), []byte(`"commit": "local-`), 1)
	writeIssue393Target(t, targets["lazy-lock"], lazyLock)
	assertIssue393AlignedAndClean(t, common, sourceRoot, "lazy.nvim writer")
	appendIssue393Target(t, targets["zsh"], "\n# persisted by zsh-newuser-install\n")
	assertIssue393AlignedAndClean(t, common, sourceRoot, "Zsh writer")
	appendIssue393Target(t, targets["git"], "\n[user]\n\tname = Temporary Home\n")
	assertIssue393AlignedAndClean(t, common, sourceRoot, "Git writer")
	appendIssue393Target(t, targets["atuin"], "\n[writer_extension]\nenabled = true\n")
	assertIssue393AlignedAndClean(t, common, sourceRoot, "Atuin writer")

	if runtime.GOOS == "darwin" {
		appendIssue393Target(t, targets["herdr"], "\n[writer_extension]\nenabled = true\n")
		assertIssue393AlignedAndClean(t, common, sourceRoot, "Herdr writer")
	} else {
		t.Run("Herdr OS-filter-independent writer fixture", func(t *testing.T) {
			testIssue393HerdrWriter(t, sourceRoot)
		})
	}

	seededStatus := runIssue393CLI(t, 0, append([]string{"status", "--output", "json"}, common...)...)
	if strings.Count(seededStatus, `"reason": "seeded-local-evolution"`) < 2 {
		t.Fatalf("status JSON missing both seeded local-evolution states:\n%s", seededStatus)
	}

	batGenerated := []byte("# Generated by bat\n--plain\n")
	writeIssue393Target(t, targets["bat"], batGenerated)
	assertIssue393RepositoryClean(t, sourceRoot, "bat writer")
	batStatus := runIssue393CLI(t, 2, append([]string{"status", "--output", "json"}, common...)...)
	for _, want := range []string{`"source": "configs/bat/config"`, `"state": "drifted"`} {
		if !strings.Contains(batStatus, want) {
			t.Fatalf("bat status JSON missing %q:\n%s", want, batStatus)
		}
	}

	zellijPersisted := []byte("theme \"catppuccin-latte\"\nsimplified_ui true\n")
	writeIssue393Target(t, targets["zellij"], zellijPersisted)
	assertIssue393RepositoryClean(t, sourceRoot, "Zellij writer")
	textStatus := runIssue393CLI(t, 2, append([]string{"status"}, common...)...)
	for _, want := range []string{"drifted", "configs/bat/config", "configs/zellij/config.kdl"} {
		if !strings.Contains(textStatus, want) {
			t.Fatalf("status text missing %q:\n%s", want, textStatus)
		}
	}

	runIssue393CLI(t, 0, append([]string{"install", "--yes", "--skip-deps"}, common...)...)
	runIssue393CLI(t, 0, append([]string{"update", "--yes"}, common...)...)
	assertIssue393Content(t, targets["bat"], batGenerated, "install/update changed bat output")
	assertIssue393Content(t, targets["zellij"], zellijPersisted, "install/update changed Zellij output")
	assertIssue393RepositoryClean(t, sourceRoot, "install/update after writer Drift")

	uninstallOutput := runIssue393CLI(t, 0, "uninstall", "--yes", "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	for _, want := range []string{"Retained Seeded Runtime State", "modified target(s) will be skipped"} {
		if !strings.Contains(uninstallOutput, want) {
			t.Fatalf("uninstall output missing %q:\n%s", want, uninstallOutput)
		}
	}
	assertIssue393Content(t, targets["bat"], batGenerated, "uninstall changed bat output")
	assertIssue393Content(t, targets["zellij"], zellijPersisted, "uninstall changed Zellij output")
	for target, want := range map[string]string{
		targets["zed-settings"]: `"writer_extension": true`,
		targets["zed-keymap"]:   "persisted by the Zed keymap editor",
		targets["lazy-lock"]:    `"commit": "local-`,
		targets["zsh"]:          "persisted by zsh-newuser-install",
		targets["git"]:          "Temporary Home",
		targets["atuin"]:        "writer_extension",
	} {
		got, err := os.ReadFile(target)
		if err != nil || !strings.Contains(string(got), want) {
			t.Fatalf("preserved writer content %q = (%q, %v), want substring %q", target, got, err, want)
		}
	}
	assertIssue393RepositoryClean(t, sourceRoot, "uninstall")
}

func testIssue393HerdrWriter(t *testing.T, sourceRoot string) {
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".dots-state")
	manifestPath := filepath.Join(t.TempDir(), "herdr.yaml")
	manifest := `version: 1
profiles:
  core:
    tags: [core]
entries:
  - source: configs/herdr/config.toml
    target: ~/.config/herdr/config.toml
    strategy: copy
    ownership: toml-subset
    tags: [core]
`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o600); err != nil {
		t.Fatalf("write Herdr fixture manifest: %v", err)
	}
	common := []string{"--profile", "core", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}
	runIssue393CLI(t, 0, append([]string{"install", "--yes", "--skip-deps"}, common...)...)
	target := filepath.Join(home, ".config", "herdr", "config.toml")
	appendIssue393Target(t, target, "\n[writer_extension]\nenabled = true\n")
	assertIssue393AlignedAndClean(t, common, sourceRoot, "Herdr writer")
	runIssue393CLI(t, 0, "uninstall", "--yes", "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	got, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(got), "writer_extension") {
		t.Fatalf("Herdr external content after uninstall = (%q, %v)", got, err)
	}
}

func runIssue393CLI(t *testing.T, wantCode int, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(args, &stdout, &stderr)
	if code != wantCode {
		t.Fatalf("cli.Run(%v) code = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, wantCode, stdout.String(), stderr.String())
	}
	return stdout.String()
}

func assertIssue393AlignedAndClean(t *testing.T, common []string, sourceRoot, writer string) {
	t.Helper()
	runIssue393CLI(t, 0, append([]string{"status", "--output", "json"}, common...)...)
	assertIssue393RepositoryClean(t, sourceRoot, writer)
}

func assertIssue393RepositoryClean(t *testing.T, sourceRoot, after string) {
	t.Helper()
	if dirty := strings.TrimSpace(runGitOutput(t, sourceRoot, "status", "--porcelain")); dirty != "" {
		t.Fatalf("Installed Repository dirty after %s: %q", after, dirty)
	}
}

func appendIssue393Target(t *testing.T, path, suffix string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	writeIssue393Target(t, path, append(content, suffix...))
}

func writeIssue393Target(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("simulate writer for %s: %v", path, err)
	}
}

func assertIssue393Content(t *testing.T, path string, want []byte, context string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("%s: got (%q, %v), want %q", context, got, err, want)
	}
}
