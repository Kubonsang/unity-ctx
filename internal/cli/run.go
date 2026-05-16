package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"unity-ctx/internal/app"
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
		return 1
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
	out := flagSet.String("out", "", "")
	task := flagSet.String("task", "", "")
	focus := flagSet.String("focus", "", "")
	maxTokens := flagSet.Int("max-tokens", 256, "")

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

	if command == "query" && countQueryFlags(*fileID, *name, *typeName) != 1 {
		_, _ = io.WriteString(stderr, "ERROR query requires exactly one of --id, --name, or --type\n")
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

	result := core.Result{
		Namespace: namespace,
		File:      file,
		View:      selectedView,
	}

	service := app.New()
	exitCode := 1

	switch command {
	case "summarize":
		result, exitCode = service.Summarize(namespace, file, selectedView, *jsonOutput)
	case "query":
		result, exitCode = service.Query(namespace, file, selectedView, *jsonOutput, app.QueryArgs{
			ID:   *fileID,
			Name: *name,
			Type: *typeName,
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

func hasQueryFlags(fileID int64, name, typeName string) bool {
	return fileID != 0 || name != "" || typeName != ""
}

func countQueryFlags(fileID int64, name, typeName string) int {
	count := 0
	if fileID != 0 {
		count++
	}
	if name != "" {
		count++
	}
	if typeName != "" {
		count++
	}
	return count
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
