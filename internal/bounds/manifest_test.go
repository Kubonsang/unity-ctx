package bounds

import (
	"os"
	"path/filepath"
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

func writeManifestFile(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "manifest.bounds.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return path
}
