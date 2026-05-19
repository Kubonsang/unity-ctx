package app

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"unity-ctx/internal/bench"
	"unity-ctx/internal/bounds"
	"unity-ctx/internal/check"
	"unity-ctx/internal/contextpack"
	"unity-ctx/internal/core"
	"unity-ctx/internal/document"
	impactscan "unity-ctx/internal/impact"
	"unity-ctx/internal/index"
	"unity-ctx/internal/mutation"
	"unity-ctx/internal/parser"
	scenepatch "unity-ctx/internal/patch"
	"unity-ctx/internal/scan"
	suggestplan "unity-ctx/internal/suggest"
)

type Service struct {
	scanRunner scan.Runner
}

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
	Manifest string
	Prefab   string
	Near     string
	Count    int
	Align    string
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
}

type loadedDoc struct {
	data   []byte
	blocks []parser.Block
	doc    *document.Doc
}

func New() *Service {
	return &Service{
		scanRunner: scan.UnityCLIRunner{},
	}
}

func NewWithScanRunner(runner scan.Runner) *Service {
	svc := New()
	if runner != nil {
		svc.scanRunner = runner
	}
	return svc
}

func (s *Service) Summarize(namespace, path string, view core.View, jsonOut bool) (core.Result, int) {
	_ = jsonOut

	loaded, err := s.load(path)
	if err != nil {
		return newErrorResult(namespace, "summarize", path, view, err), 1
	}

	return summarizeResultFromLoaded(namespace, path, view, loaded), 0
}

func (s *Service) Bench(namespace, path string, view core.View, jsonOut bool, args BenchArgs) (BenchResult, int) {
	result := BenchResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "bench",
			File:      path,
			View:      view,
		},
	}

	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR bench supports only --view compact"
		return result, 1
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	// Bench measures the actual rendered summarize/context-pack payloads for the
	// requested path, rather than path-normalized semantic content.
	summarizeResult := summarizeResultFromLoaded(namespace, path, core.ViewCompact, loaded)
	benchInput := bench.Input{
		RawBytes:       len(loaded.data),
		SummarizeBytes: len(summarizeResult.Body),
	}

	task := strings.TrimSpace(args.Task)
	if task != "" {
		rawTokens := bench.EstimateTokens(len(loaded.data))
		maxTokens := rawTokens
		measureOpts := contextpack.Options{
			Namespace: namespace,
			File:      path,
			Task:      task,
			MaxTokens: rawTokens,
		}
		minBudget := contextpack.MinimumBudgetForOptions(measureOpts, contextpack.NamedObjectCount(loaded.blocks))
		if minBudget > maxTokens {
			maxTokens = minBudget
		}
		measureOpts.MaxTokens = maxTokens

		contextPackResult, contextPackCode := contextPackResultFromOptions(loaded, measureOpts, core.ViewCompact)
		if contextPackCode != 0 {
			result.Status = contextPackResult.Status
			result.Body = contextPackResult.Body
			return result, contextPackCode
		}

		benchInput.HasContextPack = true
		benchInput.ContextPackBytes = len(contextPackResult.Body)
	}

	benchResult := bench.Build(benchInput)
	result.Status = "OK"
	result.Body = formatBenchBody(benchResult)
	if jsonOut {
		payload := benchPayloadFromResult(benchResult)
		result.Bench = &payload
	}
	return result, 0
}

func (s *Service) Query(namespace, path string, view core.View, jsonOut bool, args QueryArgs) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: namespace,
		Command:   "query",
		File:      path,
		View:      view,
	}

	if countQueryArgs(args) != 1 {
		result.Status = "ERROR"
		result.Body = "ERROR query requires exactly one of --id, --name, or --type"
		return result, 1
	}
	if args.HasID && args.ID == 0 {
		result.Status = "ERROR"
		result.Body = "ERROR query requires non-zero --id"
		return result, 1
	}
	if args.HasName && strings.TrimSpace(args.Name) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR query requires non-empty --name"
		return result, 1
	}
	if args.HasType && strings.TrimSpace(args.Type) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR query requires non-empty --type"
		return result, 1
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	switch {
	case args.HasName:
		block, err := loaded.doc.FindUniqueByName(args.Name)
		if err != nil {
			result.Status = "ERROR"
			result.Body = formatLookupError(err)
			return result, 1
		}
		result.Status = "FOUND"
		result.Body = formatFoundBlock(block, view)
		return result, 0
	case args.HasID:
		block, ok := loaded.doc.FindByFileID(args.ID)
		if !ok {
			result.Status = "ERROR"
			result.Body = fmt.Sprintf("ERROR NOT_FOUND id=%d", args.ID)
			return result, 1
		}
		result.Status = "FOUND"
		result.Body = formatFoundBlock(block, view)
		return result, 0
	default:
		matches := findByType(loaded.blocks, args.Type)
		if len(matches) == 0 {
			result.Status = "ERROR"
			result.Body = fmt.Sprintf("ERROR NOT_FOUND type=%s", args.Type)
			return result, 1
		}
		result.Status = "FOUND"
		result.Body = formatTypeMatches(args.Type, matches, view)
		return result, 0
	}
}

func (s *Service) Inspect(namespace, path string, view core.View, jsonOut bool, args InspectArgs) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: namespace,
		Command:   "inspect",
		File:      path,
		View:      view,
	}

	block, err := s.resolveInspectBlock(namespace, path, args)
	if err != nil {
		result.Status = "ERROR"
		result.Body, _ = formatServiceError(err)
		return result, 1
	}

	result.Status = "OK"
	result.Body = formatInspectBlock(block, view)
	return result, 0
}

func (s *Service) Get(namespace, path string, view core.View, jsonOut bool, args GetArgs) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: namespace,
		Command:   "get",
		File:      path,
		View:      view,
	}

	if strings.TrimSpace(args.Field) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR get requires --field"
		return result, 1
	}

	block, err := s.resolveInspectBlock(namespace, path, InspectArgs{
		HasID:     args.HasID,
		HasName:   args.HasName,
		ID:        args.ID,
		Name:      args.Name,
		Component: args.Component,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body, _ = formatServiceError(err)
		return result, 1
	}

	value, ok := document.ResolveField(block.Fields, args.Field)
	if !ok {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR FIELD_NOT_FOUND field=%s", args.Field)
		return result, 1
	}

	result.Status = "OK"
	result.Body = fmt.Sprintf("OK field=%s value=%s", args.Field, formatValue(value))
	return result, 0
}

