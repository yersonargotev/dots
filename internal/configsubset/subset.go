// Package configsubset compares dots-owned configuration fragments against
// co-owned agent configuration files.
package configsubset

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// JSONFileContains reports whether target contains every value present in
// source. Object keys are subset-owned and array items may be a subset of the
// target array.
func JSONFileContains(target, source string) (bool, error) {
	sourceData, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", source, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", target, err)
	}

	var sourceValue, targetValue any
	if err := json.Unmarshal(sourceData, &sourceValue); err != nil {
		return false, fmt.Errorf("parse source JSON %s: %w", source, err)
	}
	if err := json.Unmarshal(targetData, &targetValue); err != nil {
		return false, nil
	}
	return jsonContains(targetValue, sourceValue), nil
}

// TOMLFileContains reports whether target contains every scalar/array setting
// present in source. The source is parsed strictly because it is the dots-owned
// fragment. The target is parsed only for source-owned paths, so unrelated TOML
// added by Codex or provisioners cannot make the dots-owned subset drift.
func TOMLFileContains(target, source string) (bool, error) {
	sourceData, err := os.ReadFile(source)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", source, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", target, err)
	}

	sourceValues, err := parseSimpleTOML(sourceData, nil)
	if err != nil {
		return false, fmt.Errorf("parse source TOML %s: %w", source, err)
	}
	targetValues, err := parseSimpleTOML(targetData, sourceValuePaths(sourceValues))
	if err != nil {
		return false, nil
	}
	for key, sourceValue := range sourceValues {
		if targetValues[key] != sourceValue {
			return false, nil
		}
	}
	return true, nil
}

// MergeTOMLFile appends missing source-owned array-of-table blocks to target.
// It is intentionally narrower than a full TOML formatter: dots only needs this
// for co-owned Codex config hooks, where preserving user/Codex-owned settings is
// more important than reformatting the whole file. Scalar/table value changes
// remain conflicts unless a caller adds a dedicated migration.
func MergeTOMLFile(target, source string) error {
	contains, err := TOMLFileContains(target, source)
	if err != nil {
		return err
	}
	if contains {
		return nil
	}

	sourceData, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("stat %s: %w", target, err)
	}
	blocks, err := missingArrayTableBlocks(targetData, sourceData)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return fmt.Errorf("source-owned TOML differences cannot be merged safely")
	}

	merged := append([]byte(nil), targetData...)
	if len(merged) > 0 && merged[len(merged)-1] != '\n' {
		merged = append(merged, '\n')
	}
	merged = append(merged, '\n')
	for _, block := range blocks {
		merged = append(merged, strings.TrimRight(block, "\n")...)
		merged = append(merged, '\n', '\n')
	}
	if err := os.WriteFile(target, merged, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	contains, err = TOMLFileContains(target, source)
	if err != nil {
		return err
	}
	if !contains {
		return fmt.Errorf("merged TOML target still misses source-owned values")
	}
	return nil
}

func jsonContains(target, source any) bool {
	switch sourceTyped := source.(type) {
	case map[string]any:
		targetTyped, ok := target.(map[string]any)
		if !ok {
			return false
		}
		for key, sourceChild := range sourceTyped {
			targetChild, ok := targetTyped[key]
			if !ok || !jsonContains(targetChild, sourceChild) {
				return false
			}
		}
		return true
	case []any:
		targetTyped, ok := target.([]any)
		if !ok {
			return false
		}
		for _, sourceItem := range sourceTyped {
			found := false
			for _, targetItem := range targetTyped {
				if jsonContains(targetItem, sourceItem) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(target, source)
	}
}

func sourceValuePaths(values map[string]string) map[string]struct{} {
	paths := make(map[string]struct{}, len(values))
	for path := range values {
		paths[path] = struct{}{}
	}
	return paths
}

func parseSimpleTOML(data []byte, wanted map[string]struct{}) (map[string]string, error) {
	values := map[string]string{}
	section := ""
	for lineNo, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripTOMLComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[[") {
			if !strings.HasSuffix(line, "]]") {
				return nil, fmt.Errorf("line %d: malformed array-of-table header", lineNo+1)
			}
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
			if section == "" {
				return nil, fmt.Errorf("line %d: empty array-of-table header", lineNo+1)
			}
			continue
		}
		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("line %d: malformed table header", lineNo+1)
			}
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]"))
			if section == "" {
				return nil, fmt.Errorf("line %d: empty table header", lineNo+1)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			if wanted != nil {
				continue
			}
			return nil, fmt.Errorf("line %d: expected key/value assignment", lineNo+1)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNo+1)
		}
		path := key
		if section != "" {
			path = section + "." + key
		}
		if wanted != nil {
			if _, ok := wanted[path]; !ok {
				continue
			}
		}
		canonical, err := canonicalTOMLValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
		}
		values[path] = canonical
	}
	return values, nil
}

