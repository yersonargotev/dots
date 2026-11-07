package deps

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/yersonargotev/dots/internal/manifest"
	"golang.org/x/sys/unix"
)

// UserLocalInstaller owns the durable publication boundary. Fault and Download
// are per-instance sandbox seams; production callers leave them nil.
type UserLocalInstaller struct {
	Context   context.Context
	Home      string
	StateRoot string
	Fault     func(point string) error
	Download  func(url, checksum string) ([]byte, error)
	write     func(*os.File, []byte) (int, error)
	active    *receiptSession
}

func (i *UserLocalInstaller) check(point string) error {
	if err := receiptContext(i.Context).Err(); err != nil {
		return err
	}
	if i.Fault != nil {
		if err := i.Fault(point); err != nil {
			return fmt.Errorf("user-local %s: %w", point, err)
		}
	}
	return receiptContext(i.Context).Err()
}

type receiptSession struct {
	home, state         *receiptRoot
	homeLock, stateLock *os.File
	homeLockParent      *os.File
}

func (s *receiptSession) close() error {
	var errs []error
	errs = append(errs, closeReceiptLock(s.stateLock), closeReceiptLock(s.homeLock))
	if s.homeLockParent != nil {
		errs = append(errs, s.homeLockParent.Close())
	}
	if s.state != nil {
		errs = append(errs, s.state.file.Close())
	}
	if s.home != nil {
		errs = append(errs, s.home.file.Close())
	}
	return errors.Join(errs...)
}

func (i *UserLocalInstaller) session() (session *receiptSession, resultErr error) {
	if i.Home == "" || i.StateRoot == "" {
		return nil, errors.New("user-local install requires explicit home and state roots")
	}
	if err := i.check("lock"); err != nil {
		return nil, fmt.Errorf("open user-local transaction roots/locks: %w", err)
	}
	s := &receiptSession{}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, s.close())
		}
	}()
	var err error
	s.home, err = openReceiptRootChecked(i.Home, true, i.check)
	if err != nil {
		return nil, fmt.Errorf("open user-local transaction roots/locks: %w", err)
	}
	local, err := openReceiptDirChecked(s.home.file, ".local", true, 0755, i.check)
	if err != nil {
		return nil, fmt.Errorf("open user-local transaction roots/locks: %w", err)
	}
	s.homeLockParent = local
	s.homeLock, err = receiptLockChecked(receiptContext(i.Context), local, ".dots-dependencies.lock", i.check)
	if err != nil {
		return nil, err
	}
	s.state, err = openReceiptRootChecked(i.StateRoot, true, i.check)
	if err != nil {
		return nil, fmt.Errorf("open user-local transaction roots/locks: %w", err)
	}
	s.stateLock, err = receiptLockChecked(receiptContext(i.Context), s.state.file, "dependencies.lock", i.check)
	if err != nil {
		return nil, fmt.Errorf("open user-local transaction roots/locks: %w", err)
	}
	if err := s.validateLocks(); err != nil {
		return nil, err
	}
	return s, nil
}

func identityAt(root *receiptRoot, path string) (identity PublicationEvidence, resultErr error) {
	parent, name, err := root.parent(path, false)
	if err != nil {
		return identity, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	identity, err = namedIdentity(parent, name)
	if err != nil {
		return identity, err
	}
	if identity.Mode&unix.S_IFMT != unix.S_IFDIR && identity.Mode&unix.S_IFMT != unix.S_IFREG {
		return identity, fmt.Errorf("unsafe authority path %s", path)
	}
	return identity, nil
}

func artifactSelection(action InstallAction) UserLocalSelection {
	a := action.UserLocal
	kind := "fixed"
	if a.Digest != "" {
		kind = "rolling"
	}
	probes := action.Probes
	if len(probes) == 0 && action.Probe != "" {
		probes = []string{action.Probe}
	}
	return UserLocalSelection{Dependency: action.Dependency, Kind: kind, Recipe: a.Recipe, Platform: a.Platform, Requirement: action.Requirement, Probes: strings.Join(probes, "\x00")}
}

func selectedUserLocals(selected []manifest.Dependency, opts Options) []UserLocalSelection {
	arch := opts.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	result := make([]UserLocalSelection, 0, len(selected))
	for _, dep := range selected {
		s := UserLocalSelection{Dependency: dep.Name, Platform: platformKey(opts.OS, arch), Requirement: dep.RequirementValue(), Probes: strings.Join(dep.Probes(), "\x00")}
		if dep.UserLocal != nil {
			s.Kind = "fixed"
			s.Recipe = strings.TrimSpace(dep.UserLocal.Recipe)
			if s.Recipe == "" {
				s.Recipe = dep.Name
			}
		}
		if dep.RollingUserLocal != nil {
			s.Kind = "rolling"
			s.Recipe = strings.TrimSpace(dep.RollingUserLocal.Recipe)
		}
		result = append(result, s)
	}
	return result
}

func validComponent(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, "/\\\x00")
}

