package mutation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/document"
	"github.com/Kubonsang/unity-ctx/internal/parser"
)

var renameFile = os.Rename
var syncDirectoryFn = syncDirectory

type CommittedWriteError struct {
	Path string
	Err  error
}

func (e *CommittedWriteError) Error() string {
	return fmt.Sprintf("WRITE_COMMITTED path=%s err=%v", e.Path, e.Err)
}

func (e *CommittedWriteError) Unwrap() error {
	return e.Err
}

func (e *CommittedWriteError) WriteCommitted() bool {
	return true
}

type AssetSetRequest struct {
	Path    string
	HasID   bool
	ID      int64
	Field   string
	Value   string
	Rewrite bool
}

type AssetSetResult struct {
	Field       string
	OldValue    string
	NewValue    string
	TypeHint    string
	Changed     bool
	UpdatedData []byte
}

func PlanAssetSet(input []byte, blocks []parser.Block, req AssetSetRequest) (AssetSetResult, error) {
	if err := validateAssetMutationPath(req.Path); err != nil {
		return AssetSetResult{}, err
	}

	target, err := resolveAssetTarget(blocks, req)
	if err != nil {
		return AssetSetResult{}, err
	}

	oldValue, ok := document.ResolveField(target.Fields, req.Field)
	if !ok {
		return AssetSetResult{}, fmt.Errorf("FIELD_NOT_FOUND field=%s", req.Field)
	}

	newScalar, typeHint, changed, err := coerceScalar(oldValue, req.Value)
	if err != nil {
		return AssetSetResult{}, err
	}

	updated := cloneBytes(input)
	if !changed {
		newScalar = renderedScalarValue(input, target, req.Field, formatValue(oldValue))
	}
	if req.Rewrite && changed {
		updated, err = rewriteScalarField(input, target, req.Field, newScalar)
		if err != nil {
			return AssetSetResult{}, err
		}
	}

	return AssetSetResult{
		Field:       req.Field,
		OldValue:    formatValue(oldValue),
		NewValue:    newScalar,
		TypeHint:    typeHint,
		Changed:     changed,
		UpdatedData: updated,
	}, nil
}

func validateAssetMutationPath(path string) error {
	kind := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	if kind == ".asset" || kind == ".mat" {
		return nil
	}
	if kind == "" {
		kind = "unknown"
	}

	return fmt.Errorf("UNSUPPORTED_FILE_KIND kind=%s allowed=.asset,.mat", kind)
}

func WriteWithBackup(path string, updated []byte) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	original, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	backupPath := path + ".bak"
	mode := info.Mode().Perm()
	if err := writeFileAtomically(backupPath, original, mode); err != nil {
		return "", err
	}
	if err := writeFileAtomically(path, updated, mode); err != nil {
		var committedErr *CommittedWriteError
		if errors.As(err, &committedErr) {
			return backupPath, err
		}
		return "", err
	}

	return backupPath, nil
}

// RestoreFromBackup atomically overwrites path with the contents of backupPath,
// returning the number of bytes restored. It is the inverse of the .bak written
// by WriteWithBackup, used to recover from a committed write.
func RestoreFromBackup(path, backupPath string) (int, error) {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return 0, err
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	if err := writeFileAtomically(path, data, mode); err != nil {
		return 0, err
	}
	return len(data), nil
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	pattern := filepath.Base(path) + ".tmp-*"

	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}

	tempPath := file.Name()
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	if err = file.Chmod(mode); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	closed = true

	if err = renameFile(tempPath, path); err != nil {
		return err
	}

	if err = syncDirectoryFn(dir); err != nil {
		return &CommittedWriteError{
			Path: path,
			Err:  err,
		}
	}

	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()

	return dir.Sync()
}

func resolveAssetTarget(blocks []parser.Block, req AssetSetRequest) (parser.Block, error) {
	if req.HasID {
		for _, block := range blocks {
			if block.FileID == req.ID {
				return block, nil
			}
		}
		return parser.Block{}, fmt.Errorf("NOT_FOUND fileID=%d", req.ID)
	}

	if len(blocks) == 0 {
		return parser.Block{}, fmt.Errorf("NOT_FOUND asset_block")
	}
	if len(blocks) > 1 {
		return parser.Block{}, fmt.Errorf("NEED_RULE fileID matches=%d", len(blocks))
	}

	return blocks[0], nil
}

