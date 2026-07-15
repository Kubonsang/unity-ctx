package patch

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
	"github.com/Kubonsang/unity-ctx/internal/core"
)

const FileSchemaVersion = 1

type File struct {
	SchemaVersion int `json:"schema_version"`
	core.Result
	PatchPlan PlacePrefabPlan `json:"patch_plan"` // v1 (place_prefab)
	Ops       []Op            `json:"ops,omitempty"`
}

type rawFileV2 struct {
	SchemaVersion int       `json:"schema_version"`
	Status        string    `json:"status"`
	Namespace     string    `json:"namespace"`
	Command       string    `json:"command"`
	File          string    `json:"file"`
	View          core.View `json:"view"`
	Body          string    `json:"body"`
	Ops           []Op      `json:"ops"`
}

type rawFile struct {
	SchemaVersion int             `json:"schema_version"`
	Status        string          `json:"status"`
	Namespace     string          `json:"namespace"`
	Command       string          `json:"command"`
	File          string          `json:"file"`
	View          core.View       `json:"view"`
	Body          string          `json:"body"`
	PatchPlan     json.RawMessage `json:"patch_plan"`
}

type rawPlacePrefabPlan struct {
	Status          Status            `json:"status"`
	Reason          string            `json:"reason,omitempty"`
	PrefabPath      string            `json:"prefab_path"`
	PrefabGUID      string            `json:"prefab_guid,omitempty"`
	Position        json.RawMessage   `json:"position"`
	OverlapIDs      []int64           `json:"overlap_ids"`
	ReservedFileIDs []int64           `json:"reserved_file_ids"`
	Appends         []rawAppendIntent `json:"appends"`
}

type rawAppendIntent struct {
	Op       string `json:"op"`
	ClassID  int    `json:"class_id"`
	FileID   int64  `json:"file_id"`
	TypeName string `json:"type_name"`
}

func LoadFile(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, err
	}

	return decodeFile(data)
}

func decodeFile(data []byte) (File, error) {
	// Peek the schema version leniently so v1 and v2 each get a strict,
	// version-specific decode (DisallowUnknownFields would otherwise reject the
	// other version's fields).
	var probe struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return File{}, fmt.Errorf("invalid patch file: %w", err)
	}
	if probe.SchemaVersion == FileSchemaVersionV2 {
		return decodeFileV2(data)
	}

	var raw rawFile
	if err := decodeStrictJSON(data, &raw); err != nil {
		return File{}, fmt.Errorf("invalid patch file: %w", err)
	}
	if len(bytes.TrimSpace(raw.PatchPlan)) == 0 || bytes.Equal(bytes.TrimSpace(raw.PatchPlan), []byte("null")) {
		return File{}, fmt.Errorf("invalid patch file: missing patch_plan")
	}
	if raw.SchemaVersion != FileSchemaVersion {
		return File{}, fmt.Errorf("invalid patch file: schema_version must be %d", FileSchemaVersion)
	}
	if raw.Namespace != "scene" {
		return File{}, fmt.Errorf("invalid patch file: namespace must be %q", "scene")
	}
	if raw.Command != "patch" {
		return File{}, fmt.Errorf("invalid patch file: command must be %q", "patch")
	}
	if raw.View != core.ViewCompact {
		return File{}, fmt.Errorf("invalid patch file: view must be %q", core.ViewCompact)
	}

	plan, err := decodePlacePrefabPlan(raw.PatchPlan)
	if err != nil {
		return File{}, err
	}
	if raw.Status != string(plan.Status) {
		return File{}, fmt.Errorf("invalid patch file: status must match patch_plan.status")
	}

	return File{
		SchemaVersion: raw.SchemaVersion,
		Result: core.Result{
			Status:    raw.Status,
			Namespace: raw.Namespace,
			Command:   raw.Command,
			File:      raw.File,
			View:      raw.View,
			Body:      raw.Body,
		},
		PatchPlan: plan,
	}, nil
}

