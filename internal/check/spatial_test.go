package check

import (
	"errors"
	"math"
	"path/filepath"
	"slices"
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

func TestSpatialWallContactRejectsPointOutsideFinitePatch(t *testing.T) {
	manifest := loadSpatialFixture(t)
	result, err := CheckSpatialPlacement(SpatialRequest{
		Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab",
		Position: bounds.Vec3{100, 0, 3.87}, Rotation: bounds.Quat{0, 1, 0, 0},
		SurfaceID: "wall-north", Contact: "wall-backed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Clear || !slices.Contains(result.Codes, "INSUFFICIENT_SUPPORT") || result.Support != 0 {
		t.Fatalf("expected finite wall rejection, result=%#v", result)
	}
}

func TestSpatialCheckRefusesUnreviewedPrefabGeometry(t *testing.T) {
	manifest := loadSpatialFixture(t)
	manifest.Prefabs[0].Spatial.Reviewed = false
	_, err := CheckSpatialPlacement(SpatialRequest{Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Rotation: bounds.Quat{0, 0, 0, 1}})
	if !errors.Is(err, ErrGeometryUnreviewed) {
		t.Fatalf("got %v, want GEOMETRY_UNREVIEWED", err)
	}
}

func TestSpatialCheckRefusesUnreviewedRoomGeometry(t *testing.T) {
	manifest := loadSpatialFixture(t)
	manifest.Objects[0].Spatial.Reviewed = false
	_, err := CheckSpatialPlacement(SpatialRequest{Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Rotation: bounds.Quat{0, 0, 0, 1}})
	if !errors.Is(err, ErrRoomGeometryUnreviewed) {
		t.Fatalf("got %v, want ROOM_GEOMETRY_UNREVIEWED", err)
	}
}

func TestSpatialCheckRejectsContactKindOnWrongSurfaceType(t *testing.T) {
	manifest := loadSpatialFixture(t)
	result, err := CheckSpatialPlacement(SpatialRequest{
		Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab",
		Position: bounds.Vec3{0, 0, 0}, Rotation: bounds.Quat{0, 0, 0, 1},
		SurfaceID: "wall-north", Contact: "floor-supported",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Clear || !slices.Contains(result.Codes, "CONTACT_DIRECTION") {
		t.Fatalf("expected wrong surface type rejection, result=%#v", result)
	}
}

func TestSpatialCheckRequiresApprovedContactPolicy(t *testing.T) {
	manifest := loadSpatialFixture(t)
	manifest.Prefabs[0].Spatial.Contacts = nil
	result, err := CheckSpatialPlacement(SpatialRequest{
		Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab",
		Position: bounds.Vec3{0, 0, 3.87}, Rotation: bounds.Quat{0, 1, 0, 0},
		SurfaceID: "wall-north", Contact: "wall-backed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Clear || !slices.Contains(result.Codes, "SUPPORT_CONTRACT_MISSING") {
		t.Fatalf("expected missing policy rejection, result=%#v", result)
	}
}

func TestSpatialCeilingMountedUsesReviewedTopFrame(t *testing.T) {
	manifest := loadSpatialFixture(t)
	profile := manifest.Prefabs[0].Spatial
	profile.TopContact = &bounds.ContactFrame{ID: "top", Point: bounds.Vec3{0, 2, 0}, Normal: bounds.Vec3{0, 1, 0}, Tangent: bounds.Vec3{1, 0, 0}, Size: [2]float64{1, .2}}
	profile.Contacts = append(profile.Contacts, bounds.ContactRequirement{
		ID: "ceiling", Kind: "CeilingMounted", FrameID: "top", Target: "surface:ceiling",
		MinimumGap: .005, MaximumGap: .01, MinimumSupport: 1, DirectionAlignment: .95,
	})
	manifest.Surfaces = append(manifest.Surfaces, bounds.SurfacePatch{
		ID: "ceiling-main", Type: "ceiling", Origin: bounds.Vec3{0, 4, 0}, Normal: bounds.Vec3{0, -1, 0}, Tangent: bounds.Vec3{1, 0, 0}, Size: [2]float64{8, 8}, Reviewed: true, Supported: true,
	})
	result, err := CheckSpatialPlacement(SpatialRequest{
		Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Position: bounds.Vec3{0, 1.9925, 0},
		Rotation: bounds.Quat{0, 0, 0, 1}, SurfaceID: "ceiling-main", Contact: "ceiling-mounted",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Clear || math.Abs(result.Gap-.0075) > 1e-6 || result.Support < .999999 {
		t.Fatalf("unexpected ceiling evidence: %#v", result)
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

func loadSpatialFixture(t *testing.T) bounds.Manifest {
	t.Helper()
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
