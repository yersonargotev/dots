package configsubset

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// TOMLReconciliation describes a safe three-state update from a previously
// recorded dots-owned TOML contribution to the current Source of Truth.
type TOMLReconciliation struct {
	Content    []byte
	Changed    bool
	Compatible bool
}

type tomlEntry struct {
	path      []string
	tableKey  string
	exprStart int
	exprEnd   int
	lineStart int
	lineEnd   int
	raw       []byte
}

type tomlSection struct {
	path      []string
	headerRaw []byte
	insertAt  int
}

type tomlDocument struct {
	values         map[string]any
	entries        map[string]tomlEntry
	entryOrder     []string
	sections       map[string]tomlSection
	hasArrayTables bool
}

type tomlEdit struct {
	start   int
	end     int
	content []byte
}

// AnalyzeTOML compares a live target with a source-owned TOML fragment. Tables
// are containers whose declared values are owned individually; scalar, array,
// and inline-table values are atomic. Target-only keys and tables are allowed.
func AnalyzeTOML(targetData, sourceData []byte) (JSONFileRelation, error) {
	source, err := parseTOMLDocument(sourceData)
	if err != nil {
		return JSONFileRelation{}, fmt.Errorf("parse source TOML: %w", err)
	}
	if source.hasArrayTables {
		merged, mergeErr := mergeLegacyTOML(targetData, sourceData)
		if mergeErr != nil {
			return JSONFileRelation{}, nil
		}
		return JSONFileRelation{Contains: bytes.Equal(merged, targetData), Mergeable: !bytes.Equal(merged, targetData)}, nil
	}
	result, err := reconcileParsedTOML(targetData, nil, source)
	if err != nil {
		return JSONFileRelation{}, err
	}
	return JSONFileRelation{
		Contains:  result.Compatible && !result.Changed,
		Mergeable: result.Compatible && result.Changed,
	}, nil
}

// TOMLFileContains reports whether target contains every source-owned value.
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
	relation, err := AnalyzeTOML(targetData, sourceData)
	if err != nil {
		return false, fmt.Errorf("analyze source TOML %s: %w", source, err)
	}
	return relation.Contains, nil
}

// MergeTOML adds missing source-owned values while preserving all unrelated
// target bytes. Existing incompatible values are never overwritten.
func MergeTOML(targetData, sourceData []byte) ([]byte, error) {
	source, err := parseTOMLDocument(sourceData)
	if err != nil {
		return nil, fmt.Errorf("parse source TOML: %w", err)
	}
	if source.hasArrayTables {
		return mergeLegacyTOML(targetData, sourceData)
	}
	result, err := reconcileParsedTOML(targetData, nil, source)
	if err != nil {
		return nil, err
	}
	if !result.Compatible {
		return nil, fmt.Errorf("source-owned TOML differences cannot be merged safely")
	}
	return result.Content, nil
}

