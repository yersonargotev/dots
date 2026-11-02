package deps

import (
	"bytes"
	"context"
	"io"

	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"time"
)

type DependencyMetadata struct {
	Version      int                `json:"version"`
	Dependencies []DependencyRecord `json:"dependencies,omitempty"`
}

type DependencyRecord struct {
	Dependency   string                  `json:"dependency"`
	Provider     string                  `json:"provider"`
	Path         string                  `json:"path"`
	Version      string                  `json:"version,omitempty"`
	URL          string                  `json:"url,omitempty"`
	Artifact     string                  `json:"artifact,omitempty"`
	Digest       string                  `json:"digest,omitempty"`
	Checksum     string                  `json:"checksum,omitempty"`
	Platform     string                  `json:"platform,omitempty"`
	InstalledAt  string                  `json:"installedAt"`
	OperationID  string                  `json:"operation_id,omitempty"`
	Publications []DependencyPublication `json:"publications,omitempty"`
}

type DependencyPublication struct {
	Path     string              `json:"path"`
	Evidence PublicationEvidence `json:"evidence"`
}

func DependencyMetadataPath(stateRoot string) string {
	return filepath.Join(stateRoot, "dependencies.json")
}

func loadLegacyDependencyMetadata(path string) (DependencyMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DependencyMetadata{}, nil
		}
		return DependencyMetadata{}, fmt.Errorf("read dependency installation metadata: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return DependencyMetadata{}, errors.New("null dependency installation metadata")
	}
	if err := validateReceiptJSON(data); err != nil {
		return DependencyMetadata{}, fmt.Errorf("parse dependency installation metadata: %w", err)
	}
	var meta DependencyMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return DependencyMetadata{}, fmt.Errorf("parse dependency installation metadata: %w", err)
	}
	if meta.Version < 0 || meta.Version > 1 {
		return DependencyMetadata{}, fmt.Errorf("unsupported dependency metadata version %d", meta.Version)
	}
	seen := map[string]bool{}
	for _, record := range meta.Dependencies {
		key := record.Dependency + "\x00" + record.Provider + "\x00" + record.Path
		if record.Dependency == "" || record.Provider == "" || record.Path == "" || seen[key] {
			return DependencyMetadata{}, errors.New("invalid or conflicting finalized dependency record")
		}
		seen[key] = true
	}
	return meta, nil
}

func LoadDependencyMetadata(path string) (meta DependencyMetadata, resultErr error) {
	meta, err := loadLegacyDependencyMetadata(path)
	if err != nil {
		return meta, err
	}
	root, err := openReceiptRoot(filepath.Dir(path), false)
	if os.IsNotExist(err) {
		return meta, nil
	}
	if err != nil {
		return meta, err
	}
	defer func() { resultErr = errors.Join(resultErr, root.file.Close()) }()
	operations, err := readJournal(root)
	if err != nil {
		return meta, fmt.Errorf("load dependency recovery evidence: %w", err)
	}
	return projectJournal(meta, operations), nil
}

func SaveDependencyMetadata(path string, meta DependencyMetadata) (resultErr error) {
	root, err := openReceiptRoot(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, root.file.Close()) }()
	lock, err := receiptLock(context.Background(), root.file, "dependencies.lock")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeReceiptLock(lock)) }()
	return saveLegacyDependencyMetadata(root, filepath.Base(path), meta)
}

func saveLegacyDependencyMetadata(root *receiptRoot, name string, meta DependencyMetadata) (resultErr error) {
	if meta.Version == 0 {
		meta.Version = 1
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	id, err := receiptID()
	if err != nil {
		return err
	}
	temp := ".dependencies-" + id
	fd, err := unix.Openat(int(root.file.Fd()), temp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), temp)
	closed := false
	defer func() {
		if !closed {
			resultErr = errors.Join(resultErr, file.Close())
		}
		if err := unix.Unlinkat(int(root.file.Fd()), temp, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return err
	}
	closeErr := file.Close()
	closed = true
	if closeErr != nil {
		return closeErr
	}
	if err := unix.Renameat(int(root.file.Fd()), temp, int(root.file.Fd()), name); err != nil {
		return err
	}
	return root.file.Sync()
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

func RecordDependencyInstallation(stateRoot, home string, action InstallAction) (resultErr error) {
	if action.UserLocal == nil {
		return nil
	}
	root, err := openReceiptRoot(stateRoot, true)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, root.file.Close()) }()
	lock, err := receiptLock(context.Background(), root.file, "dependencies.lock")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, closeReceiptLock(lock)) }()
	meta, err := loadLegacyDependencyMetadata(DependencyMetadataPath(stateRoot))
	if err != nil {
		return err
	}
	meta.Upsert(DependencyRecord{
		Dependency:  action.Dependency,
		Provider:    string(TierUserLocal),
		Path:        UserLocalInstalledPath(home, action),
		Version:     action.UserLocal.Version,
		URL:         action.UserLocal.URL,
		Artifact:    action.UserLocal.Artifact,
		Digest:      action.UserLocal.Digest,
		Checksum:    action.UserLocal.Checksum,
		Platform:    action.UserLocal.Platform,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	})
	return saveLegacyDependencyMetadata(root, "dependencies.json", meta)
}
