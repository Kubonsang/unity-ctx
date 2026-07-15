package bounds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidManifest(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	manifest, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if manifest.Scene != "Assets/Scenes/SimpleScene.unity" {
		t.Fatalf("Scene mismatch: got %q want %q", manifest.Scene, "Assets/Scenes/SimpleScene.unity")
	}
	if manifest.Source != "editor" {
		t.Fatalf("Source mismatch: got %q want %q", manifest.Source, "editor")
	}
	if manifest.Version != 1 {
		t.Fatalf("Version mismatch: got %d want 1", manifest.Version)
	}
	if len(manifest.Objects) != 2 {
		t.Fatalf("object count mismatch: got %d want 2", len(manifest.Objects))
	}
	if len(manifest.Prefabs) != 2 {
		t.Fatalf("prefab count mismatch: got %d want 2", len(manifest.Prefabs))
	}

	chair := manifest.Objects[1]
	if chair.FileID != 2000 {
		t.Fatalf("object fileID mismatch: got %d want 2000", chair.FileID)
	}
	if chair.Name != "Chair_01" {
		t.Fatalf("object name mismatch: got %q want %q", chair.Name, "Chair_01")
	}
	if chair.Bounds.Center != (Vec3{2.1, 0.5, -1.25}) {
		t.Fatalf("object center mismatch: got %#v want %#v", chair.Bounds.Center, Vec3{2.1, 0.5, -1.25})
	}
	if chair.Bounds.Size != (Vec3{0.8, 1.0, 0.8}) {
		t.Fatalf("object size mismatch: got %#v want %#v", chair.Bounds.Size, Vec3{0.8, 1.0, 0.8})
	}

	prefab := manifest.Prefabs[0]
	if prefab.Path != "Assets/Prefabs/chair.prefab" {
		t.Fatalf("prefab path mismatch: got %q want %q", prefab.Path, "Assets/Prefabs/chair.prefab")
	}
	if prefab.Bounds.Center != (Vec3{0, 0.5, 0}) {
		t.Fatalf("prefab center mismatch: got %#v want %#v", prefab.Bounds.Center, Vec3{0, 0.5, 0})
	}
	if prefab.Bounds.Size != (Vec3{0.8, 1.0, 0.8}) {
		t.Fatalf("prefab size mismatch: got %#v want %#v", prefab.Bounds.Size, Vec3{0.8, 1.0, 0.8})
	}
}

func TestLoadSpatialManifestV2(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json")
	manifest, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != ManifestVersion2 {
		t.Fatalf("version=%d", manifest.Version)
	}
	if len(manifest.Surfaces) != 2 || len(manifest.Prefabs) != 1 || manifest.Prefabs[0].Spatial == nil {
		t.Fatalf("spatial manifest not decoded: %#v", manifest)
	}
	foundFloor := false
	for _, surface := range manifest.Surfaces {
		if surface.ID == "floor-main" && surface.Type == "floor" {
			foundFloor = true
		}
	}
	if !foundFloor {
		t.Fatalf("floor-main surface not decoded: %#v", manifest.Surfaces)
	}
}

func TestSpatialManifestV2PreservesArbitraryNamedContactFrame(t *testing.T) {
	manifest, err := Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile := manifest.Prefabs[0].Spatial
	profile.Frames = append(profile.Frames, ContactFrame{
		ID: "handle-seat", Point: Vec3{.25, 1, 0}, Normal: Vec3{1, 0, 0},
		Tangent: Vec3{0, 0, 1}, Size: [2]float64{.2, .4},
	})
	profile.Contacts = append(profile.Contacts, ContactRequirement{
		ID: "handle", Kind: "WallMounted", FrameID: "handle-seat", Target: "surface:wall",
		MinimumGap: .005, MaximumGap: .01, MinimumSupport: .6, DirectionAlignment: .95,
	})
	path := filepath.Join(t.TempDir(), "generic-frame.json")
	if err := Save(path, manifest); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Prefabs[0].Spatial.Frames) != 1 || loaded.Prefabs[0].Spatial.Frames[0].ID != "handle-seat" {
		t.Fatalf("generic frame lost during round trip: %#v", loaded.Prefabs[0].Spatial.Frames)
	}
}

