package mutation

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/document"
	"github.com/Kubonsang/unity-ctx/internal/parser"
)

// RepositionField is the only field scene reposition rewrites: a Transform's
// (or RectTransform's) local position, serialized by Unity as an inline flow
// mapping {x, y, z}. The existing scalar rewriter cannot touch a nested inline
// mapping entry, so reposition uses the dedicated flow-mapping rewriter below.
const RepositionField = "m_LocalPosition"

type SceneRepositionRequest struct {
	Path     string
	ID       int64
	Position [3]float64
	Rewrite  bool
}

type SceneRepositionResult struct {
	Field       string
	OldValue    string // "x,y,z"
	NewValue    string // "x,y,z"
	Changed     bool
	UpdatedData []byte
}

// PlanSceneReposition computes the byte-level rewrite that sets the target
// Transform's m_LocalPosition to req.Position. It is topology-invariant: only
// the three numeric axis tokens of one inline mapping change, so the fileID
// graph is untouched and the safety kernel's pre/temp/final checks pass for any
// input that was already sound.
func PlanSceneReposition(input []byte, blocks []parser.Block, req SceneRepositionRequest) (SceneRepositionResult, error) {
	if err := validateSceneMutationPath(req.Path); err != nil {
		return SceneRepositionResult{}, err
	}

	target, err := resolveRepositionTarget(blocks, req.ID)
	if err != nil {
		return SceneRepositionResult{}, err
	}

	raw, ok := document.ResolveField(target.Fields, RepositionField)
	if !ok {
		return SceneRepositionResult{}, fmt.Errorf("FIELD_NOT_FOUND field=%s fileID=%d", RepositionField, req.ID)
	}
	oldVec, err := vector3FromField(raw)
	if err != nil {
		return SceneRepositionResult{}, err
	}

	newVec := req.Position
	changed := oldVec != newVec

	updated := cloneBytes(input)
	if req.Rewrite && changed {
		updated, err = rewriteVector3Field(input, target, RepositionField, newVec)
		if err != nil {
			return SceneRepositionResult{}, err
		}
	}

	return SceneRepositionResult{
		Field:       RepositionField,
		OldValue:    formatVec3(oldVec),
		NewValue:    formatVec3(newVec),
		Changed:     changed,
		UpdatedData: updated,
	}, nil
}

func validateSceneMutationPath(path string) error {
	kind := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	if kind == ".unity" {
		return nil
	}
	if kind == "" {
		kind = "unknown"
	}
	return fmt.Errorf("UNSUPPORTED_FILE_KIND kind=%s allowed=.unity", kind)
}

// Unity class IDs of the transform types that structural scene ops target.
// Both carry a Vector3 m_LocalPosition and the parent/child links, so the
// reparent/delete slices (S4/S5) will gate their targets with the same helper
// rather than re-deriving the set.
const (
	classTransform     = 4
	classRectTransform = 224
)

var transformClassIDs = []int{classTransform, classRectTransform}

// isTransformClass reports whether classID is a transform type a structural
// scene op may target (Transform or RectTransform).
func isTransformClass(classID int) bool {
	for _, c := range transformClassIDs {
		if classID == c {
			return true
		}
	}
	return false
}

func allowedTransformClassList() string {
	parts := make([]string, len(transformClassIDs))
	for i, c := range transformClassIDs {
		parts[i] = strconv.Itoa(c)
	}
	return strings.Join(parts, ",")
}

func resolveRepositionTarget(blocks []parser.Block, id int64) (parser.Block, error) {
	for _, block := range blocks {
		if block.FileID == id {
			// Class guard runs before field resolution: a non-transform block
			// is not a reposition target even if it happens to carry an
			// m_LocalPosition: {x, y, z} of its own.
			if !isTransformClass(block.ClassID) {
				return parser.Block{}, fmt.Errorf("UNSUPPORTED_TARGET_CLASS field=%s id=%d class=%d allowed=%s", RepositionField, id, block.ClassID, allowedTransformClassList())
			}
			return block, nil
		}
	}
	return parser.Block{}, fmt.Errorf("NOT_FOUND fileID=%d", id)
}

// vector3FromField interprets a parsed m_LocalPosition value as a Vector3. It
// rejects anything that is not exactly {x, y, z} of numbers so a misaddressed
// field (e.g. a Quaternion {x,y,z,w}) fails loudly rather than being mangled.
func vector3FromField(raw any) ([3]float64, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return [3]float64{}, fmt.Errorf("FIELD_NOT_VECTOR3 field=%s reason=not_a_mapping", RepositionField)
	}
	if len(m) != 3 {
		return [3]float64{}, fmt.Errorf("FIELD_NOT_VECTOR3 field=%s reason=expected_xyz keys=%d", RepositionField, len(m))
	}
	var vec [3]float64
	for i, axis := range [3]string{"x", "y", "z"} {
		v, ok := m[axis]
		if !ok {
			return [3]float64{}, fmt.Errorf("FIELD_NOT_VECTOR3 field=%s reason=missing_axis axis=%s", RepositionField, axis)
		}
		f, ok := numericValue(v)
		if !ok {
			return [3]float64{}, fmt.Errorf("FIELD_NOT_VECTOR3 field=%s reason=non_numeric axis=%s", RepositionField, axis)
		}
		vec[i] = f
	}
	return vec, nil
}

