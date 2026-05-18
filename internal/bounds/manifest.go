package bounds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const manifestVersion = 1

type Vec3 [3]float64

type AABB struct {
	Center Vec3 `json:"center"`
	Size   Vec3 `json:"size"`
}

type ObjectBounds struct {
	FileID int64  `json:"fileID"`
	Name   string `json:"name"`
	Bounds AABB   `json:"bounds"`
}

type PrefabBounds struct {
	Path   string `json:"path"`
	Bounds AABB   `json:"bounds"`
}

type Manifest struct {
	Scene   string         `json:"scene"`
	Source  string         `json:"source"`
	Version int            `json:"version"`
	Objects []ObjectBounds `json:"objects"`
	Prefabs []PrefabBounds `json:"prefabs"`
}

type rawManifest struct {
	Scene   string          `json:"scene"`
	Source  string          `json:"source"`
	Version int             `json:"version"`
	Objects json.RawMessage `json:"objects"`
	Prefabs json.RawMessage `json:"prefabs"`
}

type rawObject struct {
	FileID int64           `json:"fileID"`
	Name   string          `json:"name"`
	Bounds json.RawMessage `json:"bounds"`
}

type rawPrefab struct {
	Path   string          `json:"path"`
	Bounds json.RawMessage `json:"bounds"`
}

type rawAABB struct {
	Center json.RawMessage `json:"center"`
	Size   json.RawMessage `json:"size"`
}

func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}

	manifest, err := decodeManifest(data)
	if err != nil {
		return Manifest{}, err
	}

	return manifest, nil
}

func Save(path string, manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return err
	}

	normalized := Manifest{
		Scene:   manifest.Scene,
		Source:  manifest.Source,
		Version: manifest.Version,
		Objects: append([]ObjectBounds(nil), manifest.Objects...),
		Prefabs: append([]PrefabBounds(nil), manifest.Prefabs...),
	}

	sort.Slice(normalized.Objects, func(i, j int) bool {
		return normalized.Objects[i].FileID < normalized.Objects[j].FileID
	})
	sort.Slice(normalized.Prefabs, func(i, j int) bool {
		return normalized.Prefabs[i].Path < normalized.Prefabs[j].Path
	})

	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func decodeManifest(data []byte) (Manifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var raw rawManifest
	if err := decoder.Decode(&raw); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
	}

	if err := ensureEOF(decoder); err != nil {
		return Manifest{}, fmt.Errorf("invalid manifest: %w", err)
	}

	if len(raw.Objects) == 0 {
		return Manifest{}, fmt.Errorf("invalid manifest: missing objects")
	}
	if len(raw.Prefabs) == 0 {
		return Manifest{}, fmt.Errorf("invalid manifest: missing prefabs")
	}

	objectMessages, err := decodeArray(raw.Objects, "objects")
	if err != nil {
		return Manifest{}, err
	}

	prefabMessages, err := decodeArray(raw.Prefabs, "prefabs")
	if err != nil {
		return Manifest{}, err
	}

	manifest := Manifest{
		Scene:   raw.Scene,
		Source:  raw.Source,
		Version: raw.Version,
		Objects: make([]ObjectBounds, 0, len(objectMessages)),
		Prefabs: make([]PrefabBounds, 0, len(prefabMessages)),
	}

	for i, data := range objectMessages {
		prefix := fmt.Sprintf("objects[%d]", i)
		var object rawObject
		if err := decodeStrictJSON(data, &object); err != nil {
			return Manifest{}, wrapNestedDecodeError(prefix, err)
		}
		if len(object.Bounds) == 0 || bytes.Equal(bytes.TrimSpace(object.Bounds), []byte("null")) {
			return Manifest{}, fmt.Errorf("invalid manifest: missing %s.bounds", prefix)
		}

		bounds, err := decodeBounds(object.Bounds, prefix+".bounds")
		if err != nil {
			return Manifest{}, err
		}

		manifest.Objects = append(manifest.Objects, ObjectBounds{
			FileID: object.FileID,
			Name:   object.Name,
			Bounds: bounds,
		})
	}

	for i, data := range prefabMessages {
		prefix := fmt.Sprintf("prefabs[%d]", i)
		var prefab rawPrefab
		if err := decodeStrictJSON(data, &prefab); err != nil {
			return Manifest{}, wrapNestedDecodeError(prefix, err)
		}
		if len(prefab.Bounds) == 0 || bytes.Equal(bytes.TrimSpace(prefab.Bounds), []byte("null")) {
			return Manifest{}, fmt.Errorf("invalid manifest: missing %s.bounds", prefix)
		}

		bounds, err := decodeBounds(prefab.Bounds, prefix+".bounds")
		if err != nil {
			return Manifest{}, err
		}

		manifest.Prefabs = append(manifest.Prefabs, PrefabBounds{
			Path:   prefab.Path,
			Bounds: bounds,
		})
	}

	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}

	return manifest, nil
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