func decodeFileV2(data []byte) (File, error) {
	var raw rawFileV2
	if err := decodeStrictJSON(data, &raw); err != nil {
		return File{}, fmt.Errorf("invalid patch file: %w", err)
	}
	if raw.Namespace != "scene" {
		return File{}, fmt.Errorf("invalid patch file: namespace must be %q", "scene")
	}
	if raw.Command != "patch" {
		return File{}, fmt.Errorf("invalid patch file: command must be %q", "patch")
	}
	if raw.View != core.ViewCompact {
		return File{}, fmt.Errorf("invalid patch file: view must be %q", core.ViewCompact)
	}
	if err := validateOps(raw.Ops); err != nil {
		return File{}, err
	}

	return File{
		SchemaVersion: raw.SchemaVersion,
		Result: core.Result{
			Status:    raw.Status,
			Namespace: raw.Namespace,
			Command:   raw.Command,
			File:      raw.File,
			View:      raw.View,
			Body:      raw.Body,
		},
		Ops: append([]Op(nil), raw.Ops...),
	}, nil
}

// validateOps enforces the v2 contract: exactly one op (no op mixing) — either a
// reparent (positive target, distinct non-self endpoints) or a delete (positive
// target; the parent fields are unused and must be 0).
func validateOps(ops []Op) error {
	if len(ops) != 1 {
		return fmt.Errorf("invalid patch file: ops must contain exactly 1 operation (one op per patch)")
	}
	op := ops[0]
	if op.Target <= 0 {
		return fmt.Errorf("invalid patch file: ops[0].target must be > 0")
	}
	switch op.Op {
	case OpReparent:
		if op.NewParent < 0 || op.OldParent < 0 {
			return fmt.Errorf("invalid patch file: ops[0] parent file IDs must be >= 0")
		}
		if op.NewParent == op.Target {
			return fmt.Errorf("invalid patch file: ops[0].new_parent must differ from target")
		}
	case OpDelete:
		if op.NewParent != 0 || op.OldParent != 0 {
			return fmt.Errorf("invalid patch file: ops[0] delete must not set new_parent/old_parent")
		}
	default:
		return fmt.Errorf("invalid patch file: ops[0].op must be %q or %q", OpReparent, OpDelete)
	}
	return nil
}

func decodePlacePrefabPlan(data []byte) (PlacePrefabPlan, error) {
	var raw rawPlacePrefabPlan
	if err := decodeStrictJSON(data, &raw); err != nil {
		return PlacePrefabPlan{}, fmt.Errorf("invalid patch file: patch_plan is invalid")
	}

	position, err := decodePosition(raw.Position)
	if err != nil {
		return PlacePrefabPlan{}, err
	}

	plan := PlacePrefabPlan{
		Status:          raw.Status,
		Reason:          raw.Reason,
		PrefabPath:      raw.PrefabPath,
		PrefabGUID:      raw.PrefabGUID,
		Position:        position,
		OverlapIDs:      append([]int64(nil), raw.OverlapIDs...),
		ReservedFileIDs: append([]int64(nil), raw.ReservedFileIDs...),
		Appends:         make([]AppendIntent, 0, len(raw.Appends)),
	}

	if err := validatePlan(plan, raw.Appends); err != nil {
		return PlacePrefabPlan{}, err
	}

	for _, rawAppend := range raw.Appends {
		plan.Appends = append(plan.Appends, AppendIntent(rawAppend))
	}

	return plan, nil
}

