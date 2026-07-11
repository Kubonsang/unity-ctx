package scan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/bounds"
)

const editorManifestVersion = 1

const unityCLIUsings = "System,System.Linq,UnityEditor,UnityEditor.SceneManagement,UnityEngine"

type Runner interface {
	RunEditorScan(projectPath, sceneAssetPath string, prefabPaths []string) ([]byte, error)
}

type DetailedRunner interface {
	RunDetailedEditorScan(projectPath, sceneAssetPath string, prefabPaths []string) ([]byte, error)
}

type UnityCLIRunner struct{}

var unityCLIExec = func(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

type EditorPayload struct {
	Scene   string               `json:"scene"`
	Objects []EditorObjectBounds `json:"objects"`
	Prefabs []EditorPrefabBounds `json:"prefabs"`
}

type EditorObjectBounds struct {
	FileID int64      `json:"fileID"`
	Name   string     `json:"name"`
	Center [3]float64 `json:"center"`
	Size   [3]float64 `json:"size"`
}

type EditorPrefabBounds struct {
	Path   string     `json:"path"`
	Center [3]float64 `json:"center"`
	Size   [3]float64 `json:"size"`
}

type rawEditorPayload struct {
	Scene   string          `json:"scene"`
	Objects json.RawMessage `json:"objects"`
	Prefabs json.RawMessage `json:"prefabs"`
}

type rawEditorObject struct {
	FileID int64           `json:"fileID"`
	Name   string          `json:"name"`
	Center json.RawMessage `json:"center"`
	Size   json.RawMessage `json:"size"`
}

type rawEditorPrefab struct {
	Path   string          `json:"path"`
	Center json.RawMessage `json:"center"`
	Size   json.RawMessage `json:"size"`
}

func (r UnityCLIRunner) RunEditorScan(projectPath, sceneAssetPath string, prefabPaths []string) ([]byte, error) {
	return runEditorSnippet(projectPath, buildEditorScanSnippet(sceneAssetPath, prefabPaths))
}

func (r UnityCLIRunner) RunDetailedEditorScan(projectPath, sceneAssetPath string, prefabPaths []string) ([]byte, error) {
	return runEditorSnippet(projectPath, buildDetailedEditorScanSnippet(sceneAssetPath, prefabPaths))
}

func runEditorSnippet(projectPath, snippet string) ([]byte, error) {
	projectPath = filepath.Clean(projectPath)
	args := []string{
		"exec",
		snippet,
		"--project",
		projectPath,
		"--usings",
		unityCLIUsings,
	}

	output, err := unityCLIExec("unity-cli", args...)
	if err != nil {
		message := normalizeCommandOutput(output)
		if message == "" {
			return nil, fmt.Errorf("unity-cli exec failed: %w", err)
		}
		return nil, fmt.Errorf("unity-cli exec failed: %w: %s", err, message)
	}

	return output, nil
}

func ResolveSceneAssetPath(projectPath, scenePath string) (string, error) {
	projectRoot, err := filepath.Abs(projectPath)
	if err != nil {
		return "", err
	}
	projectRoot = filepath.Clean(projectRoot)

	assetsRoot := filepath.Join(projectRoot, "Assets")
	if info, err := os.Stat(assetsRoot); err != nil || !info.IsDir() {
		return "", fmt.Errorf("project Assets root not found: %s", assetsRoot)
	}

	sceneRoot, err := filepath.Abs(scenePath)
	if err != nil {
		return "", err
	}
	sceneRoot = filepath.Clean(sceneRoot)
	if info, err := os.Stat(sceneRoot); err != nil || info.IsDir() {
		return "", fmt.Errorf("scene file not found: %s", sceneRoot)
	}

	relative, err := filepath.Rel(assetsRoot, sceneRoot)
	if err != nil {
		return "", fmt.Errorf("scene must be under project Assets/ file=%s project=%s", sceneRoot, projectRoot)
	}
	if relative == "." || relative == "" || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("scene must be under project Assets/ file=%s project=%s", sceneRoot, projectRoot)
	}
	if filepath.Ext(relative) != ".unity" {
		return "", fmt.Errorf("scene must be under project Assets/ file=%s project=%s", sceneRoot, projectRoot)
	}

	return filepath.ToSlash(filepath.Join("Assets", relative)), nil
}

