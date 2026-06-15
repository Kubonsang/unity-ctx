package mutation

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kubonsang/unity-ctx/internal/parser"
)

func TestPlanAssetSetDryRunPreservesSourceAndReportsTypeHint(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyConfig\n" +
		"  maxHealth: 200\n")

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := PlanAssetSet(input, blocks, AssetSetRequest{
		Path:  "enemy_config.asset",
		Field: "maxHealth",
		Value: "300",
	})
	if err != nil {
		t.Fatalf("PlanAssetSet() error = %v", err)
	}
	if string(result.UpdatedData) != string(input) {
		t.Fatal("dry-run plan should not write through to input bytes")
	}
	if result.TypeHint != "int" {
		t.Fatalf("TypeHint mismatch: got %q want %q", result.TypeHint, "int")
	}
	if result.OldValue != "200" || result.NewValue != "300" {
		t.Fatalf("value mismatch: got old=%q new=%q", result.OldValue, result.NewValue)
	}
	if !result.Changed {
		t.Fatal("Changed mismatch: got false want true")
	}
}

func TestPlanAssetSetRewriteProducesUpdatedBytes(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyConfig\n" +
		"  moveSpeed: 3.5\n")

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := PlanAssetSet(input, blocks, AssetSetRequest{
		Path:    "enemy_config.asset",
		Field:   "moveSpeed",
		Value:   "4.0",
		Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanAssetSet() error = %v", err)
	}
	if !strings.Contains(string(result.UpdatedData), "  moveSpeed: 4.0\n") {
		t.Fatalf("updated YAML mismatch:\n%s", string(result.UpdatedData))
	}
	if result.TypeHint != "float" {
		t.Fatalf("TypeHint mismatch: got %q want %q", result.TypeHint, "float")
	}
}

func TestPlanAssetSetRewritePreservesOriginalNewlineStyle(t *testing.T) {
	tests := []struct {
		name    string
		newline string
	}{
		{name: "crlf", newline: "\r\n"},
		{name: "cr", newline: "\r"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(strings.Join([]string{
				"%YAML 1.1",
				"--- !u!114 &11400000",
				"MonoBehaviour:",
				"  m_Name: EnemyConfig",
				"  maxHealth: 200",
				"",
			}, tc.newline))

			blocks, err := parser.Parse(input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			result, err := PlanAssetSet(input, blocks, AssetSetRequest{
				Path:    "enemy_config.asset",
				Field:   "maxHealth",
				Value:   "300",
				Rewrite: true,
			})
			if err != nil {
				t.Fatalf("PlanAssetSet() error = %v", err)
			}

			want := []byte(strings.Join([]string{
				"%YAML 1.1",
				"--- !u!114 &11400000",
				"MonoBehaviour:",
				"  m_Name: EnemyConfig",
				"  maxHealth: 300",
				"",
			}, tc.newline))
			if string(result.UpdatedData) != string(want) {
				t.Fatalf("updated YAML mismatch:\n%q", string(result.UpdatedData))
			}
		})
	}
}

func TestPlanAssetSetMissingFieldReturnsError(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyConfig\n" +
		"  maxHealth: 200\n")

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, err = PlanAssetSet(input, blocks, AssetSetRequest{
		Path:  "enemy_config.asset",
		Field: "armor",
		Value: "300",
	})
	if err == nil {
		t.Fatal("expected PlanAssetSet() to reject missing field")
	}
	if got := err.Error(); got != "FIELD_NOT_FOUND field=armor" {
		t.Fatalf("error mismatch: got %q want %q", got, "FIELD_NOT_FOUND field=armor")
	}
}

