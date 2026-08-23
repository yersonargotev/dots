// Package state reads and writes the Installation Metadata that records what the
// Dotfiles CLI installed. It is stored as installed.json under the state root
// (default ~/.local/state/dots) and lets dots status detect Drift for copied
// and templated targets where filesystem inspection alone is insufficient.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// CurrentVersion is the current Installation Metadata schema version.
const CurrentVersion = 8

type fileOperations struct {
	open       func(string, int, uint32) (int, error)
	newFile    func(uintptr, string) *os.File
	stat       func(*os.File) (os.FileInfo, error)
	flock      func(int, int) error
	closeFD    func(int) error
	closeFile  func(*os.File) error
	createTemp func(string, string) (*os.File, error)
	chmod      func(*os.File, os.FileMode) error
	write      func(*os.File, []byte) (int, error)
	remove     func(string) error
}

var stateFileOps = fileOperations{
	open:       unix.Open,
	newFile:    os.NewFile,
	stat:       func(file *os.File) (os.FileInfo, error) { return file.Stat() },
	flock:      unix.Flock,
	closeFD:    unix.Close,
	closeFile:  func(file *os.File) error { return file.Close() },
	createTemp: os.CreateTemp,
	chmod:      func(file *os.File, mode os.FileMode) error { return file.Chmod(mode) },
	write:      func(file *os.File, data []byte) (int, error) { return file.Write(data) },
	remove:     os.Remove,
}

type cleanupFailure struct {
	err error
}

func (e cleanupFailure) Error() string        { return e.err.Error() }
func (e cleanupFailure) Unwrap() error        { return e.err }
func (e cleanupFailure) CleanupFailure() bool { return true }

func asCleanupFailure(err error) error {
	if err == nil {
		return nil
	}
	return cleanupFailure{err: err}
}

// Metadata is the machine-readable record of installed managed targets.
type Metadata struct {
	Version            int                 `json:"version"`
	Provenance         Provenance          `json:"provenance,omitempty"`
	Entries            []Record            `json:"entries"`
	Provisioners       []ProvisionerRecord `json:"provisioners,omitempty"`
	InstalledSelection *InstalledSelection `json:"installed_selection,omitempty"`
}

// InstalledSelection is the authoritative machine-level install intent. It is
// separate from the historical Profile and Tag evidence on inventory records.
type InstalledSelection struct {
	Profiles     []string   `json:"profiles"`
	ExtraTags    []string   `json:"extra_tags,omitempty"`
	ResolvedTags []string   `json:"resolved_tags"`
	Provenance   Provenance `json:"provenance"`
}

// Provenance records the Source of Truth and dots binary that last updated the
// Installation Metadata. Fields are optional so older metadata remains valid.
type Provenance struct {
	SourceRoot     string `json:"source_root,omitempty"`
	SourceRevision string `json:"source_revision,omitempty"`
	DotsVersion    string `json:"dots_version,omitempty"`
	RecordedAt     string `json:"recorded_at,omitempty"`
}

func (p Provenance) Empty() bool {
	return p.SourceRoot == "" && p.SourceRevision == "" && p.DotsVersion == "" && p.RecordedAt == ""
}

// Record describes a single managed target the CLI installed. Version 4 records
// explicit whole or partial Ownership; version 5 adds opaque seeded-baseline
// evidence, version 6 adds opaque byte contribution evidence for TOML subset
// and marked-block ownership, version 7 attributes exact ownership evidence to
// each Source of Truth contribution, and version 8 adds an exact recovery
// receipt for an applied reconciliation whose terminal metadata commit has not
// completed. An empty Ownership is legacy and grants no force-removal authority.
// Copy-like strategies may record a source content hash; symlink records leave
// Hash empty because drift is detected from the link destination.
type Record struct {
	Target       string          `json:"target"`
	Source       string          `json:"source"`
	Sources      []string        `json:"sources,omitempty"`
	Strategy     string          `json:"strategy"`
	Ownership    string          `json:"ownership,omitempty"`
	OwnedContent json.RawMessage `json:"owned_content,omitempty"`
	// OwnedBytes is the exact opaque contribution for non-JSON partial ownership,
	// including TOML subset and marked-block entries.
	OwnedBytes []byte `json:"owned_bytes,omitempty"`
	// SeededBaseline is the exact opaque Source of Truth baseline last applied
	// to Seeded Runtime State. []byte uses JSON base64 encoding, so the state
	// itself need not be JSON.
	SeededBaseline []byte         `json:"seeded_baseline,omitempty"`
	Contributions  []Contribution `json:"contributions,omitempty"`
	// PendingReconciliation proves the exact live bytes and selected sources
	// produced by dots before a later terminal step failed. It never replaces
	// committed contribution evidence or Installed Selection.
	PendingReconciliation *ReconciliationReceipt `json:"pending_reconciliation,omitempty"`
	Hash                  string                 `json:"hash"`
	InstalledAt           string                 `json:"installedAt"`
	Profiles              []string               `json:"profiles,omitempty"`
	Tags                  []string               `json:"tags,omitempty"`
}

