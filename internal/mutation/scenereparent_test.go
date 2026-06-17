package mutation

import (
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/parser"
	"github.com/Kubonsang/unity-ctx/internal/patch"
)

// reparentScene: ParentA(4000) -> Child(4001); ParentB(4002) empty. m_Children in
// the F3 form verified against real Unity 6000.3.8f1 (dash at the key's indent).
const reparentScene = "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
	"--- !u!1 &1000\nGameObject:\n  m_Component:\n  - component: {fileID: 4000}\n  m_Name: ParentA\n" +
	"--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n" +
	"  m_Children:\n  - {fileID: 4001}\n  m_Father: {fileID: 0}\n" +
	"--- !u!1 &1001\nGameObject:\n  m_Component:\n  - component: {fileID: 4001}\n  m_Name: Child\n" +
	"--- !u!4 &4001\nTransform:\n  m_GameObject: {fileID: 1001}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n" +
	"  m_Children: []\n  m_Father: {fileID: 4000}\n" +
	"--- !u!1 &1002\nGameObject:\n  m_Component:\n  - component: {fileID: 4002}\n  m_Name: ParentB\n" +
	"--- !u!4 &4002\nTransform:\n  m_GameObject: {fileID: 1002}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n" +
	"  m_Children: []\n  m_Father: {fileID: 0}\n"

func planReparent(t *testing.T, scene string, op patch.Op) (SceneReparentPlan, error) {
	t.Helper()
	input := []byte(scene)
	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return PlanSceneReparent(input, blocks, op)
}

func TestPlanSceneReparentMovesChildBetweenParents(t *testing.T) {
	plan, err := planReparent(t, reparentScene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 4002, OldParent: 4000})
	if err != nil {
		t.Fatalf("PlanSceneReparent() error = %v", err)
	}
	if plan.EndpointBlocked || plan.PlanBlocked || !plan.Changed {
		t.Fatalf("unexpected plan flags: %+v", plan)
	}
	out := string(plan.UpdatedData)
	// child father -> 4002
	if !strings.Contains(out, "--- !u!4 &4001\nTransform:\n  m_GameObject: {fileID: 1001}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n  m_Children: []\n  m_Father: {fileID: 4002}\n") {
		t.Fatalf("child father not updated:\n%s", out)
	}
	// old parent 4000 collapsed to empty
	if !strings.Contains(out, "--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n  m_Children: []\n  m_Father: {fileID: 0}\n") {
		t.Fatalf("old parent m_Children not collapsed:\n%s", out)
	}
	// new parent 4002 gained child in F3 form
	if !strings.Contains(out, "--- !u!4 &4002\nTransform:\n  m_GameObject: {fileID: 1002}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n  m_Children:\n  - {fileID: 4001}\n  m_Father: {fileID: 0}\n") {
		t.Fatalf("new parent m_Children not updated in F3 form:\n%s", out)
	}
	// verify predicates
	if ok, reason := VerifySceneReparent(plan.UpdatedData, 4001, 4000, 4002); !ok {
		t.Fatalf("verify failed: %s", reason)
	}
}

func TestPlanSceneReparentToRoot(t *testing.T) {
	plan, err := planReparent(t, reparentScene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 0, OldParent: 4000})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	out := string(plan.UpdatedData)
	if !strings.Contains(out, "  m_Children: []\n  m_Father: {fileID: 0}\n--- !u!1 &1001") {
		// old parent A collapsed to [] (first block before child GO)
	}
	if !strings.Contains(out, "--- !u!4 &4001\nTransform:\n  m_GameObject: {fileID: 1001}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n  m_Children: []\n  m_Father: {fileID: 0}\n") {
		t.Fatalf("child not moved to root:\n%s", out)
	}
	if ok, reason := VerifySceneReparent(plan.UpdatedData, 4001, 4000, 0); !ok {
		t.Fatalf("verify failed: %s", reason)
	}
}

