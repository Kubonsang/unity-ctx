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

	"unity-ctx/internal/app"
	"unity-ctx/internal/bounds"
	"unity-ctx/internal/contextpack"
	"unity-ctx/internal/core"
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

func TestSetAssetDryRunDoesNotWriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enemy_config.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
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

	want := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1"
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

	want := "WRITE backup=" + path + ".bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1"
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

	want := "OK field=maxHealth old=200 new=200 type_hint=int changed=0 verified=1"
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

			wantPrefix := "WRITE backup=" + path + ".bak field=label old=starter " + tc.wantBodyNew + " type_hint=string changed=1 verified=1"
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

	want := "WRITE backup=" + path + ".bak field=speed old=1.5 new=NaN type_hint=float changed=1 verified=1"
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

	want := "DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 ack_required=1\n" +
		"SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\n" +
		"PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
	}
	if got.Impact != nil {
		t.Fatalf("expected nil impact payload for non-json dry-run, got %#v", got.Impact)
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

	want := "WRITE backup=" + target + ".bak field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 verified=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1\n" +
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

	want := "OK field=moveSpeed old=3.5 new=3.5 type_hint=float changed=0 verified=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1\n" +
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
	want := "DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=WARN scenes=2 scene_refs=3 prefabs=1 prefab_refs=1 nested_depth=3 ack_required=1\n" +
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

	want := "DRY_RUN patch=" + patchPath + " op=place_prefab append_ops=2 changed=1 verified=1"
	if got.Body != want {
		t.Fatalf("body mismatch: got %q want %q", got.Body, want)
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

	want := "WRITE backup=" + scenePath + ".bak patch=" + patchPath + " op=place_prefab append_ops=2 changed=1 verified=1"
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

func TestSetAssetSupportsIDSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 100\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
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

	want := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1"
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
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_GameObject: {fileID: 1000}\n" +
		"  m_Script: {fileID: 11500000, guid: fake_enemy_controller_guid, type: 3}\n" +
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
