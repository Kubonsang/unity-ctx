package suggest

import (
	"reflect"
	"testing"

	"unity-ctx/internal/bounds"
)

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
