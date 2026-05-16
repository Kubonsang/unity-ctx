package index

import (
	"os"
	"path/filepath"
	"testing"

	"unity-ctx/internal/parser"
)

func TestBuildSnapshotIncludesFileHashAndObjects(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	blocks, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	snapshot, err := BuildSnapshot("scene", path, blocks)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}

	if snapshot.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion mismatch: got %d want 1", snapshot.SchemaVersion)
	}
	if snapshot.Kind != "scene" {
		t.Fatalf("Kind mismatch: got %q want %q", snapshot.Kind, "scene")
	}
	if snapshot.Path != canonicalPath(path) {
		t.Fatalf("Path mismatch: got %q want %q", snapshot.Path, canonicalPath(path))
	}
	if snapshot.FileHash == "" {
		t.Fatal("expected file hash to be populated")
	}
	if len(snapshot.Objects) != 4 {
		t.Fatalf("object count mismatch: got %d want 4", len(snapshot.Objects))
	}
	first := snapshot.Objects[0]
	if first.FileID != 1000 {
		t.Fatalf("first object FileID mismatch: got %d want 1000", first.FileID)
	}
	if first.ClassID != 1 {
		t.Fatalf("first object ClassID mismatch: got %d want 1", first.ClassID)
	}
	if first.TypeName != "GameObject" {
		t.Fatalf("first object TypeName mismatch: got %q want %q", first.TypeName, "GameObject")
	}
	if first.Name != "Table_01" {
		t.Fatalf("first object Name mismatch: got %q want %q", first.Name, "Table_01")
	}

	objectsByName := make(map[string]ObjectStub, len(snapshot.Objects))
	for _, object := range snapshot.Objects {
		if object.Name != "" {
			objectsByName[object.Name] = object
		}
	}

	chair, ok := objectsByName["Chair_01"]
	if !ok {
		t.Fatal("expected named object mapping for Chair_01")
	}
	if chair.FileID != 2000 {
		t.Fatalf("Chair_01 FileID mismatch: got %d want 2000", chair.FileID)
	}
	if chair.ClassID != 1 {
		t.Fatalf("Chair_01 ClassID mismatch: got %d want 1", chair.ClassID)
	}
	if chair.TypeName != "GameObject" {
		t.Fatalf("Chair_01 TypeName mismatch: got %q want %q", chair.TypeName, "GameObject")
	}
}

func TestBuildSnapshotNormalizesPath(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	invocationPath := filepath.FromSlash("../../testdata/scenes/./simple_scene.unity")

	blocks, err := parser.ParseFile(sourcePath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	snapshot, err := BuildSnapshot("scene", invocationPath, blocks)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}

	if snapshot.Path != canonicalPath(invocationPath) {
		t.Fatalf("Path mismatch: got %q want %q", snapshot.Path, canonicalPath(invocationPath))
	}
}

func TestIsStaleDetectsFileHashMismatch(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	snapshot := Snapshot{
		SchemaVersion: 1,
		Kind:          "scene",
		Path:          canonicalPath(path),
		FileHash:      "sha256:old",
	}

	stale, reason, err := IsStale(snapshot, path)
	if err != nil {
		t.Fatalf("IsStale() error = %v", err)
	}
	if !stale {
		t.Fatal("expected snapshot to be stale")
	}
	if reason != "file_hash_mismatch" {
		t.Fatalf("reason mismatch: got %q want %q", reason, "file_hash_mismatch")
	}
}

func TestIsStaleDetectsSchemaVersionMismatch(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	hash, err := FileHash(path)
	if err != nil {
		t.Fatalf("FileHash() error = %v", err)
	}

	snapshot := Snapshot{
		SchemaVersion: schemaVersion + 1,
		Kind:          "scene",
		Path:          path,
		FileHash:      hash,
	}

	stale, reason, err := IsStale(snapshot, path)
	if err != nil {
		t.Fatalf("IsStale() error = %v", err)
	}
	if !stale {
		t.Fatal("expected snapshot to be stale")
	}
	if reason != "schema_version_mismatch" {
		t.Fatalf("reason mismatch: got %q want %q", reason, "schema_version_mismatch")
	}
}

