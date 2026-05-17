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
	manifest := "" +
		"{\n" +
		"  \"scene\": \"" + scenePath + "\",\n" +
		"  \"source\": \"editor\",\n" +
		"  \"version\": 1,\n" +
		"  \"objects\": [\n" +
		"    {\n" +
		"      \"fileID\": 3000,\n" +
		"      \"name\": \"ObjectC\",\n" +
		"      \"bounds\": {\"center\": [1.6, 0.5, 0.0], \"size\": [1.0, 1.0, 1.0]}\n" +
		"    },\n" +
		"    {\n" +
		"      \"fileID\": 1000,\n" +
		"      \"name\": \"ObjectA\",\n" +
		"      \"bounds\": {\"center\": [0.0, 0.5, 0.0], \"size\": [1.0, 1.0, 1.0]}\n" +
		"    },\n" +
		"    {\n" +
		"      \"fileID\": 2000,\n" +
		"      \"name\": \"ObjectB\",\n" +
		"      \"bounds\": {\"center\": [0.8, 0.5, 0.0], \"size\": [1.0, 1.0, 1.0]}\n" +
		"    }\n" +
		"  ],\n" +
		"  \"prefabs\": [\n" +
		"    {\n" +
		"      \"path\": \"Assets/Prefabs/chair.prefab\",\n" +
		"      \"bounds\": {\"center\": [0.0, 0.5, 0.0], \"size\": [1.2, 1.0, 1.0]}\n" +
		"    }\n" +
		"  ]\n" +
		"}\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
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
	manifest := "" +
		"{\n" +
		"  \"scene\": \"testdata/scenes/other_scene.unity\",\n" +
		"  \"source\": \"editor\",\n" +
		"  \"version\": 1,\n" +
		"  \"objects\": [],\n" +
		"  \"prefabs\": [\n" +
		"    {\n" +
		"      \"path\": \"Assets/Prefabs/chair.prefab\",\n" +
		"      \"bounds\": {\"center\": [0.0, 0.5, 0.0], \"size\": [0.8, 1.0, 0.8]}\n" +
		"    }\n" +
		"  ]\n" +
		"}\n"
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
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

	want := "ERROR manifest scene mismatch file=" + scenePath + " manifest_scene=testdata/scenes/other_scene.unity\n"
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

	want := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1\n"
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

	want := "DRY_RUN patch=testdata/patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1\n"
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

	want := "WRITE backup=" + scenePath + ".bak patch=testdata/patches/chair_place_ok.patch.json op=place_prefab append_ops=2 changed=1 verified=1\n"
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

	want := "WRITE backup=" + path + ".bak field=maxHealth old=200 new=300 type_hint=int changed=1 verified=1\n"
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

	want := "OK field=maxHealth old=200 new=200 type_hint=int changed=0 verified=1\n"
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

	want := "WRITE backup=" + path + ".bak field=label old=starter new=\"001\" type_hint=string changed=1 verified=1\n"
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

	want := "DRY_RUN field=label old=starter new=\"\" type_hint=string changed=1\n"
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
		"  m_Name: ConfigA\n" +
		"  maxHealth: 100\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
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

	want := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1\n"
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
	wantBody := "DRY_RUN field=maxHealth old=200 new=300 type_hint=int changed=1"
	if got.Body != wantBody {
		t.Fatalf("body mismatch: got %q want %q", got.Body, wantBody)
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

	binary := buildCLI(t)
	cmd := exec.Command(binary, args...)
	cmd.Dir = repoRoot(t)

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
