package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/bench"
	"github.com/Kubonsang/unity-ctx/internal/bounds"
	"github.com/Kubonsang/unity-ctx/internal/check"
	"github.com/Kubonsang/unity-ctx/internal/contextpack"
	"github.com/Kubonsang/unity-ctx/internal/core"
	"github.com/Kubonsang/unity-ctx/internal/deps"
	"github.com/Kubonsang/unity-ctx/internal/document"
	impactscan "github.com/Kubonsang/unity-ctx/internal/impact"
	"github.com/Kubonsang/unity-ctx/internal/index"
	"github.com/Kubonsang/unity-ctx/internal/mutation"
	"github.com/Kubonsang/unity-ctx/internal/parser"
	scenepatch "github.com/Kubonsang/unity-ctx/internal/patch"
	"github.com/Kubonsang/unity-ctx/internal/safety"
	"github.com/Kubonsang/unity-ctx/internal/scan"
	suggestplan "github.com/Kubonsang/unity-ctx/internal/suggest"
)

type Service struct {
	scanRunner scan.Runner
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

// readFinalState reads the on-disk bytes for the post-write final_check. It is
// a seam so tests can exercise the otherwise-unreachable final_check-failure
// branch (temp_check already validated the exact bytes written, so in
// single-writer use the re-read always matches and passes).
var readFinalState = os.ReadFile

func (s *Service) setAsset(path string, args SetArgs, result SetResult) (SetResult, int) {
	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	preCheck := phaseCheck{phase: safety.PhasePre, report: safety.CheckBytes(loaded.data)}
	if preCheck.report.Blocking() {
		result.Status = "BLOCKED"
		result.Body = blockedBody(fmt.Sprintf(" file=%s field=%s", path, args.Field), preCheck)
		result.Safety = newSafetyPayload([]phaseCheck{preCheck})
		return result, 0
	}

	plan, err := mutation.PlanAssetSet(loaded.data, loaded.blocks, mutation.AssetSetRequest{
		Path:    path,
		HasID:   args.HasID,
		ID:      args.ID,
		Field:   args.Field,
		Value:   args.Value,
		Rewrite: true,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	tempCheck := phaseCheck{phase: safety.PhaseTemp, report: safety.CheckBytes(plan.UpdatedData)}
	if tempCheck.report.Blocking() {
		result.Status = "BLOCKED"
		result.Body = blockedBody(fmt.Sprintf(" file=%s field=%s", path, args.Field), tempCheck)
		result.Safety = newSafetyPayload([]phaseCheck{preCheck, tempCheck})
		return result, 0
	}
	checks := []phaseCheck{preCheck, tempCheck}

	if !args.Write {
		result.Status = "OK"
		result.Body = fmt.Sprintf(
			"DRY_RUN field=%s old=%s new=%s type_hint=%s changed=%d%s%s",
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			checkSuffix(checks),
			checkDetailLines(checks),
		)
		result.Safety = newSafetyPayload(checks)
		return result, 0
	}

	if !plan.Changed {
		verification := s.verifySetValue(path, args)
		result.Status = "OK"
		result.Body = fmt.Sprintf(
			"OK field=%s old=%s new=%s type_hint=%s changed=%d verified=%d%s%s",
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			boolToInt(verification.Matched),
			checkSuffix(checks),
			checkDetailLines(checks),
		)
		result.Safety = newSafetyPayload(checks)
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

	finalData, finalErr := readFinalState(path)
	if finalErr != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d err=final re-read failed: %v",
			backupPath,
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			boolToInt(verification.Matched),
			finalErr,
		)
		return result, 1
	}
	finalCheck := phaseCheck{phase: safety.PhaseFinal, report: safety.CheckBytes(finalData)}
	if finalCheck.report.Blocking() {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED code=GRAPH_CHECK_FAILED phase=final_check backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d%s",
			backupPath,
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			boolToInt(verification.Matched),
			checkDetailLines([]phaseCheck{finalCheck}),
		)
		result.Safety = newSafetyPayload(append(checks, finalCheck))
		return result, 1
	}
	checks = append(checks, finalCheck)

	result.Status = "OK"
	result.Body = fmt.Sprintf(
		"WRITE backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d%s%s",
		backupPath,
		plan.Field,
		plan.OldValue,
		plan.NewValue,
		plan.TypeHint,
		boolToInt(plan.Changed),
		boolToInt(verification.Matched),
		checkSuffix(checks),
		checkDetailLines(checks),
	)
	result.Safety = newSafetyPayload(checks)
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

	preCheck := phaseCheck{phase: safety.PhasePre, report: safety.CheckBytes(loaded.data)}
	if preCheck.report.Blocking() {
		result.Status = "BLOCKED"
		result.Body = blockedBody(fmt.Sprintf(" file=%s id=%d field=%s", path, args.ID, args.Field), preCheck)
		result.Safety = newSafetyPayload([]phaseCheck{preCheck})
		return result, 0
	}

	plan, err := mutation.PlanPrefabSet(loaded.data, loaded.blocks, mutation.PrefabSetRequest{
		Path:    path,
		HasID:   args.HasID,
		ID:      args.ID,
		Field:   args.Field,
		Value:   args.Value,
		Rewrite: true,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	tempCheck := phaseCheck{phase: safety.PhaseTemp, report: safety.CheckBytes(plan.UpdatedData)}
	if tempCheck.report.Blocking() {
		result.Status = "BLOCKED"
		result.Body = blockedBody(fmt.Sprintf(" file=%s id=%d field=%s", path, args.ID, args.Field), tempCheck)
		result.Safety = newSafetyPayload([]phaseCheck{preCheck, tempCheck})
		return result, 0
	}
	checks := []phaseCheck{preCheck, tempCheck}

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
		result.Body = formatPrefabSetBody("DRY_RUN", "", plan, impactResult, 0, plan.Changed, checks)
		result.Safety = newSafetyPayload(checks)
		return result, 0
	}
	if !plan.Changed {
		verification := s.verifySetValue(path, args)
		result.Status = impactResult.Status
		result.Body = formatPrefabSetBody("OK", "", plan, impactResult, boolToInt(verification.Matched), false, checks)
		result.Safety = newSafetyPayload(checks)
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

	finalData, finalErr := readFinalState(path)
	if finalErr != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d err=final re-read failed: %v",
			backupPath,
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			boolToInt(verification.Matched),
			finalErr,
		)
		return result, 1
	}
	finalCheck := phaseCheck{phase: safety.PhaseFinal, report: safety.CheckBytes(finalData)}
	if finalCheck.report.Blocking() {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED code=GRAPH_CHECK_FAILED phase=final_check backup=%s field=%s old=%s new=%s type_hint=%s changed=%d verified=%d%s",
			backupPath,
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			boolToInt(verification.Matched),
			checkDetailLines([]phaseCheck{finalCheck}),
		)
		result.Safety = newSafetyPayload(append(checks, finalCheck))
		return result, 1
	}
	checks = append(checks, finalCheck)

	result.Status = impactResult.Status
	result.Body = formatPrefabSetBody("WRITE", backupPath, plan, impactResult, 1, false, checks)
	result.Safety = newSafetyPayload(checks)
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

// Reposition sets a scene Transform's m_LocalPosition to args.Position. It is a
// structural-edit-free, topology-invariant mutation: only three numeric axis
// tokens of one inline mapping change. It reuses the dry-run-first, --write,
// .bak, and three-phase (pre/temp/final) graph-check contract that set/apply
// already establish, including the deliberate no-auto-revert final_check policy.
func (s *Service) Reposition(namespace, path string, view core.View, jsonOut bool, args RepositionArgs) (SetResult, int) {
	_ = jsonOut
	result := SetResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "reposition",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR reposition not implemented for namespace=%s", namespace)
		return result, 1
	}
	if !args.HasID || args.ID == 0 {
		result.Status = "ERROR"
		result.Body = "ERROR reposition requires non-zero --id"
		return result, 1
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	idField := fmt.Sprintf(" file=%s id=%d field=%s", path, args.ID, mutation.RepositionField)

	preCheck := phaseCheck{phase: safety.PhasePre, report: safety.CheckBytes(loaded.data)}
	if preCheck.report.Blocking() {
		result.Status = "BLOCKED"
		result.Body = blockedBody(idField, preCheck)
		result.Safety = newSafetyPayload([]phaseCheck{preCheck})
		return result, 0
	}

	plan, err := mutation.PlanSceneReposition(loaded.data, loaded.blocks, mutation.SceneRepositionRequest{
		Path:     path,
		ID:       args.ID,
		Position: args.Position,
		Rewrite:  true,
	})
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	tempCheck := phaseCheck{phase: safety.PhaseTemp, report: safety.CheckBytes(plan.UpdatedData)}
	if tempCheck.report.Blocking() {
		result.Status = "BLOCKED"
		result.Body = blockedBody(idField, tempCheck)
		result.Safety = newSafetyPayload([]phaseCheck{preCheck, tempCheck})
		return result, 0
	}
	checks := []phaseCheck{preCheck, tempCheck}

	if !args.Write {
		result.Status = "OK"
		result.Body = fmt.Sprintf(
			"DRY_RUN id=%d field=%s old=%s new=%s changed=%d%s%s",
			args.ID, plan.Field, plan.OldValue, plan.NewValue, boolToInt(plan.Changed),
			checkSuffix(checks), checkDetailLines(checks),
		)
		result.Safety = newSafetyPayload(checks)
		return result, 0
	}

	if !plan.Changed {
		verification := s.verifyReposition(path, args.ID, args.Position)
		result.Status = "OK"
		result.Body = fmt.Sprintf(
			"OK id=%d field=%s old=%s new=%s changed=%d verified=%d%s%s",
			args.ID, plan.Field, plan.OldValue, plan.NewValue, boolToInt(plan.Changed),
			boolToInt(verification.Matched), checkSuffix(checks), checkDetailLines(checks),
		)
		result.Safety = newSafetyPayload(checks)
		return result, 0
	}

	backupPath, writeErr := mutation.WriteWithBackup(path, plan.UpdatedData)
	verification := repositionVerification{}
	if writeErr == nil || writeCommitted(writeErr) {
		verification = s.verifyReposition(path, args.ID, args.Position)
	}

	if writeErr != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s id=%d field=%s old=%s new=%s changed=%d verified=%d err=%v",
			backupPath, args.ID, plan.Field, plan.OldValue, plan.NewValue, boolToInt(plan.Changed),
			boolToInt(verification.Matched), writeErr,
		)
		if !writeCommitted(writeErr) {
			result.Body = fmt.Sprintf("ERROR %v", writeErr)
		}
		return result, 1
	}

	if !verification.Matched {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s id=%d field=%s old=%s new=%s changed=%d verified=%d err=VERIFY_FAILED expected=%s actual=%s",
			backupPath, args.ID, plan.Field, plan.OldValue, plan.NewValue, boolToInt(plan.Changed),
			0, plan.NewValue, verification.Actual,
		)
		return result, 1
	}

	finalData, finalErr := readFinalState(path)
	if finalErr != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED backup=%s id=%d field=%s old=%s new=%s changed=%d verified=%d err=final re-read failed: %v",
			backupPath, args.ID, plan.Field, plan.OldValue, plan.NewValue, boolToInt(plan.Changed),
			boolToInt(verification.Matched), finalErr,
		)
		return result, 1
	}
	finalCheck := phaseCheck{phase: safety.PhaseFinal, report: safety.CheckBytes(finalData)}
	if finalCheck.report.Blocking() {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf(
			"ERROR WRITE_COMMITTED code=GRAPH_CHECK_FAILED phase=final_check backup=%s id=%d field=%s old=%s new=%s changed=%d verified=%d%s",
			backupPath, args.ID, plan.Field, plan.OldValue, plan.NewValue, boolToInt(plan.Changed),
			boolToInt(verification.Matched), checkDetailLines([]phaseCheck{finalCheck}),
		)
		result.Safety = newSafetyPayload(append(checks, finalCheck))
		return result, 1
	}
	checks = append(checks, finalCheck)

	result.Status = "OK"
	result.Body = fmt.Sprintf(
		"WRITE backup=%s id=%d field=%s old=%s new=%s changed=%d verified=%d%s%s",
		backupPath, args.ID, plan.Field, plan.OldValue, plan.NewValue, boolToInt(plan.Changed),
		boolToInt(verification.Matched), checkSuffix(checks), checkDetailLines(checks),
	)
	result.Safety = newSafetyPayload(checks)
	return result, 0
}

