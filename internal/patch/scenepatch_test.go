package patch_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"unity-ctx/internal/bounds"
	"unity-ctx/internal/parser"
	"unity-ctx/internal/patch"
)

func TestPlanPlacePrefabClearPlacementReturnsDeterministicPlan(t *testing.T) {
	blocks := loadSceneBlocks(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	manifest := testManifest()

	plan, err := patch.PlanPlacePrefab(patch.PlacePrefabRequest{
		SceneBlocks: blocks,
		Manifest:    manifest,
		PrefabPath:  "Assets/Prefabs/Chair.prefab",
		PrefabRef:   patch.PrefabReference{GUID: "guid-chair"},
		Position:    bounds.Vec3{5, 0, 0},
	})
	if err != nil {
		t.Fatalf("PlanPlacePrefab() error = %v", err)
	}

	if plan.Status != patch.StatusOK {
		t.Fatalf("Status mismatch: got %q want %q", plan.Status, patch.StatusOK)
	}
	if plan.Reason != "" {
		t.Fatalf("Reason mismatch: got %q want empty", plan.Reason)
	}
	if plan.PrefabGUID != "guid-chair" {
		t.Fatalf("PrefabGUID mismatch: got %q want %q", plan.PrefabGUID, "guid-chair")
	}

	wantReserved := []int64{2002, 2003}
	if !reflect.DeepEqual(plan.ReservedFileIDs, wantReserved) {
		t.Fatalf("ReservedFileIDs mismatch: got %v want %v", plan.ReservedFileIDs, wantReserved)
	}

	wantAppends := []patch.AppendIntent{
		{Op: patch.AppendOpAppend, ClassID: 1, FileID: 2002, TypeName: "GameObject"},
		{Op: patch.AppendOpAppend, ClassID: 4, FileID: 2003, TypeName: "Transform"},
	}
	if !reflect.DeepEqual(plan.Appends, wantAppends) {
		t.Fatalf("Appends mismatch: got %#v want %#v", plan.Appends, wantAppends)
	}
}

func TestPlanPlacePrefabOverlapReturnsSortedOverlapIDs(t *testing.T) {
	blocks := loadSceneBlocks(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/Stage01.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{FileID: 30, Name: "Far", Bounds: bounds.AABB{Center: bounds.Vec3{4, 0.5, 0}, Size: bounds.Vec3{1, 1, 1}}},
			{FileID: 20, Name: "Near", Bounds: bounds.AABB{Center: bounds.Vec3{0.5, 0.5, 0}, Size: bounds.Vec3{1, 1, 1}}},
			{FileID: 10, Name: "Also Near", Bounds: bounds.AABB{Center: bounds.Vec3{-0.5, 0.5, 0}, Size: bounds.Vec3{1, 1, 1}}},
		},
		Prefabs: []bounds.PrefabBounds{
			{Path: "Assets/Prefabs/Chair.prefab", Bounds: bounds.AABB{Center: bounds.Vec3{0, 0.5, 0}, Size: bounds.Vec3{2, 1, 2}}},
		},
	}

	plan, err := patch.PlanPlacePrefab(patch.PlacePrefabRequest{
		SceneBlocks: blocks,
		Manifest:    manifest,
		PrefabPath:  "Assets/Prefabs/Chair.prefab",
		PrefabRef:   patch.PrefabReference{GUID: "guid-chair"},
		Position:    bounds.Vec3{0, 0, 0},
	})
	if err != nil {
		t.Fatalf("PlanPlacePrefab() error = %v", err)
	}

	if plan.Status != patch.StatusWarn {
		t.Fatalf("Status mismatch: got %q want %q", plan.Status, patch.StatusWarn)
	}

	wantOverlapIDs := []int64{10, 20}
	if !reflect.DeepEqual(plan.OverlapIDs, wantOverlapIDs) {
		t.Fatalf("OverlapIDs mismatch: got %v want %v", plan.OverlapIDs, wantOverlapIDs)
	}
}

