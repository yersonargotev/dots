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
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CurrentVersion is the current Installation Metadata schema version.
const CurrentVersion = 3

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

// Record describes a single managed target the CLI installed. Copy-like
// strategies may record a source content hash; symlink records leave Hash empty
// because drift is detected from the link destination.
type Record struct {
	Target      string   `json:"target"`
	Source      string   `json:"source"`
	Sources     []string `json:"sources,omitempty"`
	Strategy    string   `json:"strategy"`
	Hash        string   `json:"hash"`
	InstalledAt string   `json:"installedAt"`
	Profiles    []string `json:"profiles,omitempty"`
	Tags        []string `json:"tags,omitempty"`
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

// Save atomically writes Installation Metadata to path, creating the state
// directory if needed. A failed write leaves any prior metadata file intact.
func Save(path string, meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode installation metadata: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".installed-*.tmp")
	if err != nil {
		return fmt.Errorf("write installation metadata: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("write installation metadata: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write installation metadata: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("write installation metadata: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
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
