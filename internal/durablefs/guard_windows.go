//go:build windows

package durablefs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// DirectoryGuard holds every directory from the trusted root to the target
// without FILE_SHARE_DELETE. Windows then refuses rename/delete/junction swaps
// inside that trust boundary until the approval transaction closes the handles.
type DirectoryGuard struct {
	directory string
	handles   []syscall.Handle
	finalInfo syscall.ByHandleFileInformation
}

func GuardDirectoryTree(root, directory string) (*DirectoryGuard, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if !pathWithinWindows(root, directory) {
		return nil, errors.New("guarded directory escapes its trusted root")
	}
	if filepath.VolumeName(directory) == "" || !strings.EqualFold(filepath.VolumeName(root), filepath.VolumeName(directory)) {
		return nil, errors.New("guarded Windows directory requires an absolute volume path")
	}
	current := filepath.Clean(root)
	relative, err := filepath.Rel(current, directory)
	if err != nil {
		return nil, err
	}
	components := []string{current}
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" {
			continue
		}
		current = filepath.Join(current, component)
		components = append(components, current)
	}
	guard := &DirectoryGuard{directory: directory}
	for _, component := range components {
		handle, info, openErr := openGuardedDirectory(component)
		if openErr != nil {
			_ = guard.Close()
			return nil, openErr
		}
		guard.handles = append(guard.handles, handle)
		guard.finalInfo = info
	}
	if err := guard.VerifyPath(directory); err != nil {
		_ = guard.Close()
		return nil, err
	}
	return guard, nil
}

func openGuardedDirectory(path string) (syscall.Handle, syscall.ByHandleFileInformation, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, syscall.ByHandleFileInformation{}, err
	}
	// Request only directory-listing access, rather than GENERIC_READ. This is
	// the narrowest access used here that also makes the no-FILE_SHARE_DELETE
	// lease block directory rename and deletion on supported Windows versions.
	handle, err := syscall.CreateFile(
		name,
		syscall.FILE_LIST_DIRECTORY,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return 0, syscall.ByHandleFileInformation{}, fmt.Errorf("lock destination ancestor %s: %w", path, err)
	}
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &info); err != nil {
		_ = syscall.CloseHandle(handle)
		return 0, syscall.ByHandleFileInformation{}, err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 || info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 {
		_ = syscall.CloseHandle(handle)
		return 0, syscall.ByHandleFileInformation{}, errors.New("guarded destination ancestor is a reparse point or non-directory")
	}
	return handle, info, nil
}

func (guard *DirectoryGuard) Path(name string) string {
	return filepath.Join(guard.directory, filepath.Base(name))
}

func (guard *DirectoryGuard) VerifyPath(directory string) error {
	if guard == nil || len(guard.handles) == 0 || !strings.EqualFold(filepath.Clean(directory), filepath.Clean(guard.directory)) {
		return errors.New("guarded destination directory handle is invalid")
	}
	name, err := syscall.UTF16PtrFromString(directory)
	if err != nil {
		return err
	}
	probe, err := syscall.CreateFile(name, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return fmt.Errorf("reopen guarded destination directory: %w", err)
	}
	defer syscall.CloseHandle(probe)
	var current syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(probe, &current); err != nil {
		return err
	}
	if current.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 || current.VolumeSerialNumber != guard.finalInfo.VolumeSerialNumber || current.FileIndexHigh != guard.finalInfo.FileIndexHigh || current.FileIndexLow != guard.finalInfo.FileIndexLow {
		return errors.New("guarded destination directory path changed")
	}
	return nil
}

func (guard *DirectoryGuard) Sync() error {
	return SyncDirectory(guard.directory)
}

func (guard *DirectoryGuard) Close() error {
	if guard == nil {
		return nil
	}
	var firstErr error
	for index := len(guard.handles) - 1; index >= 0; index-- {
		if err := syscall.CloseHandle(guard.handles[index]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	guard.handles = nil
	return firstErr
}

func (guard *DirectoryGuard) ResolvePath(name string) (string, error) {
	if guard == nil || len(guard.handles) == 0 {
		return "", errors.New("guarded destination directory handle is invalid")
	}
	return filepath.Join(guard.directory, filepath.Base(name)), nil
}

func pathWithinWindows(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func pathHasReparsePoint(path string, _ os.FileInfo) (bool, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attributes, err := syscall.GetFileAttributes(name)
	if err != nil {
		return false, err
	}
	return attributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

func securePublicationSupported() bool { return true }
