package parser

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var headerPattern = regexp.MustCompile(`^--- !u!(\d+) &(-?\d+)(?:\s+stripped)?\s*$`)

type Block struct {
	ClassID   int
	FileID    int64
	TypeName  string
	Fields    map[string]any
	RawBody   string
	StartLine int
	EndLine   int
}

func ParseFile(path string) ([]Block, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return Parse(data)
}

func Parse(data []byte) ([]Block, error) {
	// Unity YAML is always UTF-8. Reject malformed bytes up front so callers get
	// a clear, deterministic error instead of having undecodable bytes silently
	// flow into block bodies and field values. Empty/header-only input is valid
	// (it simply yields zero blocks) and is unaffected by this check.
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("input is not valid UTF-8")
	}

	lines := splitLines(data)
	var blocks []Block

	for i := 0; i < len(lines); {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "%") {
			i++
			continue
		}

		match := headerPattern.FindStringSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("unexpected content outside block header: %q", line)
		}

		classID, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("parse classID %q: %w", match[1], err)
		}
		fileID, err := strconv.ParseInt(match[2], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse fileID %q: %w", match[2], err)
		}

		j := i + 1
		for j < len(lines) {
			trimmed := strings.TrimSpace(lines[j])
			if headerPattern.MatchString(trimmed) {
				break
			}
			if strings.HasPrefix(trimmed, "---") {
				return nil, fmt.Errorf("unexpected block header: %q", trimmed)
			}
			j++
		}

		bodyLines := lines[i+1 : j]
		endLine := j
		if j == len(lines) {
			for endLine > i+1 && strings.TrimSpace(lines[endLine-1]) == "" {
				endLine--
			}
		}
		block := Block{
			ClassID:   classID,
			FileID:    fileID,
			Fields:    make(map[string]any),
			RawBody:   strings.Join(bodyLines, "\n"),
			StartLine: i + 1,
			EndLine:   endLine,
		}
		parseBody(bodyLines, &block)
		blocks = append(blocks, block)
		i = j
	}

	return blocks, nil
}

func parseBody(lines []string, block *Block) {
	typeLine := -1
	for i, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if block.TypeName == "" && !strings.HasPrefix(line, " ") && strings.HasSuffix(trimmed, ":") {
			block.TypeName = strings.TrimSuffix(trimmed, ":")
			typeLine = i
			break
		}
	}

	if typeLine == -1 {
		return
	}

	fields, _ := parseMap(lines, typeLine+1, 2)
	block.Fields = fields
}

func splitField(line string) (string, string, bool) {
	index := strings.Index(line, ":")
	if index <= 0 {
		return "", "", false
	}

	key := strings.TrimSpace(line[:index])
	value := strings.TrimSpace(line[index+1:])
	return key, value, true
}

func parseValue(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if value == "[]" {
		return []any{}
	}

	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		if parsed, ok := parseInlineMap(value); ok {
			return parsed
		}
	}

	if unquoted, ok := parseQuotedString(value); ok {
		return unquoted
	}

	if integer, err := strconv.ParseInt(value, 10, 64); err == nil {
		return integer
	}

	if floating, err := strconv.ParseFloat(value, 64); err == nil {
		return floating
	}

	switch value {
	case "true":
		return true
	case "false":
		return false
	}

	return value
}

func parseMap(lines []string, start, indent int) (map[string]any, int) {
	result := make(map[string]any)
	i := start

	for i < len(lines) {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}

		currentIndent := leadingSpaces(line)
		if currentIndent < indent {
			break
		}
		if currentIndent != indent || strings.HasPrefix(strings.TrimSpace(line), "- ") {
			i++
			continue
		}

		key, value, ok := splitField(strings.TrimSpace(line))
		if !ok {
			i++
			continue
		}

		if value != "" {
			result[key] = parseValue(value)
			i++
			continue
		}

		next := nextContentLine(lines, i+1)
		if next == -1 {
			result[key] = ""
			i++
			continue
		}

		nextLine := strings.TrimRight(lines[next], "\r")
		nextIndent := leadingSpaces(nextLine)
		nextTrimmed := strings.TrimSpace(nextLine)
		if nextIndent < currentIndent || (nextIndent == currentIndent && !strings.HasPrefix(nextTrimmed, "- ")) {
			result[key] = ""
			i++
			continue
		}

		if strings.HasPrefix(nextTrimmed, "- ") {
			listIndent := nextIndent
			if listIndent < indent {
				listIndent = indent
			}
			listValue, nextIndex := parseList(lines, next, listIndent)
			result[key] = listValue
			i = nextIndex
			continue
		}

		mapValue, nextIndex := parseMap(lines, next, nextIndent)
		result[key] = mapValue
		i = nextIndex
	}

	return result, i
}