func TestPlanAssetSetUsesExplicitIDTarget(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 200\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigB\n" +
		"  maxHealth: 350\n")

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := PlanAssetSet(input, blocks, AssetSetRequest{
		Path:    "multi.asset",
		HasID:   true,
		ID:      11400001,
		Field:   "maxHealth",
		Value:   "400",
		Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanAssetSet() error = %v", err)
	}
	want := "" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigA\n" +
		"  maxHealth: 200\n" +
		"--- !u!114 &11400001\n" +
		"MonoBehaviour:\n" +
		"  m_Name: ConfigB\n" +
		"  maxHealth: 400\n"
	if string(result.UpdatedData) != want {
		t.Fatalf("updated YAML mismatch:\n%s", string(result.UpdatedData))
	}
	if result.OldValue != "350" {
		t.Fatalf("OldValue mismatch: got %q want %q", result.OldValue, "350")
	}
}

func TestPlanAssetSetUnchangedValueKeepsBytes(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyConfig\n" +
		"  maxHealth: 200\n")

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := PlanAssetSet(input, blocks, AssetSetRequest{
		Path:  "enemy_config.asset",
		Field: "maxHealth",
		Value: "200",
	})
	if err != nil {
		t.Fatalf("PlanAssetSet() error = %v", err)
	}
	if string(result.UpdatedData) != string(input) {
		t.Fatal("unchanged update should preserve bytes")
	}
	if result.Changed {
		t.Fatal("Changed mismatch: got true want false")
	}
}

func TestPlanAssetSetUnchangedFloatUsesRenderedSourceValue(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyConfig\n" +
		"  moveSpeed: 4.0\n")

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := PlanAssetSet(input, blocks, AssetSetRequest{
		Path:    "enemy_config.asset",
		Field:   "moveSpeed",
		Value:   "4.00",
		Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanAssetSet() error = %v", err)
	}
	if result.Changed {
		t.Fatal("Changed mismatch: got true want false")
	}
	if result.NewValue != "4.0" {
		t.Fatalf("NewValue mismatch: got %q want %q", result.NewValue, "4.0")
	}
	if string(result.UpdatedData) != string(input) {
		t.Fatal("unchanged float update should preserve bytes")
	}
}

func TestWriteWithBackupCreatesBackupAndWritesUpdatedBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "enemy_config.asset")
	original := []byte("%YAML 1.1\n--- !u!114 &11400000\nMonoBehaviour:\n  maxHealth: 200\n")
	updated := []byte("%YAML 1.1\n--- !u!114 &11400000\nMonoBehaviour:\n  maxHealth: 300\n")

	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	backupPath, err := WriteWithBackup(path, updated)
	if err != nil {
		t.Fatalf("WriteWithBackup() error = %v", err)
	}
	if backupPath != path+".bak" {
		t.Fatalf("backup path mismatch: got %q want %q", backupPath, path+".bak")
	}

	gotOriginal, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(gotOriginal) != string(original) {
		t.Fatalf("backup contents mismatch:\n%s", string(gotOriginal))
	}

	gotUpdated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(path) error = %v", err)
	}
	if string(gotUpdated) != string(updated) {
		t.Fatalf("updated contents mismatch:\n%s", string(gotUpdated))
	}
}

func TestWriteWithBackupRenameFailureKeepsOriginalAndCleansTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "enemy_config.asset")
	original := []byte("%YAML 1.1\n--- !u!114 &11400000\nMonoBehaviour:\n  maxHealth: 200\n")
	updated := []byte("%YAML 1.1\n--- !u!114 &11400000\nMonoBehaviour:\n  maxHealth: 300\n")

	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	originalRename := renameFile
	renameFile = func(oldPath, newPath string) error {
		if newPath == path {
			return errors.New("rename blocked")
		}
		return originalRename(oldPath, newPath)
	}
	t.Cleanup(func() {
		renameFile = originalRename
	})

	backupPath, err := WriteWithBackup(path, updated)
	if err == nil {
		t.Fatal("expected WriteWithBackup() to fail when rename fails")
	}
	if backupPath != "" {
		t.Fatalf("backup path mismatch on error: got %q want empty", backupPath)
	}

	gotOriginal, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(path) error = %v", readErr)
	}
	if string(gotOriginal) != string(original) {
		t.Fatalf("original file changed on rename failure:\n%s", string(gotOriginal))
	}

	gotBackup, readErr := os.ReadFile(path + ".bak")
	if readErr != nil {
		t.Fatalf("ReadFile(backup) error = %v", readErr)
	}
	if string(gotBackup) != string(original) {
		t.Fatalf("backup contents mismatch:\n%s", string(gotBackup))
	}

	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("ReadDir() error = %v", readErr)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	if got := strings.Join(names, ","); got != "enemy_config.asset,enemy_config.asset.bak" &&
		got != "enemy_config.asset.bak,enemy_config.asset" {
		t.Fatalf("unexpected directory contents after failed write: %q", got)
	}
}

