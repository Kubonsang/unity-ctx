package mutation

import (
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/parser"
	"github.com/Kubonsang/unity-ctx/internal/patch"
)

// deleteScene: Root(GO 1000 / T 4000) -> Child(GO 1001 / T 4001 + MB 114001) ->
// Grandchild(GO 1002 / T 4002). m_Children in the real Unity F3 form.
const deleteScene = "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
	"--- !u!1 &1000\nGameObject:\n  m_Component:\n  - component: {fileID: 4000}\n  m_Name: Root\n" +
	"--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Children:\n  - {fileID: 4001}\n  m_Father: {fileID: 0}\n" +
	"--- !u!1 &1001\nGameObject:\n  m_Component:\n  - component: {fileID: 4001}\n  - component: {fileID: 114001}\n  m_Name: Child\n" +
	"--- !u!4 &4001\nTransform:\n  m_GameObject: {fileID: 1001}\n  m_Children:\n  - {fileID: 4002}\n  m_Father: {fileID: 4000}\n" +
	"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n" +
	"--- !u!1 &1002\nGameObject:\n  m_Component:\n  - component: {fileID: 4002}\n  m_Name: Grandchild\n" +
	"--- !u!4 &4002\nTransform:\n  m_GameObject: {fileID: 1002}\n  m_Children: []\n  m_Father: {fileID: 4001}\n"

func planDelete(t *testing.T, scene string, target int64, cascade bool) (SceneDeletePlan, error) {
	t.Helper()
	input := []byte(scene)
	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return PlanSceneDelete(input, blocks, patch.Op{Op: patch.OpDelete, Target: target, Cascade: cascade}, "")
}

func planDeleteGUID(t *testing.T, scene, sceneGUID string, target int64, cascade bool) (SceneDeletePlan, error) {
	t.Helper()
	input := []byte(scene)
	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return PlanSceneDelete(input, blocks, patch.Op{Op: patch.OpDelete, Target: target, Cascade: cascade}, sceneGUID)
}

