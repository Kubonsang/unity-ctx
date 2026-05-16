package document

import (
	"fmt"
	"sort"
	"strings"

	"unity-ctx/internal/parser"
)

const (
	CodeAmbiguousName = "AMBIGUOUS_NAME"
	CodeNotFound      = "NOT_FOUND"
	CodeAmbiguousType = "AMBIGUOUS_TYPE"
)

type LookupError struct {
	Code  string
	Field string
	Value string
	Count int
}

func (e *LookupError) Error() string {
	if e == nil {
		return ""
	}

	switch e.Code {
	case CodeAmbiguousName:
		return fmt.Sprintf("%s %s=%q matches=%d", e.Code, e.Field, e.Value, e.Count)
	case CodeAmbiguousType:
		return fmt.Sprintf("%s %s=%q matches=%d", e.Code, e.Field, e.Value, e.Count)
	case CodeNotFound:
		return fmt.Sprintf("%s %s=%q", e.Code, e.Field, e.Value)
	default:
		return fmt.Sprintf("%s %s=%q", e.Code, e.Field, e.Value)
	}
}

type Doc struct {
	blocks   []parser.Block
	byFileID map[int64]parser.Block
	byType   map[string][]parser.Block
	byName   map[string][]parser.Block
}

func Build(blocks []parser.Block) *Doc {
	doc := &Doc{
		blocks:   make([]parser.Block, 0, len(blocks)),
		byFileID: make(map[int64]parser.Block, len(blocks)),
		byType:   make(map[string][]parser.Block),
		byName:   make(map[string][]parser.Block),
	}

	for _, block := range blocks {
		cloned := cloneBlock(block)
		doc.blocks = append(doc.blocks, cloned)
		doc.byFileID[cloned.FileID] = cloned
		doc.byType[cloned.TypeName] = append(doc.byType[cloned.TypeName], cloned)

		if name, ok := fieldString(cloned.Fields, "m_Name"); ok {
			doc.byName[name] = append(doc.byName[name], cloned)
		}
	}

	return doc
}

func (d *Doc) FindByFileID(fileID int64) (parser.Block, bool) {
	if d == nil {
		return parser.Block{}, false
	}

	block, ok := d.byFileID[fileID]
	if !ok {
		return parser.Block{}, false
	}

	return cloneBlock(block), true
}

func (d *Doc) FindUniqueByName(name string) (parser.Block, error) {
	if d == nil {
		return parser.Block{}, &LookupError{
			Code:  CodeNotFound,
			Field: "name",
			Value: name,
		}
	}

	blocks := d.byName[name]
	switch len(blocks) {
	case 0:
		return parser.Block{}, &LookupError{
			Code:  CodeNotFound,
			Field: "name",
			Value: name,
		}
	case 1:
		return cloneBlock(blocks[0]), nil
	default:
		return parser.Block{}, &LookupError{
			Code:  CodeAmbiguousName,
			Field: "name",
			Value: name,
			Count: len(blocks),
		}
	}
}

func (d *Doc) FindBlocksByType(typeName string) []parser.Block {
	if d == nil {
		return nil
	}

	blocks := d.byType[typeName]
	cloned := make([]parser.Block, 0, len(blocks))
	for _, block := range blocks {
		cloned = append(cloned, cloneBlock(block))
	}

	return cloned
}

func (d *Doc) FindUniqueByType(typeName string) (parser.Block, error) {
	if d == nil {
		return parser.Block{}, &LookupError{
			Code:  CodeNotFound,
			Field: "type",
			Value: typeName,
		}
	}

	blocks := d.byType[typeName]
	switch len(blocks) {
	case 0:
		return parser.Block{}, &LookupError{
			Code:  CodeNotFound,
			Field: "type",
			Value: typeName,
		}
	case 1:
		return cloneBlock(blocks[0]), nil
	default:
		return parser.Block{}, &LookupError{
			Code:  CodeAmbiguousType,
			Field: "type",
			Value: typeName,
			Count: len(blocks),
		}
	}
}

func ResolveField(fields map[string]any, path string) (any, bool) {
	if fields == nil || path == "" {
		return nil, false
	}

	current := any(fields)
	parts := strings.Split(path, ".")
	for _, part := range parts {
		nextMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}

		value, ok := nextMap[part]
		if !ok {
			return nil, false
		}
		current = value
	}

	return cloneValue(current), true
}

func SortedFieldKeys(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func fieldString(fields map[string]any, key string) (string, bool) {
	if fields == nil {
		return "", false
	}

	value, ok := fields[key]
	if !ok {
		return "", false
	}

	text, ok := value.(string)
	if !ok || text == "" {
		return "", false
	}

	return text, true
}

func cloneBlock(block parser.Block) parser.Block {
	return parser.Block{
		ClassID:  block.ClassID,
		FileID:   block.FileID,
		TypeName: block.TypeName,
		Fields:   cloneMap(block.Fields),
		RawBody:  block.RawBody,
	}
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}

	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = cloneValue(value)
	}

	return cloned
}

func cloneSlice(input []any) []any {
	if input == nil {
		return nil
	}

	cloned := make([]any, len(input))
	for i, value := range input {
		cloned[i] = cloneValue(value)
	}

	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		return cloneSlice(typed)
	default:
		return typed
	}
}
