// Package textblock manages marker-delimited text blocks without doing any file
// I/O. Callers own where the resulting content is read from or written to.
package textblock

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
)

const (
	DotsManagedBlockStart = "# >>> dots managed block >>>"
	DotsManagedBlockEnd   = "# <<< dots managed block <<<"
)

// DotsManagedMarkers returns the exact portable markers used by partial text
// ownership in the Install Manifest.
func DotsManagedMarkers() Markers {
	return Markers{Start: DotsManagedBlockStart, End: DotsManagedBlockEnd}
}

// Markers identify the start and end comments around a managed block.
type Markers struct {
	Start string
	End   string
}

// Upsert replaces an existing marker-delimited block or appends a new one. Any
// legacy marker pairs are migrated to primary; if multiple known blocks exist,
// the first becomes the single managed block and the rest are removed.
func Upsert(content string, primary Markers, block string, legacy ...Markers) (string, error) {
	if primary.Start == "" || primary.End == "" {
		return "", fmt.Errorf("primary markers are required")
	}

	managed := wrap(primary, strings.TrimSpace(block))
	markers := append([]Markers{primary}, legacy...)
	ranges, err := findRanges(content, markers)
	if err != nil {
		return "", err
	}
	if len(ranges) == 0 {
		return appendBlock(content, managed), nil
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	var b strings.Builder
	cursor := 0
	for i, r := range ranges {
		if r.start < cursor {
			continue
		}
		b.WriteString(content[cursor:r.start])
		if i == 0 {
			b.WriteString(managed)
		}
		cursor = r.end
	}
	b.WriteString(content[cursor:])
	return b.String(), nil
}

// ExtractBody returns the trimmed body of the first marker-delimited block.
func ExtractBody(content string, markers Markers) (string, bool, error) {
	if markers.Start == "" || markers.End == "" {
		return "", false, fmt.Errorf("markers are required")
	}
	start := strings.Index(content, markers.Start)
	if start < 0 {
		return "", false, nil
	}
	bodyStart := start + len(markers.Start)
	end := strings.Index(content[bodyStart:], markers.End)
	if end < 0 {
		return "", false, fmt.Errorf("marker %q is missing closing marker %q", markers.Start, markers.End)
	}
	return strings.TrimSpace(content[bodyStart : bodyStart+end]), true, nil
}

// Remove deletes every marker-delimited block matching the provided markers.
func Remove(content string, markers ...Markers) (string, error) {
	if len(markers) == 0 {
		return content, nil
	}
	ranges, err := findRanges(content, markers)
	if err != nil {
		return "", err
	}
	if len(ranges) == 0 {
		return content, nil
	}

	sort.Slice(ranges, func(i, j int) bool { return ranges[i].start < ranges[j].start })
	var b strings.Builder
	cursor := 0
	for _, r := range ranges {
		if r.start < cursor {
			continue
		}
		b.WriteString(content[cursor:r.start])
		cursor = r.end
	}
	b.WriteString(content[cursor:])
	return b.String(), nil
}

type blockRange struct {
	start int
	end   int
}

// Reconciliation describes a strict three-way update of one recorded block.
type Reconciliation struct {
	Content    []byte
	Changed    bool
	Compatible bool
}

// ValidOwnedSource reports whether source is exactly one complete marked block.
func ValidOwnedSource(source []byte, markers Markers) bool {
	b, ok := parseOwned(source, markers)
	return ok && b.start == 0 && b.end == len(source)
}

// ReconcileOwned replaces the exact recorded initial block and preserves every
// byte before and after it. Ambiguous placement or markers fail closed.
func ReconcileOwned(live, previous, current []byte, markers Markers) Reconciliation {
	liveBlock, ok := parseOwned(live, markers)
	if !ok || !ValidOwnedSource(previous, markers) || !ValidOwnedSource(current, markers) ||
		!bytes.Equal(live[liveBlock.start:liveBlock.end], previous) {
		return Reconciliation{}
	}
	content := replaceOwned(live, liveBlock.start, liveBlock.end, current)
	return Reconciliation{Content: content, Changed: !bytes.Equal(content, live), Compatible: true}
}

// MigrateLegacyOwned converts an exact legacy whole-file contribution,
// optionally followed by appended bytes, into the current marked block.
func MigrateLegacyOwned(live, previous, current []byte, markers Markers) Reconciliation {
	if !ValidOwnedSource(current, markers) || len(previous) == 0 || !bytes.HasPrefix(live, previous) {
		return Reconciliation{}
	}
	content := append(append([]byte(nil), current...), live[len(previous):]...)
	return Reconciliation{Content: content, Changed: !bytes.Equal(content, live), Compatible: true}
}

// RemoveOwned subtracts only the exact recorded block. Empty is true only when
// no external bytes remain, allowing removal of the physical container.
func RemoveOwned(live, owned []byte, markers Markers) (content []byte, changed, empty, compatible bool) {
	b, ok := parseOwned(live, markers)
	if !ok || !ValidOwnedSource(owned, markers) || !bytes.Equal(live[b.start:b.end], owned) {
		return nil, false, false, false
	}
	content = replaceOwned(live, b.start, b.end, nil)
	return content, true, len(content) == 0, true
}

func parseOwned(content []byte, markers Markers) (blockRange, bool) {
	if markers.Start == "" || markers.End == "" {
		return blockRange{}, false
	}
	var found blockRange
	starts, ends := 0, 0
	for offset := 0; offset < len(content); {
		next := bytes.IndexByte(content[offset:], '\n')
		lineEnd := len(content)
		if next >= 0 {
			lineEnd = offset + next + 1
		}
		text := strings.TrimSuffix(string(content[offset:lineEnd]), "\n")
		switch text {
		case markers.Start:
			starts++
			if starts == 1 {
				found.start = offset
			}
		case markers.End:
			ends++
			if ends == 1 {
				found.end = lineEnd
			}
		}
		offset = lineEnd
	}
	if starts != 1 || ends != 1 || found.end <= found.start || !commentsOnly(content[:found.start]) {
		return blockRange{}, false
	}
	return found, true
}

func commentsOnly(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return false
		}
	}
	return true
}

func replaceOwned(content []byte, start, end int, replacement []byte) []byte {
	result := make([]byte, 0, len(content)-(end-start)+len(replacement))
	result = append(result, content[:start]...)
	result = append(result, replacement...)
	result = append(result, content[end:]...)
	return result
}

func findRanges(content string, markers []Markers) ([]blockRange, error) {
	var ranges []blockRange
	for _, m := range markers {
		if m.Start == "" || m.End == "" {
			return nil, fmt.Errorf("markers are required")
		}
		searchFrom := 0
		for {
			start := strings.Index(content[searchFrom:], m.Start)
			if start == -1 {
				break
			}
			start += searchFrom
			endSearchFrom := start + len(m.Start)
			end := strings.Index(content[endSearchFrom:], m.End)
			if end == -1 {
				return nil, fmt.Errorf("marker %q is missing closing marker %q", m.Start, m.End)
			}
			end += endSearchFrom + len(m.End)
			ranges = append(ranges, blockRange{start: start, end: end})
			searchFrom = end
		}
	}
	return ranges, nil
}

func wrap(markers Markers, block string) string {
	return markers.Start + "\n" + block + "\n" + markers.End
}

func appendBlock(content, block string) string {
	if strings.TrimSpace(content) == "" {
		return block + "\n"
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + "\n" + block + "\n"
}
