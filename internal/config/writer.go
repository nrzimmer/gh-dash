package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	yamlmarshaller "gopkg.in/yaml.v3"
)

// AppendSection appends a new section entry to the named top-level list key
// (e.g. "prSections" or "issuesSections") in the config file at path,
// preserving the rest of the file byte-for-byte - including comments and
// YAML anchors - by patching the raw text instead of re-marshalling the
// whole document.
//
// section must marshal to a single YAML mapping (e.g. PrsSectionConfig or
// IssuesSectionConfig).
func AppendSection(path string, key string, section any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file %q: %w", path, err)
	}

	itemYAML, err := marshalSingleItem(section)
	if err != nil {
		return fmt.Errorf("marshalling new %s entry: %w", key, err)
	}

	patched, err := appendToSectionList(string(raw), key, itemYAML)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(patched), 0o666); err != nil {
		return fmt.Errorf("writing config file %q: %w", path, err)
	}

	return nil
}

// FieldValue describes one field to update within a section item: Name is
// the YAML key ("filters", "limit", "extraFields", or "localFilter"). When
// IsSet is false or Value is empty, the field is removed from the item
// (e.g. clearing "limit" drops the line entirely instead of leaving an
// empty scalar); "limit" is marshalled as a YAML integer, every other field
// as a plain string (which yaml.v3 renders as a multi-line block scalar
// automatically when Value contains newlines, e.g. extraFields/localFilter
// GraphQL/expr-lang blocks).
type FieldValue struct {
	Name  string
	Value string
	IsSet bool
}

// UpdateSectionFields rewrites one or more fields of the section titled
// title within the named top-level list key ("prSections" or
// "issuesSections"), preserving the rest of the file - including comments,
// anchors, and every other field of that section - untouched. Each field
// may already exist as an inline value or a multi-line block scalar (both
// are replaced by a single new value), may not exist yet (appended to the
// end of the item), or may be requested for removal (deleted entirely).
func UpdateSectionFields(path, key, title string, fields []FieldValue) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file %q: %w", path, err)
	}

	patched, err := updateSectionFieldsInList(string(raw), key, title, fields)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(patched), 0o666); err != nil {
		return fmt.Errorf("writing config file %q: %w", path, err)
	}

	return nil
}

// UpdateSectionFilter is a thin wrapper around UpdateSectionFields for the
// common case of updating only the filters value.
func UpdateSectionFilter(path, key, title, newFilters string) error {
	return UpdateSectionFields(path, key, title, []FieldValue{
		{Name: "filters", Value: newFilters, IsSet: true},
	})
}

