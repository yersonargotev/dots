package state

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type cleanupMarker interface {
	CleanupFailure() bool
}

func replaceStateFileOps(t *testing.T, mutate func(*fileOperations)) func() {
	t.Helper()
	original := stateFileOps
	mutate(&stateFileOps)
	restored := false
	restore := func() {
		if restored {
			return
		}
		stateFileOps = original
		restored = true
	}
	t.Cleanup(restore)
	return restore
}

func requireCleanupMarker(t *testing.T, err error) {
	t.Helper()
	var marker cleanupMarker
	if !errors.As(err, &marker) || !marker.CleanupFailure() {
		t.Fatalf("error = %v, want structural cleanup marker", err)
	}
}

type modeFileInfo struct {
	mode os.FileMode
}

func (i modeFileInfo) Name() string       { return "installed.json.lock" }
func (i modeFileInfo) Size() int64        { return 0 }
func (i modeFileInfo) Mode() os.FileMode  { return i.mode }
func (i modeFileInfo) ModTime() time.Time { return time.Time{} }
func (i modeFileInfo) IsDir() bool        { return i.mode.IsDir() }
func (i modeFileInfo) Sys() any           { return nil }

func TestLockMetadataJoinsInitializationCleanupFailures(t *testing.T) {
	primaryErr := errors.New("injected primary failure")
	cleanupErr := errors.New("injected cleanup failure")
	tests := []struct {
		name           string
		primaryContext string
		wantPrimary    bool
		mutate         func(*fileOperations, *int)
	}{
		{
			name:           "new file returns nil",
			primaryContext: "open lock file",
			mutate: func(ops *fileOperations, closeCalls *int) {
				newFileCalls := 0
				ops.newFile = func(fd uintptr, name string) *os.File {
					newFileCalls++
					if newFileCalls == 1 {
						return nil
					}
					return os.NewFile(fd, name)
				}
				ops.closeFD = func(fd int) error {
					(*closeCalls)++
					if err := unix.Close(fd); err != nil {
						return err
					}
					return cleanupErr
				}
			},
		},
		{
			name:           "stat",
			primaryContext: "stat lock file",
			wantPrimary:    true,
			mutate: func(ops *fileOperations, closeCalls *int) {
				statCalls := 0
				ops.stat = func(file *os.File) (os.FileInfo, error) {
					statCalls++
					if statCalls == 1 {
						return nil, primaryErr
					}
					return file.Stat()
				}
				ops.closeFile = closeFileOnceWithFailure(closeCalls, cleanupErr)
			},
		},
		{
			name:           "non regular",
			primaryContext: "is not a regular file",
			mutate: func(ops *fileOperations, closeCalls *int) {
				statCalls := 0
				ops.stat = func(file *os.File) (os.FileInfo, error) {
					statCalls++
					if statCalls == 1 {
						return modeFileInfo{mode: os.ModeDir | 0o700}, nil
					}
					return file.Stat()
				}
				ops.closeFile = closeFileOnceWithFailure(closeCalls, cleanupErr)
			},
		},
		{
			name:           "flock",
			primaryContext: "acquire lock file",
			wantPrimary:    true,
			mutate: func(ops *fileOperations, closeCalls *int) {
				lockCalls := 0
				ops.flock = func(fd, operation int) error {
					if operation == unix.LOCK_EX {
						lockCalls++
						if lockCalls == 1 {
							return primaryErr
						}
					}
					return unix.Flock(fd, operation)
				}
				ops.closeFile = closeFileOnceWithFailure(closeCalls, cleanupErr)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := Path(t.TempDir())
			closeCalls := 0
			restore := replaceStateFileOps(t, func(ops *fileOperations) {
				tt.mutate(ops, &closeCalls)
			})

			locked, err := LockMetadata(path)
			if locked != nil || err == nil {
				t.Fatalf("LockMetadata() = %#v, %v, want nil lock and error", locked, err)
			}
			if !errors.Is(err, cleanupErr) {
				t.Fatalf("error = %v, want cleanup failure", err)
			}
			if tt.wantPrimary && !errors.Is(err, primaryErr) {
				t.Fatalf("error = %v, want primary failure", err)
			}
			if !strings.Contains(err.Error(), tt.primaryContext) || !strings.Contains(err.Error(), path+".lock") {
				t.Fatalf("error = %q, want operation and lock path", err)
			}
			requireCleanupMarker(t, err)
			if closeCalls != 1 {
				t.Fatalf("close calls = %d, want exactly 1", closeCalls)
			}

			restore()
			next, err := LockMetadata(path)
			if err != nil {
				t.Fatalf("LockMetadata() after cleanup error = %v", err)
			}
			if err := next.Close(); err != nil {
				t.Fatalf("Close() after cleanup error = %v", err)
			}
		})
	}
}

