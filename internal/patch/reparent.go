package patch

// FileSchemaVersionV2 is the ops[] patch schema introduced for structural scene
// mutations (S4a reparent). v1 (place_prefab append) and v2 (ops[]) coexist;
// readers dispatch on schema_version.
const FileSchemaVersionV2 = 2

// Op tags supported in a v2 ops[] patch. Mixing multiple ops in one patch is
// deliberately out of scope; a patch carries exactly one op.
const (
	OpReparent = "reparent"
	OpDelete   = "delete"
)

// Op is one structural mutation in a v2 ops[] patch.
//
// For reparent:
//   - Target is the Transform being moved.
//   - NewParent is its new m_Father (0 = move to scene root).
//   - OldParent is its current m_Father captured at plan time (0 = was at root).
//
// For delete:
//   - Target is the GameObject being removed (its Transform and components go
//     with it; NewParent/OldParent are unused and must be 0).
//   - Cascade removes the whole Transform subtree; without it, deleting an object
//     that still has children is refused (would orphan them).
type Op struct {
	Op        string `json:"op"`
	Target    int64  `json:"target"`
	NewParent int64  `json:"new_parent"`
	OldParent int64  `json:"old_parent"`
	Cascade   bool   `json:"cascade,omitempty"`
}
