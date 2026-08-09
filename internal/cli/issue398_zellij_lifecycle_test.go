package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/cli"
	"github.com/yersonargotev/dots/internal/state"
)

func TestZellijConfigMigratesToWholeTargetAndPreservesPersistedDrift(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".dots-state")
	t.Setenv("HOME", t.TempDir())

	baseline := "theme \"catppuccin-mocha\"\n"
	persisted := "theme \"catppuccin-latte\"\nsimplified_ui true\n"
	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zellij/config.kdl":          baseline,
		"configs/zellij/config-adaptive.kdl": "theme_light \"catppuccin-latte\"\n",
		"dots.yaml":                          zellijManifest("symlink"),
	})
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")

	run := func(want int, args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		code := cli.Run(args, &stdout, &stderr)
		if code != want {
			t.Fatalf("cli.Run(%v) code = %d, want %d\nstdout:\n%s\nstderr:\n%s", args, code, want, stdout.String(), stderr.String())
		}
		return stdout.String()
	}
	common := []string{"--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}
	run(0, append([]string{"install", "--yes", "--skip-deps"}, common...)...)

	target := filepath.Join(home, ".config", "zellij", "config.kdl")
	if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("legacy Zellij config = (%v, %v), want symlink", info, err)
	}

	advanceUpstream(t, origin, "materialize Zellij config", map[string]string{"dots.yaml": zellijManifest("copy")})
	updateOutput := runUpdate(t, "--yes", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if !strings.Contains(updateOutput, "migrate") {
		t.Fatalf("update output missing Zellij migration evidence:\n%s", updateOutput)
	}
	if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("migrated Zellij config = (%v, %v), want regular file", info, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != baseline {
		t.Fatalf("migrated Zellij config = (%q, %v), want baseline", got, err)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 1 {
		t.Fatalf("migration Backup Sets = %#v, err = %v", backupMeta, err)
	}
	preserved, err := os.ReadFile(backups.FilePath(stateRoot, backupMeta.Sets[0].ID, 1, target))
	if err != nil || string(preserved) != baseline {
		t.Fatalf("migration backup = %q, err = %v", preserved, err)
	}

	if err := os.WriteFile(target, []byte(persisted), 0o600); err != nil {
		t.Fatalf("simulate Zellij persisted reconfiguration: %v", err)
	}
	if dirty := strings.TrimSpace(runGitOutput(t, sourceRoot, "status", "--porcelain")); dirty != "" {
		t.Fatalf("simulated Zellij persistence dirtied the Installed Repository: %q", dirty)
	}
	statusOutput := run(2, append([]string{"status", "--output", "json"}, common...)...)
	if !strings.Contains(statusOutput, `"state": "drifted"`) {
		t.Fatalf("status did not report persisted Zellij config as Drift:\n%s", statusOutput)
	}
	doctorOutput := run(2, append([]string{"doctor", "--output", "json"}, common...)...)
	if !strings.Contains(doctorOutput, `"state": "drifted"`) || !strings.Contains(doctorOutput, `"source": "configs/zellij/config.kdl"`) {
		t.Fatalf("doctor did not report persisted Zellij config as Drift:\n%s", doctorOutput)
	}

	installOutput := run(0, append([]string{"install", "--yes", "--skip-deps"}, common...)...)
	if !strings.Contains(installOutput, "conflict") {
		t.Fatalf("install output missing persisted-config finding:\n%s", installOutput)
	}
	updateOutput = runUpdate(t, "--yes", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if !strings.Contains(updateOutput, "conflict") {
		t.Fatalf("update output missing persisted-config finding:\n%s", updateOutput)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != persisted {
		t.Fatalf("install/update changed persisted Zellij config = (%q, %v)", got, err)
	}

	uninstallOutput := run(0, "uninstall", "--yes", "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	if !strings.Contains(uninstallOutput, "modified target(s) will be skipped") {
		t.Fatalf("uninstall output missing modified-target evidence:\n%s", uninstallOutput)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != persisted {
		t.Fatalf("uninstall changed persisted Zellij config = (%q, %v)", got, err)
	}
}

func TestZellijWholeTargetRetainsDefaultAndAdaptiveSources(t *testing.T) {
	for _, tt := range []struct {
		name      string
		extraArgs []string
		want      string
	}{
		{name: "default", want: "theme \"catppuccin-mocha\"\n"},
		{name: "adaptive", extraArgs: []string{"--tag", "adaptive-theme"}, want: "theme_light \"catppuccin-latte\"\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			stateRoot := filepath.Join(home, ".dots-state")
			t.Setenv("HOME", t.TempDir())
			_, sourceRoot := newInstalledRepo(t, map[string]string{
				"configs/zellij/config.kdl":          "theme \"catppuccin-mocha\"\n",
				"configs/zellij/config-adaptive.kdl": "theme_light \"catppuccin-latte\"\n",
				"dots.yaml":                          zellijManifest("copy"),
			})
			args := []string{"install", "--yes", "--skip-deps", "--profile", "default"}
			args = append(args, tt.extraArgs...)
			args = append(args, "--file", filepath.Join(sourceRoot, "dots.yaml"), "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
			var stdout, stderr bytes.Buffer
			if code := cli.Run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("install code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
			}
			target := filepath.Join(home, ".config", "zellij", "config.kdl")
			if info, err := os.Lstat(target); err != nil || !info.Mode().IsRegular() {
				t.Fatalf("Zellij config = (%v, %v), want regular file", info, err)
			}
			if got, err := os.ReadFile(target); err != nil || string(got) != tt.want {
				t.Fatalf("Zellij config = (%q, %v), want %q", got, err, tt.want)
			}
		})
	}
}

func TestZellijLegacySymlinkWithoutProvenanceFailsClosed(t *testing.T) {
	requireGitCLI(t)
	home := t.TempDir()
	stateRoot := filepath.Join(home, ".dots-state")
	t.Setenv("HOME", t.TempDir())

	baseline := "theme \"catppuccin-mocha\"\n"
	origin, sourceRoot := newInstalledRepo(t, map[string]string{
		"configs/zellij/config.kdl":          baseline,
		"configs/zellij/config-adaptive.kdl": "theme_light \"catppuccin-latte\"\n",
		"dots.yaml":                          zellijManifest("symlink"),
	})
	manifestPath := filepath.Join(sourceRoot, "dots.yaml")
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"install", "--yes", "--skip-deps", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot}, &stdout, &stderr); code != 0 {
		t.Fatalf("legacy install code = %d\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	meta, err := state.Load(state.Path(stateRoot))
	if err != nil {
		t.Fatalf("load legacy metadata: %v", err)
	}
	meta.Provenance = state.Provenance{}
	if err := state.Save(state.Path(stateRoot), meta); err != nil {
		t.Fatalf("remove provenance from legacy metadata: %v", err)
	}

	advanceUpstream(t, origin, "materialize Zellij config", map[string]string{"dots.yaml": zellijManifest("copy")})
	output := runUpdate(t, "--yes", "--profile", "default", "--file", manifestPath, "--home", home, "--source-root", sourceRoot, "--state-root", stateRoot)
	target := filepath.Join(home, ".config", "zellij", "config.kdl")
	if !strings.Contains(output, "conflict") {
		t.Fatalf("update output missing unproven migration conflict:\n%s", output)
	}
	if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("unproven legacy target = (%v, %v), want preserved symlink", info, err)
	}
	backupMeta, err := backups.Load(backups.Path(stateRoot))
	if err != nil || len(backupMeta.Sets) != 0 {
		t.Fatalf("unproven migration Backup Sets = %#v, err = %v", backupMeta, err)
	}
}

func zellijManifest(strategy string) string {
	return `version: 1
profiles:
  default:
    tags: [core]
entries:
  - source: configs/zellij/config.kdl
    source_overrides:
      adaptive-theme: configs/zellij/config-adaptive.kdl
    target: ~/.config/zellij/config.kdl
    strategy: ` + strategy + `
    tags: [core]
`
}