func validateCaptured(c capturedReceipt) error {
	a := c.Action.UserLocal
	if c.Format != 1 || !validReceiptID(c.ID) || c.Action.Dependency == "" || a == nil || !filepath.IsAbs(c.Home) || !filepath.IsAbs(c.State) {
		return errors.New("invalid captured dependency authority")
	}
	recipe, ok := userLocalRecipes[a.Recipe]
	if !ok || !validComponent(a.Version) || a.Command != recipe.command || a.Layout != recipe.layout || len(a.Checksum) != 64 {
		return fmt.Errorf("invalid captured artifact for %s", c.Action.Dependency)
	}
	if _, err := hex.DecodeString(a.Checksum); err != nil {
		return fmt.Errorf("invalid captured checksum: %w", err)
	}
	if a.URL == "" || a.Platform == "" || c.Selection != artifactSelection(c.Action) || c.Action.Provider != TierUserLocal || c.Action.Status != InstallActionStatusInstallable {
		return errors.New("conflicting captured provider identity")
	}
	destination, installed := userLocalPaths(c.Home, a.Recipe, a.Version, a.Layout, a.Command)
	if a.Destination != destination || a.InstalledPath != installed {
		return errors.New("captured artifact escapes selected home")
	}
	if c.Ordinal == 0 || !validEvidenceIdentity(c.HomeIdentity) || !validEvidenceIdentity(c.StateIdentity) || !validEvidenceIdentity(c.StateLockIdentity) || len(c.Parents) == 0 {
		return errors.New("missing captured root identity")
	}
	for _, name := range authorityParents(*a) {
		if !validEvidenceIdentity(c.Parents[name]) {
			return fmt.Errorf("missing parent authority %s", name)
		}
	}
	return nil
}

func authorityParents(a UserLocalArtifact) []string {
	paths := []string{".local", filepath.Join(".local", "bin"), filepath.Join(".local", ".dots-dependencies.lock")}
	if a.Layout == userLocalLayoutBundle {
		paths = append(paths, filepath.Join(".local", "opt"), filepath.Join(".local", "opt", a.Recipe))
	}
	return paths
}

func unitPaths(c capturedReceipt) []publicationUnit {
	a := c.Action.UserLocal
	recipe := userLocalRecipes[a.Recipe]
	stage := filepath.Join(".local", ".dots-deps-"+c.ID)
	if a.Layout == userLocalLayoutSingle {
		return []publicationUnit{{Source: filepath.Join(stage, "payload", recipe.binaryPath(a.archiveName(), a.Command)), Target: filepath.Join(".local", "bin", a.Command)}}
	}
	destination, _ := filepath.Rel(c.Home, a.Destination)
	units := []publicationUnit{{Source: filepath.Join(stage, "payload"), Target: destination}}
	for _, command := range recipe.links {
		units = append(units, publicationUnit{Source: filepath.Join(stage, "links", command), Target: filepath.Join(".local", "bin", command)})
	}
	return units
}

