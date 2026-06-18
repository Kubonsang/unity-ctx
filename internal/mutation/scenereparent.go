package mutation

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/parser"
	"github.com/Kubonsang/unity-ctx/internal/patch"
)

// reparentableTransformClassIDs is reparent's target/parent allow-set: plain
// Transform (4) ONLY. This is intentionally narrower than reposition's set
// (isTransformClass, which also allows RectTransform 224): reposition is
// topology-invariant (coordinates only) so a class needs no hierarchy modeling,
// whereas reparent changes the hierarchy and relies on the kernel's
// symmetry/cycle modeling — which covers class 4 only. A RectTransform (224) or
// stripped endpoint cannot be symmetry/cycle-validated, so editing its hierarchy
// raw would be an unverifiable silent-invalid write; reparent BLOCKs it.
var reparentableTransformClassIDs = []int{classTransform}

func isReparentableTransformClass(classID int) bool {
	for _, c := range reparentableTransformClassIDs {
		if classID == c {
			return true
		}
	}
	return false
}

func reparentableClassList() string {
	parts := make([]string, len(reparentableTransformClassIDs))
	for i, c := range reparentableTransformClassIDs {
		parts[i] = strconv.Itoa(c)
	}
	return strings.Join(parts, ",")
}

// SceneReparentPlan is the result of planning a reparent op. EndpointBlocked
// (Policy 1) and PlanBlocked (Policy 2) are dry-run-time refusals surfaced as
// BLOCKED before any write; on success UpdatedData holds the rewritten scene.
type SceneReparentPlan struct {
	Target      int64
	NewParent   int64
	OldParent   int64
	Changed     bool
	UpdatedData []byte

	EndpointBlocked bool
	EndpointBody    string // "reason=UNSUPPORTED_ENDPOINT_CLASS endpoint=<role> id=<N> class=<C> is_stripped=<bool> allowed=4"

	PlanBlocked bool
	PlanCode    string // WOULD_CREATE_CYCLE | WOULD_BREAK_SYMMETRY
	PlanDetail  string // "chain=a->b->a" | "reason=..."
}

