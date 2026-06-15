package mutation

import (
	"fmt"
	"strconv"
	"strings"

	"unity-ctx/internal/parser"
)

func coerceScalar(oldValue any, raw string) (string, string, bool, error) {
	value := strings.TrimSpace(raw)

	switch typed := oldValue.(type) {
	case int64:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return "", "", false, fmt.Errorf("TYPE_MISMATCH type_hint=int value=%q", raw)
		}
		return strconv.FormatInt(parsed, 10), "int", parsed != typed, nil
	case float64:
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return "", "", false, fmt.Errorf("TYPE_MISMATCH type_hint=float value=%q", raw)
		}
		return value, "float", parsed != typed, nil
	case bool:
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", "", false, fmt.Errorf("TYPE_MISMATCH type_hint=bool value=%q", raw)
		}
		rendered := strconv.FormatBool(parsed)
		return rendered, "bool", parsed != typed, nil
	case string:
		rendered := renderStringScalar(raw)
		return rendered, "string", typed != raw, nil
	default:
		return "", "", false, fmt.Errorf("FIELD_NOT_SCALAR field_type=%T", oldValue)
	}
}

func rewriteScalarField(input []byte, block parser.Block, fieldPath, rendered string) ([]byte, error) {
	lines := splitPreservedLines(input)
	lineIndex, err := findFieldLine(lines, block, fieldPath)
	if err != nil {
		return nil, err
	}

	line := lines[lineIndex].content
	colon := strings.Index(line, ":")
	if colon == -1 {
		return nil, fmt.Errorf("FIELD_NOT_REWRITABLE field=%s", fieldPath)
	}

	lines[lineIndex].content = rewriteLineScalar(line, colon, rendered)
	return joinLines(lines), nil
}

func renderedScalarValue(input []byte, block parser.Block, fieldPath, fallback string) string {
	lines := splitPreservedLines(input)
	lineIndex, err := findFieldLine(lines, block, fieldPath)
	if err != nil {
		return fallback
	}

	line := lines[lineIndex].content
	colon := strings.Index(line, ":")
	if colon == -1 {
		return fallback
	}

	return strings.TrimSpace(line[colon+1:])
}

func formatValue(value any) string {
	switch typed := value.(type) {
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	case string:
		return renderStringScalar(typed)
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func cloneBytes(input []byte) []byte {
	return append([]byte(nil), input...)
}