func decodeArray(data json.RawMessage, path string) ([]json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("invalid manifest: %s must be an array", path)
	}

	var values []json.RawMessage
	if err := decodeStrictJSON(data, &values); err != nil {
		return nil, fmt.Errorf("invalid manifest: %s must be an array", path)
	}

	return values, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	return ensureEOF(decoder)
}

func wrapNestedDecodeError(path string, err error) error {
	const prefix = "json: unknown field "
	if strings.HasPrefix(err.Error(), prefix) {
		field := strings.TrimPrefix(err.Error(), prefix)
		return fmt.Errorf("invalid manifest: %s: unknown field %s", path, field)
	}

	return fmt.Errorf("invalid manifest: %s is invalid", path)
}

func decodeBounds(data json.RawMessage, path string) (AABB, error) {
	var raw rawAABB
	if err := decodeStrictJSON(data, &raw); err != nil {
		return AABB{}, wrapNestedDecodeError(path, err)
	}

	return decodeAABB(raw, path)
}

func decodeAABB(raw rawAABB, prefix string) (AABB, error) {
	center, err := decodeVec3(raw.Center, prefix+".center")
	if err != nil {
		return AABB{}, err
	}

	size, err := decodeVec3(raw.Size, prefix+".size")
	if err != nil {
		return AABB{}, err
	}

	return AABB{
		Center: center,
		Size:   size,
	}, nil
}

func decodeVec3(data json.RawMessage, path string) (Vec3, error) {
	if len(data) == 0 {
		return Vec3{}, fmt.Errorf("invalid manifest: missing %s", path)
	}

	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return Vec3{}, fmt.Errorf("invalid manifest: %s must be an array of numbers", path)
	}
	if len(values) != 3 {
		return Vec3{}, fmt.Errorf("invalid manifest: %s must have exactly 3 numbers", path)
	}

	return Vec3{values[0], values[1], values[2]}, nil
}

func validateManifest(manifest Manifest) error {
	switch {
	case manifest.Scene == "":
		return fmt.Errorf("invalid manifest: missing scene")
	case manifest.Source == "":
		return fmt.Errorf("invalid manifest: missing source")
	case manifest.Version != manifestVersion:
		return fmt.Errorf("invalid manifest: version must be %d", manifestVersion)
	}
	if err := validateSceneAssetPath(manifest.Scene); err != nil {
		return err
	}

	seenFileIDs := make(map[int64]struct{}, len(manifest.Objects))
	for i, object := range manifest.Objects {
		if object.FileID <= 0 {
			return fmt.Errorf("invalid manifest: objects[%d].fileID must be > 0", i)
		}
		if _, ok := seenFileIDs[object.FileID]; ok {
			return fmt.Errorf("invalid manifest: duplicate objects.fileID=%d", object.FileID)
		}
		seenFileIDs[object.FileID] = struct{}{}

		if err := validateSize(object.Bounds.Size, fmt.Sprintf("objects[%d].bounds.size", i)); err != nil {
			return err
		}
	}

	seenPaths := make(map[string]struct{}, len(manifest.Prefabs))
	for i, prefab := range manifest.Prefabs {
		if err := validatePrefabAssetPath(prefab.Path, i); err != nil {
			return err
		}
		if _, ok := seenPaths[prefab.Path]; ok {
			return fmt.Errorf("invalid manifest: duplicate prefabs.path=%q", prefab.Path)
		}
		seenPaths[prefab.Path] = struct{}{}

		if err := validateSize(prefab.Bounds.Size, fmt.Sprintf("prefabs[%d].bounds.size", i)); err != nil {
			return err
		}
	}

	return nil
}

func validateSize(size Vec3, path string) error {
	for i, value := range size {
		if value <= 0 {
			return fmt.Errorf("invalid manifest: %s[%d] must be > 0", path, i)
		}
	}

	return nil
}

func validateSceneAssetPath(path string) error {
	path = strings.TrimSpace(path)
	switch {
	case path == "":
		return fmt.Errorf("invalid manifest: scene must be an Assets path ending in .unity")
	case !strings.HasPrefix(path, "Assets/"):
		return fmt.Errorf("invalid manifest: scene must be an Assets path ending in .unity")
	case !strings.HasSuffix(path, ".unity"):
		return fmt.Errorf("invalid manifest: scene must be an Assets path ending in .unity")
	default:
		return nil
	}
}

func validatePrefabAssetPath(path string, index int) error {
	path = strings.TrimSpace(path)
	switch {
	case path == "":
		return fmt.Errorf("invalid manifest: prefabs[%d].path must be an Assets path ending in .prefab", index)
	case !strings.HasPrefix(path, "Assets/"):
		return fmt.Errorf("invalid manifest: prefabs[%d].path must be an Assets path ending in .prefab", index)
	case !strings.HasSuffix(path, ".prefab"):
		return fmt.Errorf("invalid manifest: prefabs[%d].path must be an Assets path ending in .prefab", index)
	default:
		return nil
	}
}
