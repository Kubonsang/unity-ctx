package app

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/bench"
	"github.com/Kubonsang/unity-ctx/internal/core"
	"github.com/Kubonsang/unity-ctx/internal/document"
	impactscan "github.com/Kubonsang/unity-ctx/internal/impact"
	"github.com/Kubonsang/unity-ctx/internal/mutation"
	"github.com/Kubonsang/unity-ctx/internal/parser"
	scenepatch "github.com/Kubonsang/unity-ctx/internal/patch"
	suggestplan "github.com/Kubonsang/unity-ctx/internal/suggest"
)

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

func formatPrefabSetBody(prefix, backupPath string, plan mutation.PrefabSetResult, impactResult impactscan.Result, verified int, ackRequired bool, checks []phaseCheck) string {
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
	summary += checkSuffix(checks)

	lines := []string{
		summary,
		"SCENES " + formatImpactHits(impactResult.SceneHits),
		"PREFABS " + formatImpactHits(impactResult.PrefabHits),
	}
	if impactResult.DepthLimitHit {
		lines = append(lines, fmt.Sprintf("WARN IMPACT_DEPTH_LIMIT prefab=%s depth=%d more_possible=true", impactResult.PrefabPath, impactResult.MaxNestedDepth))
	}
	body := strings.Join(lines, "\n")
	return body + checkDetailLines(checks)
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
