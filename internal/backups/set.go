package backups

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// CreateOptions carries the provenance recorded for a new Backup Set.
type CreateOptions struct {
	Reason  string
	Machine string
	Repo    string
}

// MachineName identifies the workstation a Backup Set is created on so restore
// can refuse to write a set captured elsewhere. An unknown hostname is reported
// as an empty string rather than failing, letting callers decide how to treat it.
func MachineName() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

// CreateSet copies each target into a new Backup Set and appends it to the
// Backup Metadata under stateRoot. Targets are preserved by content: regular
// files and directories keep their permissions, and symlinks keep their
// destination. The created Backup Set is returned so callers can report or
// restore it later.
func CreateSet(stateRoot string, targets []string, opts CreateOptions) (BackupSet, error) {
	now := time.Now().UTC()
	set := BackupSet{
		ID:        "backup-" + now.Format("20060102T150405.000000000Z"),
		CreatedAt: now.Format(time.RFC3339),
		Reason:    opts.Reason,
		Machine:   opts.Machine,
		Repo:      opts.Repo,
		Targets:   append([]string(nil), targets...),
	}

	for i, target := range targets {
		backupFile := FilePath(stateRoot, set.ID, i+1, target)
		if err := copyTarget(target, backupFile); err != nil {
			return BackupSet{}, err
		}
	}

	metadataPath := Path(stateRoot)
	meta, err := Load(metadataPath)
	if err != nil {
		return BackupSet{}, err
	}
	meta.Version = metadataVersion
	meta.Sets = append(meta.Sets, set)
	if err := Save(metadataPath, meta); err != nil {
		return BackupSet{}, err
	}
	return set, nil
}

// copyTarget preserves a single target into the Backup Set file location.
func copyTarget(target, backupFile string) error {
	info, err := os.Lstat(target)
	if err != nil {
		return fmt.Errorf("stat backup target %s: %w", target, err)
	}
	if err := os.MkdirAll(filepath.Dir(backupFile), 0o755); err != nil {
		return fmt.Errorf("create Backup Set directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		dest, err := os.Readlink(target)
		if err != nil {
			return fmt.Errorf("read backup symlink %s: %w", target, err)
		}
		if err := os.Symlink(dest, backupFile); err != nil {
			return fmt.Errorf("backup symlink %s: %w", target, err)
		}
		return nil
	}
	if info.IsDir() {
		if err := copyDirectory(target, backupFile, info.Mode().Perm()); err != nil {
			return fmt.Errorf("backup directory %s: %w", target, err)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup target %s is not a regular file, directory, or symlink", target)
	}
	if err := copyRegularFile(target, backupFile, info.Mode().Perm()); err != nil {
		return fmt.Errorf("backup file %s: %w", target, err)
	}
	return nil
}

func copyDirectory(sourceDir, backupDir string, mode os.FileMode) error {
	type directoryMode struct {
		path string
		mode os.FileMode
	}

	if err := os.MkdirAll(backupDir, writableDirectoryMode(mode)); err != nil {
		return err
	}
	if err := os.Chmod(backupDir, writableDirectoryMode(mode)); err != nil {
		return err
	}
	dirs := []directoryMode{{path: backupDir, mode: mode}}
	if err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourceDir {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(backupDir, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkDest, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return err
			}
			return os.Symlink(linkDest, dest)
		}
		if entry.IsDir() {
			dirMode := info.Mode().Perm()
			if err := os.MkdirAll(dest, writableDirectoryMode(dirMode)); err != nil {
				return err
			}
			if err := os.Chmod(dest, writableDirectoryMode(dirMode)); err != nil {
				return err
			}
			dirs = append(dirs, directoryMode{path: dest, mode: dirMode})
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file, directory, or symlink", path)
		}
		return copyRegularFile(path, dest, info.Mode().Perm())
	}); err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := os.Chmod(dirs[i].path, dirs[i].mode); err != nil {
			return err
		}
	}
	return nil
}

func writableDirectoryMode(mode os.FileMode) os.FileMode {
	return mode | 0o700
}

// RemoveDirectoryTree removes a directory tree after making every directory
// writable by its owner. This lets restore/replace operations clean up
// user-owned read-only config directories while still failing closed when chmod
// or removal is denied.
func RemoveDirectoryTree(path string) error {
	if err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		writableMode := writableDirectoryMode(mode)
		if mode == writableMode {
			return nil
		}
		return os.Chmod(current, writableMode)
	}); err != nil {
		return err
	}
	return os.RemoveAll(path)
}

func copyRegularFile(source, dest string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return os.Chmod(dest, mode)
}

// RestoreAction describes what restoring a target will do to the current
// workstation file at that path.
type RestoreAction string

const (
	// RestoreCreate means the target is currently absent and will be created.
	RestoreCreate RestoreAction = "create"
	// RestoreOverwrite means the target currently exists and will be backed up
	// before being replaced with the preserved content.
	RestoreOverwrite RestoreAction = "overwrite"
)

// RestoreItem is one preserved file mapped back to its original target, with the
// action restoring it would take against the current filesystem.
type RestoreItem struct {
	Target     string
	BackupFile string
	Action     RestoreAction
}