func (s *Service) Set(namespace, path string, view core.View, jsonOut bool, args SetArgs) (SetResult, int) {
	result := SetResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "set",
			File:      path,
			View:      view,
		},
	}

	if namespace != "asset" && namespace != "prefab" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR set not implemented for namespace=%s", namespace)
		return result, 1
	}
	if args.HasID && args.ID == 0 {
		result.Status = "ERROR"
		result.Body = "ERROR set requires non-zero --id"
		return result, 1
	}
	if strings.TrimSpace(args.Field) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR set requires --field"
		return result, 1
	}
	if !args.HasValue && args.Value == "" {
		result.Status = "ERROR"
		result.Body = "ERROR set requires --value"
		return result, 1
	}

	if namespace == "asset" {
		return s.setAsset(path, args, result)
	}
	return s.setPrefab(path, jsonOut, args, result)
}

func (s *Service) setAsset(path string, args SetArgs, result SetResult) (SetResult, int) {
	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	plan, err := mutation.PlanAssetSet(loaded.data, loaded.blocks, mutation.AssetSetRequest{
		Path:    path,
		HasID:   args.HasID,
		ID:      args.ID,
		Field:   args.Field,
		Value:   args.Value,
		Rewrite: args.Write,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	if !args.Write {
		result.Status = "OK"
		result.Body = fmt.Sprintf(
			"DRY_RUN field=%s old=%s new=%s type_hint=%s changed=%d",
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
		)
		return result, 0
	}

	if !plan.Changed {
		verification := s.verifySetValue(path, args)
		result.Status = "OK"
		result.Body = fmt.Sprintf(
			"OK field=%s old=%s new=%s type_hint=%s changed=%d verified=%d",
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			boolToInt(verification.Matched),
		)
		return result, 0
	}

	backupPath, writeErr := mutation.WriteWithBackup(path, plan.UpdatedData)
	verification := setVerification{}
	if writeErr == nil || writeCommitted(writeErr) {
		verification = s.verifySetValue(path, args)
	}

	if writeErr != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d err=%v",
			backupPath,
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			boolToInt(verification.Matched),
			writeErr,
		)
		if !writeCommitted(writeErr) {
			result.Body = fmt.Sprintf("ERROR %v", writeErr)
		}
		return result, 1
	}

	if !verification.Matched {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d err=VERIFY_FAILED expected=%s actual=%s",
			backupPath,
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			0,
			plan.NewValue,
			verification.Actual,
		)
		return result, 1
	}

	result.Status = "OK"
	result.Body = fmt.Sprintf(
		"WRITE backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d",
		backupPath,
		plan.Field,
		plan.OldValue,
		plan.NewValue,
		plan.TypeHint,
		boolToInt(plan.Changed),
		boolToInt(verification.Matched),
	)
	return result, 0
}

func (s *Service) setPrefab(path string, jsonOut bool, args SetArgs, result SetResult) (SetResult, int) {
	project := strings.TrimSpace(args.Project)
	if project == "" {
		result.Status = "ERROR"
		result.Body = "ERROR set requires --project"
		return result, 1
	}
	if !args.HasID {
		result.Status = "ERROR"
		result.Body = "ERROR set requires --id"
		return result, 1
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	plan, err := mutation.PlanPrefabSet(loaded.data, loaded.blocks, mutation.PrefabSetRequest{
		Path:    path,
		HasID:   args.HasID,
		ID:      args.ID,
		Field:   args.Field,
		Value:   args.Value,
		Rewrite: args.Write && args.AckImpact,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	impactResult, err := impactscan.ScanPrefabImpact(impactscan.Request{
		ProjectPath: project,
		TargetPath:  path,
		MaxDepth:    3,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if jsonOut {
		result.Impact = impactPayloadFromScanResult(impactResult)
	}

	if !args.Write {
		result.Status = impactResult.Status
		result.Body = formatPrefabSetBody("DRY_RUN", "", plan, impactResult, 0, plan.Changed)
		return result, 0
	}
	if !plan.Changed {
		verification := s.verifySetValue(path, args)
		result.Status = impactResult.Status
		result.Body = formatPrefabSetBody("OK", "", plan, impactResult, boolToInt(verification.Matched), false)
		return result, 0
	}
	if !args.AckImpact {
		result.Status = "ERROR"
		result.Body = "ERROR set requires --ack-impact for prefab writes"
		return result, 1
	}

	backupPath, writeErr := mutation.WriteWithBackup(path, plan.UpdatedData)
	verification := setVerification{}
	if writeErr == nil || writeCommitted(writeErr) {
		verification = s.verifySetValue(path, args)
	}
	if writeErr != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d err=%v",
			backupPath,
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			boolToInt(verification.Matched),
			writeErr,
		)
		if !writeCommitted(writeErr) {
			result.Body = fmt.Sprintf("ERROR %v", writeErr)
		}
		return result, 1
	}
	if !verification.Matched {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d err=VERIFY_FAILED expected=%s actual=%s",
			backupPath,
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			0,
			plan.NewValue,
			verification.Actual,
		)
		return result, 1
	}

	result.Status = impactResult.Status
	result.Body = formatPrefabSetBody("WRITE", backupPath, plan, impactResult, 1, false)
	return result, 0
}

type setVerification struct {
	Matched bool
	Actual  string
}

func (s *Service) verifySetValue(path string, args SetArgs) setVerification {
	loaded, err := s.load(path)
	if err != nil {
		return setVerification{Actual: "UNREADABLE"}
	}

	target, err := resolveSetTarget(loaded.blocks, loaded.doc, args.HasID, args.ID)
	if err != nil {
		return setVerification{Actual: "NOT_RESOLVED"}
	}

	value, ok := document.ResolveField(target.Fields, args.Field)
	if !ok {
		return setVerification{Actual: "FIELD_NOT_FOUND"}
	}

	actual := formatValue(value)
	return setVerification{
		Matched: matchesSetValue(value, args.Value),
		Actual:  actual,
	}
}

func (s *Service) Index(namespace, path string, view core.View, jsonOut bool, args IndexArgs) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: namespace,
		Command:   "index",
		File:      path,
		View:      view,
	}

	if strings.TrimSpace(args.Out) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR index requires --out"
		return result, 1
	}
	if samePath(path, args.Out) {
		result.Status = "ERROR"
		result.Body = "ERROR index requires --out to differ from input file"
		return result, 1
	}

	stalePrefix, err := staleIndexPrefix(args.Out, path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	snapshot, err := index.BuildSnapshotFromData(namespace, path, loaded.data, loaded.blocks)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if err := index.Save(args.Out, snapshot); err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	result.Status = "OK"
	result.Body = stalePrefix + fmt.Sprintf("OK index file=%s out=%s objects=%d", path, args.Out, len(snapshot.Objects))
	return result, 0
}

func (s *Service) ContextPack(namespace, path string, view core.View, jsonOut bool, args ContextPackArgs) (core.Result, int) {
	_ = jsonOut

	loaded, err := s.load(path)
	if err != nil {
		return newErrorResult(namespace, "context-pack", path, view, err), 1
	}
	return contextPackResultFromLoaded(namespace, path, view, loaded, args)
}

