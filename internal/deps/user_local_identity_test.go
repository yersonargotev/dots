package deps

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUserLocalPersistentObjectIdentity(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, err := openReceiptRoot(t.TempDir(), false)
			if err != nil {
				t.Fatal(err)
			}
			defer root.file.Close()
			path := filepath.Join(root.path, "original")
			create := func() {
				t.Helper()
				var err error
				switch kind {
				case "file":
					err = os.WriteFile(path, []byte("same bytes"), 0755)
				case "directory":
					err = os.Mkdir(path, 0755)
				case "symlink":
					err = os.Symlink("missing", path)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			create()
			before, err := namedIdentity(root.file, "original")
			if err != nil {
				t.Fatal(err)
			}
			if !validEvidenceIdentity(before) {
				t.Fatal("missing durable incarnation")
			}
			if err := os.Rename(path, filepath.Join(root.path, "moved")); err != nil {
				t.Fatal(err)
			}
			moved, err := namedIdentity(root.file, "moved")
			if err != nil || !sameEvidence(before, moved) {
				t.Fatalf("rename changed incarnation: %v", err)
			}
			if kind != "directory" {
				if err := unix.Linkat(int(root.file.Fd()), "moved", int(root.file.Fd()), "linked", 0); err != nil {
					t.Fatal(err)
				}
				linked, err := namedIdentity(root.file, "linked")
				if err != nil || !sameEvidence(before, linked) {
					t.Fatalf("hardlink changed incarnation: %v", err)
				}
			}
			create()
			after, err := namedIdentity(root.file, "original")
			if err != nil {
				t.Fatal(err)
			}
			if before.Incarnation == after.Incarnation {
				t.Fatal("external replacement retained incarnation")
			}
		})
	}
}

func TestUserLocalIncarnationRequiredRecursively(t *testing.T) {
	i, a, _ := transactionFixture(t, "uv")
	i.Fault = func(point string) error {
		if point == "before-unit-0" {
			return errors.New("stop")
		}
		return nil
	}
	if err := i.RunUserLocal(a, nil); err == nil {
		t.Fatal("expected pending")
	}
	op := loadTransactionOps(t, i.StateRoot)[0]
	for name, child := range op.Ready.Units[0].Evidence.Children {
		child.Incarnation = ""
		op.Ready.Units[0].Evidence.Children[name] = child
		break
	}
	data, err := json.Marshal(op.Ready)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(i.StateRoot, operationPath(op.Captured.ID, "ready"))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot)); err == nil {
		t.Fatal("accepted uncaptured child incarnation")
	}
	if after, err := os.ReadFile(path); err != nil || !bytes.Equal(after, data) {
		t.Fatal("read-only load repaired missing identity")
	}
}

func TestUserLocalUnsupportedIdentityIsLazy(t *testing.T) {
	for _, mode := range []string{"external", "new", "pending"} {
		t.Run(mode, func(t *testing.T) {
			i, a, calls := transactionFixture(t, "uv")
			legacy := []byte("{\"version\":1,\"dependencies\":[]}\n")
			if err := os.WriteFile(DependencyMetadataPath(i.StateRoot), legacy, 0600); err != nil {
				t.Fatal(err)
			}
			if mode == "pending" {
				i.Fault = func(point string) error {
					if point == "after-captured" {
						return errors.New("stop")
					}
					return nil
				}
				if err := i.RunUserLocal(a, nil); err == nil {
					t.Fatal("expected pending")
				}
			}
			identityCalls := 0
			i.Fault = func(point string) error {
				if point == "identity" {
					identityCalls++
					return unix.EOPNOTSUPP
				}
				return nil
			}
			err := i.RunUserLocal(a, func() bool { return mode != "new" })
			if mode == "external" {
				if err != nil || identityCalls != 0 {
					t.Fatalf("external capability required identity: %v", err)
				}
			} else if !errors.Is(err, unix.EOPNOTSUPP) || identityCalls == 0 {
				t.Fatalf("identity refusal ignored: %v", err)
			}
			if *calls != 0 {
				t.Fatal("downloaded without identity authority")
			}
			if after, err := os.ReadFile(DependencyMetadataPath(i.StateRoot)); err != nil || !bytes.Equal(after, legacy) {
				t.Fatal("legacy metadata rewritten")
			}
			if _, err := os.Lstat(a.UserLocal.InstalledPath); !os.IsNotExist(err) {
				t.Fatal("published without identity authority")
			}
		})
	}
}

