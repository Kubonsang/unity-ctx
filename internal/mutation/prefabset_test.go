package mutation

import (
	"strings"
	"testing"

	"unity-ctx/internal/parser"
)

func TestPlanPrefabSetDryRunPreservesSourceAndReportsTypeHint(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyController\n" +
		"  moveSpeed: 3.5\n")

	blocks := mustParsePrefabMutationBlocks(t, input)

	result, err := PlanPrefabSet(input, blocks, PrefabSetRequest{
		Path:  "enemy.prefab",
		HasID: true,
		ID:    11400000,
		Field: "moveSpeed",
		Value: "4.0",
	})
	if err != nil {
		t.Fatalf("PlanPrefabSet() error = %v", err)
	}
	if string(result.UpdatedData) != string(input) {
		t.Fatal("dry-run plan should not write through to input bytes")
	}
	if result.TypeHint != "float" {
		t.Fatalf("TypeHint mismatch: got %q want %q", result.TypeHint, "float")
	}
	if result.OldValue != "3.5" || result.NewValue != "4.0" {
		t.Fatalf("value mismatch: got old=%q new=%q", result.OldValue, result.NewValue)
	}
	if !result.Changed {
		t.Fatal("Changed mismatch: got false want true")
	}
}

func TestPlanPrefabSetRewriteProducesUpdatedBytes(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: Enemy\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_GameObject: {fileID: 1000}\n" +
		"  health: 200\n")

	blocks := mustParsePrefabMutationBlocks(t, input)

	result, err := PlanPrefabSet(input, blocks, PrefabSetRequest{
		Path:    "enemy.prefab",
		HasID:   true,
		ID:      11400000,
		Field:   "health",
		Value:   "250",
		Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanPrefabSet() error = %v", err)
	}
	if !strings.Contains(string(result.UpdatedData), "  health: 250\n") {
		t.Fatalf("updated YAML mismatch:\n%s", string(result.UpdatedData))
	}
	if result.TypeHint != "int" {
		t.Fatalf("TypeHint mismatch: got %q want %q", result.TypeHint, "int")
	}
}

func TestPlanPrefabSetRejectsMissingFileIDTarget(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyController\n" +
		"  health: 200\n")

	blocks := mustParsePrefabMutationBlocks(t, input)

	_, err := PlanPrefabSet(input, blocks, PrefabSetRequest{
		Path:  "enemy.prefab",
		Field: "health",
		Value: "250",
	})
	if err == nil {
		t.Fatal("expected PlanPrefabSet() to reject missing fileID target")
	}
	if got := err.Error(); got != "NEED_RULE target=fileID" {
		t.Fatalf("error mismatch: got %q want %q", got, "NEED_RULE target=fileID")
	}
}

func TestPlanPrefabSetMissingFieldReturnsError(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyController\n" +
		"  health: 200\n")

	blocks := mustParsePrefabMutationBlocks(t, input)

	_, err := PlanPrefabSet(input, blocks, PrefabSetRequest{
		Path:  "enemy.prefab",
		HasID: true,
		ID:    11400000,
		Field: "armor",
		Value: "25",
	})
	if err == nil {
		t.Fatal("expected PlanPrefabSet() to reject missing field")
	}
	if got := err.Error(); got != "FIELD_NOT_FOUND field=armor" {
		t.Fatalf("error mismatch: got %q want %q", got, "FIELD_NOT_FOUND field=armor")
	}
}

func TestPlanPrefabSetMissingFileIDReturnsError(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyController\n" +
		"  health: 200\n")

	blocks := mustParsePrefabMutationBlocks(t, input)

	_, err := PlanPrefabSet(input, blocks, PrefabSetRequest{
		Path:  "enemy.prefab",
		HasID: true,
		ID:    999999,
		Field: "health",
		Value: "250",
	})
	if err == nil {
		t.Fatal("expected PlanPrefabSet() to reject missing fileID")
	}
	if got := err.Error(); got != "NOT_FOUND fileID=999999" {
		t.Fatalf("error mismatch: got %q want %q", got, "NOT_FOUND fileID=999999")
	}
}

func TestPlanPrefabSetUnchangedValueKeepsBytes(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyController\n" +
		"  health: 200\n")

	blocks := mustParsePrefabMutationBlocks(t, input)

	result, err := PlanPrefabSet(input, blocks, PrefabSetRequest{
		Path:  "enemy.prefab",
		HasID: true,
		ID:    11400000,
		Field: "health",
		Value: "200",
	})
	if err != nil {
		t.Fatalf("PlanPrefabSet() error = %v", err)
	}
	if string(result.UpdatedData) != string(input) {
		t.Fatal("unchanged update should preserve bytes")
	}
	if result.Changed {
		t.Fatal("Changed mismatch: got true want false")
	}
}

func TestPlanPrefabSetRewritePreservesCRLF(t *testing.T) {
	input := []byte(strings.Join([]string{
		"%YAML 1.1",
		"--- !u!114 &11400000",
		"MonoBehaviour:",
		"  m_Name: EnemyController",
		"  health: 200",
		"",
	}, "\r\n"))

	blocks := mustParsePrefabMutationBlocks(t, input)

	result, err := PlanPrefabSet(input, blocks, PrefabSetRequest{
		Path:    "enemy.prefab",
		HasID:   true,
		ID:      11400000,
		Field:   "health",
		Value:   "250",
		Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanPrefabSet() error = %v", err)
	}

	want := []byte(strings.Join([]string{
		"%YAML 1.1",
		"--- !u!114 &11400000",
		"MonoBehaviour:",
		"  m_Name: EnemyController",
		"  health: 250",
		"",
	}, "\r\n"))
	if string(result.UpdatedData) != string(want) {
		t.Fatalf("updated YAML mismatch:\n%q", string(result.UpdatedData))
	}
}

func mustParsePrefabMutationBlocks(t *testing.T, input []byte) []parser.Block {
	t.Helper()

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	return blocks
}