func validateUnits(c capturedReceipt, units []publicationUnit) error {
	expected := unitPaths(c)
	if len(expected) != len(units) {
		return errors.New("incomplete captured command surface")
	}
	for n, unit := range units {
		if unit.Source != expected[n].Source || unit.Target != expected[n].Target || !validEvidenceIdentity(unit.Evidence) {
			return errors.New("conflicting publication surface")
		}
		typ := unit.Evidence.Mode & unix.S_IFMT
		if c.Action.UserLocal.Layout == userLocalLayoutBundle && n == 0 {
			if typ != unix.S_IFDIR {
				return errors.New("invalid bundle publication")
			}
		} else if c.Action.UserLocal.Layout == userLocalLayoutBundle {
			command := userLocalRecipes[c.Action.UserLocal.Recipe].links[n-1]
			target := filepath.Join(c.Action.UserLocal.Destination, userLocalRecipes[c.Action.UserLocal.Recipe].binaryPath(c.Action.UserLocal.archiveName(), command))
			if typ != unix.S_IFLNK || unit.Evidence.Link != target {
				return errors.New("conflicting captured command link")
			}
		} else if typ != unix.S_IFREG || unit.Evidence.Mode&0111 == 0 {
			return errors.New("invalid executable publication")
		}
	}
	return nil
}

func (s *receiptSession) validateLocks() error {
	return errors.Join(validateHeldLock(s.homeLockParent, ".dots-dependencies.lock", s.homeLock), validateHeldLock(s.state.file, "dependencies.lock", s.stateLock))
}

func (s *receiptSession) validate(c capturedReceipt) error {
	if err := s.validateLocks(); err != nil {
		return err
	}
	if err := validateCapturedRootName(s.home, c.Home, c.HomeIdentity); err != nil {
		return err
	}
	if err := validateCapturedRootName(s.state, c.State, c.StateIdentity); err != nil {
		return err
	}
	heldState, err := fileIdentity(s.stateLock)
	if err != nil {
		return err
	}
	if !sameEvidence(heldState, c.StateLockIdentity) {
		return errors.New("captured state lock identity changed")
	}
	for name, expected := range c.Parents {
		actual, err := identityAt(s.home, name)
		if err != nil {
			return err
		}
		if !sameEvidence(expected, actual) {
			return fmt.Errorf("authority changed at %s", name)
		}
	}
	return nil
}

