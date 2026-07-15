//go:build linux || windows

package durablefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDirectoryTreeCreatesNestedPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "ledger", "approvals", "v2")
	synced := make(map[string]bool)
	if err := EnsureDirectoryTree(target, 0o700, func(path string) error {
		synced[filepath.Clean(path)] = true
		return SyncDirectory(path)
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(target)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("secure directory tree was not created as ordinary directories: info=%v err=%v", info, err)
	}
	if !synced[filepath.Clean(root)] || !synced[filepath.Join(root, "ledger")] || !synced[filepath.Join(root, "ledger", "approvals")] {
		t.Fatalf("created parents were not durably synchronized: %v", synced)
	}
}

func TestEnsureDirectoryTreeRejectsExistingNonDirectory(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "ledger")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := EnsureDirectoryTree(filepath.Join(blocker, "approvals"), 0o700, SyncDirectory)
	if err == nil {
		t.Fatal("secure directory creation accepted a non-directory ancestor")
	}
}

func TestEnsureDirectoryTreeRejectsExistingSymlink(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	link := filepath.Join(root, "ledger")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symlink creation is unavailable for this Windows account: %v", err)
	}
	err := EnsureDirectoryTree(filepath.Join(link, "approvals"), 0o700, SyncDirectory)
	if err == nil {
		t.Fatal("secure directory creation followed a symlink ancestor")
	}
	if _, err := os.Stat(filepath.Join(external, "approvals")); !os.IsNotExist(err) {
		t.Fatalf("secure directory creation modified the symlink target: %v", err)
	}
}
