package deps

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMeta(t *testing.T, root, rel, guid string) {
	t.Helper()
	assetPath := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(assetPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(assetPath, []byte("--- !u!1 &1\n"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	meta := "fileFormatVersion: 2\nguid: " + guid + "\n"
	if err := os.WriteFile(assetPath+".meta", []byte(meta), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}
}

func TestBuildIndexMapsGUIDToRelativePath(t *testing.T) {
	root := t.TempDir()
	writeMeta(t, root, "Assets/Prefabs/Chair.prefab", "a1b2c3d4e5f60718293a4b5c6d7e8f90")
	writeMeta(t, root, "Assets/Materials/Wood.mat", "0123456789abcdef0123456789abcdef")

	index, err := BuildIndex(root)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if got := index["a1b2c3d4e5f60718293a4b5c6d7e8f90"]; got != "Assets/Prefabs/Chair.prefab" {
		t.Fatalf("chair path mismatch: %q", got)
	}
	if got := index["0123456789abcdef0123456789abcdef"]; got != "Assets/Materials/Wood.mat" {
		t.Fatalf("mat path mismatch: %q", got)
	}
}

func TestResolveDedupsSortsAndFlagsUnresolved(t *testing.T) {
	index := map[string]string{
		"bbbb": "Assets/B.prefab",
		"aaaa": "Assets/A.mat",
	}
	got := Resolve(index, []string{"bbbb", "aaaa", "bbbb", "cccc", ""})
	if len(got) != 3 {
		t.Fatalf("expected 3 unique, got %d: %+v", len(got), got)
	}
	// sorted by GUID: aaaa, bbbb, cccc
	if got[0].GUID != "aaaa" || got[0].Path != "Assets/A.mat" || !got[0].Resolved {
		t.Fatalf("res[0] mismatch: %+v", got[0])
	}
	if got[1].GUID != "bbbb" || !got[1].Resolved {
		t.Fatalf("res[1] mismatch: %+v", got[1])
	}
	if got[2].GUID != "cccc" || got[2].Resolved || got[2].Path != "" {
		t.Fatalf("res[2] should be unresolved: %+v", got[2])
	}
}
