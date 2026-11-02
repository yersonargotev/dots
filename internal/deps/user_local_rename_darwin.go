package deps

import "golang.org/x/sys/unix"

func promoteReceiptDirectory(from int, source string, to int, destination string) error {
	return unix.RenameatxNp(from, source, to, destination, unix.RENAME_EXCL)
}
