package mutation

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"unity-ctx/internal/parser"
	"unity-ctx/internal/patch"
)

var parseSceneFn = parser.Parse

type SceneDiffResult struct {
	Status      patch.Status
	Operation   string
	Reason      string
	AppendOps   int
	ReservedIDs []int64
	OverlapIDs  []int64
}

type SceneApplyRequest struct {
	ScenePath string
	PatchPath string
	Envelope  patch.File
	Write     bool
}

type SceneApplyPlan struct {
	Status          patch.Status
	Operation       string
	Reason          string
	AppendOps       int
	ReservedIDs     []int64
	Changed         bool
	Verified        bool
	BackupPath      string
	ExpectedObjects int
	ActualObjects   int
	UpdatedData     []byte
}

func DescribeScenePatch(req SceneApplyRequest) (SceneDiffResult, error) {
	if err := validateScenePath(req.ScenePath); err != nil {
		return SceneDiffResult{}, err
	}

	return SceneDiffResult{
		Status:      req.Envelope.PatchPlan.Status,
		Operation:   "place_prefab",
		Reason:      req.Envelope.PatchPlan.Reason,
		AppendOps:   len(req.Envelope.PatchPlan.Appends),
		ReservedIDs: append([]int64(nil), req.Envelope.PatchPlan.ReservedFileIDs...),
		OverlapIDs:  append([]int64(nil), req.Envelope.PatchPlan.OverlapIDs...),
	}, nil
}

func PlanSceneApply(input []byte, req SceneApplyRequest) (SceneApplyPlan, error) {
	if err := validateScenePath(req.ScenePath); err != nil {
		return SceneApplyPlan{}, err
	}
	if err := validateApplyEnvelope(req.Envelope); err != nil {
		return SceneApplyPlan{}, err
	}
	if req.Envelope.PatchPlan.Status == patch.StatusUnknown {
		return SceneApplyPlan{}, fmt.Errorf("PATCH_STATUS_UNRESOLVED status=%s reason=%s", req.Envelope.PatchPlan.Status, req.Envelope.PatchPlan.Reason)
	}

	rendered, err := materializeAppendBlocks(req.Envelope.PatchPlan)
	if err != nil {
		return SceneApplyPlan{}, err
	}

	updated := appendSceneBytes(input, rendered)
	actualObjects, err := verifySceneApplyBytes(updated, req.Envelope.PatchPlan.ReservedFileIDs)
	if err != nil {
		return SceneApplyPlan{}, err
	}

	return SceneApplyPlan{
		Status:          req.Envelope.PatchPlan.Status,
		Operation:       "place_prefab",
		AppendOps:       len(req.Envelope.PatchPlan.Appends),
		ReservedIDs:     append([]int64(nil), req.Envelope.PatchPlan.ReservedFileIDs...),
		Changed:         len(req.Envelope.PatchPlan.Appends) > 0,
		Verified:        true,
		ExpectedObjects: len(req.Envelope.PatchPlan.ReservedFileIDs),
		ActualObjects:   actualObjects,
		UpdatedData:     updated,
	}, nil
}

func ApplyScene(req SceneApplyRequest, plan SceneApplyPlan) (SceneApplyPlan, error) {
	if !req.Write {
		return plan, nil
	}

	backupPath, err := WriteWithBackup(req.ScenePath, plan.UpdatedData)
	plan.BackupPath = backupPath
	if err != nil {
		return plan, err
	}

	written, err := os.ReadFile(req.ScenePath)
	if err != nil {
		return plan, err
	}

	actualObjects, verifyErr := verifySceneApplyBytes(written, req.Envelope.PatchPlan.ReservedFileIDs)
	plan.ExpectedObjects = len(req.Envelope.PatchPlan.ReservedFileIDs)
	plan.ActualObjects = actualObjects
	plan.Verified = verifyErr == nil
	if verifyErr != nil {
		return plan, verifyErr
	}

	return plan, nil
}

func validateScenePath(path string) error {
	kind := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	if kind == ".unity" {
		return nil
	}
	if kind == "" {
		kind = "unknown"
	}
	return fmt.Errorf("UNSUPPORTED_FILE_KIND kind=%s allowed=.unity", kind)
}