// marshalSingleItem renders section as a single YAML list item ("- field: value\n  field2: value2\n"),
// with no indentation of its own - the caller is responsible for reindenting
// it to match the target file's list style.
func marshalSingleItem(section any) (string, error) {
	out, err := yamlmarshaller.Marshal([]any{section})
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// marshalFieldValue renders a single "name: value" mapping entry (no
// indentation, no trailing newline), with quoting/escaping - and, for
// multi-line values, block-scalar formatting - handled by yaml.v3. "limit"
// is marshalled as an integer so it stays unquoted in the file; every other
// field is marshalled as a string.
func marshalFieldValue(name, value string) (string, error) {
	var out []byte
	var err error

	if name == "limit" {
		n, convErr := strconv.Atoi(strings.TrimSpace(value))
		if convErr != nil {
			return "", fmt.Errorf("invalid limit value %q: %w", value, convErr)
		}
		out, err = yamlmarshaller.Marshal(map[string]int{name: n})
	} else {
		out, err = yamlmarshaller.Marshal(map[string]string{name: value})
	}
	if err != nil {
		return "", err
	}

	return strings.TrimRight(string(out), "\n"), nil
}

type titleOnly struct {
	Title string `yaml:"title"`
}

// findListBlock locates the top-level list identified by key within lines,
// returning the index of the "key:" line, the exclusive end index of the
// block (next top-level key, a stray column-0 comment, or EOF), and the
// indentation used by existing items (defaulting to two spaces if the list
// is empty). found is false if key isn't present at all.
func findListBlock(lines []string, key string) (keyLineIdx, blockEnd int, indent string, found bool) {
	keyLineRe := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:\s*(\[\s*\])?\s*(#.*)?$`)

	keyLineIdx = -1
	for i, line := range lines {
		if keyLineRe.MatchString(line) {
			keyLineIdx = i
			break
		}
	}
	if keyLineIdx == -1 {
		return -1, -1, "", false
	}

	indent = "  "
	blockEnd = len(lines)
	foundItem := false
	for i := keyLineIdx + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimLeft(line, " ")

		if trimmed == "" {
			continue
		}

		leadingSpaces := len(line) - len(trimmed)
		if leadingSpaces == 0 {
			// Zero-indent, non-blank line: either a new top-level key or a
			// stray column-0 comment - both end the block.
			blockEnd = i
			break
		}

		if !foundItem && strings.HasPrefix(trimmed, "- ") {
			indent = line[:leadingSpaces]
			foundItem = true
		}
	}

	return keyLineIdx, blockEnd, indent, true
}

// appendToSectionList inserts itemYAML (as produced by marshalSingleItem)
// as the last element of the top-level YAML list named key within content.
// If key isn't present, it is appended as a new top-level key at the end of
// the file.
func appendToSectionList(content, key, itemYAML string) (string, error) {
	lines := strings.Split(content, "\n")

	// Normalize an inline empty-list value ("key: []") to a bare key so a
	// block-style item can be appended under it.
	emptyListRe := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:\s*\[\s*\]\s*(#.*)?$`)
	for i, line := range lines {
		if emptyListRe.MatchString(line) {
			lines[i] = key + ":"
			break
		}
	}

	_, blockEnd, indent, found := findListBlock(lines, key)

	if !found {
		// Key not present at all: append it as a brand-new top-level list.
		indented := reindent(itemYAML, "  ")
		var b strings.Builder
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(key)
		b.WriteString(":\n")
		b.WriteString(indented)
		b.WriteString("\n")
		return b.String(), nil
	}

	indentedItem := reindent(itemYAML, indent)

	var b strings.Builder
	b.WriteString(strings.Join(lines[:blockEnd], "\n"))
	b.WriteString("\n")
	b.WriteString(indentedItem)
	b.WriteString("\n")
	b.WriteString(strings.Join(lines[blockEnd:], "\n"))

	return b.String(), nil
}

// updateSectionFieldsInList finds the item titled title inside the
// top-level list key, and applies every field update to it in order.
func updateSectionFieldsInList(content, key, title string, fields []FieldValue) (string, error) {
	lines := strings.Split(content, "\n")

	start, end, _, fieldIndent, err := findItemByTitle(lines, key, title)
	if err != nil {
		return "", err
	}

	for _, field := range fields {
		updatedLines, newEnd, err := applyFieldUpdate(lines, start, end, fieldIndent, field)
		if err != nil {
			return "", err
		}
		lines = updatedLines
		end = newEnd
	}

	return strings.Join(lines, "\n"), nil
}

// findItemByTitle locates the item titled title inside the top-level list
// key, returning its line range [start, end), the list's item indentation,
// and the indentation of the item's own fields (indent + two spaces).
func findItemByTitle(lines []string, key, title string) (start, end int, indent, fieldIndent string, err error) {
	keyLineIdx, blockEnd, listIndent, found := findListBlock(lines, key)
	if !found {
		return 0, 0, "", "", fmt.Errorf("section %q not found: %s is not present in config", title, key)
	}

	itemMarker := listIndent + "- "
	var itemStarts []int
	for i := keyLineIdx + 1; i < blockEnd; i++ {
		if strings.HasPrefix(lines[i], itemMarker) {
			itemStarts = append(itemStarts, i)
		}
	}

	listFieldIndent := listIndent + "  "

	for i, itemStart := range itemStarts {
		itemEnd := blockEnd
		if i+1 < len(itemStarts) {
			itemEnd = itemStarts[i+1]
		}

		itemTitle, ok := parseItemTitle(lines, itemStart, itemEnd, listIndent, listFieldIndent)
		if !ok || itemTitle != title {
			continue
		}

		return itemStart, itemEnd, listIndent, listFieldIndent, nil
	}

	return 0, 0, "", "", fmt.Errorf("section %q not found in %s", title, key)
}

// RenameSection changes the title of the section currently titled oldTitle
// within the named top-level list key ("prSections"/"issuesSections") to
// newTitle, preserving the rest of the file - including every other field
// of that section, comments, and anchors - untouched. Unlike the other
// fields, title lives on the item's own "- " marker line rather than a
// fieldIndent-prefixed line, so it can't go through UpdateSectionFields'
// generic per-field path and gets its own rewrite here.
func RenameSection(path, key, oldTitle, newTitle string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file %q: %w", path, err)
	}

	patched, err := renameSectionInList(string(raw), key, oldTitle, newTitle)
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(patched), 0o666); err != nil {
		return fmt.Errorf("writing config file %q: %w", path, err)
	}

	return nil
}

func renameSectionInList(content, key, oldTitle, newTitle string) (string, error) {
	lines := strings.Split(content, "\n")

	start, _, indent, _, err := findItemByTitle(lines, key, oldTitle)
	if err != nil {
		return "", err
	}

	newTitleYAML, err := marshalFieldValue("title", newTitle)
	if err != nil {
		return "", fmt.Errorf("marshalling new title value: %w", err)
	}

	lines[start] = indent + "- " + newTitleYAML

	return strings.Join(lines, "\n"), nil
}

