package deps

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
)

const receiptJournal = "dependencies.d"

// UserLocalSelection binds recovery to the selected declaration without resolving
// a new artifact. Versions and integrity data always come from captured authority.
type UserLocalSelection struct {
	Dependency  string `json:"dependency"`
	Kind        string `json:"kind"`
	Recipe      string `json:"recipe"`
	Platform    string `json:"platform"`
	Requirement string `json:"requirement"`
	Probes      string `json:"probes"`
}

type publicationUnit struct {
	Source   string              `json:"source"`
	Target   string              `json:"target"`
	Evidence PublicationEvidence `json:"evidence"`
}

type capturedReceipt struct {
	Format            int                            `json:"format"`
	ID                string                         `json:"id"`
	Previous          string                         `json:"previous,omitempty"`
	Home              string                         `json:"home"`
	State             string                         `json:"state"`
	HomeIdentity      PublicationEvidence            `json:"home_identity"`
	StateLockIdentity PublicationEvidence            `json:"state_lock_identity"`
	StateIdentity     PublicationEvidence            `json:"state_identity"`
	Parents           map[string]PublicationEvidence `json:"parents"`
	Selection         UserLocalSelection             `json:"selection"`
	Action            InstallAction                  `json:"action"`
}

type readyReceipt struct {
	Parents  map[string]PublicationEvidence `json:"parents"`
	ID       string                         `json:"id"`
	Captured string                         `json:"captured"`
	Units    []publicationUnit              `json:"units"`
}
type finalReceipt struct {
	ID     string           `json:"id"`
	Ready  string           `json:"ready"`
	Record DependencyRecord `json:"record"`
}
type receiptMarker struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}
type journalOperation struct {
	Captured capturedReceipt
	Ready    *readyReceipt
	Final    *finalReceipt
	Complete bool
	Progress map[int]bool
}

