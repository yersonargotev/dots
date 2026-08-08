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

func TestUninstallForcePreservesModifiedCopyFromLegacyMetadata(t *testing.T) {
	e := newInstalledEnv(t)
	// A global metadata upgrade must not retroactively mark an untouched legacy
	// record as whole-owned; the per-record ownership field is the proof.
	e.meta.Version = state.CurrentVersion
	source := filepath.Join(e.sourceRoot, "configs", "shared.json")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	baseline := []byte(`{"owned":true}`)
	if err := os.WriteFile(source, baseline, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	hash := state.HashBytes(baseline)
	target := filepath.Join(e.home, ".config", "shared.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	content := `{"owned":true,"external":"preserve"}`
	if err := os.WriteFile(target, []byte(content), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}
	e.meta.Entries = append(e.meta.Entries, state.Record{
		Target: target, Source: "configs/shared.json", Strategy: "copy", Hash: hash,
	})
	e.save(t)

	out, err := e.run(t, "", "--yes", "--force")
	if err != nil {
		t.Fatalf("uninstall legacy --force error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "legacy ownership evidence is insufficient") || !strings.Contains(out, "Nothing to remove") {
		t.Fatalf("uninstall did not explain protected legacy target:\n%s", out)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read preserved target: %v", err)
	}
	if string(got) != content {
		t.Fatalf("legacy --force changed target to %s", got)
	}
	meta, err := state.Load(state.Path(e.stateRoot))
	if err != nil {
		t.Fatalf("load metadata: %v", err)
	}
	if _, ok := meta.FindByTarget(target); !ok {
		t.Fatal("legacy --force pruned ownership metadata")
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

// TestUninstallDryRunDoesNotInspectTargetOutsideHome reproduces the review
// finding: a metadata record pointing outside --home must never be inspected or
// shown as remove. The confinement guard classifies it not-owned, so --dry-run
// reports it as not-owned and the out-of-home file is left untouched.
func TestUninstallDryRunDoesNotInspectTargetOutsideHome(t *testing.T) {
	e := newInstalledEnv(t)
	outside := t.TempDir()
	victim := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(victim, []byte("content\n"), 0o600); err != nil {
		t.Fatalf("write victim: %v", err)
	}
	// A copy record whose recorded hash matches the out-of-home file's content,
	// so without the guard it would classify as remove.
	hash, err := state.HashFile(victim)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	e.meta.Entries = append(e.meta.Entries, state.Record{Target: victim, Source: "x/outside", Strategy: "copy", Hash: hash})
	e.save(t)

	out, err := e.run(t, "", "--dry-run")
	if err != nil {
		t.Fatalf("uninstall --dry-run error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "not-owned  "+victim) {
		t.Fatalf("out-of-home target should be reported not-owned:\n%s", out)
	}
	if strings.Contains(out, "remove     "+victim) {
		t.Fatalf("out-of-home target must not be shown as remove:\n%s", out)
	}
	if _, statErr := os.Lstat(victim); statErr != nil {
		t.Fatalf("out-of-home target should be untouched: %v", statErr)
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