func TestLoadSpatialManifestV2AcceptsFBXGameObjectAsset(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), "Assets/Prefabs/Bookcase.prefab", "Assets/KayKit/Models/Bookcase.fbx"))

	manifest, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got := manifest.Prefabs[0].Path; got != "Assets/KayKit/Models/Bookcase.fbx" {
		t.Fatalf("prefab path mismatch: got %q", got)
	}
}

func TestManifestGameObjectAssetExtensionAllowlist(t *testing.T) {
	for _, extension := range []string{".prefab", ".fbx", ".dae", ".3ds", ".dxf", ".obj", ".skp", ".blend", ".max", ".ma", ".mb", ".FBX"} {
		path := "Assets/Models/asset" + extension
		if err := validatePrefabAssetPath(path, 0); err != nil {
			t.Errorf("validatePrefabAssetPath(%q) error = %v", path, err)
		}
	}
	for _, extension := range []string{".mat", ".png", ".asset", ""} {
		path := "Assets/Models/asset" + extension
		if err := validatePrefabAssetPath(path, 0); err == nil {
			t.Errorf("validatePrefabAssetPath(%q) error = nil, want unsupported extension", path)
		}
	}
}

func TestLoadSpatialManifestV2RejectsUnsupportedGameObjectAssetExtension(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.ReplaceAll(string(data), "Assets/Prefabs/Bookcase.prefab", "Assets/Materials/Bookcase.mat"))

	_, err = Decode(data)
	if err == nil {
		t.Fatal("Decode() error = nil, want invalid GameObject asset extension")
	}
	want := "invalid manifest: prefabs[0].path " + gameObjectAssetPathRequirement
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestSaveSpatialManifestV2RejectsDuplicateSurfaceID(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json")
	manifest, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	duplicateID := manifest.Surfaces[0].ID
	manifest.Surfaces = append(manifest.Surfaces, manifest.Surfaces[0])

	err = Save(filepath.Join(t.TempDir(), "duplicate-surface.json"), manifest)
	if err == nil {
		t.Fatal("Save() error = nil, want duplicate surface ID")
	}
	want := "invalid manifest: duplicate surfaces.id=\"" + duplicateID + "\""
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestSaveSpatialManifestV2RejectsInvalidApprovedContactPolicy(t *testing.T) {
	manifest, err := Load(filepath.Join("..", "..", "testdata", "manifests", "spatial_room_v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Prefabs[0].Spatial.Contacts[0].MinimumSupport = 1.1
	if err := Save(filepath.Join(t.TempDir(), "invalid-contact.json"), manifest); err == nil || !strings.Contains(err.Error(), "invalid target or tolerances") {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestLoadRejectsMissingObjectBounds(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "manifests", "invalid_missing_bounds.json")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected Load() to reject manifest with missing bounds")
	}

	want := "invalid manifest: missing objects[0].bounds"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadRejectsDuplicateFileID(t *testing.T) {
	path := writeManifestFile(t, `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [
    {
      "fileID": 1000,
      "name": "Table_01",
      "bounds": {
        "center": [0.0, 0.5, 0.0],
        "size": [2.0, 1.0, 1.0]
      }
    },
    {
      "fileID": 1000,
      "name": "Chair_01",
      "bounds": {
        "center": [1.0, 0.5, 0.0],
        "size": [1.0, 1.0, 1.0]
      }
    }
  ],
  "prefabs": []
}
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected Load() to reject duplicate fileID")
	}

	want := "invalid manifest: duplicate objects.fileID=1000"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadRejectsDuplicatePrefabPath(t *testing.T) {
	path := writeManifestFile(t, `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [],
  "prefabs": [
    {
      "path": "Assets/Prefabs/chair.prefab",
      "bounds": {
        "center": [0.0, 0.5, 0.0],
        "size": [0.8, 1.0, 0.8]
      }
    },
    {
      "path": "Assets/Prefabs/chair.prefab",
      "bounds": {
        "center": [0.0, 0.5, 0.0],
        "size": [0.8, 1.0, 0.8]
      }
    }
  ]
}
`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected Load() to reject duplicate prefab path")
	}

	want := "invalid manifest: duplicate prefabs.path=\"Assets/Prefabs/chair.prefab\""
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func TestLoadRejectsMalformedVectorAndInvalidSize(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "vector length",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [
    {
      "fileID": 1000,
      "name": "Table_01",
      "bounds": {
        "center": [0.0, 0.5],
        "size": [2.0, 1.0, 1.0]
      }
    }
  ],
  "prefabs": []
}
`,
			want: "invalid manifest: objects[0].bounds.center must have exactly 3 numbers",
		},
		{
			name: "size dimension",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [],
  "prefabs": [
    {
      "path": "Assets/Prefabs/chair.prefab",
      "bounds": {
        "center": [0.0, 0.5, 0.0],
        "size": [0.8, 0.0, 0.8]
      }
    }
  ]
}
`,
			want: "invalid manifest: prefabs[0].bounds.size[1] must be > 0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeManifestFile(t, tc.body)

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected Load() to reject invalid manifest")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadRejectsInvalidPathShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "scene must be unity asset path",
			body: `{
  "scene": "Packages/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [],
  "prefabs": []
}
`,
			want: "invalid manifest: scene must be an Assets path ending in .unity",
		},
		{
			name: "prefab must be prefab asset path",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [],
  "prefabs": [
    {
      "path": "Assets/Prefabs/chair.unity",
      "bounds": {
        "center": [0.0, 0.5, 0.0],
        "size": [0.8, 1.0, 0.8]
      }
    }
  ]
}
`,
			want: "invalid manifest: prefabs[0].path " + gameObjectAssetPathRequirement,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeManifestFile(t, tc.body)

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected Load() to reject invalid path shape")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestSaveRejectsInvalidPathShapes(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		want     string
	}{
		{
			name: "scene must be unity asset path",
			manifest: Manifest{
				Scene:   "Scenes/SimpleScene.unity",
				Source:  "editor",
				Version: 1,
				Objects: []ObjectBounds{},
				Prefabs: []PrefabBounds{},
			},
			want: "invalid manifest: scene must be an Assets path ending in .unity",
		},
		{
			name: "prefab must be prefab asset path",
			manifest: Manifest{
				Scene:   "Assets/Scenes/SimpleScene.unity",
				Source:  "editor",
				Version: 1,
				Objects: []ObjectBounds{},
				Prefabs: []PrefabBounds{
					{
						Path: "Packages/Prefabs/chair.prefab",
						Bounds: AABB{
							Center: Vec3{0, 0.5, 0},
							Size:   Vec3{0.8, 1, 0.8},
						},
					},
				},
			},
			want: "invalid manifest: prefabs[0].path " + gameObjectAssetPathRequirement,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "scene.bounds.json")
			err := Save(path, tc.manifest)
			if err == nil {
				t.Fatal("expected Save() to reject invalid path shape")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadRejectsMissingTopLevelCollections(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing objects",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "prefabs": []
}
`,
			want: "invalid manifest: missing objects",
		},
		{
			name: "missing prefabs",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": []
}
`,
			want: "invalid manifest: missing prefabs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeManifestFile(t, tc.body)

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected Load() to reject missing top-level collection")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadRejectsNullTopLevelCollections(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "null objects",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": null,
  "prefabs": []
}
`,
			want: "invalid manifest: objects must be an array",
		},
		{
			name: "null prefabs",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [],
  "prefabs": null
}
`,
			want: "invalid manifest: prefabs must be an array",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeManifestFile(t, tc.body)

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected Load() to reject null top-level collection")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadRejectsUnknownNestedFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "object field",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [
    {
      "fileID": 1000,
      "name": "Table_01",
      "tag": "Environment",
      "bounds": {
        "center": [0.0, 0.5, 0.0],
        "size": [2.0, 1.0, 1.0]
      }
    }
  ],
  "prefabs": []
}
`,
			want: "invalid manifest: objects[0]: unknown field \"tag\"",
		},
		{
			name: "prefab bounds field",
			body: `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "source": "editor",
  "version": 1,
  "objects": [],
  "prefabs": [
    {
      "path": "Assets/Prefabs/chair.prefab",
      "bounds": {
        "center": [0.0, 0.5, 0.0],
        "size": [0.8, 1.0, 0.8],
        "extents": [0.4, 0.5, 0.4]
      }
    }
  ]
}
`,
			want: "invalid manifest: prefabs[0].bounds: unknown field \"extents\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeManifestFile(t, tc.body)

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected Load() to reject unknown nested field")
			}

			if err.Error() != tc.want {
				t.Fatalf("error mismatch: got %q want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestManifestSaveRoundTripPreservesDeterministicOrdering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scene.bounds.json")
	manifest := Manifest{
		Scene:   "Assets/Scenes/SimpleScene.unity",
		Source:  "editor",
		Version: 1,
		Objects: []ObjectBounds{
			{
				FileID: 2000,
				Name:   "Chair_01",
				Bounds: AABB{
					Center: Vec3{2.1, 0.5, -1.25},
					Size:   Vec3{0.8, 1.0, 0.8},
				},
			},
			{
				FileID: 1000,
				Name:   "Table_01",
				Bounds: AABB{
					Center: Vec3{5.0, 0.5, 3.0},
					Size:   Vec3{2.0, 1.0, 1.0},
				},
			},
		},
		Prefabs: []PrefabBounds{
			{
				Path: "Assets/Prefabs/table.prefab",
				Bounds: AABB{
					Center: Vec3{0, 0.5, 0},
					Size:   Vec3{2, 1, 1},
				},
			},
			{
				Path: "Assets/Prefabs/chair.prefab",
				Bounds: AABB{
					Center: Vec3{0, 0.5, 0},
					Size:   Vec3{0.8, 1, 0.8},
				},
			},
		},
	}

	if err := Save(path, manifest); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.Scene != manifest.Scene {
		t.Fatalf("scene mismatch: got %q want %q", got.Scene, manifest.Scene)
	}
	if len(got.Objects) != 2 || got.Objects[0].FileID != 1000 || got.Objects[1].FileID != 2000 {
		t.Fatalf("saved objects not sorted deterministically: %#v", got.Objects)
	}
	if len(got.Prefabs) != 2 || got.Prefabs[0].Path != "Assets/Prefabs/chair.prefab" || got.Prefabs[1].Path != "Assets/Prefabs/table.prefab" {
		t.Fatalf("saved prefabs not sorted deterministically: %#v", got.Prefabs)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	text := string(data)
	if !strings.HasSuffix(text, "\n") {
		t.Fatalf("saved manifest missing trailing newline: %q", text)
	}
	if strings.Index(text, `"fileID": 1000`) > strings.Index(text, `"fileID": 2000`) {
		t.Fatalf("saved object order is not deterministic: %s", text)
	}
	if strings.Index(text, `"path": "Assets/Prefabs/chair.prefab"`) > strings.Index(text, `"path": "Assets/Prefabs/table.prefab"`) {
		t.Fatalf("saved prefab order is not deterministic: %s", text)
	}
}

func writeManifestFile(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "manifest.bounds.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return path
}