func receiptDigest(value any) string {
	data := mustReceiptJSON(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func receiptID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("create receipt operation identity: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func validReceiptID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && strings.ToLower(id) == id
}

func readReceiptJSON(root *receiptRoot, path string, value any) (found bool, resultErr error) {
	parent, name, err := root.parent(path, false)
	if err != nil {
		if errors.Is(err, unix.ENOENT) {
			return false, nil
		}
		return false, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || !exactPrivateMode(info.Mode(), 0600) || info.Size() > 32<<20 {
		return false, fmt.Errorf("invalid receipt file %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, (32<<20)+1))
	if err != nil {
		return false, err
	}
	if len(data) > 32<<20 {
		return false, errors.New("receipt exceeds size limit")
	}
	if err := validateReceiptJSON(data); err != nil {
		return false, fmt.Errorf("parse receipt %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return false, fmt.Errorf("parse receipt %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("trailing receipt data in %s", path)
	}
	return true, nil
}

// writeReceipt publishes an immutable, fully synced file without replacing any
// named state. Replays accept only exactly equal captured data.
func (i *UserLocalInstaller) writeReceipt(root *receiptRoot, path string, value any) (resultErr error) {
	parent, name, err := root.parent(path, false)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	phase := strings.TrimSuffix(name, ".json")
	if err := i.check(phase + ":before"); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > 32<<20 {
		return errors.New("receipt exceeds size limit")
	}
	id, err := receiptID()
	if err != nil {
		return err
	}
	tempName := ".tmp-" + id
	fd, err := unix.Openat(int(parent.Fd()), tempName, unix.O_CREAT|unix.O_EXCL|unix.O_WRONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), tempName)
	closed := false
	defer func() {
		if !closed {
			resultErr = errors.Join(resultErr, f.Close())
		}
		if err := unix.Unlinkat(int(parent.Fd()), tempName, 0); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("clean receipt temporary file: %w", err))
		}
	}()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if err := i.check(phase + ":write"); err != nil {
		return err
	}
	write := f.Write
	if i.write != nil {
		write = func(data []byte) (int, error) { return i.write(f, data) }
	}
	n, err := write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err := i.check(phase + ":sync"); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	closeErr := f.Close()
	closed = true
	if closeErr != nil {
		return closeErr
	}
	if err := i.check(phase + ":close"); err != nil {
		return err
	}
	if err := i.check(phase + ":link"); err != nil {
		return err
	}
	if i.active != nil {
		if err := i.active.validateLocks(); err != nil {
			return err
		}
	}
	if err := unix.Linkat(int(parent.Fd()), tempName, int(parent.Fd()), name, 0); err != nil {
		if !errors.Is(err, unix.EEXIST) {
			return err
		}
		var existing json.RawMessage
		found, readErr := readReceiptJSON(root, path, &existing)
		if readErr != nil {
			return readErr
		}
		var expected json.RawMessage = data
		var a, b any
		if !found || json.Unmarshal(existing, &a) != nil || json.Unmarshal(expected, &b) != nil || !reflect.DeepEqual(a, b) {
			return fmt.Errorf("conflicting immutable receipt %s", path)
		}
	}
	if err := i.check(phase + ":dir-sync"); err != nil {
		return err
	}
	return parent.Sync()
}

func operationPath(id, name string) string { return filepath.Join(receiptJournal, id, name+".json") }

func readJournal(root *receiptRoot) (operations []journalOperation, resultErr error) {
	directory, err := openReceiptDir(root.file, receiptJournal, false)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, directory.Close()) }()
	info, err := directory.Stat()
	if err != nil {
		return nil, err
	}
	if !exactPrivateMode(info.Mode(), 0700) {
		return nil, errors.New("private journal permissions changed")
	}
	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].Name() < entries[b].Name() })
	for _, entry := range entries {
		id := entry.Name()
		if !validReceiptID(id) || !entry.IsDir() {
			return nil, fmt.Errorf("invalid receipt operation %s", id)
		}
		op := journalOperation{Progress: map[int]bool{}}
		found, err := readReceiptJSON(root, operationPath(id, "captured"), &op.Captured)
		if err != nil {
			return nil, err
		}
		if !found || op.Captured.ID != id || op.Captured.Format != 1 {
			return nil, fmt.Errorf("missing or conflicting pending authority %s", id)
		}
		if err := validateCaptured(op.Captured); err != nil {
			return nil, err
		}
		var ready readyReceipt
		found, err = readReceiptJSON(root, operationPath(id, "ready"), &ready)
		if err != nil {
			return nil, err
		}
		if found {
			if ready.ID != id || ready.Captured != receiptDigest(op.Captured) || len(ready.Units) == 0 {
				return nil, fmt.Errorf("invalid ready receipt %s", id)
			}
			if err := validateUnits(op.Captured, ready.Units); err != nil {
				return nil, err
			}
			if err := validateSourceParents(op.Captured, ready); err != nil {
				return nil, err
			}
			op.Ready = &ready
			for n, unit := range ready.Units {
				var marker receiptMarker
				exists, err := readReceiptJSON(root, operationPath(id, fmt.Sprintf("unit-%d", n)), &marker)
				if err != nil {
					return nil, err
				}
				if exists {
					if marker.ID != id || marker.Digest != receiptDigest(unit) {
						return nil, fmt.Errorf("conflicting publication marker %s", id)
					}
					op.Progress[n] = true
				}
			}
		}
		var final finalReceipt
		found, err = readReceiptJSON(root, operationPath(id, "final"), &final)
		if err != nil {
			return nil, err
		}
		if found {
			if op.Ready == nil || final.ID != id || final.Ready != receiptDigest(*op.Ready) || !allProgress(op) || !validFinal(op.Captured, *op.Ready, final.Record) {
				return nil, fmt.Errorf("conflicting final receipt %s", id)
			}
			op.Final = &final
		}
		var complete receiptMarker
		found, err = readReceiptJSON(root, operationPath(id, "complete"), &complete)
		if err != nil {
			return nil, err
		}
		if found {
			if op.Final == nil || complete.ID != id || complete.Digest != receiptDigest(*op.Final) {
				return nil, fmt.Errorf("conflicting completion receipt %s", id)
			}
			op.Complete = true
		}
		opDir, err := openReceiptDir(directory, id, false)
		if err != nil {
			return nil, err
		}
		info, statErr := opDir.Stat()
		if statErr != nil || !exactPrivateMode(info.Mode(), 0700) {
			return nil, errors.Join(errors.New("private operation permissions changed"), statErr, opDir.Close())
		}
		files, readErr := opDir.ReadDir(-1)
		closeErr := opDir.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return nil, err
		}
		allowed := map[string]bool{"captured.json": true, "ready.json": op.Ready != nil, "final.json": op.Final != nil, "complete.json": op.Complete}
		if op.Ready != nil {
			for n := range op.Ready.Units {
				allowed[fmt.Sprintf("unit-%d.json", n)] = true
			}
		}
		for _, file := range files {
			if !strings.HasPrefix(file.Name(), ".tmp-") && !allowed[file.Name()] {
				return nil, fmt.Errorf("unknown or conflicting receipt phase %s/%s", id, file.Name())
			}
		}
		operations = append(operations, op)
	}
	if err := validateReceiptChains(operations); err != nil {
		return nil, err
	}
	return operations, nil
}

