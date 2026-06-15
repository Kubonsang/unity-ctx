package contextpack

import (
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/parser"
)

func TestEstimateTokensUsesUtf8BytesDiv4(t *testing.T) {
	got := EstimateTokens("12345678")
	if got != 2 {
		t.Fatalf("EstimateTokens() = %d, want 2", got)
	}
}

func TestMinimumBudgetMatchesOmittedLine(t *testing.T) {
	got := MinimumBudget()
	want := EstimateTokens("OMITTED reason=token_budget lines=1\n")
	if got != want {
		t.Fatalf("MinimumBudget() = %d, want %d", got, want)
	}
}

func TestMinimumBudgetForOptionsAccountsForReflectedHeader(t *testing.T) {
	got := MinimumBudgetForOptions(Options{
		Namespace: "scene",
		File:      "Stage01.unity",
		Focus:     "Table_01",
		Task:      "Inspect Lighting Props",
	}, 1)
	if got <= MinimumBudget() {
		t.Fatalf("MinimumBudgetForOptions() = %d, want > %d", got, MinimumBudget())
	}
}

func TestBuildEmitsOmittedWhenBudgetExceeded(t *testing.T) {
	blocks := []parser.Block{
		{
			FileID:   2000,
			TypeName: "GameObject",
			Fields: map[string]any{
				"m_Name": "Chair_01",
			},
		},
		{
			FileID:   2001,
			TypeName: "GameObject",
			Fields: map[string]any{
				"m_Name": "Lamp_01",
			},
		},
	}

	lines := Build(Options{
		Namespace: "scene",
		File:      "Stage01.unity",
		Task:      "inspect lighting props",
		Focus:     "Chair_01",
		MaxTokens: 32,
	}, blocks)

	if len(lines) == 0 {
		t.Fatal("Build() returned no lines")
	}

	if !strings.HasPrefix(lines[0], "TASK_CONTEXT ") {
		t.Fatalf("first line = %q, want TASK_CONTEXT prefix", lines[0])
	}
	if !strings.Contains(lines[0], `focus="Chair_01"`) {
		t.Fatalf("first line = %q, want reflected focus", lines[0])
	}
	if !strings.Contains(lines[0], `task="inspect lighting props"`) {
		t.Fatalf("first line = %q, want reflected task", lines[0])
	}

	body := strings.Join(lines, "\n")
	if !strings.Contains(body, "OMITTED reason=token_budget") {
		t.Fatalf("Build() body missing OMITTED line:\n%s", body)
	}

	total := 0
	for _, line := range lines {
		total += EstimateTokens(line + "\n")
	}
	if total > 32 {
		t.Fatalf("Build() exceeded budget: got %d want <= 32\n%s", total, body)
	}
}

func TestBuildHonorsBudgetWhenOnlyOmittedFits(t *testing.T) {
	blocks := []parser.Block{
		{
			FileID:   2000,
			TypeName: "GameObject",
			Fields: map[string]any{
				"m_Name": "Table_01",
			},
		},
	}

	lines := Build(Options{
		Namespace: "scene",
		File:      "Stage01.unity",
		MaxTokens: 12,
	}, blocks)

	if len(lines) != 1 {
		t.Fatalf("Build() line count = %d, want 1\n%v", len(lines), lines)
	}
	if lines[0] != "OMITTED reason=token_budget lines=2" {
		t.Fatalf("Build() first line = %q, want omitted-only output", lines[0])
	}

	total := 0
	for _, line := range lines {
		total += EstimateTokens(line + "\n")
	}
	if total > 12 {
		t.Fatalf("Build() exceeded budget: got %d want <= 12\n%v", total, lines)
	}
}

func TestBuildAtMinimumBudgetIsNeverEmpty(t *testing.T) {
	blocks := []parser.Block{
		{
			FileID:   2000,
			TypeName: "GameObject",
			Fields: map[string]any{
				"m_Name": "Table_01",
			},
		},
	}

	lines := Build(Options{
		Namespace: "scene",
		File:      "Stage01.unity",
		MaxTokens: MinimumBudget(),
	}, blocks)

	if len(lines) == 0 {
		t.Fatal("Build() returned no lines at minimum budget")
	}
	if lines[0] != "OMITTED reason=token_budget lines=2" {
		t.Fatalf("Build() first line = %q, want omitted-only output", lines[0])
	}
}

func TestBuildDoesNotTreatObjectNameAsReflectedTaskOrFocus(t *testing.T) {
	blocks := []parser.Block{
		{
			FileID:   2000,
			TypeName: "GameObject",
			Fields: map[string]any{
				"m_Name": `focus=bogus task=bogus`,
			},
		},
	}

	lines := Build(Options{
		Namespace: "scene",
		File:      "Stage01.unity",
		Focus:     "Real Focus",
		Task:      "Real Task",
		MaxTokens: 18,
	}, blocks)

	if len(lines) != 0 {
		t.Fatalf("Build() = %v, want no output when real task/focus cannot be reflected", lines)
	}
}
