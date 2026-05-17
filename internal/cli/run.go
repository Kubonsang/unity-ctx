package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"unity-ctx/internal/app"
	"unity-ctx/internal/bounds"
	"unity-ctx/internal/contextpack"
	"unity-ctx/internal/core"
	"unity-ctx/internal/parser"
)

func Run(args []string, stdout, stderr io.Writer) int {
	if isHelpArgs(args) {
		_, _ = io.WriteString(stdout, usageText())
		return 0
	}

	if len(args) < 3 {
		_, _ = io.WriteString(stderr, "ERROR missing file argument\n")
		return 2
	}

	namespace := args[0]
	command := args[1]
	file := args[2]

	flagSet := flag.NewFlagSet("unity-ctx", flag.ContinueOnError)
	flagSet.SetOutput(io.Discard)

	view := flagSet.String("view", string(core.ViewCompact), "")
	jsonOutput := flagSet.Bool("json", false, "")
	name := flagSet.String("name", "", "")
	typeName := flagSet.String("type", "", "")
	fileID := flagSet.Int64("id", 0, "")
	component := flagSet.String("component", "", "")
	field := flagSet.String("field", "", "")
	value := flagSet.String("value", "", "")
	out := flagSet.String("out", "", "")
	task := flagSet.String("task", "", "")
	focus := flagSet.String("focus", "", "")
	maxTokens := flagSet.Int("max-tokens", 256, "")
	writeFlag := flagSet.Bool("write", false, "")
	manifest := flagSet.String("manifest", "", "")
	prefab := flagSet.String("prefab", "", "")
	prefabGUID := flagSet.String("prefab-guid", "", "")
	position := flagSet.String("position", "", "")
	op := flagSet.String("op", "", "")
	patchPath := flagSet.String("patch", "", "")

	if err := flagSet.Parse(args[3:]); err != nil {
		_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
		return 2
	}
	seenFlags := visitedFlags(flagSet)

	if flagSet.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "ERROR unexpected trailing arguments: %s\n", strings.Join(flagSet.Args(), " "))
		return 2
	}

	selectedView := core.View(*view)
	if !selectedView.Valid() {
		_, _ = fmt.Fprintf(stderr, "ERROR invalid view %q\n", *view)
		return 2
	}
	if command != "set" && command != "apply" && seenFlags["write"] {
		_, _ = fmt.Fprintf(stderr, "ERROR %s does not accept --write\n", command)
		return 2
	}
	if command != "set" && seenFlags["value"] {
		_, _ = fmt.Fprintf(stderr, "ERROR %s does not accept --value\n", command)
		return 2
	}
	if command != "check" && command != "patch" && anyFlagVisited(seenFlags, "manifest", "prefab", "position") {
		_, _ = fmt.Fprintf(stderr, "ERROR %s does not accept --manifest, --prefab, or --position\n", command)
		return 2
	}
	if command != "patch" && seenFlags["op"] {
		_, _ = fmt.Fprintf(stderr, "ERROR %s does not accept --op\n", command)
		return 2
	}
	if command != "patch" && seenFlags["prefab-guid"] {
		_, _ = fmt.Fprintf(stderr, "ERROR %s does not accept --prefab-guid\n", command)
		return 2
	}
	if command != "diff" && command != "apply" && seenFlags["patch"] {
		_, _ = fmt.Fprintf(stderr, "ERROR %s does not accept --patch\n", command)
		return 2
	}

	if command == "summarize" && anyFlagVisited(seenFlags, "id", "name", "type") {
		_, _ = io.WriteString(stderr, "ERROR summarize does not accept --id, --name, or --type\n")
		return 2
	}
	if command == "summarize" && anyFlagVisited(seenFlags, "component", "field") {
		_, _ = io.WriteString(stderr, "ERROR summarize does not accept --component or --field\n")
		return 2
	}
	if command == "summarize" && anyFlagVisited(seenFlags, "out") {
		_, _ = io.WriteString(stderr, "ERROR summarize does not accept --out\n")
		return 2
	}
	if command == "summarize" && anyFlagVisited(seenFlags, "task", "focus", "max-tokens") {
		_, _ = io.WriteString(stderr, "ERROR summarize does not accept --task, --focus, or --max-tokens\n")
		return 2
	}

	if command == "query" && countVisitedFlags(seenFlags, "id", "name", "type") != 1 {
		_, _ = io.WriteString(stderr, "ERROR query requires exactly one of --id, --name, or --type\n")
		return 2
	}
	if command == "query" && seenFlags["id"] && *fileID == 0 {
		_, _ = io.WriteString(stderr, "ERROR query requires non-zero --id\n")
		return 2
	}
	if command == "query" && seenFlags["name"] && strings.TrimSpace(*name) == "" {
		_, _ = io.WriteString(stderr, "ERROR query requires non-empty --name\n")
		return 2
	}
	if command == "query" && seenFlags["type"] && strings.TrimSpace(*typeName) == "" {
		_, _ = io.WriteString(stderr, "ERROR query requires non-empty --type\n")
		return 2
	}
	if command == "query" && anyFlagVisited(seenFlags, "component", "field") {
		_, _ = io.WriteString(stderr, "ERROR query does not accept --component or --field\n")
		return 2
	}
	if command == "query" && anyFlagVisited(seenFlags, "out") {
		_, _ = io.WriteString(stderr, "ERROR query does not accept --out\n")
		return 2
	}
	if command == "query" && anyFlagVisited(seenFlags, "task", "focus", "max-tokens") {
		_, _ = io.WriteString(stderr, "ERROR query does not accept --task, --focus, or --max-tokens\n")
		return 2
	}
	if command == "inspect" && anyFlagVisited(seenFlags, "type") {
		_, _ = io.WriteString(stderr, "ERROR inspect does not accept --type\n")
		return 2
	}
	if command == "inspect" && anyFlagVisited(seenFlags, "field") {
		_, _ = io.WriteString(stderr, "ERROR inspect does not accept --field\n")
		return 2
	}
	if command == "inspect" && countVisitedFlags(seenFlags, "id", "name") > 1 {
		_, _ = io.WriteString(stderr, "ERROR inspect requires at most one of --id or --name\n")
		return 2
	}
	if command == "inspect" && seenFlags["id"] && *fileID == 0 {
		_, _ = io.WriteString(stderr, "ERROR inspect requires non-zero --id\n")
		return 2
	}
	if command == "inspect" && seenFlags["name"] && strings.TrimSpace(*name) == "" {
		_, _ = io.WriteString(stderr, "ERROR inspect requires non-empty --name\n")
		return 2
	}
	if command == "inspect" && anyFlagVisited(seenFlags, "out") {
		_, _ = io.WriteString(stderr, "ERROR inspect does not accept --out\n")
		return 2
	}
	if command == "inspect" && anyFlagVisited(seenFlags, "task", "focus", "max-tokens") {
		_, _ = io.WriteString(stderr, "ERROR inspect does not accept --task, --focus, or --max-tokens\n")
		return 2
	}
	if command == "get" && anyFlagVisited(seenFlags, "type") {
		_, _ = io.WriteString(stderr, "ERROR get does not accept --type\n")
		return 2
	}
	if command == "get" && strings.TrimSpace(*field) == "" {
		_, _ = io.WriteString(stderr, "ERROR get requires --field\n")
		return 2
	}
	if command == "get" && countVisitedFlags(seenFlags, "id", "name") > 1 {
		_, _ = io.WriteString(stderr, "ERROR get requires at most one of --id or --name\n")
		return 2
	}
	if command == "get" && seenFlags["id"] && *fileID == 0 {
		_, _ = io.WriteString(stderr, "ERROR get requires non-zero --id\n")
		return 2
	}
	if command == "get" && seenFlags["name"] && strings.TrimSpace(*name) == "" {
		_, _ = io.WriteString(stderr, "ERROR get requires non-empty --name\n")
		return 2
	}
	if command == "get" && anyFlagVisited(seenFlags, "out") {
		_, _ = io.WriteString(stderr, "ERROR get does not accept --out\n")
		return 2
	}
	if command == "get" && anyFlagVisited(seenFlags, "task", "focus", "max-tokens") {
		_, _ = io.WriteString(stderr, "ERROR get does not accept --task, --focus, or --max-tokens\n")
		return 2
	}
	if command == "set" && namespace != "asset" {
		_, _ = fmt.Fprintf(stderr, "ERROR set not implemented for namespace=%s\n", namespace)
		return 2
	}
	if command == "set" && anyFlagVisited(seenFlags, "name", "type", "component", "out", "task", "focus", "max-tokens") {
		_, _ = io.WriteString(stderr, "ERROR set does not accept --name, --type, --component, --out, --task, --focus, or --max-tokens\n")
		return 2
	}
	if command == "set" && strings.TrimSpace(*field) == "" {
		_, _ = io.WriteString(stderr, "ERROR set requires --field\n")
		return 2
	}
	if command == "set" && !seenFlags["value"] {
		_, _ = io.WriteString(stderr, "ERROR set requires --value\n")
		return 2
	}
	if command == "set" && seenFlags["id"] && *fileID == 0 {
		_, _ = io.WriteString(stderr, "ERROR set requires non-zero --id\n")
		return 2
	}
	if command == "index" && strings.TrimSpace(*out) == "" {
		_, _ = io.WriteString(stderr, "ERROR index requires --out\n")
		return 2
	}
	if command == "index" && samePath(file, *out) {
		_, _ = io.WriteString(stderr, "ERROR index requires --out to differ from input file\n")
		return 2
	}
	if command == "index" && anyFlagVisited(seenFlags, "id", "name", "type", "component", "field") {
		_, _ = io.WriteString(stderr, "ERROR index does not accept --id, --name, --type, --component, or --field\n")
		return 2
	}
	if command == "index" && anyFlagVisited(seenFlags, "task", "focus", "max-tokens") {
		_, _ = io.WriteString(stderr, "ERROR index does not accept --task, --focus, or --max-tokens\n")
		return 2
	}
	if command == "context-pack" && anyFlagVisited(seenFlags, "id", "name", "type", "component", "field", "out") {
		_, _ = io.WriteString(stderr, "ERROR context-pack does not accept --id, --name, --type, --component, --field, or --out\n")
		return 2
	}
	if command == "context-pack" && strings.TrimSpace(*task) == "" && strings.TrimSpace(*focus) == "" {
		_, _ = io.WriteString(stderr, "ERROR context-pack requires --focus or --task\n")
		return 2
	}
	if command == "context-pack" && *maxTokens < contextpack.MinimumBudget() {
		_, _ = fmt.Fprintf(stderr, "ERROR context-pack requires --max-tokens >= %d\n", contextpack.MinimumBudget())
		return 2
	}
	if command == "context-pack" {
		if blocks, err := parser.ParseFile(file); err == nil {
			minBudget := contextpack.MinimumBudgetForOptions(contextpack.Options{
				Namespace: namespace,
				File:      file,
				Task:      *task,
				Focus:     *focus,
				MaxTokens: *maxTokens,
			}, contextpack.NamedObjectCount(blocks))
			if *maxTokens < minBudget {
				_, _ = fmt.Fprintf(stderr, "ERROR context-pack requires --max-tokens >= %d\n", minBudget)
				return 2
			}
		}
	}

	parsedPosition := [3]float64{}
	if command == "check" {
		if namespace != "scene" {
			_, _ = fmt.Fprintf(stderr, "ERROR check not implemented for namespace=%s\n", namespace)
			return 2
		}
		if selectedView != core.ViewCompact {
			_, _ = io.WriteString(stderr, "ERROR check supports only --view compact\n")
			return 2
		}
		if strings.TrimSpace(*manifest) == "" {
			_, _ = io.WriteString(stderr, "ERROR check requires --manifest\n")
			return 2
		}
		if strings.TrimSpace(*prefab) == "" {
			_, _ = io.WriteString(stderr, "ERROR check requires --prefab\n")
			return 2
		}
		if !seenFlags["position"] {
			_, _ = io.WriteString(stderr, "ERROR check requires --position\n")
			return 2
		}
		if anyFlagVisited(seenFlags, "id", "name", "type", "component", "field", "out", "task", "focus", "max-tokens") {
			_, _ = io.WriteString(stderr, "ERROR check does not accept --id, --name, --type, --component, --field, --out, --task, --focus, or --max-tokens\n")
			return 2
		}

		var err error
		parsedPosition, err = parsePosition(*position)
		if err != nil {
			_, _ = io.WriteString(stderr, "ERROR check requires --position as x,y,z\n")
			return 2
		}
		if !positionIsFinite(parsedPosition) {
			_, _ = io.WriteString(stderr, "ERROR check requires finite --position values\n")
			return 2
		}
	}
	if command == "patch" {
		if namespace != "scene" {
			_, _ = fmt.Fprintf(stderr, "ERROR patch not implemented for namespace=%s\n", namespace)
			return 2
		}
		if selectedView != core.ViewCompact {
			_, _ = io.WriteString(stderr, "ERROR patch supports only --view compact\n")
			return 2
		}
		if strings.TrimSpace(*op) == "" {
			_, _ = io.WriteString(stderr, "ERROR patch requires --op\n")
			return 2
		}
		if *op != "place_prefab" {
			_, _ = io.WriteString(stderr, "ERROR patch supports only --op place_prefab\n")
			return 2
		}
		if strings.TrimSpace(*manifest) == "" {
			_, _ = io.WriteString(stderr, "ERROR patch requires --manifest\n")
			return 2
		}
		if strings.TrimSpace(*prefab) == "" {
			_, _ = io.WriteString(stderr, "ERROR patch requires --prefab\n")
			return 2
		}
		if !seenFlags["position"] {
			_, _ = io.WriteString(stderr, "ERROR patch requires --position\n")
			return 2
		}
		if anyFlagVisited(seenFlags, "id", "name", "type", "component", "field", "out", "task", "focus", "max-tokens") {
			_, _ = io.WriteString(stderr, "ERROR patch does not accept --id, --name, --type, --component, --field, --out, --task, --focus, or --max-tokens\n")
			return 2
		}

		var err error
		parsedPosition, err = parsePosition(*position)
		if err != nil {
			_, _ = io.WriteString(stderr, "ERROR patch requires --position as x,y,z\n")
			return 2
		}
		if !positionIsFinite(parsedPosition) {
			_, _ = io.WriteString(stderr, "ERROR patch requires finite --position values\n")
			return 2
		}
	}
	if command == "diff" {
		if namespace != "scene" {
			_, _ = fmt.Fprintf(stderr, "ERROR diff not implemented for namespace=%s\n", namespace)
			return 2
		}
		if selectedView != core.ViewCompact {
			_, _ = io.WriteString(stderr, "ERROR diff supports only --view compact\n")
			return 2
		}
		if strings.TrimSpace(*patchPath) == "" {
			_, _ = io.WriteString(stderr, "ERROR diff requires --patch\n")
			return 2
		}
		if anyFlagVisited(seenFlags, "id", "name", "type", "component", "field", "out", "task", "focus", "max-tokens", "manifest", "prefab", "position", "op", "prefab-guid") {
			_, _ = io.WriteString(stderr, "ERROR diff does not accept --id, --name, --type, --component, --field, --out, --task, --focus, --max-tokens, --manifest, --prefab, --position, --op, or --prefab-guid\n")
			return 2
		}
	}
	if command == "apply" {
		if namespace != "scene" {
			_, _ = fmt.Fprintf(stderr, "ERROR apply not implemented for namespace=%s\n", namespace)
			return 2
		}
		if selectedView != core.ViewCompact {
			_, _ = io.WriteString(stderr, "ERROR apply supports only --view compact\n")
			return 2
		}
		if strings.TrimSpace(*patchPath) == "" {
			_, _ = io.WriteString(stderr, "ERROR apply requires --patch\n")
			return 2
		}
		if anyFlagVisited(seenFlags, "id", "name", "type", "component", "field", "out", "task", "focus", "max-tokens", "manifest", "prefab", "position", "op", "prefab-guid") {
			_, _ = io.WriteString(stderr, "ERROR apply does not accept --id, --name, --type, --component, --field, --out, --task, --focus, --max-tokens, --manifest, --prefab, --position, --op, or --prefab-guid\n")
			return 2
		}
	}

	result := core.Result{
		Namespace: namespace,
		File:      file,
		View:      selectedView,
	}

	service := app.New()
	exitCode := 1

	if command == "patch" {
		patchResult, patchExitCode := service.Patch(namespace, file, selectedView, *jsonOutput, app.PatchArgs{
			Op:          *op,
			Manifest:    *manifest,
			Prefab:      *prefab,
			PrefabGUID:  *prefabGUID,
			HasPosition: seenFlags["position"],
			Position:    parsedPosition,
		})

		if *jsonOutput {
			encoder := json.NewEncoder(stdout)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(patchResult); err != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
				return 2
			}
			return patchExitCode
		}

		_, _ = io.WriteString(stdout, patchResult.Body+"\n")
		return patchExitCode
	}
	if command == "diff" {
		diffResult, diffExitCode := service.Diff(namespace, file, selectedView, *jsonOutput, app.DiffArgs{
			Patch: *patchPath,
		})

		if *jsonOutput {
			encoder := json.NewEncoder(stdout)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(diffResult); err != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
				return 2
			}
			return diffExitCode
		}

		_, _ = io.WriteString(stdout, diffResult.Body+"\n")
		return diffExitCode
	}
	if command == "apply" {
		applyResult, applyExitCode := service.Apply(namespace, file, selectedView, *jsonOutput, app.ApplyArgs{
			Patch: *patchPath,
			Write: *writeFlag,
		})

		if *jsonOutput {
			encoder := json.NewEncoder(stdout)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(applyResult); err != nil {
				_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
				return 2
			}
			return applyExitCode
		}

		_, _ = io.WriteString(stdout, applyResult.Body+"\n")
		return applyExitCode
	}

	switch command {
	case "summarize":
		result, exitCode = service.Summarize(namespace, file, selectedView, *jsonOutput)
	case "query":
		result, exitCode = service.Query(namespace, file, selectedView, *jsonOutput, app.QueryArgs{
			HasID:   seenFlags["id"],
			HasName: seenFlags["name"],
			HasType: seenFlags["type"],
			ID:      *fileID,
			Name:    *name,
			Type:    *typeName,
		})
	case "inspect":
		result, exitCode = service.Inspect(namespace, file, selectedView, *jsonOutput, app.InspectArgs{
			HasID:     seenFlags["id"],
			HasName:   seenFlags["name"],
			ID:        *fileID,
			Name:      *name,
			Component: *component,
		})
	case "get":
		result, exitCode = service.Get(namespace, file, selectedView, *jsonOutput, app.GetArgs{
			HasID:     seenFlags["id"],
			HasName:   seenFlags["name"],
			ID:        *fileID,
			Name:      *name,
			Component: *component,
			Field:     *field,
		})
	case "set":
		result, exitCode = service.Set(namespace, file, selectedView, *jsonOutput, app.SetArgs{
			HasID:    seenFlags["id"],
			HasValue: seenFlags["value"],
			ID:       *fileID,
			Field:    *field,
			Value:    *value,
			Write:    *writeFlag,
		})
	case "index":
		result, exitCode = service.Index(namespace, file, selectedView, *jsonOutput, app.IndexArgs{
			Out: *out,
		})
	case "context-pack":
		result, exitCode = service.ContextPack(namespace, file, selectedView, *jsonOutput, app.ContextPackArgs{
			Task:      *task,
			Focus:     *focus,
			MaxTokens: *maxTokens,
		})
	case "check":
		result, exitCode = service.Check(namespace, file, selectedView, *jsonOutput, app.CheckArgs{
			Manifest:    *manifest,
			Prefab:      *prefab,
			HasPosition: seenFlags["position"],
			Position:    parsedPosition,
		})
	default:
		result.Status = "ERROR"
		result.Command = command
		result.Body = notImplementedBody(namespace, command, file, selectedView)
	}

	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(result); err != nil {
			_, _ = fmt.Fprintf(stderr, "ERROR %v\n", err)
			return 2
		}
		return exitCode
	}

	_, _ = io.WriteString(stdout, result.Body+"\n")
	return exitCode
}

