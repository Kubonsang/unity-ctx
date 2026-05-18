package scan

import (
	"os"
	"path/filepath"
	"testing"

	"unity-ctx/internal/bounds"
)

func TestBuildManifestFromEditorPayloadSortsAndNormalizes(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scan", "editor_simple_scene.json")

	payload, err := LoadEditorPayload(path)
	if err != nil {
		t.Fatalf("LoadEditorPayload() error = %v", err)
	}

	got, err := BuildManifestFromPayload(payload)
	if err != nil {
		t.Fatalf("BuildManifestFromPayload() error = %v", err)
	}

	if got.Scene != "Assets/Scenes/SimpleScene.unity" {
		t.Fatalf("Scene mismatch: got %q want %q", got.Scene, "Assets/Scenes/SimpleScene.unity")
	}
	if got.Source != "editor" {
		t.Fatalf("Source mismatch: got %q want %q", got.Source, "editor")
	}
	if got.Version != 1 {
		t.Fatalf("Version mismatch: got %d want %d", got.Version, 1)
	}
	if len(got.Objects) != 2 {
		t.Fatalf("object count mismatch: got %d want %d", len(got.Objects), 2)
	}
	if len(got.Prefabs) != 2 {
		t.Fatalf("prefab count mismatch: got %d want %d", len(got.Prefabs), 2)
	}
	if got.Objects[0].FileID != 1000 {
		t.Fatalf("object ordering mismatch: got %d want %d", got.Objects[0].FileID, 1000)
	}
	if got.Objects[1].FileID != 2000 {
		t.Fatalf("object ordering mismatch: got %d want %d", got.Objects[1].FileID, 2000)
	}
	if got.Prefabs[0].Path != "Assets/Prefabs/chair.prefab" {
		t.Fatalf("prefab ordering mismatch: got %q want %q", got.Prefabs[0].Path, "Assets/Prefabs/chair.prefab")
	}
	if got.Prefabs[1].Path != "Assets/Prefabs/table.prefab" {
		t.Fatalf("prefab ordering mismatch: got %q want %q", got.Prefabs[1].Path, "Assets/Prefabs/table.prefab")
	}

	wantObject := bounds.ObjectBounds{
		FileID: 1000,
		Name:   "Table_01",
		Bounds: bounds.AABB{
			Center: bounds.Vec3{5.0, 0.5, 3.0},
			Size:   bounds.Vec3{2.0, 1.0, 1.0},
		},
	}
	if got.Objects[0] != wantObject {
		t.Fatalf("object mismatch: got %#v want %#v", got.Objects[0], wantObject)
	}

	wantPrefab := bounds.PrefabBounds{
		Path: "Assets/Prefabs/chair.prefab",
		Bounds: bounds.AABB{
			Center: bounds.Vec3{0.0, 0.5, 0.0},
			Size:   bounds.Vec3{0.8, 1.0, 0.8},
		},
	}
	if got.Prefabs[0] != wantPrefab {
		t.Fatalf("prefab mismatch: got %#v want %#v", got.Prefabs[0], wantPrefab)
	}
}