func validatePlan(plan PlacePrefabPlan, rawAppends []rawAppendIntent) error {
	switch plan.Status {
	case StatusOK, StatusWarn, StatusUnknown:
	default:
		return fmt.Errorf("invalid patch file: patch_plan.status must be one of %q, %q, or %q", StatusOK, StatusWarn, StatusUnknown)
	}

	if strings.TrimSpace(plan.PrefabPath) == "" {
		return fmt.Errorf("invalid patch file: patch_plan.prefab_path must be non-empty")
	}
	if len(plan.ReservedFileIDs) != 2 {
		return fmt.Errorf("invalid patch file: patch_plan.reserved_file_ids must contain exactly 2 IDs")
	}
	for i, fileID := range plan.ReservedFileIDs {
		if fileID <= 0 {
			return fmt.Errorf("invalid patch file: patch_plan.reserved_file_ids[%d] must be > 0", i)
		}
	}
	for i, fileID := range plan.OverlapIDs {
		if fileID <= 0 {
			return fmt.Errorf("invalid patch file: patch_plan.overlap_ids[%d] must be > 0", i)
		}
	}

	switch plan.Status {
	case StatusOK:
		if plan.Reason != "" {
			return fmt.Errorf("invalid patch file: patch_plan.reason must be empty when status=%q", StatusOK)
		}
		if strings.TrimSpace(plan.PrefabGUID) == "" {
			return fmt.Errorf("invalid patch file: patch_plan.prefab_guid must be non-empty when status=%q", StatusOK)
		}
		if len(plan.OverlapIDs) != 0 {
			return fmt.Errorf("invalid patch file: patch_plan.overlap_ids must be empty when status=%q", StatusOK)
		}
	case StatusWarn:
		if plan.Reason != "" {
			return fmt.Errorf("invalid patch file: patch_plan.reason must be empty when status=%q", StatusWarn)
		}
		if strings.TrimSpace(plan.PrefabGUID) == "" {
			return fmt.Errorf("invalid patch file: patch_plan.prefab_guid must be non-empty when status=%q", StatusWarn)
		}
		if len(plan.OverlapIDs) == 0 {
			return fmt.Errorf("invalid patch file: patch_plan.overlap_ids must be non-empty when status=%q", StatusWarn)
		}
	case StatusUnknown:
		if plan.Reason != ReasonNeedPrefabGUID {
			return fmt.Errorf("invalid patch file: patch_plan.reason must be %q when status=%q", ReasonNeedPrefabGUID, StatusUnknown)
		}
		if strings.TrimSpace(plan.PrefabGUID) != "" {
			return fmt.Errorf("invalid patch file: patch_plan.prefab_guid must be empty when status=%q", StatusUnknown)
		}
	}

	if len(rawAppends) != 2 {
		return fmt.Errorf("invalid patch file: patch_plan.appends must contain exactly 2 operations")
	}

	wantClasses := [2]int{1, 4}
	wantTypes := [2]string{"GameObject", "Transform"}
	for i, rawAppend := range rawAppends {
		if rawAppend.Op != AppendOpAppend {
			return fmt.Errorf("invalid patch file: patch_plan.appends[%d].op must be %q", i, AppendOpAppend)
		}
		if rawAppend.ClassID != wantClasses[i] {
			return fmt.Errorf("invalid patch file: patch_plan.appends[%d].class_id must be %d", i, wantClasses[i])
		}
		if rawAppend.TypeName != wantTypes[i] {
			return fmt.Errorf("invalid patch file: patch_plan.appends[%d].type_name must be %q", i, wantTypes[i])
		}
		if rawAppend.FileID != plan.ReservedFileIDs[i] {
			return fmt.Errorf("invalid patch file: patch_plan.appends[%d].file_id must match patch_plan.reserved_file_ids[%d]", i, i)
		}
	}

	return nil
}

func decodePosition(data []byte) (bounds.Vec3, error) {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return bounds.Vec3{}, fmt.Errorf("invalid patch file: missing patch_plan.position")
	}

	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return bounds.Vec3{}, fmt.Errorf("invalid patch file: patch_plan.position must be an array of numbers")
	}
	if len(values) != 3 {
		return bounds.Vec3{}, fmt.Errorf("invalid patch file: patch_plan.position must have exactly 3 numbers")
	}

	return bounds.Vec3{values[0], values[1], values[2]}, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	return ensureEOF(decoder)
}

func ensureEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return fmt.Errorf("unexpected trailing JSON content")
}
