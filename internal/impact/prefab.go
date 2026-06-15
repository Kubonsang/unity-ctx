package impact

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/parser"
)

type Request struct {
	ProjectPath string
	TargetPath  string
	SceneScope  []string
	MaxDepth    int
}

type FileHit struct {
	Path       string
	References int
	FileIDs    []int64
}

type Result struct {
	Status         string
	PrefabPath     string
	PrefabGUID     string
	SceneHits      []FileHit
	PrefabHits     []FileHit
	DepthLimitHit  bool
	MaxNestedDepth int
}

func LoadPrefabGUID(targetPath string) (string, error) {
	targetPath = filepath.Clean(targetPath)
	metaPath := targetPath + ".meta"

	file, err := os.Open(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("prefab meta not found file=%s", targetPath)
		}
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "guid:") {
			continue
		}

		guid := strings.TrimSpace(strings.TrimPrefix(line, "guid:"))
		if guid != "" {
			return guid, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("prefab guid not found file=%s", filepath.Clean(metaPath))
}

func ScanPrefabImpact(req Request) (Result, error) {
	projectRoot, err := filepath.Abs(req.ProjectPath)
	if err != nil {
		return Result{}, err
	}
	projectRoot = filepath.Clean(projectRoot)

	assetsRoot := filepath.Join(projectRoot, "Assets")
	if info, err := os.Stat(assetsRoot); err != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("project Assets root not found: %s", assetsRoot)
	}

	targetRoot, err := filepath.Abs(req.TargetPath)
	if err != nil {
		return Result{}, err
	}
	targetRoot = filepath.Clean(targetRoot)

	targetAssetPath, err := resolvePrefabAssetPath(projectRoot, targetRoot)
	if err != nil {
		return Result{}, err
	}
	sceneScope, err := normalizeSceneScope(projectRoot, req.SceneScope)
	if err != nil {
		return Result{}, err
	}

	info, err := os.Stat(targetRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{}, fmt.Errorf("prefab file not found: %s", targetRoot)
		}
		return Result{}, err
	}
	if !info.Mode().IsRegular() {
		return Result{}, fmt.Errorf("prefab file not found: %s", targetRoot)
	}

	guid, err := LoadPrefabGUID(targetRoot)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		Status:     "OK",
		PrefabPath: targetAssetPath,
		PrefabGUID: guid,
	}

	var sceneHits []FileHit
	var prefabHits []FileHit
	reversePrefabRefs := make(map[string][]string)

	err = filepath.WalkDir(assetsRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".unity" && ext != ".prefab" {
			return nil
		}

		assetPath, err := assetPathFromAbsolute(assetsRoot, path)
		if err != nil {
			return err
		}

		blocks, err := parser.ParseFile(path)
		if err != nil {
			return err
		}

		if ext == ".unity" {
			hit := collectFileHit(assetPath, blocks, guid)
			if hit == nil {
				return nil
			}
			if len(sceneScope) > 0 {
				if _, ok := sceneScope[assetPath]; !ok {
					return nil
				}
			}
			sceneHits = append(sceneHits, *hit)
			return nil
		}

		ownGUID, err := LoadPrefabGUID(path)
		if err != nil {
			return err
		}
		referencedGUIDs := collectReferencedGUIDs(blocks)
		for referencedGUID := range referencedGUIDs {
			if referencedGUID == ownGUID {
				continue
			}
			reversePrefabRefs[referencedGUID] = append(reversePrefabRefs[referencedGUID], ownGUID)
		}

		hit := collectFileHit(assetPath, blocks, guid)
		if hit == nil {
			return nil
		}
		if assetPath == targetAssetPath {
			return nil
		}
		prefabHits = append(prefabHits, *hit)
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	sortFileHits(sceneHits)
	sortFileHits(prefabHits)
	sortReversePrefabRefs(reversePrefabRefs)
	maxNestedDepth, depthLimitHit := computeNestedDepth(guid, reversePrefabRefs, req.MaxDepth)
	result.SceneHits = sceneHits
	result.PrefabHits = prefabHits
	result.MaxNestedDepth = maxNestedDepth
	result.DepthLimitHit = depthLimitHit
	if depthLimitHit {
		result.Status = "WARN"
	}

	return result, nil
}

func resolvePrefabAssetPath(projectRoot, targetPath string) (string, error) {
	assetsRoot := filepath.Join(projectRoot, "Assets")
	relative, err := filepath.Rel(assetsRoot, targetPath)
	if err != nil {
		return "", fmt.Errorf("prefab must be under project Assets/ file=%s project=%s", targetPath, projectRoot)
	}
	if relative == "." || relative == "" || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("prefab must be under project Assets/ file=%s project=%s", targetPath, projectRoot)
	}
	if filepath.Ext(relative) != ".prefab" {
		return "", fmt.Errorf("prefab must be under project Assets/ file=%s project=%s", targetPath, projectRoot)
	}

	return filepath.ToSlash(filepath.Join("Assets", relative)), nil
}

func assetPathFromAbsolute(assetsRoot, filePath string) (string, error) {
	relative, err := filepath.Rel(assetsRoot, filepath.Clean(filePath))
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join("Assets", relative)), nil
}

