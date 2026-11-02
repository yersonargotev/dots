package deps

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	attrReturned         = uint32(0x80000000)
	attrFileID           = uint32(0x02000000)
	attrVolumeCaps       = uint32(0x00020000)
	attrVolumeAttributes = uint32(0x40000000)
	volumePathFromID     = uint32(0x00004000)
)

func openIdentityAt(parent int, name string) (int, error) {
	// O_SYMLINK opens the link itself, including dangling links. O_NOFOLLOW
	// would instead reject it, even when combined with O_SYMLINK.
	return unix.Openat(parent, name, unix.O_RDONLY|unix.O_SYMLINK|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
}

func receiptGetAttributes(file *os.File, common, volume uint32, buf []byte) error {
	attrs := struct {
		Count, Reserved                       uint16
		Common, Volume, Directory, File, Fork uint32
	}{Count: 5, Common: common, Volume: volume}
	_, _, errno := unix.Syscall6(unix.SYS_FGETATTRLIST, file.Fd(), uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func filesystemIncarnation(file *os.File, inode uint64) (string, error) {
	// Returned-attribute bitmaps make unsupported/truncated data an error,
	// rather than interpreting zero-filled compatibility fields as evidence.
	volume := make([]byte, 112)
	want := attrVolumeCaps | unix.ATTR_VOL_UUID | attrVolumeAttributes
	if err := receiptGetAttributes(file, attrReturned, want|0x80000000, volume); err != nil {
		return "", err
	}
	object := make([]byte, 32)
	if err := receiptGetAttributes(file, attrReturned|attrFileID, 0, object); err != nil {
		return "", err
	}
	return darwinIncarnation(volume, object, inode)
}

func darwinIncarnation(volume, object []byte, inode uint64) (string, error) {
	u32 := binary.LittleEndian.Uint32
	if len(volume) != 112 || len(object) != 32 || u32(volume) != 112 || u32(object) != 32 || u32(volume[8:])&(attrVolumeCaps|unix.ATTR_VOL_UUID|attrVolumeAttributes) != (attrVolumeCaps|unix.ATTR_VOL_UUID|attrVolumeAttributes) || u32(object[4:])&attrFileID == 0 {
		return "", errors.New("missing Darwin volume/object attributes")
	}
	// attr.h: PATH_FROM_ID guarantees persistent, non-recycled object IDs.
	// capabilities[format], valid[format], then valid_attributes.volume.
	if u32(volume[24:])&volumePathFromID == 0 || u32(volume[40:])&volumePathFromID == 0 || u32(volume[76:])&unix.ATTR_VOL_UUID == 0 {
		return "", errors.New("volume lacks persistent non-recycled Darwin object IDs")
	}
	uuid := volume[56:72]
	nonzero := false
	for _, b := range uuid {
		nonzero = nonzero || b != 0
	}
	id := binary.LittleEndian.Uint64(object[24:])
	if !nonzero || id == 0 || id != inode {
		return "", errors.New("invalid Darwin volume UUID or FILEID")
	}
	return fmt.Sprintf("darwin:%x:%d", uuid, id), nil
}
