package patch

// FileSchemaVersionV2 is the ops[] patch schema introduced for structural scene
// mutations (S4a reparent). v1 (place_prefab append) and v2 (ops[]) coexist;
// readers dispatch on schema_version.
const FileSchemaVersionV2 = 2

// OpReparent is the only op tag supported in S4a. Mixing multiple ops in one
// patch is deliberately out of scope for this slice.
const OpReparent = "reparent"

// Op is one structural mutation in a v2 ops[] patch. For reparent:
//   - Target is the Transform being moved.
//   - NewParent is its new m_Father (0 = move to scene root).
//   - OldParent is its current m_Father captured at plan time (0 = was at root).
type Op struct {
	Op        string `json:"op"`
	Target    int64  `json:"target"`
	NewParent int64  `json:"new_parent"`
	OldParent int64  `json:"old_parent"`
}
