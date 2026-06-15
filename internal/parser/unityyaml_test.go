package parser

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestParseFileSimpleScenePreservesBlockIdentity(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	blocks, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if len(blocks) != 4 {
		t.Fatalf("block count mismatch: got %d want 4", len(blocks))
	}

	first := blocks[0]
	if first.ClassID != 1 {
		t.Fatalf("first ClassID mismatch: got %d want 1", first.ClassID)
	}
	if first.FileID != 1000 {
		t.Fatalf("first FileID mismatch: got %d want 1000", first.FileID)
	}
	if first.TypeName != "GameObject" {
		t.Fatalf("first TypeName mismatch: got %q want %q", first.TypeName, "GameObject")
	}
	if first.StartLine != 3 {
		t.Fatalf("first StartLine mismatch: got %d want 3", first.StartLine)
	}
	if first.EndLine != 8 {
		t.Fatalf("first EndLine mismatch: got %d want 8", first.EndLine)
	}
}

func TestParseFileUnknownComponentPreservesTypeName(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "prefabs", "unknown_component.prefab")

	blocks, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("block count mismatch: got %d want 1", len(blocks))
	}

	if blocks[0].TypeName != "UnknownComponent" {
		t.Fatalf("TypeName mismatch: got %q want %q", blocks[0].TypeName, "UnknownComponent")
	}
}

func TestParseRejectsUnexpectedContentOutsideHeader(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: Root\n" +
		"--- broken header\n" +
		"Transform:\n" +
		"  m_GameObject: {fileID: 1000}\n")

	if _, err := Parse(input); err == nil {
		t.Fatal("expected Parse() to reject unexpected content")
	}
}

func TestParseRejectsBrokenHeaderInsideDocument(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: Root\n" +
		"--- broken header\n" +
		"  m_IsActive: 1\n")

	if _, err := Parse(input); err == nil {
		t.Fatal("expected Parse() to reject broken header inside document")
	}
}

func TestParseInlineMapValue(t *testing.T) {
	input := []byte(`%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!4 &1001
Transform:
  m_LocalPosition: {x: 5, y: 0, z: 3}
`)

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("block count mismatch: got %d want 1", len(blocks))
	}

	got, ok := blocks[0].Fields["m_LocalPosition"].(map[string]any)
	if !ok {
		t.Fatalf("m_LocalPosition type mismatch: got %T want map[string]any", blocks[0].Fields["m_LocalPosition"])
	}

	if got["x"] != int64(5) {
		t.Fatalf("x mismatch: got %#v want %d", got["x"], 5)
	}
	if got["y"] != int64(0) {
		t.Fatalf("y mismatch: got %#v want %d", got["y"], 0)
	}
	if got["z"] != int64(3) {
		t.Fatalf("z mismatch: got %#v want %d", got["z"], 3)
	}
}

func TestParseFileMaterialPreservesNestedContainerContent(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "assets", "material.mat")

	blocks, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("block count mismatch: got %d want 1", len(blocks))
	}

	savedProperties, ok := blocks[0].Fields["m_SavedProperties"].(map[string]any)
	if !ok {
		t.Fatalf("m_SavedProperties type mismatch: got %T want map[string]any", blocks[0].Fields["m_SavedProperties"])
	}

	colors, ok := savedProperties["m_Colors"].([]any)
	if !ok {
		t.Fatalf("m_Colors type mismatch: got %T want []any", savedProperties["m_Colors"])
	}

	if len(colors) != 1 {
		t.Fatalf("m_Colors length mismatch: got %d want 1", len(colors))
	}

	firstColor, ok := colors[0].(map[string]any)
	if !ok {
		t.Fatalf("first color entry type mismatch: got %T want map[string]any", colors[0])
	}

	colorValue, ok := firstColor["_Color"].(map[string]any)
	if !ok {
		t.Fatalf("_Color type mismatch: got %T want map[string]any", firstColor["_Color"])
	}

	if colorValue["r"] != 0.8 {
		t.Fatalf("r mismatch: got %#v want %v", colorValue["r"], 0.8)
	}
	if colorValue["a"] != int64(1) {
		t.Fatalf("a mismatch: got %#v want %d", colorValue["a"], 1)
	}
}