// applyFieldUpdate replaces, removes, or appends field within the item
// spanning [start, end) of lines, returning the updated lines slice and the
// item's new end index (adjusted for any line-count change).
func applyFieldUpdate(lines []string, start, end int, fieldIndent string, field FieldValue) ([]string, int, error) {
	fStart, fEnd, found := findFieldRange(lines, start, end, fieldIndent, field.Name)

	if !field.IsSet || field.Value == "" {
		if !found {
			return lines, end, nil
		}
		newLines := make([]string, 0, len(lines)-(fEnd-fStart))
		newLines = append(newLines, lines[:fStart]...)
		newLines = append(newLines, lines[fEnd:]...)
		return newLines, end - (fEnd - fStart), nil
	}

	newFieldYAML, err := marshalFieldValue(field.Name, field.Value)
	if err != nil {
		return nil, 0, fmt.Errorf("marshalling new %s value: %w", field.Name, err)
	}
	// marshalFieldValue may return more than one line (a multi-line block
	// scalar, e.g. extraFields/localFilter) - reindent every line, not just
	// the first.
	newFieldLines := strings.Split(reindent(newFieldYAML, fieldIndent), "\n")

	if found {
		newLines := make([]string, 0, len(lines)-(fEnd-fStart)+len(newFieldLines))
		newLines = append(newLines, lines[:fStart]...)
		newLines = append(newLines, newFieldLines...)
		newLines = append(newLines, lines[fEnd:]...)
		return newLines, end - (fEnd - fStart) + len(newFieldLines), nil
	}

	// Field doesn't exist yet: append it at the end of the item.
	newLines := make([]string, 0, len(lines)+len(newFieldLines))
	newLines = append(newLines, lines[:end]...)
	newLines = append(newLines, newFieldLines...)
	newLines = append(newLines, lines[end:]...)
	return newLines, end + len(newFieldLines), nil
}

// parseItemTitle extracts the "title" field of a single list item (lines
// [start, end) of lines, starting with its "- " marker). Only the title
// field itself is dedented and unmarshalled in isolation - not the whole
// item - so a YAML alias elsewhere in the item that references an anchor
// defined in a *different* item (as happens with extraFields/localFilter in
// this config) doesn't break title extraction.
func parseItemTitle(lines []string, start, end int, indent, fieldIndent string) (string, bool) {
	if end <= start {
		return "", false
	}

	// The item's first line combines the "- " marker with its first field
	// (usually "title"). Swap the marker for equivalent spaces so the same
	// fieldIndent-anchored search used for other fields also finds title
	// when it's on that first line.
	tmp := make([]string, end-start)
	copy(tmp, lines[start:end])
	if strings.HasPrefix(tmp[0], indent+"- ") {
		tmp[0] = fieldIndent + tmp[0][len(indent)+2:]
	}

	titleStart, titleEnd, ok := findFieldRange(tmp, 0, len(tmp), fieldIndent, "title")
	if !ok {
		return "", false
	}

	fieldLines := make([]string, titleEnd-titleStart)
	for i := titleStart; i < titleEnd; i++ {
		fieldLines[i-titleStart] = strings.TrimPrefix(tmp[i], fieldIndent)
	}

	var parsed titleOnly
	if err := yamlmarshaller.Unmarshal([]byte(strings.Join(fieldLines, "\n")), &parsed); err != nil {
		return "", false
	}

	return parsed.Title, true
}

// findFieldRange locates the line range [start, end) within [rangeStart,
// rangeEnd) that holds fieldName's value at fieldIndent - a single line for
// an inline value, or the header line plus every more-indented continuation
// line for a block scalar. Trailing blank lines are excluded from the
// range, since they belong to the spacing before whatever follows (a
// comment or the next field/item), not to this field's value.
func findFieldRange(lines []string, rangeStart, rangeEnd int, fieldIndent, fieldName string) (start, end int, found bool) {
	fieldLineRe := regexp.MustCompile(`^` + regexp.QuoteMeta(fieldIndent) + regexp.QuoteMeta(fieldName) + `:`)

	for i := rangeStart; i < rangeEnd; i++ {
		if !fieldLineRe.MatchString(lines[i]) {
			continue
		}

		valueEnd := i + 1
		for valueEnd < rangeEnd {
			line := lines[valueEnd]
			trimmed := strings.TrimLeft(line, " ")
			if trimmed == "" {
				valueEnd++
				continue
			}

			leadingSpaces := len(line) - len(trimmed)
			if leadingSpaces <= len(fieldIndent) {
				break
			}
			valueEnd++
		}

		for valueEnd > i+1 && strings.TrimSpace(lines[valueEnd-1]) == "" {
			valueEnd--
		}

		return i, valueEnd, true
	}

	return 0, 0, false
}

// reindent prepends indent to every line of block.
func reindent(block, indent string) string {
	lines := strings.Split(block, "\n")
	var b bytes.Buffer
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(indent)
		b.WriteString(line)
	}
	return b.String()
}