func NormalizePrefabList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	seen := make(map[string]struct{})
	prefabs := make([]string, 0)
	for _, entry := range strings.Split(raw, ",") {
		path := strings.TrimSpace(entry)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		prefabs = append(prefabs, path)
	}

	sort.Strings(prefabs)
	return prefabs
}

func LoadEditorPayload(path string) (EditorPayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EditorPayload{}, err
	}

	return DecodeEditorPayload(data)
}

func DecodeEditorPayload(data []byte) (EditorPayload, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var raw rawEditorPayload
	if err := decoder.Decode(&raw); err != nil {
		return EditorPayload{}, fmt.Errorf("invalid editor export: %w", err)
	}
	if err := ensureEditorEOF(decoder); err != nil {
		return EditorPayload{}, fmt.Errorf("invalid editor export: %w", err)
	}

	payload := EditorPayload{
		Scene: strings.TrimSpace(raw.Scene),
	}

	objectMessages, err := decodeEditorArray(raw.Objects, "objects")
	if err != nil {
		return EditorPayload{}, err
	}
	prefabMessages, err := decodeEditorArray(raw.Prefabs, "prefabs")
	if err != nil {
		return EditorPayload{}, err
	}

	payload.Objects = make([]EditorObjectBounds, 0, len(objectMessages))
	payload.Prefabs = make([]EditorPrefabBounds, 0, len(prefabMessages))

	for i, data := range objectMessages {
		var object rawEditorObject
		if err := decodeStrictEditorJSON(data, &object); err != nil {
			return EditorPayload{}, wrapEditorNestedDecodeError(fmt.Sprintf("objects[%d]", i), err)
		}

		center, err := decodeEditorVec3(object.Center, fmt.Sprintf("objects[%d].center", i))
		if err != nil {
			return EditorPayload{}, err
		}
		size, err := decodeEditorVec3(object.Size, fmt.Sprintf("objects[%d].size", i))
		if err != nil {
			return EditorPayload{}, err
		}

		payload.Objects = append(payload.Objects, EditorObjectBounds{
			FileID: object.FileID,
			Name:   object.Name,
			Center: center,
			Size:   size,
		})
	}

	for i, data := range prefabMessages {
		var prefab rawEditorPrefab
		if err := decodeStrictEditorJSON(data, &prefab); err != nil {
			return EditorPayload{}, wrapEditorNestedDecodeError(fmt.Sprintf("prefabs[%d]", i), err)
		}

		center, err := decodeEditorVec3(prefab.Center, fmt.Sprintf("prefabs[%d].center", i))
		if err != nil {
			return EditorPayload{}, err
		}
		size, err := decodeEditorVec3(prefab.Size, fmt.Sprintf("prefabs[%d].size", i))
		if err != nil {
			return EditorPayload{}, err
		}

		payload.Prefabs = append(payload.Prefabs, EditorPrefabBounds{
			Path:   strings.TrimSpace(prefab.Path),
			Center: center,
			Size:   size,
		})
	}

	return payload, nil
}

func normalizeCommandOutput(output []byte) string {
	if len(output) == 0 {
		return ""
	}

	fields := strings.Fields(string(output))
	return strings.TrimSpace(strings.Join(fields, " "))
}

func BuildManifestFromPayload(payload EditorPayload) (bounds.Manifest, error) {
	manifest := bounds.Manifest{
		Scene:   strings.TrimSpace(payload.Scene),
		Source:  "editor",
		Version: editorManifestVersion,
		Objects: make([]bounds.ObjectBounds, 0, len(payload.Objects)),
		Prefabs: make([]bounds.PrefabBounds, 0, len(payload.Prefabs)),
	}

	for _, object := range payload.Objects {
		manifest.Objects = append(manifest.Objects, bounds.ObjectBounds{
			FileID: object.FileID,
			Name:   object.Name,
			Bounds: bounds.AABB{
				Center: bounds.Vec3(object.Center),
				Size:   bounds.Vec3(object.Size),
			},
		})
	}

	for _, prefab := range payload.Prefabs {
		manifest.Prefabs = append(manifest.Prefabs, bounds.PrefabBounds{
			Path: strings.TrimSpace(prefab.Path),
			Bounds: bounds.AABB{
				Center: bounds.Vec3(prefab.Center),
				Size:   bounds.Vec3(prefab.Size),
			},
		})
	}

	if err := validateEditorManifest(manifest); err != nil {
		return bounds.Manifest{}, err
	}

	sort.Slice(manifest.Objects, func(i, j int) bool {
		return manifest.Objects[i].FileID < manifest.Objects[j].FileID
	})
	sort.Slice(manifest.Prefabs, func(i, j int) bool {
		return manifest.Prefabs[i].Path < manifest.Prefabs[j].Path
	})

	return manifest, nil
}

func ensureEditorEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	return fmt.Errorf("unexpected trailing JSON content")
}

func decodeEditorVec3(data json.RawMessage, path string) ([3]float64, error) {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return [3]float64{}, fmt.Errorf("invalid editor export: missing %s", path)
	}

	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return [3]float64{}, fmt.Errorf("invalid editor export: %s must be an array of numbers", path)
	}
	if len(values) != 3 {
		return [3]float64{}, fmt.Errorf("invalid editor export: %s must have exactly 3 numbers", path)
	}

	return [3]float64{values[0], values[1], values[2]}, nil
}

func decodeEditorArray(data json.RawMessage, path string) ([]json.RawMessage, error) {
	switch {
	case len(data) == 0:
		return nil, fmt.Errorf("invalid editor export: missing %s", path)
	case bytes.Equal(bytes.TrimSpace(data), []byte("null")):
		return nil, fmt.Errorf("invalid editor export: %s must be an array", path)
	}

	var values []json.RawMessage
	if err := decodeStrictEditorJSON(data, &values); err != nil {
		return nil, fmt.Errorf("invalid editor export: %s must be an array", path)
	}

	return values, nil
}

func decodeStrictEditorJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	return ensureEditorEOF(decoder)
}

func wrapEditorNestedDecodeError(path string, err error) error {
	const prefix = "json: unknown field "
	if strings.HasPrefix(err.Error(), prefix) {
		field := strings.TrimPrefix(err.Error(), prefix)
		return fmt.Errorf("invalid editor export: %s: unknown field %s", path, field)
	}

	return fmt.Errorf("invalid editor export: %s is invalid", path)
}

func validateEditorManifest(manifest bounds.Manifest) error {
	if err := validateEditorScenePath(manifest.Scene); err != nil {
		return err
	}

	seenFileIDs := make(map[int64]struct{}, len(manifest.Objects))
	for i, object := range manifest.Objects {
		if object.FileID <= 0 {
			return fmt.Errorf("invalid editor export: objects[%d].fileID must be > 0", i)
		}
		if _, ok := seenFileIDs[object.FileID]; ok {
			return fmt.Errorf("invalid editor export: duplicate objects.fileID=%d", object.FileID)
		}
		seenFileIDs[object.FileID] = struct{}{}

		if err := validateEditorSize(object.Bounds.Size, fmt.Sprintf("objects[%d].bounds.size", i)); err != nil {
			return err
		}
	}

	seenPaths := make(map[string]struct{}, len(manifest.Prefabs))
	for i, prefab := range manifest.Prefabs {
		if err := validateEditorPrefabPath(prefab.Path, i); err != nil {
			return err
		}
		if _, ok := seenPaths[prefab.Path]; ok {
			return fmt.Errorf("invalid editor export: duplicate prefabs.path=%q", prefab.Path)
		}
		seenPaths[prefab.Path] = struct{}{}

		if err := validateEditorSize(prefab.Bounds.Size, fmt.Sprintf("prefabs[%d].bounds.size", i)); err != nil {
			return err
		}
	}

	return nil
}

func validateEditorSize(size bounds.Vec3, path string) error {
	for i, value := range size {
		if value <= 0 {
			return fmt.Errorf("invalid editor export: %s[%d] must be > 0", path, i)
		}
	}

	return nil
}

func validateEditorScenePath(path string) error {
	switch {
	case path == "":
		return fmt.Errorf("invalid editor export: scene must be non-empty")
	case !strings.HasPrefix(path, "Assets/"):
		return fmt.Errorf("invalid editor export: scene must be an Assets path ending in .unity")
	case !strings.HasSuffix(path, ".unity"):
		return fmt.Errorf("invalid editor export: scene must be an Assets path ending in .unity")
	default:
		return nil
	}
}