func TestParseNestedMapPreservesSiblingAfterList(t *testing.T) {
	input := []byte(`%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!21 &2100000
Material:
  m_SavedProperties:
    m_Colors:
    - _Color: {r: 0.8, g: 0.6, b: 0.4, a: 1}
    m_Floats:
    - _Glossiness: 0.5
`)

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("block count mismatch: got %d want 1", len(blocks))
	}

	savedProperties, ok := blocks[0].Fields["m_SavedProperties"].(map[string]any)
	if !ok {
		t.Fatalf("m_SavedProperties type mismatch: got %T want map[string]any", blocks[0].Fields["m_SavedProperties"])
	}

	colors, ok := savedProperties["m_Colors"].([]any)
	if !ok {
		t.Fatalf("m_Colors type mismatch: got %T want []any", savedProperties["m_Colors"])
	}
	if len(colors) != 1 {
		t.Fatalf("m_Colors length mismatch: got %d want 1", len(colors))
	}

	floats, ok := savedProperties["m_Floats"].([]any)
	if !ok {
		t.Fatalf("m_Floats type mismatch: got %T want []any", savedProperties["m_Floats"])
	}
	if len(floats) != 1 {
		t.Fatalf("m_Floats length mismatch: got %d want 1", len(floats))
	}

	firstFloat, ok := floats[0].(map[string]any)
	if !ok {
		t.Fatalf("first float entry type mismatch: got %T want map[string]any", floats[0])
	}
	if firstFloat["_Glossiness"] != 0.5 {
		t.Fatalf("_Glossiness mismatch: got %#v want %v", firstFloat["_Glossiness"], 0.5)
	}
}

func TestParseEmptyScalarPreservesSiblingAtSameIndent(t *testing.T) {
	input := []byte(`%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!21 &2100000
Material:
  m_SavedProperties:
    m_Empty:
    m_Floats:
    - _Glossiness: 0.5
`)

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("block count mismatch: got %d want 1", len(blocks))
	}

	savedProperties, ok := blocks[0].Fields["m_SavedProperties"].(map[string]any)
	if !ok {
		t.Fatalf("m_SavedProperties type mismatch: got %T want map[string]any", blocks[0].Fields["m_SavedProperties"])
	}

	if savedProperties["m_Empty"] != "" {
		t.Fatalf("m_Empty mismatch: got %#v want empty string", savedProperties["m_Empty"])
	}

	floats, ok := savedProperties["m_Floats"].([]any)
	if !ok {
		t.Fatalf("m_Floats type mismatch: got %T want []any", savedProperties["m_Floats"])
	}
	if len(floats) != 1 {
		t.Fatalf("m_Floats length mismatch: got %d want 1", len(floats))
	}
}

func TestParseInlineEmptyListValue(t *testing.T) {
	input := []byte(`%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!21 &2100000
Material:
  m_Tags: []
`)

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	got, ok := blocks[0].Fields["m_Tags"].([]any)
	if !ok {
		t.Fatalf("m_Tags type mismatch: got %T want []any", blocks[0].Fields["m_Tags"])
	}
	if len(got) != 0 {
		t.Fatalf("m_Tags length mismatch: got %d want 0", len(got))
	}
}

func TestParseEscapedDoubleQuotedString(t *testing.T) {
	input := []byte("%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n--- !u!21 &2100000\nMaterial:\n  m_Name: \"a\\\"b\"\n")

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if blocks[0].Fields["m_Name"] != `a"b` {
		t.Fatalf("m_Name mismatch: got %#v want %q", blocks[0].Fields["m_Name"], `a"b`)
	}
}

func TestParseEscapedSingleQuotedString(t *testing.T) {
	input := []byte("%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n--- !u!21 &2100000\nMaterial:\n  m_Name: 'it''s'\n")

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if blocks[0].Fields["m_Name"] != "it's" {
		t.Fatalf("m_Name mismatch: got %#v want %q", blocks[0].Fields["m_Name"], "it's")
	}
}

func TestParseFinalBlockEndLineIgnoresTrailingNewline(t *testing.T) {
	input := []byte("%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n--- !u!1 &1000\nGameObject:\n  m_Name: Root\n--- !u!4 &1001\nTransform:\n  m_GameObject: {fileID: 1000}\n")

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(blocks) != 2 {
		t.Fatalf("block count mismatch: got %d want 2", len(blocks))
	}

	last := blocks[1]
	if last.StartLine != 6 {
		t.Fatalf("last StartLine mismatch: got %d want 6", last.StartLine)
	}
	if last.EndLine != 8 {
		t.Fatalf("last EndLine mismatch: got %d want 8", last.EndLine)
	}
}