func sameIDs(got []int64, want ...int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestPlanSceneDeleteLeaf(t *testing.T) {
	plan, err := planDelete(t, deleteScene, 1002, false) // grandchild, no children
	if err != nil {
		t.Fatalf("PlanSceneDelete() error = %v", err)
	}
	if plan.EndpointBlocked || plan.PlanBlocked || !plan.Changed {
		t.Fatalf("unexpected plan flags: %+v", plan)
	}
	if !sameIDs(plan.DeletedFileIDs, 1002, 4002) {
		t.Fatalf("DeletedFileIDs = %v, want [1002 4002]", plan.DeletedFileIDs)
	}
	if plan.ParentTransform != 4001 || plan.TargetTransform != 4002 {
		t.Fatalf("parent/target transform = %d/%d, want 4001/4002", plan.ParentTransform, plan.TargetTransform)
	}
	out := string(plan.UpdatedData)
	if strings.Contains(out, "&1002") || strings.Contains(out, "&4002") {
		t.Fatalf("deleted blocks still present:\n%s", out)
	}
	// Parent 4001's m_Children collapses to [] (only child removed).
	if !strings.Contains(out, "--- !u!4 &4001\nTransform:\n  m_GameObject: {fileID: 1001}\n  m_Children: []\n  m_Father: {fileID: 4000}\n") {
		t.Fatalf("parent m_Children not collapsed:\n%s", out)
	}
	if ok, reason := VerifySceneDelete(plan.UpdatedData, plan.DeletedFileIDs, plan.ParentTransform, plan.TargetTransform); !ok {
		t.Fatalf("verify failed: %s", reason)
	}
	// Surviving blocks remain parseable and intact.
	if _, err := parser.Parse(plan.UpdatedData); err != nil {
		t.Fatalf("result does not parse: %v", err)
	}
}

func TestPlanSceneDeleteCascade(t *testing.T) {
	plan, err := planDelete(t, deleteScene, 1001, true) // child + its subtree
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if plan.PlanBlocked || plan.EndpointBlocked {
		t.Fatalf("unexpected block: %+v", plan)
	}
	if !sameIDs(plan.DeletedFileIDs, 1001, 1002, 4001, 4002, 114001) {
		t.Fatalf("DeletedFileIDs = %v, want [1001 1002 4001 4002 114001]", plan.DeletedFileIDs)
	}
	out := string(plan.UpdatedData)
	for _, marker := range []string{"&1001", "&4001", "&114001", "&1002", "&4002"} {
		if strings.Contains(out, marker) {
			t.Fatalf("subtree block %s still present:\n%s", marker, out)
		}
	}
	// Root 4000's m_Children collapses to [].
	if !strings.Contains(out, "--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Children: []\n  m_Father: {fileID: 0}\n") {
		t.Fatalf("root m_Children not collapsed:\n%s", out)
	}
	if ok, reason := VerifySceneDelete(plan.UpdatedData, plan.DeletedFileIDs, plan.ParentTransform, plan.TargetTransform); !ok {
		t.Fatalf("verify failed: %s", reason)
	}
}

func TestPlanSceneDeleteWouldOrphan(t *testing.T) {
	plan, err := planDelete(t, deleteScene, 1001, false) // has child 1002, no cascade
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.PlanBlocked || plan.PlanCode != "WOULD_ORPHAN_CHILDREN" {
		t.Fatalf("expected WOULD_ORPHAN_CHILDREN, got %+v", plan)
	}
	if plan.UpdatedData != nil {
		t.Fatal("blocked plan must not produce updated data")
	}
}

func TestPlanSceneDeleteBlocksNonGameObject(t *testing.T) {
	plan, err := planDelete(t, deleteScene, 4002, false) // a Transform, not a GameObject
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.EndpointBlocked || !strings.Contains(plan.EndpointBody, "class=4") || !strings.Contains(plan.EndpointBody, "allowed=1") {
		t.Fatalf("expected non-GameObject endpoint block, got %+v", plan)
	}
}

func TestPlanSceneDeleteBlocksMissingTransform(t *testing.T) {
	// A GameObject whose only component is a MonoBehaviour (no Transform).
	scene := "%YAML 1.1\n--- !u!1 &1000\nGameObject:\n  m_Component:\n  - component: {fileID: 114000}\n  m_Name: NoXform\n" +
		"--- !u!114 &114000\nMonoBehaviour:\n  m_GameObject: {fileID: 1000}\n"
	plan, err := planDelete(t, scene, 1000, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.EndpointBlocked || !strings.Contains(plan.EndpointBody, "MISSING_TRANSFORM") {
		t.Fatalf("expected MISSING_TRANSFORM, got %+v", plan)
	}
}

func TestPlanSceneDeleteBlocksStripped(t *testing.T) {
	// Target GameObject is a stripped prefab-instance object.
	scene := "%YAML 1.1\n--- !u!1 &1000 stripped\nGameObject:\n  m_CorrespondingSourceObject: {fileID: 5, guid: abc, type: 3}\n"
	plan, err := planDelete(t, scene, 1000, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.EndpointBlocked || !strings.Contains(plan.EndpointBody, "is_stripped=true") {
		t.Fatalf("expected stripped endpoint block, got %+v", plan)
	}
}

func TestPlanSceneDeleteBlocksInFileReferenced(t *testing.T) {
	// MonoBehaviour 114001 (on the surviving Child) references the grandchild's
	// transform 4002 via a same-file PPtr; deleting 1002 would dangle it.
	scene := strings.Replace(deleteScene,
		"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n",
		"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n  m_Target: {fileID: 4002}\n",
		1)
	plan, err := planDelete(t, scene, 1002, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.PlanBlocked || plan.PlanCode != "IN_FILE_REFERENCED" {
		t.Fatalf("expected IN_FILE_REFERENCED, got %+v", plan)
	}
	if !strings.Contains(plan.PlanDetail, "block=114001") || !strings.Contains(plan.PlanDetail, "fileID=4002") {
		t.Fatalf("unexpected detail: %q", plan.PlanDetail)
	}
}

func TestPlanSceneDeleteAtRoot(t *testing.T) {
	// A root-level leaf object (no parent m_Children to edit).
	scene := "%YAML 1.1\n--- !u!1 &1000\nGameObject:\n  m_Component:\n  - component: {fileID: 4000}\n  m_Name: Lonely\n" +
		"--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Children: []\n  m_Father: {fileID: 0}\n"
	plan, err := planDelete(t, scene, 1000, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if plan.EndpointBlocked || plan.PlanBlocked || plan.ParentTransform != 0 {
		t.Fatalf("unexpected: %+v", plan)
	}
	if ok, reason := VerifySceneDelete(plan.UpdatedData, plan.DeletedFileIDs, plan.ParentTransform, plan.TargetTransform); !ok {
		t.Fatalf("verify failed: %s", reason)
	}
	if strings.TrimSpace(string(plan.UpdatedData)) != "%YAML 1.1" {
		t.Fatalf("expected only the header to remain, got:\n%q", string(plan.UpdatedData))
	}
}

func TestPlanSceneDeleteNotFound(t *testing.T) {
	if _, err := planDelete(t, deleteScene, 9999, false); err == nil || !strings.Contains(err.Error(), "NOT_FOUND") {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

func TestPlanSceneDeleteBlocksStrippedParent(t *testing.T) {
	// The target is a normal leaf, but its parent transform is a stripped
	// prefab-instance transform whose m_Children cannot be edited locally.
	scene := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1 &1001\nGameObject:\n  m_Component:\n  - component: {fileID: 4001}\n  m_Name: Child\n" +
		"--- !u!4 &4001\nTransform:\n  m_GameObject: {fileID: 1001}\n  m_Children: []\n  m_Father: {fileID: 9000}\n" +
		"--- !u!224 &9000 stripped\nRectTransform:\n  m_CorrespondingSourceObject: {fileID: 1, guid: abc, type: 3}\n"
	plan, err := planDelete(t, scene, 1001, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.PlanBlocked || plan.PlanCode != "PARENT_STRIPPED" {
		t.Fatalf("expected PARENT_STRIPPED, got %+v", plan)
	}
}

func TestPlanSceneDeleteRootUnlinksSceneRoots(t *testing.T) {
	// A real scene registers root transforms in a SceneRoots (class 1660057539)
	// m_Roots list. Deleting a root object must unlink it from m_Roots, not BLOCK.
	scene := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1660057539 &1\nSceneRoots:\n  m_ObjectHideFlags: 0\n  m_Roots:\n  - {fileID: 4000}\n  - {fileID: 4002}\n" +
		"--- !u!1 &1000\nGameObject:\n  m_Component:\n  - component: {fileID: 4000}\n  m_Name: RootA\n" +
		"--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Children: []\n  m_Father: {fileID: 0}\n" +
		"--- !u!1 &1002\nGameObject:\n  m_Component:\n  - component: {fileID: 4002}\n  m_Name: RootB\n" +
		"--- !u!4 &4002\nTransform:\n  m_GameObject: {fileID: 1002}\n  m_Children: []\n  m_Father: {fileID: 0}\n"
	plan, err := planDelete(t, scene, 1000, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if plan.EndpointBlocked || plan.PlanBlocked {
		t.Fatalf("root delete must not be blocked (SceneRoots must be unlinked): %+v", plan)
	}
	out := string(plan.UpdatedData)
	if strings.Contains(out, "&1000") || strings.Contains(out, "&4000") {
		t.Fatalf("blocks not removed:\n%s", out)
	}
	// SceneRoots keeps the OTHER root (4002) but drops 4000.
	if !strings.Contains(out, "  m_Roots:\n  - {fileID: 4002}\n") {
		t.Fatalf("SceneRoots m_Roots not correctly unlinked:\n%s", out)
	}
	if ok, reason := VerifySceneDelete(plan.UpdatedData, plan.DeletedFileIDs, plan.ParentTransform, plan.TargetTransform); !ok {
		t.Fatalf("verify failed: %s", reason)
	}
}

func TestPlanSceneDeleteFlowSequenceInFileRefBlocks(t *testing.T) {
	// 114001 references the deleted grandchild transform 4002 via a FLOW sequence
	// (parser renders it as an opaque string); the raw-text backstop must catch it.
	scene := strings.Replace(deleteScene,
		"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n",
		"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n  m_Targets: [{fileID: 4002}]\n",
		1)
	plan, err := planDelete(t, scene, 1002, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.PlanBlocked || plan.PlanCode != "IN_FILE_REFERENCED" {
		t.Fatalf("flow-sequence in-file ref must BLOCK, got %+v", plan)
	}
}

func TestPlanSceneDeleteAllowsNullComponent(t *testing.T) {
	// A null/broken component serializes as {fileID: 0}; it must not leak into the
	// removed set (which would spuriously match a {fileID: 0, guid: scene} ref).
	scene := "%YAML 1.1\n--- !u!1 &1000\nGameObject:\n  m_Component:\n  - component: {fileID: 4000}\n  - component: {fileID: 0}\n  m_Name: Obj\n" +
		"--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Children: []\n  m_Father: {fileID: 0}\n"
	plan, err := planDelete(t, scene, 1000, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if plan.EndpointBlocked || plan.PlanBlocked {
		t.Fatalf("unexpected block: %+v", plan)
	}
	if !sameIDs(plan.DeletedFileIDs, 1000, 4000) {
		t.Fatalf("DeletedFileIDs = %v, want [1000 4000] (no fileID 0)", plan.DeletedFileIDs)
	}
}

func TestPlanSceneDeleteInFileFlowWithExtraFieldBlocks(t *testing.T) {
	// A surviving same-file flow PPtr carrying an extra field (`[{fileID: N, type: 0}]`,
	// guid-less) — the parser leaves it opaque; the raw brace-aware scan must catch it.
	scene := strings.Replace(deleteScene,
		"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n",
		"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n  m_Refs: [{fileID: 4002, type: 0}]\n",
		1)
	plan, err := planDelete(t, scene, 1002, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.PlanBlocked || plan.PlanCode != "IN_FILE_REFERENCED" {
		t.Fatalf("flow-with-extra-field in-file ref must BLOCK, got %+v", plan)
	}
}

func TestPlanSceneDeleteInFileSelfQualifiedGUIDBlocks(t *testing.T) {
	// A same-file ref that fully-qualifies with the scene's OWN guid
	// ({fileID: 4002, guid: <sceneGUID>}) — only detectable when the in-file check
	// knows the scene guid.
	const sceneGUID = "0123456789abcdef0123456789abcdef"
	scene := strings.Replace(deleteScene,
		"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n",
		"--- !u!114 &114001\nMonoBehaviour:\n  m_GameObject: {fileID: 1001}\n  m_Enabled: 1\n  m_Self: {fileID: 4002, guid: "+sceneGUID+", type: 2}\n",
		1)
	// Without the scene guid the ref looks cross-file (not in-file): not blocked here.
	if plan, _ := planDelete(t, scene, 1002, false); plan.PlanBlocked {
		t.Fatalf("without scene guid, a guid-qualified ref is treated cross-file, not in-file: %+v", plan)
	}
	// With the scene guid, it is recognized as a same-file dangling ref -> BLOCK.
	plan, err := planDeleteGUID(t, scene, sceneGUID, 1002, false)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !plan.PlanBlocked || plan.PlanCode != "IN_FILE_REFERENCED" {
		t.Fatalf("self-qualified-guid same-file ref must BLOCK, got %+v", plan)
	}
}