// ReconciliationReceipt is a recovery-only proof for a previously applied
// forward reconciliation. Source hashes bind the receipt to the same selected
// Source of Truth inputs; TargetHash rejects any later external mutation.
type ReconciliationReceipt struct {
	TargetHash   string   `json:"target_hash"`
	Sources      []string `json:"sources"`
	SourceHashes []string `json:"source_hashes"`
	Strategy     string   `json:"strategy"`
	Ownership    string   `json:"ownership"`
}

// Clone returns an independent receipt snapshot for compare-and-set checks.
func (r *ReconciliationReceipt) Clone() *ReconciliationReceipt {
	if r == nil {
		return nil
	}
	cloned := *r
	cloned.Sources = append([]string(nil), r.Sources...)
	cloned.SourceHashes = append([]string(nil), r.SourceHashes...)
	return &cloned
}

// Contribution records the exact ownership evidence attributable to one
// selected Source of Truth contribution. SelectorTags identifies the selected
// declarative Tags that caused this source to contribute. EvidenceRecorded
// distinguishes exact empty evidence from an incomplete attributed record.
// Records continue to retain target-wide compatibility fields for older
// metadata consumers.
type Contribution struct {
	Source           string          `json:"source"`
	SelectorTags     []string        `json:"selector_tags,omitempty"`
	Ownership        string          `json:"ownership"`
	EvidenceRecorded bool            `json:"evidence_recorded,omitempty"`
	Hash             string          `json:"hash,omitempty"`
	OwnedContent     json.RawMessage `json:"owned_content,omitempty"`
	OwnedBytes       []byte          `json:"owned_bytes,omitempty"`
	SeededBaseline   []byte          `json:"seeded_baseline,omitempty"`
}

// SourceList returns every Source of Truth contribution for the managed target.
// Legacy metadata records only Source; composed subset targets additionally
// record Sources in deterministic manifest order.
func (r Record) SourceList() []string {
	if len(r.Sources) > 0 {
		return append([]string(nil), r.Sources...)
	}
	if r.Source == "" {
		return nil
	}
	return []string{r.Source}
}

// HasSource reports whether source contributed to this managed target.
func (r Record) HasSource(source string) bool {
	for _, candidate := range r.SourceList() {
		if candidate == source {
			return true
		}
	}
	return false
}

// Clone returns a deep copy suitable for retaining an immutable evidence
// snapshot while other metadata values continue to evolve.
func (r Record) Clone() Record {
	cloned := r
	cloned.Sources = append([]string(nil), r.Sources...)
	cloned.OwnedContent = append([]byte(nil), r.OwnedContent...)
	cloned.OwnedBytes = append([]byte(nil), r.OwnedBytes...)
	cloned.SeededBaseline = append([]byte(nil), r.SeededBaseline...)
	cloned.Profiles = append([]string(nil), r.Profiles...)
	cloned.Tags = append([]string(nil), r.Tags...)
	if r.PendingReconciliation != nil {
		pending := *r.PendingReconciliation
		pending.Sources = append([]string(nil), r.PendingReconciliation.Sources...)
		pending.SourceHashes = append([]string(nil), r.PendingReconciliation.SourceHashes...)
		cloned.PendingReconciliation = &pending
	}
	if r.Contributions != nil {
		cloned.Contributions = make([]Contribution, len(r.Contributions))
		for index, contribution := range r.Contributions {
			cloned.Contributions[index] = contribution
			cloned.Contributions[index].SelectorTags = append([]string(nil), contribution.SelectorTags...)
			cloned.Contributions[index].OwnedContent = append([]byte(nil), contribution.OwnedContent...)
			cloned.Contributions[index].OwnedBytes = append([]byte(nil), contribution.OwnedBytes...)
			cloned.Contributions[index].SeededBaseline = append([]byte(nil), contribution.SeededBaseline...)
		}
	}
	return cloned
}

// PendingReconciliationMatches reports whether a recovery receipt proves that
// the exact live target was produced from the same ordered current sources.
func (r Record) PendingReconciliationMatches(targetData []byte, strategy, ownership string, sources []string, sourceContents [][]byte) bool {
	pending := r.PendingReconciliation
	if pending == nil || pending.TargetHash == "" || pending.Strategy != strategy || pending.Ownership != ownership ||
		!stringSlicesEqual(pending.Sources, sources) || len(pending.SourceHashes) != len(sourceContents) {
		return false
	}
	if HashBytes(targetData) != pending.TargetHash {
		return false
	}
	for i, content := range sourceContents {
		if HashBytes(content) != pending.SourceHashes[i] {
			return false
		}
	}
	return true
}

