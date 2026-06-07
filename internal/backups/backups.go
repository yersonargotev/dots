// Package backups reads centralized Backup Metadata from the dots state root.
package backups

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Metadata is the machine-readable history of Backup Sets created by dots.
type Metadata struct {
	Version int         `json:"version"`
	Sets    []BackupSet `json:"sets"`
}

// BackupSet describes one backup operation and the targets it protected.
type BackupSet struct {
	ID        string   `json:"id"`
	CreatedAt string   `json:"createdAt"`
	Reason    string   `json:"reason"`
	Targets   []string `json:"targets"`
}

// Path returns the centralized Backup Metadata path under a state root.
func Path(stateRoot string) string {
	return filepath.Join(stateRoot, "backups", "metadata.json")
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
