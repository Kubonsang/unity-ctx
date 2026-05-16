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

	"unity-ctx/internal/contextpack"
)

func TestMainMissingFileArgument(t *testing.T) {
	result := runCLI(t, "scene", "summarize")

	if result.exitCode != 1 {
		t.Fatalf("exit code mismatch: got %d want 1", result.exitCode)
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
		name     string
		args     []string
		wantErr  string
	}{
		{
			name: "summarize rejects task",
			args: []string{"scene", "summarize", "testdata/scenes/simple_scene.unity", "--task", "inspect props"},
			wantErr: "ERROR summarize does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name: "query rejects focus",
			args: []string{"scene", "query", "testdata/scenes/simple_scene.unity", "--id", "2000", "--focus", "Chair_01"},
			wantErr: "ERROR query does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name: "inspect rejects max tokens",
			args: []string{"prefab", "inspect", "testdata/prefabs/enemy.prefab", "--component", "NavMeshAgent", "--max-tokens", "32"},
			wantErr: "ERROR inspect does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name: "get rejects task",
			args: []string{"asset", "get", "testdata/assets/enemy_config.asset", "--field", "maxHealth", "--task", "read health"},
			wantErr: "ERROR get does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name: "index rejects focus",
			args: []string{"scene", "index", "testdata/scenes/simple_scene.unity", "--out", "ignored.index.json", "--focus", "Chair_01"},
			wantErr: "ERROR index does not accept --task, --focus, or --max-tokens\n",
		},
		{
			name: "summarize rejects explicit default max tokens",
			args: []string{"scene", "summarize", "testdata/scenes/simple_scene.unity", "--max-tokens", "256"},
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
