package app_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"unity-ctx/internal/app"
	"unity-ctx/internal/contextpack"
	"unity-ctx/internal/core"
)

func TestSummarizeSceneCompact(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Summarize("scene", path, core.ViewCompact, false)
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK SCENE file=" + path + " game_objects=2 components=2 unknown=0"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}

	jsonGot, jsonCode := svc.Summarize("scene", path, core.ViewCompact, true)
	if jsonCode != 0 {
		t.Fatalf("expected json success, got code=%d body=%q", jsonCode, jsonGot.Body)
	}
	if jsonGot != got {
		t.Fatalf("json summarize mismatch: got %#v want %#v", jsonGot, got)
	}
}

func TestSummarizeSceneViewsDifferDeterministically(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()

	tiny, tinyCode := svc.Summarize("scene", path, core.ViewTiny, false)
	if tinyCode != 0 {
		t.Fatalf("expected tiny success, got code=%d body=%q", tinyCode, tiny.Body)
	}

	compact, compactCode := svc.Summarize("scene", path, core.ViewCompact, false)
	if compactCode != 0 {
		t.Fatalf("expected compact success, got code=%d body=%q", compactCode, compact.Body)
	}

	detail, detailCode := svc.Summarize("scene", path, core.ViewDetail, false)
	if detailCode != 0 {
		t.Fatalf("expected detail success, got code=%d body=%q", detailCode, detail.Body)
	}

	if tiny.Body == compact.Body {
		t.Fatalf("expected tiny and compact outputs to differ, got %q", tiny.Body)
	}

	if compact.Body == detail.Body {
		t.Fatalf("expected compact and detail outputs to differ, got %q", compact.Body)
	}

	wantTiny := "OK SCENE file=" + path + " blocks=4"
	if tiny.Body != wantTiny {
		t.Fatalf("tiny body mismatch: got %q want %q", tiny.Body, wantTiny)
	}

	wantDetail := "OK SCENE file=" + path + " game_objects=2 components=2 unknown=0 block_fileIDs=1000,1001,2000,2001"
	if detail.Body != wantDetail {
		t.Fatalf("detail body mismatch: got %q want %q", detail.Body, wantDetail)
	}
}