func allProgress(op journalOperation) bool {
	if op.Ready == nil {
		return false
	}
	for n := range op.Ready.Units {
		if !op.Progress[n] {
			return false
		}
	}
	return true
}

func receiptKey(c capturedReceipt) string { return c.Home + "\x00" + c.Action.Dependency }
func validateReceiptChains(ops []journalOperation) error {
	byID := map[string]journalOperation{}
	next := map[string]string{}
	pending := map[string]string{}
	for _, op := range ops {
		byID[op.Captured.ID] = op
	}
	for _, op := range ops {
		c := op.Captured
		if c.Previous != "" {
			prior, ok := byID[c.Previous]
			if !ok || !prior.Complete || receiptKey(prior.Captured) != receiptKey(c) {
				return fmt.Errorf("conflicting prior receipt %s", c.ID)
			}
		}
		key := receiptKey(c) + "\x00" + c.Previous
		if next[key] != "" {
			return fmt.Errorf("divergent receipt operations %s and %s", next[key], c.ID)
		}
		next[key] = c.ID
		if !op.Complete {
			key := receiptKey(c)
			if pending[key] != "" {
				return fmt.Errorf("multiple pending operations for %s", c.Action.Dependency)
			}
			pending[key] = c.ID
		}
	}
	for _, op := range ops {
		seen := map[string]bool{}
		for id := op.Captured.ID; id != ""; id = byID[id].Captured.Previous {
			if seen[id] {
				return errors.New("cyclic dependency receipt chain")
			}
			seen[id] = true
		}
	}
	return nil
}

func latestCompleted(ops []journalOperation, key string) string {
	used := map[string]bool{}
	for _, op := range ops {
		if op.Complete {
			used[op.Captured.Previous] = true
		}
	}
	for _, op := range ops {
		if op.Complete && receiptKey(op.Captured) == key && !used[op.Captured.ID] {
			return op.Captured.ID
		}
	}
	return ""
}

func projectJournal(meta DependencyMetadata, ops []journalOperation) DependencyMetadata {
	for _, op := range ops {
		if !op.Complete || latestCompleted(ops, receiptKey(op.Captured)) != op.Captured.ID {
			continue
		}
		record := op.Final.Record
		replaced := false
		for n, prior := range meta.Dependencies {
			if prior.Dependency == record.Dependency && prior.Provider == record.Provider && prior.Path == record.Path {
				meta.Dependencies[n] = record
				replaced = true
				break
			}
		}
		if !replaced {
			meta.Dependencies = append(meta.Dependencies, record)
		}
	}
	if len(ops) > 0 && meta.Version == 0 {
		meta.Version = 1
	}
	return meta
}

