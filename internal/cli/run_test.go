package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"unity-ctx/internal/bounds"
	"unity-ctx/internal/contextpack"
)

func TestMainMissingFileArgument(t *testing.T) {
	result := runCLI(t, "scene", "summarize")

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR missing file argument\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestHelp(t *testing.T) {
	result := runCLI(t, "--help")

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}

	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "usage: unity-ctx <namespace> <command> <file> [flags]\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSummarizeJSONReturnsResultEnvelope(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"summarize",
		"testdata/scenes/simple_scene.unity",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}

	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.Namespace != "scene" {
		t.Fatalf("namespace mismatch: got %q want %q", got.Namespace, "scene")
	}
	if got.Command != "summarize" {
		t.Fatalf("command mismatch: got %q want %q", got.Command, "summarize")
	}
	if got.File != "testdata/scenes/simple_scene.unity" {
		t.Fatalf("file mismatch: got %q want %q", got.File, "testdata/scenes/simple_scene.unity")
	}
	if got.View != "compact" {
		t.Fatalf("view mismatch: got %q want %q", got.View, "compact")
	}
	wantBody := "OK SCENE file=testdata/scenes/simple_scene.unity game_objects=2 components=2 unknown=0"
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
	}
}

func TestRejectsInvalidView(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"summarize",
		"testdata/scenes/simple_scene.unity",
		"--view",
		"wide",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR invalid view \"wide\"\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestRejectsUnexpectedTrailingArgs(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"summarize",
		"testdata/scenes/simple_scene.unity",
		"extra",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR unexpected trailing arguments: extra\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestRejectsIrrelevantFlagsForSummarize(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"summarize",
		"testdata/scenes/simple_scene.unity",
		"--name",
		"Enemy",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR summarize does not accept --id, --name, or --type\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestQueryByIDReturnsFoundResult(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"query",
		"testdata/scenes/simple_scene.unity",
		"--id",
		"2000",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}

	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "FOUND fileID=2000 type=GameObject name=\"Chair_01\"\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestQueryRejectsInvalidSelectorCombination(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"query",
		"testdata/scenes/simple_scene.unity",
		"--id",
		"2000",
		"--name",
		"Chair_01",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR query requires exactly one of --id, --name, or --type\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestQueryRejectsZeroIDWhenAnotherSelectorIsPresent(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"query",
		"testdata/scenes/simple_scene.unity",
		"--id",
		"0",
		"--type",
		"GameObject",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR query requires exactly one of --id, --name, or --type\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestQueryRejectsEmptyNameWhenAnotherSelectorIsPresent(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"query",
		"testdata/scenes/simple_scene.unity",
		"--name",
		"",
		"--type",
		"GameObject",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR query requires exactly one of --id, --name, or --type\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestQueryNotFoundQuotesNameWithSpaces(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"query",
		"testdata/scenes/simple_scene.unity",
		"--name",
		"Missing Boss",
	)

	if result.exitCode != 1 {
		t.Fatalf("exit code mismatch: got %d want 1", result.exitCode)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}
	want := "ERROR NOT_FOUND name=\"Missing Boss\"\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestQueryJSONRejectsInvalidSelectorCombination(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"query",
		"testdata/scenes/simple_scene.unity",
		"--id",
		"2000",
		"--name",
		"Chair_01",
		"--json",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR query requires exactly one of --id, --name, or --type\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestQueryJSONReturnsResultEnvelope(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"query",
		"testdata/scenes/simple_scene.unity",
		"--id",
		"2000",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}

	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		SchemaVersion int    `json:"schema_version"`
		Status        string `json:"status"`
		Namespace     string `json:"namespace"`
		Command       string `json:"command"`
		File          string `json:"file"`
		View          string `json:"view"`
		Body          string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "FOUND" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "FOUND")
	}
	if got.Namespace != "scene" {
		t.Fatalf("namespace mismatch: got %q want %q", got.Namespace, "scene")
	}
	if got.Command != "query" {
		t.Fatalf("command mismatch: got %q want %q", got.Command, "query")
	}
	if got.File != "testdata/scenes/simple_scene.unity" {
		t.Fatalf("file mismatch: got %q want %q", got.File, "testdata/scenes/simple_scene.unity")
	}
	if got.View != "compact" {
		t.Fatalf("view mismatch: got %q want %q", got.View, "compact")
	}
	wantBody := "FOUND fileID=2000 type=GameObject name=\"Chair_01\""
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
	}
}

func TestInspectReturnsComponentFields(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"inspect",
		"testdata/prefabs/enemy.prefab",
		"--component",
		"NavMeshAgent",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}

	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "OK component=NavMeshAgent fileID=4000 fields=m_Acceleration,m_AngularSpeed,m_AutoBraking,m_GameObject,m_Speed,m_StoppingDistance\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestGetReturnsAssetField(t *testing.T) {
	result := runCLI(
		t,
		"asset",
		"get",
		"testdata/assets/enemy_config.asset",
		"--field",
		"maxHealth",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}

	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "OK field=maxHealth value=200\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSceneCheckWarnsWithSortedOverlapIDs(t *testing.T) {
	scenePath := "testdata/scenes/simple_scene.unity"
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

	result := runCLI(
		t,
		"scene",
		"check",
		scenePath,
		"--manifest",
		manifestPath,
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"0.8,0,0",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "WARN manifest=" + manifestPath + " prefab=Assets/Prefabs/chair.prefab position=0.8,0,0 overlap_ids=1000,2000,3000\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSceneCheckJSONReturnsResultEnvelope(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"check",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"5,0,0",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.Namespace != "scene" {
		t.Fatalf("namespace mismatch: got %q want %q", got.Namespace, "scene")
	}
	if got.Command != "check" {
		t.Fatalf("command mismatch: got %q want %q", got.Command, "check")
	}
	if got.File != "testdata/scenes/simple_scene.unity" {
		t.Fatalf("file mismatch: got %q want %q", got.File, "testdata/scenes/simple_scene.unity")
	}
	if got.View != "compact" {
		t.Fatalf("view mismatch: got %q want %q", got.View, "compact")
	}
	wantBody := "OK manifest=testdata/manifests/simple_scene.bounds.json prefab=Assets/Prefabs/chair.prefab position=5,0,0 overlap_ids=none"
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
	}
}