// PlanSceneReparent moves op.Target under op.NewParent (0 = root) within one
// scene, updating three blocks atomically: target.m_Father, the old parent's
// m_Children (remove), and the new parent's m_Children (add). It re-derives the
// old parent from the scene and rejects a stale patch. Policies 1/2 run before
// any byte rewrite.
func PlanSceneReparent(input []byte, blocks []parser.Block, op patch.Op) (SceneReparentPlan, error) {
	if op.Op != patch.OpReparent {
		return SceneReparentPlan{}, fmt.Errorf("UNSUPPORTED_OP op=%s", op.Op)
	}

	byID := make(map[int64]parser.Block, len(blocks))
	for _, b := range blocks {
		byID[b.FileID] = b
	}

	target, ok := byID[op.Target]
	if !ok {
		return SceneReparentPlan{}, fmt.Errorf("NOT_FOUND fileID=%d", op.Target)
	}
	actualOldParent := blockFatherID(target)
	if actualOldParent != op.OldParent {
		return SceneReparentPlan{}, fmt.Errorf("PATCH_STALE target=%d scene_old_parent=%d patch_old_parent=%d", op.Target, actualOldParent, op.OldParent)
	}

	plan := SceneReparentPlan{Target: op.Target, NewParent: op.NewParent, OldParent: actualOldParent}

	// ---- Policy 1: endpoint class / stripped guard ----
	type endpoint struct {
		role string
		id   int64
	}
	endpoints := []endpoint{{"target", op.Target}}
	if actualOldParent != 0 {
		endpoints = append(endpoints, endpoint{"old_parent", actualOldParent})
	}
	if op.NewParent != 0 {
		endpoints = append(endpoints, endpoint{"new_parent", op.NewParent})
	}
	for _, e := range endpoints {
		b, ok := byID[e.id]
		if !ok {
			plan.EndpointBlocked = true
			plan.EndpointBody = fmt.Sprintf("reason=UNSUPPORTED_ENDPOINT_CLASS endpoint=%s id=%d class=absent is_stripped=false allowed=%s", e.role, e.id, reparentableClassList())
			return plan, nil
		}
		if b.IsStripped || !isReparentableTransformClass(b.ClassID) {
			plan.EndpointBlocked = true
			plan.EndpointBody = fmt.Sprintf("reason=UNSUPPORTED_ENDPOINT_CLASS endpoint=%s id=%d class=%d is_stripped=%t allowed=%s", e.role, e.id, b.ClassID, b.IsStripped, reparentableClassList())
			return plan, nil
		}
	}

	// No-op: target is already under new_parent. Short-circuit BEFORE the
	// remove/add (which would otherwise reorder siblings on the same parent and
	// then fail verify), so a same-parent reparent is an idempotent no change.
	if op.NewParent == actualOldParent {
		plan.Changed = false
		plan.UpdatedData = cloneBytes(input)
		return plan, nil
	}

	// ---- Policy 2: plan-phase pre-check on the virtual post-reparent graph ----
	if op.NewParent != 0 {
		if chain, cyclic := reparentWouldCycle(byID, op.Target, op.NewParent); cyclic {
			plan.PlanBlocked = true
			plan.PlanCode = "WOULD_CREATE_CYCLE"
			plan.PlanDetail = "chain=" + chain
			return plan, nil
		}
	}
	if actualOldParent != 0 && !mChildrenContains(byID[actualOldParent], op.Target) {
		// The reparent premise (old parent lists the target) is broken; the new
		// blocks would not be symmetric. (A clean scene never hits this — pre_check
		// would already flag the pre-existing asymmetry — but the plan phase names
		// it precisely before any write.)
		plan.PlanBlocked = true
		plan.PlanCode = "WOULD_BREAK_SYMMETRY"
		plan.PlanDetail = fmt.Sprintf("reason=old_parent_missing_child old_parent=%d target=%d", actualOldParent, op.Target)
		return plan, nil
	}

	// ---- Build the mutation (re-parse between edits so line indices stay valid) ----
	data := cloneBytes(input)
	var err error
	if data, err = applySetMFather(data, op.Target, op.NewParent); err != nil {
		return plan, err
	}
	if actualOldParent != 0 {
		if data, err = applyRemoveChild(data, actualOldParent, op.Target); err != nil {
			return plan, err
		}
	}
	if op.NewParent != 0 {
		if data, err = applyAddChild(data, op.NewParent, op.Target); err != nil {
			return plan, err
		}
	}

	plan.Changed = !bytesEqual(input, data)
	plan.UpdatedData = data
	return plan, nil
}

// VerifySceneReparent re-reads bytes and confirms the three reparent predicates:
// target.m_Father == new parent; new parent lists target; old parent no longer
// lists target. (Replaces the append-era "reserved fileID exists" verify.)
func VerifySceneReparent(data []byte, target, oldParent, newParent int64) (bool, string) {
	blocks, err := parser.Parse(data)
	if err != nil {
		return false, "reparse_failed"
	}
	byID := make(map[int64]parser.Block, len(blocks))
	for _, b := range blocks {
		byID[b.FileID] = b
	}
	tb, ok := byID[target]
	if !ok {
		return false, "target_missing"
	}
	if blockFatherID(tb) != newParent {
		return false, fmt.Sprintf("father_mismatch got=%d want=%d", blockFatherID(tb), newParent)
	}
	if newParent != 0 && !mChildrenContains(byID[newParent], target) {
		return false, "new_parent_missing_child"
	}
	if oldParent != 0 && mChildrenContains(byID[oldParent], target) {
		return false, "old_parent_still_lists_child"
	}
	return true, ""
}

// --- helpers ---

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func blockFatherID(b parser.Block) int64 {
	m, ok := b.Fields["m_Father"].(map[string]any)
	if !ok {
		return 0
	}
	id, _ := parser.AsInt64(m["fileID"])
	return id
}

func mChildrenContains(b parser.Block, childID int64) bool {
	for _, id := range blockChildIDs(b) {
		if id == childID {
			return true
		}
	}
	return false
}

