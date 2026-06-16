package parser

import (
	"reflect"
	"testing"
)

func TestStripTrailingComment(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"map with comment", "{x: 1, y: 2, z: 3}  # note", "{x: 1, y: 2, z: 3}  "},
		{"brace inside comment", "{x: 5, y: 0, z: 3} # }{", "{x: 5, y: 0, z: 3} "},
		{"empty seq with comment", "[] # children", "[] "},
		{"no comment", "{x: 1, y: 2, z: 3}", "{x: 1, y: 2, z: 3}"},
		{"hash inside double quote", `"a # b"`, `"a # b"`},
		{"hash inside single quote", `'a # b'`, `'a # b'`},
		{"hash without preceding space is data", "a#b", "a#b"},
		{"hash inside braces is not a comment", `{x: "#", y: 0}`, `{x: "#", y: 0}`},
		{"leading hash is whole comment", "# all", ""},
		{"trailing comment after scalar", "hello # c", "hello "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripTrailingComment(tc.in); got != tc.want {
				t.Fatalf("stripTrailingComment(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseValueInlineMapWithTrailingComment(t *testing.T) {
	got := parseValue("{x: 1, y: 2, z: 3}   # anchor")
	want := map[string]any{"x": int64(1), "y": int64(2), "z": int64(3)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseValue with trailing comment = %#v, want %#v", got, want)
	}
}

// TestParseValueScalarCommentUnchanged locks the scope of the fix: stripping is
// applied only for flow-collection detection. A plain scalar that contains '#'
// is left exactly as-is, so the change does not loosen scalar parsing.
func TestParseValueScalarCommentUnchanged(t *testing.T) {
	if got := parseValue("hello # c"); got != "hello # c" {
		t.Fatalf("scalar parsing was loosened: parseValue(%q) = %#v", "hello # c", got)
	}
}
