package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Kubonsang/unity-ctx/internal/parser"
)

const (
	schemaVersion = 1
	generatedBy   = "unity-ctx 0.2.0"
)

type Snapshot struct {
	SchemaVersion int          `json:"schema_version"`
	Kind          string       `json:"kind"`
	Path          string       `json:"path"`
	FileHash      string       `json:"file_hash"`
	GeneratedBy   string       `json:"generated_by"`
	Objects       []ObjectStub `json:"objects"`
}

type ObjectStub struct {
	FileID   int64  `json:"file_id"`
	ClassID  int    `json:"class_id"`
	TypeName string `json:"type_name"`
	Name     string `json:"name,omitempty"`
}

func FileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return FileHashBytes(data), nil
}

func FileHashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func BuildSnapshot(kind, path string, blocks []parser.Block) (Snapshot, error) {
	hash, err := FileHash(path)
	if err != nil {
		return Snapshot{}, err
	}

	return buildSnapshot(kind, path, hash, blocks), nil
}

func BuildSnapshotFromData(kind, path string, data []byte, blocks []parser.Block) (Snapshot, error) {
	return buildSnapshot(kind, path, FileHashBytes(data), blocks), nil
}

func buildSnapshot(kind, path, hash string, blocks []parser.Block) Snapshot {

	objects := make([]ObjectStub, 0, len(blocks))
	for _, block := range blocks {
		object := ObjectStub{
			FileID:   block.FileID,
			ClassID:  block.ClassID,
			TypeName: block.TypeName,
		}

		name, ok := block.Fields["m_Name"].(string)
		if ok && name != "" {
			object.Name = name
		}

		objects = append(objects, object)
	}

	return Snapshot{
		SchemaVersion: schemaVersion,
		Kind:          kind,
		Path:          canonicalPath(path),
		FileHash:      hash,
		GeneratedBy:   generatedBy,
		Objects:       objects,
	}
}

func Save(path string, snapshot Snapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func Load(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, err
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	if err := validateSnapshot(snapshot); err != nil {
		return Snapshot{}, err
	}

	return snapshot, nil
}

func IsStale(snapshot Snapshot, sourcePath string) (bool, string, error) {
	if snapshot.SchemaVersion != schemaVersion {
		return true, "schema_version_mismatch", nil
	}
	if canonicalPath(sourcePath) != snapshot.Path {
		return true, "path_mismatch", nil
	}

	hash, err := FileHash(sourcePath)
	if err != nil {
		return false, "", err
	}

	if hash != snapshot.FileHash {
		return true, "file_hash_mismatch", nil
	}

	return false, "", nil
}

func canonicalPath(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	return filepath.Clean(absPath)
}

func validateSnapshot(snapshot Snapshot) error {
	switch {
	case snapshot.SchemaVersion == 0:
		return fmt.Errorf("missing schema_version")
	case snapshot.Kind == "":
		return fmt.Errorf("missing kind")
	case snapshot.Path == "":
		return fmt.Errorf("missing path")
	case snapshot.FileHash == "":
		return fmt.Errorf("missing file_hash")
	case snapshot.GeneratedBy == "":
		return fmt.Errorf("missing generated_by")
	case snapshot.Objects == nil:
		return fmt.Errorf("missing objects")
	default:
		return nil
	}
}
