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

	"unity-ctx/internal/contextpack"
	"unity-ctx/internal/core"
	"unity-ctx/internal/document"
	"unity-ctx/internal/index"
	"unity-ctx/internal/mutation"
	"unity-ctx/internal/parser"
)

type Service struct{}

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
	HasID    bool
	HasValue bool
	ID       int64
	Field    string
	Value    string
	Write    bool
}

type IndexArgs struct {
	Out string
}

type ContextPackArgs struct {
	Task      string
	Focus     string
	MaxTokens int
}

type loadedDoc struct {
	data   []byte
	blocks []parser.Block
	doc    *document.Doc
}

func New() *Service {
	return &Service{}
}

func (s *Service) Summarize(namespace, path string, view core.View, jsonOut bool) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: namespace,
		Command:   "summarize",
		File:      path,
		View:      view,
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}

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

	result.Status = "OK"
	result.Body = formatSummarizeBody(namespace, path, view, loaded.blocks, gameObjects, components, unknown)
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
	case args.Name != "":
		block, err := loaded.doc.FindUniqueByName(args.Name)
		if err != nil {
			result.Status = "ERROR"
			result.Body = formatLookupError(err)
			return result, 1
		}
		result.Status = "FOUND"
		result.Body = formatFoundBlock(block, view)
		return result, 0
	case args.ID != 0:
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

func (s *Service) Set(namespace, path string, view core.View, jsonOut bool, args SetArgs) (core.Result, int) {
	_ = jsonOut

	result := core.Result{
		Namespace: namespace,
		Command:   "set",
		File:      path,
		View:      view,
	}

	if namespace != "asset" {
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
		result.Status = "OK"
		result.Body = fmt.Sprintf(
			"OK field=%s old=%s new=%s type_hint=%s changed=%d verified=%d",
			plan.Field,
			plan.OldValue,
			plan.NewValue,
			plan.TypeHint,
			boolToInt(plan.Changed),
			1,
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

	result := core.Result{
		Namespace: namespace,
		Command:   "context-pack",
		File:      path,
		View:      view,
	}

	if strings.TrimSpace(args.Task) == "" && strings.TrimSpace(args.Focus) == "" {
		result.Status = "ERROR"
		result.Body = "ERROR context-pack requires --focus or --task"
		return result, 1
	}
	if args.MaxTokens < contextpack.MinimumBudget() {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR context-pack requires --max-tokens >= %d", contextpack.MinimumBudget())
		return result, 1
	}

	loaded, err := s.load(path)
	if err != nil {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR %v", err)
		return result, 1
	}
	minBudget := contextpack.MinimumBudgetForOptions(contextpack.Options{
		Namespace: namespace,
		File:      path,
		Task:      args.Task,
		Focus:     args.Focus,
		MaxTokens: args.MaxTokens,
	}, contextpack.NamedObjectCount(loaded.blocks))
	if args.MaxTokens < minBudget {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR context-pack requires --max-tokens >= %d", minBudget)
		return result, 1
	}

	lines := contextpack.Build(contextpack.Options{
		Namespace: namespace,
		File:      path,
		Task:      args.Task,
		Focus:     args.Focus,
		MaxTokens: args.MaxTokens,
	}, loaded.blocks)
	if len(lines) == 0 {
		result.Status = "ERROR"
		result.Body = fmt.Sprintf("ERROR context-pack requires --max-tokens >= %d", minBudget)
		return result, 1
	}

	result.Status = "OK"
	result.Body = strings.Join(lines, "\n")
	return result, 0
}

func (s *Service) load(path string) (*loadedDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
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

func staleIndexPrefix(outPath, sourcePath string) (string, error) {
	snapshot, err := index.Load(outPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
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
	if args.HasID || args.ID != 0 {
		count++
	}
	if args.HasName || args.Name != "" {
		count++
	}
	if args.HasType || args.Type != "" {
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