func parseList(lines []string, start, indent int) ([]any, int) {
	var result []any
	i := start

	for i < len(lines) {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}

		currentIndent := leadingSpaces(line)
		if currentIndent < indent {
			break
		}
		if currentIndent == indent && !strings.HasPrefix(trimmed, "- ") {
			break
		}
		if currentIndent != indent {
			i++
			continue
		}

		itemText := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		next := nextContentLine(lines, i+1)
		var childIndent int
		if next != -1 {
			childIndent = leadingSpaces(strings.TrimRight(lines[next], "\r"))
		}

		if strings.HasPrefix(itemText, "{") && strings.HasSuffix(itemText, "}") {
			result = append(result, parseValue(itemText))
			i++
			continue
		}

		if _, ok := parseQuotedString(itemText); ok {
			result = append(result, parseValue(itemText))
			i++
			continue
		}

		if looksLikeListMapEntry(itemText) {
			key, value, ok := splitField(itemText)
			if !ok {
				result = append(result, parseValue(itemText))
				i++
				continue
			}

			item := map[string]any{}
			if value != "" {
				item[key] = parseValue(value)
				if next != -1 && childIndent > currentIndent {
					childMap, nextIndex := parseMap(lines, next, childIndent)
					for childKey, childValue := range childMap {
						item[childKey] = childValue
					}
					result = append(result, item)
					i = nextIndex
					continue
				}
				result = append(result, item)
				i++
				continue
			}

			if next != -1 && childIndent > currentIndent {
				if strings.HasPrefix(strings.TrimSpace(strings.TrimRight(lines[next], "\r")), "- ") {
					childList, nextIndex := parseList(lines, next, childIndent)
					item[key] = childList
					result = append(result, item)
					i = nextIndex
					continue
				}

				childMap, nextIndex := parseMap(lines, next, childIndent)
				item[key] = childMap
				result = append(result, item)
				i = nextIndex
				continue
			}

			item[key] = ""
			result = append(result, item)
			i++
			continue
		}

		result = append(result, parseValue(itemText))
		i++
	}

	return result, i
}

func parseInlineMap(value string) (map[string]any, bool) {
	content := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}"))
	result := make(map[string]any)
	if content == "" {
		return result, true
	}

	parts := splitInlineMapEntries(content)
	for _, part := range parts {
		key, rawValue, ok := splitField(strings.TrimSpace(part))
		if !ok {
			return nil, false
		}
		result[key] = parseValue(rawValue)
	}

	return result, true
}

func splitInlineMapEntries(content string) []string {
	var parts []string
	var current strings.Builder
	depth := 0
	bracketDepth := 0
	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for _, r := range content {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		switch r {
		case '\\':
			if inDoubleQuote {
				current.WriteRune(r)
				escaped = true
				continue
			}
		case '\'':
			if !inDoubleQuote {
				inSingleQuote = !inSingleQuote
			}
		case '"':
			if !inSingleQuote {
				inDoubleQuote = !inDoubleQuote
			}
		case '{':
			if !inSingleQuote && !inDoubleQuote {
				depth++
			}
		case '}':
			if !inSingleQuote && !inDoubleQuote && depth > 0 {
				depth--
			}
		case '[':
			if !inSingleQuote && !inDoubleQuote {
				bracketDepth++
			}
		case ']':
			if !inSingleQuote && !inDoubleQuote && bracketDepth > 0 {
				bracketDepth--
			}
		case ',':
			if !inSingleQuote && !inDoubleQuote && depth == 0 && bracketDepth == 0 {
				parts = append(parts, current.String())
				current.Reset()
				continue
			}
		}

		current.WriteRune(r)
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func parseQuotedString(value string) (string, bool) {
	if len(value) < 2 {
		return "", false
	}

	if value[0] == '"' && value[len(value)-1] == '"' {
		unquoted, err := strconv.Unquote(value)
		if err == nil {
			return unquoted, true
		}
		return value[1 : len(value)-1], true
	}

	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), true
	}

	return "", false
}

func splitLines(data []byte) []string {
	normalized := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
	return strings.Split(string(normalized), "\n")
}

func nextContentLine(lines []string, start int) int {
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(strings.TrimRight(lines[i], "\r")) != "" {
			return i
		}
	}

	return -1
}

func leadingSpaces(line string) int {
	count := 0
	for _, r := range line {
		if r != ' ' {
			break
		}
		count++
	}

	return count
}

func looksLikeListMapEntry(itemText string) bool {
	return strings.Contains(itemText, ": ") || strings.HasSuffix(itemText, ":")
}
