package patch_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/core"
	"github.com/Kubonsang/unity-ctx/internal/patch"
)

func TestLoadFileLoadsPersistedPatchFixtures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		path       string
		status     patch.Status
		reason     string
		prefabGUID string
	}{
		{
			name:       "UnknownNeedsPrefabGUID",
			path:       filepath.Join("..", "..", "testdata", "patches", "chair_place_unknown.patch.json"),
			status:     patch.StatusUnknown,
			reason:     patch.ReasonNeedPrefabGUID,
			prefabGUID: "",
		},
		{
			name:       "OKWithPrefabGUID",
			path:       filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"),
			status:     patch.StatusOK,
			reason:     "",
			prefabGUID: "guid-chair",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := patch.LoadFile(tc.path)
			if err != nil {
				t.Fatalf("LoadFile() error = %v", err)
			}

			if got.Namespace != "scene" {
				t.Fatalf("Namespace mismatch: got %q want %q", got.Namespace, "scene")
			}
			if got.Command != "patch" {
				t.Fatalf("Command mismatch: got %q want %q", got.Command, "patch")
			}
			if got.View != core.ViewCompact {
				t.Fatalf("View mismatch: got %q want %q", got.View, core.ViewCompact)
			}
			if got.Status != string(tc.status) {
				t.Fatalf("Status mismatch: got %q want %q", got.Status, tc.status)
			}
			if got.PatchPlan.Status != tc.status {
				t.Fatalf("PatchPlan.Status mismatch: got %q want %q", got.PatchPlan.Status, tc.status)
			}
			if got.PatchPlan.Reason != tc.reason {
				t.Fatalf("PatchPlan.Reason mismatch: got %q want %q", got.PatchPlan.Reason, tc.reason)
			}
			if got.PatchPlan.PrefabGUID != tc.prefabGUID {
				t.Fatalf("PatchPlan.PrefabGUID mismatch: got %q want %q", got.PatchPlan.PrefabGUID, tc.prefabGUID)
			}
		})
	}
}

func TestLoadFileRejectsMissingPatchPlan(t *testing.T) {
	t.Parallel()

	path := writePatchFile(t, `{
  "schema_version": 1,
  "status": "OK",
  "namespace": "scene",
  "command": "patch",
  "file": "testdata/scenes/simple_scene.unity",
  "view": "compact",
  "body": "OK"
}`)

	_, err := patch.LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want missing patch_plan error")
	}

	want := "invalid patch file: missing patch_plan"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadFileRejectsNonSceneNamespace(t *testing.T) {
	t.Parallel()

	path := writePatchFile(t, `{
  "schema_version": 1,
  "status": "OK",
  "namespace": "prefab",
  "command": "patch",
  "file": "testdata/scenes/simple_scene.unity",
  "view": "compact",
  "body": "OK",
  "patch_plan": {
    "status": "OK",
    "prefab_path": "Assets/Prefabs/chair.prefab",
    "prefab_guid": "guid-chair",
    "position": [5, 0, 0],
    "overlap_ids": [],
    "reserved_file_ids": [2002, 2003],
    "appends": [
      {"op": "append", "class_id": 1, "file_id": 2002, "type_name": "GameObject"},
      {"op": "append", "class_id": 4, "file_id": 2003, "type_name": "Transform"}
    ]
  }
}`)

	_, err := patch.LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want namespace validation error")
	}

	want := "invalid patch file: namespace must be \"scene\""
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadFileRejectsNonPatchCommand(t *testing.T) {
	t.Parallel()

	path := writePatchFile(t, `{
  "schema_version": 1,
  "status": "OK",
  "namespace": "scene",
  "command": "summarize",
  "file": "testdata/scenes/simple_scene.unity",
  "view": "compact",
  "body": "OK",
  "patch_plan": {
    "status": "OK",
    "prefab_path": "Assets/Prefabs/chair.prefab",
    "prefab_guid": "guid-chair",
    "position": [5, 0, 0],
    "overlap_ids": [],
    "reserved_file_ids": [2002, 2003],
    "appends": [
      {"op": "append", "class_id": 1, "file_id": 2002, "type_name": "GameObject"},
      {"op": "append", "class_id": 4, "file_id": 2003, "type_name": "Transform"}
    ]
  }
}`)

	_, err := patch.LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want command validation error")
	}

	want := "invalid patch file: command must be \"patch\""
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadFileRejectsUnsupportedAppendOpType(t *testing.T) {
	t.Parallel()

	path := writePatchFile(t, `{
  "schema_version": 1,
  "status": "OK",
  "namespace": "scene",
  "command": "patch",
  "file": "testdata/scenes/simple_scene.unity",
  "view": "compact",
  "body": "OK",
  "patch_plan": {
    "status": "OK",
    "prefab_path": "Assets/Prefabs/chair.prefab",
    "prefab_guid": "guid-chair",
    "position": [5, 0, 0],
    "overlap_ids": [],
    "reserved_file_ids": [2002, 2003],
    "appends": [
      {"op": "delete", "class_id": 1, "file_id": 2002, "type_name": "GameObject"},
      {"op": "append", "class_id": 4, "file_id": 2003, "type_name": "Transform"}
    ]
  }
}`)

	_, err := patch.LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want append op validation error")
	}

	want := "invalid patch file: patch_plan.appends[0].op must be \"append\""
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadFileRejectsMalformedPositionArray(t *testing.T) {
	t.Parallel()

	path := writePatchFile(t, `{
  "schema_version": 1,
  "status": "OK",
  "namespace": "scene",
  "command": "patch",
  "file": "testdata/scenes/simple_scene.unity",
  "view": "compact",
  "body": "OK",
  "patch_plan": {
    "status": "OK",
    "prefab_path": "Assets/Prefabs/chair.prefab",
    "prefab_guid": "guid-chair",
    "position": [5, 0],
    "overlap_ids": [],
    "reserved_file_ids": [2002, 2003],
    "appends": [
      {"op": "append", "class_id": 1, "file_id": 2002, "type_name": "GameObject"},
      {"op": "append", "class_id": 4, "file_id": 2003, "type_name": "Transform"}
    ]
  }
}`)

	_, err := patch.LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want position validation error")
	}

	want := "invalid patch file: patch_plan.position must have exactly 3 numbers"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadFileRejectsSchemaVersionMismatch(t *testing.T) {
	t.Parallel()

	path := writePatchFile(t, `{
  "schema_version": 2,
  "status": "OK",
  "namespace": "scene",
  "command": "patch",
  "file": "testdata/scenes/simple_scene.unity",
  "view": "compact",
  "body": "OK",
  "patch_plan": {
    "status": "OK",
    "prefab_path": "Assets/Prefabs/chair.prefab",
    "prefab_guid": "guid-chair",
    "position": [5, 0, 0],
    "overlap_ids": [],
    "reserved_file_ids": [2002, 2003],
    "appends": [
      {"op": "append", "class_id": 1, "file_id": 2002, "type_name": "GameObject"},
      {"op": "append", "class_id": 4, "file_id": 2003, "type_name": "Transform"}
    ]
  }
}`)

	_, err := patch.LoadFile(path)
	if err == nil {
		t.Fatal("LoadFile() error = nil, want schema_version validation error")
	}

	want := "invalid patch file: schema_version must be 1"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func writePatchFile(t *testing.T, contents string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "persisted.patch.json")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return path
}