func TestWriteWithBackupSyncFailureReportsCommittedWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "enemy_config.asset")
	original := []byte("%YAML 1.1\n--- !u!114 &11400000\nMonoBehaviour:\n  maxHealth: 200\n")
	updated := []byte("%YAML 1.1\n--- !u!114 &11400000\nMonoBehaviour:\n  maxHealth: 300\n")

	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	originalSyncDirectory := syncDirectoryFn
	syncCalls := 0
	syncDirectoryFn = func(path string) error {
		syncCalls++
		if syncCalls == 2 {
			return errors.New("directory sync blocked")
		}
		return originalSyncDirectory(path)
	}
	t.Cleanup(func() {
		syncDirectoryFn = originalSyncDirectory
	})

	backupPath, err := WriteWithBackup(path, updated)
	if err == nil {
		t.Fatal("expected WriteWithBackup() to fail when directory sync fails")
	}
	if backupPath != path+".bak" {
		t.Fatalf("backup path mismatch: got %q want %q", backupPath, path+".bak")
	}

	var committedErr *CommittedWriteError
	if !errors.As(err, &committedErr) {
		t.Fatalf("error type mismatch: got %T want *CommittedWriteError", err)
	}
	if !committedErr.WriteCommitted() {
		t.Fatal("WriteCommitted mismatch: got false want true")
	}
	if committedErr.Path != path {
		t.Fatalf("committed path mismatch: got %q want %q", committedErr.Path, path)
	}

	gotUpdated, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile(path) error = %v", readErr)
	}
	if string(gotUpdated) != string(updated) {
		t.Fatalf("updated contents mismatch:\n%s", string(gotUpdated))
	}

	gotBackup, readErr := os.ReadFile(backupPath)
	if readErr != nil {
		t.Fatalf("ReadFile(backup) error = %v", readErr)
	}
	if string(gotBackup) != string(original) {
		t.Fatalf("backup contents mismatch:\n%s", string(gotBackup))
	}
}

func TestPlanAssetSetRewriteAllowsMaterialFiles(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"%TAG !u! tag:unity3d.com,2011:\n" +
		"--- !u!21 &2100000\n" +
		"Material:\n" +
		"  m_Name: Stone\n" +
		"  m_CustomRenderQueue: -1\n")

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	result, err := PlanAssetSet(input, blocks, AssetSetRequest{
		Path:    "material.mat",
		Field:   "m_CustomRenderQueue",
		Value:   "2500",
		Rewrite: true,
	})
	if err != nil {
		t.Fatalf("PlanAssetSet() error = %v", err)
	}
	if !strings.Contains(string(result.UpdatedData), "  m_CustomRenderQueue: 2500\n") {
		t.Fatalf("updated YAML mismatch:\n%s", string(result.UpdatedData))
	}
	if result.TypeHint != "int" {
		t.Fatalf("TypeHint mismatch: got %q want %q", result.TypeHint, "int")
	}
}

