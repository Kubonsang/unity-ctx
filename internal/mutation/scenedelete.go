package mutation

import (
	"fmt"
	"sort"

	"github.com/Kubonsang/unity-ctx/internal/parser"
	"github.com/Kubonsang/unity-ctx/internal/patch"
)

const classGameObject = 1

// SceneDeletePlan is the result of planning a delete op. EndpointBlocked
// (Policy 1: target class / stripped / missing-transform) and PlanBlocked
// (would-orphan / stripped-in-subtree / in-file-referenced) are dry-run-time
// refusals surfaced as BLOCKED before any write; on success UpdatedData holds the
// rewritten scene and DeletedFileIDs is the full set of removed fileIDs (the
// cross-file scan input).
type SceneDeletePlan struct {
	Target          int64
	Cascade         bool
	DeletedFileIDs  []int64
	ParentTransform int64 // target transform's m_Father (0 = root)
	TargetTransform int64
	Changed         bool
	UpdatedData     []byte

	EndpointBlocked bool
	EndpointBody    string // "reason=UNSUPPORTED_ENDPOINT_CLASS ..." | "reason=MISSING_TRANSFORM ..."

	PlanBlocked bool
	PlanCode    string // WOULD_ORPHAN_CHILDREN | STRIPPED_IN_SUBTREE | IN_FILE_REFERENCED
	PlanDetail  string
}

// PlanSceneDelete removes a GameObject (op.Target) and its component blocks from
// one scene, unlinking its Transform from the parent's m_Children. With
// op.Cascade it removes the whole subtree; without it, deleting an object that
// still has children is refused (would orphan them). The target must be a
// non-stripped GameObject (Policy 1); no stripped/prefab-instance block may be in
// the removed set; and no SURVIVING same-file PPtr may still reference a removed
// fileID (would dangle in-file — the graph-check has no dangling validator, so
// this is checked here). Cross-file references are the apply layer's concern.
func PlanSceneDelete(input []byte, blocks []parser.Block, op patch.Op) (SceneDeletePlan, error) {
	if op.Op != patch.OpDelete {
		return SceneDeletePlan{}, fmt.Errorf("UNSUPPORTED_OP op=%s", op.Op)
	}

	byID := make(map[int64]parser.Block, len(blocks))
	for _, b := range blocks {
		byID[b.FileID] = b
	}

	targetGO, ok := byID[op.Target]
	if !ok {
		return SceneDeletePlan{}, fmt.Errorf("NOT_FOUND fileID=%d", op.Target)
	}
	plan := SceneDeletePlan{Target: op.Target, Cascade: op.Cascade}

	// ---- Policy 1: target must be a non-stripped GameObject ----
	if targetGO.IsStripped || targetGO.ClassID != classGameObject {
		plan.EndpointBlocked = true
		plan.EndpointBody = fmt.Sprintf("reason=UNSUPPORTED_ENDPOINT_CLASS endpoint=target id=%d class=%d is_stripped=%t allowed=%d",
			op.Target, targetGO.ClassID, targetGO.IsStripped, classGameObject)
		return plan, nil
	}

	targetTransform, ok := gameObjectTransform(byID, targetGO)
	if !ok {
		plan.EndpointBlocked = true
		plan.EndpointBody = fmt.Sprintf("reason=MISSING_TRANSFORM id=%d", op.Target)
		return plan, nil
	}
	plan.TargetTransform = targetTransform
	tb := byID[targetTransform]
	plan.ParentTransform = blockFatherID(tb)

	// ---- Parent-editability guard: to unlink the target we must rewrite the
	// parent's m_Children. A stripped (prefab-instance) parent has no local
	// m_Children — its child list lives in the PrefabInstance/source asset — so the
	// unlink cannot be done safely; refuse. A dangling m_Father is a pre-existing
	// graph issue named here precisely. ----
	if plan.ParentTransform != 0 {
		pb, ok := byID[plan.ParentTransform]
		if !ok {
			plan.PlanBlocked = true
			plan.PlanCode = "PARENT_NOT_FOUND"
			plan.PlanDetail = fmt.Sprintf("parent=%d (target's m_Father is dangling)", plan.ParentTransform)
			return plan, nil
		}
		if pb.IsStripped {
			plan.PlanBlocked = true
			plan.PlanCode = "PARENT_STRIPPED"
			plan.PlanDetail = fmt.Sprintf("parent=%d is a prefab-instance transform; its m_Children cannot be edited", plan.ParentTransform)
			return plan, nil
		}
	}

	// ---- Would-orphan guard: a non-cascade delete of an object with children ----
	children := blockChildIDs(tb)
	if len(children) > 0 && !op.Cascade {
		plan.PlanBlocked = true
		plan.PlanCode = "WOULD_ORPHAN_CHILDREN"
		plan.PlanDetail = fmt.Sprintf("target=%d child_count=%d (use --cascade to delete the subtree)", op.Target, len(children))
		return plan, nil
	}

	// ---- Collect the removed set (object + components, + subtree if cascading) ----
	deleted := map[int64]struct{}{}
	collectDeleteSet(byID, targetGO, deleted, op.Cascade)

	// Never raw-delete prefab-instance (stripped) content: its overrides live in a
	// PrefabInstance/source asset and a raw block removal would corrupt the link.
	for id := range deleted {
		if b, ok := byID[id]; ok && b.IsStripped {
			plan.PlanBlocked = true
			plan.PlanCode = "STRIPPED_IN_SUBTREE"
			plan.PlanDetail = fmt.Sprintf("id=%d (prefab-instance content cannot be raw-deleted)", id)
			return plan, nil
		}
	}
	plan.DeletedFileIDs = sortedSet(deleted)

	// ---- Build the mutation: unlink from parent m_Children, then remove blocks ----
	data := cloneBytes(input)
	var err error
	if plan.ParentTransform != 0 {
		if data, err = applyRemoveChild(data, plan.ParentTransform, targetTransform); err != nil {
			return plan, err
		}
	}
	if data, err = removeBlocks(data, deleted); err != nil {
		return plan, err
	}

	// ---- In-file dangling guard: any surviving SAME-FILE PPtr into the removed
	// set would dangle. The graph-check (fgcheck) has no dangling validator, so the
	// removal is refused here rather than committing a broken scene. ----
	if dangler, danglee, found, perr := firstInFileDangling(data, deleted); perr != nil {
		return plan, perr
	} else if found {
		plan.PlanBlocked = true
		plan.PlanCode = "IN_FILE_REFERENCED"
		plan.PlanDetail = fmt.Sprintf("block=%d still references deleted fileID=%d", dangler, danglee)
		return plan, nil
	}

	plan.Changed = !bytesEqual(input, data)
	plan.UpdatedData = data
	return plan, nil
}