func TestQueryByNameAmbiguous(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "duplicate_names_scene.unity")

	svc := app.New()
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{Name: "Enemy"})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR AMBIGUOUS_NAME name=\"Enemy\" matches=2"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestQueryByIDSuccessQuotesName(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{ID: 2000})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "FOUND fileID=2000 type=GameObject name=\"Chair_01\""
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestQueryByNameSuccessQuotesNameWithSpaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "space_name_scene.unity")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!1 &4000\n" +
		"GameObject:\n" +
		"  m_Name: Boss Enemy\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{Name: "Boss Enemy"})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "FOUND fileID=4000 type=GameObject name=\"Boss Enemy\""
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestSummarizeSceneCompactUnknownDoesNotCountAsComponent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown_scene.unity")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: Root\n" +
		"--- !u!9999 &2000\n" +
		"mystery: 5\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Summarize("scene", path, core.ViewCompact, false)
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK SCENE file=" + path + " game_objects=1 components=0 unknown=1"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestInspectPrefabComponentCompact(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "prefabs", "enemy.prefab")

	svc := app.New()
	got, code := svc.Inspect("prefab", path, core.ViewCompact, false, app.InspectArgs{Component: "NavMeshAgent"})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK component=NavMeshAgent fileID=4000 fields=m_Acceleration,m_AngularSpeed,m_AutoBraking,m_GameObject,m_Speed,m_StoppingDistance"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestGetAssetField(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "assets", "enemy_config.asset")

	svc := app.New()
	got, code := svc.Get("asset", path, core.ViewCompact, false, app.GetArgs{Field: "maxHealth"})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK field=maxHealth value=200"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestGetFieldNotFound(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "assets", "enemy_config.asset")

	svc := app.New()
	got, code := svc.Get("asset", path, core.ViewCompact, false, app.GetArgs{Field: "armor"})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}

	want := "ERROR FIELD_NOT_FOUND field=armor"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestIndexReportsStaleSnapshotAndRewritesOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scene.unity")
	initialContent := "" +
		"%YAML 1.1\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: Root\n"
	if err := os.WriteFile(path, []byte(initialContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	out := filepath.Join(t.TempDir(), "scene.index.json")
	svc := app.New()

	first, firstCode := svc.Index("scene", path, core.ViewCompact, false, app.IndexArgs{Out: out})
	if firstCode != 0 {
		t.Fatalf("expected initial success, got code=%d body=%q", firstCode, first.Body)
	}

	updatedContent := initialContent +
		"--- !u!4 &2000\n" +
		"Transform:\n" +
		"  m_GameObject: {fileID: 1000}\n"
	if err := os.WriteFile(path, []byte(updatedContent), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, code := svc.Index("scene", path, core.ViewCompact, false, app.IndexArgs{Out: out})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "INDEX_STALE file=" + path + " reason=file_hash_mismatch reparse=true\n" +
		"OK index file=" + path + " out=" + out + " objects=2"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestIndexRecoversFromInvalidExistingSnapshot(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	out := filepath.Join(t.TempDir(), "broken.index.json")
	if err := os.WriteFile(out, []byte("{"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Index("scene", path, core.ViewCompact, false, app.IndexArgs{Out: out})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}
	wantPrefix := "INDEX_STALE file=" + path + " reason=invalid_snapshot reparse=true\n"
	if !strings.HasPrefix(got.Body, wantPrefix) {
		t.Fatalf("body mismatch: got %q want prefix %q", got.Body, wantPrefix)
	}
}

func TestIndexRejectsOutPathMatchingInput(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Index("scene", path, core.ViewCompact, false, app.IndexArgs{Out: path})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}
	want := "ERROR index requires --out to differ from input file"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestContextPackEmitsOmittedWhenBudgetExceeded(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.ContextPack("scene", path, core.ViewCompact, false, app.ContextPackArgs{
		Focus:     "Chair_01",
		MaxTokens: 18,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.Command != "context-pack" {
		t.Fatalf("command mismatch: got %q want %q", got.Command, "context-pack")
	}
	if !strings.Contains(got.Body, "OMITTED reason=token_budget") {
		t.Fatalf("body missing OMITTED line:\n%s", got.Body)
	}
}

func TestContextPackPrefabReflectsFocusAndTask(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "prefabs", "enemy.prefab")

	svc := app.New()
	got, code := svc.ContextPack("prefab", path, core.ViewCompact, false, app.ContextPackArgs{
		Task:      "inspect navigation setup",
		Focus:     "Enemy",
		MaxTokens: 32,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	if !strings.Contains(got.Body, `focus="Enemy"`) {
		t.Fatalf("body missing focus reflection:\n%s", got.Body)
	}
	if !strings.Contains(got.Body, `task="inspect navigation setup"`) {
		t.Fatalf("body missing task reflection:\n%s", got.Body)
	}
}

func TestContextPackAssetWorks(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "assets", "enemy_config.asset")

	svc := app.New()
	got, code := svc.ContextPack("asset", path, core.ViewCompact, false, app.ContextPackArgs{
		Focus:     "EnemyConfig",
		MaxTokens: 32,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}
	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if !strings.Contains(got.Body, "TASK_CONTEXT") && !strings.Contains(got.Body, "OMITTED reason=token_budget") {
		t.Fatalf("unexpected body:\n%s", got.Body)
	}
}

func TestContextPackRejectsTooSmallMaxTokens(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.ContextPack("scene", path, core.ViewCompact, false, app.ContextPackArgs{
		Focus:     "Table_01",
		MaxTokens: 1,
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}
	want := "ERROR context-pack requires --max-tokens >= " + strconv.Itoa(contextpack.MinimumBudget())
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestContextPackRejectsBudgetTooSmallForLargeOmittedCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "many_names.unity")
	var builder strings.Builder
	builder.WriteString("%YAML 1.1\n")
	for i := 0; i < 11; i++ {
		builder.WriteString("--- !u!1 &")
		builder.WriteString(strconv.Itoa(1000 + i))
		builder.WriteString("\nGameObject:\n  m_Name: Object_")
		builder.WriteString(strconv.Itoa(i))
		builder.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.ContextPack("scene", path, core.ViewCompact, false, app.ContextPackArgs{
		Focus:     "Object_0",
		MaxTokens: contextpack.MinimumBudget(),
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}
	want := "ERROR context-pack requires --max-tokens >= " + strconv.Itoa(contextpack.MinimumBudgetForOptions(contextpack.Options{
		Namespace: "scene",
		File:      path,
		Focus:     "Object_0",
		MaxTokens: contextpack.MinimumBudget(),
	}, 11))
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestQueryNotFoundQuotesNameWithSpaces(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{Name: "Missing Boss"})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}
	want := "ERROR NOT_FOUND name=\"Missing Boss\""
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestGetRejectsExplicitZeroIDSelector(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "assets", "enemy_config.asset")

	svc := app.New()
	got, code := svc.Get("asset", path, core.ViewCompact, false, app.GetArgs{
		HasID:     true,
		ID:        0,
		Field:     "maxHealth",
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}
	want := "ERROR inspect/get requires non-zero --id"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestAssetInspectAndGetHonorSelectorsWithoutComponent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 200\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigB\n" +
		"  maxHealth: 350\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()

	inspectByID, inspectCode := svc.Inspect("asset", path, core.ViewCompact, false, app.InspectArgs{
		HasID: true,
		ID:    11400001,
	})
	if inspectCode != 0 {
		t.Fatalf("inspect expected success, got code=%d body=%q", inspectCode, inspectByID.Body)
	}
	if !strings.Contains(inspectByID.Body, "fileID=11400001") {
		t.Fatalf("inspect body mismatch: %q", inspectByID.Body)
	}

	getByName, getCode := svc.Get("asset", path, core.ViewCompact, false, app.GetArgs{
		HasName: true,
		Name:    "ConfigB",
		Field:   "maxHealth",
	})
	if getCode != 0 {
		t.Fatalf("get expected success, got code=%d body=%q", getCode, getByName.Body)
	}
	want := "OK field=maxHealth value=350"
	if getByName.Body != want {
		t.Fatalf("get body mismatch: got %q want %q", getByName.Body, want)
	}
}

func TestInspectAmbiguousComponentForObject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ambiguous_component.prefab")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: Enemy\n" +
		"--- !u!195 &4000\n" +
		"NavMeshAgent:\n" +
		"  m_GameObject: {fileID: 1000}\n" +
		"  m_Speed: 3.5\n" +
		"--- !u!195 &4001\n" +
		"NavMeshAgent:\n" +
		"  m_GameObject: {fileID: 1000}\n" +
		"  m_Speed: 4.0\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Inspect("prefab", path, core.ViewCompact, false, app.InspectArgs{
		ID:        1000,
		Component: "NavMeshAgent",
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}

	want := "ERROR AMBIGUOUS_COMPONENT component=NavMeshAgent matches=2"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestAssetInspectComponentHonorsExplicitIDSelector(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi_object.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: EnemyGO\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyConfig\n" +
		"  maxHealth: 200\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Inspect("asset", path, core.ViewCompact, false, app.InspectArgs{
		ID:        11400000,
		Component: "GameObject",
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}

	want := "ERROR UNKNOWN_COMPONENT component=GameObject"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestAssetInspectAmbiguousComponentPreservesAmbiguity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ambiguous_component.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 200\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigB\n" +
		"  maxHealth: 300\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Inspect("asset", path, core.ViewCompact, false, app.InspectArgs{
		Component: "MonoBehaviour",
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}

	want := "ERROR AMBIGUOUS_TYPE component=MonoBehaviour matches=2"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}
