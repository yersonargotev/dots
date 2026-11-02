package deps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// PublicationEvidence identifies an entire publication unit, including empty
// files. Content equality alone never establishes ownership of another inode.
type PublicationEvidence struct {
	Device   uint64                         `json:"device"`
	Inode    uint64                         `json:"inode"`
	Mode     uint32                         `json:"mode"`
	Digest   string                         `json:"sha256,omitempty"`
	Link     string                         `json:"link,omitempty"`
	Children map[string]PublicationEvidence `json:"children,omitempty"`
}

type receiptRoot struct {
	path string
	file *os.File
}

func openReceiptRoot(path string, create bool) (*receiptRoot, error) {
	return openReceiptRootChecked(path, create, nil)
}
func durabilityCheck(check func(string) error, point string) error {
	if check != nil {
		return check(point)
	}
	return nil
}
func openReceiptRootChecked(path string, create bool, check func(string) error) (root *receiptRoot, resultErr error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if create {
		prefix := absolute
		var missing []string
		for {
			info, err := os.Stat(prefix)
			if err == nil {
				if !info.IsDir() {
					return nil, errors.New("receipt root is not a directory")
				}
				break
			}
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			missing = append(missing, filepath.Base(prefix))
			prefix = filepath.Dir(prefix)
		}
		canonical, err := filepath.EvalSymlinks(prefix)
		if err != nil {
			return nil, err
		}
		fd, err := unix.Open(canonical, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, err
		}
		dir := os.NewFile(uintptr(fd), canonical)
		for n := len(missing) - 1; n >= 0; n-- {
			next, err := openReceiptDirChecked(dir, missing[n], true, 0755, check)
			closeErr := dir.Close()
			if err != nil || closeErr != nil {
				if next != nil {
					closeErr = errors.Join(closeErr, next.Close())
				}
				return nil, errors.Join(err, closeErr)
			}
			dir = next
		}
		if err := syncReceiptAncestors(absolute, check); err != nil {
			return nil, errors.Join(err, dir.Close())
		}
		return &receiptRoot{path: absolute, file: dir}, nil
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open(canonical, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return &receiptRoot{path: absolute, file: os.NewFile(uintptr(fd), canonical)}, nil
}

func syncReceiptAncestors(path string, check func(string) error) error {
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	for current := canonical; ; current = filepath.Dir(current) {
		if err := durabilityCheck(check, "scaffold:ancestor-sync"); err != nil {
			return err
		}
		fd, err := unix.Open(current, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(fd), current)
		if err := errors.Join(file.Sync(), file.Close()); err != nil {
			return err
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return nil
}

func openReceiptDir(parent *os.File, name string, create bool) (*os.File, error) {
	return openReceiptDirChecked(parent, name, create, 0755, nil)
}
func openReceiptDirChecked(parent *os.File, name string, create bool, mode uint32, check func(string) error) (*os.File, error) {
	if name != "." && (!filepath.IsLocal(name) || strings.Contains(name, string(filepath.Separator))) {
		return nil, fmt.Errorf("invalid directory component %q", name)
	}
	created := false
	if create {
		if err := durabilityCheck(check, "mkdir:"+name+":before"); err != nil {
			return nil, err
		}
		err := unix.Mkdirat(int(parent.Fd()), name, mode)
		if err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, err
		}
		created = err == nil
		// A restrictive umask may otherwise prevent opening this newly created
		// private directory. The protocol exclusively owns this fresh private name.
		if created && mode == 0700 {
			if err := unix.Fchmodat(int(parent.Fd()), name, 0700, 0); err != nil {
				return nil, err
			}
		}
	}
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if create && mode == 0700 {
		if created {
			err = unix.Fchmod(fd, 0700)
		} else {
			info, statErr := file.Stat()
			err = statErr
			if err == nil && !exactPrivateMode(info.Mode(), 0700) {
				err = errors.New("private receipt directory has conflicting permissions")
			}
		}
		if err != nil {
			return nil, errors.Join(err, file.Close())
		}
	}
	if created {
		if err := durabilityCheck(check, "mkdir:"+name+":sync"); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		if err := file.Sync(); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		if err := durabilityCheck(check, "mkdir:"+name+":parent-sync"); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		if err := parent.Sync(); err != nil {
			return nil, errors.Join(err, file.Close())
		}
	}
	return file, nil
}

// parent opens each component without following symlinks. The returned handle
// remains pinned even if an external writer substitutes an ancestor pathname.
func (r *receiptRoot) parent(path string, create bool) (result *os.File, name string, resultErr error) {
	if !filepath.IsLocal(path) {
		return nil, "", fmt.Errorf("unsafe receipt path %q", path)
	}
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	dir, err := openReceiptDir(r.file, ".", false)
	if err != nil {
		return nil, "", err
	}
	for _, part := range parts[:len(parts)-1] {
		next, err := openReceiptDir(dir, part, create)
		closeErr := dir.Close()
		if err != nil || closeErr != nil {
			if next != nil {
				closeErr = errors.Join(closeErr, next.Close())
			}
			return nil, "", errors.Join(err, closeErr)
		}
		dir = next
	}
	return dir, parts[len(parts)-1], nil
}

func captureEvidence(parent *os.File, name string, sync bool) (node PublicationEvidence, resultErr error) {
	var st unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return node, err
	}
	node = PublicationEvidence{Device: uint64(st.Dev), Inode: st.Ino, Mode: uint32(st.Mode)}
	switch st.Mode & unix.S_IFMT {
	case unix.S_IFLNK:
		buf := make([]byte, 4096)
		n, err := unix.Readlinkat(int(parent.Fd()), name, buf)
		if err != nil {
			return node, err
		}
		if n == len(buf) {
			return node, errors.New("publication link exceeds limit")
		}
		node.Link = string(buf[:n])
	case unix.S_IFREG, unix.S_IFDIR:
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if st.Mode&unix.S_IFMT == unix.S_IFDIR {
			flags |= unix.O_DIRECTORY
		}
		fd, err := unix.Openat(int(parent.Fd()), name, flags, 0)
		if err != nil {
			return node, err
		}
		f := os.NewFile(uintptr(fd), name)
		defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
		var opened unix.Stat_t
		if err := unix.Fstat(fd, &opened); err != nil {
			return node, err
		}
		if opened.Dev != st.Dev || opened.Ino != st.Ino || opened.Mode != st.Mode {
			return node, errors.New("publication identity changed while opening")
		}
		if st.Mode&unix.S_IFMT == unix.S_IFDIR {
			children, err := f.ReadDir(-1)
			if err != nil {
				return node, err
			}
			if len(children) > 0 {
				node.Children = make(map[string]PublicationEvidence, len(children))
			}
			for _, child := range children {
				evidence, err := captureEvidence(f, child.Name(), sync)
				if err != nil {
					return node, err
				}
				node.Children[child.Name()] = evidence
			}
		} else {
			hash := sha256.New()
			n, err := io.Copy(hash, io.LimitReader(f, maxExtractedBytes+1))
			if err != nil {
				return node, err
			}
			if n > maxExtractedBytes {
				return node, errors.New("publication file exceeds limit")
			}
			node.Digest = hex.EncodeToString(hash.Sum(nil))
		}
		if sync {
			if err := f.Sync(); err != nil {
				return node, err
			}
		}
	default:
		return node, errors.New("unsupported publication file type")
	}
	var after unix.Stat_t
	if err := unix.Fstatat(int(parent.Fd()), name, &after, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return node, err
	}
	if after.Ino != st.Ino || after.Dev != st.Dev || after.Mode != st.Mode {
		return node, errors.New("publication identity changed during capture")
	}
	return node, nil
}

func (r *receiptRoot) evidence(path string, sync bool) (node PublicationEvidence, resultErr error) {
	parent, name, err := r.parent(path, false)
	if err != nil {
		return node, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	return captureEvidence(parent, name, sync)
}

func sameEvidence(a, b PublicationEvidence) bool { return reflect.DeepEqual(a, b) }

func rootIdentity(r *receiptRoot) (PublicationEvidence, error) {
	var st unix.Stat_t
	if err := unix.Fstat(int(r.file.Fd()), &st); err != nil {
		return PublicationEvidence{}, err
	}
	return PublicationEvidence{Device: uint64(st.Dev), Inode: st.Ino, Mode: uint32(st.Mode)}, nil
}

func (r *receiptRoot) validate(identity PublicationEvidence) error {
	current, err := openReceiptRoot(r.path, false)
	if err != nil {
		return err
	}
	actual, statErr := rootIdentity(current)
	closeErr := current.file.Close()
	if statErr != nil || closeErr != nil {
		return errors.Join(statErr, closeErr)
	}
	if !sameEvidence(identity, actual) {
		return fmt.Errorf("root identity changed at %s", r.path)
	}
	return nil
}

func receiptLock(ctx context.Context, dir *os.File, name string) (*os.File, error) {
	return receiptLockChecked(ctx, dir, name, nil)
}
func receiptLockChecked(ctx context.Context, dir *os.File, name string, check func(string) error) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_CREAT|unix.O_EXCL|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	created := err == nil
	if errors.Is(err, unix.EEXIST) {
		fd, err = unix.Openat(int(dir.Fd()), name, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("openat lock %s: %w", name, err)
	}
	file := os.NewFile(uintptr(fd), name)
	if created {
		if err := unix.Fchmod(fd, 0600); err != nil {
			return nil, errors.Join(err, file.Close())
		}
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !exactPrivateMode(info.Mode(), 0600) {
		return nil, errors.Join(errors.New("receipt lock has conflicting type/permissions"), err, file.Close())
	}
	for {
		if err = unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return nil, errors.Join(err, file.Close())
		}
		if err := durabilityCheck(check, "lock:"+name+":contended"); err != nil {
			return nil, errors.Join(err, file.Close())
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), file.Close())
		case <-time.After(10 * time.Millisecond):
		}
	}
	fail := func(err error) (*os.File, error) { return nil, errors.Join(err, closeReceiptLock(file)) }
	if err := durabilityCheck(check, "lock:"+name+":held"); err != nil {
		return fail(err)
	}
	if err := validateHeldLock(dir, name, file); err != nil {
		return fail(err)
	}
	if err := durabilityCheck(check, "lock:"+name+":sync"); err != nil {
		return fail(err)
	}
	if err := file.Sync(); err != nil {
		return fail(err)
	}
	if err := durabilityCheck(check, "lock:"+name+":parent-sync"); err != nil {
		return fail(err)
	}
	if err := dir.Sync(); err != nil {
		return fail(err)
	}
	return file, nil
}

func validateHeldLock(parent *os.File, name string, held *os.File) error {
	var actual, expected unix.Stat_t
	if err := unix.Fstat(int(held.Fd()), &expected); err != nil {
		return err
	}
	if err := unix.Fstatat(int(parent.Fd()), name, &actual, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if actual.Dev != expected.Dev || actual.Ino != expected.Ino || actual.Mode != expected.Mode || actual.Mode&unix.S_IFMT != unix.S_IFREG || actual.Mode&07777 != 0600 {
		return fmt.Errorf("held lock identity changed at %s", name)
	}
	return nil
}

func closeReceiptLock(f *os.File) error {
	if f == nil {
		return nil
	}
	return errors.Join(unix.Flock(int(f.Fd()), unix.LOCK_UN), f.Close())
}

func readRootBytes(root *receiptRoot, path string) (data []byte, resultErr error) {
	parent, name, err := root.parent(path, false)
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, parent.Close()) }()
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer func() { resultErr = errors.Join(resultErr, file.Close()) }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("invalid artifact provenance file")
	}
	return io.ReadAll(io.LimitReader(file, 1<<20))
}

func fileIdentity(file *os.File) (PublicationEvidence, error) {
	return rootIdentity(&receiptRoot{file: file})
}

func exactPrivateMode(mode os.FileMode, want os.FileMode) bool {
	return mode.Perm() == want && mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0
}
