package deps

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func openIdentityAt(parent int, name string) (int, error) {
	return unix.Openat(parent, name, unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
}

func filesystemIncarnation(file *os.File, _ uint64) (string, error) {
	fd := int(file.Fd())
	handle, _, err := unix.NameToHandleAt(fd, "", unix.AT_EMPTY_PATH)
	if err != nil {
		return "", err
	}
	var st unix.Statx_t
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID_UNIQUE, &st); err != nil {
		return "", err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	return linuxIncarnation(strings.TrimSpace(string(boot)), st.Mask, st.Mnt_id, handle.Type(), handle.Bytes())
}

func linuxIncarnation(boot string, mask uint32, mount uint64, kind int32, handle []byte) (string, error) {
	compact := strings.ReplaceAll(boot, "-", "")
	uuid, err := hex.DecodeString(compact)
	if err != nil || len(uuid) != 16 || compact == strings.Repeat("0", 32) || strings.ToLower(compact) != compact {
		return "", errors.New("missing or invalid Linux boot UUID")
	}
	if boot != fmt.Sprintf("%s-%s-%s-%s-%s", compact[:8], compact[8:12], compact[12:16], compact[16:20], compact[20:]) {
		return "", errors.New("missing or invalid Linux boot UUID")
	}
	if mask&unix.STATX_MNT_ID_UNIQUE == 0 || mount == 0 || len(handle) == 0 || len(handle) > 128 {
		return "", errors.New("Linux requires export handles and STATX_MNT_ID_UNIQUE (kernel 6.8+)")
	}
	return fmt.Sprintf("linux:%s:%d:%d:%x", compact, mount, kind, handle), nil
}