func blockChildIDs(b parser.Block) []int64 {
	return blockListFileIDs(b, "m_Children")
}

// blockListFileIDs reads a dash-list field of {fileID: N} entries (m_Children,
// m_Roots, ...) and returns the fileIDs. Returns nil for an absent/inline-empty
// or flow-style (string-rendered) field.
func blockListFileIDs(b parser.Block, field string) []int64 {
	raw, ok := b.Fields[field]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := parser.AsInt64(m["fileID"]); ok {
			out = append(out, id)
		}
	}
	return out
}

// ReparentTargetFileIDs returns the set of local fileIDs that together identify
// the reparented object for a cross-file reference scan: the target Transform,
// its backing GameObject, and every component on that GameObject. An external
// referrer almost always points at the GameObject (`m_GameObject: {fileID: N}`)
// or a sibling component (a UnityEvent target, a component PPtr), NOT the
// Transform — so scanning the Transform fileID alone misses the common
// reference class. The xref scanner takes a fileID SET precisely so the consumer
// can pass this whole object. Returns a sorted, de-duplicated slice; the target
// Transform fileID is always included even if its GameObject can't be resolved.
func ReparentTargetFileIDs(blocks []parser.Block, targetTransformID int64) []int64 {
	ids := map[int64]struct{}{targetTransformID: {}}
	if tb, ok := findBlockByID(blocks, targetTransformID); ok {
		if goID := blockGameObjectID(tb); goID != 0 {
			ids[goID] = struct{}{}
			if gb, ok := findBlockByID(blocks, goID); ok {
				for _, cid := range blockComponentIDs(gb) {
					if cid != 0 {
						ids[cid] = struct{}{}
					}
				}
			}
		}
	}
	out := make([]int64, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// blockGameObjectID reads a Transform/component block's m_GameObject back-pointer.
func blockGameObjectID(b parser.Block) int64 {
	m, ok := b.Fields["m_GameObject"].(map[string]any)
	if !ok {
		return 0
	}
	id, _ := parser.AsInt64(m["fileID"])
	return id
}

// blockComponentIDs reads a GameObject's m_Component list, returning each
// component's fileID (the Transform is itself one of them). It accepts every
// serialized entry shape: the modern `- component: {fileID: N}`, the legacy
// numeric-classID key `- 4: {fileID: N}` / `- 114: {fileID: N}`, and a bare
// `- {fileID: N}` — the fileID is taken from the entry's nested PPtr under
// whatever key it carries.
func blockComponentIDs(b parser.Block) []int64 {
	raw, ok := b.Fields["m_Component"].([]any)
	if !ok {
		return nil
	}
	out := make([]int64, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := parser.AsInt64(m["fileID"]); ok { // bare `- {fileID: N}`
			out = append(out, id)
			continue
		}
		// `- <key>: {fileID: N}` where key is "component" (modern) or a numeric
		// classID (legacy). A component entry is a single-key map, so the first
		// nested PPtr value is the component reference.
		for _, v := range m {
			if inner, ok := v.(map[string]any); ok {
				if id, ok := parser.AsInt64(inner["fileID"]); ok {
					out = append(out, id)
					break
				}
			}
		}
	}
	return out
}

func findBlockByID(blocks []parser.Block, id int64) (parser.Block, bool) {
	for _, b := range blocks {
		if b.FileID == id {
			return b, true
		}
	}
	return parser.Block{}, false
}

// reparentWouldCycle reports whether moving target under newParent creates a
// cycle (newParent is target or a descendant of target), and returns the cycle
// chain "target->...->newParent->target".
func reparentWouldCycle(byID map[int64]parser.Block, target, newParent int64) (string, bool) {
	visited := map[int64]bool{}
	pathUp := []int64{}
	cur := newParent
	for cur != 0 {
		pathUp = append(pathUp, cur)
		if cur == target {
			// pathUp is newParent..target; reverse to target..newParent.
			rev := make([]int64, len(pathUp))
			for i, v := range pathUp {
				rev[len(pathUp)-1-i] = v
			}
			parts := make([]string, 0, len(rev)+1)
			for _, v := range rev {
				parts = append(parts, strconv.FormatInt(v, 10))
			}
			parts = append(parts, strconv.FormatInt(target, 10))
			return strings.Join(parts, "->"), true
		}
		if visited[cur] {
			break // pre-existing cycle; not introduced by this reparent
		}
		visited[cur] = true
		b, ok := byID[cur]
		if !ok {
			break
		}
		cur = blockFatherID(b)
	}
	return "", false
}

func applySetMFather(data []byte, targetID, newParent int64) ([]byte, error) {
	blocks, err := parser.Parse(data)
	if err != nil {
		return nil, err
	}
	block, ok := findBlockByID(blocks, targetID)
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND fileID=%d", targetID)
	}
	lines := splitPreservedLines(data)
	idx, err := findFieldLine(lines, block, "m_Father")
	if err != nil {
		return nil, err
	}
	line := lines[idx].content
	colon := strings.Index(line, ":")
	if colon == -1 {
		return nil, fmt.Errorf("FIELD_NOT_REWRITABLE field=m_Father")
	}
	lines[idx].content = rewriteLineScalar(line, colon, fmt.Sprintf("{fileID: %d}", newParent))
	return joinLines(lines), nil
}

// scanMChildren locates the m_Children block of a parent and its child dash
// lines. It rejects inline-filled forms (out of scope), matching the parser's
// supported F1/F2/F3/F6 shapes.
type mChildScan struct {
	keyLine       int
	keyIndent     int
	isInlineEmpty bool
	dashIndent    int
	childLineIdx  []int
	childIDs      []int64
}

func scanMChildren(lines []preservedLine, block parser.Block, field string) (mChildScan, error) {
	idx, err := findFieldLine(lines, block, field)
	if err != nil {
		return mChildScan{}, err
	}
	content := lines[idx].content
	colon := strings.Index(content, ":")
	sc := mChildScan{keyLine: idx, keyIndent: leadingSpaces(content), dashIndent: -1}
	value := strings.TrimSpace(content[colon+1:])
	if value == "[]" {
		sc.isInlineEmpty = true
		return sc, nil
	}
	if value != "" {
		return mChildScan{}, fmt.Errorf("UNSUPPORTED_LIST_SHAPE field=%s value=%q", field, value)
	}

	end := block.EndLine
	if end > len(lines) {
		end = len(lines)
	}
	for i := idx + 1; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i].content)
		if trimmed == "" {
			continue
		}
		ind := leadingSpaces(lines[i].content)
		isDash := trimmed == "-" || strings.HasPrefix(trimmed, "- ")
		if !isDash {
			if ind <= sc.keyIndent {
				break // next sibling / parent key
			}
			continue // deeper non-dash (multiline child item)
		}
		if sc.dashIndent == -1 {
			sc.dashIndent = ind
		}
		if ind != sc.dashIndent {
			continue
		}
		fid, ok := parseDashChildID(trimmed)
		if !ok {
			return mChildScan{}, fmt.Errorf("UNSUPPORTED_LIST_SHAPE field=%s entry=%q", field, trimmed)
		}
		sc.childLineIdx = append(sc.childLineIdx, i)
		sc.childIDs = append(sc.childIDs, fid)
	}
	return sc, nil
}