func closeFileOnceWithFailure(closeCalls *int, injected error) func(*os.File) error {
	return func(file *os.File) error {
		(*closeCalls)++
		if err := file.Close(); err != nil {
			return err
		}
		if *closeCalls == 1 {
			return injected
		}
		return nil
	}
}

func TestPrimaryFailureIsNotMarkedAsCleanup(t *testing.T) {
	path := Path(t.TempDir())
	primaryErr := errors.New("injected write failure")
	replaceStateFileOps(t, func(ops *fileOperations) {
		ops.write = func(*os.File, []byte) (int, error) { return 0, primaryErr }
	})

	err := save(path, Metadata{Version: CurrentVersion})
	if !errors.Is(err, primaryErr) {
		t.Fatalf("save() error = %v, want primary failure", err)
	}
	var marker cleanupMarker
	if errors.As(err, &marker) {
		t.Fatalf("save() error = %v, pure primary failure must not be marked as cleanup", err)
	}
}

func TestLockedMetadataCloseJoinsIndependentCleanupFailuresOnce(t *testing.T) {
	for _, tt := range []struct {
		name       string
		failUnlock bool
		failClose  bool
	}{
		{name: "unlock", failUnlock: true},
		{name: "close", failClose: true},
		{name: "unlock and close", failUnlock: true, failClose: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := Path(t.TempDir())
			locked, err := LockMetadata(path)
			if err != nil {
				t.Fatal(err)
			}
			unlockErr := errors.New("injected unlock failure")
			closeErr := errors.New("injected close failure")
			unlockCalls := 0
			closeCalls := 0
			restore := replaceStateFileOps(t, func(ops *fileOperations) {
				ops.flock = func(fd, operation int) error {
					if operation != unix.LOCK_UN {
						return unix.Flock(fd, operation)
					}
					unlockCalls++
					if err := unix.Flock(fd, operation); err != nil {
						return err
					}
					if tt.failUnlock {
						return unlockErr
					}
					return nil
				}
				ops.closeFile = func(file *os.File) error {
					closeCalls++
					if err := file.Close(); err != nil {
						return err
					}
					if tt.failClose {
						return closeErr
					}
					return nil
				}
			})

			err = locked.Close()
			if err == nil {
				t.Fatal("Close() error = nil, want injected cleanup failure")
			}
			if tt.failUnlock && (!errors.Is(err, unlockErr) || !strings.Contains(err.Error(), "unlock installation metadata lock "+path+".lock")) {
				t.Fatalf("Close() error = %v, want contextual unlock failure", err)
			}
			if tt.failClose && (!errors.Is(err, closeErr) || !strings.Contains(err.Error(), "close installation metadata lock "+path+".lock")) {
				t.Fatalf("Close() error = %v, want contextual close failure", err)
			}
			requireCleanupMarker(t, err)
			if unlockCalls != 1 || closeCalls != 1 {
				t.Fatalf("cleanup calls = unlock %d, close %d, want exactly 1 each", unlockCalls, closeCalls)
			}
			if err := locked.Close(); err != nil {
				t.Fatalf("second Close() error = %v, want nil", err)
			}
			if unlockCalls != 1 || closeCalls != 1 {
				t.Fatalf("cleanup calls after second Close = unlock %d, close %d, want unchanged", unlockCalls, closeCalls)
			}

			restore()
			next, err := LockMetadata(path)
			if err != nil {
				t.Fatalf("LockMetadata() after real release = %v", err)
			}
			if err := next.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSaveTempPrimaryFailureJoinsCloseAndPreservesPreviousMetadata(t *testing.T) {
	for _, operation := range []string{"chmod", "write"} {
		t.Run(operation, func(t *testing.T) {
			path := Path(t.TempDir())
			previous := []byte("{\"version\":7,\"entries\":[]}\n")
			if err := os.WriteFile(path, previous, 0o600); err != nil {
				t.Fatal(err)
			}
			primaryErr := errors.New("injected " + operation + " failure")
			closeErr := errors.New("injected temp close failure")
			closeCalls := 0
			restore := replaceStateFileOps(t, func(ops *fileOperations) {
				if operation == "chmod" {
					ops.chmod = func(*os.File, os.FileMode) error { return primaryErr }
				} else {
					ops.write = func(*os.File, []byte) (int, error) { return 0, primaryErr }
				}
				ops.closeFile = func(file *os.File) error {
					closeCalls++
					if err := file.Close(); err != nil {
						return err
					}
					return closeErr
				}
			})

			err := save(path, Metadata{Version: CurrentVersion})
			if !errors.Is(err, primaryErr) || !errors.Is(err, closeErr) {
				t.Fatalf("save() error = %v, want primary and close failures", err)
			}
			if !strings.Contains(err.Error(), operation+" installation metadata temporary file ") || !strings.Contains(err.Error(), "close installation metadata temporary file ") {
				t.Fatalf("save() error = %q, want lowercase operation and temp path contexts", err)
			}
			requireCleanupMarker(t, err)
			if closeCalls != 1 {
				t.Fatalf("temp close calls = %d, want exactly 1", closeCalls)
			}
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(got) != string(previous) {
				t.Fatalf("installed metadata = %q, want prior bytes %q", got, previous)
			}

			restore()
			if err := save(path, Metadata{Version: CurrentVersion}); err != nil {
				t.Fatalf("save() rerun error = %v", err)
			}
		})
	}
}

func TestSaveTempPrimaryFailureJoinsRemoveCleanup(t *testing.T) {
	path := Path(t.TempDir())
	previous := []byte("{\"version\":7,\"entries\":[]}\n")
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	primaryErr := errors.New("injected write failure")
	removeErr := errors.New("injected remove failure")
	removeCalls := 0
	restore := replaceStateFileOps(t, func(ops *fileOperations) {
		ops.write = func(*os.File, []byte) (int, error) { return 0, primaryErr }
		originalRemove := ops.remove
		ops.remove = func(path string) error {
			removeCalls++
			if err := originalRemove(path); err != nil {
				return err
			}
			return removeErr
		}
	})

	err := save(path, Metadata{Version: CurrentVersion})
	if !errors.Is(err, primaryErr) || !errors.Is(err, removeErr) {
		t.Fatalf("save() error = %v, want primary and remove failures", err)
	}
	if !strings.Contains(err.Error(), "write installation metadata temporary file ") || !strings.Contains(err.Error(), "remove installation metadata temporary file ") {
		t.Fatalf("save() error = %q, want lowercase operation and temp path contexts", err)
	}
	requireCleanupMarker(t, err)
	if removeCalls != 1 {
		t.Fatalf("remove calls = %d, want exactly 1", removeCalls)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(previous) {
		t.Fatalf("installed metadata = %q, want prior bytes %q", got, previous)
	}

	restore()
	if err := save(path, Metadata{Version: CurrentVersion}); err != nil {
		t.Fatalf("save() rerun error = %v", err)
	}
}

func TestSaveTempCloseFailureBeforeRenamePreservesPreviousMetadata(t *testing.T) {
	path := Path(t.TempDir())
	previous := []byte("{\"version\":7,\"entries\":[]}\n")
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	closeErr := errors.New("injected temp close failure")
	closeCalls := 0
	replaceStateFileOps(t, func(ops *fileOperations) {
		ops.closeFile = func(file *os.File) error {
			closeCalls++
			if err := file.Close(); err != nil {
				return err
			}
			return closeErr
		}
	})

	err := save(path, Metadata{Version: CurrentVersion})
	if !errors.Is(err, closeErr) || !strings.Contains(err.Error(), "close installation metadata temporary file ") {
		t.Fatalf("save() error = %v, want contextual close failure", err)
	}
	requireCleanupMarker(t, err)
	if closeCalls != 1 {
		t.Fatalf("temp close calls = %d, want exactly 1", closeCalls)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(previous) {
		t.Fatalf("installed metadata = %q, want prior bytes %q", got, previous)
	}
}

func TestSaveJoinsPrimaryAndLockCleanupFailuresOnce(t *testing.T) {
	path := Path(t.TempDir())
	previous := []byte("{\"version\":7,\"entries\":[]}\n")
	if err := os.WriteFile(path, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	writeErr := errors.New("injected write failure")
	lockCloseErr := errors.New("injected lock close failure")
	tempCloseCalls := 0
	lockCloseCalls := 0
	restore := replaceStateFileOps(t, func(ops *fileOperations) {
		ops.write = func(*os.File, []byte) (int, error) { return 0, writeErr }
		ops.closeFile = func(file *os.File) error {
			if strings.HasSuffix(file.Name(), ".lock") {
				lockCloseCalls++
				if err := file.Close(); err != nil {
					return err
				}
				return lockCloseErr
			}
			tempCloseCalls++
			return file.Close()
		}
	})

	err := Save(path, Metadata{Version: CurrentVersion})
	if !errors.Is(err, writeErr) || !errors.Is(err, lockCloseErr) {
		t.Fatalf("Save() error = %v, want write and lock close failures", err)
	}
	requireCleanupMarker(t, err)
	if tempCloseCalls != 1 || lockCloseCalls != 1 {
		t.Fatalf("close calls = temp %d, lock %d, want exactly 1 each", tempCloseCalls, lockCloseCalls)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(previous) {
		t.Fatalf("installed metadata = %q, want prior bytes %q", got, previous)
	}

	restore()
	if err := Save(path, Metadata{Version: CurrentVersion}); err != nil {
		t.Fatalf("Save() rerun error = %v", err)
	}
}

func TestUpdateJoinsCallbackAndLockCleanupFailuresOnce(t *testing.T) {
	path := Path(t.TempDir())
	if err := Save(path, Metadata{Version: CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	updateErr := errors.New("injected update failure")
	lockCloseErr := errors.New("injected lock close failure")
	closeCalls := 0
	restore := replaceStateFileOps(t, func(ops *fileOperations) {
		ops.closeFile = func(file *os.File) error {
			closeCalls++
			if err := file.Close(); err != nil {
				return err
			}
			return lockCloseErr
		}
	})

	err := Update(path, func(*Metadata) error { return updateErr })
	if !errors.Is(err, updateErr) || !errors.Is(err, lockCloseErr) {
		t.Fatalf("Update() error = %v, want callback and lock close failures", err)
	}
	requireCleanupMarker(t, err)
	if closeCalls != 1 {
		t.Fatalf("lock close calls = %d, want exactly 1", closeCalls)
	}

	restore()
	if err := Update(path, func(meta *Metadata) error {
		meta.Entries = append(meta.Entries, Record{Target: "/rerun"})
		return nil
	}); err != nil {
		t.Fatalf("Update() rerun error = %v", err)
	}
}

func TestSaveReturnsLockCleanupFailuresAfterDurableMetadata(t *testing.T) {
	path := Path(t.TempDir())
	want := Metadata{Version: CurrentVersion, Entries: []Record{{Target: "/durable", Strategy: "copy"}}}
	unlockErr := errors.New("injected unlock failure")
	closeErr := errors.New("injected close failure")
	unlockCalls := 0
	lockCloseCalls := 0
	tempCloseCalls := 0
	restore := replaceStateFileOps(t, func(ops *fileOperations) {
		ops.flock = func(fd, operation int) error {
			if operation != unix.LOCK_UN {
				return unix.Flock(fd, operation)
			}
			unlockCalls++
			if err := unix.Flock(fd, operation); err != nil {
				return err
			}
			return unlockErr
		}
		ops.closeFile = func(file *os.File) error {
			if strings.HasSuffix(file.Name(), ".lock") {
				lockCloseCalls++
				if err := file.Close(); err != nil {
					return err
				}
				return closeErr
			}
			tempCloseCalls++
			return file.Close()
		}
	})

	err := Save(path, want)
	if !errors.Is(err, unlockErr) || !errors.Is(err, closeErr) {
		t.Fatalf("Save() error = %v, want unlock and close failures", err)
	}
	requireCleanupMarker(t, err)
	if unlockCalls != 1 || lockCloseCalls != 1 || tempCloseCalls != 1 {
		t.Fatalf("cleanup calls = unlock %d, lock close %d, temp close %d, want 1 each", unlockCalls, lockCloseCalls, tempCloseCalls)
	}
	wantBytes, marshalErr := json.MarshalIndent(want, "", "  ")
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	wantBytes = append(wantBytes, '\n')
	gotBytes, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Fatalf("durable metadata = %q, want exact bytes %q", gotBytes, wantBytes)
	}

	restore()
	locked, err := LockMetadata(path)
	if err != nil {
		t.Fatalf("LockMetadata() after durable cleanup error = %v", err)
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	next := Metadata{Version: CurrentVersion, Entries: []Record{{Target: "/rerun", Strategy: "copy"}}}
	if err := Save(path, next); err != nil {
		t.Fatalf("Save() rerun error = %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Entries) != 1 || loaded.Entries[0].Target != "/rerun" {
		t.Fatalf("metadata after rerun = %#v, want rerun record", loaded)
	}
}

func TestSaveShortWriteJoinsTempCloseFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "installed.json")
	closeErr := errors.New("injected close failure")
	closeCalls := 0
	replaceStateFileOps(t, func(ops *fileOperations) {
		ops.write = func(*os.File, []byte) (int, error) { return 0, nil }
		ops.closeFile = func(file *os.File) error {
			closeCalls++
			if err := file.Close(); err != nil {
				return err
			}
			return closeErr
		}
	})

	err := save(path, Metadata{Version: CurrentVersion})
	if !errors.Is(err, io.ErrShortWrite) || !errors.Is(err, closeErr) {
		t.Fatalf("save() error = %v, want short write and close failures", err)
	}
	if closeCalls != 1 {
		t.Fatalf("temp close calls = %d, want exactly 1", closeCalls)
	}
}
