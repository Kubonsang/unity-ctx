package mutation

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/parser"
	"github.com/Kubonsang/unity-ctx/internal/patch"
)

func TestSceneApplyDiffSummaryReportsAppendOpsAndReservedIDs(t *testing.T) {
	envelope := mustLoadPatchFile(t, filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"))
	got, err := DescribeScenePatch(SceneApplyRequest{
		ScenePath: filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"),
		PatchPath: filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"),
		Envelope:  envelope,
	})
	if err != nil {
		t.Fatalf("DescribeScenePatch() error = %v", err)
	}
	if got.Status != patch.StatusOK {
		t.Fatalf("Status mismatch: got %q want %q", got.Status, patch.StatusOK)
	}
	if got.AppendOps != 2 {
		t.Fatalf("AppendOps mismatch: got %d want 2", got.AppendOps)
	}
	if len(got.ReservedIDs) != 2 || got.ReservedIDs[0] != 2002 || got.ReservedIDs[1] != 2003 {
		t.Fatalf("ReservedIDs mismatch: got %v want [2002 2003]", got.ReservedIDs)
	}
}

func TestSceneApplyDryRunReportsChangedWithoutWriting(t *testing.T) {
	scenePath := writeSceneCopy(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	original, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	envelope := mustLoadPatchFile(t, filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"))

	got, err := PlanSceneApply(original, SceneApplyRequest{
		ScenePath: scenePath,
		PatchPath: filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"),
		Envelope:  envelope,
	})
	if err != nil {
		t.Fatalf("PlanSceneApply() error = %v", err)
	}
	if !got.Changed {
		t.Fatal("Changed mismatch: got false want true")
	}
	if !got.Verified {
		t.Fatal("Verified mismatch: got false want true")
	}

	current, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() current error = %v", err)
	}
	if !bytes.Equal(current, original) {
		t.Fatal("dry-run mutated scene bytes")
	}
}

func TestSceneApplyWriteCreatesBackupAndPreservesParseability(t *testing.T) {
	scenePath := writeSceneCopy(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	original, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	envelope := mustLoadPatchFile(t, filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"))

	plan, err := PlanSceneApply(original, SceneApplyRequest{
		ScenePath: scenePath,
		PatchPath: filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"),
		Envelope:  envelope,
		Write:     true,
	})
	if err != nil {
		t.Fatalf("PlanSceneApply() error = %v", err)
	}

	got, err := ApplyScene(SceneApplyRequest{
		ScenePath: scenePath,
		PatchPath: filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"),
		Envelope:  envelope,
		Write:     true,
	}, plan)
	if err != nil {
		t.Fatalf("ApplyScene() error = %v", err)
	}
	if got.BackupPath != scenePath+".bak" {
		t.Fatalf("BackupPath mismatch: got %q want %q", got.BackupPath, scenePath+".bak")
	}
	if !got.Verified {
		t.Fatal("Verified mismatch: got false want true")
	}

	backup, err := os.ReadFile(scenePath + ".bak")
	if err != nil {
		t.Fatalf("ReadFile() backup error = %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatal("backup contents mismatch")
	}

	updated, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() updated error = %v", err)
	}
	blocks, err := parser.Parse(updated)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(blocks) != 6 {
		t.Fatalf("block count mismatch: got %d want %d", len(blocks), 6)
	}
	if !strings.Contains(string(updated), "--- !u!1 &2002\nGameObject:\n  m_Name: chair\n  m_IsActive: 1\n  m_Component:\n  - component: {fileID: 2003}\n") {
		t.Fatalf("updated scene missing appended GameObject block with component list:\n%s", string(updated))
	}
	if !strings.Contains(string(updated), "  m_Father: {fileID: 0}\n  m_Children: []\n") {
		t.Fatalf("updated scene missing Transform hierarchy fields:\n%s", string(updated))
	}
}

func TestSceneApplyParseVerificationFailureReturnsExplicitError(t *testing.T) {
	scenePath := writeSceneCopy(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	original, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	envelope := mustLoadPatchFile(t, filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"))

	plan, err := PlanSceneApply(original, SceneApplyRequest{
		ScenePath: scenePath,
		PatchPath: filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"),
		Envelope:  envelope,
		Write:     true,
	})
	if err != nil {
		t.Fatalf("PlanSceneApply() error = %v", err)
	}

	originalParse := parseSceneFn
	parseSceneFn = func(data []byte) ([]parser.Block, error) {
		return nil, os.ErrInvalid
	}
	defer func() {
		parseSceneFn = originalParse
	}()

	got, err := ApplyScene(SceneApplyRequest{
		ScenePath: scenePath,
		PatchPath: filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"),
		Envelope:  envelope,
		Write:     true,
	}, plan)
	if err == nil {
		t.Fatal("ApplyScene() error = nil, want verification error")
	}
	want := "APPLY_VERIFY_FAILED expected_objects=2 actual_objects=0"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
	if got.BackupPath != scenePath+".bak" {
		t.Fatalf("BackupPath mismatch: got %q want %q", got.BackupPath, scenePath+".bak")
	}
}

func TestSceneApplyRejectsUnsupportedAppendPlanContents(t *testing.T) {
	scenePath := writeSceneCopy(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"))
	original, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	envelope := mustLoadPatchFile(t, filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"))
	envelope.PatchPlan.Appends[1].TypeName = "PrefabInstance"

	_, err = PlanSceneApply(original, SceneApplyRequest{
		ScenePath: scenePath,
		PatchPath: filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json"),
		Envelope:  envelope,
	})
	if err == nil {
		t.Fatal("PlanSceneApply() error = nil, want unsupported append type error")
	}
	want := "UNSUPPORTED_APPEND type_name=PrefabInstance"
	if err.Error() != want {
		t.Fatalf("error mismatch: got %q want %q", err.Error(), want)
	}
}

func mustLoadPatchFile(t *testing.T, path string) patch.File {
	t.Helper()

	got, err := patch.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile() error = %v", err)
	}
	return got
}

func writeSceneCopy(t *testing.T, source string) string {
	t.Helper()

	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile() source error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "scene.unity")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
