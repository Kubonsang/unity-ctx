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
	fds       []int
	paths     []string
	stableDir string
}

func GuardDirectoryTree(root, directory string) (*DirectoryGuard, error) {
	if !pathWithin(root, directory) {
		return nil, errors.New("guarded directory escapes its trusted root")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Open(string(filepath.Separator), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open guarded filesystem root: %w", err)
	}
	fds := []int{fd}
	paths := []string{string(filepath.Separator)}
	current := string(filepath.Separator)
	for _, component := range strings.Split(strings.TrimPrefix(absolute, string(filepath.Separator)), string(filepath.Separator)) {
		if component == "" {
			continue
		}
		next, openErr := syscall.Openat(fd, component, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		if openErr != nil {
			for index := len(fds) - 1; index >= 0; index-- {
				_ = syscall.Close(fds[index])
			}
			return nil, fmt.Errorf("open guarded destination component %s: %w", component, openErr)
		}
		fd = next
		fds = append(fds, fd)
		current = filepath.Join(current, component)
		paths = append(paths, current)
	}
	guard := &DirectoryGuard{fds: fds, paths: paths, stableDir: filepath.Join("/proc/self/fd", strconv.Itoa(fd))}
	if err := guard.VerifyPath(directory); err != nil {
		_ = guard.Close()
		return nil, err
	}
	return guard, nil
}

func (guard *DirectoryGuard) Path(name string) string {
	return filepath.Join(guard.stableDir, filepath.Base(name))
}

func (guard *DirectoryGuard) VerifyPath(directory string) error {
	if guard == nil || len(guard.fds) == 0 || len(guard.fds) != len(guard.paths) {
		return errors.New("guarded destination directory handle is invalid")
	}
	for index, fd := range guard.fds {
		stablePath := filepath.Join("/proc/self/fd", strconv.Itoa(fd))
		stable, err := os.Stat(stablePath)
		if err != nil || !stable.IsDir() {
			return errors.New("guarded destination ancestor handle is invalid")
		}
		logical, err := os.Stat(guard.paths[index])
		if err != nil || !logical.IsDir() || !os.SameFile(stable, logical) {
			return errors.New("guarded destination ancestor path changed")
		}
	}
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
	if guard == nil || len(guard.fds) == 0 {
		return errors.New("guarded destination directory handle is invalid")
	}
	return syscall.Fsync(guard.fds[len(guard.fds)-1])
}

func (guard *DirectoryGuard) Close() error {
	if guard == nil || len(guard.fds) == 0 {
		return nil
	}
	var firstErr error
	for index := len(guard.fds) - 1; index >= 0; index-- {
		if err := syscall.Close(guard.fds[index]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	guard.fds = nil
	guard.paths = nil
	return firstErr
}

func (guard *DirectoryGuard) ResolvePath(name string) (string, error) {
	if guard == nil || guard.stableDir == "" {
		return "", errors.New("guarded destination directory handle is invalid")
	}
	directory, err := os.Readlink(guard.stableDir)
	if err != nil || !filepath.IsAbs(directory) || strings.HasSuffix(directory, " (deleted)") {
		return "", errors.New("guarded destination directory no longer has a stable logical path")
	}
	return filepath.Join(directory, filepath.Base(name)), nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func pathHasReparsePoint(_ string, info os.FileInfo) (bool, error) {
	return info.Mode()&os.ModeSymlink != 0, nil
}

func securePublicationSupported() bool {
	fd, err := syscall.Open(string(filepath.Separator), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer syscall.Close(fd)
	stablePath := filepath.Join("/proc/self/fd", strconv.Itoa(fd))
	stable, stableErr := os.Stat(stablePath)
	root, rootErr := os.Stat(string(filepath.Separator))
	_, linkErr := os.Readlink(stablePath)
	return stableErr == nil && rootErr == nil && linkErr == nil && stable.IsDir() && os.SameFile(stable, root)
}