func validateApplyEnvelope(envelope patch.File) error {
	if envelope.SchemaVersion != patch.FileSchemaVersion {
		return fmt.Errorf("PATCH_VERSION_MISMATCH version=%d", envelope.SchemaVersion)
	}
	if envelope.Namespace != "scene" {
		return fmt.Errorf("PATCH_NAMESPACE_MISMATCH namespace=%s", envelope.Namespace)
	}
	if envelope.Command != "patch" {
		return fmt.Errorf("PATCH_COMMAND_MISMATCH command=%s", envelope.Command)
	}
	plan := envelope.PatchPlan
	if len(plan.ReservedFileIDs) != len(plan.Appends) {
		return fmt.Errorf("PATCH_APPEND_COUNT_MISMATCH reserved=%d appends=%d", len(plan.ReservedFileIDs), len(plan.Appends))
	}
	if len(plan.Appends) != 2 {
		return fmt.Errorf("UNSUPPORTED_APPEND_COUNT count=%d", len(plan.Appends))
	}

	wantClasses := [2]int{1, 4}
	wantTypes := [2]string{"GameObject", "Transform"}
	for i, appendOp := range plan.Appends {
		if appendOp.Op != patch.AppendOpAppend {
			return fmt.Errorf("UNSUPPORTED_APPEND_OP op=%s", appendOp.Op)
		}
		if appendOp.ClassID != wantClasses[i] {
			return fmt.Errorf("UNSUPPORTED_APPEND_CLASS class_id=%d", appendOp.ClassID)
		}
		if appendOp.TypeName != wantTypes[i] {
			return fmt.Errorf("UNSUPPORTED_APPEND type_name=%s", appendOp.TypeName)
		}
		if appendOp.FileID != plan.ReservedFileIDs[i] {
			return fmt.Errorf("PATCH_FILE_ID_MISMATCH append_index=%d file_id=%d reserved_file_id=%d", i, appendOp.FileID, plan.ReservedFileIDs[i])
		}
	}

	return nil
}

func materializeAppendBlocks(plan patch.PlacePrefabPlan) ([]byte, error) {
	gameObjectID := plan.ReservedFileIDs[0]
	transformID := plan.ReservedFileIDs[1]
	name := prefabBaseName(plan.PrefabPath)
	position := plan.Position

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("--- !u!1 &%d\n", gameObjectID))
	builder.WriteString("GameObject:\n")
	builder.WriteString("  m_Name: ")
	builder.WriteString(name)
	builder.WriteString("\n")
	builder.WriteString("  m_IsActive: 1\n")
	builder.WriteString(fmt.Sprintf("--- !u!4 &%d\n", transformID))
	builder.WriteString("Transform:\n")
	builder.WriteString(fmt.Sprintf("  m_GameObject: {fileID: %d}\n", gameObjectID))
	builder.WriteString(fmt.Sprintf("  m_LocalPosition: {x: %s, y: %s, z: %s}\n",
		formatScalar(position[0]),
		formatScalar(position[1]),
		formatScalar(position[2]),
	))
	builder.WriteString("  m_LocalRotation: {x: 0, y: 0, z: 0, w: 1}\n")
	builder.WriteString("  m_LocalScale: {x: 1, y: 1, z: 1}\n")
	return []byte(builder.String()), nil
}

func appendSceneBytes(input, rendered []byte) []byte {
	updated := cloneBytes(input)
	if len(updated) > 0 && updated[len(updated)-1] != '\n' {
		updated = append(updated, '\n')
	}
	updated = append(updated, rendered...)
	return updated
}

func verifySceneApplyBytes(data []byte, reservedIDs []int64) (int, error) {
	blocks, err := parseSceneFn(data)
	if err != nil {
		return 0, fmt.Errorf("APPLY_VERIFY_FAILED expected_objects=%d actual_objects=0", len(reservedIDs))
	}

	found := 0
	for _, target := range reservedIDs {
		for _, block := range blocks {
			if block.FileID == target {
				found++
				break
			}
		}
	}
	if found != len(reservedIDs) {
		return found, fmt.Errorf("APPLY_VERIFY_FAILED expected_objects=%d actual_objects=%d", len(reservedIDs), found)
	}

	return found, nil
}

func prefabBaseName(path string) string {
	name := strings.TrimSpace(path)
	name = filepath.Base(name)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if name == "" {
		return "Prefab"
	}
	return name
}

func formatScalar(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
