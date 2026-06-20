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

// installedEnv mirrors the on-disk and metadata state dots leaves after an
// install, so the uninstall command tests can run against realistic input.
type installedEnv struct {
	home       string
	sourceRoot string
	stateRoot  string
	meta       state.Metadata
}

func newInstalledEnv(t *testing.T) *installedEnv {
	t.Helper()
	return &installedEnv{
		home:       t.TempDir(),
		sourceRoot: t.TempDir(),
		stateRoot:  t.TempDir(),
		meta:       state.Metadata{Version: 1},
	}
}

func (e *installedEnv) installSymlink(t *testing.T, rel, name string) string {
	t.Helper()
	src := filepath.Join(e.sourceRoot, rel)
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(src, []byte("managed "+name+"\n"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	target := filepath.Join(e.home, name)
	if err := os.Symlink(src, target); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	e.meta.Entries = append(e.meta.Entries, state.Record{Target: target, Source: rel, Strategy: "symlink"})
	return target
}

func (e *installedEnv) installCopy(t *testing.T, rel, name string) string {
	t.Helper()
	src := filepath.Join(e.sourceRoot, rel)
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	content := "managed " + name + "\n"
	if err := os.WriteFile(src, []byte(content), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	hash, err := state.HashFile(src)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	target := filepath.Join(e.home, name)
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	e.meta.Entries = append(e.meta.Entries, state.Record{Target: target, Source: rel, Strategy: "copy", Hash: hash})
	return target
}

func (e *installedEnv) save(t *testing.T) {
	t.Helper()
	if err := state.Save(state.Path(e.stateRoot), e.meta); err != nil {
		t.Fatalf("save metadata: %v", err)
	}
}

func (e *installedEnv) run(t *testing.T, stdin string, extra ...string) (string, error) {
	t.Helper()
	cmd := cli.NewRootCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if stdin != "" {
		cmd.SetIn(strings.NewReader(stdin))
	}
	args := append([]string{"uninstall", "--home", e.home, "--source-root", e.sourceRoot, "--state-root", e.stateRoot}, extra...)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestUninstallDryRunListsPlanAndChangesNothing(t *testing.T) {
	e := newInstalledEnv(t)
	target := e.installSymlink(t, "shell/zshrc", ".zshrc")
	e.save(t)

	out, err := e.run(t, "", "--dry-run")
	if err != nil {
		t.Fatalf("uninstall --dry-run error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Uninstall Plan") || !strings.Contains(out, "remove") || !strings.Contains(out, target) {
		t.Fatalf("dry run did not list the plan:\n%s", out)
	}
	if !strings.Contains(out, "Dry run: no files changed.") {
		t.Fatalf("missing dry run notice:\n%s", out)
	}
	if _, statErr := os.Lstat(target); statErr != nil {
		t.Fatalf("dry run removed the target: %v", statErr)
	}
}

func TestUninstallYesRemovesOwnedTargetsAndPrunes(t *testing.T) {
	e := newInstalledEnv(t)
	link := e.installSymlink(t, "shell/zshrc", ".zshrc")
	copied := e.installCopy(t, "git/gitconfig", ".gitconfig")
	e.save(t)

	out, err := e.run(t, "", "--yes")
	if err != nil {
		t.Fatalf("uninstall --yes error = %v\n%s", err, out)
	}
	for _, target := range []string{link, copied} {
		if _, statErr := os.Lstat(target); !os.IsNotExist(statErr) {
			t.Fatalf("target %s not removed: %v", target, statErr)
		}
	}
	if !strings.Contains(out, "Removed 2 targets.") {
		t.Fatalf("missing removal summary:\n%s", out)
	}
	if _, statErr := os.Lstat(state.Path(e.stateRoot)); !os.IsNotExist(statErr) {
		t.Fatalf("metadata file should be deleted when empty: %v", statErr)
	}
}

func TestUninstallCancelLeavesTargetsUntouched(t *testing.T) {
	e := newInstalledEnv(t)
	target := e.installSymlink(t, "shell/zshrc", ".zshrc")
	e.save(t)

	out, err := e.run(t, "n\n")
	if err != nil {
		t.Fatalf("uninstall cancel error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "Uninstall canceled") {
		t.Fatalf("missing cancel notice:\n%s", out)
	}
	if _, statErr := os.Lstat(target); statErr != nil {
		t.Fatalf("cancel removed the target: %v", statErr)
	}
}

func TestUninstallNoMetadataReports(t *testing.T) {
	e := newInstalledEnv(t)

	out, err := e.run(t, "", "--yes")
	if err != nil {
		t.Fatalf("uninstall without metadata error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "No recorded targets to uninstall") {
		t.Fatalf("missing empty-metadata notice:\n%s", out)
	}
}

func TestUninstallRestoreBackupsReturnsPreInstallContent(t *testing.T) {
	e := newInstalledEnv(t)
	target := filepath.Join(e.home, ".zshrc")

	// Capture the user's original file as a pre-install Backup Set, then install
	// the managed symlink over it, exactly as install would.
	original := "original user content\n"
	if err := os.WriteFile(target, []byte(original), 0o600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	if _, err := backups.CreateSet(e.stateRoot, []string{target}, backups.CreateOptions{Reason: "pre-install conflict protection"}); err != nil {
		t.Fatalf("create backup set: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatalf("remove original: %v", err)
	}
	e.installSymlink(t, "shell/zshrc", ".zshrc")
	e.save(t)

	out, err := e.run(t, "", "--yes", "--restore-backups")
	if err != nil {
		t.Fatalf("uninstall --restore-backups error = %v\n%s", err, out)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored target: %v", err)
	}
	if string(got) != original {
		t.Fatalf("restored content = %q, want %q", got, original)
	}
	if !strings.Contains(out, "Restored") {
		t.Fatalf("missing restore summary:\n%s", out)
	}
}