func findFieldLine(lines []preservedLine, block parser.Block, fieldPath string) (int, error) {
	bodyStart := block.StartLine
	bodyEnd := block.EndLine
	if bodyStart < 0 || bodyEnd > len(lines) || bodyStart >= bodyEnd {
		return 0, fmt.Errorf("FIELD_NOT_REWRITABLE field=%s", fieldPath)
	}

	typeLine := -1
	for i := bodyStart; i < bodyEnd; i++ {
		trimmed := strings.TrimSpace(lines[i].content)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(lines[i].content, " ") && trimmed == block.TypeName+":" {
			typeLine = i
			break
		}
	}
	if typeLine == -1 {
		return 0, fmt.Errorf("FIELD_NOT_REWRITABLE field=%s", fieldPath)
	}

	parts := strings.Split(fieldPath, ".")
	searchStart := typeLine + 1
	searchEnd := bodyEnd
	for depth, part := range parts {
		expectedIndent := 2 * (depth + 1)
		found := -1
		nextSearchEnd := searchEnd

		for i := searchStart; i < searchEnd; i++ {
			line := lines[i].content
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			indent := leadingSpaces(line)
			if indent < expectedIndent {
				break
			}
			if indent != expectedIndent || strings.HasPrefix(trimmed, "- ") {
				continue
			}

			key, _, ok := splitField(trimmed)
			if !ok || key != part {
				continue
			}

			found = i
			nextSearchEnd = findNestedSectionEnd(lines, i+1, searchEnd, expectedIndent)
			break
		}

		if found == -1 {
			return 0, fmt.Errorf("FIELD_NOT_REWRITABLE field=%s", fieldPath)
		}

		if depth == len(parts)-1 {
			_, _, ok := splitField(strings.TrimSpace(lines[found].content))
			if !ok {
				return 0, fmt.Errorf("FIELD_NOT_SCALAR field=%s", fieldPath)
			}
			return found, nil
		}

		searchStart = found + 1
		searchEnd = nextSearchEnd
	}

	return 0, fmt.Errorf("FIELD_NOT_REWRITABLE field=%s", fieldPath)
}

func findNestedSectionEnd(lines []preservedLine, start, limit, indent int) int {
	for i := start; i < limit; i++ {
		trimmed := strings.TrimSpace(lines[i].content)
		if trimmed == "" {
			continue
		}
		if leadingSpaces(lines[i].content) <= indent {
			return i
		}
	}

	return limit
}

func renderStringScalar(raw string) string {
	if raw == "" {
		return `""`
	}
	if strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, ":#{}[],&*!|>'\"%@`\\") || looksLikeNonStringScalar(raw) {
		return strconv.Quote(raw)
	}
	return raw
}

func looksLikeNonStringScalar(raw string) bool {
	lower := strings.ToLower(raw)
	switch lower {
	case "~", "null", "true", "false", "yes", "no", "on", "off":
		return true
	}

	if _, err := strconv.ParseBool(raw); err == nil {
		return true
	}
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return true
	}

	return false
}

type preservedLine struct {
	content string
	ending  string
}

func splitPreservedLines(data []byte) []preservedLine {
	text := string(data)
	lines := make([]preservedLine, 0, strings.Count(text, "\n")+strings.Count(text, "\r")+1)
	start := 0

	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			lines = append(lines, preservedLine{content: text[start:i], ending: "\n"})
			start = i + 1
		case '\r':
			ending := "\r"
			if i+1 < len(text) && text[i+1] == '\n' {
				ending = "\r\n"
				i++
			}
			lines = append(lines, preservedLine{content: text[start : i-len(ending)+1], ending: ending})
			start = i + 1
		}
	}

	lines = append(lines, preservedLine{content: text[start:], ending: ""})
	return lines
}

func joinLines(lines []preservedLine) []byte {
	var builder strings.Builder
	for _, line := range lines {
		builder.WriteString(line.content)
		builder.WriteString(line.ending)
	}
	return []byte(builder.String())
}

func rewriteLineScalar(line string, colon int, rendered string) string {
	valueStart := colon + 1
	for valueStart < len(line) && line[valueStart] == ' ' {
		valueStart++
	}

	if valueStart < len(line) {
		return line[:valueStart] + rendered
	}
	if valueStart == colon+1 {
		return line + " " + rendered
	}
	return line + rendered
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