func validateEditorPrefabPath(path string, index int) error {
	switch {
	case path == "":
		return fmt.Errorf("invalid editor export: prefabs[%d].path must be non-empty", index)
	case !strings.HasPrefix(path, "Assets/"):
		return fmt.Errorf("invalid editor export: prefabs[%d].path must be an Assets path ending in .prefab", index)
	case !strings.HasSuffix(path, ".prefab"):
		return fmt.Errorf("invalid editor export: prefabs[%d].path must be an Assets path ending in .prefab", index)
	default:
		return nil
	}
}

func buildEditorScanSnippet(sceneAssetPath string, prefabPaths []string) string {
	sceneAssetPath = filepath.ToSlash(strings.TrimSpace(sceneAssetPath))
	prefabPaths = append([]string(nil), prefabPaths...)
	sort.Strings(prefabPaths)
	return fmt.Sprintf(
		`var scenePath = %s;
var prefabPaths = new [] { %s };
var openedScene = EditorSceneManager.OpenScene(scenePath, OpenSceneMode.Single);
var sceneObjects = UnityEngine.Object.FindObjectsByType<Renderer>(FindObjectsInactive.Include, FindObjectsSortMode.None)
	.Where(renderer => renderer != null && renderer.gameObject.scene.path == openedScene.path)
	.Select(renderer => new {
		fileID = Unsupported.GetLocalIdentifierInFileForPersistentObject(renderer.gameObject),
		name = renderer.gameObject.name,
		center = new [] { (double) renderer.bounds.center.x, (double) renderer.bounds.center.y, (double) renderer.bounds.center.z },
		size = new [] { (double) renderer.bounds.size.x, (double) renderer.bounds.size.y, (double) renderer.bounds.size.z },
	})
	.Where(item => item.fileID > 0)
	.GroupBy(item => item.fileID)
	.Select(group => group.First())
	.OrderBy(item => item.fileID)
	.ToArray();
var prefabObjects = prefabPaths
	.Select(path => {
		var prefabRoot = AssetDatabase.LoadAssetAtPath<GameObject>(path);
		if (prefabRoot == null) {
			throw new Exception("prefab not found: " + path);
		}
		var renderers = prefabRoot.GetComponentsInChildren<Renderer>(true);
		if (renderers.Length == 0) {
			throw new Exception("prefab has no renderer bounds: " + path);
		}
		var prefabBounds = renderers[0].bounds;
		for (var i = 1; i < renderers.Length; i++) {
			prefabBounds.Encapsulate(renderers[i].bounds);
		}
		return new {
			path = path,
			center = new [] { (double) prefabBounds.center.x, (double) prefabBounds.center.y, (double) prefabBounds.center.z },
			size = new [] { (double) prefabBounds.size.x, (double) prefabBounds.size.y, (double) prefabBounds.size.z },
		};
	})
	.OrderBy(item => item.path)
	.ToArray();
return new {
	scene = scenePath,
	objects = sceneObjects,
	prefabs = prefabObjects,
};`,
		strconv.Quote(sceneAssetPath),
		joinQuotedCSharpStrings(prefabPaths),
	)
}

