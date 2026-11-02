package deps

import (
	"encoding/binary"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinIncarnationRequiresVolumeAndObjectGuarantees(t *testing.T) {
	for _, kind := range []string{"valid", "truncated", "no-capability", "invalid-capability", "missing-uuid", "invalid-uuid-attribute", "missing-fileid", "zero-fileid", "wrong-fileid"} {
		t.Run(kind, func(t *testing.T) {
			v, o := make([]byte, 112), make([]byte, 32)
			put := binary.LittleEndian.PutUint32
			put(v, 112)
			put(o, 32)
			put(v[8:], attrVolumeCaps|unix.ATTR_VOL_UUID|attrVolumeAttributes)
			put(o[4:], attrFileID)
			put(v[24:], volumePathFromID)
			put(v[40:], volumePathFromID)
			put(v[76:], unix.ATTR_VOL_UUID)
			v[56] = 1
			binary.LittleEndian.PutUint64(o[24:], 42)
			switch kind {
			case "truncated":
				v = v[:111]
			case "no-capability":
				put(v[24:], 0)
			case "invalid-capability":
				put(v[40:], 0)
			case "missing-uuid":
				v[56] = 0
			case "invalid-uuid-attribute":
				put(v[76:], 0)
			case "missing-fileid":
				put(o[4:], 0)
			case "zero-fileid":
				binary.LittleEndian.PutUint64(o[24:], 0)
			case "wrong-fileid":
				binary.LittleEndian.PutUint64(o[24:], 43)
			}
			got, err := darwinIncarnation(v, o, 42)
			if kind == "valid" {
				if err != nil || got != "darwin:01000000000000000000000000000000:42" {
					t.Fatalf("invalid identity %s %v", got, err)
				}
			} else if err == nil {
				t.Fatal("accepted weak object identity")
			}
		})
	}
}