func (s *Service) Impact(namespace, path string, view core.View, jsonOut bool, args ImpactArgs) (ImpactResult, int) {
	result := ImpactResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "impact",
			File:      path,
			View:      view,
		},
	}

	if namespace != "prefab" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR impact not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR impact supports only --view compact"
		return result, 1
	}

	project := strings.TrimSpace(args.Project)
	if project == "" {
		result.Status = "ERROR"
		result.Body = "ERROR impact requires --project"
		return result, 1
	}

	impactResult, err := impactscan.ScanPrefabImpact(impactscan.Request{
		ProjectPath: project,
		TargetPath:  path,
		SceneScope:  normalizeImpactSceneScope(args.Scenes),
		MaxDepth:    3,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	result.Status = impactResult.Status
	result.Body = formatImpactBody(impactResult)
	if jsonOut || impactResult.Status != "" {
		result.Impact = impactPayloadFromScanResult(impactResult)
	}
	return result, 0
}

func (s *Service) Suggest(namespace, path string, view core.View, jsonOut bool, args SuggestArgs) (SuggestResult, int) {
	result := SuggestResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "suggest",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR suggest not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR suggest supports only --view compact"
		return result, 1
	}
	if strings.TrimSpace(args.Manifest) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR suggest requires --manifest"
		return result, 1
	}
	if strings.TrimSpace(args.Prefab) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR suggest requires --prefab"
		return result, 1
	}
	if strings.TrimSpace(args.Near) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR suggest requires --near"
		return result, 1
	}

	count := args.Count
	if count <= 0 {
		count = 4
	}
	align := strings.TrimSpace(args.Align)
	if align == "" {
		align = string(suggestplan.AlignFloor)
	}
	if align != string(suggestplan.AlignFloor) && align != string(suggestplan.AlignGrid) {
		result.Status = "ERROR"
		result.Body = "ERROR suggest supports only --align floor|grid"
		return result, 1
	}

	manifest, err := bounds.Load(args.Manifest)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	plan, err := suggestplan.Plan(suggestplan.Request{
		Manifest: manifest,
		Prefab:   args.Prefab,
		Near:     args.Near,
		Count:    count,
		Align:    suggestplan.Align(align),
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	result.Status = plan.Status
	result.Body = formatSuggestBody(args.Manifest, args.Prefab, plan)
	if jsonOut {
		result.Suggest = suggestPayloadFromPlan(args.Manifest, plan)
	}
	return result, 0
}

func (s *Service) Scan(namespace, path string, view core.View, jsonOut bool, args ScanArgs) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: namespace,
		Command:   "scan",
		File:      path,
		View:      view,
	}

	if namespace != "scene" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR scan not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR scan supports only --view compact"
		return result, 1
	}

	mode := strings.TrimSpace(args.Mode)
	if mode == "" {
		result.Status = "ERROR"
		result.Body = "ERROR scan requires --mode"
		return result, 1
	}
	if mode != "editor" {
		result.Status = "ERROR"
		result.Body = "ERROR scan supports only --mode editor"
		return result, 1
	}

	project := strings.TrimSpace(args.Project)
	if project == "" {
		result.Status = "ERROR"
		result.Body = "ERROR scan requires --project"
		return result, 1
	}

	outPath := strings.TrimSpace(args.Out)
	if outPath == "" {
		result.Status = "ERROR"
		result.Body = "ERROR scan requires --out"
		return result, 1
	}

	sceneAssetPath, err := scan.ResolveSceneAssetPath(project, path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	prefabs := scan.NormalizePrefabList(args.Prefabs)
	payloadBytes, err := s.scanRunner.RunEditorScan(project, sceneAssetPath, prefabs)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR SCAN_EDITOR_FAILED project=%s scene=%s err=%v", project, sceneAssetPath, err)
		return result, 1
	}

	payload, err := scan.DecodeEditorPayload(payloadBytes)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if payload.Scene != sceneAssetPath {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR scan payload scene mismatch requested=%s payload=%s", sceneAssetPath, payload.Scene)
		return result, 1
	}

	manifest, err := scan.BuildManifestFromPayload(payload)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if err := bounds.Save(outPath, manifest); err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	result.Status = "OK"
	result.Body = fmt.Sprintf(
		"OK mode=editor project=%s scene=%s out=%s objects=%d prefabs=%d source=%s",
		project,
		sceneAssetPath,
		outPath,
		len(manifest.Objects),
		len(manifest.Prefabs),
		manifest.Source,
	)
	return result, 0
}

func (s *Service) Check(namespace, path string, view core.View, jsonOut bool, args CheckArgs) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: namespace,
		Command:   "check",
		File:      path,
		View:      view,
	}

	if namespace != "scene" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR check not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR check supports only --view compact"
		return result, 1
	}
	if strings.TrimSpace(args.Manifest) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR check requires --manifest"
		return result, 1
	}
	if strings.TrimSpace(args.Prefab) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR check requires --prefab"
		return result, 1
	}
	if !args.HasPosition {
		result.Status = "ERROR"
		result.Body = "ERROR check requires --position"
		return result, 1
	}
	if !positionIsFinite(args.Position) {
		result.Status = "ERROR"
		result.Body = "ERROR check requires finite --position values"
		return result, 1
	}

	if _, err := s.load(path); err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	manifest, err := bounds.Load(args.Manifest)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if !sameSceneReference(path, manifest.Scene) {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR manifest scene mismatch file=%s manifest_scene=%s", path, manifest.Scene)
		return result, 1
	}

	checkResult, err := check.CheckPlacement(manifest, args.Prefab, bounds.Vec3(args.Position))
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	if checkResult.Clear {
		result.Status = "OK"
		result.Body = formatCheckBody("OK", args.Manifest, args.Prefab, args.Position, nil)
		return result, 0
	}

	result.Status = "WARN"
	result.Body = formatCheckBody("WARN", args.Manifest, args.Prefab, args.Position, checkResult.OverlapIDs)
	return result, 0
}