func TestIsStaleDetectsNormalizedPathMismatch(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	copiedPath := filepath.Join(t.TempDir(), "simple_scene.unity")
	if err := os.WriteFile(copiedPath, data, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	hash, err := FileHash(copiedPath)
	if err != nil {
		t.Fatalf("FileHash() error = %v", err)
	}

	snapshot := Snapshot{
		SchemaVersion: schemaVersion,
		Kind:          "scene",
		Path:          canonicalPath(sourcePath),
		FileHash:      hash,
	}

	stale, reason, err := IsStale(snapshot, copiedPath)
	if err != nil {
		t.Fatalf("IsStale() error = %v", err)
	}
	if !stale {
		t.Fatal("expected snapshot to be stale")
	}
	if reason != "path_mismatch" {
		t.Fatalf("reason mismatch: got %q want %q", reason, "path_mismatch")
	}
}

func TestSaveLoadRoundTripUsesTrailingNewline(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "scenes", "simple_scene.unity")

	blocks, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	snapshot, err := BuildSnapshot("scene", path, blocks)
	if err != nil {
		t.Fatalf("BuildSnapshot() error = %v", err)
	}

	out := filepath.Join(t.TempDir(), "simple_scene.index.json")
	if err := Save(out, snapshot); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatal("expected saved JSON to end with newline")
	}

	loaded, err := Load(out)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.SchemaVersion != snapshot.SchemaVersion {
		t.Fatalf("SchemaVersion mismatch: got %d want %d", loaded.SchemaVersion, snapshot.SchemaVersion)
	}
	if loaded.Kind != snapshot.Kind {
		t.Fatalf("Kind mismatch: got %q want %q", loaded.Kind, snapshot.Kind)
	}
	if loaded.Path != snapshot.Path {
		t.Fatalf("Path mismatch: got %q want %q", loaded.Path, snapshot.Path)
	}
	if loaded.FileHash != snapshot.FileHash {
		t.Fatalf("FileHash mismatch: got %q want %q", loaded.FileHash, snapshot.FileHash)
	}
	if loaded.GeneratedBy != snapshot.GeneratedBy {
		t.Fatalf("GeneratedBy mismatch: got %q want %q", loaded.GeneratedBy, snapshot.GeneratedBy)
	}
	if len(loaded.Objects) != len(snapshot.Objects) {
		t.Fatalf("object count mismatch: got %d want %d", len(loaded.Objects), len(snapshot.Objects))
	}
}

func TestLoadRejectsMissingRequiredFields(t *testing.T) {
	out := filepath.Join(t.TempDir(), "invalid.index.json")
	if err := os.WriteFile(out, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := Load(out); err == nil {
		t.Fatal("expected Load() to reject invalid snapshot")
	}
}

func TestBuildSnapshotFromDataUsesProvidedBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scene.unity")
	initial := "" +
		"%YAML 1.1\n" +
		"--- !u!1 &1000\n" +
		"GameObject:\n" +
		"  m_Name: Root\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	blocks, err := parser.Parse([]byte(initial))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	updated := initial +
		"--- !u!4 &2000\n" +
		"Transform:\n" +
		"  m_GameObject: {fileID: 1000}\n"
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	snapshot, err := BuildSnapshotFromData("scene", path, []byte(initial), blocks)
	if err != nil {
		t.Fatalf("BuildSnapshotFromData() error = %v", err)
	}

	wantHash := FileHashBytes([]byte(initial))
	if snapshot.FileHash != wantHash {
		t.Fatalf("FileHash mismatch: got %q want %q", snapshot.FileHash, wantHash)
	}
	if len(snapshot.Objects) != 1 {
		t.Fatalf("object count mismatch: got %d want 1", len(snapshot.Objects))
	}
}