func parseDashChildID(trimmed string) (int64, bool) {
	s := strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return 0, false
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if !strings.HasPrefix(inner, "fileID:") {
		return 0, false
	}
	v := strings.TrimSpace(strings.TrimPrefix(inner, "fileID:"))
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func applyAddChild(data []byte, parentID, childID int64) ([]byte, error) {
	blocks, err := parser.Parse(data)
	if err != nil {
		return nil, err
	}
	block, ok := findBlockByID(blocks, parentID)
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND fileID=%d", parentID)
	}
	lines := splitPreservedLines(data)
	sc, err := scanMChildren(lines, block, "m_Children")
	if err != nil {
		return nil, err
	}
	for _, id := range sc.childIDs {
		if id == childID {
			return data, nil // already present: idempotent
		}
	}

	dashIndent := sc.dashIndent
	if dashIndent == -1 {
		dashIndent = sc.keyIndent // F3: dash aligns with the key
	}
	dashContent := strings.Repeat(" ", dashIndent) + "- {fileID: " + strconv.FormatInt(childID, 10) + "}"

	if sc.isInlineEmpty {
		// "m_Children: []" -> "m_Children:" + a child dash line.
		key := lines[sc.keyLine].content
		colon := strings.Index(key, ":")
		lines[sc.keyLine].content = key[:colon+1]
		lines = insertChildLine(lines, sc.keyLine, dashContent)
		return joinLines(lines), nil
	}

	afterIdx := sc.keyLine
	if len(sc.childLineIdx) > 0 {
		afterIdx = sc.childLineIdx[len(sc.childLineIdx)-1]
	}
	lines = insertChildLine(lines, afterIdx, dashContent)
	return joinLines(lines), nil
}