func TestParseBlankScalarPreservesSiblingField(t *testing.T) {
	input := []byte("%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n--- !u!114 &11400000\nMonoBehaviour:\n  m_EditorClassIdentifier:\n  m_Name: Example\n")

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if blocks[0].Fields["m_EditorClassIdentifier"] != "" {
		t.Fatalf("m_EditorClassIdentifier mismatch: got %#v want empty string", blocks[0].Fields["m_EditorClassIdentifier"])
	}
	if blocks[0].Fields["m_Name"] != "Example" {
		t.Fatalf("m_Name mismatch: got %#v want %q", blocks[0].Fields["m_Name"], "Example")
	}
}

func TestParseHeaderAcceptsStrippedObjectSuffix(t *testing.T) {
	input := []byte("%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n--- !u!1 &123 stripped\nGameObject:\n  m_Name: Child\n")

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("block count mismatch: got %d want 1", len(blocks))
	}
	if blocks[0].ClassID != 1 {
		t.Fatalf("ClassID mismatch: got %d want 1", blocks[0].ClassID)
	}
	if blocks[0].FileID != 123 {
		t.Fatalf("FileID mismatch: got %d want 123", blocks[0].FileID)
	}
}

func TestParseListBareInlineMapItem(t *testing.T) {
	input := []byte(`%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!4 &1001
Transform:
  m_Children:
  - {fileID: 123}
`)

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	children, ok := blocks[0].Fields["m_Children"].([]any)
	if !ok {
		t.Fatalf("m_Children type mismatch: got %T want []any", blocks[0].Fields["m_Children"])
	}
	if len(children) != 1 {
		t.Fatalf("m_Children length mismatch: got %d want 1", len(children))
	}

	child, ok := children[0].(map[string]any)
	if !ok {
		t.Fatalf("child type mismatch: got %T want map[string]any", children[0])
	}
	if child["fileID"] != int64(123) {
		t.Fatalf("fileID mismatch: got %#v want %d", child["fileID"], 123)
	}
}

func TestParseQuotedListScalarContainingColon(t *testing.T) {
	input := []byte(`%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!4 &1001
Transform:
  m_Labels:
  - "https://example"
`)

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	labels, ok := blocks[0].Fields["m_Labels"].([]any)
	if !ok {
		t.Fatalf("m_Labels type mismatch: got %T want []any", blocks[0].Fields["m_Labels"])
	}
	if len(labels) != 1 {
		t.Fatalf("m_Labels length mismatch: got %d want 1", len(labels))
	}
	if labels[0] != "https://example" {
		t.Fatalf("label mismatch: got %#v want %q", labels[0], "https://example")
	}
}

func TestParseInlineMapWithEscapedQuoteAndCommaInQuotedValue(t *testing.T) {
	input := []byte("%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n--- !u!21 &2100000\nMaterial:\n  m_Meta: {label: \"a\\\\\\\"b,c\", n: 1}\n")

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	meta, ok := blocks[0].Fields["m_Meta"].(map[string]any)
	if !ok {
		t.Fatalf("m_Meta type mismatch: got %T want map[string]any", blocks[0].Fields["m_Meta"])
	}
	if meta["label"] != "a\\\"b,c" {
		t.Fatalf("label mismatch: got %#v want %q", meta["label"], "a\\\"b,c")
	}
	if meta["n"] != int64(1) {
		t.Fatalf("n mismatch: got %#v want %d", meta["n"], 1)
	}
}

func TestParseUnquotedListScalarContainingColon(t *testing.T) {
	input := []byte(`%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!4 &1001
Transform:
  m_Labels:
  - https://example
`)

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	labels, ok := blocks[0].Fields["m_Labels"].([]any)
	if !ok {
		t.Fatalf("m_Labels type mismatch: got %T want []any", blocks[0].Fields["m_Labels"])
	}
	if len(labels) != 1 {
		t.Fatalf("m_Labels length mismatch: got %d want 1", len(labels))
	}
	if labels[0] != "https://example" {
		t.Fatalf("label mismatch: got %#v want %q", labels[0], "https://example")
	}
}

func TestParseInlineMapWithListValueAndSiblingKey(t *testing.T) {
	input := []byte(`%YAML 1.1
%TAG !u! tag:unity3d.com,2011:
--- !u!21 &2100000
Material:
  m_Meta: {items: [1, 2], n: 1}
`)

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	meta, ok := blocks[0].Fields["m_Meta"].(map[string]any)
	if !ok {
		t.Fatalf("m_Meta type mismatch: got %T want map[string]any", blocks[0].Fields["m_Meta"])
	}
	if meta["items"] != "[1, 2]" {
		t.Fatalf("items mismatch: got %#v want %q", meta["items"], "[1, 2]")
	}
	if meta["n"] != int64(1) {
		t.Fatalf("n mismatch: got %#v want %d", meta["n"], 1)
	}
}

