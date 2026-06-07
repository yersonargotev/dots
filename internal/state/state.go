// Package state reads and writes the Installation Metadata that records what the
// Dotfiles CLI installed. It is stored as installed.json under the state root
// (default ~/.local/state/dots) and lets dots status detect Drift for copied
// and templated targets where filesystem inspection alone is insufficient.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Metadata is the machine-readable record of installed managed targets.
type Metadata struct {
	Version int      `json:"version"`
	Entries []Record `json:"entries"`
}

// Record describes a single managed target the CLI installed, including the
// source content hash captured at install time so later runs can detect Drift.
type Record struct {
	Target      string `json:"target"`
	Source      string `json:"source"`
	Strategy    string `json:"strategy"`
	Hash        string `json:"hash"`
	InstalledAt string `json:"installedAt"`
}

// Path returns the location of installed.json under a state root.
func Path(stateRoot string) string {
	return filepath.Join(stateRoot, "installed.json")
}

// Load reads Installation Metadata from path. A missing file is not an error:
// it simply means nothing has been recorded yet, so an empty Metadata is
// returned.
func Load(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Metadata{}, nil
		}
		return Metadata{}, fmt.Errorf("read installation metadata: %w", err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("parse installation metadata: %w", err)
	}
	return meta, nil
}

// Save writes Installation Metadata to path, creating the state directory if
// needed.
func Save(path string, meta Metadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode installation metadata: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write installation metadata: %w", err)
	}
	return nil
}

// FindByTarget returns the record for a resolved target path, if one exists.
func (m Metadata) FindByTarget(target string) (Record, bool) {
	for _, r := range m.Entries {
		if r.Target == target {
			return r, true
		}
	}
	return Record{}, false
}

// HashFile returns the hex-encoded SHA-256 of a file's content.
func HashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open source for hashing %s: %w", path, err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash source %s: %w", path, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
