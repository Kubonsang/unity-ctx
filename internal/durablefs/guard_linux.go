//go:build linux

package durablefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// DirectoryGuard resolves child operations through an open directory file
// descriptor, so later ancestor renames or symlink swaps cannot redirect them.
type DirectoryGuard struct {
	fd        int
	stableDir string
}

func GuardDirectoryTree(root, directory string) (*DirectoryGuard, error) {
	if !pathWithin(root, directory) {
		return nil, errors.New("guarded directory escapes its trusted root")
	}
	fd, err := syscall.Open(directory, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open guarded destination directory: %w", err)
	}
	guard := &DirectoryGuard{fd: fd, stableDir: filepath.Join("/proc/self/fd", strconv.Itoa(fd))}
	if err := guard.VerifyPath(directory); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return guard, nil
}

func (guard *DirectoryGuard) Path(name string) string {
	return filepath.Join(guard.stableDir, filepath.Base(name))
}

func (guard *DirectoryGuard) VerifyPath(directory string) error {
	stable, err := os.Stat(guard.stableDir)
	if err != nil || !stable.IsDir() {
		return errors.New("guarded destination directory handle is invalid")
	}
	current, err := os.Stat(directory)
	if err != nil || !current.IsDir() || !os.SameFile(stable, current) {
		return errors.New("guarded destination directory path changed")
	}
	return nil
}

func (guard *DirectoryGuard) Sync() error {
	return syscall.Fsync(guard.fd)
}

func (guard *DirectoryGuard) Close() error {
	if guard == nil || guard.fd < 0 {
		return nil
	}
	err := syscall.Close(guard.fd)
	guard.fd = -1
	return err
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