func TestParseEmptyInputReturnsNoBlocks(t *testing.T) {
	blocks, err := Parse([]byte{})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("block count mismatch: got %d want 0", len(blocks))
	}
}

func TestParseNilInputReturnsNoBlocks(t *testing.T) {
	blocks, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("block count mismatch: got %d want 0", len(blocks))
	}
}

func TestParseHeaderOnlyInputReturnsNoBlocks(t *testing.T) {
	input := []byte("%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n")

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("block count mismatch: got %d want 0", len(blocks))
	}
}

func TestParseHeaderOnlyInputWithoutTrailingNewline(t *testing.T) {
	// Preamble line with no terminating newline and no document blocks.
	input := []byte("%YAML 1.1")

	blocks, err := Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("block count mismatch: got %d want 0", len(blocks))
	}
}

func TestParseRejectsInvalidUTF8AtTopLevel(t *testing.T) {
	input := []byte{0xff, 0xfe, 0x00, 0x01}

	if _, err := Parse(input); err == nil {
		t.Fatal("expected Parse() to reject invalid UTF-8 input")
	}
}

func TestParseRejectsInvalidUTF8InsideBlockBody(t *testing.T) {
	// A structurally valid header/body whose field value carries an invalid
	// UTF-8 byte sequence. This must be rejected, not silently accepted, so the
	// undecodable bytes never reach Fields or RawBody.
	input := append([]byte("%YAML 1.1\n--- !u!1 &1000\nGameObject:\n  m_Name: "), 0xff, 0xfe)
	input = append(input, '\n')

	if _, err := Parse(input); err == nil {
		t.Fatal("expected Parse() to reject invalid UTF-8 inside a block body")
	}
}

// TestParseIsDeterministic guards against any non-determinism in parser output.
// Go map iteration is randomized, so if any value-construction path in the
// parser depended on map order the structured result could vary between runs.
// We parse the same fixture many times and assert the results are byte-for-byte
// identical once normalized into a key-ordered canonical form.
func TestParseIsDeterministic(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "assets", "material.mat")

	const iterations = 20
	var reference string
	for i := 0; i < iterations; i++ {
		blocks, err := ParseFile(path)
		if err != nil {
			t.Fatalf("ParseFile() iteration %d error = %v", i, err)
		}

		canonical := canonicalizeBlocks(blocks)
		if i == 0 {
			reference = canonical
			continue
		}
		if canonical != reference {
			t.Fatalf("non-deterministic parse output on iteration %d:\n--- first ---\n%s\n--- iteration %d ---\n%s", i, reference, i, canonical)
		}
	}
}

func TestParseIsDeterministicAcrossAllFixtures(t *testing.T) {
	fixtures := []string{
		filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"),
		filepath.Join("..", "..", "testdata", "prefabs", "unknown_component.prefab"),
	}

	for _, path := range fixtures {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			first, err := ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}
			second, err := ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}

			a := canonicalizeBlocks(first)
			b := canonicalizeBlocks(second)
			if a != b {
				t.Fatalf("non-deterministic parse output:\n--- first ---\n%s\n--- second ---\n%s", a, b)
			}

			// reflect.DeepEqual is order-insensitive for maps and so confirms
			// the structured content (not just the canonical string) matches.
			if !reflect.DeepEqual(first, second) {
				t.Fatal("repeated ParseFile() produced structurally different blocks")
			}
		})
	}
}

// canonicalizeBlocks renders parsed blocks into a stable, key-ordered string so
// that two results can be compared independent of Go map iteration order.
func canonicalizeBlocks(blocks []Block) string {
	var sb strings.Builder
	for _, block := range blocks {
		fmt.Fprintf(&sb, "block classID=%d fileID=%d type=%q start=%d end=%d\n", block.ClassID, block.FileID, block.TypeName, block.StartLine, block.EndLine)
		fmt.Fprintf(&sb, "rawBody=%q\n", block.RawBody)
		canonicalizeValue(&sb, block.Fields, 0)
	}
	return sb.String()
}

func canonicalizeValue(sb *strings.Builder, value any, depth int) {
	indent := strings.Repeat("  ", depth)
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(sb, "%s%s:\n", indent, k)
			canonicalizeValue(sb, v[k], depth+1)
		}
	case []any:
		for idx, item := range v {
			fmt.Fprintf(sb, "%s- [%d]\n", indent, idx)
			canonicalizeValue(sb, item, depth+1)
		}
	default:
		fmt.Fprintf(sb, "%s= %T:%#v\n", indent, v, v)
	}
}