func applyRemoveChild(data []byte, parentID, childID int64) ([]byte, error) {
	return applyRemoveListEntry(data, parentID, "m_Children", childID)
}

// applyRemoveListEntry removes the `- {fileID: childID}` entry from a block's
// dash-list field (m_Children, m_Roots, ...), collapsing the field to the
// canonical `[]` when it was the only entry. Used for reparent/delete m_Children
// edits and for unlinking a deleted root from the scene's SceneRoots m_Roots.
func applyRemoveListEntry(data []byte, parentID int64, field string, childID int64) ([]byte, error) {
	blocks, err := parser.Parse(data)
	if err != nil {
		return nil, err
	}
	block, ok := findBlockByID(blocks, parentID)
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND fileID=%d", parentID)
	}
	lines := splitPreservedLines(data)
	sc, err := scanMChildren(lines, block, field)
	if err != nil {
		return nil, err
	}

	removeAt := -1
	for i, id := range sc.childIDs {
		if id == childID {
			removeAt = sc.childLineIdx[i]
			break
		}
	}
	if removeAt == -1 {
		return nil, fmt.Errorf("LIST_ENTRY_NOT_FOUND parent=%d field=%s entry=%d", parentID, field, childID)
	}

	if len(sc.childIDs) == 1 {
		// Removing the only child: collapse to the Unity-canonical empty form.
		key := lines[sc.keyLine].content
		colon := strings.Index(key, ":")
		lines[sc.keyLine].content = key[:colon+1] + " []"
		lines = removeLineAt(lines, removeAt)
		return joinLines(lines), nil
	}
	lines = removeLineAt(lines, removeAt)
	return joinLines(lines), nil
}

// insertChildLine inserts content as a new line immediately after afterIdx,
// fixing up endings so the new line never collapses onto its predecessor — in
// particular when the predecessor was the file's last line with NO trailing
// newline (ending == ""). In that case the predecessor gets the file's newline
// and the inserted (now-last) line inherits the empty ending, preserving the
// file's "no trailing newline" property.
func insertChildLine(lines []preservedLine, afterIdx int, content string) []preservedLine {
	childEnding := lines[afterIdx].ending
	if lines[afterIdx].ending == "" {
		lines[afterIdx].ending = fileNewline(lines)
		childEnding = ""
	}
	return insertLine(lines, afterIdx+1, preservedLine{content: content, ending: childEnding})
}

// fileNewline returns the file's line terminator (CRLF/LF), defaulting to "\n".
func fileNewline(lines []preservedLine) string {
	for _, ln := range lines {
		if ln.ending != "" {
			return ln.ending
		}
	}
	return "\n"
}

func insertLine(lines []preservedLine, at int, line preservedLine) []preservedLine {
	if at < 0 {
		at = 0
	}
	if at > len(lines) {
		at = len(lines)
	}
	out := make([]preservedLine, 0, len(lines)+1)
	out = append(out, lines[:at]...)
	out = append(out, line)
	out = append(out, lines[at:]...)
	return out
}

func removeLineAt(lines []preservedLine, at int) []preservedLine {
	if at < 0 || at >= len(lines) {
		return lines
	}
	out := make([]preservedLine, 0, len(lines)-1)
	out = append(out, lines[:at]...)
	out = append(out, lines[at+1:]...)
	return out
}