// RecordEvidenceFingerprint binds an operation to the exact record that
// authorized it while excluding the recovery receipt that operation may add.
// Callers compare the expected receipt separately in the same locked
// transaction.
func RecordEvidenceFingerprint(record Record) (string, error) {
	cloned := record.Clone()
	cloned.PendingReconciliation = nil
	data, err := json.Marshal(cloned)
	if err != nil {
		return "", fmt.Errorf("fingerprint record evidence for %s: %w", record.Target, err)
	}
	return HashBytes(data), nil
}

// ProvisionerRecord describes the last known result for one selected
// Provisioner command in a profile.
type ProvisionerRecord struct {
	Profile    string   `json:"profile"`
	Profiles   []string `json:"profiles,omitempty"`
	Tags       []string `json:"tags,omitempty"`
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

// CaptureProvenance best-effort captures the current Source of Truth revision
// and dots binary version without failing the install path when Git metadata is
// unavailable.
func CaptureProvenance(sourceRoot, dotsVersion string) Provenance {
	prov := Provenance{SourceRoot: sourceRoot, DotsVersion: dotsVersion, RecordedAt: time.Now().UTC().Format(time.RFC3339)}
	if abs, err := filepath.Abs(sourceRoot); err == nil {
		prov.SourceRoot = filepath.Clean(abs)
	}
	cmd := exec.Command("git", "-C", sourceRoot, "rev-parse", "--short", "HEAD")
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.Output(); err == nil {
		prov.SourceRevision = strings.TrimSpace(string(out))
	}
	return prov
}

// Load reads Installation Metadata from path. A missing file is not an error:
// it simply means nothing has been recorded yet, so an empty Metadata is
// returned.
func Load(path string) (Metadata, error) {
	return load(path)
}

func load(path string) (Metadata, error) {
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

// LockedMetadata serializes Installation Metadata read-modify-write operations
// for one installed.json path. Callers must Close it.
type LockedMetadata struct {
	path   string
	file   *os.File
	closed bool
}

// LockMetadata acquires an exclusive advisory lock next to installed.json.
// Every production writer uses this lock so a complete read-modify-write cycle
// cannot overwrite another dots process's entries or receipts.
func LockMetadata(path string) (*LockedMetadata, error) {
	if path == "" {
		return nil, fmt.Errorf("lock installation metadata: path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("lock installation metadata: create state directory: %w", err)
	}
	lockPath := path + ".lock"
	fd, err := stateFileOps.open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock installation metadata: open lock file %s: %w", lockPath, err)
	}
	file := stateFileOps.newFile(uintptr(fd), lockPath)
	if file == nil {
		primaryErr := fmt.Errorf("lock installation metadata: open lock file %s returned no handle", lockPath)
		closeErr := stateFileOps.closeFD(fd)
		if closeErr != nil {
			closeErr = asCleanupFailure(fmt.Errorf("lock installation metadata: close lock file descriptor %s: %w", lockPath, closeErr))
		}
		return nil, errors.Join(primaryErr, closeErr)
	}
	info, err := stateFileOps.stat(file)
	if err != nil {
		primaryErr := fmt.Errorf("lock installation metadata: stat lock file %s: %w", lockPath, err)
		return nil, errors.Join(primaryErr, closeLockFile(file, lockPath))
	}
	if !info.Mode().IsRegular() {
		primaryErr := fmt.Errorf("lock installation metadata: lock path %s is not a regular file", lockPath)
		return nil, errors.Join(primaryErr, closeLockFile(file, lockPath))
	}
	if err := stateFileOps.flock(fd, unix.LOCK_EX); err != nil {
		primaryErr := fmt.Errorf("lock installation metadata: acquire lock file %s: %w", lockPath, err)
		return nil, errors.Join(primaryErr, closeLockFile(file, lockPath))
	}
	return &LockedMetadata{path: path, file: file}, nil
}

func closeLockFile(file *os.File, lockPath string) error {
	if err := stateFileOps.closeFile(file); err != nil {
		return asCleanupFailure(fmt.Errorf("lock installation metadata: close lock file %s: %w", lockPath, err))
	}
	return nil
}

// Load reads the metadata while the exclusive lock is held.
func (l *LockedMetadata) Load() (Metadata, error) {
	if l == nil || l.closed {
		return Metadata{}, fmt.Errorf("load locked installation metadata: lock is closed")
	}
	return load(l.path)
}

// Save atomically writes metadata while the exclusive lock is held.
func (l *LockedMetadata) Save(meta Metadata) error {
	if l == nil || l.closed {
		return fmt.Errorf("save locked installation metadata: lock is closed")
	}
	return save(l.path, meta)
}

// Remove deletes installed.json while the exclusive lock is held.
func (l *LockedMetadata) Remove() error {
	if l == nil || l.closed {
		return fmt.Errorf("remove locked installation metadata: lock is closed")
	}
	if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove installation metadata: %w", err)
	}
	return nil
}

// Close releases the metadata lock.
func (l *LockedMetadata) Close() error {
	if l == nil || l.closed {
		return nil
	}
	l.closed = true
	lockPath := l.path + ".lock"
	unlockErr := stateFileOps.flock(int(l.file.Fd()), unix.LOCK_UN)
	closeErr := stateFileOps.closeFile(l.file)
	if unlockErr != nil {
		unlockErr = asCleanupFailure(fmt.Errorf("unlock installation metadata lock %s: %w", lockPath, unlockErr))
	}
	if closeErr != nil {
		closeErr = asCleanupFailure(fmt.Errorf("close installation metadata lock %s: %w", lockPath, closeErr))
	}
	return errors.Join(unlockErr, closeErr)
}

// Update performs one serialized read-modify-write transaction.
func Update(path string, update func(*Metadata) error) (resultErr error) {
	if update == nil {
		return fmt.Errorf("update installation metadata: callback is required")
	}
	locked, err := LockMetadata(path)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, locked.Close()) }()
	meta, err := locked.Load()
	if err != nil {
		return err
	}
	if err := update(&meta); err != nil {
		return err
	}
	return locked.Save(meta)
}