func (s *Service) Patch(namespace, path string, view core.View, jsonOut bool, args PatchArgs) (PatchResult, int) {
	_ = jsonOut

	result := PatchResult{
		SchemaVersion: scenepatch.FileSchemaVersion,
		Result: core.Result{
			Namespace: namespace,
			Command:   "patch",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR patch not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR patch supports only --view compact"
		return result, 1
	}
	if strings.TrimSpace(args.Op) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR patch requires --op"
		return result, 1
	}
	if args.Op != "place_prefab" {
		result.Status = "ERROR"
		result.Body = "ERROR patch supports only --op place_prefab"
		return result, 1
	}
	if strings.TrimSpace(args.Manifest) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR patch requires --manifest"
		return result, 1
	}
	if strings.TrimSpace(args.Prefab) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR patch requires --prefab"
		return result, 1
	}
	if !args.HasPosition {
		result.Status = "ERROR"
		result.Body = "ERROR patch requires --position"
		return result, 1
	}
	if !positionIsFinite(args.Position) {
		result.Status = "ERROR"
		result.Body = "ERROR patch requires finite --position values"
		return result, 1
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	manifest, err := bounds.Load(args.Manifest)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if !sameSceneReference(path, manifest.Scene) {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR manifest scene mismatch file=%s manifest_scene=%s", path, manifest.Scene)
		return result, 1
	}

	plan, err := scenepatch.PlanPlacePrefab(scenepatch.PlacePrefabRequest{
		SceneBlocks: loaded.blocks,
		Manifest:    manifest,
		PrefabPath:  args.Prefab,
		PrefabRef: scenepatch.PrefabReference{
			GUID: args.PrefabGUID,
		},
		Position: bounds.Vec3(args.Position),
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	result.Status = string(plan.Status)
	result.Body = formatPatchBody(plan.Status, args.Op, args.Manifest, args.Prefab, args.Position, plan)
	result.PatchPlan = &plan
	return result, 0
}

func (s *Service) Diff(namespace, path string, view core.View, jsonOut bool, args DiffArgs) (PatchResult, int) {
	_ = jsonOut

	result := PatchResult{
		SchemaVersion: scenepatch.FileSchemaVersion,
		Result: core.Result{
			Namespace: namespace,
			Command:   "diff",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR diff not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR diff supports only --view compact"
		return result, 1
	}
	if strings.TrimSpace(args.Patch) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR diff requires --patch"
		return result, 1
	}

	envelope, err := scenepatch.LoadFile(args.Patch)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if !sameSceneReference(path, envelope.File) {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR patch scene mismatch file=%s patch_file=%s", path, envelope.File)
		return result, 1
	}

	diffResult, err := mutation.DescribeScenePatch(mutation.SceneApplyRequest{
		ScenePath: path,
		PatchPath: args.Patch,
		Envelope:  envelope,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	plan := envelope.PatchPlan
	result.Status = string(diffResult.Status)
	result.Body = formatDiffBody(diffResult.Status, args.Patch, diffResult)
	result.PatchPlan = &plan
	return result, 0
}

func (s *Service) Apply(namespace, path string, view core.View, jsonOut bool, args ApplyArgs) (PatchResult, int) {
	_ = jsonOut

	result := PatchResult{
		SchemaVersion: scenepatch.FileSchemaVersion,
		Result: core.Result{
			Namespace: namespace,
			Command:   "apply",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR apply not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR apply supports only --view compact"
		return result, 1
	}
	if strings.TrimSpace(args.Patch) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR apply requires --patch"
		return result, 1
	}

	envelope, err := scenepatch.LoadFile(args.Patch)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if !sameSceneReference(path, envelope.File) {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR patch scene mismatch file=%s patch_file=%s", path, envelope.File)
		return result, 1
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	plan, err := mutation.PlanSceneApply(loaded.data, mutation.SceneApplyRequest{
		ScenePath: path,
		PatchPath: args.Patch,
		Envelope:  envelope,
		Write:     args.Write,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	applied := plan
	if args.Write {
		applied, err = mutation.ApplyScene(mutation.SceneApplyRequest{
			ScenePath: path,
			PatchPath: args.Patch,
			Envelope:  envelope,
			Write:     true,
		}, plan)
		if err != nil {
			result.Status = "ERROR"
			if strings.HasPrefix(err.Error(), "APPLY_VERIFY_FAILED") {
				result.Body = fmt.Sprintf("ERROR %s", err)
			} else if writeCommitted(err) {
				result.Body = fmt.Sprintf(
					"ERROR WRITE_COMMITTED backup=%s patch=%s op=%s append_ops=%d changed=%d verified=%d err=%v",
					applied.BackupPath,
					args.Patch,
					applied.Operation,
					applied.AppendOps,
					boolToInt(applied.Changed),
					boolToInt(applied.Verified),
					err,
				)
			} else {
				result.Body = fmt.Sprintf("ERROR %v", err)
			}
			planCopy := envelope.PatchPlan
			result.PatchPlan = &planCopy
			return result, 1
		}
	}

	result.Status = "OK"
	if args.Write {
		result.Body = fmt.Sprintf(
			"WRITE backup=%s patch=%s op=%s append_ops=%d changed=%d verified=%d",
			applied.BackupPath,
			args.Patch,
			applied.Operation,
			applied.AppendOps,
			boolToInt(applied.Changed),
			boolToInt(applied.Verified),
		)
	} else {
		result.Body = fmt.Sprintf(
			"DRY_RUN patch=%s op=%s append_ops=%d changed=%d verified=%d",
			args.Patch,
			applied.Operation,
			applied.AppendOps,
			boolToInt(applied.Changed),
			boolToInt(applied.Verified),
		)
	}
	planCopy := envelope.PatchPlan
	result.PatchPlan = &planCopy
	return result, 0
}

func (s *Service) load(path string) (*loadedDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return loadDocFromBytes(data)
}

func loadDocFromBytes(data []byte) (*loadedDoc, error) {
	blocks, err := parser.Parse(data)
	if err != nil {
		return nil, err
	}

	return &loadedDoc{
		data:   data,
		blocks: blocks,
		doc:    document.Build(blocks),
	}, nil
}

func summarizeResultFromLoaded(namespace, path string, view core.View, loaded *loadedDoc) core.Result {
	gameObjects := 0
	components := 0
	unknown := 0
	for _, block := range loaded.blocks {
		switch {
		case block.TypeName == "GameObject":
			gameObjects++
		case block.TypeName == "":
			unknown++
		default:
			components++
		}
	}

	return core.Result{
		Status:    "OK",
		Namespace: namespace,
		Command:   "summarize",
		File:      path,
		View:      view,
		Body:      formatSummarizeBody(namespace, path, view, loaded.blocks, gameObjects, components, unknown),
	}
}

func contextPackResultFromLoaded(namespace, path string, view core.View, loaded *loadedDoc, args ContextPackArgs) (core.Result, int) {
	return contextPackResultFromOptions(loaded, contextpack.Options{
		Namespace: namespace,
		File:      path,
		Task:      args.Task,
		Focus:     args.Focus,
		MaxTokens: args.MaxTokens,
	}, view)
}

func contextPackResultFromOptions(loaded *loadedDoc, opts contextpack.Options, view core.View) (core.Result, int) {
	result := core.Result{
		Namespace: opts.Namespace,
		Command:   "context-pack",
		File:      opts.File,
		View:      view,
	}

	if strings.TrimSpace(opts.Task) == "" && strings.TrimSpace(opts.Focus) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR context-pack requires --focus or --task"
		return result, 1
	}
	if opts.MaxTokens < contextpack.MinimumBudget() {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR context-pack requires --max-tokens >= %d", contextpack.MinimumBudget())
		return result, 1
	}

	minBudget := contextpack.MinimumBudgetForOptions(opts, contextpack.NamedObjectCount(loaded.blocks))
	if opts.MaxTokens < minBudget {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR context-pack requires --max-tokens >= %d", minBudget)
		return result, 1
	}

	lines := contextpack.Build(opts, loaded.blocks)
	if len(lines) == 0 {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR context-pack requires --max-tokens >= %d", minBudget)
		return result, 1
	}

	result.Status = "OK"
	result.Body = strings.Join(lines, "\n")
	return result, 0
}

func newErrorResult(namespace, command, path string, view core.View, err error) core.Result {
	return core.Result{
		Status:    "ERROR",
		Namespace: namespace,
		Command:   command,
		File:      path,
		View:      view,
		Body:      fmt.Sprintf("ERROR %v", err),
	}
}

func staleIndexPrefix(outPath, sourcePath string) (string, error) {
	snapshot, err := index.Load(outPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return "", err
		}
		return fmt.Sprintf("INDEX_STALE file=%s reason=invalid_snapshot reparse=true\n", sourcePath), nil
	}

	stale, reason, err := index.IsStale(snapshot, sourcePath)
	if err != nil {
		return "", err
	}
	if !stale {
		return "", nil
	}

	return fmt.Sprintf("INDEX_STALE file=%s reason=%s reparse=true\n", sourcePath, reason), nil
}

func countQueryArgs(args QueryArgs) int {
	count := 0
	if args.HasID {
		count++
	}
	if args.HasName {
		count++
	}
	if args.HasType {
		count++
	}
	return count
}

func (s *Service) resolveInspectBlock(namespace, path string, args InspectArgs) (parser.Block, error) {
	loaded, err := s.load(path)
	if err != nil {
		return parser.Block{}, err
	}
	hasID := args.HasID || args.ID != 0
	hasName := args.HasName || strings.TrimSpace(args.Name) != ""
	if args.HasID && args.ID == 0 {
		return parser.Block{}, serviceError{body: "ERROR inspect/get requires non-zero --id"}
	}
	if args.HasName && strings.TrimSpace(args.Name) == "" {
		return parser.Block{}, serviceError{body: "ERROR inspect/get requires non-empty --name"}
	}
	if hasID && hasName {
		return parser.Block{}, serviceError{body: "ERROR inspect/get requires at most one of --id or --name"}
	}

	if namespace == "asset" {
		if args.Component != "" {
			if hasID || hasName {
				targetID, err := s.resolveObjectID(loaded.doc, hasID, args.ID, hasName, args.Name)
				if err != nil {
					return parser.Block{}, err
				}
				block, ok := loaded.doc.FindByFileID(targetID)
				if !ok {
					return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR NOT_FOUND id=%d", targetID)}
				}
				if block.TypeName != args.Component {
					return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR UNKNOWN_COMPONENT component=%s", args.Component)}
				}
				return block, nil
			}

			block, err := loaded.doc.FindUniqueByType(args.Component)
			if err != nil {
				var lookupErr *document.LookupError
				if errors.As(err, &lookupErr) && lookupErr.Code == document.CodeAmbiguousType {
					return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR %s component=%s matches=%d", lookupErr.Code, args.Component, lookupErr.Count)}
				}
				return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR UNKNOWN_COMPONENT component=%s", args.Component)}
			}
			return block, nil
		}
		if hasID || hasName {
			targetID, err := s.resolveObjectID(loaded.doc, hasID, args.ID, hasName, args.Name)
			if err != nil {
				return parser.Block{}, err
			}
			block, ok := loaded.doc.FindByFileID(targetID)
			if !ok {
				return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR NOT_FOUND id=%d", targetID)}
			}
			return block, nil
		}
		if len(loaded.blocks) == 0 {
			return parser.Block{}, serviceError{body: "ERROR NOT_FOUND asset_block"}
		}
		block, ok := loaded.doc.FindByFileID(loaded.blocks[0].FileID)
		if !ok {
			return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR NOT_FOUND id=%d", loaded.blocks[0].FileID)}
		}
		return block, nil
	}

	if args.Component != "" {
		if hasID || hasName {
			targetID, err := s.resolveObjectID(loaded.doc, hasID, args.ID, hasName, args.Name)
			if err != nil {
				return parser.Block{}, err
			}
			block, count := findComponentsForObject(loaded.blocks, targetID, args.Component)
			if count == 0 {
				return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR UNKNOWN_COMPONENT component=%s", args.Component)}
			}
			if count > 1 {
				return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR AMBIGUOUS_COMPONENT component=%s matches=%d", args.Component, count)}
			}
			return block, nil
		}

		block, err := loaded.doc.FindUniqueByType(args.Component)
		if err != nil {
			var lookupErr *document.LookupError
			if errors.As(err, &lookupErr) && lookupErr.Code == document.CodeAmbiguousType {
				return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR %s component=%s matches=%d", lookupErr.Code, args.Component, lookupErr.Count)}
			}
			return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR UNKNOWN_COMPONENT component=%s", args.Component)}
		}
		return block, nil
	}

	if hasID || hasName {
		targetID, err := s.resolveObjectID(loaded.doc, hasID, args.ID, hasName, args.Name)
		if err != nil {
			return parser.Block{}, err
		}
		block, ok := loaded.doc.FindByFileID(targetID)
		if !ok {
			return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR NOT_FOUND id=%d", targetID)}
		}
		return block, nil
	}

	if namespace == "prefab" || namespace == "scene" {
		if len(loaded.blocks) == 0 {
			return parser.Block{}, serviceError{body: "ERROR NOT_FOUND block"}
		}
		block, ok := loaded.doc.FindByFileID(loaded.blocks[0].FileID)
		if !ok {
			return parser.Block{}, serviceError{body: fmt.Sprintf("ERROR NOT_FOUND id=%d", loaded.blocks[0].FileID)}
		}
		return block, nil
	}

	return parser.Block{}, serviceError{body: "ERROR unsupported namespace"}
}

func (s *Service) resolveObjectID(doc *document.Doc, hasID bool, id int64, hasName bool, name string) (int64, error) {
	if hasID && hasName {
		return 0, serviceError{body: "ERROR inspect/get requires at most one of --id or --name"}
	}
	if hasID {
		if id == 0 {
			return 0, serviceError{body: "ERROR inspect/get requires non-zero --id"}
		}
		return id, nil
	}
	if hasName {
		if strings.TrimSpace(name) == "" {
			return 0, serviceError{body: "ERROR inspect/get requires non-empty --name"}
		}
	}
	block, err := doc.FindUniqueByName(name)
	if err != nil {
		body, _ := formatServiceError(err)
		return 0, serviceError{body: body}
	}
	return block.FileID, nil
}

type serviceError struct {
	body string
}

func (e serviceError) Error() string {
	return e.body
}

func formatServiceError(err error) (string, bool) {
	switch typed := err.(type) {
	case serviceError:
		return typed.body, true
	case *document.LookupError:
		return formatLookupError(typed), true
	default:
		return fmt.Sprintf("ERROR %v", err), false
	}
}

func formatLookupError(err error) string {
	lookupErr, ok := err.(*document.LookupError)
	if !ok {
		return fmt.Sprintf("ERROR %v", err)
	}
	value := strconv.Quote(lookupErr.Value)

	switch lookupErr.Code {
	case document.CodeAmbiguousName:
		return fmt.Sprintf("ERROR %s %s=%s matches=%d", lookupErr.Code, lookupErr.Field, value, lookupErr.Count)
	case document.CodeNotFound:
		return fmt.Sprintf("ERROR %s %s=%s", lookupErr.Code, lookupErr.Field, value)
	default:
		return fmt.Sprintf("ERROR %s %s=%s", lookupErr.Code, lookupErr.Field, value)
	}
}

func samePath(left, right string) bool {
	leftAbs, err := filepath.Abs(left)
	if err != nil {
		return false
	}
	rightAbs, err := filepath.Abs(right)
	if err != nil {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(leftAbs); err == nil {
		leftAbs = resolved
	}
	if resolved, err := filepath.EvalSymlinks(rightAbs); err == nil {
		rightAbs = resolved
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}

func sameSceneReference(left, right string) bool {
	if samePath(left, right) {
		return true
	}

	leftBase := strings.TrimSuffix(filepath.Base(left), filepath.Ext(left))
	rightBase := strings.TrimSuffix(filepath.Base(right), filepath.Ext(right))
	return normalizeSceneReference(leftBase) == normalizeSceneReference(rightBase) &&
		strings.EqualFold(filepath.Ext(left), filepath.Ext(right))
}

func normalizeSceneReference(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			continue
		}
		if r >= 'A' && r <= 'Z' {
			builder.WriteRune(r + ('a' - 'A'))
		}
	}
	return builder.String()
}

func formatCheckBody(prefix, manifestPath, prefabPath string, position [3]float64, overlapIDs []int64) string {
	var builder strings.Builder
	builder.WriteString(prefix)
	builder.WriteString(" manifest=")
	builder.WriteString(manifestPath)
	builder.WriteString(" prefab=")
	builder.WriteString(prefabPath)
	builder.WriteString(" position=")
	builder.WriteString(formatPosition(position))
	builder.WriteString(" overlap_ids=")
	if len(overlapIDs) == 0 {
		builder.WriteString("none")
		return builder.String()
	}

	ids := append([]int64(nil), overlapIDs...)
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	for i, id := range ids {
		if i > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatInt(id, 10))
	}
	return builder.String()
}

func formatImpactBody(result impactscan.Result) string {
	summary := fmt.Sprintf(
		"%s prefab=%s guid=%s scenes=%d scene_refs=%d prefabs=%d prefab_refs=%d nested_depth=%d",
		result.Status,
		result.PrefabPath,
		result.PrefabGUID,
		len(result.SceneHits),
		sumImpactReferences(result.SceneHits),
		len(result.PrefabHits),
		sumImpactReferences(result.PrefabHits),
		result.MaxNestedDepth,
	)

	lines := []string{
		summary,
		"SCENES " + formatImpactHits(result.SceneHits),
		"PREFABS " + formatImpactHits(result.PrefabHits),
	}
	if result.DepthLimitHit {
		lines = append(lines, fmt.Sprintf("WARN IMPACT_DEPTH_LIMIT prefab=%s depth=%d more_possible=true", result.PrefabPath, result.MaxNestedDepth))
	}
	return strings.Join(lines, "\n")
}

func formatSuggestBody(manifestPath, prefabPath string, plan suggestplan.Result) string {
	clearCount := 0
	for _, candidate := range plan.Candidates {
		if candidate.Status == "OK" {
			clearCount++
		}
	}

	lines := []string{
		fmt.Sprintf(
			"%s manifest=%s prefab=%s near=%d align=%s count=%d candidates=%d clear=%d warn=%d",
			plan.Status,
			manifestPath,
			prefabPath,
			plan.Near.FileID,
			plan.Align,
			plan.Count,
			len(plan.Candidates),
			clearCount,
			len(plan.Candidates)-clearCount,
		),
	}

	for _, candidate := range plan.Candidates {
		lines = append(lines, fmt.Sprintf(
			"CANDIDATE rank=%d direction=%s position=%s status=%s overlap_ids=%s anchor_id=%d anchor_name=%s",
			candidate.Rank,
			candidate.Direction,
			formatPosition([3]float64(candidate.Position)),
			candidate.Status,
			formatPatchIDList(candidate.OverlapIDs),
			plan.Near.FileID,
			plan.Near.Name,
		))
	}

	return strings.Join(lines, "\n")
}

func formatPrefabSetBody(prefix, backupPath string, plan mutation.PrefabSetResult, impactResult impactscan.Result, verified int, ackRequired bool) string {
	summary := prefix
	if backupPath != "" {
		summary = fmt.Sprintf("%s backup=%s", summary, backupPath)
	}
	summary = fmt.Sprintf(
		"%s field=%s old=%s new=%s type_hint=%s changed=%d",
		summary,
		plan.Field,
		plan.OldValue,
		plan.NewValue,
		plan.TypeHint,
		boolToInt(plan.Changed),
	)
	if prefix != "DRY_RUN" {
		summary = fmt.Sprintf("%s verified=%d", summary, verified)
	}
	summary = fmt.Sprintf(
		"%s impact_status=%s scenes=%d scene_refs=%d prefabs=%d prefab_refs=%d nested_depth=%d",
		summary,
		impactResult.Status,
		len(impactResult.SceneHits),
		sumImpactReferences(impactResult.SceneHits),
		len(impactResult.PrefabHits),
		sumImpactReferences(impactResult.PrefabHits),
		impactResult.MaxNestedDepth,
	)
	if prefix == "DRY_RUN" {
		summary = fmt.Sprintf("%s ack_required=%d", summary, boolToInt(ackRequired))
	}

	lines := []string{
		summary,
		"SCENES " + formatImpactHits(impactResult.SceneHits),
		"PREFABS " + formatImpactHits(impactResult.PrefabHits),
	}
	if impactResult.DepthLimitHit {
		lines = append(lines, fmt.Sprintf("WARN IMPACT_DEPTH_LIMIT prefab=%s depth=%d more_possible=true", impactResult.PrefabPath, impactResult.MaxNestedDepth))
	}
	return strings.Join(lines, "\n")
}

func formatImpactHits(hits []impactscan.FileHit) string {
	if len(hits) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(hits))
	for _, hit := range hits {
		parts = append(parts, fmt.Sprintf("%s refs=%d fileIDs=%s", hit.Path, hit.References, formatPatchIDList(hit.FileIDs)))
	}
	return strings.Join(parts, " ")
}

func sumImpactReferences(hits []impactscan.FileHit) int {
	total := 0
	for _, hit := range hits {
		total += hit.References
	}
	return total
}

func normalizeImpactSceneScope(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	scenes := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		scene := strings.TrimSpace(part)
		if scene == "" {
			continue
		}
		scenes = append(scenes, scene)
	}
	if len(scenes) == 0 {
		return nil
	}
	return scenes
}

func impactPayloadFromScanResult(result impactscan.Result) *ImpactPayload {
	return &ImpactPayload{
		Status:         result.Status,
		PrefabPath:     result.PrefabPath,
		PrefabGUID:     result.PrefabGUID,
		SceneHits:      impactFileHitsFromScan(result.SceneHits),
		PrefabHits:     impactFileHitsFromScan(result.PrefabHits),
		DepthLimitHit:  result.DepthLimitHit,
		MaxNestedDepth: result.MaxNestedDepth,
	}
}

func suggestPayloadFromPlan(manifestPath string, plan suggestplan.Result) *SuggestPayload {
	candidates := make([]SuggestCandidatePayload, 0, len(plan.Candidates))
	for _, candidate := range plan.Candidates {
		candidates = append(candidates, SuggestCandidatePayload{
			Rank:       candidate.Rank,
			Direction:  candidate.Direction,
			Position:   candidate.Position,
			Status:     candidate.Status,
			OverlapIDs: append([]int64(nil), candidate.OverlapIDs...),
		})
	}

	return &SuggestPayload{
		Status:     plan.Status,
		Manifest:   manifestPath,
		PrefabPath: plan.PrefabPath,
		Near: SuggestAnchorPayload{
			FileID: plan.Near.FileID,
			Name:   plan.Near.Name,
		},
		Align:      string(plan.Align),
		Count:      plan.Count,
		Candidates: candidates,
	}
}

func impactFileHitsFromScan(hits []impactscan.FileHit) []ImpactFileHit {
	if len(hits) == 0 {
		return nil
	}

	out := make([]ImpactFileHit, 0, len(hits))
	for _, hit := range hits {
		out = append(out, ImpactFileHit{
			Path:       hit.Path,
			References: hit.References,
			FileIDs:    append([]int64(nil), hit.FileIDs...),
		})
	}
	return out
}

func formatPatchBody(status scenepatch.Status, op, manifestPath, prefabPath string, position [3]float64, plan scenepatch.PlacePrefabPlan) string {
	var builder strings.Builder
	builder.WriteString(string(status))
	builder.WriteString(" op=")
	builder.WriteString(op)
	builder.WriteString(" manifest=")
	builder.WriteString(manifestPath)
	builder.WriteString(" prefab=")
	builder.WriteString(prefabPath)
	builder.WriteString(" position=")
	builder.WriteString(formatPosition(position))
	if plan.Reason != "" {
		builder.WriteString(" reason=")
		builder.WriteString(plan.Reason)
	}
	builder.WriteString(" overlap_ids=")
	builder.WriteString(formatPatchIDList(plan.OverlapIDs))
	builder.WriteString(" reserved_fileIDs=")
	builder.WriteString(formatPatchIDList(plan.ReservedFileIDs))
	builder.WriteString("\nPLAN prefab_guid=")
	if plan.PrefabGUID == "" {
		builder.WriteString("UNKNOWN")
	} else {
		builder.WriteString(strconv.Quote(plan.PrefabGUID))
	}
	builder.WriteString(" append_ops=")
	builder.WriteString(formatAppendOps(plan.Appends))
	return builder.String()
}

func formatDiffBody(status scenepatch.Status, patchPath string, diffResult mutation.SceneDiffResult) string {
	var builder strings.Builder
	builder.WriteString(string(status))
	builder.WriteString(" patch=")
	builder.WriteString(patchPath)
	builder.WriteString(" op=")
	builder.WriteString(diffResult.Operation)
	if diffResult.Reason != "" {
		builder.WriteString(" reason=")
		builder.WriteString(diffResult.Reason)
	}
	if len(diffResult.OverlapIDs) > 0 {
		builder.WriteString(" overlap_ids=")
		builder.WriteString(formatPatchIDList(diffResult.OverlapIDs))
	}
	builder.WriteString(" append_ops=")
	builder.WriteString(strconv.Itoa(diffResult.AppendOps))
	builder.WriteString(" reserved_fileIDs=")
	builder.WriteString(formatPatchIDList(diffResult.ReservedIDs))
	return builder.String()
}

func formatPatchIDList(ids []int64) string {
	if len(ids) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func formatAppendOps(appends []scenepatch.AppendIntent) string {
	if len(appends) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(appends))
	for _, appendOp := range appends {
		parts = append(parts, fmt.Sprintf("%s:%d:%d:%s", appendOp.Op, appendOp.ClassID, appendOp.FileID, appendOp.TypeName))
	}
	return strings.Join(parts, ",")
}

func formatPosition(position [3]float64) string {
	parts := [3]string{
		strconv.FormatFloat(position[0], 'f', -1, 64),
		strconv.FormatFloat(position[1], 'f', -1, 64),
		strconv.FormatFloat(position[2], 'f', -1, 64),
	}
	return strings.Join(parts[:], ",")
}

func positionIsFinite(position [3]float64) bool {
	for _, value := range position {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

type committedWriter interface {
	WriteCommitted() bool
}

func writeCommitted(err error) bool {
	var committedErr committedWriter
	return errors.As(err, &committedErr) && committedErr.WriteCommitted()
}

func formatInspectBlock(block parser.Block, view core.View) string {
	keys := document.SortedFieldKeys(block.Fields)
	base := fmt.Sprintf("OK component=%s fileID=%d fields=%s", block.TypeName, block.FileID, strings.Join(keys, ","))
	if view == core.ViewTiny {
		return fmt.Sprintf("OK component=%s fileID=%d", block.TypeName, block.FileID)
	}
	if view == core.ViewDetail {
		return base + " classID=" + strconv.Itoa(block.ClassID)
	}
	return base
}

func formatSummarizeBody(namespace, path string, view core.View, blocks []parser.Block, gameObjects, components, unknown int) string {
	namespace = strings.ToUpper(namespace)

	switch view {
	case core.ViewTiny:
		return fmt.Sprintf("OK %s file=%s blocks=%d", namespace, path, len(blocks))
	case core.ViewDetail:
		return fmt.Sprintf(
			"OK %s file=%s game_objects=%d components=%d unknown=%d block_fileIDs=%s",
			namespace,
			path,
			gameObjects,
			components,
			unknown,
			joinBlockFileIDs(blocks),
		)
	default:
		return fmt.Sprintf(
			"OK %s file=%s game_objects=%d components=%d unknown=%d",
			namespace,
			path,
			gameObjects,
			components,
			unknown,
		)
	}
}

func formatBenchBody(result bench.Result) string {
	var builder strings.Builder
	builder.WriteString("OK raw_bytes=")
	builder.WriteString(strconv.Itoa(result.RawBytes))
	builder.WriteString(" raw_tokens=")
	builder.WriteString(strconv.Itoa(result.RawTokens))
	builder.WriteString(" summarize_bytes=")
	builder.WriteString(strconv.Itoa(result.Summarize.Bytes))
	builder.WriteString(" summarize_tokens=")
	builder.WriteString(strconv.Itoa(result.Summarize.Tokens))
	builder.WriteString(" summarize_ratio=")
	builder.WriteString(strconv.FormatFloat(result.Summarize.Ratio, 'f', -1, 64))
	builder.WriteString(" summarize_saved_tokens=")
	builder.WriteString(strconv.Itoa(result.Summarize.SavedTokens))
	if result.ContextPack != nil {
		builder.WriteString(" context_pack_bytes=")
		builder.WriteString(strconv.Itoa(result.ContextPack.Bytes))
		builder.WriteString(" context_pack_tokens=")
		builder.WriteString(strconv.Itoa(result.ContextPack.Tokens))
		builder.WriteString(" context_pack_ratio=")
		builder.WriteString(strconv.FormatFloat(result.ContextPack.Ratio, 'f', -1, 64))
		builder.WriteString(" context_pack_saved_tokens=")
		builder.WriteString(strconv.Itoa(result.ContextPack.SavedTokens))
	}
	return builder.String()
}

func benchPayloadFromResult(result bench.Result) BenchPayload {
	payload := BenchPayload{
		RawBytes:  result.RawBytes,
		RawTokens: result.RawTokens,
		Summarize: BenchMetricPayload{
			Bytes:       result.Summarize.Bytes,
			Tokens:      result.Summarize.Tokens,
			Ratio:       result.Summarize.Ratio,
			SavedTokens: result.Summarize.SavedTokens,
		},
	}
	if result.ContextPack != nil {
		metric := BenchMetricPayload{
			Bytes:       result.ContextPack.Bytes,
			Tokens:      result.ContextPack.Tokens,
			Ratio:       result.ContextPack.Ratio,
			SavedTokens: result.ContextPack.SavedTokens,
		}
		payload.ContextPack = &metric
	}
	return payload
}

func formatFoundBlock(block parser.Block, view core.View) string {
	var builder strings.Builder
	builder.WriteString("FOUND fileID=")
	builder.WriteString(strconv.FormatInt(block.FileID, 10))
	builder.WriteString(" type=")
	builder.WriteString(block.TypeName)

	if name, ok := block.Fields["m_Name"].(string); ok && name != "" {
		builder.WriteString(" name=")
		builder.WriteString(strconv.Quote(name))
	}

	if view == core.ViewDetail {
		builder.WriteString(" classID=")
		builder.WriteString(strconv.Itoa(block.ClassID))
	}

	return builder.String()
}

func findByType(blocks []parser.Block, typeName string) []parser.Block {
	matches := make([]parser.Block, 0)
	for _, block := range blocks {
		if block.TypeName == typeName {
			matches = append(matches, block)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].FileID < matches[j].FileID
	})
	return matches
}

func formatTypeMatches(typeName string, blocks []parser.Block, view core.View) string {
	ids := make([]string, 0, len(blocks))
	for _, block := range blocks {
		ids = append(ids, strconv.FormatInt(block.FileID, 10))
	}

	if view == core.ViewTiny {
		return fmt.Sprintf("FOUND type=%s matches=%d", typeName, len(blocks))
	}

	return fmt.Sprintf("FOUND type=%s matches=%d fileIDs=%s", typeName, len(blocks), strings.Join(ids, ","))
}

func findComponentsForObject(blocks []parser.Block, objectID int64, component string) (parser.Block, int) {
	var found parser.Block
	count := 0
	for _, block := range blocks {
		if block.TypeName != component {
			continue
		}
		gameObjectRef, ok := block.Fields["m_GameObject"].(map[string]any)
		if !ok {
			continue
		}
		fileID, ok := asInt64(gameObjectRef["fileID"])
		if !ok || fileID != objectID {
			continue
		}
		if count == 0 {
			found = block
		}
		count++
	}
	return found, count
}

func asInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	case float64:
		return int64(typed), true
	default:
		return 0, false
	}
}

func resolveSetTarget(blocks []parser.Block, doc *document.Doc, hasID bool, id int64) (parser.Block, error) {
	if hasID {
		block, ok := doc.FindByFileID(id)
		if !ok {
			return parser.Block{}, fmt.Errorf("NOT_FOUND fileID=%d", id)
		}
		return block, nil
	}

	if len(blocks) == 0 {
		return parser.Block{}, fmt.Errorf("NOT_FOUND asset_block")
	}
	if len(blocks) > 1 {
		return parser.Block{}, fmt.Errorf("NEED_RULE fileID matches=%d", len(blocks))
	}
	return blocks[0], nil
}

func matchesSetValue(current any, raw string) bool {
	switch typed := current.(type) {
	case string:
		return typed == raw
	case int64:
		want, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		return err == nil && typed == want
	case int:
		want, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		return err == nil && int64(typed) == want
	case float64:
		want, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			return false
		}
		if math.IsNaN(typed) && math.IsNaN(want) {
			return true
		}
		return typed == want
	case bool:
		want, err := strconv.ParseBool(strings.TrimSpace(raw))
		return err == nil && typed == want
	default:
		return false
	}
}

func formatValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case map[string]any:
		keys := document.SortedFieldKeys(typed)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s:%s", key, formatValue(typed[key])))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, formatValue(item))
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		return fmt.Sprintf("%v", value)
	}
}

func joinBlockFileIDs(blocks []parser.Block) string {
	ids := make([]string, 0, len(blocks))
	for _, block := range blocks {
		ids = append(ids, strconv.FormatInt(block.FileID, 10))
	}

	return strings.Join(ids, ",")
}