// RunUserLocal publishes AND finalizes; callers cannot report success between
// those phases. Presence is checked again while the home mutation lock is held.
func (i *UserLocalInstaller) RunUserLocal(action InstallAction, present func() bool) (resultErr error) {
	if err := retiredActionError(action); err != nil {
		return err
	}
	s := i.active
	owned := s == nil
	var err error
	if owned {
		s, err = i.session()
		if err != nil {
			return err
		}
	}
	if owned {
		i.active = s
	}
	defer func() {
		if owned {
			i.active = nil
			resultErr = errors.Join(resultErr, s.close())
		}
		if resultErr != nil {
			resultErr = fmt.Errorf("user-local recovery: %w; preserve pending evidence in %s and external paths; restore captured state or move conflicting paths explicitly before retrying", resultErr, filepath.Join(i.StateRoot, receiptJournal))
		}
	}()
	if err := i.syncJournal(s); err != nil {
		return err
	}
	ops, err := readJournal(s.state)
	if err != nil {
		return err
	}
	if err := rejectRetiredPending(s.home, ops); err != nil {
		return err
	}
	for _, op := range ops {
		if op.Complete || op.Captured.Action.Dependency != action.Dependency {
			continue
		}
		matches, err := homeScopeMatchesChecked(s.home, op.Captured, i.check)
		if err != nil {
			return err
		}
		if matches {
			if artifactSelection(action) != op.Captured.Selection {
				return errors.New("pending operation conflicts with selected provider identity")
			}
			return i.resume(s, op)
		}
	}
	if present != nil && present() {
		return nil
	}
	if action.UserLocal == nil {
		return errors.New("missing user-local artifact")
	}
	if err := i.check("identity"); err != nil {
		return err
	}
	ordinal, err := nextReceiptOrdinal(ops)
	if err != nil {
		return err
	}
	artifact := *action.UserLocal
	action.UserLocal = &artifact
	destination, installed := userLocalPaths(s.home.path, artifact.Recipe, artifact.Version, artifact.Layout, artifact.Command)
	if (artifact.Destination != "" && artifact.Destination != destination) || (artifact.InstalledPath != "" && artifact.InstalledPath != installed) {
		return errors.New("reviewed artifact home identity changed")
	}
	artifact.Destination, artifact.InstalledPath = destination, installed
	id, err := receiptID()
	if err != nil {
		return err
	}
	c := capturedReceipt{Format: 1, ID: id, Ordinal: ordinal, Home: s.home.path, State: s.state.path, Action: action, Selection: artifactSelection(action), Parents: map[string]PublicationEvidence{}}
	c.HomeIdentity, err = rootIdentity(s.home)
	if err != nil {
		return err
	}
	c.StateIdentity, err = rootIdentity(s.state)
	if err != nil {
		return err
	}
	c.StateLockIdentity, err = fileIdentity(s.stateLock)
	if err != nil {
		return err
	}
	c.Previous = latestCompleted(ops, receiptKey(c))
	for _, path := range authorityParents(artifact) {
		if strings.HasSuffix(path, ".lock") {
			c.Parents[path], err = fileIdentity(s.homeLock)
		} else {
			parent, name, openErr := s.home.parent(path, true)
			if openErr != nil {
				return openErr
			}
			dir, dirErr := openReceiptDirChecked(parent, name, true, 0755, i.check)
			closeErr := parent.Close()
			if dir != nil {
				closeErr = errors.Join(closeErr, dir.Sync(), dir.Close())
			}
			if dirErr != nil || closeErr != nil {
				return errors.Join(dirErr, closeErr)
			}
			c.Parents[path], err = identityAt(s.home, path)
		}
		if err != nil {
			return err
		}
	}
	if err := validateCaptured(c); err != nil {
		return err
	}
	for _, unit := range unitPaths(c) {
		if _, err := s.home.evidence(unit.Target, false); !errors.Is(err, unix.ENOENT) {
			if err == nil {
				return fmt.Errorf("destination %s is occupied; move it explicitly before retrying", unit.Target)
			}
			return err
		}
	}
	if _, err := LoadDependencyMetadata(DependencyMetadataPath(s.state.path)); err != nil {
		return err
	}
	journal, err := openReceiptDirChecked(s.state.file, receiptJournal, true, 0700, i.check)
	if err != nil {
		return err
	}
	opDir, dirErr := openReceiptDirChecked(journal, id, true, 0700, i.check)
	syncErr := journal.Sync()
	closeErr := journal.Close()
	if opDir != nil {
		closeErr = errors.Join(closeErr, opDir.Close())
	}
	if err := errors.Join(dirErr, syncErr, closeErr, s.state.file.Sync()); err != nil {
		return err
	}
	if err := s.validate(c); err != nil {
		return err
	}
	if err := i.writeReceipt(s.state, operationPath(id, "authority-"+receiptDigest(c)), c); err != nil {
		var saved capturedReceipt
		found, readErr := readReceiptJSON(s.state, operationPath(id, "authority-"+receiptDigest(c)), &saved)
		if readErr == nil && !found {
			journal, openErr := openReceiptDir(s.state.file, receiptJournal, false)
			if openErr != nil {
				return errors.Join(err, openErr)
			}
			removeErr := unix.Unlinkat(int(journal.Fd()), id, unix.AT_REMOVEDIR)
			return errors.Join(err, removeErr, journal.Sync(), journal.Close())
		}
		return errors.Join(err, readErr)
	}
	return i.resume(s, journalOperation{Captured: c, Progress: map[int]bool{}})
}

