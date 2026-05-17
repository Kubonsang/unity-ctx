package check

import (
	"reflect"
	"testing"

	"unity-ctx/internal/bounds"
)

func TestCheckPlacementReturnsTranslatedPlacementAABB(t *testing.T) {
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/Stage01.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{
				FileID: 300,
				Name:   "Overlap",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{4.0, 2.5, 3.0},
					Size:   bounds.Vec3{1, 1, 1},
				},
			},
		},
		Prefabs: []bounds.PrefabBounds{
			{
				Path: "Assets/Prefabs/Chair.prefab",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{1, 0.5, -2},
					Size:   bounds.Vec3{2, 1, 4},
				},
			},
		},
	}

	result, err := CheckPlacement(manifest, "Assets/Prefabs/Chair.prefab", bounds.Vec3{3, 2, 5})
	if err != nil {
		t.Fatalf("CheckPlacement() error = %v", err)
	}

	wantPlacement := bounds.AABB{
		Center: bounds.Vec3{4, 2.5, 3},
		Size:   bounds.Vec3{2, 1, 4},
	}
	if !reflect.DeepEqual(result.Placement, wantPlacement) {
		t.Fatalf("CheckPlacement() Placement = %+v, want %+v", result.Placement, wantPlacement)
	}

	if result.Clear {
		t.Fatal("CheckPlacement() Clear = true, want false")
	}

	wantIDs := []int64{300}
	if !reflect.DeepEqual(result.OverlapIDs, wantIDs) {
		t.Fatalf("CheckPlacement() OverlapIDs = %v, want %v", result.OverlapIDs, wantIDs)
	}
}

func TestCheckPlacementDetectsSortedOverlaps(t *testing.T) {
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/Stage01.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{
				FileID: 30,
				Name:   "Far",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{4, 0.5, 0},
					Size:   bounds.Vec3{1, 1, 1},
				},
			},
			{
				FileID: 20,
				Name:   "Near",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{0.5, 0.5, 0},
					Size:   bounds.Vec3{1, 1, 1},
				},
			},
			{
				FileID: 10,
				Name:   "Also Near",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{-0.5, 0.5, 0},
					Size:   bounds.Vec3{1, 1, 1},
				},
			},
		},
		Prefabs: []bounds.PrefabBounds{
			{
				Path: "Assets/Prefabs/Chair.prefab",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{0, 0.5, 0},
					Size:   bounds.Vec3{2, 1, 2},
				},
			},
		},
	}

	result, err := CheckPlacement(manifest, "Assets/Prefabs/Chair.prefab", bounds.Vec3{0, 0, 0})
	if err != nil {
		t.Fatalf("CheckPlacement() error = %v", err)
	}

	if result.Clear {
		t.Fatal("CheckPlacement() Clear = true, want false")
	}

	wantIDs := []int64{10, 20}
	if !reflect.DeepEqual(result.OverlapIDs, wantIDs) {
		t.Fatalf("CheckPlacement() OverlapIDs = %v, want %v", result.OverlapIDs, wantIDs)
	}
}

func TestCheckPlacementTreatsEdgeTouchAsClear(t *testing.T) {
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/Stage01.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{
				FileID: 100,
				Name:   "Table",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{2, 0.5, 0},
					Size:   bounds.Vec3{2, 1, 2},
				},
			},
		},
		Prefabs: []bounds.PrefabBounds{
			{
				Path: "Assets/Prefabs/Chair.prefab",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{0, 0.5, 0},
					Size:   bounds.Vec3{2, 1, 2},
				},
			},
		},
	}

	result, err := CheckPlacement(manifest, "Assets/Prefabs/Chair.prefab", bounds.Vec3{0, 0, 0})
	if err != nil {
		t.Fatalf("CheckPlacement() error = %v", err)
	}

	if !result.Clear {
		t.Fatal("CheckPlacement() Clear = false, want true for edge touch")
	}
	if len(result.OverlapIDs) != 0 {
		t.Fatalf("CheckPlacement() OverlapIDs = %v, want none", result.OverlapIDs)
	}
}

func TestCheckPlacementReturnsMissingPrefabError(t *testing.T) {
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/Stage01.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{},
		Prefabs: []bounds.PrefabBounds{
			{
				Path: "Assets/Prefabs/Table.prefab",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{0, 0.5, 0},
					Size:   bounds.Vec3{2, 1, 2},
				},
			},
		},
	}

	_, err := CheckPlacement(manifest, "Assets/Prefabs/Chair.prefab", bounds.Vec3{0, 0, 0})
	if err == nil {
		t.Fatal("CheckPlacement() error = nil, want missing prefab error")
	}

	want := "missing prefab manifest entry for path=\"Assets/Prefabs/Chair.prefab\""
	if err.Error() != want {
		t.Fatalf("CheckPlacement() error = %q, want %q", err.Error(), want)
	}
}