func collectFileHit(assetPath string, blocks []parser.Block, guid string) *FileHit {
	fileIDSet := make(map[int64]struct{})

	for _, block := range blocks {
		if !containsGUID(block.Fields, guid) {
			continue
		}
		fileIDSet[block.FileID] = struct{}{}
	}

	if len(fileIDSet) == 0 {
		return nil
	}

	fileIDs := make([]int64, 0, len(fileIDSet))
	for fileID := range fileIDSet {
		fileIDs = append(fileIDs, fileID)
	}
	sort.Slice(fileIDs, func(i, j int) bool {
		return fileIDs[i] < fileIDs[j]
	})

	return &FileHit{
		Path:       assetPath,
		References: len(fileIDs),
		FileIDs:    fileIDs,
	}
}

func containsGUID(value any, guid string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "guid" {
				if stringValue, ok := child.(string); ok && stringValue == guid {
					return true
				}
				continue
			}
			if containsGUID(child, guid) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsGUID(child, guid) {
				return true
			}
		}
	}

	return false
}

func sortFileHits(hits []FileHit) {
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Path < hits[j].Path
	})
}

func normalizeSceneScope(projectRoot string, raw []string) (map[string]struct{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	scope := make(map[string]struct{}, len(raw))
	for _, entry := range raw {
		scene := strings.TrimSpace(entry)
		if scene == "" {
			continue
		}

		normalized, err := normalizeScenePath(projectRoot, scene)
		if err != nil {
			return nil, err
		}
		scope[normalized] = struct{}{}
	}

	if len(scope) == 0 {
		return nil, nil
	}
	return scope, nil
}

func normalizeScenePath(projectRoot, scene string) (string, error) {
	if strings.HasPrefix(scene, "Assets/") {
		cleaned := filepath.ToSlash(filepath.Clean(scene))
		if !strings.HasPrefix(cleaned, "Assets/") || !strings.HasSuffix(cleaned, ".unity") {
			return "", fmt.Errorf("scene must be under project Assets/ file=%s project=%s", filepath.Clean(scene), projectRoot)
		}
		if cleaned == "Assets" || strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
			return "", fmt.Errorf("scene must be under project Assets/ file=%s project=%s", filepath.Clean(scene), projectRoot)
		}
		return cleaned, nil
	}

	sceneRoot, err := filepath.Abs(scene)
	if err != nil {
		return "", err
	}
	sceneRoot = filepath.Clean(sceneRoot)

	assetsRoot := filepath.Join(projectRoot, "Assets")
	relative, err := filepath.Rel(assetsRoot, sceneRoot)
	if err != nil {
		return "", fmt.Errorf("scene must be under project Assets/ file=%s project=%s", sceneRoot, projectRoot)
	}
	if relative == "." || relative == "" || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.Ext(relative) != ".unity" {
		return "", fmt.Errorf("scene must be under project Assets/ file=%s project=%s", sceneRoot, projectRoot)
	}
	return filepath.ToSlash(filepath.Join("Assets", relative)), nil
}

func computeNestedDepth(targetGUID string, reversePrefabRefs map[string][]string, maxDepth int) (int, bool) {
	if targetGUID == "" {
		return 0, false
	}

	maxFound := 0
	depthLimitHit := false
	stack := map[string]struct{}{targetGUID: {}}

	var visit func(currentGUID string, depth int)
	visit = func(currentGUID string, depth int) {
		for _, nextGUID := range reversePrefabRefs[currentGUID] {
			if _, ok := stack[nextGUID]; ok {
				continue
			}

			nextDepth := depth + 1
			if nextDepth > maxFound {
				maxFound = nextDepth
			}
			if maxDepth > 0 && nextDepth > maxDepth {
				depthLimitHit = true
			}

			stack[nextGUID] = struct{}{}
			visit(nextGUID, nextDepth)
			delete(stack, nextGUID)
		}
	}
	visit(targetGUID, 0)

	if maxDepth > 0 && maxFound > maxDepth {
		return maxDepth, depthLimitHit
	}
	return maxFound, depthLimitHit
}

func collectReferencedGUIDs(blocks []parser.Block) map[string]struct{} {
	referenced := make(map[string]struct{})
	for _, block := range blocks {
		collectGUIDs(block.Fields, referenced)
	}
	return referenced
}

func collectGUIDs(value any, referenced map[string]struct{}) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "guid" {
				if stringValue, ok := child.(string); ok && stringValue != "" {
					referenced[stringValue] = struct{}{}
				}
				continue
			}
			collectGUIDs(child, referenced)
		}
	case []any:
		for _, child := range typed {
			collectGUIDs(child, referenced)
		}
	}
}

func sortReversePrefabRefs(reversePrefabRefs map[string][]string) {
	for guid, referrers := range reversePrefabRefs {
		if len(referrers) == 0 {
			continue
		}
		sort.Strings(referrers)
		out := referrers[:0]
		for _, referrer := range referrers {
			if len(out) > 0 && out[len(out)-1] == referrer {
				continue
			}
			out = append(out, referrer)
		}
		reversePrefabRefs[guid] = out
	}
}
