package deps

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxIncarnationRequiresUniqueMountAndBoot(t *testing.T) {
	boot := "11111111-2222-3333-4444-555555555555"
	base, err := linuxIncarnation(boot, unix.STATX_MNT_ID_UNIQUE, 42, 1, []byte{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"new-boot", "new-mount", "ordinary-mount", "zero-mount", "bad-boot", "zero-boot", "empty-handle", "oversized-handle"} {
		t.Run(kind, func(t *testing.T) {
			b, mask, mount, handle := boot, uint32(unix.STATX_MNT_ID_UNIQUE), uint64(42), []byte{1, 2}
			switch kind {
			case "new-boot":
				b = "21111111-2222-3333-4444-555555555555"
			case "new-mount":
				mount = 43
			case "ordinary-mount":
				mask = unix.STATX_MNT_ID
			case "zero-mount":
				mount = 0
			case "bad-boot":
				b = "bad"
			case "zero-boot":
				b = "00000000-0000-0000-0000-000000000000"
			case "empty-handle":
				handle = nil
			case "oversized-handle":
				handle = make([]byte, 129)
			}
			got, err := linuxIncarnation(b, mask, mount, 1, handle)
			if kind == "new-boot" || kind == "new-mount" {
				if err != nil || got == base {
					t.Fatalf("epoch collision: %s %v", got, err)
				}
			} else if err == nil {
				t.Fatal("accepted weak Linux identity")
			}
		})
	}
}