func TestPlanPlacePrefabMissingPrefabManifestEntryReturnsError(t *testing.T) {
	blocks := loadSceneBlocks(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/Stage01.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{},
		Prefabs: []bounds.PrefabBounds{
			{Path: "Assets/Prefabs/Table.prefab", Bounds: bounds.AABB{Center: bounds.Vec3{0, 0.5, 0}, Size: bounds.Vec3{2, 1, 2}}},
		},
	}

	_, err := patch.PlanPlacePrefab(patch.PlacePrefabRequest{
		SceneBlocks: blocks,
		Manifest:    manifest,
		PrefabPath:  "Assets/Prefabs/Chair.prefab",
		PrefabRef:   patch.PrefabReference{GUID: "guid-chair"},
		Position:    bounds.Vec3{0, 0, 0},
	})
	if err == nil {
		t.Fatal("PlanPlacePrefab() error = nil, want missing prefab manifest entry error")
	}

	want := "missing prefab manifest entry for path=\"Assets/Prefabs/Chair.prefab\""
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestPlanPlacePrefabUnresolvedPrefabReferenceReturnsUnknown(t *testing.T) {
	blocks := loadSceneBlocks(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	manifest := testManifest()

	plan, err := patch.PlanPlacePrefab(patch.PlacePrefabRequest{
		SceneBlocks: blocks,
		Manifest:    manifest,
		PrefabPath:  "Assets/Prefabs/Chair.prefab",
		Position:    bounds.Vec3{5, 0, 0},
	})
	if err != nil {
		t.Fatalf("PlanPlacePrefab() error = %v", err)
	}

	if plan.Status != patch.StatusUnknown {
		t.Fatalf("Status mismatch: got %q want %q", plan.Status, patch.StatusUnknown)
	}
	if plan.Reason != patch.ReasonNeedPrefabGUID {
		t.Fatalf("Reason mismatch: got %q want %q", plan.Reason, patch.ReasonNeedPrefabGUID)
	}
	if plan.PrefabGUID != "" {
		t.Fatalf("PrefabGUID mismatch: got %q want empty", plan.PrefabGUID)
	}

	wantReserved := []int64{2002, 2003}
	if !reflect.DeepEqual(plan.ReservedFileIDs, wantReserved) {
		t.Fatalf("ReservedFileIDs mismatch: got %v want %v", plan.ReservedFileIDs, wantReserved)
	}
}

func TestPlanPlacePrefabUsesCurrentMaxSceneFileID(t *testing.T) {
	blocks := []parser.Block{
		{FileID: 7},
		{FileID: 42},
		{FileID: 15},
	}

	plan, err := patch.PlanPlacePrefab(patch.PlacePrefabRequest{
		SceneBlocks: blocks,
		Manifest:    testManifest(),
		PrefabPath:  "Assets/Prefabs/Chair.prefab",
		PrefabRef:   patch.PrefabReference{GUID: "guid-chair"},
		Position:    bounds.Vec3{5, 0, 0},
	})
	if err != nil {
		t.Fatalf("PlanPlacePrefab() error = %v", err)
	}

	wantReserved := []int64{43, 44}
	if !reflect.DeepEqual(plan.ReservedFileIDs, wantReserved) {
		t.Fatalf("ReservedFileIDs mismatch: got %v want %v", plan.ReservedFileIDs, wantReserved)
	}
}

func loadSceneBlocks(t *testing.T, path string) []parser.Block {
	t.Helper()

	blocks, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}
	return blocks
}

func testManifest() bounds.Manifest {
	return bounds.Manifest{
		Scene:   "Assets/Scenes/Stage01.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{FileID: 1000, Name: "Table_01", Bounds: bounds.AABB{Center: bounds.Vec3{5, 0.5, 3}, Size: bounds.Vec3{1, 1, 1}}},
			{FileID: 2000, Name: "Chair_01", Bounds: bounds.AABB{Center: bounds.Vec3{2.1, 0.5, 3.4}, Size: bounds.Vec3{1, 1, 1}}},
		},
		Prefabs: []bounds.PrefabBounds{
			{Path: "Assets/Prefabs/Chair.prefab", Bounds: bounds.AABB{Center: bounds.Vec3{0, 0.5, 0}, Size: bounds.Vec3{1, 1, 1}}},
		},
	}
}