// PlanRestore computes, without changing anything, what restoring set would do.
// It fails if any preserved item referenced by the set is missing, so a corrupt
// Backup Set is reported before any target is touched.
func PlanRestore(stateRoot string, set BackupSet) ([]RestoreItem, error) {
	items := make([]RestoreItem, 0, len(set.Targets))
	for i, target := range set.Targets {
		backupFile := FilePath(stateRoot, set.ID, i+1, target)
		if _, err := os.Lstat(backupFile); err != nil {
			return nil, fmt.Errorf("preserved item for %s missing from Backup Set %s: %w", target, set.ID, err)
		}

		action := RestoreCreate
		if _, err := os.Lstat(target); err == nil {
			action = RestoreOverwrite
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat restore target %s: %w", target, err)
		}

		items = append(items, RestoreItem{Target: target, BackupFile: backupFile, Action: action})
	}
	return items, nil
}

// RestoreSafetyReason is the reason recorded on the Backup Set that Restore
// captures for any target it would overwrite, so a restore is never one-way.
const RestoreSafetyReason = "pre-restore safety backup"

// RestoreOptions carries the provenance recorded on the pre-restore safety
// Backup Set taken before overwriting existing targets.
type RestoreOptions struct {
	Machine string
	Repo    string
}

// RestoreResult reports what Restore did: the per-target plan it executed and,
// when any existing target was overwritten, the safety Backup Set it recorded
// first (nil when nothing needed overwriting).
type RestoreResult struct {
	Items     []RestoreItem
	SafetySet *BackupSet
}

// Restore returns the targets recorded in set to their preserved content. It is
// the single safe entry point for writing a restore: it plans against the
// current filesystem, records a pre-restore safety Backup Set for every target
// it would overwrite, and only then writes the preserved files back. Bundling
// the safety backup with the write here makes the "never overwrite without a
// backup" invariant impossible for callers to skip — the write primitive is not
// exported on its own.
func Restore(stateRoot string, set BackupSet, opts RestoreOptions) (RestoreResult, error) {
	items, err := PlanRestore(stateRoot, set)
	if err != nil {
		return RestoreResult{}, err
	}
	result := RestoreResult{Items: items}

	if overwritten := overwriteTargets(items); len(overwritten) > 0 {
		safety, err := CreateSet(stateRoot, overwritten, CreateOptions{
			Reason:  RestoreSafetyReason,
			Machine: opts.Machine,
			Repo:    opts.Repo,
		})
		if err != nil {
			return RestoreResult{}, err
		}
		result.SafetySet = &safety
	}

	if err := applyRestore(items); err != nil {
		return result, err
	}
	return result, nil
}

// overwriteTargets returns the targets whose current content Restore must back
// up before replacing them.
func overwriteTargets(items []RestoreItem) []string {
	targets := make([]string, 0, len(items))
	for _, item := range items {
		if item.Action == RestoreOverwrite {
			targets = append(targets, item.Target)
		}
	}
	return targets
}

// applyRestore writes each preserved item back to its target. It is unexported
// on purpose: callers reach it only through Restore, which takes the safety
// backup first.
func applyRestore(items []RestoreItem) error {
	for _, item := range items {
		if err := restoreItem(item); err != nil {
			return err
		}
	}
	return nil
}

func restoreItem(item RestoreItem) error {
	info, err := os.Lstat(item.BackupFile)
	if err != nil {
		return fmt.Errorf("stat preserved file %s: %w", item.BackupFile, err)
	}
	if err := os.MkdirAll(filepath.Dir(item.Target), 0o755); err != nil {
		return fmt.Errorf("create parent directory for %s: %w", item.Target, err)
	}
	if info.IsDir() {
		if err := removeTargetForDirectoryRestore(item.Target); err != nil {
			return fmt.Errorf("remove current target %s: %w", item.Target, err)
		}
		if err := copyDirectory(item.BackupFile, item.Target, info.Mode().Perm()); err != nil {
			return fmt.Errorf("restore directory %s: %w", item.Target, err)
		}
		return nil
	}
	// A Backup Set can preserve regular files, directories, and symlinks. When
	// restoring a file or symlink, never recursively delete a directory tree that
	// now sits at the target. Refuse a directory and remove only a file or
	// symlink, which fails closed (os.Remove will not delete a non-empty
	// directory).
	if current, err := os.Lstat(item.Target); err == nil {
		if current.IsDir() {
			return fmt.Errorf("refusing to restore over directory %s", item.Target)
		}
		if err := os.Remove(item.Target); err != nil {
			return fmt.Errorf("remove current target %s: %w", item.Target, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat current target %s: %w", item.Target, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		dest, err := os.Readlink(item.BackupFile)
		if err != nil {
			return fmt.Errorf("read preserved symlink %s: %w", item.BackupFile, err)
		}
		if err := os.Symlink(dest, item.Target); err != nil {
			return fmt.Errorf("restore symlink %s: %w", item.Target, err)
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("preserved file %s is not a regular file, directory, or symlink", item.BackupFile)
	}
	if err := copyRegularFile(item.BackupFile, item.Target, info.Mode().Perm()); err != nil {
		return fmt.Errorf("restore target %s: %w", item.Target, err)
	}
	return nil
}

func removeTargetForDirectoryRestore(target string) error {
	current, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if current.IsDir() {
		return RemoveDirectoryTree(target)
	}
	return os.Remove(target)
}