type repositionVerification struct {
	Matched bool
	Actual  string
}

// verifyReposition re-reads the file and confirms the target Transform's
// m_LocalPosition now equals want, mirroring verifySetValue for the Vector3
// case.
func (s *Service) verifyReposition(path string, id int64, want [3]float64) repositionVerification {
	loaded, err := s.load(path)
	if err != nil {
		return repositionVerification{Actual: "UNREADABLE"}
	}
	block, ok := loaded.doc.FindByFileID(id)
	if !ok {
		return repositionVerification{Actual: "NOT_RESOLVED"}
	}
	raw, ok := document.ResolveField(block.Fields, mutation.RepositionField)
	if !ok {
		return repositionVerification{Actual: "FIELD_NOT_FOUND"}
	}
	got, ok := vec3FromAny(raw)
	if !ok {
		return repositionVerification{Actual: "FIELD_NOT_VECTOR3"}
	}
	return repositionVerification{
		Matched: got == want,
		Actual:  formatVec3Display(got),
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

// MetaGUID resolves the GUID of an asset from its sibling .meta file.
// It never guesses: a missing .meta or guid entry yields NEED_PREFAB_GUID
// (exit 0) so callers can ask the user instead of proceeding.
func (s *Service) MetaGUID(path string, project string, jsonOut bool) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: "meta",
		Command:   "guid",
		File:      path,
		View:      core.ViewCompact,
	}

	resolved := path
	guid, err := impactscan.LoadPrefabGUID(resolved)
	if err != nil && strings.TrimSpace(project) != "" && !filepath.IsAbs(path) {
		joined := filepath.Join(project, path)
		if joinedGUID, joinedErr := impactscan.LoadPrefabGUID(joined); joinedErr == nil {
			resolved = joined
			guid = joinedGUID
			err = nil
		}
	}
	if err != nil {
		reason := "guid_missing"
		if strings.Contains(err.Error(), "meta not found") {
			reason = "meta_not_found"
		}
		result.Status = "NEED_PREFAB_GUID"
		result.Body = fmt.Sprintf("NEED_PREFAB_GUID file=%s reason=%s", path, reason)
		return result, 0
	}

	result.Status = "OK"
	result.Body = fmt.Sprintf("OK guid=%s file=%s meta=%s.meta", guid, resolved, resolved)
	return result, 0
}