// MergeTOMLFile applies MergeTOML without changing the target's permissions.
func MergeTOMLFile(target, source string) error {
	sourceData, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	targetData, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	merged, err := MergeTOML(targetData, sourceData)
	if err != nil {
		return fmt.Errorf("merge target TOML %s with source TOML %s: %w", target, source, err)
	}
	if bytes.Equal(merged, targetData) {
		return nil
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("stat %s: %w", target, err)
	}
	if err := os.WriteFile(target, merged, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

// ReconcileTOML adds new values, removes retired values that still match their
// recorded contribution, and replaces changed baseline values only when the
// live target still matches either the previous or current atomic value.
func ReconcileTOML(targetData, previousData, currentData []byte) (TOMLReconciliation, error) {
	previous, err := parseTOMLDocument(previousData)
	if err != nil {
		return TOMLReconciliation{}, fmt.Errorf("parse previous owned TOML: %w", err)
	}
	current, err := parseTOMLDocument(currentData)
	if err != nil {
		return TOMLReconciliation{}, fmt.Errorf("parse current owned TOML: %w", err)
	}
	if previous.hasArrayTables || current.hasArrayTables {
		if !previous.hasArrayTables && current.hasArrayTables {
			ordinary, reconcileErr := reconcileParsedTOML(targetData, previous, current)
			if reconcileErr != nil || !ordinary.Compatible {
				return TOMLReconciliation{}, reconcileErr
			}
			merged, mergeErr := mergeLegacyTOML(ordinary.Content, currentData)
			if mergeErr != nil {
				return TOMLReconciliation{}, nil
			}
			return TOMLReconciliation{Content: merged, Changed: !bytes.Equal(merged, targetData), Compatible: true}, nil
		}
		if !bytes.Equal(previousData, currentData) {
			return TOMLReconciliation{}, nil
		}
		merged, mergeErr := mergeLegacyTOML(targetData, currentData)
		if mergeErr != nil {
			return TOMLReconciliation{}, nil
		}
		return TOMLReconciliation{Content: merged, Changed: !bytes.Equal(merged, targetData), Compatible: true}, nil
	}
	return reconcileParsedTOML(targetData, previous, current)
}

// RemoveTOML subtracts the exact recorded dots-owned TOML contribution. It
// preserves target-only bytes and fails closed when a formerly owned value has
// changed. Empty is false when comments remain, so uninstall never discards
// unclassified human or application notes.
func RemoveTOML(targetData, ownedData []byte) (content []byte, changed, empty, compatible bool, err error) {
	owned, parseErr := parseTOMLDocument(ownedData)
	if parseErr != nil {
		return nil, false, false, false, fmt.Errorf("parse owned TOML: %w", parseErr)
	}
	if owned.hasArrayTables {
		return nil, false, false, false, nil
	}
	result, reconcileErr := reconcileParsedTOML(targetData, owned, &tomlDocument{
		values:     map[string]any{},
		entries:    map[string]tomlEntry{},
		sections:   map[string]tomlSection{"": {}},
		entryOrder: nil,
	})
	if reconcileErr != nil {
		return nil, false, false, false, reconcileErr
	}
	if !result.Compatible {
		return nil, false, false, false, nil
	}
	document, parseErr := parseTOMLDocument(result.Content)
	if parseErr != nil {
		return nil, false, false, false, fmt.Errorf("parse TOML after removing owned content: %w", parseErr)
	}
	empty = tomlValueEmpty(document.values) && !tomlContainsComments(result.Content)
	return result.Content, result.Changed, empty, true, nil
}

func reconcileParsedTOML(targetData []byte, previous, current *tomlDocument) (TOMLReconciliation, error) {
	target, err := parseTOMLDocument(targetData)
	if err != nil {
		return TOMLReconciliation{}, nil
	}
	if previous == nil {
		previous = &tomlDocument{values: map[string]any{}, entries: map[string]tomlEntry{}, sections: map[string]tomlSection{}}
	}

	var edits []tomlEdit
	additions := map[string][]tomlEntry{}
	var additionOrder []string
	for _, pathKey := range previous.entryOrder {
		previousEntry := previous.entries[pathKey]
		previousValue, ok := tomlValueAt(previous.values, previousEntry.path)
		if !ok {
			return TOMLReconciliation{}, fmt.Errorf("previous owned TOML path %q has no semantic value", displayTOMLPath(previousEntry.path))
		}
		targetValue, targetExists := tomlValueAt(target.values, previousEntry.path)
		targetEntry, targetHasExpression := target.entries[pathKey]
		if !targetExists || !targetHasExpression {
			return TOMLReconciliation{}, nil
		}
		currentEntry, remainsOwned := current.entries[pathKey]
		if !remainsOwned {
			if !tomlValuesEqual(targetValue, previousValue) {
				return TOMLReconciliation{}, nil
			}
			edits = append(edits, tomlEdit{start: targetEntry.lineStart, end: targetEntry.lineEnd})
			continue
		}
		currentValue, ok := tomlValueAt(current.values, currentEntry.path)
		if !ok {
			return TOMLReconciliation{}, fmt.Errorf("current owned TOML path %q has no semantic value", displayTOMLPath(currentEntry.path))
		}
		switch {
		case tomlValuesEqual(targetValue, currentValue):
			continue
		case tomlValuesEqual(targetValue, previousValue):
			if targetEntry.tableKey != currentEntry.tableKey {
				return TOMLReconciliation{}, nil
			}
			edits = append(edits, tomlEdit{start: targetEntry.exprStart, end: targetEntry.exprEnd, content: append([]byte(nil), currentEntry.raw...)})
		default:
			return TOMLReconciliation{}, nil
		}
	}

	for _, pathKey := range current.entryOrder {
		if _, existed := previous.entries[pathKey]; existed {
			continue
		}
		currentEntry := current.entries[pathKey]
		currentValue, ok := tomlValueAt(current.values, currentEntry.path)
		if !ok {
			return TOMLReconciliation{}, fmt.Errorf("current owned TOML path %q has no semantic value", displayTOMLPath(currentEntry.path))
		}
		targetValue, targetExists := tomlValueAt(target.values, currentEntry.path)
		if targetExists {
			if !tomlValuesEqual(targetValue, currentValue) {
				return TOMLReconciliation{}, nil
			}
			continue
		}
		if currentEntry.tableKey != "" {
			if _, tableExists := tomlValueAt(target.values, current.sections[currentEntry.tableKey].path); tableExists {
				if _, sectionExists := target.sections[currentEntry.tableKey]; !sectionExists {
					dotted, ok := renderDottedTOMLEntry(currentEntry.path, currentEntry.raw)
					if !ok {
						return TOMLReconciliation{}, nil
					}
					currentEntry.tableKey = ""
					currentEntry.raw = dotted
				}
			}
		}
		if _, exists := additions[currentEntry.tableKey]; !exists {
			additionOrder = append(additionOrder, currentEntry.tableKey)
		}
		additions[currentEntry.tableKey] = append(additions[currentEntry.tableKey], currentEntry)
	}

	for _, tableKey := range additionOrder {
		entries := additions[tableKey]
		section, exists := target.sections[tableKey]
		var addition []byte
		if exists {
			addition = joinTOMLEntries(entries)
			edits = append(edits, tomlEdit{start: section.insertAt, end: section.insertAt, content: formatTOMLInsertion(targetData, section.insertAt, addition, false)})
			continue
		}
		sourceSection, ok := current.sections[tableKey]
		if !ok || tableKey == "" {
			return TOMLReconciliation{}, fmt.Errorf("current owned TOML section %q is missing", displayTOMLPath(entries[0].path[:len(entries[0].path)-1]))
		}
		addition = append(addition, sourceSection.headerRaw...)
		addition = append(addition, '\n')
		addition = append(addition, joinTOMLEntries(entries)...)
		edits = append(edits, tomlEdit{start: len(targetData), end: len(targetData), content: formatTOMLInsertion(targetData, len(targetData), addition, true)})
	}

	content, err := applyTOMLEdits(targetData, edits)
	if err != nil {
		return TOMLReconciliation{}, err
	}
	final, err := parseTOMLDocument(content)
	if err != nil {
		return TOMLReconciliation{}, nil
	}
	for _, pathKey := range current.entryOrder {
		entry := current.entries[pathKey]
		want, _ := tomlValueAt(current.values, entry.path)
		got, ok := tomlValueAt(final.values, entry.path)
		if !ok || !tomlValuesEqual(got, want) {
			return TOMLReconciliation{}, nil
		}
	}
	return TOMLReconciliation{Content: content, Changed: !bytes.Equal(content, targetData), Compatible: true}, nil
}

func parseTOMLDocument(data []byte) (*tomlDocument, error) {
	values := map[string]any{}
	if err := toml.Unmarshal(data, &values); err != nil {
		return nil, err
	}
	document := &tomlDocument{
		values:   values,
		entries:  map[string]tomlEntry{},
		sections: map[string]tomlSection{"": {insertAt: len(data)}},
	}
	parser := unstable.Parser{}
	parser.Reset(data)
	var currentTable []string
	currentTableKey := ""
	skipArrayTable := false
	activeSectionKey := ""
	hasActiveSection := true
	for parser.NextExpression() {
		expression := parser.Expression()
		switch expression.Kind {
		case unstable.Table, unstable.ArrayTable:
			path := tomlNodeKey(expression)
			lineStart, lineEnd := tomlLineBounds(data, tomlNodeOffset(expression))
			if hasActiveSection {
				previous := document.sections[activeSectionKey]
				previous.insertAt = tomlInsertionPoint(data, lineStart)
				document.sections[activeSectionKey] = previous
			}
			currentTable = path
			currentTableKey = tomlPathKey(path)
			skipArrayTable = expression.Kind == unstable.ArrayTable
			if skipArrayTable {
				document.hasArrayTables = true
				hasActiveSection = false
				continue
			}
			section := tomlSection{
				path:      append([]string(nil), path...),
				headerRaw: bytes.TrimRight(data[lineStart:lineEnd], "\r\n"),
				insertAt:  len(data),
			}
			document.sections[currentTableKey] = section
			activeSectionKey = currentTableKey
			hasActiveSection = true
		case unstable.KeyValue:
			if skipArrayTable {
				continue
			}
			path := append(append([]string(nil), currentTable...), tomlNodeKey(expression)...)
			pathKey := tomlPathKey(path)
			if _, duplicate := document.entries[pathKey]; duplicate {
				return nil, fmt.Errorf("duplicate TOML ownership path %q", displayTOMLPath(path))
			}
			exprStart := int(expression.Raw.Offset)
			exprEnd := exprStart + int(expression.Raw.Length)
			lineStart, lineEnd := tomlExpressionLineBounds(data, exprStart, exprEnd)
			entry := tomlEntry{
				path:      path,
				tableKey:  currentTableKey,
				exprStart: exprStart,
				exprEnd:   exprEnd,
				lineStart: lineStart,
				lineEnd:   lineEnd,
				raw:       append([]byte(nil), data[exprStart:exprEnd]...),
			}
			document.entries[pathKey] = entry
			document.entryOrder = append(document.entryOrder, pathKey)
		}
	}
	if err := parser.Error(); err != nil {
		return nil, err
	}
	if hasActiveSection {
		last := document.sections[activeSectionKey]
		last.insertAt = len(data)
		document.sections[activeSectionKey] = last
	}
	return document, nil
}

func tomlNodeKey(node *unstable.Node) []string {
	iterator := node.Key()
	var key []string
	for iterator.Next() {
		key = append(key, string(iterator.Node().Data))
	}
	return key
}

func tomlNodeOffset(node *unstable.Node) int {
	key := node.Key()
	if !key.Next() {
		return 0
	}
	return int(key.Node().Raw.Offset)
}

func tomlLineBounds(data []byte, offset int) (int, int) {
	start := bytes.LastIndexByte(data[:offset], '\n') + 1
	endOffset := bytes.IndexByte(data[offset:], '\n')
	if endOffset < 0 {
		return start, len(data)
	}
	return start, offset + endOffset + 1
}

func tomlExpressionLineBounds(data []byte, startOffset, endOffset int) (int, int) {
	lineStart := bytes.LastIndexByte(data[:startOffset], '\n') + 1
	lineEndOffset := bytes.IndexByte(data[endOffset:], '\n')
	if lineEndOffset < 0 {
		return lineStart, len(data)
	}
	return lineStart, endOffset + lineEndOffset + 1
}

func tomlInsertionPoint(data []byte, nextHeader int) int {
	point := nextHeader
	for point > 0 {
		lineEnd := point
		lineStart := bytes.LastIndexByte(data[:lineEnd-1], '\n') + 1
		line := strings.TrimSpace(string(data[lineStart:lineEnd]))
		if line != "" && !strings.HasPrefix(line, "#") {
			break
		}
		point = lineStart
	}
	return point
}

func tomlPathKey(path []string) string {
	return strings.Join(path, "\x00")
}

func displayTOMLPath(path []string) string {
	return strings.Join(path, ".")
}

func tomlValueAt(root map[string]any, path []string) (any, bool) {
	var value any = root
	for _, part := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

func tomlValuesEqual(left, right any) bool {
	if reflect.DeepEqual(left, right) {
		return true
	}
	leftFloat, leftIsFloat := left.(float64)
	rightFloat, rightIsFloat := right.(float64)
	if leftIsFloat && rightIsFloat {
		return math.IsNaN(leftFloat) && math.IsNaN(rightFloat)
	}
	leftArray, leftIsArray := left.([]any)
	rightArray, rightIsArray := right.([]any)
	if leftIsArray && rightIsArray && len(leftArray) == len(rightArray) {
		for index := range leftArray {
			if !tomlValuesEqual(leftArray[index], rightArray[index]) {
				return false
			}
		}
		return true
	}
	leftObject, leftIsObject := left.(map[string]any)
	rightObject, rightIsObject := right.(map[string]any)
	if leftIsObject && rightIsObject && len(leftObject) == len(rightObject) {
		for key, leftValue := range leftObject {
			rightValue, exists := rightObject[key]
			if !exists || !tomlValuesEqual(leftValue, rightValue) {
				return false
			}
		}
		return true
	}
	return false
}

func joinTOMLEntries(entries []tomlEntry) []byte {
	var result []byte
	for index, entry := range entries {
		if index > 0 {
			result = append(result, '\n')
		}
		result = append(result, entry.raw...)
	}
	return result
}

func renderDottedTOMLEntry(path []string, expression []byte) ([]byte, bool) {
	equals := tomlAssignmentEquals(expression)
	if equals < 0 {
		return nil, false
	}
	parts := make([]string, len(path))
	for index, part := range path {
		if isBareTOMLKey(part) {
			parts[index] = part
		} else {
			parts[index] = strconv.Quote(part)
		}
	}
	result := []byte(strings.Join(parts, ".") + " ")
	result = append(result, expression[equals:]...)
	return result, true
}

func tomlAssignmentEquals(expression []byte) int {
	inBasicString := false
	inLiteralString := false
	escaped := false
	for index, value := range expression {
		switch {
		case escaped:
			escaped = false
		case inBasicString && value == '\\':
			escaped = true
		case !inLiteralString && value == '"':
			inBasicString = !inBasicString
		case !inBasicString && value == '\'':
			inLiteralString = !inLiteralString
		case !inBasicString && !inLiteralString && value == '=':
			return index
		}
	}
	return -1
}

func isBareTOMLKey(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func formatTOMLInsertion(data []byte, offset int, addition []byte, separate bool) []byte {
	var result []byte
	if offset > 0 && data[offset-1] != '\n' {
		result = append(result, '\n')
	}
	if separate && offset > 0 {
		result = append(result, '\n')
	}
	result = append(result, addition...)
	if len(result) == 0 || result[len(result)-1] != '\n' {
		result = append(result, '\n')
	}
	return result
}

func applyTOMLEdits(data []byte, edits []tomlEdit) ([]byte, error) {
	insertions := map[int][]byte{}
	var coalesced []tomlEdit
	var insertionOrder []int
	for _, edit := range edits {
		if edit.start != edit.end {
			coalesced = append(coalesced, edit)
			continue
		}
		if _, exists := insertions[edit.start]; !exists {
			insertionOrder = append(insertionOrder, edit.start)
		}
		insertions[edit.start] = append(insertions[edit.start], edit.content...)
	}
	for _, offset := range insertionOrder {
		coalesced = append(coalesced, tomlEdit{start: offset, end: offset, content: insertions[offset]})
	}
	edits = coalesced
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].start == edits[j].start {
			return edits[i].end > edits[j].end
		}
		return edits[i].start > edits[j].start
	})
	result := append([]byte(nil), data...)
	lastStart := len(data)
	for _, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(data) || edit.end > lastStart {
			return nil, fmt.Errorf("overlapping or invalid TOML edit at bytes %d:%d", edit.start, edit.end)
		}
		result = append(result[:edit.start], append(edit.content, result[edit.end:]...)...)
		lastStart = edit.start
	}
	return result, nil
}

func tomlValueEmpty(value any) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, child := range object {
		if !tomlValueEmpty(child) {
			return false
		}
	}
	return true
}

func tomlContainsComments(data []byte) bool {
	parser := unstable.Parser{KeepComments: true}
	parser.Reset(data)
	for parser.NextExpression() {
		if tomlNodeContainsComment(parser.Expression()) {
			return true
		}
	}
	return false
}

func tomlNodeContainsComment(node *unstable.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == unstable.Comment {
		return true
	}
	return tomlNodeContainsComment(node.Child()) || tomlNodeContainsComment(node.Next())
}