// Save serializes and atomically writes Installation Metadata to path,
// creating the state directory if needed. A failed write leaves prior metadata
// intact.
func Save(path string, meta Metadata) error {
	locked, err := LockMetadata(path)
	if err != nil {
		return err
	}
	saveErr := locked.Save(meta)
	return errors.Join(saveErr, locked.Close())
}

func save(path string, meta Metadata) (resultErr error) {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode installation metadata: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temp, err := stateFileOps.createTemp(dir, ".installed-*.tmp")
	if err != nil {
		return fmt.Errorf("create installation metadata temporary file for %s: %w", path, err)
	}
	tempPath := temp.Name()
	defer func() {
		if err := stateFileOps.remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, asCleanupFailure(fmt.Errorf("remove installation metadata temporary file %s: %w", tempPath, err)))
		}
	}()
	if err := stateFileOps.chmod(temp, 0o600); err != nil {
		primaryErr := fmt.Errorf("chmod installation metadata temporary file %s: %w", tempPath, err)
		return errors.Join(primaryErr, closeTemporaryMetadata(temp, tempPath))
	}
	if n, err := stateFileOps.write(temp, data); err != nil {
		primaryErr := fmt.Errorf("write installation metadata temporary file %s: %w", tempPath, err)
		return errors.Join(primaryErr, closeTemporaryMetadata(temp, tempPath))
	} else if n != len(data) {
		primaryErr := fmt.Errorf("write installation metadata temporary file %s: %w", tempPath, io.ErrShortWrite)
		return errors.Join(primaryErr, closeTemporaryMetadata(temp, tempPath))
	}
	if err := closeTemporaryMetadata(temp, tempPath); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename installation metadata temporary file %s to %s: %w", tempPath, path, err)
	}
	return nil
}

func closeTemporaryMetadata(temp *os.File, tempPath string) error {
	if err := stateFileOps.closeFile(temp); err != nil {
		return asCleanupFailure(fmt.Errorf("close installation metadata temporary file %s: %w", tempPath, err))
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

// MatchesEntry reports whether Installation Metadata proves that source and
// strategy contribute to the managed target.
func (m Metadata) MatchesEntry(target, source, strategy string) bool {
	rec, ok := m.FindByTarget(target)
	return ok && rec.Strategy == strategy && rec.HasSource(source)
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

	pruned := Metadata{
		Version:            m.Version,
		Provenance:         m.Provenance,
		Provisioners:       append([]ProvisionerRecord(nil), m.Provisioners...),
		InstalledSelection: cloneInstalledSelection(m.InstalledSelection),
	}
	for _, r := range m.Entries {
		if _, ok := drop[r.Target]; ok {
			continue
		}
		pruned.Entries = append(pruned.Entries, r)
	}
	return pruned
}

func cloneInstalledSelection(installed *InstalledSelection) *InstalledSelection {
	if installed == nil {
		return nil
	}
	cloned := *installed
	cloned.Profiles = append([]string(nil), installed.Profiles...)
	cloned.ExtraTags = append([]string(nil), installed.ExtraTags...)
	cloned.ResolvedTags = append([]string(nil), installed.ResolvedTags...)
	return &cloned
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

// HashBytes returns the same content digest used by HashFile without requiring
// a temporary file.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
