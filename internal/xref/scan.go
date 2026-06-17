// Package xref scans a Unity project for inbound PPtr references to a set of
// fileIDs inside a target asset. It is per-mutation (no caching — a cached index
// could go stale and mask a dangling reference) and conservative: a file whose
// references cannot be fully parsed is reported as "indeterminate" so a consumer
// can treat uncertainty as a block reason.
//
// The API takes a fileID SET (a single reparent target is the size-1 case) so the
// same scanner backs the delete/--cascade subtree case later (S5). reparent uses
// only the inbound list + indeterminate list as visibility; delete will use them
// as block reasons.
package xref

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/impact"
	"github.com/Kubonsang/unity-ctx/internal/safety"
)

type Request struct {
	ProjectPath string
	TargetPath  string  // the asset being edited (excluded from its own scan)
	FileIDs     []int64 // target local fileIDs to find inbound references to
}

// InboundHit is one other file that references the target asset's fileIDs.
type InboundHit struct {
	Path    string
	FileIDs []int64
	Count   int
}

type Result struct {
	TargetGUID string
	// Inbound: files (other than the target) holding a PPtr {fileID in set, guid:
	// target} — a cross-file reference into the edited object set.
	Inbound []InboundHit
	// Indeterminate: files whose references could not be fully parsed (ExtractRefs
	// warning/parse failure). Conservative signal: their inbound refs are unknown.
	Indeterminate []string
}

// ScanInbound walks the project's Assets/ and enumerates inbound PPtr references
// to (target GUID, fileID set), excluding the target file itself (in-file links
// are the graph-check's job). It never writes.
func ScanInbound(req Request) (Result, error) {
	projectRoot, err := filepath.Abs(req.ProjectPath)
	if err != nil {
		return Result{}, err
	}
	projectRoot = filepath.Clean(projectRoot)

	assetsRoot := filepath.Join(projectRoot, "Assets")
	if info, statErr := os.Stat(assetsRoot); statErr != nil || !info.IsDir() {
		return Result{}, fmt.Errorf("project Assets root not found: %s", assetsRoot)
	}

	targetAbs, err := filepath.Abs(req.TargetPath)
	if err != nil {
		return Result{}, err
	}
	targetAbs = filepath.Clean(targetAbs)

	guid, err := impact.LoadPrefabGUID(targetAbs) // reads <path>.meta guid (generic)
	if err != nil {
		return Result{}, err
	}

	fileIDSet := make(map[int64]struct{}, len(req.FileIDs))
	for _, id := range req.FileIDs {
		fileIDSet[id] = struct{}{}
	}

	result := Result{TargetGUID: guid}
	indeterminate := map[string]struct{}{}

	walkErr := filepath.WalkDir(assetsRoot, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".unity" && ext != ".prefab" && ext != ".asset" {
			return nil
		}
		abs, absErr := filepath.Abs(path)
		if absErr == nil && filepath.Clean(abs) == targetAbs {
			return nil // self: in-file links are the graph-check's responsibility
		}
		assetPath := assetPathRel(assetsRoot, path)

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			indeterminate[assetPath] = struct{}{}
			return nil
		}
		report, refErr := safety.ExtractRefs(data, namespaceForExt(ext), assetPath)
		if refErr != nil {
			// Could not even block-parse: references are unknown -> indeterminate.
			indeterminate[assetPath] = struct{}{}
			return nil
		}

		hitIDs := map[int64]struct{}{}
		for _, ref := range report.Refs {
			if !ref.HasGUID || ref.GUID != guid {
				continue
			}
			if _, ok := fileIDSet[ref.FileID]; ok {
				hitIDs[ref.FileID] = struct{}{}
			}
		}
		if len(hitIDs) > 0 {
			result.Inbound = append(result.Inbound, InboundHit{
				Path:    assetPath,
				FileIDs: sortedInt64Keys(hitIDs),
				Count:   len(hitIDs),
			})
		}
		// A file may yield some parsed inbound refs AND unparsed ones; record both.
		if len(report.Warnings) > 0 {
			indeterminate[assetPath] = struct{}{}
		}
		return nil
	})
	if walkErr != nil {
		return Result{}, walkErr
	}

	sort.Slice(result.Inbound, func(i, j int) bool { return result.Inbound[i].Path < result.Inbound[j].Path })
	for p := range indeterminate {
		result.Indeterminate = append(result.Indeterminate, p)
	}
	sort.Strings(result.Indeterminate)
	return result, nil
}

func namespaceForExt(ext string) string {
	switch ext {
	case ".unity":
		return "scene"
	case ".prefab":
		return "prefab"
	default:
		return "asset"
	}
}

func assetPathRel(assetsRoot, path string) string {
	rel, err := filepath.Rel(assetsRoot, filepath.Clean(path))
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(filepath.Join("Assets", rel))
}

func sortedInt64Keys(m map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