func TestBuildManifestFromEditorPayloadRejectsDuplicatePrefabPath(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scan", "editor_invalid_duplicate_prefab.json")

	payload, err := LoadEditorPayload(path)
	if err != nil {
		t.Fatalf("LoadEditorPayload() error = %v", err)
	}

	_, err = BuildManifestFromPayload(payload)
	if err == nil {
		t.Fatal("BuildManifestFromPayload() error = nil, want duplicate prefab error")
	}

	want := "invalid editor export: duplicate prefabs.path=\"Assets/Prefabs/chair.prefab\""
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestBuildManifestFromEditorPayloadRejectsDuplicateObjectFileID(t *testing.T) {
	path := writeEditorPayloadFile(t, `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [
    {
      "fileID": 2000,
      "name": "Chair_01",
      "center": [2.1, 0.5, -1.25],
      "size": [0.8, 1.0, 0.8]
    },
    {
      "fileID": 2000,
      "name": "Table_01",
      "center": [5.0, 0.5, 3.0],
      "size": [2.0, 1.0, 1.0]
    }
  ],
  "prefabs": []
}`)

	payload, err := LoadEditorPayload(path)
	if err != nil {
		t.Fatalf("LoadEditorPayload() error = %v", err)
	}

	_, err = BuildManifestFromPayload(payload)
	if err == nil {
		t.Fatal("BuildManifestFromPayload() error = nil, want duplicate fileID error")
	}

	want := "invalid editor export: duplicate objects.fileID=2000"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadEditorPayloadRejectsMalformedVectors(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "wrong length",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [
    {
      "fileID": 1000,
      "name": "Table_01",
      "center": [5.0, 0.5],
      "size": [2.0, 1.0, 1.0]
    }
  ],
  "prefabs": []
}`,
			want: "invalid editor export: objects[0].center must have exactly 3 numbers",
		},
		{
			name: "non numeric",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [
    {
      "fileID": 1000,
      "name": "Table_01",
      "center": "oops",
      "size": [2.0, 1.0, 1.0]
    }
  ],
  "prefabs": []
}`,
			want: "invalid editor export: objects[0].center must be an array of numbers",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeEditorPayloadFile(t, tc.body)

			_, err := LoadEditorPayload(path)
			if err == nil {
				t.Fatal("LoadEditorPayload() error = nil, want malformed vector error")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadEditorPayloadRejectsParserContractViolations(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown field",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [],
  "prefabs": [],
  "extra": true
}`,
			want: "invalid editor export: json: unknown field \"extra\"",
		},
		{
			name: "trailing content",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [],
  "prefabs": []
} []`,
			want: "invalid editor export: unexpected trailing JSON content",
		},
		{
			name: "missing objects",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "prefabs": []
}`,
			want: "invalid editor export: missing objects",
		},
		{
			name: "null objects",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": null,
  "prefabs": []
}`,
			want: "invalid editor export: objects must be an array",
		},
		{
			name: "missing prefabs",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": []
}`,
			want: "invalid editor export: missing prefabs",
		},
		{
			name: "null prefabs",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [],
  "prefabs": null
}`,
			want: "invalid editor export: prefabs must be an array",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeEditorPayloadFile(t, tc.body)

			_, err := LoadEditorPayload(path)
			if err == nil {
				t.Fatal("LoadEditorPayload() error = nil, want parser contract error")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestBuildManifestFromEditorPayloadRejectsNonPositiveSize(t *testing.T) {
	payload := EditorPayload{
		Scene: "Assets/Scenes/SimpleScene.unity",
		Objects: []EditorObjectBounds{
			{
				FileID: 1000,
				Name:   "Table_01",
				Center: [3]float64{5.0, 0.5, 3.0},
				Size:   [3]float64{2.0, 0.0, 1.0},
			},
		},
		Prefabs: []EditorPrefabBounds{},
	}

	_, err := BuildManifestFromPayload(payload)
	if err == nil {
		t.Fatal("BuildManifestFromPayload() error = nil, want invalid size error")
	}

	want := "invalid editor export: objects[0].bounds.size[1] must be > 0"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestBuildManifestFromEditorPayloadRejectsInvalidTrimmedPaths(t *testing.T) {
	tests := []struct {
		name    string
		payload EditorPayload
		want    string
	}{
		{
			name: "scene empty after trim",
			payload: EditorPayload{
				Scene:   "   ",
				Objects: []EditorObjectBounds{},
				Prefabs: []EditorPrefabBounds{},
			},
			want: "invalid editor export: scene must be non-empty",
		},
		{
			name: "scene wrong suffix after trim",
			payload: EditorPayload{
				Scene:   "  Assets/Scenes/SimpleScene.prefab  ",
				Objects: []EditorObjectBounds{},
				Prefabs: []EditorPrefabBounds{},
			},
			want: "invalid editor export: scene must be an Assets path ending in .unity",
		},
		{
			name: "prefab wrong shape after trim",
			payload: EditorPayload{
				Scene:   "Assets/Scenes/SimpleScene.unity",
				Objects: []EditorObjectBounds{},
				Prefabs: []EditorPrefabBounds{
					{
						Path:   "  Packages/chair.prefab  ",
						Center: [3]float64{0.0, 0.5, 0.0},
						Size:   [3]float64{0.8, 1.0, 0.8},
					},
				},
			},
			want: "invalid editor export: prefabs[0].path must be an Assets path ending in .prefab",
		},
		{
			name: "prefab empty after trim",
			payload: EditorPayload{
				Scene:   "Assets/Scenes/SimpleScene.unity",
				Objects: []EditorObjectBounds{},
				Prefabs: []EditorPrefabBounds{
					{
						Path:   "   ",
						Center: [3]float64{0.0, 0.5, 0.0},
						Size:   [3]float64{0.8, 1.0, 0.8},
					},
				},
			},
			want: "invalid editor export: prefabs[0].path must be non-empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := BuildManifestFromPayload(tc.payload)
			if err == nil {
				t.Fatal("BuildManifestFromPayload() error = nil, want invalid path error")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestBuildManifestFromEditorPayloadValidatesBeforeSorting(t *testing.T) {
	payload := EditorPayload{
		Scene: "Assets/Scenes/SimpleScene.unity",
		Objects: []EditorObjectBounds{
			{
				FileID: 2000,
				Name:   "Chair_01",
				Center: [3]float64{2.1, 0.5, -1.25},
				Size:   [3]float64{0.8, 1.0, 0.8},
			},
			{
				FileID: 1000,
				Name:   "Table_01",
				Center: [3]float64{5.0, 0.5, 3.0},
				Size:   [3]float64{2.0, 0.0, 1.0},
			},
		},
		Prefabs: []EditorPrefabBounds{},
	}

	_, err := BuildManifestFromPayload(payload)
	if err == nil {
		t.Fatal("BuildManifestFromPayload() error = nil, want invalid size error")
	}

	want := "invalid editor export: objects[1].bounds.size[1] must be > 0"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func writeEditorPayloadFile(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "editor-payload.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return path
}
