package contextpack

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Kubonsang/unity-ctx/internal/parser"
)

type Options struct {
	Namespace string
	File      string
	Task      string
	Focus     string
	MaxTokens int
}

func EstimateTokens(text string) int {
	return (len([]byte(text)) + 3) / 4
}

func MinimumBudget() int {
	return EstimateTokens("OMITTED reason=token_budget lines=1\n")
}

func MinimumBudgetForLines(lineCount int) int {
	if lineCount < 1 {
		lineCount = 1
	}
	return EstimateTokens(fmt.Sprintf("OMITTED reason=token_budget lines=%d\n", lineCount))
}

func NamedObjectCount(blocks []parser.Block) int {
	count := 0
	for _, block := range blocks {
		name, ok := block.Fields["m_Name"].(string)
		if ok && name != "" {
			count++
		}
	}
	return count
}

func MinimumBudgetForOptions(opts Options, namedObjectCount int) int {
	minBudget := MinimumBudgetForLines(1 + namedObjectCount)
	if opts.Focus == "" && opts.Task == "" {
		return minBudget
	}

	omittedTokens := 0
	if namedObjectCount > 0 {
		omittedTokens = EstimateTokens(fmt.Sprintf("OMITTED reason=token_budget lines=%d\n", namedObjectCount))
	}

	best := 0
	for _, candidate := range taskContextCandidates(opts) {
		if !reflectsContextInputs([]string{candidate}, opts) {
			continue
		}
		tokens := EstimateTokens(candidate + "\n")
		if namedObjectCount > 0 {
			tokens += omittedTokens
		}
		if best == 0 || tokens < best {
			best = tokens
		}
	}
	if best > minBudget {
		return best
	}
	return minBudget
}

func Build(opts Options, blocks []parser.Block) []string {
	objectLines := namedObjectLines(blocks)
	header := selectTaskContextLine(opts, objectLines)
	if header == "" && (opts.Focus != "" || opts.Task != "") {
		return nil
	}
	lines := make([]string, 0, len(objectLines)+1)
	if header != "" {
		lines = append(lines, header)
	}
	lines = append(lines, objectLines...)
	return enforceBudget(lines, opts.MaxTokens)
}

func selectTaskContextLine(opts Options, objectLines []string) string {
	candidates := taskContextCandidates(opts)
	if len(candidates) == 0 {
		return ""
	}

	fallback := ""
	for _, candidate := range candidates {
		lines := append([]string{candidate}, objectLines...)
		final := enforceBudget(lines, opts.MaxTokens)
		if len(final) == 0 {
			continue
		}
		if fallback == "" {
			fallback = candidate
		}
		if reflectsContextInputs(final, opts) {
			return candidate
		}
	}

	if opts.Focus != "" || opts.Task != "" {
		return ""
	}
	return fallback
}

func taskContextCandidates(opts Options) []string {
	base := taskContextBase(opts)
	focus := ""
	if opts.Focus != "" {
		focus = "focus=" + strconv.Quote(opts.Focus)
	}
	task := ""
	if opts.Task != "" {
		task = "task=" + strconv.Quote(opts.Task)
	}
	budget := ""
	if opts.MaxTokens > 0 {
		budget = fmt.Sprintf("budget=%dtok", opts.MaxTokens)
	}

	var combos [][]string
	if base != "" || focus != "" || task != "" || budget != "" {
		combos = append(combos,
			[]string{base, focus, task, budget},
			[]string{base, focus, task},
			[]string{base, focus, budget},
			[]string{base, focus},
			[]string{base, task, budget},
			[]string{base, task},
			[]string{focus, task, budget},
			[]string{focus, task},
			[]string{focus},
			[]string{task},
			[]string{base, budget},
			[]string{base},
			nil,
		)
	}

	seen := make(map[string]struct{}, len(combos))
	candidates := make([]string, 0, len(combos))
	for _, parts := range combos {
		line := "TASK_CONTEXT"
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			if part != "" {
				filtered = append(filtered, part)
			}
		}
		if len(filtered) > 0 {
			line += " " + strings.Join(filtered, " ")
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		candidates = append(candidates, line)
	}

	if len(candidates) == 0 {
		return []string{"TASK_CONTEXT"}
	}
	return candidates
}

func taskContextBase(opts Options) string {
	if opts.Namespace != "" && opts.File != "" {
		return opts.Namespace + "=" + opts.File
	}
	if opts.File != "" {
		return "file=" + opts.File
	}
	if opts.Namespace != "" {
		return opts.Namespace
	}
	return ""
}

func reflectsContextInputs(lines []string, opts Options) bool {
	body := strings.Join(lines, "\n")
	if opts.Focus != "" && !strings.Contains(body, "focus="+strconv.Quote(opts.Focus)) {
		return false
	}
	if opts.Task != "" && !strings.Contains(body, "task="+strconv.Quote(opts.Task)) {
		return false
	}
	return true
}

func namedObjectLines(blocks []parser.Block) []string {
	type namedBlock struct {
		name  string
		id    int64
		typ   string
		order int
	}

	named := make([]namedBlock, 0, len(blocks))
	for i, block := range blocks {
		name, ok := block.Fields["m_Name"].(string)
		if !ok || name == "" {
			continue
		}
		named = append(named, namedBlock{
			name:  name,
			id:    block.FileID,
			typ:   block.TypeName,
			order: i,
		})
	}

	sort.Slice(named, func(i, j int) bool {
		if named[i].name != named[j].name {
			return named[i].name < named[j].name
		}
		if named[i].id != named[j].id {
			return named[i].id < named[j].id
		}
		if named[i].typ != named[j].typ {
			return named[i].typ < named[j].typ
		}
		return named[i].order < named[j].order
	})

	lines := make([]string, 0, len(named))
	for _, block := range named {
		lines = append(lines, fmt.Sprintf(
			"OBJECT name=%s id=%d type=%s",
			strconv.Quote(block.name),
			block.id,
			strconv.Quote(block.typ),
		))
	}

	return lines
}

func enforceBudget(lines []string, maxTokens int) []string {
	if len(lines) == 0 || maxTokens <= 0 {
		return nil
	}

	kept := make([]string, 0, len(lines))
	used := 0
	omitted := 0

	for _, line := range lines {
		lineTokens := EstimateTokens(line + "\n")
		if used+lineTokens > maxTokens {
			omitted++
			continue
		}
		kept = append(kept, line)
		used += lineTokens
	}

	if omitted == 0 {
		return kept
	}

	for {
		omittedLine := fmt.Sprintf("OMITTED reason=token_budget lines=%d", omitted)
		omittedTokens := EstimateTokens(omittedLine + "\n")
		if used+omittedTokens <= maxTokens {
			return append(kept, omittedLine)
		}
		if len(kept) == 0 {
			return nil
		}

		last := kept[len(kept)-1]
		kept = kept[:len(kept)-1]
		used -= EstimateTokens(last + "\n")
		omitted++
	}
}
