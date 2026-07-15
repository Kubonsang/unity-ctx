package suggest

import (
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
	"github.com/Kubonsang/unity-ctx/internal/check"
)

func TestPlanWallUsesReviewedSurfaceAndRotation(t *testing.T) {
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Plan(Request{Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Count: 4, Align: AlignWall, SurfaceID: "wall-north"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Align != AlignWall || len(result.Candidates) != 4 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Contact != "wall-backed" {
		t.Fatalf("contact=%q, want wall-backed", result.Contact)
	}
	for _, candidate := range result.Candidates {
		if candidate.Rotation == (bounds.Quat{}) {
			t.Fatal("wall candidate has no rotation")
		}
		if math.Abs(candidate.Position[1]-0.005) > 1e-6 {
			t.Fatalf("bookcase must be floor supported, y=%g", candidate.Position[1])
		}
		wall, err := checkWallCandidate(manifest, candidate, "wall-backed", "wall-north")
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(wall.Gap-0.03) > 1e-6 || wall.Support < 0.999999 {
			t.Fatalf("unexpected wall evidence: %#v", wall)
		}
		floor, err := checkWallCandidate(manifest, candidate, "floor-supported", "floor-main")
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(floor.Gap-0.005) > 1e-6 || floor.Support < 0.6 {
			t.Fatalf("unexpected floor evidence: %#v", floor)
		}
	}
}

func TestPlanWallUsesWallMountedPolicyWithoutFloorProjection(t *testing.T) {
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Prefabs[0].Spatial.Contacts = []bounds.ContactRequirement{{
		ID: "mount", Kind: "WallMounted", FrameID: "back", Target: "surface:wall",
		MinimumGap: .005, MaximumGap: .01, MinimumSupport: 1, DirectionAlignment: .95,
	}}
	result, err := Plan(Request{Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Count: 1, Align: AlignWall, SurfaceID: "wall-north"})
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := checkWallCandidate(manifest, result.Candidates[0], "wall-mounted", "wall-north")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(evidence.Gap-.0075) > 1e-6 {
		t.Fatalf("gap=%g, want .0075", evidence.Gap)
	}
}

func TestPlanWallRejectsMissingReviewedContactPolicy(t *testing.T) {
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Prefabs[0].Spatial.Contacts = nil
	_, err = Plan(Request{Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Count: 1, Align: AlignWall, SurfaceID: "wall-north"})
	if err == nil || err.Error() != `SUPPORT_CONTRACT_MISSING contact=""` {
		t.Fatalf("got %v", err)
	}
}

func TestPlanWallRejectsUnreviewedGeometry(t *testing.T) {
	manifest, err := bounds.Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Prefabs[0].Spatial.Reviewed = false
	_, err = Plan(Request{Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Count: 1, Align: AlignWall, SurfaceID: "wall-north"})
	if !errors.Is(err, check.ErrGeometryUnreviewed) {
		t.Fatalf("got %v, want GEOMETRY_UNREVIEWED", err)
	}
}

func checkWallCandidate(manifest bounds.Manifest, candidate Candidate, contact, surfaceID string) (check.SpatialResult, error) {
	return check.CheckSpatialPlacement(check.SpatialRequest{
		Manifest: manifest, Prefab: "Assets/Prefabs/Bookcase.prefab", Position: candidate.Position,
		Rotation: candidate.Rotation, SurfaceID: surfaceID, Contact: contact,
	})
}

func TestPlanReturnsFourDirectionalCandidates(t *testing.T) {
	manifest := testManifest()

	result, err := Plan(Request{
		Manifest: manifest,
		Prefab:   "Assets/Prefabs/Chair.prefab",
		Near:     "100",
		Count:    4,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	if result.Status != "OK" {
		t.Fatalf("Plan() Status = %q, want %q", result.Status, "OK")
	}

	wantDirections := []string{"east", "west", "north", "south"}
	gotDirections := make([]string, 0, len(result.Candidates))
	for i, candidate := range result.Candidates {
		gotDirections = append(gotDirections, candidate.Direction)
		if candidate.Rank != i+1 {
			t.Fatalf("Plan() candidate rank = %d, want %d", candidate.Rank, i+1)
		}
	}

	if !reflect.DeepEqual(gotDirections, wantDirections) {
		t.Fatalf("Plan() directions = %v, want %v", gotDirections, wantDirections)
	}
}

func TestPlanFloorAlignmentPlacesPrefabOnGround(t *testing.T) {
	manifest := testManifest()

	result, err := Plan(Request{
		Manifest: manifest,
		Prefab:   "Assets/Prefabs/OffsetChair.prefab",
		Near:     "100",
		Count:    1,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	candidate := result.Candidates[0]
	if candidate.Position[1] != 0.5 {
		t.Fatalf("Plan() candidate y = %v, want %v", candidate.Position[1], 0.5)
	}
	bottom := candidate.Position[1] + manifest.Prefabs[1].Bounds.Center[1] - manifest.Prefabs[1].Bounds.Size[1]/2
	if bottom != 0 {
		t.Fatalf("Plan() placement bottom = %v, want 0", bottom)
	}
}

func TestPlanGridAlignmentSnapsXZToHalfMeterGrid(t *testing.T) {
	manifest := testManifest()
	manifest.Objects = append(manifest.Objects, bounds.ObjectBounds{
		FileID: 200,
		Name:   "Shelf",
		Bounds: bounds.AABB{
			Center: bounds.Vec3{0.8, 0.5, 0.3},
			Size:   bounds.Vec3{2.2, 1, 1.4},
		},
	})

	result, err := Plan(Request{
		Manifest: manifest,
		Prefab:   "Assets/Prefabs/Chair.prefab",
		Near:     "Shelf",
		Count:    1,
		Align:    AlignGrid,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	candidate := result.Candidates[0]
	if centerX := candidate.Position[0] + manifest.Prefabs[0].Bounds.Center[0]; centerX != 2.5 {
		t.Fatalf("Plan() placement center x = %v, want %v", centerX, 2.5)
	}
	if centerZ := candidate.Position[2] + manifest.Prefabs[0].Bounds.Center[2]; centerZ != 0.5 {
		t.Fatalf("Plan() placement center z = %v, want %v", centerZ, 0.5)
	}
}

func TestPlanRanksClearCandidatesBeforeWarnCandidates(t *testing.T) {
	manifest := testManifest()
	manifest.Objects = append(manifest.Objects, bounds.ObjectBounds{
		FileID: 400,
		Name:   "BlockEast",
		Bounds: bounds.AABB{
			Center: bounds.Vec3{2, 0.5, 0},
			Size:   bounds.Vec3{1, 1, 1},
		},
	})

	result, err := Plan(Request{
		Manifest: manifest,
		Prefab:   "Assets/Prefabs/Chair.prefab",
		Near:     "100",
		Count:    4,
	})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}

	want := []struct {
		direction string
		status    string
	}{
		{direction: "west", status: "OK"},
		{direction: "north", status: "OK"},
		{direction: "south", status: "OK"},
		{direction: "east", status: "WARN"},
	}

	if len(result.Candidates) != len(want) {
		t.Fatalf("Plan() candidate count = %d, want %d", len(result.Candidates), len(want))
	}

	for i, wantCandidate := range want {
		got := result.Candidates[i]
		if got.Direction != wantCandidate.direction || got.Status != wantCandidate.status {
			t.Fatalf("Plan() candidate[%d] = {%q %q}, want {%q %q}", i, got.Direction, got.Status, wantCandidate.direction, wantCandidate.status)
		}
	}

	if result.Status != "OK" {
		t.Fatalf("Plan() Status = %q, want %q", result.Status, "OK")
	}
}

func TestPlanRejectsMissingAnchor(t *testing.T) {
	_, err := Plan(Request{
		Manifest: testManifest(),
		Prefab:   "Assets/Prefabs/Chair.prefab",
		Near:     "Missing",
		Count:    1,
	})
	if err == nil {
		t.Fatal("Plan() error = nil, want missing anchor error")
	}

	want := `missing anchor near="Missing"`
	if err.Error() != want {
		t.Fatalf("Plan() error = %q, want %q", err.Error(), want)
	}
}

func TestPlanRejectsAmbiguousAnchorName(t *testing.T) {
	manifest := testManifest()
	manifest.Objects = append(manifest.Objects, bounds.ObjectBounds{
		FileID: 401,
		Name:   "Duplicate",
		Bounds: bounds.AABB{
			Center: bounds.Vec3{0, 0.5, 4},
			Size:   bounds.Vec3{1, 1, 1},
		},
	}, bounds.ObjectBounds{
		FileID: 402,
		Name:   "Duplicate",
		Bounds: bounds.AABB{
			Center: bounds.Vec3{0, 0.5, 6},
			Size:   bounds.Vec3{1, 1, 1},
		},
	})

	_, err := Plan(Request{
		Manifest: manifest,
		Prefab:   "Assets/Prefabs/Chair.prefab",
		Near:     "Duplicate",
		Count:    1,
	})
	if err == nil {
		t.Fatal("Plan() error = nil, want ambiguous anchor error")
	}

	want := `AMBIGUOUS_NAME name="Duplicate" matches=2`
	if err.Error() != want {
		t.Fatalf("Plan() error = %q, want %q", err.Error(), want)
	}
}

func TestPlanRejectsCountBelowOne(t *testing.T) {
	_, err := Plan(Request{
		Manifest: testManifest(),
		Prefab:   "Assets/Prefabs/Chair.prefab",
		Near:     "100",
		Count:    0,
	})
	if err == nil {
		t.Fatal("Plan() error = nil, want count validation error")
	}

	want := "count must be >= 1"
	if err.Error() != want {
		t.Fatalf("Plan() error = %q, want %q", err.Error(), want)
	}
}

func testManifest() bounds.Manifest {
	return bounds.Manifest{
		Scene:   "Assets/Scenes/Test.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{
				FileID: 100,
				Name:   "Anchor",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{0, 0.5, 0},
					Size:   bounds.Vec3{2, 1, 2},
				},
			},
		},
		Prefabs: []bounds.PrefabBounds{
			{
				Path: "Assets/Prefabs/Chair.prefab",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{0, 0.5, 0},
					Size:   bounds.Vec3{1, 1, 1},
				},
			},
			{
				Path: "Assets/Prefabs/OffsetChair.prefab",
				Bounds: bounds.AABB{
					Center: bounds.Vec3{0, 0, 0},
					Size:   bounds.Vec3{1, 1, 1},
				},
			},
		},
	}
}
