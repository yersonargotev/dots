package deps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type DependencyMetadata struct {
	Version      int                `json:"version"`
	Dependencies []DependencyRecord `json:"dependencies,omitempty"`
}

type DependencyRecord struct {
	Dependency  string `json:"dependency"`
	Provider    string `json:"provider"`
	Path        string `json:"path"`
	Version     string `json:"version,omitempty"`
	Checksum    string `json:"checksum,omitempty"`
	InstalledAt string `json:"installedAt"`
}

func DependencyMetadataPath(stateRoot string) string {
	return filepath.Join(stateRoot, "dependencies.json")
}

func LoadDependencyMetadata(path string) (DependencyMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DependencyMetadata{}, nil
		}
		return DependencyMetadata{}, fmt.Errorf("read dependency installation metadata: %w", err)
	}
	var meta DependencyMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return DependencyMetadata{}, fmt.Errorf("parse dependency installation metadata: %w", err)
	}
	return meta, nil
}

func SaveDependencyMetadata(path string, meta DependencyMetadata) error {
	if meta.Version == 0 {
		meta.Version = 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dependency metadata directory: %w", err)
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dependency installation metadata: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write dependency installation metadata: %w", err)
	}
	return nil
}

func (m *DependencyMetadata) Upsert(rec DependencyRecord) {
	for i := range m.Dependencies {
		if m.Dependencies[i].Dependency == rec.Dependency && m.Dependencies[i].Provider == rec.Provider {
			m.Dependencies[i] = rec
			return
		}
	}
	m.Dependencies = append(m.Dependencies, rec)
}

func UserLocalInstalledPath(home string, action InstallAction) string {
	if action.UserLocal == nil {
		return ""
	}
	return filepath.Join(home, ".local", "bin", action.UserLocal.Command)
}

func RecordDependencyInstallation(stateRoot, home string, action InstallAction) error {
	if action.UserLocal == nil {
		return nil
	}
	meta, err := LoadDependencyMetadata(DependencyMetadataPath(stateRoot))
	if err != nil {
		return err
	}
	meta.Upsert(DependencyRecord{
		Dependency:  action.Dependency,
		Provider:    string(TierUserLocal),
		Path:        UserLocalInstalledPath(home, action),
		Version:     action.UserLocal.Version,
		Checksum:    action.UserLocal.Checksum,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	})
	return SaveDependencyMetadata(DependencyMetadataPath(stateRoot), meta)
}
