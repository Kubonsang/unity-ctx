// Package xref scans a Unity project for inbound PPtr references to a set of
// fileIDs inside a target asset. It is per-mutation (no caching — a cached index
// could go stale and mask a dangling reference) and conservative: a file whose
// references cannot be fully accounted for is reported as "indeterminate" so a
// consumer can treat uncertainty as a block reason.
//
// The API takes a fileID SET (a single reparent target is the size-1 case) so the
// same scanner backs the delete/--cascade subtree case later (S5). reparent uses
// only the inbound list + indeterminate list as visibility; delete will use them
// as block reasons.
//
// It uses unity-ctx's own parser (which parses both inline `{fileID, guid}` and
// block-style multi-line PPtrs) rather than the kernel's single-line ExtractRefs,
// which silently drops block-style references. As a completeness backstop, any
// file whose raw bytes mention the target GUID more times than structured PPtr
// references were recovered is flagged indeterminate — so an unparseable
// reference form can never be silently reported as "no refs".
package xref

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/impact"
	"github.com/Kubonsang/unity-ctx/internal/parser"
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
	// Indeterminate: files whose references could not be fully accounted for
	// (parse failure, or a target-GUID mention not recovered as a structured
	// PPtr). Conservative signal: their inbound refs are unknown.
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
	guidBytes := []byte(guid)

	fileIDSet := make(map[int64]struct{}, len(req.FileIDs))
	for _, id := range req.FileIDs {
		fileIDSet[id] = struct{}{}
	}
	targetBase := filepath.Base(targetAbs)

	result := Result{TargetGUID: guid}
	indeterminate := map[string]struct{}{}

	walkErr := filepath.WalkDir(assetsRoot, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			// A per-entry error (unreadable directory, broken symlink, transient
			// I/O) must NEVER abort the whole walk — aborting would silently drop
			// ALL inbound detection and report a clean "no refs", the exact failure
			// this package promises to avoid. Flag the path indeterminate and keep
			// scanning everything else.
			indeterminate[assetPathRel(assetsRoot, path)] = struct{}{}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		// Self-exclusion: skip the file being edited (its in-file links are the
		// graph-check's job). Compare by physical-path identity (resolves symlinks
		// and cwd-relative spellings) so the target is never scanned as its own
		// referrer; gate on basename first so the symlink resolution stays off the
		// hot path for the files that cannot be the target.
		if filepath.Base(path) == targetBase && impact.SamePath(path, targetAbs) {
			return nil
		}

		// Identify Unity text-YAML assets by content (every text-serialized Unity
		// asset starts with "%YAML"), not by an extension allow-list — so .mat,
		// .controller, .anim, .asset, and any future text asset are all covered,
		// while binaries are cheaply skipped via the header peek.
		data, isYAML, readErr := readUnityYAML(path)
		if readErr != nil {
			if isYAML {
				indeterminate[assetPathRel(assetsRoot, path)] = struct{}{}
			}
			return nil
		}
		if !isYAML {
			return nil
		}
		assetPath := assetPathRel(assetsRoot, path)

		blocks, parseErr := parser.Parse(data)
		if parseErr != nil {
			indeterminate[assetPath] = struct{}{} // can't structure refs -> unknown
			return nil
		}

		hitIDs := map[int64]struct{}{}
		parsedGUIDMentions := 0
		for i := range blocks {
			parsedGUIDMentions += countGUIDMentions(blocks[i].Fields, guid)
			collectPPtrs(blocks[i].Fields, func(fileID int64, refGUID string, hasGUID bool) {
				if !hasGUID || refGUID != guid {
					return
				}
				if _, ok := fileIDSet[fileID]; ok {
					hitIDs[fileID] = struct{}{}
				}
			})
		}
		if len(hitIDs) > 0 {
			result.Inbound = append(result.Inbound, InboundHit{
				Path:    assetPath,
				FileIDs: sortedInt64Keys(hitIDs),
				Count:   len(hitIDs),
			})
		}
		// Completeness backstop: if the target GUID appears in the raw bytes more
		// times than the parser recovered it as a value (in ANY field — a PPtr
		// guid, an Addressables m_AssetGUID, even prose), some reference form was
		// not parsed -> conservatively indeterminate (never a silent "no refs").
		// Counting every recovered mention, not just fileID-bearing PPtrs, avoids
		// false-flagging files whose target-GUID uses are all understood.
		if bytes.Count(data, guidBytes) > parsedGUIDMentions {
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

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// readUnityYAML returns the file's bytes only if it is a Unity text-YAML asset
// (header begins with "%YAML"); otherwise isYAML is false and data is nil. It
// peeks the header before reading the whole file so binaries are not loaded. A
// leading UTF-8 BOM (some non-Unity editors prepend one) and leading whitespace
// are tolerated for detection, and the BOM is stripped from the returned bytes
// so the parser and the raw-mention backstop see clean YAML — otherwise a
// BOM-prefixed referrer would be silently skipped as "not YAML".
func readUnityYAML(path string) (data []byte, isYAML bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	var head [24]byte
	n, _ := io.ReadFull(f, head[:])
	sniff := bytes.TrimLeft(bytes.TrimPrefix(head[:n], utf8BOM), " \t\r\n")
	if !bytes.HasPrefix(sniff, []byte("%YAML")) {
		return nil, false, nil
	}
	rest, readErr := io.ReadAll(f)
	if readErr != nil {
		return nil, true, readErr
	}
	full := make([]byte, 0, n+len(rest))
	full = append(full, head[:n]...)
	full = append(full, rest...)
	full = bytes.TrimPrefix(full, utf8BOM)
	return full, true, nil
}

// countGUIDMentions returns how many times guid occurs as a substring of any
// string value in a parsed field tree (recursing maps and lists). It is the
// completeness-backstop denominator: every target-GUID mention the parser
// recovered as a value — whether a PPtr guid, a bare guid field, or text — is
// "accounted for", so only mentions that survive in the raw bytes but not here
// (an unparsed/malformed reference form) drive a file to indeterminate.
func countGUIDMentions(value any, guid string) int {
	switch v := value.(type) {
	case string:
		return strings.Count(v, guid)
	case map[string]any:
		n := 0
		for _, child := range v {
			n += countGUIDMentions(child, guid)
		}
		return n
	case []any:
		n := 0
		for _, child := range v {
			n += countGUIDMentions(child, guid)
		}
		return n
	default:
		return 0
	}
}

// collectPPtrs walks a parsed field tree and reports every PPtr-shaped mapping
// (a map carrying a "fileID"), with its optional "guid". Handles inline and
// block-style PPtrs identically (the parser yields a nested map for both).
func collectPPtrs(value any, onRef func(fileID int64, guid string, hasGUID bool)) {
	switch v := value.(type) {
	case map[string]any:
		if fidRaw, ok := v["fileID"]; ok {
			if fileID, ok := asInt64(fidRaw); ok {
				g, hasG := v["guid"].(string)
				onRef(fileID, g, hasG && g != "")
			}
		}
		for _, child := range v {
			collectPPtrs(child, onRef)
		}
	case []any:
		for _, child := range v {
			collectPPtrs(child, onRef)
		}
	}
}

func asInt64(value any) (int64, bool) {
	switch v := value.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
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
