package install

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yersonargotev/dots/internal/backups"
	"github.com/yersonargotev/dots/internal/plan"
	"github.com/yersonargotev/dots/internal/selectionreconciliation"
	"github.com/yersonargotev/dots/internal/state"
)

func sourceSwitchAction(t *testing.T) (plan.Action, Options) {
	t.Helper()
	home, sourceRoot, stateRoot := t.TempDir(), t.TempDir(), t.TempDir()
	previous := []byte("[ui]\nwidth=38\n")
	live := []byte("[ui]\nwidth=99\n[personal]\nkeep=true\n")
	current := []byte("[ui]\nwidth=26\n")
	target := filepath.Join(home, "target.toml")
	for path, content := range map[string][]byte{target: live, filepath.Join(sourceRoot, "new.toml"): current} {
		if err := os.WriteFile(path, content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	record := state.Record{Target: target, Source: "old.toml", Strategy: "copy", Ownership: "toml-subset", Hash: state.HashBytes(previous), OwnedBytes: previous, Contributions: []state.Contribution{{Source: "old.toml", Ownership: "toml-subset", EvidenceRecorded: true, Hash: state.HashBytes(previous), OwnedBytes: previous}}}
	if err := state.Save(state.Path(stateRoot), state.Metadata{Version: state.CurrentVersion, Entries: []state.Record{record}}); err != nil {
		t.Fatal(err)
	}
	fingerprint, err := state.RecordEvidenceFingerprint(record)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	a := plan.Action{Source: "new.toml", Target: target, Strategy: "copy", Ownership: "toml-subset", Status: plan.StatusConflict, Reason: selectionreconciliation.ReasonOwnedSourceConflict, PreviousContent: previous, PreviousRecordFingerprint: fingerprint, SourceSwitch: &plan.SourceSwitch{TargetIdentity: identity, LiveContent: live, SourceContent: current}, Contributions: []plan.Contribution{{Source: "new.toml", SelectorTags: []string{"new"}}}}
	return a, Options{Home: home, SourceRoot: sourceRoot, StateRoot: stateRoot, ConflictDecisions: map[string]ConflictDecision{target: DecisionReplace}}
}
func switchRead(t *testing.T, path string) []byte {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func switchWrite(t *testing.T, path string, b []byte) {
	t.Helper()
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
}

func TestOwnedSourceSwitchRejectsStaleAuthority(t *testing.T) {
	for _, kind := range []string{"source", "target", "same-byte-target-inode", "target-mode", "record", "receipt", "target-symlink", "source-escape", "source-in-root-symlink", "backup-failure"} {
		t.Run(kind, func(t *testing.T) {
			a, opts := sourceSwitchAction(t)
			source := filepath.Join(opts.SourceRoot, a.Source)
			switch kind {
			case "source":
				switchWrite(t, source, []byte("changed=true\n"))
			case "target":
				switchWrite(t, a.Target, []byte("external=true\n"))
			case "target-mode":
				if err := os.Chmod(a.Target, 0644); err != nil {
					t.Fatal(err)
				}
			case "same-byte-target-inode":
				content := switchRead(t, a.Target)
				if err := os.Rename(a.Target, a.Target+"-old"); err != nil {
					t.Fatal(err)
				}
				switchWrite(t, a.Target, content)
			case "record", "receipt":
				err := state.Update(state.Path(opts.StateRoot), func(m *state.Metadata) error {
					if kind == "record" {
						m.Entries[0].Contributions[0].SelectorTags = []string{"changed"}
					} else {
						m.Entries[0].PendingReconciliation = &state.ReconciliationReceipt{TargetHash: "changed"}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			case "target-symlink":
				if err := os.Remove(a.Target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(source, a.Target); err != nil {
					t.Fatal(err)
				}
			case "source-escape", "source-in-root-symlink":
				dest := filepath.Join(opts.SourceRoot, "in-root")
				if kind == "source-escape" {
					dest = filepath.Join(t.TempDir(), "outside")
				}
				switchWrite(t, dest, a.SourceSwitch.SourceContent)
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if kind == "source-in-root-symlink" {
					dest = filepath.Base(dest)
				}
				if err := os.Symlink(dest, source); err != nil {
					t.Fatal(err)
				}
			case "backup-failure":
				switchWrite(t, filepath.Join(opts.StateRoot, "backups"), []byte("blocked"))
			}
			before := switchRead(t, a.Target)
			metaBefore := switchRead(t, state.Path(opts.StateRoot))
			_, err := ApplyManagedEntries(plan.Plan{Actions: []plan.Action{a}}, opts)
			if kind == "source-in-root-symlink" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatalf("stale %s accepted", kind)
			}
			if !bytes.Equal(before, switchRead(t, a.Target)) || !bytes.Equal(metaBefore, switchRead(t, state.Path(opts.StateRoot))) {
				t.Fatal("stale apply changed target or metadata")
			}
		})
	}
}

func TestOwnedSourceSwitchReceiptFailureCompensatesExactIdentity(t *testing.T) {
	for _, kind := range []string{"restore", "external-edit", "same-byte-new-inode"} {
		t.Run(kind, func(t *testing.T) {
			a, opts := sourceSwitchAction(t)
			before := switchRead(t, a.Target)
			moved := opts.StateRoot + "-moved"
			var postimage []byte
			_, err := applyManagedEntriesWithApply(plan.Plan{Actions: []plan.Action{a}}, opts, func(action plan.Action, source string, opts Options) (managedActionResult, error) {
				result, err := applyManagedAction(action, source, opts)
				if err != nil {
					return result, err
				}
				postimage = append([]byte{}, result.TargetContent...)
				if kind == "external-edit" {
					switchWrite(t, a.Target, []byte("external=true\n"))
				}
				if kind == "same-byte-new-inode" {
					if err := os.Rename(a.Target, a.Target+"-old"); err != nil {
						t.Fatal(err)
					}
					switchWrite(t, a.Target, postimage)
				}
				if err := os.Rename(opts.StateRoot, moved); err != nil {
					t.Fatal(err)
				}
				switchWrite(t, opts.StateRoot, []byte("blocked"))
				return result, nil
			})
			if removeErr := os.Remove(opts.StateRoot); removeErr != nil {
				t.Fatal(removeErr)
			}
			if renameErr := os.Rename(moved, opts.StateRoot); renameErr != nil {
				t.Fatal(renameErr)
			}
			if err == nil || !strings.Contains(err.Error(), "persist reconciliation receipt") {
				t.Fatalf("err=%v", err)
			}
			want := before
			if kind == "external-edit" {
				want = []byte("external=true\n")
			}
			if kind == "same-byte-new-inode" {
				want = postimage
				if !strings.Contains(err.Error(), "changed identity") {
					t.Fatalf("no identity rejection: %v", err)
				}
			}
			if !bytes.Equal(switchRead(t, a.Target), want) {
				t.Fatal("incorrect compensation")
			}
			meta, e := state.Load(state.Path(opts.StateRoot))
			if e != nil {
				t.Fatal(e)
			}
			if meta.Entries[0].PendingReconciliation != nil || meta.Entries[0].Contributions[0].Source != "old.toml" {
				t.Fatal("failed receipt changed evidence")
			}
		})
	}
}

func TestConfinedSourceSwitchBackupRejectsInterleavings(t *testing.T) {
	for _, kind := range []string{"edit", "symlink", "same-byte-inode", "mode"} {
		t.Run(kind, func(t *testing.T) {
			a, opts := sourceSwitchAction(t)
			outside := filepath.Join(t.TempDir(), "private")
			switchWrite(t, outside, []byte("outside sentinel"))
			_, err := updateConfinedRegularFileWithIdentity(a.Target, opts.Home, nil, func(live []byte) ([]byte, bool, error) { return []byte("new target"), true, nil }, func(live []byte, mode os.FileMode) error {
				if kind == "mode" {
					if err := os.Chmod(a.Target, 0644); err != nil {
						return err
					}
				} else if kind == "edit" {
					switchWrite(t, a.Target, []byte("later edit"))
				} else {
					if err := os.Rename(a.Target, a.Target+"-old"); err != nil {
						return err
					}
					if kind == "symlink" {
						if err := os.Symlink(outside, a.Target); err != nil {
							return err
						}
					} else {
						switchWrite(t, a.Target, live)
					}
				}
				_, err := backups.CreateContentSet(opts.StateRoot, a.Target, live, mode, backups.CreateOptions{})
				return err
			})
			if err == nil {
				t.Fatal("interleaved target accepted")
			}
			if string(switchRead(t, outside)) != "outside sentinel" {
				t.Fatal("outside target changed")
			}
			metadata, e := backups.Load(backups.Path(opts.StateRoot))
			if e != nil {
				t.Fatal(e)
			}
			if len(metadata.Sets) != 1 || !bytes.Equal(switchRead(t, backups.FilePath(opts.StateRoot, metadata.Sets[0].ID, 1, a.Target)), a.SourceSwitch.LiveContent) {
				t.Fatal("backup did not capture descriptor preimage")
			}
		})
	}
}

func TestOwnedSourceSwitchCrashBeforeReceipt(t *testing.T) {
	if os.Getenv("DOTS_593_CRASH_CHILD") == "1" {
		a, opts := sourceSwitchAction(t)
		// Report fixture roots to the parent, then terminate after the actual write
		// and before the receipt transaction can save its result.
		_, err := applyManagedEntriesWithApply(plan.Plan{Actions: []plan.Action{a}}, opts, func(action plan.Action, source string, opts Options) (managedActionResult, error) {
			result, err := applyManagedAction(action, source, opts)
			if err != nil {
				return result, err
			}
			fmt.Printf("ROOTS:%s|%s|%s\n", opts.Home, opts.SourceRoot, opts.StateRoot)
			os.Exit(73)
			return result, nil
		})
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestOwnedSourceSwitchCrashBeforeReceipt$")
	command.Env = append(os.Environ(), "DOTS_593_CRASH_CHILD=1")
	out, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 73 {
		t.Fatalf("child=%v %s", err, out)
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "ROOTS:", 2)[1])
	roots := strings.Split(line, "|")
	for _, r := range roots {
		t.Cleanup(func() {
			if err := os.RemoveAll(r); err != nil {
				t.Error(err)
			}
		})
	}
	meta, err := state.Load(state.Path(roots[2]))
	if err != nil {
		t.Fatal(err)
	}
	rec := meta.Entries[0]
	live := switchRead(t, rec.Target)
	if rec.PendingReconciliation != nil || rec.Contributions[0].Source != "old.toml" || !bytes.Contains(live, []byte("width=26")) {
		t.Fatalf("bad crash state: %#v %s", meta, live)
	}
	// Without a receipt this exact desired postimage still needs explicit Replace.
	replacement, err := backups.Load(backups.Path(roots[2]))
	if err != nil || len(replacement.Sets) != 1 {
		t.Fatalf("captured backup missing: %v", err)
	}
}

func TestSourceSwitchRewriteFaultsPreservePreimage(t *testing.T) {
	for _, fault := range []string{"short-write", "sync", "truncate", "seek"} {
		t.Run(fault, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "target")
			before := []byte("previous content")
			switchWrite(t, path, before)
			file, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			err = rewriteOpenedRegularFile(&sourceSwitchFaultFile{File: file, fault: fault}, path, []byte("desired content"), before)
			if e := file.Close(); e != nil {
				t.Fatal(e)
			}
			if err == nil {
				t.Fatal("fault accepted")
			}
			if !bytes.Equal(switchRead(t, path), before) {
				t.Fatal("fault lost preimage")
			}
		})
	}
}

type sourceSwitchFaultFile struct {
	*os.File
	fault string
}

func (f *sourceSwitchFaultFile) Write(b []byte) (int, error) {
	if f.fault == "short-write" {
		f.fault = ""
		return 0, io.ErrShortWrite
	}
	return f.File.Write(b)
}
func (f *sourceSwitchFaultFile) Sync() error {
	if f.fault == "sync" {
		f.fault = ""
		return errors.New("injected sync failure")
	}
	return f.File.Sync()
}
func (f *sourceSwitchFaultFile) Truncate(n int64) error {
	if f.fault == "truncate" {
		f.fault = ""
		return errors.New("injected truncate failure")
	}
	return f.File.Truncate(n)
}
func (f *sourceSwitchFaultFile) Seek(n int64, whence int) (int64, error) {
	if f.fault == "seek" {
		f.fault = ""
		return 0, errors.New("injected seek failure")
	}
	return f.File.Seek(n, whence)
}
