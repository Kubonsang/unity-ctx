package check

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
)

func TestSpatialWallBackedContactPasses(t *testing.T) {
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := CheckSpatialPlacement(SpatialRequest{Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Position: bounds.Vec3{0, 0, 3.87}, Rotation: bounds.Quat{0, 1, 0, 0}, SurfaceID: "wall-north", Contact: "wall-backed"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Clear {
		t.Fatalf("expected clear placement, codes=%v overlaps=%v gap=%g", result.Codes, result.OverlapIDs, result.Gap)
	}
	if result.Gap < .01 || result.Gap > .05 {
		t.Fatalf("gap=%g", result.Gap)
	}
}

func TestSpatialCheckRefusesManifestV1(t *testing.T) {
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = CheckSpatialPlacement(SpatialRequest{Manifest: manifest, Prefab: "Assets/Prefabs/chair.prefab", Rotation: bounds.Quat{0, 0, 0, 1}, Contact: "wall-backed", SurfaceID: "wall"})
	if !errors.Is(err, ErrNeedGeometryV2) {
		t.Fatalf("got %v, want NEED_GEOMETRY_V2", err)
	}
}