func missingArrayTableBlocks(targetData, sourceData []byte) ([]string, error) {
	sourceValues, err := parseSimpleTOML(sourceData, nil)
	if err != nil {
		return nil, err
	}
	targetValues, err := parseSimpleTOML(targetData, sourceValuePaths(sourceValues))
	if err != nil {
		return nil, err
	}
	missing := map[string]struct{}{}
	for path, sourceValue := range sourceValues {
		if targetValues[path] != sourceValue {
			missing[path] = struct{}{}
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}

	var blocks []string
	lines := strings.Split(string(sourceData), "\n")
	for i := 0; i < len(lines); {
		line := strings.TrimSpace(stripTOMLComment(lines[i]))
		if !strings.HasPrefix(line, "[[") || !strings.HasSuffix(line, "]]") {
			i++
			continue
		}
		start := i
		section := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "[["), "]]"))
		i++
		blockValues := map[string]string{}
		for i < len(lines) {
			next := strings.TrimSpace(stripTOMLComment(lines[i]))
			if strings.HasPrefix(next, "[") && strings.HasSuffix(next, "]") {
				break
			}
			key, value, ok := strings.Cut(next, "=")
			if ok {
				path := section + "." + strings.TrimSpace(key)
				canonical, err := canonicalTOMLValue(strings.TrimSpace(value))
				if err != nil {
					return nil, err
				}
				blockValues[path] = canonical
			}
			i++
		}
		appendBlock := false
		for path := range blockValues {
			if _, ok := missing[path]; ok {
				appendBlock = true
				break
			}
		}
		if appendBlock {
			blocks = append(blocks, strings.Join(lines[start:i], "\n"))
		}
	}
	return blocks, nil
}

func stripTOMLComment(line string) string {
	inString := false
	escaped := false
	for i, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == '#' && !inString:
			return line[:i]
		}
	}
	return line
}

func canonicalTOMLValue(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("empty value")
	}
	if strings.HasPrefix(value, "[") {
		return canonicalTOMLStringArray(value)
	}
	if strings.HasPrefix(value, "\"") {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		encoded, _ := json.Marshal(unquoted)
		return string(encoded), nil
	}
	lower := strings.ToLower(value)
	if lower == "true" || lower == "false" {
		return lower, nil
	}
	return value, nil
}

func canonicalTOMLStringArray(value string) (string, error) {
	if !strings.HasSuffix(value, "]") {
		return "", fmt.Errorf("multi-line arrays are unsupported")
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if inner == "" {
		return "[]", nil
	}
	parts, err := splitTOMLArray(inner)
	if err != nil {
		return "", err
	}
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item, err := strconv.Unquote(strings.TrimSpace(part))
		if err != nil {
			return "", err
		}
		items = append(items, item)
	}
	encoded, _ := json.Marshal(items)
	return string(encoded), nil
}

func splitTOMLArray(inner string) ([]string, error) {
	var parts []string
	start := 0
	inString := false
	escaped := false
	for i, r := range inner {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case r == ',' && !inString:
			parts = append(parts, inner[start:i])
			start = i + 1
		}
	}
	if inString {
		return nil, fmt.Errorf("unterminated string in array")
	}
	parts = append(parts, inner[start:])
	return parts, nil
}
