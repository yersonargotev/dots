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
// present in source. It intentionally supports the simple TOML shape dots owns
// for agent settings: table headers plus single-line scalar and string-array
// assignments. Unknown or unsupported target syntax is treated as not aligned.
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

	sourceValues, err := parseSimpleTOML(sourceData)
	if err != nil {
		return false, fmt.Errorf("parse source TOML %s: %w", source, err)
	}
	targetValues, err := parseSimpleTOML(targetData)
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

func parseSimpleTOML(data []byte) (map[string]string, error) {
	values := map[string]string{}
	section := ""
	for lineNo, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripTOMLComment(rawLine))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[[") {
			return nil, fmt.Errorf("line %d: arrays of tables are unsupported", lineNo+1)
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
		canonical, err := canonicalTOMLValue(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo+1, err)
		}
		values[path] = canonical
	}
	return values, nil
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
