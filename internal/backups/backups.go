// Package backups reads centralized Backup Metadata from the dots state root.
package backups

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// metadataVersion is the current Backup Metadata schema version.
const metadataVersion = 1

// Metadata is the machine-readable history of Backup Sets created by dots.
type Metadata struct {
	Version int         `json:"version"`
	Sets    []BackupSet `json:"sets"`
}

// BackupSet describes one backup operation and the targets it protected. Machine
// and Repo record the provenance of the backup so restore can refuse to write a
// set captured on a different workstation unless the operator forces it. They are
// optional so Backup Sets recorded before provenance tracking still load.
type BackupSet struct {
	ID        string   `json:"id"`
	CreatedAt string   `json:"createdAt"`
	Reason    string   `json:"reason"`
	Machine   string   `json:"machine,omitempty"`
	Repo      string   `json:"repo,omitempty"`
	Targets   []string `json:"targets"`
}

// Path returns the centralized Backup Metadata path under a state root.
func Path(stateRoot string) string {
	return filepath.Join(stateRoot, "backups", "metadata.json")
}

// SetDir returns the directory holding one Backup Set's preserved files.
func SetDir(stateRoot, id string) string {
	return filepath.Join(stateRoot, "backups", id)
}

// FilePath returns the stored backup file path for the target at the given
// 1-based index within a Backup Set. The index keeps multiple targets that share
// a base name distinct and stable across writes and reads.
func FilePath(stateRoot, id string, index int, target string) string {
	name := fmt.Sprintf("%06d-%s", index, filepath.Base(target))
	return filepath.Join(SetDir(stateRoot, id), "files", name)
}

// FindSet returns the Backup Set with the given ID, if it exists.
func (m Metadata) FindSet(id string) (BackupSet, bool) {
	for _, set := range m.Sets {
		if set.ID == id {
			return set, true
		}
	}
	return BackupSet{}, false
}

// Load reads Backup Metadata from path. Missing metadata means no Backup Sets
// have been recorded yet, so it returns empty Metadata instead of failing.
func Load(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil
		}
		return Metadata{}, fmt.Errorf("read Backup Metadata: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return Metadata{}, nil
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("parse Backup Metadata: %w", err)
	}
	return meta, nil
}

// Save writes Backup Metadata to path, creating parent directories as needed.
func Save(path string, meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Backup Metadata: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Backup Metadata directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write Backup Metadata: %w", err)
	}
	return nil
}
