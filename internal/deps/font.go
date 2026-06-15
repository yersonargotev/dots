package deps

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// ScanFonts reports whether any installed font file under the given root
// directories matches the declared glob. The match is case-insensitive against
// the file name and the scan is recursive. A missing directory or a permission
// error on a root is treated as "not found here" rather than an error, so a
// Dependency check never aborts on an unreadable font directory.
func ScanFonts(roots []string, match string) bool {
	pattern := strings.ToLower(strings.TrimSpace(match))
	if pattern == "" {
		return false
	}

	for _, root := range roots {
		found := false
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// Unreadable directory or entry: skip it, keep scanning.
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if ok, _ := filepath.Match(pattern, strings.ToLower(d.Name())); ok {
				found = true
				return fs.SkipAll
			}
			return nil
		})
		if found {
			return true
		}
	}
	return false
}
