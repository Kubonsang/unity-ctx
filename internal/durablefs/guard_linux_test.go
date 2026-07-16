//go:build linux

package durablefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxDirectoryGuardTracksRenamedAncestorForRecovery(t *testing.T) {
	root := t.TempDir()
	ancestor := filepath.Join(root, "Assets")
	directory := filepath.Join(ancestor, "SpatialContracts")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	guard, err := GuardDirectoryTree(root, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if err := os.WriteFile(guard.Path("backup.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "MovedAssets")
	if err := os.Rename(ancestor, moved); err != nil {
		t.Fatal(err)
	}
	if err := guard.VerifyPath(directory); err == nil {
		t.Fatal("guard accepted the old logical path after an ancestor rename")
	}
	recovery, err := guard.ResolvePath("backup.json")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(moved, "SpatialContracts", "backup.json")
	if filepath.Clean(recovery) != filepath.Clean(want) {
		t.Fatalf("recovery path=%s want=%s", recovery, want)
	}
	if _, err := os.Stat(recovery); err != nil {
		t.Fatalf("resolved recovery file is unavailable: %v", err)
	}
}