// VerifySceneDelete re-reads bytes and confirms the delete predicates: every
// removed fileID is absent, and the parent (if any) no longer lists the target
// transform. (Replaces the append-era "reserved fileID exists" verify with an
// absence assertion.)
func VerifySceneDelete(data []byte, deleted []int64, parentTransform, targetTransform int64) (bool, string) {
	blocks, err := parser.Parse(data)
	if err != nil {
		return false, "reparse_failed"
	}
	byID := make(map[int64]parser.Block, len(blocks))
	for _, b := range blocks {
		byID[b.FileID] = b
	}
	for _, id := range deleted {
		if _, present := byID[id]; present {
			return false, fmt.Sprintf("still_present id=%d", id)
		}
	}
	if parentTransform != 0 {
		pb, ok := byID[parentTransform]
		if !ok {
			return false, fmt.Sprintf("parent_missing id=%d", parentTransform)
		}
		if mChildrenContains(pb, targetTransform) {
			return false, fmt.Sprintf("parent_still_lists_child parent=%d child=%d", parentTransform, targetTransform)
		}
	}
	return true, ""
}

// --- helpers ---

// gameObjectTransform returns the fileID of the GameObject's Transform/
// RectTransform component (the hierarchy node carrying m_Father/m_Children).
func gameObjectTransform(byID map[int64]parser.Block, gameObject parser.Block) (int64, bool) {
	for _, cid := range blockComponentIDs(gameObject) {
		if b, ok := byID[cid]; ok && isTransformClass(b.ClassID) {
			return cid, true
		}
	}
	return 0, false
}