func TestSceneCheckRequiresStrictPositionFormat(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"check",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"1,2",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR check requires --position as x,y,z\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneCheckRejectsIrrelevantFlags(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"check",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"5,0,0",
		"--write",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR check does not accept --write\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneCheckMissingFileReturnsError(t *testing.T) {
	scenePath := filepath.Join(t.TempDir(), "missing_scene.unity")

	result := runCLI(
		t,
		"scene",
		"check",
		scenePath,
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"5,0,0",
	)

	if result.exitCode != 1 {
		t.Fatalf("exit code mismatch: got %d want 1 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "ERROR open " + scenePath + ": no such file or directory\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSceneCheckRejectsNonCompactView(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"check",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"5,0,0",
		"--view",
		"tiny",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR check supports only --view compact\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneCheckRejectsNonFinitePosition(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"check",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"NaN,0,0",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR check requires finite --position values\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneCheckRejectsManifestSceneMismatch(t *testing.T) {
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

	scenePath := "testdata/scenes/simple_scene.unity"
	result := runCLI(
		t,
		"scene",
		"check",
		scenePath,
		"--manifest",
		manifestPath,
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"5,0,0",
	)

	if result.exitCode != 1 {
		t.Fatalf("exit code mismatch: got %d want 1 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "ERROR manifest scene mismatch file=" + scenePath + " manifest_scene=Assets/Scenes/OtherScene.unity\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestScenePatchRequiresOp(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR patch requires --op\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestScenePatchRejectsUnsupportedOp(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--op",
		"move_object",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR patch supports only --op place_prefab\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestScenePatchRequiresManifest(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--op",
		"place_prefab",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR patch requires --manifest\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestScenePatchRequiresPrefab(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--op",
		"place_prefab",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR patch requires --prefab\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestScenePatchRequiresPosition(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--op",
		"place_prefab",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR patch requires --position\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestScenePatchRejectsWrite(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--write",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR patch does not accept --write\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestScenePatchRejectsSelectorFlags(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--op",
		"place_prefab",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"5,0,0",
		"--id",
		"2000",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR patch does not accept --id, --name, --type, --component, --field, --out, --task, --focus, or --max-tokens\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestScenePatchCompactOutputMatchesExpectedText(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--op",
		"place_prefab",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--position",
		"5,0,0",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "UNKNOWN op=place_prefab manifest=testdata/manifests/simple_scene.bounds.json prefab=Assets/Prefabs/chair.prefab position=5,0,0 reason=NEED_PREFAB_GUID overlap_ids=none reserved_fileIDs=2002,2003\n" +
		"PLAN prefab_guid=UNKNOWN append_ops=append:1:2002:GameObject,append:4:2003:Transform\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestScenePatchWithPrefabGUIDReturnsOK(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--op",
		"place_prefab",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--prefab-guid",
		"guid-chair",
		"--position",
		"5,0,0",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "OK op=place_prefab manifest=testdata/manifests/simple_scene.bounds.json prefab=Assets/Prefabs/chair.prefab position=5,0,0 overlap_ids=none reserved_fileIDs=2002,2003\n" +
		"PLAN prefab_guid=\"guid-chair\" append_ops=append:1:2002:GameObject,append:4:2003:Transform\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestScenePatchJSONReturnsDeterministicEnvelopeWithPatchPlan(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"patch",
		"testdata/scenes/simple_scene.unity",
		"--op",
		"place_prefab",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--prefab-guid",
		"guid-chair",
		"--position",
		"5,0,0",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		SchemaVersion int    `json:"schema_version"`
		Status        string `json:"status"`
		Namespace     string `json:"namespace"`
		Command       string `json:"command"`
		File          string `json:"file"`
		View          string `json:"view"`
		Body          string `json:"body"`
		PatchPlan     *struct {
			Status          string     `json:"status"`
			Reason          string     `json:"reason"`
			PrefabPath      string     `json:"prefab_path"`
			PrefabGUID      string     `json:"prefab_guid"`
			OverlapIDs      []int64    `json:"overlap_ids"`
			ReservedFileIDs []int64    `json:"reserved_file_ids"`
			Position        [3]float64 `json:"position"`
			Appends         []struct {
				Op       string `json:"op"`
				ClassID  int    `json:"class_id"`
				FileID   int64  `json:"file_id"`
				TypeName string `json:"type_name"`
			} `json:"appends"`
		} `json:"patch_plan"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schema_version mismatch: got %d want %d", got.SchemaVersion, 1)
	}
	if got.Command != "patch" {
		t.Fatalf("command mismatch: got %q want %q", got.Command, "patch")
	}
	wantBody := "OK op=place_prefab manifest=testdata/manifests/simple_scene.bounds.json prefab=Assets/Prefabs/chair.prefab position=5,0,0 overlap_ids=none reserved_fileIDs=2002,2003\nPLAN prefab_guid=\"guid-chair\" append_ops=append:1:2002:GameObject,append:4:2003:Transform"
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
	}
	if got.PatchPlan == nil {
		t.Fatal("patch_plan is nil, want populated plan")
	}
	if got.PatchPlan.Status != "OK" {
		t.Fatalf("patch_plan.status mismatch: got %q want %q", got.PatchPlan.Status, "OK")
	}
	if got.PatchPlan.Reason != "" {
		t.Fatalf("patch_plan.reason mismatch: got %q want empty", got.PatchPlan.Reason)
	}
	if got.PatchPlan.PrefabPath != "Assets/Prefabs/chair.prefab" {
		t.Fatalf("patch_plan.prefab_path mismatch: got %q", got.PatchPlan.PrefabPath)
	}
	if got.PatchPlan.PrefabGUID != "guid-chair" {
		t.Fatalf("patch_plan.prefab_guid mismatch: got %q want %q", got.PatchPlan.PrefabGUID, "guid-chair")
	}
	if len(got.PatchPlan.OverlapIDs) != 0 {
		t.Fatalf("patch_plan.overlap_ids mismatch: got %v want none", got.PatchPlan.OverlapIDs)
	}
	wantReserved := []int64{2002, 2003}
	if fmt.Sprintf("%v", got.PatchPlan.ReservedFileIDs) != fmt.Sprintf("%v", wantReserved) {
		t.Fatalf("patch_plan.reserved_file_ids mismatch: got %v want %v", got.PatchPlan.ReservedFileIDs, wantReserved)
	}
	if len(got.PatchPlan.Appends) != 2 {
		t.Fatalf("patch_plan.appends mismatch: got %d want 2", len(got.PatchPlan.Appends))
	}
}

func TestSetAssetDryRunReturnsPlanAndDoesNotWrite(t *testing.T) {
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

	result := runCLI(
		t,
		"asset",
		"set",
		path,
		"--field",
		"maxHealth",
		"--value",
		"300",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1 pre_check=OK temp_check=OK\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != content {
		t.Fatal("dry-run should not modify file")
	}
}

func TestDiffRequiresPatch(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"diff",
		"testdata/scenes/simple_scene.unity",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR diff requires --patch\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestDiffReturnsCompactSummary(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"diff",
		"testdata/scenes/simple_scene.unity",
		"--patch",
		"testdata/patches/chair_place_ok.patch.json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "OK patch=testdata/patches/chair_place_ok.patch.json op=place_prefab append_ops=2 reserved_fileIDs=2002,2003\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestApplyDryRunReturnsCompactSummary(t *testing.T) {
	scenePath := copyFixtureToTemp(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"), "simple_scene.unity")

	result := runCLI(
		t,
		"scene",
		"apply",
		scenePath,
		"--patch",
		"testdata/patches/chair_place_ok.patch.json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "DRY_RUN patch=testdata/patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1 pre_check=OK temp_check=OK\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestApplyWriteCreatesBackup(t *testing.T) {
	scenePath := copyFixtureToTemp(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"), "simple_scene.unity")

	result := runCLI(
		t,
		"scene",
		"apply",
		scenePath,
		"--patch",
		"testdata/patches/chair_place_ok.patch.json",
		"--write",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "WRITE backup=" + scenePath + ".bak patch=testdata/patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}

	if _, err := os.Stat(scenePath + ".bak"); err != nil {
		t.Fatalf("backup stat error = %v", err)
	}
}

func TestApplyRejectsUnknownPatchStatus(t *testing.T) {
	scenePath := copyFixtureToTemp(t, filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity"), "simple_scene.unity")

	result := runCLI(
		t,
		"scene",
		"apply",
		scenePath,
		"--patch",
		"testdata/patches/chair_place_unknown.patch.json",
	)

	if result.exitCode != 1 {
		t.Fatalf("exit code mismatch: got %d want 1 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "ERROR PATCH_STATUS_UNRESOLVED status=UNKNOWN reason=NEED_PREFAB_GUID\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
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

	result := runCLI(
		t,
		"asset",
		"set",
		path,
		"--field",
		"maxHealth",
		"--value",
		"300",
		"--write",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "WRITE backup=" + path + ".bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
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

	result := runCLI(
		t,
		"asset",
		"set",
		path,
		"--field",
		"maxHealth",
		"--value",
		"200",
		"--write",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "OK field=maxHealth old=200 new=200 type_hint=int changed=0 verified=1 pre_check=OK temp_check=OK\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
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

func TestSetAssetWriteVerifiesStringLookingScalarSemantically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enemy_config.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: EnemyConfig\n" +
		"  label: starter\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result := runCLI(
		t,
		"asset",
		"set",
		path,
		"--field",
		"label",
		"--value",
		"001",
		"--write",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "WRITE backup=" + path + ".bak field=label old=starter new=\"001\" type_hint=string changed=1 verified=1 pre_check=OK temp_check=OK final_check=OK\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSetRejectsUnsupportedNamespace(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"set",
		"testdata/scenes/simple_scene.unity",
		"--id",
		"2000",
		"--field",
		"m_Name",
		"--value",
		"Chair_02",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR set not implemented for namespace=scene\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func copyFixtureToTemp(t *testing.T, source, name string) string {
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

func TestSetRequiresField(t *testing.T) {
	result := runCLI(
		t,
		"asset",
		"set",
		"testdata/assets/enemy_config.asset",
		"--value",
		"300",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR set requires --field\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSetRequiresValue(t *testing.T) {
	result := runCLI(
		t,
		"asset",
		"set",
		"testdata/assets/enemy_config.asset",
		"--field",
		"maxHealth",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR set requires --value\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSetAllowsExplicitEmptyValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enemy_config.asset")
	content := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Script: {fileID: 11500000, guid: a1b2c3d4e5f60718293a4b5c6d7e8f90, type: 3}\n" +
		"  m_Name: EnemyConfig\n" +
		"  label: starter\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result := runCLI(
		t,
		"asset",
		"set",
		path,
		"--field",
		"label",
		"--value",
		"",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "DRY_RUN field=label old=starter new=\"\" type_hint=string changed=1 pre_check=OK temp_check=OK\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSetRejectsIrrelevantFlags(t *testing.T) {
	result := runCLI(
		t,
		"asset",
		"set",
		"testdata/assets/enemy_config.asset",
		"--field",
		"maxHealth",
		"--value",
		"300",
		"--component",
		"MonoBehaviour",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR set does not accept --name, --type, --component, --out, --task, --focus, or --max-tokens\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSetSupportsIDSelection(t *testing.T) {
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

	result := runCLI(
		t,
		"asset",
		"set",
		path,
		"--id",
		"11400001",
		"--field",
		"maxHealth",
		"--value",
		"300",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1 pre_check=OK temp_check=OK\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSetJSONReturnsResultEnvelope(t *testing.T) {
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

	result := runCLI(
		t,
		"asset",
		"set",
		path,
		"--field",
		"maxHealth",
		"--value",
		"300",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.Namespace != "asset" {
		t.Fatalf("namespace mismatch: got %q want %q", got.Namespace, "asset")
	}
	if got.Command != "set" {
		t.Fatalf("command mismatch: got %q want %q", got.Command, "set")
	}
	if got.File != path {
		t.Fatalf("file mismatch: got %q want %q", got.File, path)
	}
	if got.View != "compact" {
		t.Fatalf("view mismatch: got %q want %q", got.View, "compact")
	}
	wantBody := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1 pre_check=OK temp_check=OK"
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
	}
}

func TestPrefabSetRequiresProject(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"set",
		"testdata/impact/project/Assets/Prefabs/Enemy.prefab",
		"--id",
		"11400000",
		"--field",
		"moveSpeed",
		"--value",
		"4.0",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	if result.stderr != "ERROR set requires --project\n" {
		t.Fatalf("stderr mismatch: got %q", result.stderr)
	}
}

func TestPrefabSetRequiresID(t *testing.T) {
	project := copyImpactProjectToTemp(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetProjectTarget(t, target, "fake_enemy_guid")

	result := runCLI(
		t,
		"prefab",
		"set",
		target,
		"--project",
		project,
		"--field",
		"moveSpeed",
		"--value",
		"4.0",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	if result.stderr != "ERROR set requires --id\n" {
		t.Fatalf("stderr mismatch: got %q", result.stderr)
	}
}

func TestPrefabSetRejectsIrrelevantFlags(t *testing.T) {
	project := copyImpactProjectToTemp(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetProjectTarget(t, target, "fake_enemy_guid")

	result := runCLI(
		t,
		"prefab",
		"set",
		target,
		"--project",
		project,
		"--id",
		"11400000",
		"--field",
		"moveSpeed",
		"--value",
		"4.0",
		"--component",
		"NavMeshAgent",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR set does not accept --name, --type, --component, --out, --task, --focus, --max-tokens, --scenes, --mode, --prefabs, --manifest, --prefab, --position, --op, --prefab-guid, or --patch\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestPrefabSetDryRunReturnsImpactSummary(t *testing.T) {
	project := copyImpactProjectToTemp(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetProjectTarget(t, target, "fake_enemy_guid")

	result := runCLI(
		t,
		"prefab",
		"set",
		target,
		"--project",
		project,
		"--id",
		"11400000",
		"--field",
		"moveSpeed",
		"--value",
		"4.0",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}
	want := "DRY_RUN field=moveSpeed old=3.5 new=4.0 type_hint=float changed=1 impact_status=OK scenes=2 scene_refs=3 prefabs=1 prefab_refs=2 nested_depth=1 ack_required=1 pre_check=OK temp_check=OK\n" +
		"SCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000 Assets/Scenes/Stage01.unity refs=2 fileIDs=1000,2000\n" +
		"PREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestPrefabSetJSONReturnsEnvelopeAndImpactPayload(t *testing.T) {
	project := copyImpactProjectToTemp(t)
	target := filepath.Join(project, "Assets", "Prefabs", "Enemy.prefab")
	writePrefabSetProjectTarget(t, target, "fake_enemy_guid")

	result := runCLI(
		t,
		"prefab",
		"set",
		target,
		"--project",
		project,
		"--id",
		"11400000",
		"--field",
		"moveSpeed",
		"--value",
		"4.0",
		"--json",
	)
	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		View      string `json:"view"`
		Impact    struct {
			Status         string `json:"status"`
			PrefabPath     string `json:"prefab_path"`
			PrefabGUID     string `json:"prefab_guid"`
			DepthLimitHit  bool   `json:"depth_limit_hit"`
			MaxNestedDepth int    `json:"max_nested_depth"`
		} `json:"impact"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}
	if got.Status != "OK" || got.Namespace != "prefab" || got.Command != "set" || got.View != "compact" {
		t.Fatalf("envelope mismatch: %#v", got)
	}
	if got.Impact.PrefabGUID != "fake_enemy_guid" || got.Impact.PrefabPath != "Assets/Prefabs/Enemy.prefab" {
		t.Fatalf("impact payload mismatch: %#v", got.Impact)
	}
}

func TestGetRejectsMissingField(t *testing.T) {
	result := runCLI(
		t,
		"asset",
		"get",
		"testdata/assets/enemy_config.asset",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}

	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}

	want := "ERROR get requires --field\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestInspectJSONReturnsResultEnvelope(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"inspect",
		"testdata/prefabs/enemy.prefab",
		"--component",
		"NavMeshAgent",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}

	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" || got.Command != "inspect" || got.Namespace != "prefab" || got.View != "compact" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	wantBody := "OK component=NavMeshAgent fileID=4000 fields=m_Acceleration,m_AngularSpeed,m_AutoBraking,m_GameObject,m_Speed,m_StoppingDistance"
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
	}
}

func TestIndexWritesSnapshotAndSucceeds(t *testing.T) {
	source := filepath.Join(repoRoot(t), "testdata", "scenes", "simple_scene.unity")
	out := filepath.Join(t.TempDir(), "simple_scene.index.json")

	result := runCLI(
		t,
		"scene",
		"index",
		source,
		"--out",
		out,
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	want := "OK index file=" + source + " out=" + out + " objects=4\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected snapshot file to be written")
	}
}

func TestIndexJSONReturnsResultEnvelope(t *testing.T) {
	source := filepath.Join(repoRoot(t), "testdata", "scenes", "simple_scene.unity")
	out := filepath.Join(t.TempDir(), "simple_scene.index.json")

	result := runCLI(
		t,
		"scene",
		"index",
		source,
		"--out",
		out,
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" || got.Command != "index" || got.Namespace != "scene" || got.View != "compact" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	wantBody := "OK index file=" + source + " out=" + out + " objects=4"
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
	}
}

func TestIndexRejectsOutPathMatchingInput(t *testing.T) {
	path := "testdata/scenes/simple_scene.unity"
	result := runCLI(
		t,
		"scene",
		"index",
		path,
		"--out",
		path,
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR index requires --out to differ from input file\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestIndexRejectsOutPathEquivalentToInput(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"index",
		"testdata/scenes/simple_scene.unity",
		"--out",
		"testdata/scenes/../scenes/simple_scene.unity",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR index requires --out to differ from input file\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestIndexRejectsSymlinkAliasToInput(t *testing.T) {
	source := filepath.Join(repoRoot(t), "testdata", "scenes", "simple_scene.unity")
	linkPath := filepath.Join(t.TempDir(), "scene-link.json")
	if err := os.Symlink(source, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	result := runCLI(
		t,
		"scene",
		"index",
		source,
		"--out",
		linkPath,
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR index requires --out to differ from input file\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneScanRequiresMode(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"scan",
		"testdata/scenes/simple_scene.unity",
		"--project",
		"/tmp/project",
		"--out",
		"/private/tmp/simple_scene.bounds.json",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR scan requires --mode\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneScanRejectsUnsupportedMode(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"scan",
		"testdata/scenes/simple_scene.unity",
		"--mode",
		"offline",
		"--project",
		"/tmp/project",
		"--out",
		"/private/tmp/simple_scene.bounds.json",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR scan supports only --mode editor\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneScanRequiresProject(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"scan",
		"testdata/scenes/simple_scene.unity",
		"--mode",
		"editor",
		"--out",
		"/private/tmp/simple_scene.bounds.json",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR scan requires --project\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneScanRequiresOut(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"scan",
		"testdata/scenes/simple_scene.unity",
		"--mode",
		"editor",
		"--project",
		"/tmp/project",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR scan requires --out\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneScanRejectsIrrelevantFlags(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"scan",
		"testdata/scenes/simple_scene.unity",
		"--mode",
		"editor",
		"--project",
		"/tmp/project",
		"--out",
		"/private/tmp/simple_scene.bounds.json",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR scan does not accept --id, --name, --type, --component, --field, --value, --write, --manifest, --prefab, --position, --op, --prefab-guid, --task, --focus, or --max-tokens\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneScanRejectsScenesFlag(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"scan",
		"testdata/scenes/simple_scene.unity",
		"--mode",
		"editor",
		"--project",
		"/tmp/project",
		"--out",
		"/private/tmp/simple_scene.bounds.json",
		"--scenes",
		"Assets/Scenes/BossRoom.unity",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	if result.stderr != "ERROR scan does not accept --scenes\n" {
		t.Fatalf("stderr mismatch: got %q", result.stderr)
	}
}

func TestSceneScanJSONReturnsDeterministicEnvelope(t *testing.T) {
	project := t.TempDir()
	scenePath := filepath.Join(project, "Assets", "Scenes", "SimpleScene.unity")
	if err := os.MkdirAll(filepath.Dir(scenePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scenePath, []byte("%YAML 1.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "simple_scene.bounds.json")
	pathEnv := fakeUnityCLIPathEnv(t, `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [
    {"fileID": 2000, "name": "Chair_01", "center": [3, 0.5, 1], "size": [1, 1, 1]},
    {"fileID": 1000, "name": "Table_01", "center": [1, 0.5, 2], "size": [2, 1, 1]}
  ],
  "prefabs": [
    {"path": "Assets/Prefabs/table.prefab", "center": [0, 0.5, 0], "size": [2, 1, 1]},
    {"path": "Assets/Prefabs/chair.prefab", "center": [0, 0.5, 0], "size": [1, 1, 1]}
  ]
}`)

	result := runCLIWithEnv(
		t,
		[]string{pathEnv},
		"scene",
		"scan",
		scenePath,
		"--mode",
		"editor",
		"--project",
		project,
		"--prefabs",
		"Assets/Prefabs/table.prefab,Assets/Prefabs/chair.prefab",
		"--out",
		outPath,
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q want %q", got.Status, "OK")
	}
	if got.Namespace != "scene" {
		t.Fatalf("namespace mismatch: got %q want %q", got.Namespace, "scene")
	}
	if got.Command != "scan" {
		t.Fatalf("command mismatch: got %q want %q", got.Command, "scan")
	}
	if got.File != scenePath {
		t.Fatalf("file mismatch: got %q want %q", got.File, scenePath)
	}
	if got.View != "compact" {
		t.Fatalf("view mismatch: got %q want %q", got.View, "compact")
	}
	wantBody := "OK mode=editor project=" + project + " scene=Assets/Scenes/SimpleScene.unity out=" + outPath + " objects=2 prefabs=2 source=editor"
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
	}
}

func TestSceneScanCompactOutputMatchesExpectedText(t *testing.T) {
	project := t.TempDir()
	scenePath := filepath.Join(project, "Assets", "Scenes", "SimpleScene.unity")
	if err := os.MkdirAll(filepath.Dir(scenePath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(scenePath, []byte("%YAML 1.1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "simple_scene.bounds.json")
	pathEnv := fakeUnityCLIPathEnv(t, `{
  "scene": "Assets/Scenes/SimpleScene.unity",
  "objects": [
    {"fileID": 2000, "name": "Chair_01", "center": [3, 0.5, 1], "size": [1, 1, 1]},
    {"fileID": 1000, "name": "Table_01", "center": [1, 0.5, 2], "size": [2, 1, 1]}
  ],
  "prefabs": [
    {"path": "Assets/Prefabs/table.prefab", "center": [0, 0.5, 0], "size": [2, 1, 1]},
    {"path": "Assets/Prefabs/chair.prefab", "center": [0, 0.5, 0], "size": [1, 1, 1]}
  ]
}`)

	result := runCLIWithEnv(
		t,
		[]string{pathEnv},
		"scene",
		"scan",
		scenePath,
		"--mode",
		"editor",
		"--project",
		project,
		"--prefabs",
		"Assets/Prefabs/table.prefab,Assets/Prefabs/chair.prefab",
		"--out",
		outPath,
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}
	want := "OK mode=editor project=" + project + " scene=Assets/Scenes/SimpleScene.unity out=" + outPath + " objects=2 prefabs=2 source=editor\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSceneSuggestRejectsUnsupportedNamespace(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"suggest",
		"testdata/prefabs/enemy.prefab",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--near",
		"1000",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	if result.stderr != "ERROR suggest not implemented for namespace=prefab\n" {
		t.Fatalf("stderr mismatch: got %q", result.stderr)
	}
}

func TestSceneSuggestRequiresManifestPrefabAndNear(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "manifest",
			args: []string{
				"scene", "suggest", "testdata/scenes/simple_scene.unity",
				"--prefab", "Assets/Prefabs/chair.prefab",
				"--near", "1000",
			},
			want: "ERROR suggest requires --manifest\n",
		},
		{
			name: "prefab",
			args: []string{
				"scene", "suggest", "testdata/scenes/simple_scene.unity",
				"--manifest", "testdata/manifests/simple_scene.bounds.json",
				"--near", "1000",
			},
			want: "ERROR suggest requires --prefab\n",
		},
		{
			name: "near",
			args: []string{
				"scene", "suggest", "testdata/scenes/simple_scene.unity",
				"--manifest", "testdata/manifests/simple_scene.bounds.json",
				"--prefab", "Assets/Prefabs/chair.prefab",
			},
			want: "ERROR suggest requires --near\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args...)
			if result.exitCode != 2 {
				t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
			}
			if result.stdout != "" {
				t.Fatalf("expected empty stdout, got %q", result.stdout)
			}
			if result.stderr != tt.want {
				t.Fatalf("stderr mismatch: got %q want %q", result.stderr, tt.want)
			}
		})
	}
}

func TestSceneSuggestRejectsInvalidCount(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"suggest",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--near",
		"1000",
		"--count",
		"0",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	if result.stderr != "ERROR suggest requires --count >= 1\n" {
		t.Fatalf("stderr mismatch: got %q", result.stderr)
	}
}

func TestSceneSuggestRejectsInvalidAlign(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"suggest",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--near",
		"1000",
		"--align",
		"wall",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	if result.stderr != "ERROR suggest supports only --align floor|grid\n" {
		t.Fatalf("stderr mismatch: got %q", result.stderr)
	}
}

func TestSceneSuggestRejectsIrrelevantFlags(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"suggest",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--near",
		"1000",
		"--id",
		"1000",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR suggest does not accept --id, --name, --type, --component, --field, --value, --write, --scenes, --prefabs, --position, --op, --task, --focus, --max-tokens, --patch, --ack-impact, or --mode\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestSceneSuggestAcceptsProjectForGUIDAutoResolve(t *testing.T) {
	project := t.TempDir()
	prefabAbs := filepath.Join(project, "Assets", "Prefabs", "chair.prefab")
	if err := os.MkdirAll(filepath.Dir(prefabAbs), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(prefabAbs, []byte("--- !u!1 &1000\nGameObject:\n  m_Name: chair\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(prefabAbs+".meta", []byte("fileFormatVersion: 2\nguid: 3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.meta) error = %v", err)
	}

	outFile := filepath.Join(project, "out.patch.json")
	result := runCLI(t,
		"scene", "suggest", "testdata/scenes/simple_scene.unity",
		"--manifest", "testdata/manifests/simple_scene.bounds.json",
		"--prefab", "Assets/Prefabs/chair.prefab",
		"--near", "1000",
		"--project", project,
		"--pick", "1",
		"--out", outFile,
	)
	if result.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q stdout=%q", result.exitCode, result.stderr, result.stdout)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("patch file not created: %v", err)
	}
	if !strings.Contains(string(data), `"prefab_guid":"3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b"`) {
		t.Fatalf("patch did not carry the auto-resolved guid:\n%s", string(data))
	}
}

func TestSceneSuggestWritesPatchFileViaOut(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "out.patch.json")
	result := runCLI(t,
		"scene", "suggest", "testdata/scenes/simple_scene.unity",
		"--manifest", "testdata/manifests/simple_scene.bounds.json",
		"--prefab", "Assets/Prefabs/chair.prefab",
		"--near", "1000",
		"--prefab-guid", "guid-chair",
		"--out", outFile,
	)
	if result.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s stdout=%s", result.exitCode, result.stderr, result.stdout)
	}
	if !strings.Contains(result.stdout, "PATCH_OUT rank=1") {
		t.Fatalf("stdout missing PATCH_OUT line:\n%s", result.stdout)
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("patch file not created: %v", err)
	}
}

func TestSceneSuggestPickSelectsRank(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "out.patch.json")
	result := runCLI(t,
		"scene", "suggest", "testdata/scenes/simple_scene.unity",
		"--manifest", "testdata/manifests/simple_scene.bounds.json",
		"--prefab", "Assets/Prefabs/chair.prefab",
		"--near", "1000",
		"--prefab-guid", "guid-chair",
		"--out", outFile,
		"--pick", "2",
	)
	if result.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", result.exitCode, result.stderr)
	}
	if !strings.Contains(result.stdout, "PATCH_OUT rank=2") {
		t.Fatalf("stdout missing PATCH_OUT rank=2:\n%s", result.stdout)
	}
}

func TestSceneSuggestRejectsPickWithoutOut(t *testing.T) {
	result := runCLI(t,
		"scene", "suggest", "testdata/scenes/simple_scene.unity",
		"--manifest", "testdata/manifests/simple_scene.bounds.json",
		"--prefab", "Assets/Prefabs/chair.prefab",
		"--near", "1000",
		"--pick", "2",
	)
	if result.exitCode == 0 {
		t.Fatalf("expected non-zero exit")
	}
	if result.stderr != "ERROR suggest --pick requires --out\n" {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestSceneSuggestRejectsPrefabGUIDWithoutOut(t *testing.T) {
	result := runCLI(t,
		"scene", "suggest", "testdata/scenes/simple_scene.unity",
		"--manifest", "testdata/manifests/simple_scene.bounds.json",
		"--prefab", "Assets/Prefabs/chair.prefab",
		"--near", "1000",
		"--prefab-guid", "some-guid",
	)
	if result.exitCode == 0 {
		t.Fatalf("expected non-zero exit")
	}
	if result.stderr != "ERROR suggest --prefab-guid requires --out\n" {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestSceneSuggestRejectsPickBelowOne(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "out.patch.json")
	result := runCLI(t,
		"scene", "suggest", "testdata/scenes/simple_scene.unity",
		"--manifest", "testdata/manifests/simple_scene.bounds.json",
		"--prefab", "Assets/Prefabs/chair.prefab",
		"--near", "1000",
		"--out", outFile,
		"--pick", "0",
	)
	if result.exitCode == 0 {
		t.Fatalf("expected non-zero exit")
	}
	if result.stderr != "ERROR suggest requires --pick >= 1\n" {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestPickRejectedOnNonSuggestCommand(t *testing.T) {
	result := runCLI(t,
		"scene", "summarize", "testdata/scenes/simple_scene.unity",
		"--pick", "2",
	)
	if result.exitCode == 0 {
		t.Fatalf("expected non-zero exit")
	}
	if result.stderr != "ERROR summarize does not accept --near, --count, --align, or --pick\n" {
		t.Fatalf("unexpected stderr: %q", result.stderr)
	}
}

func TestSceneSuggestReturnsCompactOutput(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"suggest",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--near",
		"1000",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}
	want := "OK manifest=testdata/manifests/simple_scene.bounds.json prefab=Assets/Prefabs/chair.prefab near=1000 align=floor count=4 candidates=4 clear=4 warn=0\n" +
		"CANDIDATE rank=1 direction=east position=1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=2 direction=west position=-1.4,0,0 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=3 direction=north position=0,0,1 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01\n" +
		"CANDIDATE rank=4 direction=south position=0,0,-1 status=OK overlap_ids=none anchor_id=1000 anchor_name=Table_01\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestSceneSuggestJSONReturnsEnvelopePlusSuggestPayload(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"suggest",
		"testdata/scenes/simple_scene.unity",
		"--manifest",
		"testdata/manifests/simple_scene.bounds.json",
		"--prefab",
		"Assets/Prefabs/chair.prefab",
		"--near",
		"1000",
		"--align",
		"grid",
		"--count",
		"2",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
		Suggest   struct {
			Status   string `json:"status"`
			Manifest string `json:"manifest"`
			Prefab   string `json:"prefab"`
			Anchor   struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"anchor"`
			Align      string `json:"align"`
			Count      int    `json:"count"`
			Candidates []struct {
				Rank      int       `json:"rank"`
				Direction string    `json:"direction"`
				Position  []float64 `json:"position"`
				Status    string    `json:"status"`
			} `json:"candidates"`
		} `json:"suggest"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" || got.Namespace != "scene" || got.Command != "suggest" || got.View != "compact" {
		t.Fatalf("envelope mismatch: %#v", got)
	}
	if got.Suggest.Manifest != "testdata/manifests/simple_scene.bounds.json" || got.Suggest.Prefab != "Assets/Prefabs/chair.prefab" {
		t.Fatalf("suggest payload mismatch: %#v", got.Suggest)
	}
	if got.Suggest.Anchor.ID != 1000 || got.Suggest.Anchor.Name != "Table_01" {
		t.Fatalf("anchor mismatch: %#v", got.Suggest.Anchor)
	}
	if got.Suggest.Align != "grid" || got.Suggest.Count != 2 || len(got.Suggest.Candidates) != 2 {
		t.Fatalf("suggest metadata mismatch: %#v", got.Suggest)
	}
}

func TestPrefabImpactRequiresProject(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"impact",
		"testdata/impact/project/Assets/Prefabs/Enemy.prefab",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	if result.stderr != "ERROR impact requires --project\n" {
		t.Fatalf("stderr mismatch: got %q", result.stderr)
	}
}

func TestPrefabImpactRejectsIrrelevantFlags(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"impact",
		"testdata/impact/project/Assets/Prefabs/Enemy.prefab",
		"--project",
		"testdata/impact/project",
		"--id",
		"1000",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR impact does not accept --id, --name, --type, --component, --field, --value, --write, --manifest, --prefab, --position, --op, --prefab-guid, --task, --focus, --max-tokens, --out, --mode, --prefabs, or --patch\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestPrefabImpactReturnsCompactOutput(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"impact",
		"testdata/impact/project/Assets/Prefabs/Enemy.prefab",
		"--project",
		"testdata/impact/project",
		"--scenes",
		"Assets/Scenes/BossRoom.unity",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}
	want := "OK prefab=Assets/Prefabs/Enemy.prefab guid=fake_enemy_guid scenes=1 scene_refs=1 prefabs=1 prefab_refs=2 nested_depth=1\nSCENES Assets/Scenes/BossRoom.unity refs=1 fileIDs=4000\nPREFABS Assets/Prefabs/EnemyElite.prefab refs=2 fileIDs=3000,3001\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestPrefabImpactJSONReturnsEnvelopePlusImpactPayload(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"impact",
		"testdata/impact/project/Assets/Prefabs/Enemy.prefab",
		"--project",
		"testdata/impact/project",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
		Impact    struct {
			Status         string `json:"status"`
			PrefabPath     string `json:"prefab_path"`
			PrefabGUID     string `json:"prefab_guid"`
			DepthLimitHit  bool   `json:"depth_limit_hit"`
			MaxNestedDepth int    `json:"max_nested_depth"`
			SceneHits      []struct {
				Path       string  `json:"path"`
				References int     `json:"references"`
				FileIDs    []int64 `json:"file_ids"`
			} `json:"scene_hits"`
			PrefabHits []struct {
				Path       string  `json:"path"`
				References int     `json:"references"`
				FileIDs    []int64 `json:"file_ids"`
			} `json:"prefab_hits"`
		} `json:"impact"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" || got.Namespace != "prefab" || got.Command != "impact" || got.View != "compact" {
		t.Fatalf("envelope mismatch: %#v", got)
	}
	if got.Impact.PrefabPath != "Assets/Prefabs/Enemy.prefab" || got.Impact.PrefabGUID != "fake_enemy_guid" {
		t.Fatalf("impact payload mismatch: %#v", got.Impact)
	}
	if len(got.Impact.SceneHits) != 2 || len(got.Impact.PrefabHits) != 1 {
		t.Fatalf("impact hit counts mismatch: %#v", got.Impact)
	}
}

func TestContextPackRequiresFocusOrTask(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"context-pack",
		"testdata/scenes/simple_scene.unity",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR context-pack requires --focus or --task\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestContextPackRejectsTooSmallMaxTokens(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "rejects one",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--max-tokens", "1"},
		},
		{
			name: "rejects negative",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--max-tokens", "-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args...)

			if result.exitCode != 2 {
				t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
			}
			if result.stdout != "" {
				t.Fatalf("expected empty stdout, got %q", result.stdout)
			}
			want := "ERROR context-pack requires --max-tokens >= " + strconv.Itoa(contextpack.MinimumBudget()) + "\n"
			if result.stderr != want {
				t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
			}
		})
	}
}

func TestContextPackRejectsFileDependentTooSmallMaxTokens(t *testing.T) {
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

	result := runCLI(
		t,
		"scene",
		"context-pack",
		path,
		"--focus",
		"Object_0",
		"--max-tokens",
		strconv.Itoa(contextpack.MinimumBudget()),
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR context-pack requires --max-tokens >= " + strconv.Itoa(contextpack.MinimumBudgetForOptions(contextpack.Options{
		Namespace: "scene",
		File:      path,
		Focus:     "Object_0",
		MaxTokens: contextpack.MinimumBudget(),
	}, 11)) + "\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestInspectRejectsExplicitZeroIDSelector(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"inspect",
		"testdata/scenes/simple_scene.unity",
		"--id",
		"0",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR inspect requires non-zero --id\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestGetRejectsWhitespaceOnlyField(t *testing.T) {
	result := runCLI(
		t,
		"asset",
		"get",
		"testdata/assets/enemy_config.asset",
		"--field",
		"   ",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR get requires --field\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestIndexRejectsWhitespaceOnlyOut(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"index",
		"testdata/scenes/simple_scene.unity",
		"--out",
		"   ",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR index requires --out\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestContextPackJSONReturnsResultEnvelope(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"context-pack",
		"testdata/scenes/simple_scene.unity",
		"--task",
		"inspect props",
		"--focus",
		"Chair_01",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" || got.Command != "context-pack" || got.Namespace != "scene" || got.View != "compact" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if !bytes.Contains([]byte(got.Body), []byte("TASK_CONTEXT scene=testdata/scenes/simple_scene.unity")) {
		t.Fatalf("body missing task context: %q", got.Body)
	}
}

func TestContextPackRejectsIrrelevantFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "rejects id",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--id", "2000"},
		},
		{
			name: "rejects name",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--name", "Table_01"},
		},
		{
			name: "rejects type",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--type", "GameObject"},
		},
		{
			name: "rejects component",
			args: []string{"prefab", "context-pack", "testdata/prefabs/enemy.prefab", "--focus", "Enemy", "--component", "NavMeshAgent"},
		},
		{
			name: "rejects field",
			args: []string{"asset", "context-pack", "testdata/assets/enemy_config.asset", "--focus", "EnemyConfig", "--field", "maxHealth"},
		},
		{
			name: "rejects out",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--out", "ignored.index.json"},
		},
		{
			name: "rejects explicit zero id",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--id", "0"},
		},
		{
			name: "rejects explicit empty name",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--name", ""},
		},
		{
			name: "rejects explicit empty type",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--type", ""},
		},
		{
			name: "rejects explicit empty component",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--component", ""},
		},
		{
			name: "rejects explicit empty field",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--field", ""},
		},
		{
			name: "rejects explicit empty out",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Table_01", "--out", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args...)

			if result.exitCode != 2 {
				t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
			}
			if result.stdout != "" {
				t.Fatalf("expected empty stdout, got %q", result.stdout)
			}
			want := "ERROR context-pack does not accept --id, --name, --type, --component, --field, or --out\n"
			if result.stderr != want {
				t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
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

	inspect := runCLI(t, "asset", "inspect", path, "--id", "11400001")
	if inspect.exitCode != 0 {
		t.Fatalf("inspect exit code mismatch: got %d want 0 stderr=%q", inspect.exitCode, inspect.stderr)
	}
	wantInspect := "OK component=MonoBehaviour fileID=11400001 fields=m_Name,maxHealth\n"
	if inspect.stdout != wantInspect {
		t.Fatalf("inspect stdout mismatch: got %q want %q", inspect.stdout, wantInspect)
	}

	get := runCLI(t, "asset", "get", path, "--name", "ConfigB", "--field", "maxHealth")
	if get.exitCode != 0 {
		t.Fatalf("get exit code mismatch: got %d want 0 stderr=%q", get.exitCode, get.stderr)
	}
	wantGet := "OK field=maxHealth value=350\n"
	if get.stdout != wantGet {
		t.Fatalf("get stdout mismatch: got %q want %q", get.stdout, wantGet)
	}
}

func TestContextPackPrefabJSONReturnsResultEnvelope(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"context-pack",
		"testdata/prefabs/enemy.prefab",
		"--focus",
		"Enemy",
		"--task",
		"inspect navigation setup",
		"--max-tokens",
		"32",
		"--json",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0", result.exitCode)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}

	var got struct {
		Status    string `json:"status"`
		Namespace string `json:"namespace"`
		Command   string `json:"command"`
		File      string `json:"file"`
		View      string `json:"view"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}

	if got.Status != "OK" || got.Command != "context-pack" || got.Namespace != "prefab" || got.View != "compact" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if !bytes.Contains([]byte(got.Body), []byte(`focus="Enemy"`)) {
		t.Fatalf("body missing reflected focus: %q", got.Body)
	}
}

func TestNonContextPackCommandsRejectContextPackFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "summarize rejects task",
			args:    []string{"scene", "summarize", "testdata/scenes/simple_scene.unity", "--task", "inspect props"},
			wantErr: "ERROR summarize does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name:    "query rejects focus",
			args:    []string{"scene", "query", "testdata/scenes/simple_scene.unity", "--id", "2000", "--focus", "Chair_01"},
			wantErr: "ERROR query does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name:    "inspect rejects max tokens",
			args:    []string{"prefab", "inspect", "testdata/prefabs/enemy.prefab", "--component", "NavMeshAgent", "--max-tokens", "32"},
			wantErr: "ERROR inspect does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name:    "get rejects task",
			args:    []string{"asset", "get", "testdata/assets/enemy_config.asset", "--field", "maxHealth", "--task", "read health"},
			wantErr: "ERROR get does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name:    "index rejects focus",
			args:    []string{"scene", "index", "testdata/scenes/simple_scene.unity", "--out", "ignored.index.json", "--focus", "Chair_01"},
			wantErr: "ERROR index does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name:    "summarize rejects explicit default max tokens",
			args:    []string{"scene", "summarize", "testdata/scenes/simple_scene.unity", "--max-tokens", "256"},
			wantErr: "ERROR summarize does not accept --task, --focus, or --max-tokens\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args...)

			if result.exitCode != 2 {
				t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
			}
			if result.stdout != "" {
				t.Fatalf("expected empty stdout, got %q", result.stdout)
			}
			if result.stderr != tt.wantErr {
				t.Fatalf("stderr mismatch: got %q want %q", result.stderr, tt.wantErr)
			}
		})
	}
}

func TestNonSetCommandsRejectWriteFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "summarize",
			args: []string{"scene", "summarize", "testdata/scenes/simple_scene.unity", "--write"},
		},
		{
			name: "query",
			args: []string{"scene", "query", "testdata/scenes/simple_scene.unity", "--id", "2000", "--write"},
		},
		{
			name: "inspect",
			args: []string{"prefab", "inspect", "testdata/prefabs/enemy.prefab", "--component", "NavMeshAgent", "--write"},
		},
		{
			name: "get",
			args: []string{"asset", "get", "testdata/assets/enemy_config.asset", "--field", "maxHealth", "--write"},
		},
		{
			name: "index",
			args: []string{"scene", "index", "testdata/scenes/simple_scene.unity", "--out", "ignored.index.json", "--write"},
		},
		{
			name: "context-pack",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Chair_01", "--write"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runCLI(t, tc.args...)

			if result.exitCode != 2 {
				t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
			}
			if result.stdout != "" {
				t.Fatalf("expected empty stdout, got %q", result.stdout)
			}

			want := fmt.Sprintf("ERROR %s does not accept --write\n", tc.name)
			if result.stderr != want {
				t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
			}
		})
	}
}

func TestNonSetCommandsRejectValueFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "summarize",
			args: []string{"scene", "summarize", "testdata/scenes/simple_scene.unity", "--value", "300"},
		},
		{
			name: "query",
			args: []string{"scene", "query", "testdata/scenes/simple_scene.unity", "--id", "2000", "--value", "300"},
		},
		{
			name: "inspect",
			args: []string{"prefab", "inspect", "testdata/prefabs/enemy.prefab", "--component", "NavMeshAgent", "--value", "300"},
		},
		{
			name: "get",
			args: []string{"asset", "get", "testdata/assets/enemy_config.asset", "--field", "maxHealth", "--value", "300"},
		},
		{
			name: "index",
			args: []string{"scene", "index", "testdata/scenes/simple_scene.unity", "--out", "ignored.index.json", "--value", "300"},
		},
		{
			name: "context-pack",
			args: []string{"scene", "context-pack", "testdata/scenes/simple_scene.unity", "--focus", "Chair_01", "--value", "300"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runCLI(t, tc.args...)

			if result.exitCode != 2 {
				t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
			}
			if result.stdout != "" {
				t.Fatalf("expected empty stdout, got %q", result.stdout)
			}

			want := fmt.Sprintf("ERROR %s does not accept --value\n", tc.name)
			if result.stderr != want {
				t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
			}
		})
	}
}

func TestNonSetCommandsRejectAckImpactFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "summarize",
			args: []string{"scene", "summarize", "testdata/scenes/simple_scene.unity", "--ack-impact"},
		},
		{
			name: "impact",
			args: []string{"prefab", "impact", "testdata/impact/project/Assets/Prefabs/Enemy.prefab", "--project", "testdata/impact/project", "--ack-impact"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := runCLI(t, tc.args...)

			if result.exitCode != 2 {
				t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
			}
			if result.stdout != "" {
				t.Fatalf("expected empty stdout, got %q", result.stdout)
			}

			want := fmt.Sprintf("ERROR %s does not accept --ack-impact\n", tc.name)
			if result.stderr != want {
				t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
			}
		})
	}
}

func TestSummarizeRejectsOutFlag(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"summarize",
		"testdata/scenes/simple_scene.unity",
		"--out",
		"ignored.index.json",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR summarize does not accept --out\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestInspectRejectsInvalidSelectorCombination(t *testing.T) {
	result := runCLI(
		t,
		"prefab",
		"inspect",
		"testdata/prefabs/enemy.prefab",
		"--id",
		"1000",
		"--name",
		"Enemy",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR inspect requires at most one of --id or --name\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestGetRejectsInvalidSelectorCombination(t *testing.T) {
	result := runCLI(
		t,
		"asset",
		"get",
		"testdata/assets/enemy_config.asset",
		"--id",
		"11400000",
		"--name",
		"EnemyConfig",
		"--field",
		"maxHealth",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stdout != "" {
		t.Fatalf("expected empty stdout, got %q", result.stdout)
	}
	want := "ERROR get requires at most one of --id or --name\n"
	if result.stderr != want {
		t.Fatalf("stderr mismatch: got %q want %q", result.stderr, want)
	}
}

func TestBenchReturnsCompactOutput(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"bench",
		"testdata/scenes/simple_scene.unity",
	)

	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d want 0 stderr=%q", result.exitCode, result.stderr)
	}
	if result.stderr != "" {
		t.Fatalf("expected empty stderr, got %q", result.stderr)
	}
	if !strings.Contains(result.stdout, "raw_bytes=") || !strings.Contains(result.stdout, "summarize_ratio=") {
		t.Fatalf("stdout mismatch: got %q", result.stdout)
	}
}

func TestBenchJSONIncludesNestedPayload(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"bench",
		"testdata/scenes/simple_scene.unity",
		"--task",
		"inspect placement safety",
		"--json",
	)

	var got struct {
		Command string `json:"command"`
		Bench   struct {
			RawBytes    int `json:"raw_bytes"`
			RawTokens   int `json:"raw_tokens"`
			ContextPack *struct {
				Tokens int `json:"tokens"`
			} `json:"context_pack"`
		} `json:"bench"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v", err)
	}
	if got.Command != "bench" || got.Bench.ContextPack == nil {
		t.Fatalf("bench payload mismatch: %#v", got)
	}
}

func TestBenchRejectsIrrelevantFlags(t *testing.T) {
	result := runCLI(
		t,
		"scene",
		"bench",
		"testdata/scenes/simple_scene.unity",
		"--focus",
		"Table_01",
	)

	if result.exitCode != 2 {
		t.Fatalf("exit code mismatch: got %d want 2", result.exitCode)
	}
	if result.stderr != "ERROR bench does not accept --focus, --max-tokens, --id, --name, --type, --component, --field, --value, --write, --out, --manifest, --prefab, --position, --op, --prefab-guid, --project, --scenes, --prefabs, --patch, --ack-impact, --near, --count, or --align\n" {
		t.Fatalf("stderr mismatch: got %q", result.stderr)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}

	return root
}

type runResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func runCLI(t *testing.T, args ...string) runResult {
	t.Helper()

	return runCLIWithEnv(t, nil, args...)
}

func runCLIWithEnv(t *testing.T, env []string, args ...string) runResult {
	t.Helper()

	binary := buildCLI(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), env...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run cli: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}

	return runResult{
		exitCode: exitCode,
		stdout:   stdout.String(),
		stderr:   stderr.String(),
	}
}

func fakeUnityCLIPathEnv(t *testing.T, output string) string {
	t.Helper()

	binDir := t.TempDir()
	scriptPath := filepath.Join(binDir, "unity-cli")
	script := "#!/bin/sh\ncat <<'EOF'\n" + output + "\nEOF\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	return "PATH=" + binDir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func copyImpactProjectToTemp(t *testing.T) string {
	t.Helper()

	source := filepath.Join(repoRoot(t), "testdata", "impact", "project")
	dest := filepath.Join(t.TempDir(), "project")
	if err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
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
	}); err != nil {
		t.Fatalf("copy impact project: %v", err)
	}
	return dest
}

func writePrefabSetProjectTarget(t *testing.T, prefabPath, guid string) {
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

func buildCLI(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "unity-ctx")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/unity-ctx")
	cmd.Dir = repoRoot(t)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build cli: %v: %s", err, fmt.Sprintf("%s", stderr.String()))
	}

	return binary
}

func TestMetaGUIDResolvesFromMetaFile(t *testing.T) {
	dir := t.TempDir()
	prefabPath := filepath.Join(dir, "Chair.prefab")
	if err := os.WriteFile(prefabPath, []byte("--- !u!1 &1000\nGameObject:\n  m_Name: Chair\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := "fileFormatVersion: 2\nguid: 3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b\nPrefabImporter:\n  externalObjects: {}\n"
	if err := os.WriteFile(prefabPath+".meta", []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile(.meta) error = %v", err)
	}

	result := runCLI(t, "meta", "guid", prefabPath)
	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d stderr=%q", result.exitCode, result.stderr)
	}
	want := "OK guid=3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b file=" + prefabPath + " meta=" + prefabPath + ".meta\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestRefsReturnsReferenceEvidence(t *testing.T) {
	result := runCLI(t, "prefab", "refs", "testdata/prefabs/enemy.prefab")
	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d stderr=%q", result.exitCode, result.stderr)
	}
	want := "OK refs file=testdata/prefabs/enemy.prefab count=4 warnings=0\n" +
		"REF block=1000 class=GameObject field=m_Component[0].component file_id=2000\n" +
		"REF block=1000 class=GameObject field=m_Component[1].component file_id=3000\n" +
		"REF block=1000 class=GameObject field=m_Component[2].component file_id=4000\n" +
		"REF block=3000 class=MonoBehaviour field=m_Script file_id=11500000 guid=a1b2c3d4e5f60718293a4b5c6d7e8f90 type=3\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestMetaGUIDResolvesRelativePathViaProject(t *testing.T) {
	dir := t.TempDir()
	prefabRel := filepath.Join("Assets", "Prefabs", "Chair.prefab")
	prefabAbs := filepath.Join(dir, prefabRel)
	if err := os.MkdirAll(filepath.Dir(prefabAbs), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(prefabAbs, []byte("--- !u!1 &1000\nGameObject:\n  m_Name: Chair\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	meta := "fileFormatVersion: 2\nguid: 3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b\n"
	if err := os.WriteFile(prefabAbs+".meta", []byte(meta), 0o644); err != nil {
		t.Fatalf("WriteFile(.meta) error = %v", err)
	}

	result := runCLI(t, "meta", "guid", prefabRel, "--project", dir)
	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d stderr=%q", result.exitCode, result.stderr)
	}
	want := "OK guid=3e8a1f2b4c5d6e7f8a9b0c1d2e3f4a5b file=" + prefabAbs + " meta=" + prefabAbs + ".meta\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestMetaGUIDMissingMetaReturnsNeedPrefabGUID(t *testing.T) {
	dir := t.TempDir()
	prefabPath := filepath.Join(dir, "Chair.prefab")
	if err := os.WriteFile(prefabPath, []byte("--- !u!1 &1000\nGameObject:\n  m_Name: Chair\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	result := runCLI(t, "meta", "guid", prefabPath)
	if result.exitCode != 0 {
		t.Fatalf("NEED_PREFAB_GUID must exit 0, got %d stderr=%q", result.exitCode, result.stderr)
	}
	want := "NEED_PREFAB_GUID file=" + prefabPath + " reason=meta_not_found\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestMetaGUIDMissingGUIDEntryReturnsNeedPrefabGUID(t *testing.T) {
	dir := t.TempDir()
	prefabPath := filepath.Join(dir, "Chair.prefab")
	if err := os.WriteFile(prefabPath, []byte("--- !u!1 &1000\nGameObject:\n  m_Name: Chair\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(prefabPath+".meta", []byte("fileFormatVersion: 2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.meta) error = %v", err)
	}

	result := runCLI(t, "meta", "guid", prefabPath)
	if result.exitCode != 0 {
		t.Fatalf("NEED_PREFAB_GUID must exit 0, got %d stderr=%q", result.exitCode, result.stderr)
	}
	want := "NEED_PREFAB_GUID file=" + prefabPath + " reason=guid_missing\n"
	if result.stdout != want {
		t.Fatalf("stdout mismatch: got %q want %q", result.stdout, want)
	}
}

func TestMetaGUIDRejectsUnknownCommandAndFlags(t *testing.T) {
	result := runCLI(t, "meta", "summarize", "Chair.prefab")
	if result.exitCode != 2 {
		t.Fatalf("expected usage error, got %d", result.exitCode)
	}

	result = runCLI(t, "meta", "guid", "Chair.prefab", "--write")
	if result.exitCode != 2 {
		t.Fatalf("expected usage error for --write, got %d", result.exitCode)
	}
}

func TestRefsJSONReturnsPayload(t *testing.T) {
	result := runCLI(t, "prefab", "refs", "testdata/prefabs/enemy.prefab", "--json")
	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d stderr=%q", result.exitCode, result.stderr)
	}

	var got struct {
		Status string `json:"status"`
		Refs   struct {
			References []struct {
				BlockFileID int64  `json:"block_file_id"`
				Class       string `json:"class"`
				Field       string `json:"field"`
				FileID      int64  `json:"file_id"`
				GUID        string `json:"guid"`
				Type        *int   `json:"type"`
			} `json:"references"`
			Warnings int `json:"warnings"`
		} `json:"refs"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}
	if got.Status != "OK" {
		t.Fatalf("status mismatch: got %q", got.Status)
	}
	if len(got.Refs.References) != 4 {
		t.Fatalf("reference count mismatch: got %d want 4", len(got.Refs.References))
	}
	last := got.Refs.References[3]
	if last.Field != "m_Script" || last.GUID != "a1b2c3d4e5f60718293a4b5c6d7e8f90" || last.Type == nil || *last.Type != 3 {
		t.Fatalf("script reference mismatch: %+v", last)
	}
}

func TestRefsJSONCarriesIssueDetail(t *testing.T) {
	result := runCLI(t, "prefab", "refs", "testdata/prefabs/refs_warn.prefab", "--json")
	if result.exitCode != 0 {
		t.Fatalf("WARN refs must exit 0, got %d stderr=%q", result.exitCode, result.stderr)
	}

	var got struct {
		Status string `json:"status"`
		Refs   struct {
			Warnings int `json:"warnings"`
			Issues   []struct {
				Severity string `json:"severity"`
				Code     string `json:"code"`
				FileID   int64  `json:"file_id"`
				Message  string `json:"message"`
			} `json:"issues"`
		} `json:"refs"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse stdout json: %v\nstdout=%q", err, result.stdout)
	}
	if got.Status != "WARN" {
		t.Fatalf("status mismatch: got %q want WARN", got.Status)
	}
	if got.Refs.Warnings != 1 || len(got.Refs.Issues) != 1 {
		t.Fatalf("expected one issue, got warnings=%d issues=%+v", got.Refs.Warnings, got.Refs.Issues)
	}
	issue := got.Refs.Issues[0]
	if issue.Severity != "WARN" || issue.Code != "UNKNOWN_FIELD_SHAPE" || issue.FileID != 11400000 || issue.Message == "" {
		t.Fatalf("issue detail mismatch: %+v", issue)
	}
}

func TestRefsSceneNamespaceWorks(t *testing.T) {
	result := runCLI(t, "scene", "refs", "testdata/scenes/simple_scene.unity")
	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d stderr=%q", result.exitCode, result.stderr)
	}
	if !strings.HasPrefix(result.stdout, "OK refs file=testdata/scenes/simple_scene.unity count=2 warnings=0\n") {
		t.Fatalf("stdout mismatch: got %q", result.stdout)
	}
}

func TestRefsRejectsIrrelevantFlags(t *testing.T) {
	result := runCLI(t, "prefab", "refs", "testdata/prefabs/enemy.prefab", "--id", "1000")
	if result.exitCode != 2 {
		t.Fatalf("expected usage error, got %d stdout=%q", result.exitCode, result.stdout)
	}
}

func TestRefsRejectsUnknownNamespace(t *testing.T) {
	result := runCLI(t, "manifest", "refs", "testdata/prefabs/enemy.prefab")
	if result.exitCode != 1 {
		t.Fatalf("expected error exit, got %d stdout=%q", result.exitCode, result.stdout)
	}
	if !strings.HasPrefix(result.stdout, "") && result.stderr == "" {
		t.Fatalf("expected error output, got stdout=%q stderr=%q", result.stdout, result.stderr)
	}
}

func TestValidateReturnsOKForSoundFile(t *testing.T) {
	result := runCLI(t, "asset", "validate", "testdata/assets/enemy_config.asset")
	if result.exitCode != 0 {
		t.Fatalf("exit code mismatch: got %d stderr=%q", result.exitCode, result.stderr)
	}
	if !strings.HasPrefix(result.stdout, "OK validate file=testdata/assets/enemy_config.asset blocks=1 ") {
		t.Fatalf("stdout mismatch: got %q", result.stdout)
	}
}

func TestValidateWarnsButExitsZero(t *testing.T) {
	result := runCLI(t, "prefab", "validate", "testdata/prefabs/enemy.prefab")
	if result.exitCode != 0 {
		t.Fatalf("WARN must exit 0, got %d stderr=%q", result.exitCode, result.stderr)
	}
	if !strings.HasPrefix(result.stdout, "WARN validate ") {
		t.Fatalf("expected WARN prefix, got %q", result.stdout)
	}
	if !strings.Contains(result.stdout, "WARN code=UNKNOWN_CLASS_ID") {
		t.Fatalf("expected finding line, got %q", result.stdout)
	}
}

func TestValidateErrorsAndExitsOneOnBrokenGraph(t *testing.T) {
	result := runCLI(t, "scene", "validate", "testdata/broken/duplicate_fileid.unity")
	if result.exitCode != 1 {
		t.Fatalf("ERROR must exit 1, got %d stdout=%q", result.exitCode, result.stdout)
	}
	lines := strings.Split(strings.TrimSpace(result.stdout), "\n")
	if !strings.HasPrefix(lines[0], "ERROR validate ") || !strings.Contains(lines[0], "errors=1") {
		t.Fatalf("first line mismatch: got %q", lines[0])
	}
	if lines[1] != "ERROR code=DUPLICATE_FILE_ID file_id=1000 duplicates=2" {
		t.Fatalf("finding line mismatch: got %q", lines[1])
	}
}

func TestValidateJSONCarriesPayload(t *testing.T) {
	result := runCLI(t, "scene", "validate", "testdata/broken/duplicate_fileid.unity", "--json")
	if result.exitCode != 1 {
		t.Fatalf("exit code mismatch: got %d", result.exitCode)
	}
	var got struct {
		Status   string `json:"status"`
		Validate struct {
			Errors   int `json:"errors"`
			Findings []struct {
				Severity string `json:"severity"`
				Code     string `json:"code"`
			} `json:"findings"`
		} `json:"validate"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &got); err != nil {
		t.Fatalf("parse json: %v\nstdout=%q", err, result.stdout)
	}
	if got.Status != "ERROR" || got.Validate.Errors != 1 || len(got.Validate.Findings) != 1 {
		t.Fatalf("payload mismatch: %+v", got)
	}
	if got.Validate.Findings[0].Code != "DUPLICATE_FILE_ID" {
		t.Fatalf("finding code mismatch: %+v", got.Validate.Findings[0])
	}
}

func TestValidateRejectsIrrelevantFlags(t *testing.T) {
	result := runCLI(t, "prefab", "validate", "testdata/prefabs/enemy.prefab", "--id", "1000")
	if result.exitCode != 2 {
		t.Fatalf("expected usage error, got %d stdout=%q", result.exitCode, result.stdout)
	}
}

func TestRestoreRecoversFromBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.asset")
	src := []byte("%YAML 1.1\n" +
		"%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyConfig\n" +
		"  m_Script: {fileID: 11500000, guid: f0e1d2c3b4a5968778695a4b3c2d1e0f, type: 3}\n" +
		"  maxHealth: 200\n")
	if err := os.WriteFile(path, src, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Mutate (creates .bak), then restore.
	w := runCLI(t, "asset", "set", path, "--field", "maxHealth", "--value", "999", "--write")
	if w.exitCode != 0 || !strings.HasPrefix(w.stdout, "WRITE ") {
		t.Fatalf("set --write failed: code=%d stdout=%q", w.exitCode, w.stdout)
	}

	r := runCLI(t, "asset", "restore", path)
	if r.exitCode != 0 {
		t.Fatalf("restore exit code: got %d stderr=%q", r.exitCode, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "OK restore file="+path+" backup="+path+".bak bytes=") || !strings.Contains(r.stdout, "check=OK") {
		t.Fatalf("restore stdout mismatch: got %q", r.stdout)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if string(after) != string(src) {
		t.Fatalf("restore did not recover original content")
	}
}

func TestRestoreErrorsWhenNoBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.asset")
	if err := os.WriteFile(path, []byte("--- !u!114 &1\nMonoBehaviour:\n  m_Name: X\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := runCLI(t, "asset", "restore", path)
	if r.exitCode != 1 {
		t.Fatalf("expected exit 1, got %d stdout=%q", r.exitCode, r.stdout)
	}
	if !strings.HasPrefix(r.stdout, "ERROR restore no backup found backup="+path+".bak") {
		t.Fatalf("stdout mismatch: got %q", r.stdout)
	}
}

func TestRestoreRejectsIrrelevantFlags(t *testing.T) {
	r := runCLI(t, "asset", "restore", "testdata/assets/enemy_config.asset", "--field", "x")
	if r.exitCode != 2 {
		t.Fatalf("expected usage error, got %d stdout=%q", r.exitCode, r.stdout)
	}
}

func TestDepsResolvesAndReportsUnresolved(t *testing.T) {
	root := t.TempDir()
	mkfile := func(rel, content string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	// A material asset with a known GUID + its .meta.
	mkfile("Assets/Materials/Wood.mat", "--- !u!21 &2100000\nMaterial:\n  m_Name: Wood\n")
	mkfile("Assets/Materials/Wood.mat.meta", "fileFormatVersion: 2\nguid: 0123456789abcdef0123456789abcdef\n")
	// A prefab referencing the material GUID plus an unknown GUID.
	prefab := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!1 &1000\nGameObject:\n  m_Name: Box\n  m_Component:\n  - component: {fileID: 4000}\n  - component: {fileID: 2300000}\n" +
		"--- !u!4 &4000\nTransform:\n  m_GameObject: {fileID: 1000}\n  m_Father: {fileID: 0}\n  m_Children: []\n" +
		"--- !u!23 &2300000\nMeshRenderer:\n  m_GameObject: {fileID: 1000}\n  m_Materials:\n  - {fileID: 2100000, guid: 0123456789abcdef0123456789abcdef, type: 2}\n  m_Mesh: {fileID: 4300000, guid: ffffffffffffffffffffffffffffffff, type: 3}\n"
	mkfile("Assets/Prefabs/Box.prefab", prefab)

	r := runCLI(t, "prefab", "deps", filepath.Join(root, "Assets/Prefabs/Box.prefab"), "--project", root)
	if r.exitCode != 0 {
		t.Fatalf("exit code: got %d stderr=%q", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "refs=2 resolved=1 unresolved=1") {
		t.Fatalf("summary mismatch: %q", r.stdout)
	}
	if !strings.Contains(r.stdout, "DEP guid=0123456789abcdef0123456789abcdef path=Assets/Materials/Wood.mat") {
		t.Fatalf("resolved dep missing: %q", r.stdout)
	}
	if !strings.Contains(r.stdout, "DEP guid=ffffffffffffffffffffffffffffffff path=UNKNOWN") {
		t.Fatalf("unresolved dep missing: %q", r.stdout)
	}
}

func TestDepsRequiresProject(t *testing.T) {
	r := runCLI(t, "prefab", "deps", "testdata/prefabs/enemy.prefab")
	if r.exitCode != 1 || !strings.HasPrefix(r.stdout, "ERROR deps requires --project") {
		t.Fatalf("expected requires --project, got code=%d stdout=%q", r.exitCode, r.stdout)
	}
}

func TestDepsWritesDOT(t *testing.T) {
	root := t.TempDir()
	dot := filepath.Join(root, "g.dot")
	r := runCLI(t, "asset", "deps", "testdata/assets/enemy_config.asset", "--project", ".", "--out", dot)
	if r.exitCode != 0 {
		t.Fatalf("exit code: got %d stderr=%q", r.exitCode, r.stderr)
	}
	data, err := os.ReadFile(dot)
	if err != nil {
		t.Fatalf("dot not written: %v", err)
	}
	if !strings.HasPrefix(string(data), "digraph deps {") {
		t.Fatalf("dot content mismatch: %q", string(data))
	}
}

func TestChangesReportsChangedBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.asset")
	src := "%YAML 1.1\n%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!114 &11400000\nMonoBehaviour:\n  m_Name: Cfg\n" +
		"  m_Script: {fileID: 11500000, guid: f0e1d2c3b4a5968778695a4b3c2d1e0f, type: 3}\n  maxHealth: 200\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	w := runCLI(t, "asset", "set", path, "--field", "maxHealth", "--value", "777", "--write")
	if w.exitCode != 0 {
		t.Fatalf("set --write failed: %d %q", w.exitCode, w.stdout)
	}
	r := runCLI(t, "asset", "changes", path)
	if r.exitCode != 0 {
		t.Fatalf("changes exit: %d stderr=%q", r.exitCode, r.stderr)
	}
	if !strings.Contains(r.stdout, "added=0 removed=0 changed=1") {
		t.Fatalf("summary mismatch: %q", r.stdout)
	}
	if !strings.Contains(r.stdout, "CHANGED fileID=11400000 type=MonoBehaviour") {
		t.Fatalf("edit line missing: %q", r.stdout)
	}
}

func TestChangesErrorsWhenNoBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.asset")
	if err := os.WriteFile(path, []byte("--- !u!114 &1\nMonoBehaviour:\n  m_Name: X\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	r := runCLI(t, "asset", "changes", path)
	if r.exitCode != 1 || !strings.HasPrefix(r.stdout, "ERROR changes no backup found") {
		t.Fatalf("expected no-backup error, got %d %q", r.exitCode, r.stdout)
	}
}

func TestChangesRejectsIrrelevantFlags(t *testing.T) {
	r := runCLI(t, "asset", "changes", "testdata/assets/enemy_config.asset", "--field", "x")
	if r.exitCode != 2 {
		t.Fatalf("expected usage error, got %d %q", r.exitCode, r.stdout)
	}
}
