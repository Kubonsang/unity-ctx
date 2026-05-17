package check

import (
	"fmt"
	"sort"

	"unity-ctx/internal/bounds"
)

type PlacementResult struct {
	Clear      bool
	Placement  bounds.AABB
	OverlapIDs []int64
}

func CheckPlacement(manifest bounds.Manifest, prefabPath string, position bounds.Vec3) (PlacementResult, error) {
	prefabBounds, ok := findPrefabBounds(manifest.Prefabs, prefabPath)
	if !ok {
		return PlacementResult{}, fmt.Errorf("missing prefab manifest entry for path=%q", prefabPath)
	}

	placement := translateAABB(prefabBounds, position)
	overlapIDs := make([]int64, 0, len(manifest.Objects))
	for _, object := range manifest.Objects {
		if intersectsAABB(placement, object.Bounds) {
			overlapIDs = append(overlapIDs, object.FileID)
		}
	}

	sort.Slice(overlapIDs, func(i, j int) bool {
		return overlapIDs[i] < overlapIDs[j]
	})

	return PlacementResult{
		Clear:      len(overlapIDs) == 0,
		Placement:  placement,
		OverlapIDs: overlapIDs,
	}, nil
}

func findPrefabBounds(prefabs []bounds.PrefabBounds, prefabPath string) (bounds.AABB, bool) {
	for _, prefab := range prefabs {
		if prefab.Path == prefabPath {
			return prefab.Bounds, true
		}
	}
	return bounds.AABB{}, false
}

func translateAABB(box bounds.AABB, offset bounds.Vec3) bounds.AABB {
	return bounds.AABB{
		Center: bounds.Vec3{
			box.Center[0] + offset[0],
			box.Center[1] + offset[1],
			box.Center[2] + offset[2],
		},
		Size: box.Size,
	}
}

func intersectsAABB(a bounds.AABB, b bounds.AABB) bool {
	return overlapsOnAxis(a.Center[0], a.Size[0], b.Center[0], b.Size[0]) &&
		overlapsOnAxis(a.Center[1], a.Size[1], b.Center[1], b.Size[1]) &&
		overlapsOnAxis(a.Center[2], a.Size[2], b.Center[2], b.Size[2])
}

func overlapsOnAxis(centerA, sizeA, centerB, sizeB float64) bool {
	minA, maxA := axisRange(centerA, sizeA)
	minB, maxB := axisRange(centerB, sizeB)
	return minA < maxB && minB < maxA
}

func axisRange(center, size float64) (float64, float64) {
	half := size / 2
	return center - half, center + half
}
