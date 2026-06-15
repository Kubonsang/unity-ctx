// Package deps resolves the external asset dependencies of a Unity file by
// mapping the GUIDs it references to asset paths within a project. It builds a
// guid→path index from the project's .meta files; the caller supplies the
// referenced GUIDs (extracted via the safety kernel) so this package stays free
// of any kernel dependency.
package deps

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Resolution is one referenced GUID and the asset path it maps to within the
// project, if any.
type Resolution struct {
	GUID     string
	Path     string // project-relative asset path, "" when unresolved
	Resolved bool
}

// BuildIndex walks the project root for *.meta files and returns a map from
// each asset's GUID to its project-relative asset path (the .meta path minus
// the ".meta" suffix).
func BuildIndex(projectRoot string) (map[string]string, error) {
	root, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}
	index := make(map[string]string)
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".meta") {
			return nil
		}
		guid, err := readMetaGUID(path)
		if err != nil || guid == "" {
			return nil //nolint:nilerr // a malformed .meta just doesn't contribute
		}
		assetAbs := strings.TrimSuffix(path, ".meta")
		rel, relErr := filepath.Rel(root, assetAbs)
		if relErr != nil {
			rel = assetAbs
		}
		index[guid] = filepath.ToSlash(rel)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return index, nil
}

// Resolve maps each referenced GUID to its asset path using the index. The
// result is deduplicated and sorted by GUID for deterministic output.
func Resolve(index map[string]string, guids []string) []Resolution {
	seen := make(map[string]bool, len(guids))
	unique := make([]string, 0, len(guids))
	for _, g := range guids {
		if g == "" || seen[g] {
			continue
		}
		seen[g] = true
		unique = append(unique, g)
	}
	sort.Strings(unique)

	resolutions := make([]Resolution, 0, len(unique))
	for _, g := range unique {
		path, ok := index[g]
		resolutions = append(resolutions, Resolution{GUID: g, Path: path, Resolved: ok})
	}
	return resolutions
}

func readMetaGUID(metaPath string) (string, error) {
	file, err := os.Open(metaPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "guid:") {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(line, "guid:")), nil
	}
	return "", scanner.Err()
}