// WithUserLocalBoundary holds both locks through recovery, negative presence
// checks and dependency execution. The installer is scoped to one invocation.
func (i *UserLocalInstaller) WithUserLocalBoundary(selected []UserLocalSelection, consume func([]InstallAction) error) (resultErr error) {
	if len(selected) == 0 {
		if _, err := pendingSelections(i.Home, i.StateRoot, nil); err != nil {
			return err
		}
		return consume(nil)
	}
	if i.active != nil {
		return errors.New("nested user-local mutation boundary")
	}
	s, err := i.session()
	if err != nil {
		return err
	}
	i.active = s
	defer func() { i.active = nil; resultErr = errors.Join(resultErr, s.close()) }()
	if err := i.syncJournal(s); err != nil {
		return err
	}
	if _, err := loadLegacyDependencyMetadata(DependencyMetadataPath(s.state.path)); err != nil {
		return err
	}
	ops, err := readJournal(s.state)
	if err != nil {
		return err
	}
	if err := rejectRetiredPending(s.home, ops); err != nil {
		return err
	}
	if err := i.check("after-recovery-read"); err != nil {
		return err
	}
	var actions []InstallAction
	for _, op := range ops {
		if op.Complete {
			continue
		}
		for _, selection := range selected {
			if selection.Dependency != op.Captured.Action.Dependency {
				continue
			}
			matches, err := homeScopeMatchesChecked(s.home, op.Captured, i.check)
			if err != nil {
				return err
			}
			if !matches {
				continue
			}
			if selection != op.Captured.Selection {
				return errors.New("pending selected provider identity changed")
			}
			if err := i.resume(s, op); err != nil {
				return fmt.Errorf("recover %s: %w; preserve %s and restore captured paths before retrying", selection.Dependency, err, filepath.Join(i.StateRoot, receiptJournal))
			}
			actions = append(actions, op.Captured.Action)
		}
	}
	if err := s.validateLocks(); err != nil {
		return err
	}
	return consume(actions)
}

func (i *UserLocalInstaller) RecoverUserLocal(selected []UserLocalSelection) (actions []InstallAction, resultErr error) {
	resultErr = i.WithUserLocalBoundary(selected, func(recovered []InstallAction) error { actions = recovered; return nil })
	return actions, resultErr
}

