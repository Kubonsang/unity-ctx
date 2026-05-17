package patch

import (
	"strings"

	"unity-ctx/internal/bounds"
	"unity-ctx/internal/check"
	"unity-ctx/internal/parser"
)

type Status string

const (
	StatusOK      Status = "OK"
	StatusWarn    Status = "WARN"
	StatusUnknown Status = "UNKNOWN"
)

const (
	ReasonNeedPrefabGUID = "NEED_PREFAB_GUID"
	AppendOpAppend       = "append"
)

type PrefabReference struct {
	GUID string
}

type PlacePrefabRequest struct {
	SceneBlocks []parser.Block
	Manifest    bounds.Manifest
	PrefabPath  string
	PrefabRef   PrefabReference
	Position    bounds.Vec3
}

type AppendIntent struct {
	Op       string
	ClassID  int
	FileID   int64
	TypeName string
}

type PlacePrefabPlan struct {
	Status          Status
	Reason          string
	PrefabPath      string
	PrefabGUID      string
	Position        bounds.Vec3
	OverlapIDs      []int64
	ReservedFileIDs []int64
	Appends         []AppendIntent
}

func PlanPlacePrefab(req PlacePrefabRequest) (PlacePrefabPlan, error) {
	checkResult, err := check.CheckPlacement(req.Manifest, req.PrefabPath, req.Position)
	if err != nil {
		return PlacePrefabPlan{}, err
	}

	gameObjectID, transformID := nextReservedIDs(req.SceneBlocks)
	plan := PlacePrefabPlan{
		PrefabPath: req.PrefabPath,
		PrefabGUID: strings.TrimSpace(req.PrefabRef.GUID),
		Position:   req.Position,
		OverlapIDs: append([]int64(nil), checkResult.OverlapIDs...),
		ReservedFileIDs: []int64{
			gameObjectID,
			transformID,
		},
		Appends: []AppendIntent{
			{Op: AppendOpAppend, ClassID: 1, FileID: gameObjectID, TypeName: "GameObject"},
			{Op: AppendOpAppend, ClassID: 4, FileID: transformID, TypeName: "Transform"},
		},
	}

	if plan.PrefabGUID == "" {
		plan.Status = StatusUnknown
		plan.Reason = ReasonNeedPrefabGUID
		return plan, nil
	}

	if checkResult.Clear {
		plan.Status = StatusOK
		return plan, nil
	}

	plan.Status = StatusWarn
	return plan, nil
}

func nextReservedIDs(blocks []parser.Block) (int64, int64) {
	var maxFileID int64
	for _, block := range blocks {
		if block.FileID > maxFileID {
			maxFileID = block.FileID
		}
	}

	return maxFileID + 1, maxFileID + 2
}