func TestUserLocalReceiptEpochProjectionOrder(t *testing.T) {
	makeOp := func(id string, ordinal uint64, epoch string, complete bool) journalOperation {
		return journalOperation{Captured: capturedReceipt{ID: strings.Repeat(id, 32), Ordinal: ordinal, HomeIdentity: PublicationEvidence{Device: 1, Inode: 2, Incarnation: epoch}, Action: InstallAction{Dependency: "uv"}}, Complete: complete, Final: &finalReceipt{Record: DependencyRecord{Dependency: "uv", Provider: "user-local", Path: "/home/.local/bin/uv", Version: id}}}
	}
	a, b := makeOp("f", 1, "boot-A", true), makeOp("a", 2, "boot-B", true)
	legacy := DependencyMetadata{Dependencies: []DependencyRecord{{Dependency: "other", Path: "/other"}}}
	for _, ops := range [][]journalOperation{{a, b}, {b, a}} {
		firstID := ops[0].Captured.ID
		if err := validateReceiptChains(ops); err != nil {
			t.Fatal(err)
		}
		meta := projectJournal(legacy, ops)
		if len(meta.Dependencies) != 2 || meta.Dependencies[0].Dependency != "other" || meta.Dependencies[1].Version != "a" {
			t.Fatalf("stale epoch projected: %+v", meta)
		}
		if ops[0].Captured.ID != firstID {
			t.Fatal("input mutated")
		}
	}
	b.Complete = false
	if meta := projectJournal(DependencyMetadata{}, []journalOperation{b, a}); len(meta.Dependencies) != 1 || meta.Dependencies[0].Version != "f" {
		t.Fatal("pending displaced completed audit")
	}
	for _, ordinal := range []uint64{0, 1} {
		b.Captured.Ordinal = ordinal
		if err := validateReceiptChains([]journalOperation{a, b}); err == nil {
			t.Fatal("accepted missing/duplicate ordinal")
		}
	}
	b.Captured.Ordinal = 1
	a.Captured.Ordinal = 2
	b.Captured.HomeIdentity = a.Captured.HomeIdentity
	b.Captured.Previous = a.Captured.ID
	if err := validateReceiptChains([]journalOperation{a, b}); err == nil {
		t.Fatal("accepted nonincreasing previous edge")
	}
}