func (i *UserLocalInstaller) prepare(s *receiptSession, c capturedReceipt) (ready readyReceipt, resultErr error) {
	if err := s.validate(c); err != nil {
		return ready, err
	}
	stage := filepath.Join(".local", ".dots-deps-"+c.ID)
	parent, name, err := s.home.parent(stage, false)
	if err != nil {
		return ready, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	// An interrupted preparation has no complete postimage authority. Retain it,
	// rather than deleting/adopting it or accumulating another extraction area.
	if err := i.check("before-stage"); err != nil {
		return ready, err
	}
	if err := unix.Mkdirat(int(parent.Fd()), name, 0700); err != nil {
		return ready, fmt.Errorf("ambiguous private staging %s; preserve or move it explicitly before retrying: %w", stage, err)
	}
	if err := unix.Fchmodat(int(parent.Fd()), name, 0700, 0); err != nil {
		return ready, err
	}
	download := i.Download
	if download == nil {
		download = func(url, checksum string) ([]byte, error) {
			return downloadAndVerifyContext(receiptContext(i.Context), url, checksum)
		}
	}
	a := c.Action.UserLocal
	if err := i.check("before-download"); err != nil {
		return ready, err
	}
	data, err := download(a.URL, a.Checksum)
	if err != nil {
		return ready, err
	}
	if err := i.check("after-download"); err != nil {
		return ready, err
	}
	stageDir, err := openReceiptDir(parent, name, false)
	if err != nil {
		return ready, err
	}
	defer func() { resultErr = errors.Join(resultErr, stageDir.Close()) }()
	if err := stageDir.Chmod(0700); err != nil {
		return ready, err
	}
	payloadDir, err := openReceiptDirChecked(stageDir, "payload", true, 0700, i.check)
	if err != nil {
		return ready, err
	}
	defer func() { resultErr = errors.Join(resultErr, payloadDir.Close()) }()
	payload := &receiptRoot{path: filepath.Join(c.Home, stage, "payload"), file: payloadDir}
	recipe := userLocalRecipes[a.Recipe]
	if err := stageUserLocalArtifactAt(data, a.archiveName(), recipe, payload, func() error { return i.check("extract") }); err != nil {
		return ready, err
	}
	for _, command := range recipe.links {
		node, err := payload.evidence(recipe.binaryPath(a.archiveName(), command), false)
		if err != nil {
			return ready, fmt.Errorf("stat extracted executable: %w", err)
		}
		if node.Mode&unix.S_IFMT != unix.S_IFREG || node.Mode&0111 == 0 {
			return ready, errors.New("invalid extracted executable")
		}
	}
	linksDir, err := openReceiptDirChecked(stageDir, "links", true, 0700, i.check)
	if err != nil {
		return ready, err
	}
	defer func() { resultErr = errors.Join(resultErr, linksDir.Close()) }()
	if a.Layout == userLocalLayoutBundle {
		for _, command := range recipe.links {
			target := filepath.Join(a.Destination, recipe.binaryPath(a.archiveName(), command))
			if err := unix.Symlinkat(target, int(linksDir.Fd()), command); err != nil {
				return ready, err
			}
		}
	}
	if err := errors.Join(payloadDir.Sync(), linksDir.Sync(), stageDir.Sync()); err != nil {
		return ready, err
	}
	ready = readyReceipt{ID: c.ID, Captured: receiptDigest(c), Units: unitPaths(c)}
	ready.Parents = map[string]PublicationEvidence{}
	for _, path := range sourceParentPaths(c) {
		identity, err := identityAt(s.home, path)
		if err != nil {
			return ready, err
		}
		ready.Parents[path] = identity
	}
	if err := validateSourceParents(c, ready); err != nil {
		return ready, err
	}

	for n, unit := range ready.Units {
		evidence, err := s.home.evidence(unit.Source, true)
		if err != nil {
			return ready, err
		}
		ready.Units[n].Evidence = evidence
	}
	if _, err := s.home.evidence(stage, true); err != nil {
		return ready, err
	}
	if err := parent.Sync(); err != nil {
		return ready, err
	}
	if err := validateUnits(c, ready.Units); err != nil {
		return ready, err
	}
	if err := s.validate(c); err != nil {
		return ready, err
	}
	if err := i.writeReceipt(s.state, operationPath(c.ID, "ready"), ready); err != nil {
		return ready, err
	}
	return ready, nil
}

func (i *UserLocalInstaller) publish(s *receiptSession, c capturedReceipt, unit publicationUnit, published bool, parents map[string]PublicationEvidence) (resultErr error) {
	if err := s.validate(c); err != nil {
		return err
	}
	actual, err := s.home.evidence(unit.Target, false)
	if err == nil {
		if !sameEvidence(actual, unit.Evidence) {
			return fmt.Errorf("external change at %s", unit.Target)
		}
		return s.validate(c)
	}
	if !errors.Is(err, unix.ENOENT) {
		return err
	}
	if published {
		return fmt.Errorf("published command removed at %s", unit.Target)
	}
	source, err := s.home.evidence(unit.Source, false)
	if err != nil {
		return err
	}
	if !sameEvidence(source, unit.Evidence) {
		return fmt.Errorf("staged artifact changed at %s", unit.Source)
	}
	from, sourceName, err := s.home.parent(unit.Source, false)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, from.Close()) }()
	to, targetName, err := s.home.parent(unit.Target, false)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, to.Close()) }()
	if err := s.validate(c); err != nil {
		return err
	}
	if err := i.check("source:before-consume"); err != nil {
		return err
	}
	for path, expected := range parents {
		actual, err := identityAt(s.home, path)
		if err != nil {
			return err
		}
		if !sameEvidence(actual, expected) {
			return fmt.Errorf("private source ancestor changed at %s", path)
		}
	}
	currentParent, _, err := s.home.parent(unit.Source, false)
	if err != nil {
		return err
	}
	currentIdentity, currentErr := fileIdentity(currentParent)
	pinnedIdentity, pinnedErr := fileIdentity(from)
	if err := errors.Join(currentErr, pinnedErr, currentParent.Close()); err != nil {
		return err
	}
	if !sameEvidence(currentIdentity, pinnedIdentity) {
		return errors.New("staged source parent changed at consumption boundary")
	}

	source, err = captureEvidence(from, sourceName, false)
	if err != nil {
		return err
	}
	if !sameEvidence(source, unit.Evidence) {
		return errors.New("staged identity changed at consumption boundary")
	}
	if err := s.validate(c); err != nil {
		return err
	}
	if err := i.check("destination:before-consume"); err != nil {
		return err
	}
	// At the consumption boundary NOREPLACE/linkat refuses a late foreign entry.
	if unit.Evidence.Mode&unix.S_IFMT == unix.S_IFDIR {
		err = promoteReceiptDirectory(int(from.Fd()), sourceName, int(to.Fd()), targetName)
	} else {
		err = unix.Linkat(int(from.Fd()), sourceName, int(to.Fd()), targetName, 0)
	}
	if err != nil {
		return fmt.Errorf("publish %s without replacing external state: %w", unit.Target, err)
	}
	if err := errors.Join(from.Sync(), to.Sync()); err != nil {
		return err
	}
	if err := i.check("destination:after-publish"); err != nil {
		return err
	}
	actual, err = s.home.evidence(unit.Target, false)
	if err != nil {
		return err
	}
	if !sameEvidence(actual, unit.Evidence) {
		return fmt.Errorf("publication changed at %s", unit.Target)
	}
	return s.validate(c)
}