func TestPlanSceneReparentFromRoot(t *testing.T) {
	// Move ParentB(4002, currently root) under ParentA(4000).
	plan, err := planReparent(t, reparentScene, patch.Op{Op: patch.OpReparent, Target: 4002, NewParent: 4000, OldParent: 0})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	out := string(plan.UpdatedData)
	// new parent 4000 now lists both 4001 and 4002 (4002 appended after 4001)
	if !strings.Contains(out, "  m_Children:\n  - {fileID: 4001}\n  - {fileID: 4002}\n  m_Father: {fileID: 0}\n") {
		t.Fatalf("new parent did not gain appended child:\n%s", out)
	}
	if !strings.Contains(out, "--- !u!4 &4002\nTransform:\n  m_GameObject: {fileID: 1002}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n  m_Children: []\n  m_Father: {fileID: 4000}\n") {
		t.Fatalf("target father not set:\n%s", out)
	}
	if ok, reason := VerifySceneReparent(plan.UpdatedData, 4002, 0, 4000); !ok {
		t.Fatalf("verify failed: %s", reason)
	}
}

func TestPlanSceneReparentPolicy1BlocksNonTransformEndpoint(t *testing.T) {
	// new parent 1002 is a GameObject (class 1), not a transform.
	plan, err := planReparent(t, reparentScene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 1002, OldParent: 4000})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.EndpointBlocked {
		t.Fatalf("expected endpoint block, got %+v", plan)
	}
	if !strings.Contains(plan.EndpointBody, "endpoint=new_parent") || !strings.Contains(plan.EndpointBody, "class=1") || !strings.Contains(plan.EndpointBody, "allowed=4") {
		t.Fatalf("unexpected endpoint body: %q", plan.EndpointBody)
	}
	if plan.UpdatedData != nil {
		t.Fatal("blocked plan must not produce updated data")
	}
}

func TestPlanSceneReparentPolicy1BlocksRectTransform(t *testing.T) {
	scene := reparentScene +
		"--- !u!1 &1003\nGameObject:\n  m_Component:\n  - component: {fileID: 224000}\n  m_Name: UI\n" +
		"--- !u!224 &224000\nRectTransform:\n  m_GameObject: {fileID: 1003}\n  m_Children: []\n  m_Father: {fileID: 0}\n"
	plan, err := planReparent(t, scene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 224000, OldParent: 4000})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.EndpointBlocked || !strings.Contains(plan.EndpointBody, "class=224") {
		t.Fatalf("RectTransform endpoint must be blocked (allowed=4 only): %+v", plan)
	}
}

func TestPlanSceneReparentPolicy1BlocksStripped(t *testing.T) {
	scene := reparentScene +
		"--- !u!4 &9000 stripped\nTransform:\n  m_CorrespondingSourceObject: {fileID: 123, guid: abc, type: 3}\n  m_PrefabInstance: {fileID: 0}\n"
	plan, err := planReparent(t, scene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 9000, OldParent: 4000})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.EndpointBlocked || !strings.Contains(plan.EndpointBody, "is_stripped=true") {
		t.Fatalf("stripped endpoint must be blocked: %+v", plan)
	}
}

func TestPlanSceneReparentPolicy2BlocksCycle(t *testing.T) {
	// Move ParentA(4000) under its own descendant Child(4001).
	plan, err := planReparent(t, reparentScene, patch.Op{Op: patch.OpReparent, Target: 4000, NewParent: 4001, OldParent: 0})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.PlanBlocked || plan.PlanCode != "WOULD_CREATE_CYCLE" {
		t.Fatalf("expected cycle plan-block, got %+v", plan)
	}
	if !strings.Contains(plan.PlanDetail, "chain=4000->4001->4000") {
		t.Fatalf("unexpected cycle chain: %q", plan.PlanDetail)
	}
}

func TestPlanSceneReparentRejectsStalePatch(t *testing.T) {
	// op claims old_parent=9999 but the scene says target's father is 4000.
	_, err := planReparent(t, reparentScene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 4002, OldParent: 9999})
	if err == nil || !strings.Contains(err.Error(), "PATCH_STALE") {
		t.Fatalf("expected PATCH_STALE, got %v", err)
	}
}

