package backups

import (
	"fmt"
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
// files keep their permissions and symlinks keep their destination. The created
// Backup Set is returned so callers can report or restore it later.
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
	if !info.Mode().IsRegular() {
		return fmt.Errorf("backup target %s is not a regular file or symlink", target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read backup target %s: %w", target, err)
	}
	if err := os.WriteFile(backupFile, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write backup target %s: %w", backupFile, err)
	}
	return nil
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
// It fails if any preserved file referenced by the set is missing, so a corrupt
// Backup Set is reported before any target is touched.
func PlanRestore(stateRoot string, set BackupSet) ([]RestoreItem, error) {
	items := make([]RestoreItem, 0, len(set.Targets))
	for i, target := range set.Targets {
		backupFile := FilePath(stateRoot, set.ID, i+1, target)
		if _, err := os.Lstat(backupFile); err != nil {
			return nil, fmt.Errorf("preserved file for %s missing from Backup Set %s: %w", target, set.ID, err)
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

// ApplyRestore writes each preserved file back to its target, replacing any file
// currently at that path. Callers are responsible for backing up overwritten
// targets first via CreateSet; ApplyRestore performs only the write.
func ApplyRestore(items []RestoreItem) error {
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
	// A Backup Set only ever preserves regular files and symlinks, so restoring
	// one must never recursively delete a directory tree that now sits at the
	// target. Refuse a directory and remove only a file or symlink, which fails
	// closed (os.Remove will not delete a non-empty directory).
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
		return fmt.Errorf("preserved file %s is not a regular file or symlink", item.BackupFile)
	}
	data, err := os.ReadFile(item.BackupFile)
	if err != nil {
		return fmt.Errorf("read preserved file %s: %w", item.BackupFile, err)
	}
	if err := os.WriteFile(item.Target, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("restore target %s: %w", item.Target, err)
	}
	return nil
}