func verifyPublished(s *receiptSession, c capturedReceipt, ready readyReceipt) error {
	if err := s.validate(c); err != nil {
		return err
	}
	for _, unit := range ready.Units {
		actual, err := s.home.evidence(unit.Target, false)
		if err != nil {
			return err
		}
		if !sameEvidence(actual, unit.Evidence) {
			return fmt.Errorf("published surface changed at %s", unit.Target)
		}
	}
	return nil
}

func (i *UserLocalInstaller) resume(s *receiptSession, op journalOperation) error {
	if err := retiredActionError(op.Captured.Action); err != nil {
		return fmt.Errorf("pending custom provider journal %s requires manual remediation; preserve journal, staging and external paths: %w", op.Captured.ID, err)
	}
	c := op.Captured
	if err := s.validate(c); err != nil {
		return err
	}
	if !op.CapturedProjection {
		if err := i.writeReceipt(s.state, operationPath(c.ID, "captured"), c); err != nil {
			return err
		}
		op.CapturedProjection = true
	}
	if err := i.check("after-captured"); err != nil {
		return err
	}

	if op.Ready == nil {
		ready, err := i.prepare(s, c)
		if err != nil {
			return err
		}
		op.Ready = &ready
	}
	for n, unit := range op.Ready.Units {
		if err := i.check(fmt.Sprintf("before-unit-%d", n)); err != nil {
			return err
		}
		if err := i.publish(s, c, unit, op.Progress[n], op.Ready.Parents); err != nil {
			return err
		}
		if err := i.check(fmt.Sprintf("after-unit-%d", n)); err != nil {
			return err
		}
		if err := s.validate(c); err != nil {
			return err
		}
		if err := i.writeReceipt(s.state, operationPath(c.ID, fmt.Sprintf("unit-%d", n)), receiptMarker{ID: c.ID, Digest: receiptDigest(unit)}); err != nil {
			return err
		}
		op.Progress[n] = true
	}
	if err := i.check("before-final"); err != nil {
		return err
	}
	if err := verifyPublished(s, c, *op.Ready); err != nil {
		return err
	}
	if op.Final == nil {
		final := finalReceipt{ID: c.ID, Ready: receiptDigest(*op.Ready), Record: recordForOperation(c, *op.Ready, time.Now().UTC().Format(time.RFC3339Nano))}
		if err := i.writeReceipt(s.state, operationPath(c.ID, "final"), final); err != nil {
			return err
		}
		op.Final = &final
	}
	if err := i.check("after-final"); err != nil {
		return err
	}
	if err := verifyPublished(s, c, *op.Ready); err != nil {
		return err
	}
	if err := i.writeReceipt(s.state, operationPath(c.ID, "complete"), receiptMarker{ID: c.ID, Digest: receiptDigest(*op.Final)}); err != nil {
		return err
	}
	return verifyPublished(s, c, *op.Ready)
}