func buildDetailedEditorScanSnippet(sceneAssetPath string, prefabPaths []string) string {
	sceneAssetPath = filepath.ToSlash(strings.TrimSpace(sceneAssetPath))
	prefabPaths = append([]string(nil), prefabPaths...)
	sort.Strings(prefabPaths)
	return fmt.Sprintf(
		`var scenePath = %s;
var prefabPaths = new [] { %s };
var openedScene = EditorSceneManager.OpenScene(scenePath, OpenSceneMode.Single);
System.Func<Vector3,double[]> vec = value => new [] { (double)value.x, (double)value.y, (double)value.z };
System.Func<Quaternion,double[]> quat = value => new [] { (double)value.x, (double)value.y, (double)value.z, (double)value.w };
System.Func<Renderer,object> rendererObb = renderer => {
	var local = renderer.localBounds;
	var scale = renderer.transform.lossyScale;
	return new { id = renderer.gameObject.name, center = vec(renderer.transform.TransformPoint(local.center)), size = vec(Vector3.Scale(local.size, new Vector3(Mathf.Abs(scale.x), Mathf.Abs(scale.y), Mathf.Abs(scale.z)))), rotation = quat(renderer.transform.rotation) };
};
var sceneObjects = UnityEngine.Object.FindObjectsByType<Renderer>(FindObjectsInactive.Include, FindObjectsSortMode.None)
	.Where(renderer => renderer != null && renderer.gameObject.scene.path == openedScene.path)
	.GroupBy(renderer => Unsupported.GetLocalIdentifierInFileForPersistentObject(renderer.gameObject))
	.Where(group => group.Key > 0)
	.Select(group => {
		var renderers = group.ToArray(); var aggregate = renderers[0].bounds;
		for (var i = 1; i < renderers.Length; i++) aggregate.Encapsulate(renderers[i].bounds);
		return new { fileID = group.Key, name = renderers[0].gameObject.name, bounds = new { center = vec(aggregate.center), size = vec(aggregate.size) }, spatial = new { obbs = renderers.Select(rendererObb).ToArray(), forward = vec(Vector3.forward), up = vec(Vector3.up), pivot_offset = vec(Vector3.zero), source = "renderer-bounds", confidence = 0.6, reviewed = false } };
	}).OrderBy(item => item.fileID).ToArray();
var prefabObjects = prefabPaths.Select(path => {
	var root = AssetDatabase.LoadAssetAtPath<GameObject>(path); if (root == null) throw new Exception("prefab not found: " + path);
	var renderers = root.GetComponentsInChildren<Renderer>(true); if (renderers.Length == 0) throw new Exception("prefab has no renderer bounds: " + path);
	var aggregate = renderers[0].bounds; for (var i = 1; i < renderers.Length; i++) aggregate.Encapsulate(renderers[i].bounds);
	var bottom = new { id = "bottom", point = vec(new Vector3(aggregate.center.x, aggregate.min.y, aggregate.center.z)), normal = vec(Vector3.down), tangent = vec(Vector3.right), size = new [] { (double)aggregate.size.x, (double)aggregate.size.z } };
	var back = new { id = "back", point = vec(new Vector3(aggregate.center.x, aggregate.center.y, aggregate.min.z)), normal = vec(Vector3.back), tangent = vec(Vector3.right), size = new [] { (double)aggregate.size.x, (double)aggregate.size.y } };
	return new { path = path, guid = AssetDatabase.AssetPathToGUID(path), bounds = new { center = vec(aggregate.center), size = vec(aggregate.size) }, spatial = new { obbs = renderers.Select(rendererObb).ToArray(), forward = vec(Vector3.forward), up = vec(Vector3.up), pivot_offset = vec(aggregate.center), bottom_contact = bottom, back_contact = back, source = "renderer-bounds", confidence = 0.6, reviewed = false, dependency_hash = AssetDatabase.GetAssetDependencyHash(path).ToString() } };
}).OrderBy(item => item.path).ToArray();
var surfaces = UnityEngine.Object.FindObjectsByType<MonoBehaviour>(FindObjectsInactive.Include, FindObjectsSortMode.None)
	.Where(item => item != null && item.gameObject.scene.path == openedScene.path && item.GetType().FullName == "UnityDecoScene.DungeonDecorator.RoomSurface")
	.Select(item => { var type = item.GetType(); var origin = (Vector3)type.GetProperty("Origin").GetValue(item); var normal = (Vector3)type.GetProperty("Normal").GetValue(item); var tangent = (Vector3)type.GetProperty("Tangent").GetValue(item); var dimensions = (Vector2)type.GetProperty("Size").GetValue(item); return new { id = (string)type.GetProperty("SurfaceId").GetValue(item), type = type.GetProperty("SurfaceType").GetValue(item).ToString().ToLowerInvariant(), origin = vec(origin), normal = vec(normal), tangent = vec(tangent), size = new [] { (double)dimensions.x, (double)dimensions.y }, reviewed = (bool)type.GetProperty("Reviewed").GetValue(item), supported = (bool)type.GetProperty("Supported").GetValue(item), reason = (string)type.GetProperty("UnsupportedReason").GetValue(item) }; }).OrderBy(item => item.id).ToArray();
return new { scene = scenePath, source = "editor", version = 2, objects = sceneObjects, prefabs = prefabObjects, capabilities = new [] { "aabb", "contact-frames", "obb", "surfaces" }, surfaces = surfaces };`,
		strconv.Quote(sceneAssetPath), joinQuotedCSharpStrings(prefabPaths))
}

func joinQuotedCSharpStrings(values []string) string {
	if len(values) == 0 {
		return ""
	}

	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	return strings.Join(quoted, ", ")
}