// pendingSelections is read-only, including when roots or journal do not exist.
func pendingSelections(home, state string, selected []UserLocalSelection) (actions []InstallAction, resultErr error) {
	if home == "" || state == "" {
		return nil, nil
	}
	root, err := openReceiptRoot(state, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, root.file.Close()) }()
	if _, err := loadLegacyDependencyMetadata(DependencyMetadataPath(state)); err != nil {
		return nil, err
	}
	ops, err := readJournal(root)
	if err != nil {
		return nil, err
	}
	absolute, err := filepath.Abs(home)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		if op.Complete || op.Captured.Home != absolute {
			continue
		}
		for _, selection := range selected {
			if selection.Dependency != op.Captured.Action.Dependency {
				continue
			}
			if selection != op.Captured.Selection {
				return nil, fmt.Errorf("pending dependency %q no longer matches selected provider/recipe/platform; preserve %s and restore the declaration before retrying", selection.Dependency, op.Captured.ID)
			}
			if err := root.validate(op.Captured.StateIdentity); err != nil {
				return nil, err
			}
			h, err := openReceiptRoot(home, false)
			if err != nil {
				return nil, err
			}
			checkErr := h.validate(op.Captured.HomeIdentity)
			if err := errors.Join(checkErr, h.file.Close()); err != nil {
				return nil, err
			}
			actions = append(actions, op.Captured.Action)
		}
	}
	return actions, nil
}

func receiptContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// Decode through a strict decoder rather than accepting a valid JSON prefix.
func sameReceipt(a, b any) bool        { return bytes.Equal(mustReceiptJSON(a), mustReceiptJSON(b)) }
func mustReceiptJSON(value any) []byte { data, _ := json.Marshal(value); return data }

// Visible immutable phases may come from a prior directory-sync failure. Make
// them durable under both locks before relying on either presence or absence.
func (i *UserLocalInstaller) syncJournal(s *receiptSession) (resultErr error) {
	if err := s.validateLocks(); err != nil {
		return err
	}
	journal, err := openReceiptDir(s.state.file, receiptJournal, false)
	if errors.Is(err, unix.ENOENT) {
		return s.state.file.Sync()
	}
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, journal.Close()) }()
	info, err := journal.Stat()
	if err != nil {
		return err
	}
	if !exactPrivateMode(info.Mode(), 0700) {
		return errors.New("private journal permissions changed")
	}
	entries, err := journal.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !validReceiptID(entry.Name()) || !entry.IsDir() {
			return errors.New("invalid private receipt operation")
		}
		dir, err := openReceiptDirChecked(journal, entry.Name(), true, 0700, i.check)
		if err != nil {
			return err
		}
		files, readErr := dir.ReadDir(-1)
		if readErr != nil {
			return errors.Join(readErr, dir.Close())
		}
		for _, phase := range files {
			if strings.HasPrefix(phase.Name(), ".tmp-") {
				continue
			}
			if err := i.check("replay:" + phase.Name() + ":file-sync"); err != nil {
				return errors.Join(err, dir.Close())
			}
			fd, err := unix.Openat(int(dir.Fd()), phase.Name(), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
			if err != nil {
				return errors.Join(err, dir.Close())
			}
			file := os.NewFile(uintptr(fd), phase.Name())
			info, statErr := file.Stat()
			if statErr != nil || !info.Mode().IsRegular() || !exactPrivateMode(info.Mode(), 0600) {
				return errors.Join(errors.New("invalid private receipt mode/type"), statErr, file.Close(), dir.Close())
			}
			if err := errors.Join(file.Sync(), file.Close()); err != nil {
				return errors.Join(err, dir.Close())
			}
		}
		if err := i.check("replay:operation-dir-sync"); err != nil {
			return errors.Join(err, dir.Close())
		}
		if err := errors.Join(dir.Sync(), dir.Close()); err != nil {
			return err
		}
	}
	if err := i.check("replay:journal-dir-sync"); err != nil {
		return err
	}
	if err := errors.Join(journal.Sync(), s.state.file.Sync()); err != nil {
		return err
	}
	return s.validateLocks()
}

// Duplicate fields are contradictory authority, even when encoding/json would
// otherwise accept the last value. Validate legacy records the same way.
func validateReceiptJSON(data []byte) error {
	if !utf8.Valid(data) {
		return errors.New("receipt is not valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value func() error
	value = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate or invalid receipt field")
				}
				seen[name] = true
				if err := value(); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := value(); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid receipt delimiter")
		}
		_, err = decoder.Token()
		return err
	}
	if err := value(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing receipt data")
	}
	return nil
}