func recordForOperation(c capturedReceipt, ready readyReceipt, when string) DependencyRecord {
	a := c.Action.UserLocal
	record := DependencyRecord{Dependency: c.Action.Dependency, Provider: string(TierUserLocal), Path: a.InstalledPath, Version: a.Version, URL: a.URL, Artifact: a.Artifact, Digest: a.Digest, Checksum: a.Checksum, Platform: a.Platform, InstalledAt: when, OperationID: c.ID}
	for _, unit := range ready.Units {
		record.Publications = append(record.Publications, DependencyPublication{Path: filepath.Join(c.Home, unit.Target), Evidence: unit.Evidence})
	}
	return record
}

func validFinal(c capturedReceipt, ready readyReceipt, record DependencyRecord) bool {
	if _, err := time.Parse(time.RFC3339Nano, record.InstalledAt); err != nil {
		return false
	}
	return sameReceipt(recordForOperation(c, ready, record.InstalledAt), record)
}

func sourceParentPaths(c capturedReceipt) []string {
	seen := map[string]bool{}
	var result []string
	for _, unit := range unitPaths(c) {
		parts := strings.Split(filepath.Dir(unit.Source), string(filepath.Separator))
		for n := 2; n <= len(parts); n++ {
			path := filepath.Join(parts[:n]...)
			if !seen[path] {
				seen[path] = true
				result = append(result, path)
			}
		}
	}
	return result
}
func validateSourceParents(c capturedReceipt, ready readyReceipt) error {
	expected := sourceParentPaths(c)
	if len(expected) != len(ready.Parents) {
		return errors.New("incomplete private source-parent evidence")
	}
	stage := filepath.Join(".local", ".dots-deps-"+c.ID)
	for _, path := range expected {
		node, ok := ready.Parents[path]
		if !ok || !validEvidenceIdentity(node) || node.Mode&unix.S_IFMT != unix.S_IFDIR {
			return errors.New("invalid private source-parent evidence")
		}
		if path == stage || path == filepath.Join(stage, "links") || path == filepath.Join(stage, "payload") {
			if node.Mode&07777 != 0700 {
				return errors.New("private staging permissions changed")
			}
		}
	}
	return nil
}

func homeScopeMatches(root *receiptRoot, c capturedReceipt) (bool, error) {
	return homeScopeMatchesChecked(root, c, nil)
}

func homeScopeMatchesChecked(root *receiptRoot, c capturedReceipt, check func(string) error) (bool, error) {
	// The held descriptor pins this transient scope precheck. A different inode
	// is not this operation's home, so unrelated pending history does not demand
	// strong filesystem capabilities from an externally satisfied home.
	var st unix.Stat_t
	if err := unix.Fstat(int(root.file.Fd()), &st); err != nil {
		return false, err
	}
	if uint64(st.Dev) != c.HomeIdentity.Device || st.Ino != c.HomeIdentity.Inode {
		if root.path == c.Home {
			return false, errors.New("selected home identity changed since capture")
		}
		// A remount may change device numbers. A different spelling can still
		// name the same current root; that is changed authority, not an unrelated
		// home whose external commands may bypass this pending operation.
		original, err := openReceiptRoot(c.Home, false)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var binding unix.Stat_t
		if err := errors.Join(unix.Fstat(int(original.file.Fd()), &binding), original.file.Close()); err != nil {
			return false, err
		}
		if binding.Dev != st.Dev || binding.Ino != st.Ino {
			return false, nil
		}
	}
	if err := durabilityCheck(check, "identity"); err != nil {
		return false, err
	}
	actual, err := fileIdentity(root.file)
	if err != nil {
		return false, err
	}
	if actual.Device == c.HomeIdentity.Device && actual.Inode == c.HomeIdentity.Inode {
		if actual.Incarnation != c.HomeIdentity.Incarnation {
			return false, errors.New("selected home filesystem incarnation changed since capture")
		}
		return true, nil
	}
	return false, errors.New("selected home identity changed since capture")
}
func validateCapturedRootName(current *receiptRoot, path string, expected PublicationEvidence) error {
	if err := current.validate(expected); err != nil {
		return err
	}
	if current.path == path {
		return nil
	}
	original, err := openReceiptRoot(path, false)
	if err != nil {
		return err
	}
	return errors.Join(original.validate(expected), original.file.Close())
}
