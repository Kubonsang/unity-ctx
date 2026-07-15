package scan

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
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

func TestDecodeEditorPayloadMatchesFileLoader(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scan", "editor_simple_scene.json")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	got, err := DecodeEditorPayload(data)
	if err != nil {
		t.Fatalf("DecodeEditorPayload() error = %v", err)
	}

	want, err := LoadEditorPayload(path)
	if err != nil {
		t.Fatalf("LoadEditorPayload() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload mismatch: got %#v want %#v", got, want)
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
			want: "invalid editor export: prefabs[0].path " + editorGameObjectAssetPathRequirement,
		},
		{
			name: "prefab unsupported GameObject asset extension",
			payload: EditorPayload{
				Scene:   "Assets/Scenes/SimpleScene.unity",
				Objects: []EditorObjectBounds{},
				Prefabs: []EditorPrefabBounds{
					{
						Path:   "Assets/Models/chair.mat",
						Center: [3]float64{0.0, 0.5, 0.0},
						Size:   [3]float64{0.8, 1.0, 0.8},
					},
				},
			},
			want: "invalid editor export: prefabs[0].path " + editorGameObjectAssetPathRequirement,
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

func TestBuildManifestFromEditorPayloadAcceptsFBXGameObjectAsset(t *testing.T) {
	payload := EditorPayload{
		Scene:   "Assets/Scenes/SimpleScene.unity",
		Objects: []EditorObjectBounds{},
		Prefabs: []EditorPrefabBounds{
			{
				Path:   "Assets/KayKit/Models/bookcase.fbx",
				Center: [3]float64{0, 1, 0},
				Size:   [3]float64{1, 2, 0.5},
			},
		},
	}

	manifest, err := BuildManifestFromPayload(payload)
	if err != nil {
		t.Fatalf("BuildManifestFromPayload() error = %v", err)
	}
	if got := manifest.Prefabs[0].Path; got != "Assets/KayKit/Models/bookcase.fbx" {
		t.Fatalf("prefab path mismatch: got %q", got)
	}
}

func TestEditorGameObjectAssetExtensionAllowlist(t *testing.T) {
	for _, extension := range []string{".prefab", ".fbx", ".dae", ".3ds", ".dxf", ".obj", ".skp", ".blend", ".max", ".ma", ".mb", ".FBX"} {
		path := "Assets/Models/asset" + extension
		if err := validateEditorPrefabPath(path, 0); err != nil {
			t.Errorf("validateEditorPrefabPath(%q) error = %v", path, err)
		}
	}
	for _, extension := range []string{".mat", ".png", ".asset", ""} {
		path := "Assets/Models/asset" + extension
		if err := validateEditorPrefabPath(path, 0); err == nil {
			t.Errorf("validateEditorPrefabPath(%q) error = nil, want unsupported extension", path)
		}
	}
}

func TestBuildDetailedEditorScanSnippetUsesColliderFirstWithRendererFallback(t *testing.T) {
	snippet := buildDetailedEditorScanSnippet(
		"Assets/Scenes/Room.unity",
		[]string{"Assets/Prefabs/Torch.prefab", "Assets/KayKit/Models/Bookcase.fbx"},
	)

	for _, required := range []string{
		"transform.GetComponents<Collider>()",
		"return transform.GetComponents<Renderer>()",
		"root.GetComponentsInChildren<Collider>(true)",
		"root.GetComponentsInChildren<Renderer>(true)",
		"var components = colliders.Length > 0 ? colliders : renderers;",
		"source = colliderBacked ? \"collider\" : \"renderer-bounds\"",
		"aggregateComponents(components)",
		"Assets/KayKit/Models/Bookcase.fbx",
	} {
		if !strings.Contains(snippet, required) {
			t.Fatalf("detailed snippet missing %q", required)
		}
	}
	if strings.Contains(snippet, "FindObjectsByType<Renderer>") {
		t.Fatal("detailed snippet only discovers renderer-backed scene objects")
	}
	if strings.Index(snippet, "Assets/KayKit/Models/Bookcase.fbx") > strings.Index(snippet, "Assets/Prefabs/Torch.prefab") {
		t.Fatal("detailed snippet does not sort GameObject asset paths")
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

func TestResolveSceneAssetPathMapsProjectSceneToAssetPath(t *testing.T) {
	project := filepath.Join(t.TempDir(), "MyUnityProject")
	scene := filepath.Join(project, "Assets", "Scenes", "SimpleScene.unity")
	if err := os.MkdirAll(filepath.Dir(scene), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scene, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := ResolveSceneAssetPath(project, scene)
	if err != nil {
		t.Fatalf("ResolveSceneAssetPath() error = %v", err)
	}

	if got != "Assets/Scenes/SimpleScene.unity" {
		t.Fatalf("asset path mismatch: got %q want %q", got, "Assets/Scenes/SimpleScene.unity")
	}
}

func TestResolveSceneAssetPathRejectsOutsideProject(t *testing.T) {
	project := filepath.Join(t.TempDir(), "MyUnityProject")
	scene := filepath.Join(t.TempDir(), "OutsideScene.unity")
	assetsRoot := filepath.Join(project, "Assets")
	if err := os.MkdirAll(assetsRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scene, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := ResolveSceneAssetPath(project, scene)
	if err == nil {
		t.Fatal("ResolveSceneAssetPath() error = nil, want outside-project error")
	}

	want := "scene must be under project Assets/ file=" + filepath.Clean(scene) + " project=" + filepath.Clean(project)
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestResolveSceneAssetPathRejectsMissingScene(t *testing.T) {
	project := filepath.Join(t.TempDir(), "MyUnityProject")
	assetsRoot := filepath.Join(project, "Assets")
	scene := filepath.Join(assetsRoot, "Scenes", "MissingScene.unity")
	if err := os.MkdirAll(assetsRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	_, err := ResolveSceneAssetPath(project, scene)
	if err == nil {
		t.Fatal("ResolveSceneAssetPath() error = nil, want missing scene error")
	}

	want := "scene file not found: " + filepath.Clean(scene)
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestResolveSceneAssetPathRejectsMissingAssetsRoot(t *testing.T) {
	project := filepath.Join(t.TempDir(), "MyUnityProject")
	scene := filepath.Join(project, "Assets", "Scenes", "SimpleScene.unity")
	if err := os.MkdirAll(filepath.Dir(scene), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scene, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.RemoveAll(filepath.Join(project, "Assets")); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}

	_, err := ResolveSceneAssetPath(project, scene)
	if err == nil {
		t.Fatal("ResolveSceneAssetPath() error = nil, want missing Assets root error")
	}

	want := "project Assets root not found: " + filepath.Clean(filepath.Join(project, "Assets"))
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestNormalizePrefabListTrimsDedupesAndSorts(t *testing.T) {
	raw := " Assets/Prefabs/table.prefab,Assets/Prefabs/chair.prefab,, Assets/Prefabs/table.prefab , Assets/Prefabs/bench.prefab "

	got := NormalizePrefabList(raw)
	want := []string{
		"Assets/Prefabs/bench.prefab",
		"Assets/Prefabs/chair.prefab",
		"Assets/Prefabs/table.prefab",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NormalizePrefabList() mismatch: got %#v want %#v", got, want)
	}
}

func TestUnityCLIRunnerRunEditorScanBuildsExpectedInvocation(t *testing.T) {
	restore := unityCLIExec
	defer func() {
		unityCLIExec = restore
	}()

	var gotName string
	var gotArgs []string
	wantOutput := []byte(`{"scene":"Assets/Scenes/SimpleScene.unity","objects":[],"prefabs":[]}`)
	unityCLIExec = func(name string, args ...string) ([]byte, error) {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return wantOutput, nil
	}

	runner := UnityCLIRunner{}
	project := filepath.Join(t.TempDir(), "MyUnityProject")
	scene := "Assets/Scenes/SimpleScene.unity"
	prefabs := []string{"Assets/Prefabs/table.prefab", "Assets/Prefabs/chair.prefab"}

	got, err := runner.RunEditorScan(project, scene, prefabs)
	if err != nil {
		t.Fatalf("RunEditorScan() error = %v", err)
	}
	if !bytes.Equal(got, wantOutput) {
		t.Fatalf("RunEditorScan() output mismatch: got %q want %q", string(got), string(wantOutput))
	}
	if gotName != "unity-cli" {
		t.Fatalf("command name mismatch: got %q want %q", gotName, "unity-cli")
	}
	if len(gotArgs) < 5 {
		t.Fatalf("expected unity-cli args, got %#v", gotArgs)
	}
	if gotArgs[0] != "exec" {
		t.Fatalf("first arg mismatch: got %q want %q", gotArgs[0], "exec")
	}
	if gotArgs[2] != "--project" || gotArgs[3] != filepath.Clean(project) {
		t.Fatalf("project args mismatch: got %#v", gotArgs)
	}
	if gotArgs[4] != "--usings" {
		t.Fatalf("usings flag mismatch: got %#v", gotArgs)
	}
	if !strings.Contains(gotArgs[1], `var scenePath = "Assets/Scenes/SimpleScene.unity";`) {
		t.Fatalf("snippet missing scene path: %q", gotArgs[1])
	}
	if !strings.Contains(gotArgs[1], `"Assets/Prefabs/chair.prefab"`) || !strings.Contains(gotArgs[1], `"Assets/Prefabs/table.prefab"`) {
		t.Fatalf("snippet missing prefab paths: %q", gotArgs[1])
	}
	if strings.Index(gotArgs[1], `"Assets/Prefabs/chair.prefab"`) > strings.Index(gotArgs[1], `"Assets/Prefabs/table.prefab"`) {
		t.Fatalf("prefab order in snippet is not sorted: %q", gotArgs[1])
	}
}

func TestUnityCLIRunnerRunEditorScanWrapsCommandError(t *testing.T) {
	restore := unityCLIExec
	defer func() {
		unityCLIExec = restore
	}()

	unityCLIExec = func(name string, args ...string) ([]byte, error) {
		return []byte("boom\n details \n\nnext line"), errors.New("exit status 1")
	}

	runner := UnityCLIRunner{}
	_, err := runner.RunEditorScan("/tmp/MyUnityProject", "Assets/Scenes/SimpleScene.unity", nil)
	if err == nil {
		t.Fatal("RunEditorScan() error = nil, want wrapped command error")
	}

	want := "unity-cli exec failed: exit status 1: boom details next line"
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