// collectDeleteSet adds the GameObject and all its components to deleted; when
// cascade is set it recurses the Transform subtree (each child Transform's
// GameObject and components). A seen-guard makes a malformed cyclic hierarchy
// terminate. A child Transform whose GameObject cannot be resolved still has its
// own block removed.
func collectDeleteSet(byID map[int64]parser.Block, gameObject parser.Block, deleted map[int64]struct{}, cascade bool) {
	if _, seen := deleted[gameObject.FileID]; seen {
		return
	}
	deleted[gameObject.FileID] = struct{}{}
	for _, cid := range blockComponentIDs(gameObject) {
		deleted[cid] = struct{}{}
	}
	if !cascade {
		return
	}
	tID, ok := gameObjectTransform(byID, gameObject)
	if !ok {
		return
	}
	for _, childTransform := range blockChildIDs(byID[tID]) {
		ct, ok := byID[childTransform]
		if !ok {
			continue // dangling child link: a pre-existing graph issue, not ours to chase
		}
		if childGO, ok := byID[blockGameObjectID(ct)]; ok {
			collectDeleteSet(byID, childGO, deleted, true)
		} else {
			deleted[childTransform] = struct{}{}
		}
	}
}

// removeBlocks rewrites data with every block whose fileID is in remove deleted
// (header line through the line before the next block). Line indices come from a
// re-parse of the SAME bytes, so they align with splitPreservedLines.
func removeBlocks(data []byte, remove map[int64]struct{}) ([]byte, error) {
	blocks, err := parser.Parse(data)
	if err != nil {
		return nil, err
	}
	lines := splitPreservedLines(data)
	drop := make([]bool, len(lines))
	for _, b := range blocks {
		if _, ok := remove[b.FileID]; !ok {
			continue
		}
		header := b.StartLine - 1 // StartLine is the first body line (0-based); header precedes it
		end := b.EndLine          // exclusive: the next block's header (or trimmed EOF)
		if header < 0 {
			header = 0
		}
		if end > len(lines) {
			end = len(lines)
		}
		for i := header; i < end; i++ {
			drop[i] = true
		}
	}
	out := make([]preservedLine, 0, len(lines))
	for i, ln := range lines {
		if !drop[i] {
			out = append(out, ln)
		}
	}
	return joinLines(out), nil
}

// firstInFileDangling returns the first surviving same-file PPtr (a {fileID}
// reference with no guid) that points at a removed fileID. data is the
// post-removal scene, so every block in it survives; any such reference would
// dangle once the delete commits.
func firstInFileDangling(data []byte, deleted map[int64]struct{}) (dangler, danglee int64, found bool, err error) {
	blocks, err := parser.Parse(data)
	if err != nil {
		return 0, 0, false, err
	}
	for i := range blocks {
		hit := int64(0)
		walkSameFilePPtrs(blocks[i].Fields, func(fileID int64) {
			if hit != 0 {
				return
			}
			if _, ok := deleted[fileID]; ok {
				hit = fileID
			}
		})
		if hit != 0 {
			return blocks[i].FileID, hit, true, nil
		}
	}
	return 0, 0, false, nil
}

// walkSameFilePPtrs reports the fileID of every same-file PPtr (a map carrying
// "fileID" and NO non-empty "guid"; fileID != 0) in a parsed field tree. A PPtr
// with a guid is a cross-file/asset reference and is the xref scanner's concern.
func walkSameFilePPtrs(value any, onRef func(fileID int64)) {
	switch v := value.(type) {
	case map[string]any:
		if fidRaw, ok := v["fileID"]; ok {
			if fileID, ok := parser.AsInt64(fidRaw); ok && fileID != 0 {
				if g, hasG := v["guid"].(string); !hasG || g == "" {
					onRef(fileID)
				}
			}
		}
		for _, child := range v {
			walkSameFilePPtrs(child, onRef)
		}
	case []any:
		for _, child := range v {
			walkSameFilePPtrs(child, onRef)
		}
	}
}

func sortedSet(m map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
