// Package textblock manages marker-delimited text blocks without doing any file
// I/O. Callers own where the resulting content is read from or written to.
package textblock

import (
	"fmt"
	"sort"
	"strings"
)

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