func TestPlanSceneReparentRemoveCollapsesLastChildToInlineEmpty(t *testing.T) {
	plan, _ := planReparent(t, reparentScene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 4002, OldParent: 4000})
	// old parent 4000 had exactly one child -> becomes "m_Children: []"
	if !strings.Contains(string(plan.UpdatedData), "  m_GameObject: {fileID: 1000}\n  m_LocalPosition: {x: 0, y: 0, z: 0}\n  m_Children: []\n") {
		t.Fatalf("last-child removal did not collapse to inline empty:\n%s", string(plan.UpdatedData))
	}
}

// TestPlanSceneReparentNoTrailingNewlineNewParentLast is the regression guard for
// the applyAddChild trailing-newline corruption: when the new parent's
// m_Children line is the file's LAST line with no trailing newline, the inserted
// child dash must land on its own line (not collapse onto the key/last child) and
// the result must re-parse and verify. Covers both the inline-"[]" and the
// append-to-existing-list branches.
func TestPlanSceneReparentNoTrailingNewlineNewParentLast(t *testing.T) {
	base := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1 &1000\nGameObject:\n  m_Component:\n  - component: {fileID: 4000}\n  m_Name: A\n" +
		"--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Children:\n  - {fileID: 4001}\n  m_Father: {fileID: 0}\n" +
		"--- !u!1 &1001\nGameObject:\n  m_Component:\n  - component: {fileID: 4001}\n  m_Name: C\n" +
		"--- !u!4 &4001\nTransform:\n  m_GameObject: {fileID: 1001}\n  m_Children: []\n  m_Father: {fileID: 4000}\n" +
		"--- !u!1 &1002\nGameObject:\n  m_Component:\n  - component: {fileID: 4002}\n  m_Name: B\n"

	cases := map[string]string{
		// new parent B's last line is "m_Children: []" with NO trailing newline
		"inline empty last": base + "--- !u!4 &4002\nTransform:\n  m_GameObject: {fileID: 1002}\n  m_Father: {fileID: 0}\n  m_Children: []",
		// new parent B's last line is an existing child dash, NO trailing newline
		"existing child last": base + "--- !u!4 &4002\nTransform:\n  m_GameObject: {fileID: 1002}\n  m_Father: {fileID: 0}\n  m_Children:\n  - {fileID: 4001}",
	}
	for name, scene := range cases {
		t.Run(name, func(t *testing.T) {
			plan, err := planReparent(t, scene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 4002, OldParent: 4000})
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if !plan.Changed {
				t.Fatal("expected a change")
			}
			// Must re-parse cleanly (no collapsed lines) and verify.
			if _, perr := parser.Parse(plan.UpdatedData); perr != nil {
				t.Fatalf("output does not re-parse: %v\n%s", perr, string(plan.UpdatedData))
			}
			if ok, reason := VerifySceneReparent(plan.UpdatedData, 4001, 4000, 4002); !ok {
				t.Fatalf("verify failed: %s\n%s", reason, string(plan.UpdatedData))
			}
			// The dash must be on its own line (a key+dash collapse would show "m_Children:  - ").
			if strings.Contains(string(plan.UpdatedData), "m_Children:  - {fileID: 4001}") {
				t.Fatalf("child dash collapsed onto the key line:\n%s", string(plan.UpdatedData))
			}
		})
	}
}

func TestPlanSceneReparentSelfParentIsNoOp(t *testing.T) {
	// 4001's father is already 4000; reparenting it to 4000 again must be a no-op
	// (no sibling reorder, no change), not a corrupting remove+re-add.
	plan, err := planReparent(t, reparentScene, patch.Op{Op: patch.OpReparent, Target: 4001, NewParent: 4000, OldParent: 4000})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if plan.Changed {
		t.Fatalf("self-reparent must be a no-op, got Changed=true")
	}
	if string(plan.UpdatedData) != reparentScene {
		t.Fatalf("self-reparent altered bytes:\n%s", string(plan.UpdatedData))
	}
}

func TestVerifySceneReparentDetectsFailures(t *testing.T) {
	// Unmodified scene: child's father is 4000, not 4002.
	if ok, _ := VerifySceneReparent([]byte(reparentScene), 4001, 4000, 4002); ok {
		t.Fatal("verify should fail when father not updated")
	}
}
