package patch

import (
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
	"github.com/Kubonsang/unity-ctx/internal/check"
	"github.com/Kubonsang/unity-ctx/internal/parser"
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
	GUID string `json:"guid"`
}

type PlacePrefabRequest struct {
	SceneBlocks []parser.Block
	Manifest    bounds.Manifest
	PrefabPath  string
	PrefabRef   PrefabReference
	Position    bounds.Vec3
}

type AppendIntent struct {
	Op       string `json:"op"`
	ClassID  int    `json:"class_id"`
	FileID   int64  `json:"file_id"`
	TypeName string `json:"type_name"`
}

type PlacePrefabPlan struct {
	Status          Status         `json:"status"`
	Reason          string         `json:"reason,omitempty"`
	PrefabPath      string         `json:"prefab_path"`
	PrefabGUID      string         `json:"prefab_guid,omitempty"`
	Position        bounds.Vec3    `json:"position"`
	OverlapIDs      []int64        `json:"overlap_ids"`
	ReservedFileIDs []int64        `json:"reserved_file_ids"`
	Appends         []AppendIntent `json:"appends"`
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
		OverlapIDs: append([]int64{}, checkResult.OverlapIDs...),
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