// Refs surfaces the safety kernel's PPtr/GUID reference evidence so agents
// can trace what a file points at without reading raw YAML.
func (s *Service) Refs(namespace, path string, view core.View, jsonOut bool) (RefsResult, int) {
	result := RefsResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "refs",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" && namespace != "prefab" && namespace != "asset" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR refs not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR refs supports only --view compact"
		return result, 1
	}

	data, err := os.ReadFile(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	report, err := safety.ExtractRefs(data, namespace, path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	lines := []string{fmt.Sprintf(
		"%s refs file=%s count=%d warnings=%d",
		report.Status,
		path,
		len(report.Refs),
		len(report.Warnings),
	)}
	for _, ref := range report.Refs {
		line := fmt.Sprintf("REF block=%d class=%s field=%s file_id=%d", ref.Block, ref.Class, ref.Field, ref.FileID)
		if ref.HasGUID {
			line += fmt.Sprintf(" guid=%s", ref.GUID)
		}
		if ref.HasType {
			line += fmt.Sprintf(" type=%d", ref.Type)
		}
		lines = append(lines, line)
	}
	for _, warning := range report.Warnings {
		lines = append(lines, fmt.Sprintf("WARN code=%s file_id=%d message=%q", warning.Code, warning.FileID, warning.Message))
	}

	result.Status = report.Status
	result.Body = strings.Join(lines, "\n")

	if jsonOut {
		payload := &RefsPayload{
			References: []RefsPayloadReference{},
			Warnings:   len(report.Warnings),
			Issues:     []RefsPayloadIssue{},
		}
		for _, ref := range report.Refs {
			jsonRef := RefsPayloadReference{
				BlockFileID: ref.Block,
				Class:       ref.Class,
				Field:       ref.Field,
				FileID:      ref.FileID,
			}
			if ref.HasGUID {
				jsonRef.GUID = ref.GUID
			}
			if ref.HasType {
				refType := ref.Type
				jsonRef.Type = &refType
			}
			payload.References = append(payload.References, jsonRef)
		}
		for _, warning := range report.Warnings {
			payload.Issues = append(payload.Issues, RefsPayloadIssue{
				Severity: "WARN",
				Code:     warning.Code,
				FileID:   warning.FileID,
				Message:  warning.Message,
			})
		}
		result.Refs = payload
	}

	return result, 0
}

// Validate runs the unity-fileid-graph integrity check over a file and reports
// the result without mutating anything. It is the read-only form of the check
// that gates every write path, so agents can confirm a file is structurally
// sound before editing it. OK/WARN exit 0; ERROR (broken graph) exits 1.
func (s *Service) Validate(namespace, path string, view core.View, jsonOut bool) (ValidateResult, int) {
	result := ValidateResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "validate",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" && namespace != "prefab" && namespace != "asset" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR validate not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR validate supports only --view compact"
		return result, 1
	}

	data, err := os.ReadFile(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	report := safety.CheckBytes(data)
	errors, warnings := 0, 0
	for _, f := range report.Findings {
		if f.Severity == "ERROR" {
			errors++
		} else {
			warnings++
		}
	}

	lines := []string{fmt.Sprintf(
		"%s validate file=%s blocks=%d gameobjects=%d components=%d transforms=%d errors=%d warnings=%d",
		report.Status, path, report.Blocks, report.GameObjects, report.Components, report.Transforms, errors, warnings,
	)}
	for _, f := range report.Findings {
		line := fmt.Sprintf("%s code=%s", f.Severity, f.Code)
		if f.Detail != "" {
			line += " " + f.Detail
		}
		lines = append(lines, line)
	}

	result.Status = report.Status
	result.Body = strings.Join(lines, "\n")

	if jsonOut {
		payload := &ValidatePayload{
			Blocks:      report.Blocks,
			GameObjects: report.GameObjects,
			Components:  report.Components,
			Transforms:  report.Transforms,
			Errors:      errors,
			Warnings:    warnings,
			Findings:    []ValidateFinding{},
		}
		for _, f := range report.Findings {
			payload.Findings = append(payload.Findings, ValidateFinding{
				Severity: f.Severity,
				Code:     f.Code,
				Detail:   f.Detail,
			})
		}
		result.Validate = payload
	}

	exitCode := 0
	if report.Status == "ERROR" {
		exitCode = 1
	}
	return result, exitCode
}

// Changes reports the structural difference between a file and its sibling
// <file>.bak — i.e. what the last committed write changed — by matching blocks
// on fileID. Read-only; pairs with restore for inspecting/recovering an edit.
func (s *Service) Changes(namespace, path string, view core.View, jsonOut bool) (ChangesResult, int) {
	result := ChangesResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "changes",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" && namespace != "prefab" && namespace != "asset" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR changes not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR changes supports only --view compact"
		return result, 1
	}

	backupPath := path + ".bak"
	current, err := parser.ParseFile(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	if _, statErr := os.Stat(backupPath); statErr != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR changes no backup found backup=%s", backupPath)
		return result, 1
	}
	previous, err := parser.ParseFile(backupPath)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	prevByID := map[int64]parser.Block{}
	for _, b := range previous {
		prevByID[b.FileID] = b
	}
	curByID := map[int64]parser.Block{}
	for _, b := range current {
		curByID[b.FileID] = b
	}

	var edits []ChangeEdit
	for _, b := range current {
		old, ok := prevByID[b.FileID]
		if !ok {
			edits = append(edits, ChangeEdit{Kind: "ADDED", FileID: b.FileID, Type: b.TypeName})
		} else if old.RawBody != b.RawBody {
			edits = append(edits, ChangeEdit{Kind: "CHANGED", FileID: b.FileID, Type: b.TypeName})
		}
	}
	for _, b := range previous {
		if _, ok := curByID[b.FileID]; !ok {
			edits = append(edits, ChangeEdit{Kind: "REMOVED", FileID: b.FileID, Type: b.TypeName})
		}
	}
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].FileID != edits[j].FileID {
			return edits[i].FileID < edits[j].FileID
		}
		return edits[i].Kind < edits[j].Kind
	})

	added, removed, changed := 0, 0, 0
	for _, e := range edits {
		switch e.Kind {
		case "ADDED":
			added++
		case "REMOVED":
			removed++
		case "CHANGED":
			changed++
		}
	}

	lines := []string{fmt.Sprintf(
		"OK changes file=%s vs=%s added=%d removed=%d changed=%d",
		path, backupPath, added, removed, changed,
	)}
	for _, e := range edits {
		lines = append(lines, fmt.Sprintf("%s fileID=%d type=%s", e.Kind, e.FileID, e.Type))
	}

	result.Status = "OK"
	result.Body = strings.Join(lines, "\n")
	if jsonOut {
		payload := &ChangesPayload{Backup: backupPath, Added: added, Removed: removed, Changed: changed, Edits: edits}
		if payload.Edits == nil {
			payload.Edits = []ChangeEdit{}
		}
		result.Changes = payload
	}
	return result, 0
}