func notImplementedBody(namespace, command, file string, view core.View) string {
	var builder strings.Builder
	builder.WriteString("ERROR not implemented")
	builder.WriteString(" namespace=")
	builder.WriteString(namespace)
	builder.WriteString(" command=")
	builder.WriteString(command)
	builder.WriteString(" file=")
	builder.WriteString(file)
	builder.WriteString(" view=")
	builder.WriteString(string(view))
	return builder.String()
}

func parsePosition(raw string) ([3]float64, error) {
	parts := strings.Split(raw, ",")
	if len(parts) != 3 {
		return [3]float64{}, fmt.Errorf("position must contain exactly 3 comma-separated floats")
	}

	var position bounds.Vec3
	for i, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return [3]float64{}, fmt.Errorf("position value %d is empty", i)
		}
		number, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return [3]float64{}, err
		}
		position[i] = number
	}

	return [3]float64(position), nil
}

func positionIsFinite(position [3]float64) bool {
	for _, value := range position {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func visitedFlags(flagSet *flag.FlagSet) map[string]bool {
	seen := make(map[string]bool)
	flagSet.Visit(func(f *flag.Flag) {
		seen[f.Name] = true
	})
	return seen
}

func anyFlagVisited(seen map[string]bool, names ...string) bool {
	for _, name := range names {
		if seen[name] {
			return true
		}
	}
	return false
}

func countVisitedFlags(seen map[string]bool, names ...string) int {
	count := 0
	for _, name := range names {
		if seen[name] {
			count++
		}
	}
	return count
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

func isHelpArgs(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
}

func usageText() string {
	return "usage: unity-ctx <namespace> <command> <file> [flags]\n"
}