func numericValue(v any) (float64, bool) {
	switch t := v.(type) {
	case int64:
		return float64(t), true
	case float64:
		return t, true
	default:
		return 0, false
	}
}

func formatVec3(v [3]float64) string {
	return strings.Join([]string{
		strconv.FormatFloat(v[0], 'f', -1, 64),
		strconv.FormatFloat(v[1], 'f', -1, 64),
		strconv.FormatFloat(v[2], 'f', -1, 64),
	}, ",")
}

// rewriteVector3Field locates the field's line (reusing the scalar finder,
// which also matches an inline-mapping value line) and rewrites only the inline
// {x, y, z} value, preserving the key, colon, indentation, and line ending.
func rewriteVector3Field(input []byte, block parser.Block, fieldPath string, vec [3]float64) ([]byte, error) {
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

	valueStart := colon + 1
	for valueStart < len(line) && (line[valueStart] == ' ' || line[valueStart] == '\t') {
		valueStart++
	}

	rewritten, err := rewriteVector3Flow(line[valueStart:], vec)
	if err != nil {
		return nil, err
	}

	lines[lineIndex].content = line[:valueStart] + rewritten
	return joinLines(lines), nil
}

// rewriteVector3Flow rewrites the three axis values of an inline flow mapping
// while preserving every byte of structure around them: brace placement, the
// exact comma/space separators between entries, key order, and any per-entry
// whitespace. Only the x/y/z scalar tokens are replaced. Anything after the
// closing brace (e.g. trailing whitespace) is preserved verbatim. It rejects a
// mapping that is not exactly {x, y, z}.
func rewriteVector3Flow(value string, vec [3]float64) (string, error) {
	if len(value) == 0 || value[0] != '{' {
		return "", fmt.Errorf("FIELD_NOT_FLOW_MAPPING field=%s", RepositionField)
	}
	closeIdx := matchBrace(value, 0)
	if closeIdx == -1 {
		return "", fmt.Errorf("FIELD_NOT_FLOW_MAPPING field=%s reason=unterminated", RepositionField)
	}

	inner := value[1:closeIdx]
	suffix := value[closeIdx+1:]
	entries := splitTopLevelEntries(inner)
	if len(entries) != 3 {
		return "", fmt.Errorf("FIELD_NOT_VECTOR3 field=%s reason=expected_3_axes got=%d", RepositionField, len(entries))
	}

	axisValues := map[string]string{
		"x": strconv.FormatFloat(vec[0], 'f', -1, 64),
		"y": strconv.FormatFloat(vec[1], 'f', -1, 64),
		"z": strconv.FormatFloat(vec[2], 'f', -1, 64),
	}
	seen := make(map[string]bool, 3)

	var b strings.Builder
	b.WriteByte('{')
	for i, entry := range entries {
		if i > 0 {
			b.WriteByte(',')
		}
		colon := strings.IndexByte(entry, ':')
		if colon == -1 {
			return "", fmt.Errorf("FIELD_NOT_VECTOR3 field=%s reason=missing_colon", RepositionField)
		}
		key := strings.TrimSpace(entry[:colon])
		nv, ok := axisValues[key]
		if !ok {
			return "", fmt.Errorf("FIELD_NOT_VECTOR3 field=%s reason=unexpected_key key=%s", RepositionField, key)
		}
		if seen[key] {
			return "", fmt.Errorf("FIELD_NOT_VECTOR3 field=%s reason=duplicate_key key=%s", RepositionField, key)
		}
		seen[key] = true

		valuePart := entry[colon+1:]
		trimmedLeft := strings.TrimLeft(valuePart, " \t")
		lead := valuePart[:len(valuePart)-len(trimmedLeft)]
		trimmed := strings.TrimRight(trimmedLeft, " \t")
		trail := trimmedLeft[len(trimmed):]

		b.WriteString(entry[:colon+1]) // key + colon, preserving spaces before colon
		b.WriteString(lead)            // whitespace between colon and value
		b.WriteString(nv)              // new axis value
		b.WriteString(trail)           // trailing whitespace inside the entry
	}
	b.WriteByte('}')
	b.WriteString(suffix)
	return b.String(), nil
}

// matchBrace returns the index of the '}' that closes the '{' at open, or -1 if
// the braces are unbalanced.
func matchBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitTopLevelEntries splits a flow-mapping body on commas that sit outside any
// nested {} or [] so an entry whose value is itself a mapping/sequence stays
// intact. For a Vector3 there is no nesting, but the depth tracking keeps the
// rewriter correct if it is ever pointed at a richer mapping.
func splitTopLevelEntries(inner string) []string {
	if strings.TrimSpace(inner) == "" {
		return nil
	}
	var entries []string
	depth := 0
	start := 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ',':
			if depth == 0 {
				entries = append(entries, inner[start:i])
				start = i + 1
			}
		}
	}
	entries = append(entries, inner[start:])
	return entries
}
