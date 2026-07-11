package check

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
)

func TestSharedSpatialFixtureMatchesOBBSATVerdicts(t *testing.T) {
	var fixture struct {
		Cases []struct {
			ID      string     `json:"id"`
			Left    fixtureBox `json:"left"`
			Right   fixtureBox `json:"right"`
			Overlap bool       `json:"overlap"`
		} `json:"cases"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "spatial", "spatial_cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, item := range fixture.Cases {
		left := fixtureWorldOBB(item.Left)
		right := fixtureWorldOBB(item.Right)
		if got := intersectsOBB(left, right); got != item.Overlap {
			t.Fatalf("%s overlap=%v want=%v", item.ID, got, item.Overlap)
		}
	}
}

type fixtureBox struct {
	Center   bounds.Vec3 `json:"center"`
	Size     bounds.Vec3 `json:"size"`
	Rotation bounds.Quat `json:"rotation"`
}

func fixtureWorldOBB(value fixtureBox) worldOBB {
	q, _ := normalizedQuat(value.Rotation)
	return worldOBB{center: value.Center, extents: mul(value.Size, .5), axes: [3]bounds.Vec3{rotate(q, bounds.Vec3{1, 0, 0}), rotate(q, bounds.Vec3{0, 1, 0}), rotate(q, bounds.Vec3{0, 0, 1})}}
}