// Deps reports the external asset dependencies of a file: the GUIDs it
// references (via the safety kernel) resolved to asset paths within --project.
// Text by default, --json for a structured payload, --out writes a Graphviz
// DOT graph. Read-only.
func (s *Service) Deps(namespace, path string, view core.View, jsonOut bool, args DepsArgs) (DepsResult, int) {
	result := DepsResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "deps",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" && namespace != "prefab" && namespace != "asset" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR deps not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR deps supports only --view compact"
		return result, 1
	}
	if strings.TrimSpace(args.Project) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR deps requires --project"
		return result, 1
	}

	data, err := os.ReadFile(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	report, err := safety.ExtractRefs(data, namespace, path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	guids := make([]string, 0, len(report.Refs))
	for _, ref := range report.Refs {
		if ref.HasGUID {
			guids = append(guids, ref.GUID)
		}
	}

	index, err := deps.BuildIndex(args.Project)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	resolutions := deps.Resolve(index, guids)

	resolved, unresolved := 0, 0
	for _, r := range resolutions {
		if r.Resolved {
			resolved++
		} else {
			unresolved++
		}
	}

	if strings.TrimSpace(args.Out) != "" {
		if err := os.WriteFile(args.Out, []byte(renderDepsDOT(path, resolutions)), 0o644); err != nil {
			result.Status = "ERROR"
			result.Body = fmt.Sprintf("ERROR %v", err)
			return result, 1
		}
	}

	lines := []string{fmt.Sprintf(
		"OK deps file=%s project=%s refs=%d resolved=%d unresolved=%d",
		path, args.Project, len(resolutions), resolved, unresolved,
	)}
	for _, r := range resolutions {
		p := r.Path
		if !r.Resolved {
			p = "UNKNOWN"
		}
		lines = append(lines, fmt.Sprintf("DEP guid=%s path=%s", r.GUID, p))
	}
	if strings.TrimSpace(args.Out) != "" {
		lines = append(lines, fmt.Sprintf("DOT_OUT file=%s", args.Out))
	}

	result.Status = "OK"
	result.Body = strings.Join(lines, "\n")
	if jsonOut {
		payload := &DepsPayload{Project: args.Project, Refs: len(resolutions), Resolved: resolved, Unresolved: unresolved, Dependencies: []DepEdge{}}
		for _, r := range resolutions {
			payload.Dependencies = append(payload.Dependencies, DepEdge{GUID: r.GUID, Path: r.Path, Resolved: r.Resolved})
		}
		result.Deps = payload
	}
	return result, 0
}

func renderDepsDOT(file string, resolutions []deps.Resolution) string {
	var b strings.Builder
	b.WriteString("digraph deps {\n")
	b.WriteString("  rankdir=LR;\n")
	fmt.Fprintf(&b, "  %q;\n", file)
	for _, r := range resolutions {
		target := r.Path
		if !r.Resolved {
			target = "guid:" + r.GUID
		}
		fmt.Fprintf(&b, "  %q -> %q;\n", file, target)
	}
	b.WriteString("}\n")
	return b.String()
}

// Restore overwrites a file with its sibling <file>.bak, recovering the
// pre-write state left by the last committed mutation. It reports the
// integrity status of the restored content so an agent knows what state it
// recovered to.
func (s *Service) Restore(namespace, path string, view core.View, jsonOut bool) (RestoreResult, int) {
	result := RestoreResult{
		Result: core.Result{
			Namespace: namespace,
			Command:   "restore",
			File:      path,
			View:      view,
		},
	}

	if namespace != "scene" && namespace != "prefab" && namespace != "asset" {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR restore not implemented for namespace=%s", namespace)
		return result, 1
	}
	if view != core.ViewCompact {
		result.Status = "ERROR"
		result.Body = "ERROR restore supports only --view compact"
		return result, 1
	}

	backupPath := path + ".bak"
	if _, err := os.Stat(backupPath); err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR restore no backup found backup=%s", backupPath)
		return result, 1
	}

	bytesWritten, err := mutation.RestoreFromBackup(path, backupPath)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

	restored, readErr := os.ReadFile(path)
	check := "UNKNOWN"
	if readErr == nil {
		check = safety.CheckBytes(restored).Status
	}

	result.Status = "OK"
	result.Body = fmt.Sprintf("OK restore file=%s backup=%s bytes=%d check=%s", path, backupPath, bytesWritten, check)
	if jsonOut {
		result.Restore = &RestorePayload{Backup: backupPath, Bytes: bytesWritten, Check: check}
	}
	return result, 0
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

	if args.PatchOut != "" {
		pick := args.Pick
		if pick < 1 {
			pick = 1
		}
		if pick > len(plan.Candidates) {
			result.Status = "ERROR"
			result.Body = fmt.Sprintf("ERROR suggest --pick %d is out of range, candidates=%d", pick, len(plan.Candidates))
			return result, 1
		}

		candidate := plan.Candidates[pick-1]

		patchResult, patchCode := s.Patch("scene", path, view, true, PatchArgs{
			Op:          "place_prefab",
			Manifest:    args.Manifest,
			Prefab:      args.Prefab,
			PrefabGUID:  args.PrefabGUID,
			Project:     args.Project,
			HasPosition: true,
			Position:    [3]float64(candidate.Position),
		})
		if patchCode != 0 {
			result.Status = "ERROR"
			result.Body = patchResult.Body
			return result, 1
		}

		data, marshalErr := json.Marshal(patchResult)
		if marshalErr != nil {
			result.Status = "ERROR"
			result.Body = fmt.Sprintf("ERROR marshal patch: %v", marshalErr)
			return result, 1
		}
		if writeErr := os.WriteFile(args.PatchOut, data, 0o644); writeErr != nil {
			result.Status = "ERROR"
			result.Body = fmt.Sprintf("ERROR writing patch file %s: %v", args.PatchOut, writeErr)
			return result, 1
		}

		result.Body = strings.TrimRight(result.Body, "\n") + fmt.Sprintf("\nPATCH_OUT rank=%d file=%s status=%s candidate_status=%s", pick, args.PatchOut, patchResult.Status, candidate.Status)
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

// resolvePrefabGUID looks up the prefab GUID from its .meta file, retrying
// under the project root for relative paths. It returns "" when nothing can
// be resolved so the patch planner falls back to its NEED_PREFAB_GUID
// contract instead of guessing.
func resolvePrefabGUID(prefabPath, project string) string {
	if guid, err := impactscan.LoadPrefabGUID(prefabPath); err == nil {
		return guid
	}
	if strings.TrimSpace(project) != "" && !filepath.IsAbs(prefabPath) {
		if guid, err := impactscan.LoadPrefabGUID(filepath.Join(project, prefabPath)); err == nil {
			return guid
		}
	}
	return ""
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

	prefabGUID := strings.TrimSpace(args.PrefabGUID)
	if prefabGUID == "" {
		prefabGUID = resolvePrefabGUID(args.Prefab, args.Project)
	}

	plan, err := scenepatch.PlanPlacePrefab(scenepatch.PlacePrefabRequest{
		SceneBlocks: loaded.blocks,
		Manifest:    manifest,
		PrefabPath:  args.Prefab,
		PrefabRef: scenepatch.PrefabReference{
			GUID: prefabGUID,
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

	preCheck := phaseCheck{phase: safety.PhasePre, report: safety.CheckBytes(loaded.data)}
	if preCheck.report.Blocking() {
		result.Status = "BLOCKED"
		result.Body = blockedBody(fmt.Sprintf(" patch=%s file=%s", args.Patch, path), preCheck)
		result.Safety = newSafetyPayload([]phaseCheck{preCheck})
		planCopy := envelope.PatchPlan
		result.PatchPlan = &planCopy
		return result, 0
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

	tempCheck := phaseCheck{phase: safety.PhaseTemp, report: safety.CheckBytes(plan.UpdatedData)}
	if tempCheck.report.Blocking() {
		result.Status = "BLOCKED"
		result.Body = blockedBody(fmt.Sprintf(" patch=%s file=%s", args.Patch, path), tempCheck)
		result.Safety = newSafetyPayload([]phaseCheck{preCheck, tempCheck})
		planCopy := envelope.PatchPlan
		result.PatchPlan = &planCopy
		return result, 0
	}
	checks := []phaseCheck{preCheck, tempCheck}

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

		finalData, err := readFinalState(path)
		if err != nil {
			result.Status = "ERROR"
			result.Body = fmt.Sprintf(
				"ERROR WRITE_COMMITTED backup=%s patch=%s op=%s append_ops=%d changed=%d verified=%d err=final re-read failed: %v",
				applied.BackupPath,
				args.Patch,
				applied.Operation,
				applied.AppendOps,
				boolToInt(applied.Changed),
				boolToInt(applied.Verified),
				err,
			)
			planCopy := envelope.PatchPlan
			result.PatchPlan = &planCopy
			return result, 1
		}
		finalCheck := phaseCheck{phase: safety.PhaseFinal, report: safety.CheckBytes(finalData)}
		if finalCheck.report.Blocking() {
			result.Status = "ERROR"
			result.Body = fmt.Sprintf(
				"ERROR WRITE_COMMITTED code=GRAPH_CHECK_FAILED phase=final_check backup=%s patch=%s op=%s append_ops=%d changed=%d verified=%d%s",
				applied.BackupPath,
				args.Patch,
				applied.Operation,
				applied.AppendOps,
				boolToInt(applied.Changed),
				boolToInt(applied.Verified),
				checkDetailLines([]phaseCheck{finalCheck}),
			)
			result.Safety = newSafetyPayload(append(checks, finalCheck))
			planCopy := envelope.PatchPlan
			result.PatchPlan = &planCopy
			return result, 1
		}
		checks = append(checks, finalCheck)
	}

	result.Status = "OK"
	if args.Write {
		result.Body = fmt.Sprintf(
			"WRITE backup=%s patch=%s op=%s append_ops=%d changed=%d verified=%d%s%s",
			applied.BackupPath,
			args.Patch,
			applied.Operation,
			applied.AppendOps,
			boolToInt(applied.Changed),
			boolToInt(applied.Verified),
			checkSuffix(checks),
			checkDetailLines(checks),
		)
	} else {
		result.Body = fmt.Sprintf(
			"DRY_RUN patch=%s op=%s append_ops=%d changed=%d verified=%d%s%s",
			args.Patch,
			applied.Operation,
			applied.AppendOps,
			boolToInt(applied.Changed),
			boolToInt(applied.Verified),
			checkSuffix(checks),
			checkDetailLines(checks),
		)
	}
	result.Safety = newSafetyPayload(checks)
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
