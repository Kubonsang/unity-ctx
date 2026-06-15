package app_test

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Kubonsang/unity-ctx/internal/app"
	"github.com/Kubonsang/unity-ctx/internal/bounds"
	"github.com/Kubonsang/unity-ctx/internal/contextpack"
	"github.com/Kubonsang/unity-ctx/internal/core"
)

type fakeScanRunner struct {
	output  []byte
	err     error
	project string
	scene   string
	prefabs []string
}

func (r *fakeScanRunner) RunEditorScan(projectPath, sceneAssetPath string, prefabPaths []string) ([]byte, error) {
	r.project = projectPath
	r.scene = sceneAssetPath
	r.prefabs = append([]string(nil), prefabPaths...)
	if r.err != nil {
		return nil, r.err
	}
	return append([]byte(nil), r.output...), nil
}

func TestScanRejectsNonSceneNamespace(t *testing.T) {
	svc := app.New()
	got, code := svc.Scan("prefab", "ignored.prefab", core.ViewCompact, false, app.ScanArgs{
		Mode:    "editor",
		Project: "/tmp/project",
		Out:     "/tmp/out.json",
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR scan not implemented for namespace=prefab"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestImpactRejectsNonPrefabNamespace(t *testing.T) {
	target := filepath.Join("..", "..", "testdata", "impact", "project", "Assets", "Prefabs", "Enemy.prefab")

	svc := app.New()
	got, code := svc.Impact("scene", target, core.ViewCompact, false, app.ImpactArgs{
		Project: filepath.Join("..", "..", "testdata", "impact", "project"),
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR impact not implemented for namespace=scene"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestImpactRejectsNonCompactView(t *testing.T) {
	target := filepath.Join("..", "..", "testdata", "impact", "project", "Assets", "Prefabs", "Enemy.prefab")

	svc := app.New()
	got, code := svc.Impact("prefab", target, core.ViewDetail, false, app.ImpactArgs{
		Project: filepath.Join("..", "..", "testdata", "impact", "project"),
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR impact supports only --view compact"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestImpactRejectsMissingProject(t *testing.T) {
	target := filepath.Join("..", "..", "testdata", "impact", "project", "Assets", "Prefabs", "Enemy.prefab")

	svc := app.New()
	got, code := svc.Impact("prefab", target, core.ViewCompact, false, app.ImpactArgs{})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR impact requires --project"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestImpactReturnsDeterministicSummary(t *testing.T) {
	project := filepath.Join("..", "..", "testdata", "impact", "project")
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")

	svc := app.New()
	got, code := svc.Impact("prefab", target, core.ViewCompact, true, app.ImpactArgs{
		Project: project,
		Scenes:  " Assets/Scenes/BossRoom.unity , Assets/Scenes/Unused.unity ",
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK prefab=Assets/Prefabs/Enemy.prefab guid=fake_enemy_guid scenes=1 scene_refs=1 prefabs=1 prefab_refs=2 nested_depth=1\nSCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000\nPREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if got.Impact == nil {
		t.Fatal("expected Impact payload for jsonOut=true")
	}
	if got.Impact.PrefabPath != "Assets/Prefabs/Enemy.prefab" {
		t.Fatalf("impact prefab path mismatch: got %q", got.Impact.PrefabPath)
	}
}

func TestImpactJSONMarshalsNestedImpactPayloadShape(t *testing.T) {
	project := filepath.Join("..", "..", "testdata", "impact", "project")
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")

	svc := app.New()
	got, code := svc.Impact("prefab", target, core.ViewCompact, true, app.ImpactArgs{
		Project: project,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	impactValue, ok := payload["impact"].(map[string]any)
	if !ok {
		t.Fatalf("impact payload missing or wrong type: %#v", payload["impact"])
	}

	if _, ok := impactValue["prefab_path"]; !ok {
		t.Fatalf("missing prefab_path key: %#v", impactValue)
	}
	if _, ok := impactValue["prefab_guid"]; !ok {
		t.Fatalf("missing prefab_guid key: %#v", impactValue)
	}
	if _, ok := impactValue["scene_hits"]; !ok {
		t.Fatalf("missing scene_hits key: %#v", impactValue)
	}
	if _, ok := impactValue["prefab_hits"]; !ok {
		t.Fatalf("missing prefab_hits key: %#v", impactValue)
	}
	if _, ok := impactValue["depth_limit_hit"]; !ok {
		t.Fatalf("missing depth_limit_hit key: %#v", impactValue)
	}
	if _, ok := impactValue["max_nested_depth"]; !ok {
		t.Fatalf("missing max_nested_depth key: %#v", impactValue)
	}
	if _, ok := impactValue["PrefabPath"]; ok {
		t.Fatalf("unexpected Go field name leaked into JSON: %#v", impactValue)
	}

	sceneHits, ok := impactValue["scene_hits"].([]any)
	if !ok || len(sceneHits) == 0 {
		t.Fatalf("scene_hits missing or empty: %#v", impactValue["scene_hits"])
	}
	firstScene, ok := sceneHits[0].(map[string]any)
	if !ok {
		t.Fatalf("first scene hit wrong type: %#v", sceneHits[0])
	}
	if _, ok := firstScene["path"]; !ok {
		t.Fatalf("scene hit missing path key: %#v", firstScene)
	}
	if _, ok := firstScene["references"]; !ok {
		t.Fatalf("scene hit missing references key: %#v", firstScene)
	}
	if _, ok := firstScene["file_ids"]; !ok {
		t.Fatalf("scene hit missing file_ids key: %#v", firstScene)
	}
	if _, ok := firstScene["FileIDs"]; ok {
		t.Fatalf("unexpected Go field name leaked into scene hit JSON: %#v", firstScene)
	}
}

func TestImpactWarnsWithDepthLimitLine(t *testing.T) {
	project := copyImpactProjectForService(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")

	writeImpactPrefabAsset(t, filepath.Join(project, "Assets", "Prefabs", "EnemyElite.prefab"), "fake_enemy_elite_guid", "fake_enemy_guid")
	writeImpactPrefabAsset(t, filepath.Join(project, "Assets", "Prefabs", "EnemyBoss.prefab"), "fake_enemy_boss_guid", "fake_enemy_elite_guid")
	writeImpactPrefabAsset(t, filepath.Join(project, "Assets", "Prefabs", "EnemyUltra.prefab"), "fake_enemy_ultra_guid", "fake_enemy_boss_guid")
	writeImpactPrefabAsset(t, filepath.Join(project, "Assets", "Prefabs", "EnemyLegend.prefab"), "fake_enemy_legend_guid", "fake_enemy_ultra_guid")

	svc := app.New()
	got, code := svc.Impact("prefab", target, core.ViewCompact, false, app.ImpactArgs{
		Project: project,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "WARN prefab=Assets/Prefabs/Enemy.prefab guid=fake_enemy_guid scenes=2 scene_refs=3 prefabs=1 prefab_refs=1 nested_depth=3\nSCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\nPREFABS Assets/Prefabs/EnemyElite.prefab refs=1 fileIDs=3000\nWARN IMPACT_DEPTH_LIMIT prefab=Assets/Prefabs/Enemy.prefab depth=3 more_possible=true"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if got.Impact == nil {
		t.Fatal("expected Impact payload on success")
	}
	if !got.Impact.DepthLimitHit {
		t.Fatal("expected depth limit warning in impact payload")
	}
}

func TestScanRejectsMissingMode(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Scan("scene", scenePath, core.ViewCompact, false, app.ScanArgs{})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR scan requires --mode"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestScanRejectsNonCompactView(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Scan("scene", scenePath, core.ViewDetail, false, app.ScanArgs{
		Mode:    "editor",
		Project: "/tmp/project",
		Out:     "/tmp/out.json",
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR scan supports only --view compact"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestScanRejectsUnsupportedMode(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Scan("scene", scenePath, core.ViewCompact, false, app.ScanArgs{Mode: "offline"})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR scan supports only --mode editor"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestScanRejectsMissingProject(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Scan("scene", scenePath, core.ViewCompact, false, app.ScanArgs{Mode: "editor"})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR scan requires --project"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestScanRejectsMissingOut(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Scan("scene", scenePath, core.ViewCompact, false, app.ScanArgs{
		Mode:    "editor",
		Project: "/tmp/project",
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR scan requires --out"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestScanRejectsSceneOutsideProjectAssets(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "Assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	scenePath := filepath.Join(t.TempDir(), "OutsideScene.unity")
	if err := os.WriteFile(scenePath, []byte("%YAML 1.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Scan("scene", scenePath, core.ViewCompact, false, app.ScanArgs{
		Mode:    "editor",
		Project: project,
		Out:     filepath.Join(t.TempDir(), "out.json"),
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR scene must be under project Assets/ file=" + scenePath + " project=" + project
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestScanWritesManifestAndReturnsDeterministicSummary(t *testing.T) {
	project := t.TempDir()
	scenePath := filepath.Join(project, "Assets", "Scenes", "SimpleScene.unity")
	if err := os.MkdirAll(filepath.Dir(scenePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scenePath, []byte("%YAML 1.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "simple_scene.bounds.json")
	runner := &fakeScanRunner{
		output: []byte(`{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [
    {"fileID": 2000, "name": "Chair_01", "center": [3, 0.5, 1], "size": [1, 1, 1]},
    {"fileID": 1000, "name": "Table_01", "center": [1, 0.5, 2], "size": [2, 1, 1]}
  ],
  "prefabs": [
    {"path": "Assets/Prefabs/table.prefab", "center": [0, 0.5, 0], "size": [2, 1, 1]},
    {"path": "Assets/Prefabs/chair.prefab", "center": [0, 0.5, 0], "size": [1, 1, 1]}
  ]
}`),
	}

	svc := app.NewWithScanRunner(runner)
	got, code := svc.Scan("scene", scenePath, core.ViewCompact, false, app.ScanArgs{
		Mode:    "editor",
		Project: project,
		Out:     outPath,
		Prefabs: " Assets/Prefabs/table.prefab , Assets/Prefabs/chair.prefab , Assets/Prefabs/table.prefab ",
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK mode=editor project=" + project + " scene=Assets/Scenes/SimpleScene.unity out=" + outPath + " objects=2 prefabs=2 source=editor"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}

	if runner.project != project {
		t.Fatalf("runner project mismatch: got %q want %q", runner.project, project)
	}
	if runner.scene != "Assets/Scenes/SimpleScene.unity" {
		t.Fatalf("runner scene mismatch: got %q want %q", runner.scene, "Assets/Scenes/SimpleScene.unity")
	}
	if !reflect.DeepEqual(runner.prefabs, []string{"Assets/Prefabs/chair.prefab", "Assets/Prefabs/table.prefab"}) {
		t.Fatalf("runner prefabs mismatch: got %#v", runner.prefabs)
	}

	manifest, err := bounds.Load(outPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if manifest.Scene != "Assets/Scenes/SimpleScene.unity" {
		t.Fatalf("manifest scene mismatch: got %q", manifest.Scene)
	}
	if manifest.Source != "editor" {
		t.Fatalf("manifest source mismatch: got %q", manifest.Source)
	}
	if len(manifest.Objects) != 2 || manifest.Objects[0].FileID != 1000 || manifest.Objects[1].FileID != 2000 {
		t.Fatalf("manifest objects mismatch: got %#v", manifest.Objects)
	}
	if len(manifest.Prefabs) != 2 || manifest.Prefabs[0].Path != "Assets/Prefabs/chair.prefab" || manifest.Prefabs[1].Path != "Assets/Prefabs/table.prefab" {
		t.Fatalf("manifest prefabs mismatch: got %#v", manifest.Prefabs)
	}
}

func TestScanRejectsPayloadSceneMismatch(t *testing.T) {
	project := t.TempDir()
	scenePath := filepath.Join(project, "Assets", "Scenes", "SimpleScene.unity")
	if err := os.MkdirAll(filepath.Dir(scenePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scenePath, []byte("%YAML 1.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &fakeScanRunner{
		output: []byte(`{
  "scene": "Assets/Scenes/OtherScene.unity",
  "objects": [],
  "prefabs": []
}`),
	}

	svc := app.NewWithScanRunner(runner)
	got, code := svc.Scan("scene", scenePath, core.ViewCompact, false, app.ScanArgs{
		Mode:    "editor",
		Project: project,
		Out:     filepath.Join(t.TempDir(), "out.json"),
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR scan payload scene mismatch requested=Assets/Scenes/SimpleScene.unity payload=Assets/Scenes/OtherScene.unity"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestScanRunnerFailureReturnsStableError(t *testing.T) {
	project := t.TempDir()
	scenePath := filepath.Join(project, "Assets", "Scenes", "SimpleScene.unity")
	if err := os.MkdirAll(filepath.Dir(scenePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scenePath, []byte("%YAML 1.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runner := &fakeScanRunner{err: errors.New("unity-cli exec failed")}
	svc := app.NewWithScanRunner(runner)
	got, code := svc.Scan("scene", scenePath, core.ViewCompact, false, app.ScanArgs{
		Mode:    "editor",
		Project: project,
		Out:     filepath.Join(t.TempDir(), "out.json"),
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR SCAN_EDITOR_FAILED project=" + project + " scene=Assets/Scenes/SimpleScene.unity err=unity-cli exec failed"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

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

func TestBenchSummarizeOnly(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Bench("scene", path, core.ViewCompact, false, app.BenchArgs{})
	if code != 0 {
		t.Fatalf("Bench() code = %d, want 0 body=%q", code, got.Body)
	}

	wantBody := expectedBenchBody(len(raw), summarizeBodyForPath(path), "")
	if got.Body != wantBody {
		t.Fatalf("Bench().Body = %q, want %q", got.Body, wantBody)
	}
	if got.Bench != nil {
		t.Fatalf("Bench().Bench = %#v, want nil for text output", got.Bench)
	}
}

func TestBenchWithTaskIncludesContextPack(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	task := "Find spawn points"

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	svc := app.New()
	rawBytes := len(raw)
	rawTokens := estimateBenchTokens(rawBytes)

	got, code := svc.Bench("scene", path, core.ViewCompact, false, app.BenchArgs{Task: "  " + task + "  "})
	if code != 0 {
		t.Fatalf("Bench() code = %d, want 0 body=%q", code, got.Body)
	}

	wantBody := expectedBenchBody(rawBytes, summarizeBodyForPath(path), simpleSceneContextPackBody(path, rawTokens))
	if got.Body != wantBody {
		t.Fatalf("Bench().Body = %q, want %q", got.Body, wantBody)
	}
}

func TestBenchWithTaskUpliftsBudgetAboveRawTokenEstimate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny_scene.unity")
	data := []byte("%YAML 1.1\n--- !u!1 &1\nGameObject:\n  m_Name: A\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rawTokens := estimateBenchTokens(len(data))
	svc := app.New()

	contextPack, contextPackCode := svc.ContextPack("scene", path, core.ViewCompact, false, app.ContextPackArgs{
		Task:      "inspect",
		MaxTokens: rawTokens,
	})
	if contextPackCode != 1 {
		t.Fatalf("ContextPack() code = %d, want 1 body=%q", contextPackCode, contextPack.Body)
	}
	wantPrefix := "ERROR context-pack requires --max-tokens >= "
	if !strings.HasPrefix(contextPack.Body, wantPrefix) {
		t.Fatalf("ContextPack().Body = %q, want prefix %q", contextPack.Body, wantPrefix)
	}

	requiredTokens, err := strconv.Atoi(strings.TrimPrefix(contextPack.Body, wantPrefix))
	if err != nil {
		t.Fatalf("Atoi(%q) error = %v", contextPack.Body, err)
	}
	if requiredTokens <= rawTokens {
		t.Fatalf("required tokens = %d, want > raw tokens %d", requiredTokens, rawTokens)
	}

	// Derive the expected context-pack body from a direct service call using the uplifted budget.
	// This avoids hardcoding the internal rendering format (e.g. exact "lines=N" count).
	upliftedPack, upliftedCode := svc.ContextPack("scene", path, core.ViewCompact, false, app.ContextPackArgs{
		Task:      "inspect",
		MaxTokens: requiredTokens,
	})
	if upliftedCode != 0 {
		t.Fatalf("ContextPack(uplifted) code = %d, want 0 body=%q", upliftedCode, upliftedPack.Body)
	}
	upliftedBody := upliftedPack.Body

	got, code := svc.Bench("scene", path, core.ViewCompact, false, app.BenchArgs{Task: "inspect"})
	if code != 0 {
		t.Fatalf("Bench() code = %d, want 0 body=%q", code, got.Body)
	}
	if !strings.Contains(got.Body, " context_pack_bytes=") {
		t.Fatalf("Bench().Body missing context_pack metrics: %q", got.Body)
	}
	if !strings.Contains(got.Body, " summarize_saved_tokens=") {
		t.Fatalf("Bench().Body missing summarize_saved_tokens: %q", got.Body)
	}
	metrics := parseBenchMetrics(t, got.Body)
	if metrics["raw_bytes"] != len(data) {
		t.Fatalf("raw_bytes = %d, want %d", metrics["raw_bytes"], len(data))
	}
	if metrics["raw_tokens"] != rawTokens {
		t.Fatalf("raw_tokens = %d, want %d", metrics["raw_tokens"], rawTokens)
	}
	if metrics["context_pack_bytes"] != len(upliftedBody) {
		t.Fatalf("context_pack_bytes = %d, want %d", metrics["context_pack_bytes"], len(upliftedBody))
	}
	if metrics["context_pack_tokens"] != estimateBenchTokens(len(upliftedBody)) {
		t.Fatalf("context_pack_tokens = %d, want %d", metrics["context_pack_tokens"], estimateBenchTokens(len(upliftedBody)))
	}

	jsonGot, jsonCode := svc.Bench("scene", path, core.ViewCompact, true, app.BenchArgs{Task: "inspect"})
	if jsonCode != 0 {
		t.Fatalf("Bench(json) code = %d, want 0 body=%q", jsonCode, jsonGot.Body)
	}
	if jsonGot.Bench == nil || jsonGot.Bench.ContextPack == nil {
		t.Fatalf("Bench(json) payload missing context pack: %#v", jsonGot.Bench)
	}
	if jsonGot.Bench.RawBytes != len(data) || jsonGot.Bench.RawTokens != rawTokens {
		t.Fatalf("Bench(json) raw metrics = %+v, want bytes=%d tokens=%d", jsonGot.Bench, len(data), rawTokens)
	}
	if jsonGot.Bench.Summarize.Bytes != len(summarizeBodyForSingleObjectPath(path)) {
		t.Fatalf("Bench(json) summarize bytes = %d, want %d", jsonGot.Bench.Summarize.Bytes, len(summarizeBodyForSingleObjectPath(path)))
	}
	if jsonGot.Bench.ContextPack.Bytes != len(upliftedBody) || jsonGot.Bench.ContextPack.Tokens != estimateBenchTokens(len(upliftedBody)) {
		t.Fatalf("Bench(json) context pack metrics = %+v, want bytes=%d tokens=%d", *jsonGot.Bench.ContextPack, len(upliftedBody), estimateBenchTokens(len(upliftedBody)))
	}
}

func TestBenchRejectsNonCompactView(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Bench("scene", path, core.ViewDetail, false, app.BenchArgs{})
	if code != 1 {
		t.Fatalf("Bench() code = %d, want 1 body=%q", code, got.Body)
	}

	want := "ERROR bench supports only --view compact"
	if got.Body != want {
		t.Fatalf("Bench().Body = %q, want %q", got.Body, want)
	}
}

func TestBenchJSONOmitsContextPackWithoutTaskAndIncludesItWithTask(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	rawBytes := len(raw)
	rawTokens := estimateBenchTokens(rawBytes)
	summarizeBytes := len(summarizeBodyForPath(path))
	summarizeTokens := estimateBenchTokens(summarizeBytes)
	contextPackBytes := len(simpleSceneContextPackBody(path, rawTokens))
	contextPackTokens := estimateBenchTokens(contextPackBytes)

	svc := app.New()

	withoutTask, withoutCode := svc.Bench("scene", path, core.ViewCompact, true, app.BenchArgs{})
	if withoutCode != 0 {
		t.Fatalf("Bench() without task code = %d, want 0 body=%q", withoutCode, withoutTask.Body)
	}
	if withoutTask.Bench == nil {
		t.Fatal("Bench() without task payload = nil, want payload")
	}

	withoutJSON, err := json.Marshal(withoutTask)
	if err != nil {
		t.Fatalf("json.Marshal() without task error = %v", err)
	}

	var withoutMap map[string]any
	if err := json.Unmarshal(withoutJSON, &withoutMap); err != nil {
		t.Fatalf("json.Unmarshal() without task error = %v", err)
	}

	withoutBench, ok := withoutMap["bench"].(map[string]any)
	if !ok {
		t.Fatalf("bench payload missing or wrong type: %#v", withoutMap["bench"])
	}
	if _, ok := withoutBench["raw_bytes"]; !ok {
		t.Fatalf("bench payload missing raw_bytes: %#v", withoutBench)
	}
	if got := jsonNumberAsInt(t, withoutBench["raw_bytes"]); got != rawBytes {
		t.Fatalf("bench.raw_bytes = %d, want %d", got, rawBytes)
	}
	if got := jsonNumberAsInt(t, withoutBench["raw_tokens"]); got != rawTokens {
		t.Fatalf("bench.raw_tokens = %d, want %d", got, rawTokens)
	}

	summarizeValue, ok := withoutBench["summarize"].(map[string]any)
	if !ok {
		t.Fatalf("bench summarize missing or wrong type: %#v", withoutBench["summarize"])
	}
	if got := jsonNumberAsInt(t, summarizeValue["bytes"]); got != summarizeBytes {
		t.Fatalf("bench.summarize.bytes = %d, want %d", got, summarizeBytes)
	}
	if got := jsonNumberAsInt(t, summarizeValue["tokens"]); got != summarizeTokens {
		t.Fatalf("bench.summarize.tokens = %d, want %d", got, summarizeTokens)
	}
	if got := jsonNumberAsFloat(t, summarizeValue["ratio"]); got != benchRatio(summarizeTokens, rawTokens) {
		t.Fatalf("bench.summarize.ratio = %v, want %v", got, benchRatio(summarizeTokens, rawTokens))
	}
	if got := jsonNumberAsInt(t, summarizeValue["saved_tokens"]); got != benchSavedTokens(rawTokens, summarizeTokens) {
		t.Fatalf("bench.summarize.saved_tokens = %d, want %d", got, benchSavedTokens(rawTokens, summarizeTokens))
	}
	if _, ok := withoutBench["context_pack"]; ok {
		t.Fatalf("bench payload unexpectedly included context_pack: %#v", withoutBench)
	}

	withTask, withCode := svc.Bench("scene", path, core.ViewCompact, true, app.BenchArgs{Task: "Find spawn points"})
	if withCode != 0 {
		t.Fatalf("Bench() with task code = %d, want 0 body=%q", withCode, withTask.Body)
	}
	if withTask.Bench == nil {
		t.Fatal("Bench() with task payload = nil, want payload")
	}

	withJSON, err := json.Marshal(withTask)
	if err != nil {
		t.Fatalf("json.Marshal() with task error = %v", err)
	}

	var withMap map[string]any
	if err := json.Unmarshal(withJSON, &withMap); err != nil {
		t.Fatalf("json.Unmarshal() with task error = %v", err)
	}

	withBench, ok := withMap["bench"].(map[string]any)
	if !ok {
		t.Fatalf("bench payload missing or wrong type: %#v", withMap["bench"])
	}
	if got := jsonNumberAsInt(t, withBench["raw_bytes"]); got != rawBytes {
		t.Fatalf("bench.raw_bytes = %d, want %d", got, rawBytes)
	}
	if got := jsonNumberAsInt(t, withBench["raw_tokens"]); got != rawTokens {
		t.Fatalf("bench.raw_tokens = %d, want %d", got, rawTokens)
	}
	contextPackValue, ok := withBench["context_pack"].(map[string]any)
	if !ok {
		t.Fatalf("bench payload missing context_pack object: %#v", withBench["context_pack"])
	}
	if got := jsonNumberAsInt(t, contextPackValue["bytes"]); got != contextPackBytes {
		t.Fatalf("bench.context_pack.bytes = %d, want %d", got, contextPackBytes)
	}
	if got := jsonNumberAsInt(t, contextPackValue["tokens"]); got != contextPackTokens {
		t.Fatalf("bench.context_pack.tokens = %d, want %d", got, contextPackTokens)
	}
	if got := jsonNumberAsFloat(t, contextPackValue["ratio"]); got != benchRatio(contextPackTokens, rawTokens) {
		t.Fatalf("bench.context_pack.ratio = %v, want %v", got, benchRatio(contextPackTokens, rawTokens))
	}
	if got := jsonNumberAsInt(t, contextPackValue["saved_tokens"]); got != benchSavedTokens(rawTokens, contextPackTokens) {
		t.Fatalf("bench.context_pack.saved_tokens = %d, want %d", got, benchSavedTokens(rawTokens, contextPackTokens))
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
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{HasName: true, Name: "Enemy"})
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
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{HasID: true, ID: 2000})
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
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{HasName: true, Name: "Boss Enemy"})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "FOUND fileID=4000 type=GameObject name=\"Boss Enemy\""
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestQueryRejectsExplicitInvalidSelectorPresence(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	tests := []struct {
		name string
		args app.QueryArgs
	}{
		{
			name: "zero id with type",
			args: app.QueryArgs{HasID: true, ID: 0, HasType: true, Type: "GameObject"},
		},
		{
			name: "empty name with type",
			args: app.QueryArgs{HasName: true, Name: "", HasType: true, Type: "GameObject"},
		},
	}

	svc := app.New()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, code := svc.Query("scene", path, core.ViewCompact, false, tc.args)
			if code != 1 {
				t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
			}

			want := "ERROR query requires exactly one of --id, --name, or --type"
			if got.Body != want {
				t.Fatalf("body mismatch: got %q want %q", got.Body, want)
			}
		})
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

func TestCheckSceneWarnsWithSortedOverlapIDs(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join(t.TempDir(), "scene.bounds.json")
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/SimpleScene.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{
				FileID: 3000,
				Name:   "ObjectC",
				Bounds: bounds.AABB{Center: bounds.Vec3{1.6, 0.5, 0.0}, Size: bounds.Vec3{1.0, 1.0, 1.0}},
			},
			{
				FileID: 1000,
				Name:   "ObjectA",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.0, 0.5, 0.0}, Size: bounds.Vec3{1.0, 1.0, 1.0}},
			},
			{
				FileID: 2000,
				Name:   "ObjectB",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.8, 0.5, 0.0}, Size: bounds.Vec3{1.0, 1.0, 1.0}},
			},
		},
		Prefabs: []bounds.PrefabBounds{
			{
				Path:   "Assets/Prefabs/chair.prefab",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.0, 0.5, 0.0}, Size: bounds.Vec3{1.2, 1.0, 1.0}},
			},
		},
	}
	if err := bounds.Save(manifestPath, manifest); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Check("scene", scenePath, core.ViewCompact, false, app.CheckArgs{
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		HasPosition: true,
		Position:    [3]float64{0.8, 0, 0},
	})
	if code != 0 {
		t.Fatalf("expected warn success, got code=%d body=%q", code, got.Body)
	}

	want := "WARN manifest=" + manifestPath + " prefab=Assets/Prefabs/chair.prefab position=0.8,0,0 overlap_ids=1000,2000,3000"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if got.Status != "WARN" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "WARN")
	}
}

func TestCheckSceneOKAndJSONMatches(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Check("scene", scenePath, core.ViewCompact, false, app.CheckArgs{
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK manifest=" + manifestPath + " prefab=Assets/Prefabs/chair.prefab position=5,0,0 overlap_ids=none"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}

	jsonGot, jsonCode := svc.Check("scene", scenePath, core.ViewCompact, true, app.CheckArgs{
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if jsonCode != 0 {
		t.Fatalf("expected json success, got code=%d body=%q", jsonCode, jsonGot.Body)
	}
	if jsonGot != got {
		t.Fatalf("json check mismatch: got %#v want %#v", jsonGot, got)
	}
}

func TestSuggestRejectsUnsupportedNamespace(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "prefabs", "enemy.prefab")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Suggest("prefab", path, core.ViewCompact, false, app.SuggestArgs{
		Manifest: manifestPath,
		Prefab:   "Assets/Prefabs/chair.prefab",
		Near:     "1000",
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR suggest not implemented for namespace=prefab"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestSuggestRejectsNonCompactView(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Suggest("scene", scenePath, core.ViewDetail, false, app.SuggestArgs{
		Manifest: manifestPath,
		Prefab:   "Assets/Prefabs/chair.prefab",
		Near:     "1000",
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR suggest supports only --view compact"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestSuggestRequiresManifestPrefabAndNear(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	tests := []struct {
		name string
		args app.SuggestArgs
		want string
	}{
		{
			name: "manifest",
			args: app.SuggestArgs{
				Prefab: "Assets/Prefabs/chair.prefab",
				Near:   "1000",
			},
			want: "ERROR suggest requires --manifest",
		},
		{
			name: "prefab",
			args: app.SuggestArgs{
				Manifest: filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json"),
				Near:     "1000",
			},
			want: "ERROR suggest requires --prefab",
		},
		{
			name: "near",
			args: app.SuggestArgs{
				Manifest: filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json"),
				Prefab:   "Assets/Prefabs/chair.prefab",
			},
			want: "ERROR suggest requires --near",
		},
	}

	svc := app.New()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, code := svc.Suggest("scene", scenePath, core.ViewCompact, false, tt.args)
			if code != 1 {
				t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
			}
			if got.Body != tt.want {
				t.Fatalf("body mismatch: got %q want %q", got.Body, tt.want)
			}
		})
	}
}

func TestSuggestCompactBodyIsDeterministicOnSimpleScene(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Suggest("scene", scenePath, core.ViewCompact, false, app.SuggestArgs{
		Manifest: manifestPath,
		Prefab:   "Assets/Prefabs/chair.prefab",
		Near:     "1000",
	})
	if code != 0 {
		t.Fatalf("expected success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.Suggest != nil {
		t.Fatalf("Suggest payload should be nil when jsonOut=false: %#v", got.Suggest)
	}

	want := "OK manifest=" + manifestPath + " prefab=Assets/Prefabs/chair.prefab near=1000 align=floor count=4 candidates=4 clear=4 warn=0\n" +
		"CANDIDATE rank=1 direction=east position=1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=2 direction=west position=-1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=3 direction=north position=0,0,1 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=4 direction=south position=0,0,-1 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestSuggestJSONIncludesNestedSuggestPayload(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Suggest("scene", scenePath, core.ViewCompact, true, app.SuggestArgs{
		Manifest: manifestPath,
		Prefab:   "Assets/Prefabs/chair.prefab",
		Near:     "1000",
	})
	if code != 0 {
		t.Fatalf("expected success exit code, got %d body=%q", code, got.Body)
	}
	if got.Suggest == nil {
		t.Fatal("expected Suggest payload for jsonOut=true")
	}

	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	suggestValue, ok := payload["suggest"].(map[string]any)
	if !ok {
		t.Fatalf("suggest payload missing or wrong type: %#v", payload["suggest"])
	}
	if suggestValue["manifest"] != manifestPath {
		t.Fatalf("suggest manifest mismatch: got %#v", suggestValue["manifest"])
	}
	if suggestValue["prefab"] != "Assets/Prefabs/chair.prefab" {
		t.Fatalf("suggest prefab mismatch: got %#v", suggestValue["prefab"])
	}

	anchorValue, ok := suggestValue["anchor"].(map[string]any)
	if !ok {
		t.Fatalf("anchor payload missing or wrong type: %#v", suggestValue["anchor"])
	}
	if anchorValue["id"] != float64(1000) {
		t.Fatalf("anchor id mismatch: got %#v", anchorValue["id"])
	}
	if anchorValue["name"] != "Table_01" {
		t.Fatalf("anchor name mismatch: got %#v", anchorValue["name"])
	}

	candidatesValue, ok := suggestValue["candidates"].([]any)
	if !ok || len(candidatesValue) != 4 {
		t.Fatalf("candidates missing or wrong count: %#v", suggestValue["candidates"])
	}
	firstCandidate, ok := candidatesValue[0].(map[string]any)
	if !ok {
		t.Fatalf("first candidate wrong type: %#v", candidatesValue[0])
	}
	if firstCandidate["direction"] != "east" {
		t.Fatalf("first candidate direction mismatch: got %#v", firstCandidate["direction"])
	}
	if firstCandidate["status"] != "OK" {
		t.Fatalf("first candidate status mismatch: got %#v", firstCandidate["status"])
	}
}

func TestSuggestWarnsWhenAllCandidatesOverlap(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join(t.TempDir(), "scene.bounds.json")
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/SimpleScene.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{
				FileID: 1000,
				Name:   "Table_01",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.0, 0.5, 0.0}, Size: bounds.Vec3{2.0, 1.0, 1.2}},
			},
			{
				FileID: 3000,
				Name:   "BlockEast",
				Bounds: bounds.AABB{Center: bounds.Vec3{1.4, 0.5, 0.0}, Size: bounds.Vec3{0.8, 1.0, 0.8}},
			},
			{
				FileID: 4000,
				Name:   "BlockWest",
				Bounds: bounds.AABB{Center: bounds.Vec3{-1.4, 0.5, 0.0}, Size: bounds.Vec3{0.8, 1.0, 0.8}},
			},
			{
				FileID: 5000,
				Name:   "BlockNorth",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.0, 0.5, 1.0}, Size: bounds.Vec3{0.8, 1.0, 0.8}},
			},
			{
				FileID: 6000,
				Name:   "BlockSouth",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.0, 0.5, -1.0}, Size: bounds.Vec3{0.8, 1.0, 0.8}},
			},
		},
		Prefabs: []bounds.PrefabBounds{
			{
				Path:   "Assets/Prefabs/chair.prefab",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.0, 0.5, 0.0}, Size: bounds.Vec3{0.8, 1.0, 0.8}},
			},
		},
	}
	if err := bounds.Save(manifestPath, manifest); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Suggest("scene", scenePath, core.ViewCompact, false, app.SuggestArgs{
		Manifest: manifestPath,
		Prefab:   "Assets/Prefabs/chair.prefab",
		Near:     "1000",
	})
	if code != 0 {
		t.Fatalf("expected warn success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "WARN" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "WARN")
	}

	want := "WARN manifest=" + manifestPath + " prefab=Assets/Prefabs/chair.prefab near=1000 align=floor count=4 candidates=4 clear=0 warn=4\n" +
		"CANDIDATE rank=1 direction=east position=1.4,0,0 status=WARN overlap_ids=3000 anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=2 direction=west position=-1.4,0,0 status=WARN overlap_ids=4000 anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=3 direction=north position=0,0,1 status=WARN overlap_ids=5000 anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=4 direction=south position=0,0,-1 status=WARN overlap_ids=6000 anchor_id=1000 anchor_name=Table_01"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestSuggestWithOutWritesPatchFileForRankOne(t *testing.T) {
	svc := app.New()
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")
	outFile := filepath.Join(t.TempDir(), "out.patch.json")

	got, code := svc.Suggest("scene", scenePath, core.ViewCompact, false, app.SuggestArgs{
		Manifest:   manifestPath,
		Prefab:     "Assets/Prefabs/chair.prefab",
		Near:       "1000",
		Count:      4,
		Align:      "floor",
		PatchOut:   outFile,
		Pick:       1,
		PrefabGUID: "guid-chair",
	})
	if code != 0 {
		t.Fatalf("expected code=0, got %d body=%s", code, got.Body)
	}
	// Suggest reports OK for the east candidate (anchor excluded from overlap check).
	// Patch may report WARN because it does not exclude the anchor from overlap checks.
	// candidate_status reflects the suggest planner result; status reflects the patch result.
	if !strings.Contains(got.Body, "PATCH_OUT rank=1 file="+outFile) {
		t.Fatalf("body missing PATCH_OUT line:\n%s", got.Body)
	}
	if !strings.Contains(got.Body, "candidate_status=OK") {
		t.Fatalf("body missing candidate_status=OK:\n%s", got.Body)
	}
	if strings.Contains(got.Body, "candidate_status=WARN") {
		t.Fatalf("unexpected candidate_status=WARN for east candidate:\n%s", got.Body)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("patch file not written: %v", err)
	}
	var pf map[string]any
	if err := json.Unmarshal(data, &pf); err != nil {
		t.Fatalf("patch file is not valid JSON: %v", err)
	}
	if pf["schema_version"].(float64) != 1 {
		t.Fatalf("unexpected schema_version: %v", pf["schema_version"])
	}
	if pf["command"] != "patch" {
		t.Fatalf("unexpected command: %v", pf["command"])
	}
	pp, ok := pf["patch_plan"].(map[string]any)
	if !ok {
		t.Fatalf("patch_plan missing or wrong type")
	}
	if pp["prefab_guid"] != "guid-chair" {
		t.Fatalf("prefab_guid mismatch: %v", pp["prefab_guid"])
	}
}

func TestSuggestWithOutNoGUIDWritesUnknownPatch(t *testing.T) {
	svc := app.New()
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")
	outFile := filepath.Join(t.TempDir(), "out.patch.json")

	got, code := svc.Suggest("scene", scenePath, core.ViewCompact, false, app.SuggestArgs{
		Manifest: manifestPath,
		Prefab:   "Assets/Prefabs/chair.prefab",
		Near:     "1000",
		Count:    4,
		Align:    "floor",
		PatchOut: outFile,
		Pick:     1,
	})
	if code != 0 {
		t.Fatalf("expected code=0, got %d body=%s", code, got.Body)
	}
	if !strings.Contains(got.Body, "PATCH_OUT rank=1 file="+outFile+" status=UNKNOWN") {
		t.Fatalf("body missing PATCH_OUT UNKNOWN line:\n%s", got.Body)
	}
	if !strings.Contains(got.Body, "candidate_status=") {
		t.Fatalf("body missing candidate_status field:\n%s", got.Body)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("patch file not written: %v", err)
	}
	var pf map[string]any
	if err := json.Unmarshal(data, &pf); err != nil {
		t.Fatalf("patch file is not valid JSON: %v", err)
	}
	if pf["status"] != "UNKNOWN" {
		t.Fatalf("expected status=UNKNOWN, got %v", pf["status"])
	}
}

func TestSuggestWithOutPickSelectsRank(t *testing.T) {
	svc := app.New()
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")
	outFile1 := filepath.Join(t.TempDir(), "rank1.patch.json")
	outFile2 := filepath.Join(t.TempDir(), "rank2.patch.json")

	_, code1 := svc.Suggest("scene", scenePath, core.ViewCompact, false, app.SuggestArgs{
		Manifest:   manifestPath,
		Prefab:     "Assets/Prefabs/chair.prefab",
		Near:       "1000",
		Count:      4,
		Align:      "floor",
		PatchOut:   outFile1,
		Pick:       1,
		PrefabGUID: "guid-chair",
	})
	if code1 != 0 {
		t.Fatalf("rank-1 failed")
	}

	got2, code2 := svc.Suggest("scene", scenePath, core.ViewCompact, false, app.SuggestArgs{
		Manifest:   manifestPath,
		Prefab:     "Assets/Prefabs/chair.prefab",
		Near:       "1000",
		Count:      4,
		Align:      "floor",
		PatchOut:   outFile2,
		Pick:       2,
		PrefabGUID: "guid-chair",
	})
	if code2 != 0 {
		t.Fatalf("rank-2 failed: %s", got2.Body)
	}
	if !strings.Contains(got2.Body, "PATCH_OUT rank=2") {
		t.Fatalf("body missing PATCH_OUT rank=2:\n%s", got2.Body)
	}

	d1, _ := os.ReadFile(outFile1)
	d2, _ := os.ReadFile(outFile2)
	if string(d1) == string(d2) {
		t.Fatal("rank-1 and rank-2 patch files must differ")
	}
}

func TestSuggestWithOutPickOutOfRangeReturnsError(t *testing.T) {
	svc := app.New()
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")
	outFile := filepath.Join(t.TempDir(), "out.patch.json")

	got, code := svc.Suggest("scene", scenePath, core.ViewCompact, false, app.SuggestArgs{
		Manifest: manifestPath,
		Prefab:   "Assets/Prefabs/chair.prefab",
		Near:     "1000",
		Count:    2,
		Align:    "floor",
		PatchOut: outFile,
		Pick:     5,
	})
	if code == 0 {
		t.Fatalf("expected non-zero code, got body=%s", got.Body)
	}
	if !strings.Contains(got.Body, "out of range") {
		t.Fatalf("expected out-of-range error, got: %s", got.Body)
	}
}

func TestSuggestWithOutPatchFailurePropagatesError(t *testing.T) {
	svc := app.New()
	outFile := filepath.Join(t.TempDir(), "out.patch.json")

	// nonexistent scene file causes s.Patch to fail
	got, code := svc.Suggest("scene", "testdata/scenes/nonexistent.unity", core.ViewCompact, false, app.SuggestArgs{
		Manifest: "testdata/manifests/simple_scene.bounds.json",
		Prefab:   "Assets/Prefabs/chair.prefab",
		Near:     "1000",
		Count:    4,
		Align:    "floor",
		PatchOut: outFile,
		Pick:     1,
	})
	if code == 0 {
		t.Fatalf("expected non-zero code, got body=%s", got.Body)
	}
	if got.Status != "ERROR" {
		t.Fatalf("expected ERROR status, got %s", got.Status)
	}
}

func TestSetAssetDryRunDoesNotWriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enemy_config.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: EnemyConfig\n" +
		"  maxHealth: 200\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Set("asset", path, core.ViewCompact, false, app.SetArgs{
		Field: "maxHealth",
		Value: "300",
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1 pre_check=OK temp_check=OK"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != content {
		t.Fatal("dry-run should not modify file")
	}
}

func TestSetAssetWriteCreatesBackupAndVerifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enemy_config.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: EnemyConfig\n" +
		"  maxHealth: 200\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Set("asset", path, core.ViewCompact, false, app.SetArgs{
		Field: "maxHealth",
		Value: "300",
		Write: true,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "WRITE backup=" + path + ".bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "  maxHealth: 300\n") {
		t.Fatalf("updated file mismatch:\n%s", string(data))
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("ReadFile(.bak) error = %v", err)
	}
	if string(backup) != content {
		t.Fatalf("backup mismatch: got %q want %q", string(backup), content)
	}
}

func TestSetAssetWriteNoOpDoesNotWriteOrCreateBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enemy_config.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: EnemyConfig\n" +
		"  maxHealth: 200\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	wantTime := time.Unix(1_700_000_000, 123_000_000)
	if err := os.Chtimes(path, wantTime, wantTime); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() before error = %v", err)
	}

	svc := app.New()
	got, code := svc.Set("asset", path, core.ViewCompact, false, app.SetArgs{
		Field: "maxHealth",
		Value: "200",
		Write: true,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK field=maxHealth old=200 new=200 type_hint=int changed=0 verified=1 pre_check=OK temp_check=OK"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}

	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() after error = %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("mtime changed: got %v want %v", after.ModTime(), before.ModTime())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != content {
		t.Fatal("no-op write should not modify file")
	}

	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("expected no backup file, got err=%v", err)
	}
}

func TestSetAssetWriteVerifiesStringValuesSemantically(t *testing.T) {
	tests := []struct {
		name        string
		initialLine string
		value       string
		wantLine    string
		wantBodyNew string
	}{
		{
			name:        "empty string",
			initialLine: "  label: starter\n",
			value:       "",
			wantLine:    "  label: \"\"\n",
			wantBodyNew: `new=""`,
		},
		{
			name:        "string looking scalar",
			initialLine: "  label: starter\n",
			value:       "001",
			wantLine:    "  label: \"001\"\n",
			wantBodyNew: `new="001"`,
		},
		{
			name:        "quoted string",
			initialLine: "  label: starter\n",
			value:       "needs:quote",
			wantLine:    "  label: \"needs:quote\"\n",
			wantBodyNew: `new="needs:quote"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "enemy_config.asset")
			content := "" +
				"%YAML 1.1\n" +
				"--- !u!114 &11400000\n" +
				"MonoBehaviour:\n" +
				"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
				"  m_Name: EnemyConfig\n" +
				tc.initialLine
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			svc := app.New()
			got, code := svc.Set("asset", path, core.ViewCompact, false, app.SetArgs{
				Field:    "label",
				Value:    tc.value,
				HasValue: true,
				Write:    true,
			})
			if code != 0 {
				t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
			}

			wantPrefix := "WRITE backup=" + path + ".bak field=label old=starter " + tc.wantBodyNew + " type_hint=string changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK"
			if got.Body != wantPrefix {
				t.Fatalf("body mismatch: got %q want %q", got.Body, wantPrefix)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if !strings.Contains(string(data), tc.wantLine) {
				t.Fatalf("updated file mismatch:\n%s", string(data))
			}
		})
	}
}

func TestSetAssetWriteVerifiesNaNSemantically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enemy_config.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: EnemyConfig\n" +
		"  speed: 1.5\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Set("asset", path, core.ViewCompact, false, app.SetArgs{
		Field:    "speed",
		Value:    "NaN",
		HasValue: true,
		Write:    true,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "WRITE backup=" + path + ".bak field=speed old=1.5 new=NaN type_hint=float changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "  speed: NaN\n") {
		t.Fatalf("updated file mismatch:\n%s", string(data))
	}

	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("ReadFile(.bak) error = %v", err)
	}
	if string(backup) != content {
		t.Fatalf("backup mismatch: got %q want %q", string(backup), content)
	}
}

func TestSetPrefabDryRunReturnsImpactSummary(t *testing.T) {
	project := copyImpactProjectForService(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetTarget(t, target, "fake_enemy_guid")

	svc := app.New()
	got, code := svc.Set("prefab", target, core.ViewCompact, false, app.SetArgs{
		HasID:   true,
		ID:      11400000,
		Field:   "moveSpeed",
		Value:   "4.0",
		Project: project,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 ack_required=1 pre_check=OK temp_check=OK\n" +
		"SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\n" +
		"PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if got.Impact != nil {
		t.Fatalf("expected nil impact payload for non-json dry-run, got %#v", got.Impact)
	}
}

func TestSetPrefabBlocksWhenPreCheckFails(t *testing.T) {
	project := copyImpactProjectForService(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	broken, err := os.ReadFile(filepath.Join("..", "..", "testdata", "broken", "duplicate_fileid.prefab"))
	if err != nil {
		t.Fatalf("ReadFile(broken fixture) error = %v", err)
	}
	if err := os.WriteFile(target, broken, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Set("prefab", target, core.ViewCompact, false, app.SetArgs{
		HasID:     true,
		ID:        1000,
		Field:     "m_IsActive",
		Value:     "0",
		Project:   project,
		Write:     true,
		AckImpact: true,
	})
	if code != 0 {
		t.Fatalf("BLOCKED must exit 0, got %d body=%q", code, got.Body)
	}
	if got.Status != "BLOCKED" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "BLOCKED")
	}

	lines := strings.Split(got.Body, "\n")
	wantFirst := "BLOCKED code=GRAPH_CHECK_FAILED phase=pre_check file=" + target + " id=1000 field=m_IsActive"
	if lines[0] != wantFirst {
		t.Fatalf("first line mismatch: got %q want %q", lines[0], wantFirst)
	}
	if len(lines) < 3 || lines[2] != "ERROR code=DUPLICATE_FILE_ID file_id=2000 duplicates=2" {
		t.Fatalf("finding line mismatch: got %q", got.Body)
	}

	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(after) != string(broken) {
		t.Fatal("blocked set modified the prefab")
	}
	if _, err := os.Stat(target + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("blocked set must not create a backup, stat err = %v", err)
	}
}

func TestSetPrefabWriteRequiresAckImpact(t *testing.T) {
	project := copyImpactProjectForService(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetTarget(t, target, "fake_enemy_guid")

	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() before error = %v", err)
	}

	svc := app.New()
	got, code := svc.Set("prefab", target, core.ViewCompact, false, app.SetArgs{
		HasID:   true,
		ID:      11400000,
		Field:   "moveSpeed",
		Value:   "4.0",
		Project: project,
		Write:   true,
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}
	if got.Body != "ERROR set requires --ack-impact for prefab writes" {
		t.Fatalf("body mismatch: got %q", got.Body)
	}

	after, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("write without ack-impact should not modify file")
	}
	if _, err := os.Stat(target + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("expected no backup file, got err=%v", err)
	}
}

func TestSetPrefabWriteCreatesBackupAndVerifies(t *testing.T) {
	project := copyImpactProjectForService(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetTarget(t, target, "fake_enemy_guid")

	svc := app.New()
	got, code := svc.Set("prefab", target, core.ViewCompact, true, app.SetArgs{
		HasID:     true,
		ID:        11400000,
		Field:     "moveSpeed",
		Value:     "4.0",
		Project:   project,
		Write:     true,
		AckImpact: true,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "WRITE backup=" + target + ".bak field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 verified=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 pre_check=OK temp_check=OK final_check=OK\n" +
		"SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\n" +
		"PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if got.Impact == nil {
		t.Fatal("expected Impact payload for jsonOut=true")
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "  moveSpeed: 4.0\n") {
		t.Fatalf("updated file mismatch:\n%s", string(data))
	}
	backup, err := os.ReadFile(target + ".bak")
	if err != nil {
		t.Fatalf("ReadFile(.bak) error = %v", err)
	}
	if !strings.Contains(string(backup), "  moveSpeed: 3.5\n") {
		t.Fatalf("backup mismatch:\n%s", string(backup))
	}
}

func TestSetPrefabWriteNoOpDoesNotCreateBackup(t *testing.T) {
	project := copyImpactProjectForService(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetTarget(t, target, "fake_enemy_guid")

	svc := app.New()
	got, code := svc.Set("prefab", target, core.ViewCompact, false, app.SetArgs{
		HasID:   true,
		ID:      11400000,
		Field:   "moveSpeed",
		Value:   "3.5",
		Project: project,
		Write:   true,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "OK field=moveSpeed old=3.5 new=3.5 type_hint=float changed=0 verified=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 pre_check=OK temp_check=OK\n" +
		"SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\n" +
		"PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if _, err := os.Stat(target + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("expected no backup file, got err=%v", err)
	}
}

func TestSetPrefabWarnsWithImpactDepthLimit(t *testing.T) {
	project := copyImpactProjectForService(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetTarget(t, target, "fake_enemy_guid")
	writeImpactPrefabAsset(t, filepath.Join(project, "Assets", "Prefabs", "EnemyElite.prefab"), "fake_enemy_elite_guid", "fake_enemy_guid")
	writeImpactPrefabAsset(t, filepath.Join(project, "Assets", "Prefabs", "EnemyBoss.prefab"), "fake_enemy_boss_guid", "fake_enemy_elite_guid")
	writeImpactPrefabAsset(t, filepath.Join(project, "Assets", "Prefabs", "EnemyUltra.prefab"), "fake_enemy_ultra_guid", "fake_enemy_boss_guid")
	writeImpactPrefabAsset(t, filepath.Join(project, "Assets", "Prefabs", "EnemyLegend.prefab"), "fake_enemy_legend_guid", "fake_enemy_ultra_guid")

	svc := app.New()
	got, code := svc.Set("prefab", target, core.ViewCompact, false, app.SetArgs{
		HasID:   true,
		ID:      11400000,
		Field:   "moveSpeed",
		Value:   "4.0",
		Project: project,
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}
	want := "DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=WARN scenes=2 scene_refs=3 prefabs=1 prefab_refs=1 nested_depth=3 ack_required=1 pre_check=OK temp_check=OK\n" +
		"SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\n" +
		"PREFABS Assets/Prefabs/EnemyElite.prefab refs=1 fileIDs=3000\n" +
		"WARN IMPACT_DEPTH_LIMIT prefab=Assets/Prefabs/Enemy.prefab depth=3 more_possible=true"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestSetRejectsUnsupportedNamespace(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Set("scene", path, core.ViewCompact, false, app.SetArgs{
		Field: "m_Name",
		Value: "Chair_02",
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}

	want := "ERROR set not implemented for namespace=scene"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestCheckSceneMissingFileReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Check("scene", path, core.ViewCompact, false, app.CheckArgs{
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR open " + path + ": no such file or directory"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestCheckSceneRejectsNonCompactView(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Check("scene", scenePath, core.ViewTiny, false, app.CheckArgs{
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR check supports only --view compact"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestCheckSceneRejectsNonFinitePosition(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Check("scene", scenePath, core.ViewCompact, false, app.CheckArgs{
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		HasPosition: true,
		Position:    [3]float64{math.NaN(), 0, 0},
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR check requires finite --position values"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestCheckSceneRejectsManifestSceneMismatch(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join(t.TempDir(), "mismatch.bounds.json")
	manifest := bounds.Manifest{
		Scene:   "Assets/Scenes/OtherScene.unity",
		Source:  "editor",
		Version: 1,
		Objects: []bounds.ObjectBounds{
			{
				FileID: 1000,
				Name:   "OtherObject",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.0, 0.5, 0.0}, Size: bounds.Vec3{1.0, 1.0, 1.0}},
			},
		},
		Prefabs: []bounds.PrefabBounds{
			{
				Path:   "Assets/Prefabs/chair.prefab",
				Bounds: bounds.AABB{Center: bounds.Vec3{0.0, 0.5, 0.0}, Size: bounds.Vec3{0.8, 1.0, 0.8}},
			},
		},
	}
	if err := bounds.Save(manifestPath, manifest); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Check("scene", scenePath, core.ViewCompact, false, app.CheckArgs{
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR manifest scene mismatch file=" + scenePath + " manifest_scene=Assets/Scenes/OtherScene.unity"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestPatchRejectsNonSceneNamespace(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "prefabs", "enemy.prefab")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Patch("prefab", path, core.ViewCompact, false, app.PatchArgs{
		Op:          "place_prefab",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		PrefabGUID:  "guid-chair",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR patch not implemented for namespace=prefab"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestPatchRejectsNonCompactView(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Patch("scene", scenePath, core.ViewDetail, false, app.PatchArgs{
		Op:          "place_prefab",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		PrefabGUID:  "guid-chair",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR patch supports only --view compact"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestPatchRejectsMissingOp(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Patch("scene", scenePath, core.ViewCompact, false, app.PatchArgs{
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		PrefabGUID:  "guid-chair",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR patch requires --op"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestPatchRejectsUnsupportedOp(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Patch("scene", scenePath, core.ViewCompact, false, app.PatchArgs{
		Op:          "move_object",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		PrefabGUID:  "guid-chair",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR patch supports only --op place_prefab"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestPatchClearPlacementReturnsOKSummaryPlusPlan(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Patch("scene", scenePath, core.ViewCompact, false, app.PatchArgs{
		Op:          "place_prefab",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		PrefabGUID:  "guid-chair",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 0 {
		t.Fatalf("expected success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.PatchPlan == nil {
		t.Fatal("PatchPlan = nil, want populated plan")
	}
	if got.PatchPlan.Status != "OK" {
		t.Fatalf("PatchPlan.Status mismatch: got %q want %q", got.PatchPlan.Status, "OK")
	}

	want := "OK op=place_prefab manifest=" + manifestPath + " prefab=Assets/Prefabs/chair.prefab position=5,0,0 overlap_ids=none reserved_fileIDs=2002,2003\n" +
		"PLAN prefab_guid=\"guid-chair\" append_ops=append:1:2002:GameObject,append:4:2003:Transform"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}

	jsonGot, jsonCode := svc.Patch("scene", scenePath, core.ViewCompact, true, app.PatchArgs{
		Op:          "place_prefab",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		PrefabGUID:  "guid-chair",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if jsonCode != 0 {
		t.Fatalf("expected json success exit code, got %d body=%q", jsonCode, jsonGot.Body)
	}
	if jsonGot.Result != got.Result {
		t.Fatalf("json patch result mismatch: got %#v want %#v", jsonGot.Result, got.Result)
	}
	if jsonGot.PatchPlan == nil || got.PatchPlan == nil {
		t.Fatalf("expected PatchPlan in both results: json=%#v text=%#v", jsonGot.PatchPlan, got.PatchPlan)
	}
	if !reflect.DeepEqual(*jsonGot.PatchPlan, *got.PatchPlan) {
		t.Fatalf("json patch plan mismatch: got %#v want %#v", *jsonGot.PatchPlan, *got.PatchPlan)
	}
}

func TestPatchOverlapPlacementReturnsWARNSummaryPlusPlan(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Patch("scene", scenePath, core.ViewCompact, false, app.PatchArgs{
		Op:          "place_prefab",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		PrefabGUID:  "guid-chair",
		HasPosition: true,
		Position:    [3]float64{2.1, 0, -1.25},
	})
	if code != 0 {
		t.Fatalf("expected warn success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "WARN" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "WARN")
	}
	if got.PatchPlan == nil {
		t.Fatal("PatchPlan = nil, want populated plan")
	}
	if got.PatchPlan.Status != "WARN" {
		t.Fatalf("PatchPlan.Status mismatch: got %q want %q", got.PatchPlan.Status, "WARN")
	}

	want := "WARN op=place_prefab manifest=" + manifestPath + " prefab=Assets/Prefabs/chair.prefab position=2.1,0,-1.25 overlap_ids=2000 reserved_fileIDs=2002,2003\n" +
		"PLAN prefab_guid=\"guid-chair\" append_ops=append:1:2002:GameObject,append:4:2003:Transform"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestPatchUnresolvedPrefabReferenceReturnsUnknown(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Patch("scene", scenePath, core.ViewCompact, false, app.PatchArgs{
		Op:          "place_prefab",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 0 {
		t.Fatalf("expected unknown success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "UNKNOWN" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "UNKNOWN")
	}
	if got.PatchPlan == nil {
		t.Fatal("PatchPlan = nil, want populated plan")
	}
	if got.PatchPlan.Reason != "NEED_PREFAB_GUID" {
		t.Fatalf("PatchPlan.Reason mismatch: got %q want %q", got.PatchPlan.Reason, "NEED_PREFAB_GUID")
	}

	want := "UNKNOWN op=place_prefab manifest=" + manifestPath + " prefab=Assets/Prefabs/chair.prefab position=5,0,0 reason=NEED_PREFAB_GUID overlap_ids=none reserved_fileIDs=2002,2003\n" +
		"PLAN prefab_guid=UNKNOWN append_ops=append:1:2002:GameObject,append:4:2003:Transform"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestPatchAutoResolvesPrefabGUIDFromMeta(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	project := t.TempDir()
	prefabRel := filepath.Join("Assets", "Prefabs", "chair.prefab")
	prefabAbs := filepath.Join(project, prefabRel)
	if err := os.MkdirAll(filepath.Dir(prefabAbs), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(prefabAbs, []byte("--- !u!1 &1000\nGameObject:\n  m_Name: chair\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := "fileFormatVersion: 2\nguid: 3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b\n"
	if err := os.WriteFile(prefabAbs+".meta", []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile(.meta) error = %v", err)
	}

	svc := app.New()
	got, code := svc.Patch("scene", scenePath, core.ViewCompact, false, app.PatchArgs{
		Op:          "place_prefab",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		Project:     project,
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 0 {
		t.Fatalf("expected success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.PatchPlan == nil || got.PatchPlan.PrefabGUID != "3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b" {
		t.Fatalf("PatchPlan guid mismatch: %#v", got.PatchPlan)
	}
	if !strings.Contains(got.Body, "PLAN prefab_guid=\"3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b\"") {
		t.Fatalf("body missing resolved guid: %q", got.Body)
	}
}

func TestPatchWithoutMetaKeepsNeedPrefabGUID(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	manifestPath := filepath.Join("..", "..", "testdata", "manifests", "simple_scene.bounds.json")

	svc := app.New()
	got, code := svc.Patch("scene", scenePath, core.ViewCompact, false, app.PatchArgs{
		Op:          "place_prefab",
		Manifest:    manifestPath,
		Prefab:      "Assets/Prefabs/chair.prefab",
		Project:     t.TempDir(),
		HasPosition: true,
		Position:    [3]float64{5, 0, 0},
	})
	if code != 0 {
		t.Fatalf("expected unknown success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "UNKNOWN" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "UNKNOWN")
	}
	if got.PatchPlan == nil || got.PatchPlan.Reason != "NEED_PREFAB_GUID" {
		t.Fatalf("expected NEED_PREFAB_GUID, got %#v", got.PatchPlan)
	}
}

func TestDiffReturnsPatchSummaryAndPlan(t *testing.T) {
	scenePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	patchPath := filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json")

	svc := app.New()
	got, code := svc.Diff("scene", scenePath, core.ViewCompact, false, app.DiffArgs{
		Patch: patchPath,
	})
	if code != 0 {
		t.Fatalf("expected success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion mismatch: got %d want %d", got.SchemaVersion, 1)
	}
	if got.PatchPlan == nil {
		t.Fatal("PatchPlan = nil, want populated plan")
	}

	want := "OK patch=" + patchPath + " op=place_prefab append_ops=2 reserved_fileIDs=2002,2003"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestApplyDryRunReturnsVerifiedSummaryWithoutWriting(t *testing.T) {
	scenePath := copyFixtureFile(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"), "simple_scene.unity")
	patchPath := filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json")

	before, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() before error = %v", err)
	}

	svc := app.New()
	got, code := svc.Apply("scene", scenePath, core.ViewCompact, false, app.ApplyArgs{
		Patch: patchPath,
	})
	if code != 0 {
		t.Fatalf("expected success exit code, got %d body=%q", code, got.Body)
	}
	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}

	want := "DRY_RUN patch=" + patchPath + " op=place_prefab append_ops=2 changed=1 verified=1 pre_check=OK temp_check=OK"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if got.Safety == nil || got.Safety.PreCheck != "OK" || got.Safety.TempCheck != "OK" || got.Safety.FinalCheck != "" {
		t.Fatalf("safety payload mismatch: %+v", got.Safety)
	}

	after, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("dry-run changed scene file bytes")
	}
}

func TestApplyWriteCreatesBackupAndWritesScene(t *testing.T) {
	scenePath := copyFixtureFile(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"), "simple_scene.unity")
	patchPath := filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json")

	svc := app.New()
	got, code := svc.Apply("scene", scenePath, core.ViewCompact, false, app.ApplyArgs{
		Patch: patchPath,
		Write: true,
	})
	if code != 0 {
		t.Fatalf("expected success exit code, got %d body=%q", code, got.Body)
	}

	want := "WRITE backup=" + scenePath + ".bak patch=" + patchPath + " op=place_prefab append_ops=2 changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}

	if _, err := os.Stat(scenePath + ".bak"); err != nil {
		t.Fatalf("backup stat error = %v", err)
	}
	updated, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() updated error = %v", err)
	}
	if !strings.Contains(string(updated), "--- !u!1 &2002\nGameObject:\n  m_Name: chair\n") {
		t.Fatalf("updated scene missing appended block:\n%s", string(updated))
	}
}

func TestApplyBlocksWhenPreCheckFails(t *testing.T) {
	scenePath := copyFixtureFile(t, filepath.Join("..", "..", "testdata", "broken", "duplicate_fileid.unity"), "simple_scene.unity")
	patchPath := filepath.Join("..", "..", "testdata", "patches", "chair_place_ok.patch.json")

	before, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() before error = %v", err)
	}

	svc := app.New()
	got, code := svc.Apply("scene", scenePath, core.ViewCompact, false, app.ApplyArgs{
		Patch: patchPath,
		Write: true,
	})
	if code != 0 {
		t.Fatalf("BLOCKED must exit 0, got %d body=%q", code, got.Body)
	}
	if got.Status != "BLOCKED" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "BLOCKED")
	}

	wantFirst := "BLOCKED code=GRAPH_CHECK_FAILED phase=pre_check patch=" + patchPath + " file=" + scenePath
	lines := strings.Split(got.Body, "\n")
	if lines[0] != wantFirst {
		t.Fatalf("first line mismatch: got %q want %q", lines[0], wantFirst)
	}
	if len(lines) < 3 {
		t.Fatalf("expected CHECK and finding lines, got %q", got.Body)
	}
	if lines[1] != "CHECK phase=pre_check status=ERROR errors=1 warnings=0" {
		t.Fatalf("CHECK line mismatch: got %q", lines[1])
	}
	if lines[2] != "ERROR code=DUPLICATE_FILE_ID file_id=1000 duplicates=2" {
		t.Fatalf("finding line mismatch: got %q", lines[2])
	}
	if got.Safety == nil || got.Safety.PreCheck != "ERROR" || len(got.Safety.Findings) != 1 {
		t.Fatalf("safety payload mismatch: %+v", got.Safety)
	}
	if f := got.Safety.Findings[0]; f.Phase != "pre_check" || f.Code != "DUPLICATE_FILE_ID" {
		t.Fatalf("safety finding mismatch: %+v", f)
	}
	if got.PatchPlan == nil {
		t.Fatal("BLOCKED result must still carry the patch plan envelope")
	}

	after, err := os.ReadFile(scenePath)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(after) != string(before) {
		t.Fatal("blocked apply modified the scene file")
	}
	if _, err := os.Stat(scenePath + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("blocked apply must not create a backup, stat err = %v", err)
	}
}

func TestApplyRejectsUnknownPatchStatus(t *testing.T) {
	scenePath := copyFixtureFile(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"), "simple_scene.unity")
	patchPath := filepath.Join("..", "..", "testdata", "patches", "chair_place_unknown.patch.json")

	svc := app.New()
	got, code := svc.Apply("scene", scenePath, core.ViewCompact, false, app.ApplyArgs{
		Patch: patchPath,
	})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d body=%q", code, got.Body)
	}

	want := "ERROR PATCH_STATUS_UNRESOLVED status=UNKNOWN reason=NEED_PREFAB_GUID"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestSetAssetBlocksWhenPreCheckFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 100\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 200\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Set("asset", path, core.ViewCompact, false, app.SetArgs{
		Field: "maxHealth",
		Value: "300",
		Write: true,
	})
	if code != 0 {
		t.Fatalf("BLOCKED must exit 0, got %d body=%q", code, got.Body)
	}
	if got.Status != "BLOCKED" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "BLOCKED")
	}
	lines := strings.Split(got.Body, "\n")
	wantFirst := "BLOCKED code=GRAPH_CHECK_FAILED phase=pre_check file=" + path + " field=maxHealth"
	if lines[0] != wantFirst {
		t.Fatalf("first line mismatch: got %q want %q", lines[0], wantFirst)
	}
	if len(lines) < 3 || lines[2] != "ERROR code=DUPLICATE_FILE_ID file_id=11400000 duplicates=2" {
		t.Fatalf("finding line mismatch: got %q", got.Body)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() after error = %v", err)
	}
	if string(after) != content {
		t.Fatal("blocked asset set modified the file")
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("blocked asset set must not create a backup, stat err = %v", err)
	}
}

func TestSetAssetSupportsIDSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 100\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: ConfigB\n" +
		"  maxHealth: 200\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	svc := app.New()
	got, code := svc.Set("asset", path, core.ViewCompact, false, app.SetArgs{
		HasID: true,
		ID:    11400001,
		Field: "maxHealth",
		Value: "300",
	})
	if code != 0 {
		t.Fatalf("expected success, got code=%d body=%q", code, got.Body)
	}

	want := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1 pre_check=OK temp_check=OK"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func copyFixtureFile(t *testing.T, source, name string) string {
	t.Helper()

	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("ReadFile() source error = %v", err)
	}

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
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

func TestIndexPropagatesIOErrorOnExistingSnapshotFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping: chmod has no effect as root")
	}

	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	out := filepath.Join(t.TempDir(), "unreadable.index.json")

	if err := os.WriteFile(out, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(out, 0o000); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(out, 0o644) })

	svc := app.New()
	_, code := svc.Index("scene", path, core.ViewCompact, false, app.IndexArgs{Out: out})
	if code != 1 {
		t.Fatalf("expected error exit code, got %d", code)
	}
}

func TestQueryIgnoresBareIDWithoutHasIDSentinel(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{ID: 2000})
	if code != 1 {
		t.Fatalf("expected error (HasID not set), got code=%d body=%q", code, got.Body)
	}
	want := "ERROR query requires exactly one of --id, --name, or --type"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestQueryIgnoresBareNameWithoutHasNameSentinel(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{Name: "Table_01"})
	if code != 1 {
		t.Fatalf("expected error (HasName not set), got code=%d body=%q", code, got.Body)
	}
	want := "ERROR query requires exactly one of --id, --name, or --type"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
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

func estimateBenchTokens(bytes int) int {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 3) / 4
}

func benchRatio(tokens int, rawTokens int) float64 {
	if rawTokens <= 0 {
		return 0
	}
	return float64(tokens) / float64(rawTokens)
}

func benchSavedTokens(rawTokens int, tokens int) int {
	saved := rawTokens - tokens
	if saved < 0 {
		return 0
	}
	return saved
}

func summarizeBodyForPath(path string) string {
	return "OK SCENE file=" + path + " game_objects=2 components=2 unknown=0"
}

func summarizeBodyForSingleObjectPath(path string) string {
	return "OK SCENE file=" + path + " game_objects=1 components=0 unknown=0"
}

func simpleSceneContextPackBody(path string, rawTokens int) string {
	return "TASK_CONTEXT scene=" + path + ` task="Find spawn points" budget=` + strconv.Itoa(rawTokens) + "tok\n" +
		`OBJECT name="Chair_01" id=2000 type="GameObject"` + "\n" +
		`OBJECT name="Table_01" id=1000 type="GameObject"`
}

func expectedBenchBody(rawBytes int, summarizeBody string, contextPackBody string) string {
	rawTokens := estimateBenchTokens(rawBytes)
	summarizeTokens := estimateBenchTokens(len(summarizeBody))

	body := "OK" +
		" raw_bytes=" + strconv.Itoa(rawBytes) +
		" raw_tokens=" + strconv.Itoa(rawTokens) +
		" summarize_bytes=" + strconv.Itoa(len(summarizeBody)) +
		" summarize_tokens=" + strconv.Itoa(summarizeTokens) +
		" summarize_ratio=" + strconv.FormatFloat(benchRatio(summarizeTokens, rawTokens), 'f', -1, 64) +
		" summarize_saved_tokens=" + strconv.Itoa(benchSavedTokens(rawTokens, summarizeTokens))
	if contextPackBody == "" {
		return body
	}

	contextPackTokens := estimateBenchTokens(len(contextPackBody))
	return body +
		" context_pack_bytes=" + strconv.Itoa(len(contextPackBody)) +
		" context_pack_tokens=" + strconv.Itoa(contextPackTokens) +
		" context_pack_ratio=" + strconv.FormatFloat(benchRatio(contextPackTokens, rawTokens), 'f', -1, 64) +
		" context_pack_saved_tokens=" + strconv.Itoa(benchSavedTokens(rawTokens, contextPackTokens))
}

func parseBenchMetrics(t *testing.T, body string) map[string]int {
	t.Helper()

	fields := strings.Fields(body)
	metrics := make(map[string]int, len(fields))
	for _, field := range fields[1:] {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.Contains(parts[1], ".") {
			continue
		}
		value, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		metrics[parts[0]] = value
	}
	return metrics
}

func jsonNumberAsInt(t *testing.T, value any) int {
	t.Helper()

	number, ok := value.(float64)
	if !ok {
		t.Fatalf("json value = %#v (%T), want number", value, value)
	}
	return int(number)
}

func jsonNumberAsFloat(t *testing.T, value any) float64 {
	t.Helper()

	number, ok := value.(float64)
	if !ok {
		t.Fatalf("json value = %#v (%T), want number", value, value)
	}
	return number
}

func TestQueryNotFoundQuotesNameWithSpaces(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	svc := app.New()
	got, code := svc.Query("scene", path, core.ViewCompact, false, app.QueryArgs{HasName: true, Name: "Missing Boss"})
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
		HasID: true,
		ID:    0,
		Field: "maxHealth",
	})
	if code != 1 {
		t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
	}
	want := "ERROR inspect/get requires non-zero --id"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
}

func TestInspectRejectsExplicitInvalidSelectorPresenceWithComponent(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "prefabs", "enemy.prefab")

	tests := []struct {
		name string
		args app.InspectArgs
		want string
	}{
		{
			name: "zero id with component",
			args: app.InspectArgs{
				HasID:     true,
				ID:        0,
				Component: "NavMeshAgent",
			},
			want: "ERROR inspect/get requires non-zero --id",
		},
		{
			name: "empty name with component",
			args: app.InspectArgs{
				HasName:   true,
				Name:      "",
				Component: "NavMeshAgent",
			},
			want: "ERROR inspect/get requires non-empty --name",
		},
	}

	svc := app.New()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, code := svc.Inspect("prefab", path, core.ViewCompact, false, tc.args)
			if code != 1 {
				t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
			}
			if got.Body != tc.want {
				t.Fatalf("body mismatch: got %q want %q", got.Body, tc.want)
			}
		})
	}
}

func TestGetRejectsExplicitInvalidSelectorPresenceWithComponent(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "assets", "enemy_config.asset")

	tests := []struct {
		name string
		args app.GetArgs
		want string
	}{
		{
			name: "zero id with component",
			args: app.GetArgs{
				HasID:     true,
				ID:        0,
				Component: "MonoBehaviour",
				Field:     "maxHealth",
			},
			want: "ERROR inspect/get requires non-zero --id",
		},
		{
			name: "empty name with component",
			args: app.GetArgs{
				HasName:   true,
				Name:      "",
				Component: "MonoBehaviour",
				Field:     "maxHealth",
			},
			want: "ERROR inspect/get requires non-empty --name",
		},
	}

	svc := app.New()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, code := svc.Get("asset", path, core.ViewCompact, false, tc.args)
			if code != 1 {
				t.Fatalf("expected error, got code=%d body=%q", code, got.Body)
			}
			if got.Body != tc.want {
				t.Fatalf("body mismatch: got %q want %q", got.Body, tc.want)
			}
		})
	}
}

func TestAssetInspectAndGetHonorSelectorsWithoutComponent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 200\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
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
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 200\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
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

func copyImpactProjectForService(t *testing.T) string {
	t.Helper()

	source := filepath.Join("..", "..", "testdata", "impact", "project")
	dest := filepath.Join(t.TempDir(), "project")
	if err := copyTreeForService(source, dest); err != nil {
		t.Fatalf("copyTreeForService() error = %v", err)
	}
	return dest
}

func copyTreeForService(source, dest string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, relative)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func writeImpactPrefabAsset(t *testing.T, prefabPath, guid, referencedGUID string) {
	t.Helper()

	prefab := "" +
		"%YAML 1.1\n" +
		"%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1001 &3000\n" +
		"PrefabInstance:\n" +
		"  m_SourcePrefab: {fileID: 100100000, guid: " + referencedGUID + ", type: 3}\n"
	if err := os.WriteFile(prefabPath, []byte(prefab), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", prefabPath, err)
	}

	meta := "" +
		"fileFormatVersion: 2\n" +
		"guid: " + guid + "\n" +
		"PrefabImporter:\n" +
		"  externalObjects: {}\n"
	if err := os.WriteFile(prefabPath+".meta", []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile(%s.meta) error = %v", prefabPath, err)
	}
}

func writePrefabSetTarget(t *testing.T, prefabPath, guid string) {
	t.Helper()

	prefab := "" +
		"%YAML 1.1\n" +
		"%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: Enemy\n" +
		"  m_Component:\n" +
		"  - component: {fileID: 4000}\n" +
		"  - component: {fileID: 11400000}\n" +
		"--- !u!4 &4000\n" +
		"Transform:\n" +
		"  m_GameObject: {fileID: 1000}\n" +
		"  m_Father: {fileID: 0}\n" +
		"  m_Children: []\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_GameObject: {fileID: 1000}\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  moveSpeed: 3.5\n"
	if err := os.WriteFile(prefabPath, []byte(prefab), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", prefabPath, err)
	}

	meta := "" +
		"fileFormatVersion: 2\n" +
		"guid: " + guid + "\n" +
		"PrefabImporter:\n" +
		"  externalObjects: {}\n"
	if err := os.WriteFile(prefabPath+".meta", []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile(%s.meta) error = %v", prefabPath, err)
	}
}
