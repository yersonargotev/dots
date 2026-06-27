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
	Version      int                 `json:"version"`
	Entries      []Record            `json:"entries"`
	Provisioners []ProvisionerRecord `json:"provisioners,omitempty"`
}

// Record describes a single managed target the CLI installed. Copy-like
// strategies may record a source content hash; symlink records leave Hash empty
// because drift is detected from the link destination.
type Record struct {
	Target      string `json:"target"`
	Source      string `json:"source"`
	Strategy    string `json:"strategy"`
	Hash        string `json:"hash"`
	InstalledAt string `json:"installedAt"`
}

// ProvisionerRecord describes the last known result for one selected
// Provisioner command in a profile.
type ProvisionerRecord struct {
	Profile    string   `json:"profile"`
	Tool       string   `json:"tool"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	Status     string   `json:"status"`
	Missing    []string `json:"missing,omitempty"`
	LastRunAt  string   `json:"lastRunAt"`
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

// FindProvisioner returns the last result for a selected Provisioner command.
func (m Metadata) FindProvisioner(profile, tool, executable string, args []string) (ProvisionerRecord, bool) {
	for _, r := range m.Provisioners {
		if r.Profile == profile && r.Tool == tool && r.Executable == executable && stringSlicesEqual(r.Args, args) {
			return r, true
		}
	}
	return ProvisionerRecord{}, false
}

// UpsertProvisioner records the latest outcome for a selected Provisioner.
func (m *Metadata) UpsertProvisioner(rec ProvisionerRecord) {
	for i := range m.Provisioners {
		if m.Provisioners[i].Profile == rec.Profile && m.Provisioners[i].Tool == rec.Tool && m.Provisioners[i].Executable == rec.Executable && stringSlicesEqual(m.Provisioners[i].Args, rec.Args) {
			m.Provisioners[i] = rec
			return
		}
	}
	m.Provisioners = append(m.Provisioners, rec)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Remove returns a copy of the Metadata with every record whose Target appears
// in targets dropped. The receiver is left unchanged so callers can prune
// Installation Metadata after a successful uninstall removal without mutating the
// version they loaded. Version and any unmatched records are preserved.
func (m Metadata) Remove(targets ...string) Metadata {
	drop := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		drop[t] = struct{}{}
	}

	pruned := Metadata{Version: m.Version}
	for _, r := range m.Entries {
		if _, ok := drop[r.Target]; ok {
			continue
		}
		pruned.Entries = append(pruned.Entries, r)
	}
	return pruned
}

// HashFile returns the hex-encoded SHA-256 of a regular file's content.
func HashFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("stat source for hashing %s: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("hash source %s: directories are not supported", path)
	}

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
