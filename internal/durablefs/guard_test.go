//go:build linux || windows

package durablefs

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirectoryGuardSupportsDurableChildPublication(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "Assets", "SpatialContracts")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	guard, err := GuardDirectoryTree(root, directory)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	path := guard.Path("probe.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := guard.Sync(); err != nil {
		t.Fatalf("directory metadata sync is unsupported at runtime: %v", err)
	}
	if err := guard.VerifyPath(directory); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := os.Rename(directory, directory+"-moved"); err == nil {
			t.Fatal("Windows directory guard allowed an ancestor rename while the transaction was active")
		}
	}
}
