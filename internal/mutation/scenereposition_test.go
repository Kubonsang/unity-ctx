package mutation

import (
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/parser"
)

const repositionScene = "" +
	"%YAML 1.1\n" +
	"%TAG !u! tag:unity3d.com,2011:\n" +
	"--- !u!1 &1000\n" +
	"GameObject:\n" +
	"  m_Name: Table_01\n" +
	"  m_Component:\n" +
	"  - component: {fileID: 1001}\n" +
	"--- !u!4 &1001\n" +
	"Transform:\n" +
	"  m_GameObject: {fileID: 1000}\n" +
	"  m_LocalPosition: {x: 5, y: 0, z: 3}\n" +
	"  m_LocalRotation: {x: 0, y: 0, z: 0, w: 1}\n" +
	"  m_LocalScale: {x: 1, y: 1, z: 1}\n" +
	"  m_Father: {fileID: 0}\n" +
	"  m_Children: []\n"

// TestRewriteVector3FlowPreservesStructure is the core regression guard for the
// reposition net-negative: only the three axis tokens may change; every byte of
// surrounding structure (braces, separators, key order, per-entry whitespace,
// trailing content) must survive verbatim.
func TestRewriteVector3FlowPreservesStructure(t *testing.T) {
	tests := []struct {
		name string
		in   string
		vec  [3]float64
		want string
	}{
		{"canonical", "{x: 5, y: 0, z: 3}", [3]float64{1, 2, 3}, "{x: 1, y: 2, z: 3}"},
		{"no spaces", "{x:5,y:0,z:3}", [3]float64{1, 2, 3}, "{x:1,y:2,z:3}"},
		{"reordered keys", "{z: 3, y: 0, x: 5}", [3]float64{7, 8, 9}, "{z: 9, y: 8, x: 7}"},
		{"irregular whitespace", "{z: 3,y:0,  x: 5}", [3]float64{7, 8, 9}, "{z: 9,y:8,  x: 7}"},
		{"negative and decimal", "{x: 5, y: 0, z: 3}", [3]float64{-3.4, 0, 2.5}, "{x: -3.4, y: 0, z: 2.5}"},
		{"large value no exponent", "{x: 5, y: 0, z: 3}", [3]float64{1e10, 0, 0}, "{x: 10000000000, y: 0, z: 0}"},
		{"tiny value no exponent", "{x: 5, y: 0, z: 3}", [3]float64{0.00001, 0, 0}, "{x: 0.00001, y: 0, z: 0}"},
		{"trailing content preserved", "{x: 5, y: 0, z: 3}   ", [3]float64{1, 1, 1}, "{x: 1, y: 1, z: 1}   "},
		{"trailing comment preserved", "{x: 5, y: 0, z: 3} # anchor", [3]float64{1, 1, 1}, "{x: 1, y: 1, z: 1} # anchor"},
		{"brace inside trailing comment", "{x:5,y:0,z:3} # }{", [3]float64{2, 2, 2}, "{x:2,y:2,z:2} # }{"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rewriteVector3Flow(tc.in, tc.vec)
			if err != nil {
				t.Fatalf("rewriteVector3Flow(%q) error = %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("rewriteVector3Flow(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestRewriteVector3FlowRejectsNonVector3 proves the rewriter refuses anything
// that is not exactly {x, y, z}, so a misaddressed field (e.g. a Quaternion)
// fails loudly instead of being silently corrupted.
func TestRewriteVector3FlowRejectsNonVector3(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"quaternion four keys", "{x: 0, y: 0, z: 0, w: 1}"},
		{"missing axis", "{x: 0, y: 0}"},
		{"unexpected key", "{x: 0, y: 0, w: 1}"},
		{"duplicate axis", "{x: 0, x: 1, y: 0}"},
		{"not a mapping", "0"},
		{"unterminated", "{x: 0, y: 0, z: 0"},
		{"missing colon", "{x 0, y: 0, z: 0}"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := rewriteVector3Flow(tc.in, [3]float64{1, 1, 1}); err == nil {
				t.Fatalf("rewriteVector3Flow(%q) expected error, got nil", tc.in)
			}
		})
	}
}

func TestPlanSceneRepositionRewritesOnlyTargetLine(t *testing.T) {
	input := []byte(repositionScene)
	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	plan, err := PlanSceneReposition(input, blocks, SceneRepositionRequest{
		Path:     "scene.unity",
		ID:       1001,
		Position: [3]float64{1.5, 2, -3.4},
		Rewrite:  true,
	})
	if err != nil {
		t.Fatalf("PlanSceneReposition() error = %v", err)
	}
	if !plan.Changed {
		t.Fatal("expected Changed=true")
	}
	if plan.OldValue != "5,0,3" || plan.NewValue != "1.5,2,-3.4" {
		t.Fatalf("value mismatch: old=%q new=%q", plan.OldValue, plan.NewValue)
	}

	// Exactly one line differs, and it is the target m_LocalPosition line.
	wantLine := "  m_LocalPosition: {x: 1.5, y: 2, z: -3.4}"
	gotLines := strings.Split(string(plan.UpdatedData), "\n")
	srcLines := strings.Split(repositionScene, "\n")
	diffs := 0
	for i := range srcLines {
		if gotLines[i] != srcLines[i] {
			diffs++
			if gotLines[i] != wantLine {
				t.Fatalf("line %d changed to %q, want %q", i, gotLines[i], wantLine)
			}
		}
	}
	if diffs != 1 {
		t.Fatalf("expected exactly 1 changed line, got %d", diffs)
	}
}

// TestPlanSceneRepositionPreservesCRLF guards that a Windows-line-ending scene
// round-trips: only the target axis values change, every \r\n stays intact.
func TestPlanSceneRepositionPreservesCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(repositionScene, "\n", "\r\n")
	input := []byte(crlf)
	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	plan, err := PlanSceneReposition(input, blocks, SceneRepositionRequest{
		Path: "scene.unity", ID: 1001, Position: [3]float64{1, 2, 3}, Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanSceneReposition() error = %v", err)
	}

	want := strings.ReplaceAll(crlf,
		"  m_LocalPosition: {x: 5, y: 0, z: 3}\r\n",
		"  m_LocalPosition: {x: 1, y: 2, z: 3}\r\n")
	if string(plan.UpdatedData) != want {
		t.Fatalf("CRLF not preserved:\n got %q\nwant %q", string(plan.UpdatedData), want)
	}
}

// TestPlanSceneRepositionTabSeparator covers a tab between the colon and the
// flow mapping (surfaced by adversarial probing). It must round-trip, not
// refuse: the tab is preserved and only the axis values change.
func TestPlanSceneRepositionTabSeparator(t *testing.T) {
	scene := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!4 &1001\nTransform:\n  m_GameObject: {fileID: 1000}\n" +
		"  m_LocalPosition:\t{x: 5, y: 0, z: 3}\n  m_LocalScale: {x: 1, y: 1, z: 1}\n"
	input := []byte(scene)
	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	plan, err := PlanSceneReposition(input, blocks, SceneRepositionRequest{
		Path: "scene.unity", ID: 1001, Position: [3]float64{9, 8, 7}, Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanSceneReposition() error = %v", err)
	}
	if !strings.Contains(string(plan.UpdatedData), "  m_LocalPosition:\t{x: 9, y: 8, z: 7}\n") {
		t.Fatalf("tab separator not preserved / not repositioned:\n%q", string(plan.UpdatedData))
	}
}

// TestPlanSceneRepositionRectTransform proves reposition works on a
// RectTransform (class 224), whose m_LocalPosition is also a Vector3, and that
// its sibling Vector2 m_AnchorMin {x, y} is left untouched.
func TestPlanSceneRepositionRectTransform(t *testing.T) {
	scene := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!224 &555\nRectTransform:\n  m_GameObject: {fileID: 111}\n" +
		"  m_LocalPosition: {x: 1, y: 2, z: 3}\n  m_AnchorMin: {x: 0, y: 0}\n" +
		"  m_Father: {fileID: 0}\n  m_Children: []\n"
	input := []byte(scene)
	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	plan, err := PlanSceneReposition(input, blocks, SceneRepositionRequest{
		Path: "scene.unity", ID: 555, Position: [3]float64{9, 8, 7}, Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanSceneReposition() error = %v", err)
	}
	out := string(plan.UpdatedData)
	if !strings.Contains(out, "  m_LocalPosition: {x: 9, y: 8, z: 7}\n") {
		t.Fatalf("RectTransform not repositioned:\n%s", out)
	}
	if !strings.Contains(out, "  m_AnchorMin: {x: 0, y: 0}\n") {
		t.Fatalf("sibling Vector2 m_AnchorMin altered:\n%s", out)
	}
}

// TestPlanSceneRepositionRefusesBlockStyle proves a block-style (non-flow)
// m_LocalPosition is refused rather than corrupted. Unity never serializes a
// Vector3 this way, but a hand-edited file might.
func TestPlanSceneRepositionRefusesBlockStyle(t *testing.T) {
	blockStyle := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1 &1000\nGameObject:\n  m_Name: T\n  m_Component:\n  - component: {fileID: 1001}\n" +
		"--- !u!4 &1001\nTransform:\n  m_GameObject: {fileID: 1000}\n" +
		"  m_LocalPosition:\n    x: 5\n    y: 0\n    z: 3\n" +
		"  m_Father: {fileID: 0}\n  m_Children: []\n"
	input := []byte(blockStyle)
	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, err = PlanSceneReposition(input, blocks, SceneRepositionRequest{
		Path: "scene.unity", ID: 1001, Position: [3]float64{1, 2, 3}, Rewrite: true,
	})
	if err == nil || !strings.Contains(err.Error(), "FIELD_NOT_FLOW_MAPPING") {
		t.Fatalf("expected FIELD_NOT_FLOW_MAPPING refusal, got %v", err)
	}
}

func TestPlanSceneRepositionNoOpWhenUnchanged(t *testing.T) {
	input := []byte(repositionScene)
	blocks, _ := parser.Parse(input)

	plan, err := PlanSceneReposition(input, blocks, SceneRepositionRequest{
		Path:     "scene.unity",
		ID:       1001,
		Position: [3]float64{5, 0, 3},
		Rewrite:  true,
	})
	if err != nil {
		t.Fatalf("PlanSceneReposition() error = %v", err)
	}
	if plan.Changed {
		t.Fatal("expected Changed=false for identical position")
	}
	if string(plan.UpdatedData) != repositionScene {
		t.Fatal("no-op plan must leave bytes untouched")
	}
}

func TestPlanSceneRepositionErrors(t *testing.T) {
	input := []byte(repositionScene)
	blocks, _ := parser.Parse(input)

	tests := []struct {
		name    string
		path    string
		id      int64
		wantSub string
	}{
		{"gameobject has no m_LocalPosition", "scene.unity", 1000, "FIELD_NOT_FOUND"},
		{"missing fileID", "scene.unity", 4242, "NOT_FOUND"},
		{"non-scene file kind", "thing.asset", 1001, "UNSUPPORTED_FILE_KIND"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PlanSceneReposition(input, blocks, SceneRepositionRequest{
				Path:     tc.path,
				ID:       tc.id,
				Position: [3]float64{0, 0, 0},
				Rewrite:  true,
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}
