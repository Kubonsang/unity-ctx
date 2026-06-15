package app

import (
	"unity-ctx/internal/bounds"
	"unity-ctx/internal/core"
	scenepatch "unity-ctx/internal/patch"
)

type QueryArgs struct {
	HasID   bool
	HasName bool
	HasType bool
	ID      int64
	Name    string
	Type    string
}

type InspectArgs struct {
	HasID     bool
	HasName   bool
	ID        int64
	Name      string
	Component string
}

type GetArgs struct {
	HasID     bool
	HasName   bool
	ID        int64
	Name      string
	Component string
	Field     string
}

type SetArgs struct {
	HasID     bool
	HasValue  bool
	ID        int64
	Field     string
	Value     string
	Project   string
	AckImpact bool
	Write     bool
}

type IndexArgs struct {
	Out string
}

type ContextPackArgs struct {
	Task      string
	Focus     string
	MaxTokens int
}

type BenchArgs struct {
	Task string
}

type CheckArgs struct {
	Manifest    string
	Prefab      string
	HasPosition bool
	Position    [3]float64
}

type PatchArgs struct {
	Op          string
	Manifest    string
	Prefab      string
	PrefabGUID  string
	Project     string
	HasPosition bool
	Position    [3]float64
}

type DiffArgs struct {
	Patch string
}

type ApplyArgs struct {
	Patch string
	Write bool
}

type ScanArgs struct {
	Mode    string
	Project string
	Out     string
	Prefabs string
}

type ImpactArgs struct {
	Project string
	Scenes  string
}

type SuggestArgs struct {
	Manifest   string
	Prefab     string
	Near       string
	Count      int
	Align      string
	PatchOut   string
	Pick       int
	PrefabGUID string
	Project    string
}

type ImpactFileHit struct {
	Path       string  `json:"path"`
	References int     `json:"references"`
	FileIDs    []int64 `json:"file_ids"`
}

type ImpactPayload struct {
	Status         string          `json:"status"`
	PrefabPath     string          `json:"prefab_path"`
	PrefabGUID     string          `json:"prefab_guid"`
	SceneHits      []ImpactFileHit `json:"scene_hits"`
	PrefabHits     []ImpactFileHit `json:"prefab_hits"`
	DepthLimitHit  bool            `json:"depth_limit_hit"`
	MaxNestedDepth int             `json:"max_nested_depth"`
}

type SuggestAnchorPayload struct {
	FileID int64  `json:"id"`
	Name   string `json:"name"`
}

type SuggestCandidatePayload struct {
	Rank       int         `json:"rank"`
	Direction  string      `json:"direction"`
	Position   bounds.Vec3 `json:"position"`
	Status     string      `json:"status"`
	OverlapIDs []int64     `json:"overlap_ids"`
}

type SuggestPayload struct {
	Status     string                    `json:"status"`
	Manifest   string                    `json:"manifest"`
	PrefabPath string                    `json:"prefab"`
	Near       SuggestAnchorPayload      `json:"anchor"`
	Align      string                    `json:"align"`
	Count      int                       `json:"count"`
	Candidates []SuggestCandidatePayload `json:"candidates"`
}

type BenchMetricPayload struct {
	Bytes       int     `json:"bytes"`
	Tokens      int     `json:"tokens"`
	Ratio       float64 `json:"ratio"`
	SavedTokens int     `json:"saved_tokens"`
}

type BenchPayload struct {
	RawBytes    int                 `json:"raw_bytes"`
	RawTokens   int                 `json:"raw_tokens"`
	Summarize   BenchMetricPayload  `json:"summarize"`
	ContextPack *BenchMetricPayload `json:"context_pack,omitempty"`
}

type BenchResult struct {
	core.Result
	Bench *BenchPayload `json:"bench,omitempty"`
}

type PatchResult struct {
	SchemaVersion int `json:"schema_version,omitempty"`
	core.Result
	PatchPlan *scenepatch.PlacePrefabPlan `json:"patch_plan,omitempty"`
	Safety    *SafetyPayload              `json:"safety,omitempty"`
}

type ImpactResult struct {
	core.Result
	Impact *ImpactPayload `json:"impact,omitempty"`
}

type SuggestResult struct {
	core.Result
	Suggest *SuggestPayload `json:"suggest,omitempty"`
}

type SetResult struct {
	core.Result
	Impact *ImpactPayload `json:"impact,omitempty"`
	Safety *SafetyPayload `json:"safety,omitempty"`
}

type RefsPayloadReference struct {
	BlockFileID int64  `json:"block_file_id"`
	Class       string `json:"class"`
	Field       string `json:"field"`
	FileID      int64  `json:"file_id"`
	GUID        string `json:"guid,omitempty"`
	Type        *int   `json:"type,omitempty"`
}

type RefsPayloadIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	FileID   int64  `json:"file_id,omitempty"`
	Message  string `json:"message,omitempty"`
}

type RefsPayload struct {
	References []RefsPayloadReference `json:"references"`
	Warnings   int                    `json:"warnings"`
	Issues     []RefsPayloadIssue     `json:"issues"`
}

type ChangeEdit struct {
	Kind   string `json:"kind"`
	FileID int64  `json:"file_id"`
	Type   string `json:"type"`
}

type ChangesPayload struct {
	Backup  string       `json:"backup"`
	Added   int          `json:"added"`
	Removed int          `json:"removed"`
	Changed int          `json:"changed"`
	Edits   []ChangeEdit `json:"edits"`
}

type ChangesResult struct {
	core.Result
	Changes *ChangesPayload `json:"changes,omitempty"`
}

type DepsArgs struct {
	Project string
	Out     string
}

type DepEdge struct {
	GUID     string `json:"guid"`
	Path     string `json:"path,omitempty"`
	Resolved bool   `json:"resolved"`
}

type DepsPayload struct {
	Project      string    `json:"project"`
	Refs         int       `json:"refs"`
	Resolved     int       `json:"resolved"`
	Unresolved   int       `json:"unresolved"`
	Dependencies []DepEdge `json:"dependencies"`
}

type DepsResult struct {
	core.Result
	Deps *DepsPayload `json:"deps,omitempty"`
}

type RestorePayload struct {
	Backup string `json:"backup"`
	Bytes  int    `json:"bytes"`
	Check  string `json:"check"`
}

type RestoreResult struct {
	core.Result
	Restore *RestorePayload `json:"restore,omitempty"`
}

type ValidateFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Detail   string `json:"detail,omitempty"`
}

type ValidatePayload struct {
	Blocks      int               `json:"blocks"`
	GameObjects int               `json:"gameobjects"`
	Components  int               `json:"components"`
	Transforms  int               `json:"transforms"`
	Errors      int               `json:"errors"`
	Warnings    int               `json:"warnings"`
	Findings    []ValidateFinding `json:"findings"`
}

type ValidateResult struct {
	core.Result
	Validate *ValidatePayload `json:"validate,omitempty"`
}

type RefsResult struct {
	core.Result
	Refs *RefsPayload `json:"refs,omitempty"`
}