// Rewrite a coherent captured-only fixture, retaining the hash-addressed root.
func rewriteCapturedIdentityFixture(t *testing.T, state string, old capturedReceipt, changed capturedReceipt) {
	t.Helper()
	data, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(state, operationPath(old.ID, "authority-"+receiptDigest(old)))
	newPath := filepath.Join(state, operationPath(changed.ID, "authority-"+receiptDigest(changed)))
	if err := os.WriteFile(newPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if oldPath != newPath {
		if err := os.Remove(oldPath); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(state, operationPath(changed.ID, "captured")), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestUserLocalOldEpochPendingAndOrdinalExhaustion(t *testing.T) {
	for _, mode := range []string{"epoch", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == "after-captured" {
					return errors.New("stop")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("expected pending")
			}
			old := loadTransactionOps(t, i.StateRoot)[0].Captured
			changed := old
			if mode == "epoch" {
				parts := strings.Split(changed.HomeIdentity.Incarnation, ":")
				if parts[1][0] == '1' {
					parts[1] = "2" + parts[1][1:]
				} else {
					parts[1] = "1" + parts[1][1:]
				}
				changed.HomeIdentity.Incarnation = strings.Join(parts, ":")
			} else {
				changed.Ordinal = ^uint64(0)
			}
			rewriteCapturedIdentityFixture(t, i.StateRoot, old, changed)
			i.Fault = nil
			if mode == "epoch" {
				if err := i.RunUserLocal(a, func() bool { return true }); err == nil {
					t.Fatal("old epoch pending silently satisfied")
				}
			} else {
				j, b, _ := transactionFixture(t, "starship")
				j.Home = i.Home
				j.StateRoot = i.StateRoot
				b.UserLocal.Destination = ""
				b.UserLocal.InstalledPath = ""
				if err := j.RunUserLocal(b, nil); err == nil || !strings.Contains(err.Error(), "ordinal exhausted") {
					t.Fatalf("overflow ignored: %v", err)
				}
			}
			ops := loadTransactionOps(t, i.StateRoot)
			if len(ops) != 1 || ops[0].Captured.ID != old.ID || ops[0].Complete {
				t.Fatal("failure created or completed operation")
			}
		})
	}
}

func TestUserLocalForeignHomePendingDoesNotRequireIdentity(t *testing.T) {
	for _, mode := range []string{"direct", "boundary", "install-prepared"} {
		t.Run(mode, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == "after-captured" {
					return errors.New("stop")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("expected pending")
			}
			pending := loadTransactionOps(t, i.StateRoot)[0]
			j, b, downloads := transactionFixture(t, "uv")
			j.StateRoot = i.StateRoot
			identityCalls := 0
			j.Fault = func(point string) error {
				if point == "identity" {
					identityCalls++
					return unix.EOPNOTSUPP
				}
				return nil
			}
			var err error
			switch mode {
			case "direct":
				err = j.RunUserLocal(b, func() bool { return true })
			case "boundary":
				err = j.WithUserLocalBoundary([]UserLocalSelection{artifactSelection(b)}, func(recovered []InstallAction) error {
					if len(recovered) != 0 {
						t.Fatal("recovered another home's pending")
					}
					return nil
				})
			case "install-prepared":
				opts := Options{Profile: "default", OS: "linux", Arch: "amd64", Home: j.Home, StateRoot: j.StateRoot}
				_, err = Install(transactionManifest(b), opts, func(string) bool { return true }, func(string) bool { return false }, TierGeneric, realTransactionRunner{&j})
			}
			if err != nil || identityCalls != 0 || *downloads != 0 {
				t.Fatalf("foreign pending required identity: %v calls=%d", err, identityCalls)
			}
			ops := loadTransactionOps(t, i.StateRoot)
			if len(ops) != 1 || !sameReceipt(ops[0].Captured, pending.Captured) || ops[0].Complete {
				t.Fatal("foreign pending changed")
			}
		})
	}
}

