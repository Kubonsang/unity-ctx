package suggest

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"unity-ctx/internal/bounds"
	"unity-ctx/internal/check"
)

type Align string

const (
	AlignFloor Align = "floor"
	AlignGrid  Align = "grid"
)

type Anchor struct {
	FileID int64
	Name   string
}

type Candidate struct {
	Rank       int
	Status     string
	Direction  string
	Position   bounds.Vec3
	OverlapIDs []int64
}

type Request struct {
	Manifest bounds.Manifest
	Prefab   string
	Near     string
	Count    int
	Align    Align
}

type Result struct {
	Status     string
	Manifest   string
	PrefabPath string
	Near       Anchor
	Align      Align
	Count      int
	Candidates []Candidate
}

type directionSpec struct {
	name  string
	order int
}

var directionOrder = []directionSpec{
	{name: "east", order: 0},
	{name: "west", order: 1},
	{name: "north", order: 2},
	{name: "south", order: 3},
}

func Plan(req Request) (Result, error) {
	if req.Count < 1 {
		return Result{}, fmt.Errorf("count must be >= 1")
	}
	align := req.Align
	if align == "" {
		align = AlignFloor
	}

	anchorObject, err := resolveAnchor(req.Manifest, req.Near)
	if err != nil {
		return Result{}, err
	}

	anchor := Anchor{
		FileID: anchorObject.FileID,
		Name:   anchorObject.Name,
	}

	prefabBounds, err := prefabBounds(req.Manifest, req.Prefab)
	if err != nil {
		return Result{}, err
	}

	candidates := make([]Candidate, 0, len(directionOrder))
	for _, spec := range directionOrder {
		position := candidatePosition(anchorObject.Bounds, prefabBounds, spec.name, align)
		placement, err := check.CheckPlacement(req.Manifest, req.Prefab, position)
		if err != nil {
			return Result{}, err
		}
		overlapIDs := excludeFileID(placement.OverlapIDs, anchorObject.FileID)

		status := "WARN"
		if len(overlapIDs) == 0 {
			status = "OK"
		}

		candidates = append(candidates, Candidate{
			Status:     status,
			Direction:  spec.name,
			Position:   position,
			OverlapIDs: overlapIDs,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]

		if left.Status != right.Status {
			return statusRank(left.Status) < statusRank(right.Status)
		}

		leftDir := directionRank(left.Direction)
		rightDir := directionRank(right.Direction)
		if leftDir != rightDir {
			return leftDir < rightDir
		}

		if left.Position[0] != right.Position[0] {
			return left.Position[0] < right.Position[0]
		}
		if left.Position[1] != right.Position[1] {
			return left.Position[1] < right.Position[1]
		}
		return left.Position[2] < right.Position[2]
	})

	limit := req.Count
	if limit > len(candidates) {
		limit = len(candidates)
	}
	if limit > 4 {
		limit = 4
	}
	candidates = append([]Candidate(nil), candidates[:limit]...)
	for i := range candidates {
		candidates[i].Rank = i + 1
	}

	result := Result{
		Status:     "WARN",
		Manifest:   req.Manifest.Scene,
		PrefabPath: req.Prefab,
		Near:       anchor,
		Align:      align,
		Count:      limit,
		Candidates: candidates,
	}
	for _, candidate := range candidates {
		if candidate.Status == "OK" {
			result.Status = "OK"
			break
		}
	}

	return result, nil
}

func resolveAnchor(manifest bounds.Manifest, near string) (bounds.ObjectBounds, error) {
	trimmed := strings.TrimSpace(near)
	if trimmed == "" {
		return bounds.ObjectBounds{}, fmt.Errorf(`missing anchor near=""`)
	}

	if id, ok := parseNonZeroDecimal(trimmed); ok {
		for _, object := range manifest.Objects {
			if object.FileID == id {
				return object, nil
			}
		}
		return bounds.ObjectBounds{}, fmt.Errorf("missing anchor near=%q", trimmed)
	}

	matches := make([]bounds.ObjectBounds, 0, 1)
	for _, object := range manifest.Objects {
		if object.Name == trimmed {
			matches = append(matches, object)
		}
	}

	switch len(matches) {
	case 0:
		return bounds.ObjectBounds{}, fmt.Errorf("missing anchor near=%q", trimmed)
	case 1:
		return matches[0], nil
	default:
		return bounds.ObjectBounds{}, fmt.Errorf("AMBIGUOUS_NAME name=%q matches=%d", trimmed, len(matches))
	}
}

func prefabBounds(manifest bounds.Manifest, prefabPath string) (bounds.AABB, error) {
	for _, prefab := range manifest.Prefabs {
		if prefab.Path == prefabPath {
			return prefab.Bounds, nil
		}
	}

	_, err := check.CheckPlacement(manifest, prefabPath, bounds.Vec3{})
	if err != nil {
		return bounds.AABB{}, err
	}
	return bounds.AABB{}, fmt.Errorf("missing prefab manifest entry for path=%q", prefabPath)
}

func candidatePosition(anchor bounds.AABB, prefab bounds.AABB, direction string, align Align) bounds.Vec3 {
	center := bounds.Vec3{
		anchor.Center[0],
		prefab.Size[1] / 2,
		anchor.Center[2],
	}

	switch direction {
	case "east":
		center[0] += anchor.Size[0]/2 + prefab.Size[0]/2
	case "west":
		center[0] -= anchor.Size[0]/2 + prefab.Size[0]/2
	case "north":
		center[2] += anchor.Size[2]/2 + prefab.Size[2]/2
	case "south":
		center[2] -= anchor.Size[2]/2 + prefab.Size[2]/2
	}

	if align == AlignGrid {
		center[0] = snapHalfMeter(center[0])
		center[2] = snapHalfMeter(center[2])
	}

	return bounds.Vec3{
		center[0] - prefab.Center[0],
		center[1] - prefab.Center[1],
		center[2] - prefab.Center[2],
	}
}

func snapHalfMeter(value float64) float64 {
	return math.Round(value*2) / 2
}

func parseNonZeroDecimal(raw string) (int64, bool) {
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, false
	}
	return value, true
}

func excludeFileID(ids []int64, fileID int64) []int64 {
	if len(ids) == 0 {
		return nil
	}

	filtered := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == fileID {
			continue
		}
		filtered = append(filtered, id)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func statusRank(status string) int {
	switch status {
	case "OK":
		return 0
	case "WARN":
		return 1
	default:
		return 2
	}
}

func directionRank(direction string) int {
	for _, spec := range directionOrder {
		if spec.name == direction {
			return spec.order
		}
	}
	return len(directionOrder)
}