func TestPlanAssetSetRejectsUnsupportedFileKind(t *testing.T) {
	input := []byte("" +
		"%YAML 1.1\n" +
		"--- !u!114 &11400000\n" +
		"MonoBehaviour:\n" +
		"  m_Name: EnemyConfig\n" +
		"  maxHealth: 200\n")

	blocks, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	_, err = PlanAssetSet(input, blocks, AssetSetRequest{
		Path:  "enemy.prefab",
		Field: "maxHealth",
		Value: "300",
	})
	if err == nil {
		t.Fatal("expected PlanAssetSet() to reject unsupported file kinds")
	}
	if got := err.Error(); got != "UNSUPPORTED_FILE_KIND kind=.prefab allowed=.asset,.mat" {
		t.Fatalf("error mismatch: got %q want %q", got, "UNSUPPORTED_FILE_KIND kind=.prefab allowed=.asset,.mat")
	}
}

func TestPlanAssetSetRewriteStringLookingScalarsStayStrings(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		wantRendered string
	}{
		{name: "bool true", value: "true", wantRendered: `"true"`},
		{name: "bool false", value: "false", wantRendered: `"false"`},
		{name: "int", value: "123", wantRendered: `"123"`},
		{name: "float", value: "3.5", wantRendered: `"3.5"`},
		{name: "quoted", value: `say "hi"`, wantRendered: `"say \"hi\""`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte("" +
				"%YAML 1.1\n" +
				"--- !u!114 &11400000\n" +
				"MonoBehaviour:\n" +
				"  m_Name: EnemyConfig\n" +
				"  label: starter\n")

			blocks, err := parser.Parse(input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			result, err := PlanAssetSet(input, blocks, AssetSetRequest{
				Path:    "enemy_config.asset",
				Field:   "label",
				Value:   tc.value,
				Rewrite: true,
			})
			if err != nil {
				t.Fatalf("PlanAssetSet() error = %v", err)
			}
			if result.TypeHint != "string" {
				t.Fatalf("TypeHint mismatch: got %q want %q", result.TypeHint, "string")
			}
			if result.NewValue != tc.wantRendered {
				t.Fatalf("NewValue mismatch: got %q want %q", result.NewValue, tc.wantRendered)
			}
			if !strings.Contains(string(result.UpdatedData), "  label: "+tc.wantRendered+"\n") {
				t.Fatalf("updated YAML mismatch:\n%s", string(result.UpdatedData))
			}

			updatedBlocks, err := parser.Parse(result.UpdatedData)
			if err != nil {
				t.Fatalf("Parse(updated) error = %v", err)
			}
			gotValue, ok := updatedBlocks[0].Fields["label"]
			if !ok {
				t.Fatal("updated field missing")
			}
			gotString, ok := gotValue.(string)
			if !ok {
				t.Fatalf("updated field type mismatch: got %T want string", gotValue)
			}
			if gotString != tc.value {
				t.Fatalf("updated field value mismatch: got %q want %q", gotString, tc.value)
			}
		})
	}
}

func TestPlanAssetSetRewriteAllowsEmptyStringScalarFields(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		inputLine string
		value     string
		wantLine  string
	}{
		{
			name:      "label",
			field:     "label",
			inputLine: "  label:",
			value:     "starter",
			wantLine:  "  label: starter\n",
		},
		{
			name:      "m_Name",
			field:     "m_Name",
			inputLine: "  m_Name:",
			value:     "EnemyConfig",
			wantLine:  "  m_Name: EnemyConfig\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte("" +
				"%YAML 1.1\n" +
				"--- !u!114 &11400000\n" +
				"MonoBehaviour:\n" +
				tc.inputLine + "\n" +
				"  maxHealth: 200\n")

			blocks, err := parser.Parse(input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			result, err := PlanAssetSet(input, blocks, AssetSetRequest{
				Path:    "enemy_config.asset",
				Field:   tc.field,
				Value:   tc.value,
				Rewrite: true,
			})
			if err != nil {
				t.Fatalf("PlanAssetSet() error = %v", err)
			}
			if result.TypeHint != "string" {
				t.Fatalf("TypeHint mismatch: got %q want %q", result.TypeHint, "string")
			}
			if !strings.Contains(string(result.UpdatedData), tc.wantLine) {
				t.Fatalf("updated YAML mismatch:\n%s", string(result.UpdatedData))
			}
		})
	}
}