func TestUserLocalMalformedIncarnationFailsPersistedLoads(t *testing.T) {
	for _, where := range []string{"root", "parent", "ready-child"} {
		t.Run(where, func(t *testing.T) {
			i, a, _ := transactionFixture(t, "uv")
			i.Fault = func(point string) error {
				if point == "before-unit-0" {
					return errors.New("stop")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("expected pending")
			}
			op := loadTransactionOps(t, i.StateRoot)[0]
			path := filepath.Join(i.StateRoot, operationPath(op.Captured.ID, "captured"))
			if where == "ready-child" {
				for name, child := range op.Ready.Units[0].Evidence.Children {
					child.Incarnation = "garbage"
					op.Ready.Units[0].Evidence.Children[name] = child
					break
				}
				path = filepath.Join(i.StateRoot, operationPath(op.Captured.ID, "ready"))
				data, _ := json.Marshal(op.Ready)
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				c := op.Captured
				if where == "root" {
					c.HomeIdentity.Incarnation = "garbage"
				} else {
					c.Parents = map[string]PublicationEvidence{}
					for name, node := range op.Captured.Parents {
						node.Incarnation = "garbage"
						c.Parents[name] = node
					}
				}
				rewriteCapturedIdentityFixture(t, i.StateRoot, op.Captured, c)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := LoadDependencyMetadata(DependencyMetadataPath(i.StateRoot)); err == nil {
				t.Fatal("read-only load accepted malformed incarnation")
			}
			i.Fault = nil
			if _, err := i.RecoverUserLocal([]UserLocalSelection{artifactSelection(a)}); err == nil {
				t.Fatal("mutable load accepted malformed incarnation")
			}
			if after, err := os.ReadFile(path); err != nil || !bytes.Equal(before, after) {
				t.Fatal("malformed evidence repaired")
			}
		})
	}
}

func TestUserLocalPersistedIncarnationSyntax(t *testing.T) {
	uuid := "11111111222233334444555555555555"
	for _, value := range []string{"garbage", "unknown:" + uuid + ":42", "darwin:" + uuid + ":042", "darwin:" + strings.ToUpper(uuid) + "AA:42", "darwin:" + strings.Repeat("0", 32) + ":42", "darwin:" + uuid + ":41", "linux:" + uuid + ":01:1:ab", "linux:" + uuid + ":1:+1:ab", "linux:" + uuid + ":1:1:AB", "linux:" + uuid + ":1:1:a", "linux:" + uuid + ":1:2147483648:ab", "linux:" + uuid + ":1:1:" + strings.Repeat("ab", 129)} {
		if validIncarnation(value, 42) {
			t.Fatalf("accepted noncanonical incarnation %q", value)
		}
	}
	for _, value := range []string{"darwin:" + uuid + ":42", "linux:" + uuid + ":1:1:ab"} {
		if !validIncarnation(value, 42) {
			t.Fatalf("rejected valid incarnation %q", value)
		}
	}
}

func TestUserLocalAliasAndEpochChangeCannotBypassPending(t *testing.T) {
	for _, mode := range []string{"direct", "prepare", "install-prepared"} {
		t.Run(mode, func(t *testing.T) {
			i, a, downloads := transactionFixture(t, "uv")
			physical := i.Home
			alias := filepath.Join(t.TempDir(), "alias")
			if err := os.Symlink(physical, alias); err != nil {
				t.Fatal(err)
			}
			i.Home = alias
			a.UserLocal.Destination, a.UserLocal.InstalledPath = userLocalPaths(alias, "uv", a.UserLocal.Version, a.UserLocal.Layout, "uv")
			i.Fault = func(point string) error {
				if point == "after-captured" {
					return errors.New("stop")
				}
				return nil
			}
			if err := i.RunUserLocal(a, nil); err == nil {
				t.Fatal("expected pending")
			}
			op := loadTransactionOps(t, i.StateRoot)[0]
			opts := Options{Profile: "default", OS: "linux", Arch: "amd64", Home: physical, StateRoot: i.StateRoot}
			var prepared PreparedInstall
			if mode == "install-prepared" {
				var err error
				prepared, err = PrepareInstall(transactionManifest(a), opts, func(string) bool { return true }, func(string) bool { return false }, TierGeneric)
				if err != nil {
					t.Fatal(err)
				}
			}
			changed := op.Captured
			changed.HomeIdentity.Device ^= 1
			changed.HomeIdentity.Inode++
			parts := strings.Split(changed.HomeIdentity.Incarnation, ":")
			if parts[1][0] == '1' {
				parts[1] = "2" + parts[1][1:]
			} else {
				parts[1] = "1" + parts[1][1:]
			}
			if parts[0] == "darwin" {
				parts[2] = strconv.FormatUint(changed.HomeIdentity.Inode, 10)
			}
			changed.HomeIdentity.Incarnation = strings.Join(parts, ":")
			rewriteCapturedIdentityFixture(t, i.StateRoot, op.Captured, changed)
			i.Home = physical
			identityCalls, presenceCalls := 0, 0
			i.Fault = func(point string) error {
				if point == "identity" {
					identityCalls++
				}
				return nil
			}
			present := func(string) bool { presenceCalls++; return true }
			var err error
			switch mode {
			case "direct":
				err = i.RunUserLocal(a, func() bool { return present("uv") })
			case "prepare":
				_, err = PrepareInstall(transactionManifest(a), opts, present, func(string) bool { return false }, TierGeneric)
			case "install-prepared":
				_, err = InstallPrepared(prepared, opts, present, func(string) bool { return false }, realTransactionRunner{&i})
			}
			if err == nil || *downloads != 0 || presenceCalls != 0 {
				t.Fatalf("alias epoch bypassed pending: %v downloads%d presence%d", err, *downloads, presenceCalls)
			}
			if mode != "prepare" && identityCalls == 0 {
				t.Fatal("same current alias skipped strong identity")
			}
			ops := loadTransactionOps(t, i.StateRoot)
			if len(ops) != 1 || ops[0].Complete || !sameReceipt(ops[0].Captured, changed) {
				t.Fatal("changed epoch evidence mutated")
			}
		})
	}
}
