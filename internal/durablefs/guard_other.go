//go:build !linux && !windows

package durablefs

import (
	"errors"
	"os"
)

type DirectoryGuard struct{}

func GuardDirectoryTree(_, _ string) (*DirectoryGuard, error) {
	return nil, errors.New("secure handle-relative spatial publication is unsupported on this operating system")
}

func (guard *DirectoryGuard) Path(name string) string { return name }
func (guard *DirectoryGuard) VerifyPath(string) error {
	return errors.New("secure handle-relative spatial publication is unsupported on this operating system")
}
func (guard *DirectoryGuard) Sync() error {
	return errors.New("secure handle-relative spatial publication is unsupported on this operating system")
}
func (guard *DirectoryGuard) Close() error { return nil }
func (guard *DirectoryGuard) ResolvePath(string) (string, error) {
	return "", errors.New("secure handle-relative spatial publication is unsupported on this operating system")
}

func pathHasReparsePoint(_ string, info os.FileInfo) (bool, error) {
	return info.Mode()&os.ModeSymlink != 0, nil
}

func securePublicationSupported() bool { return false }
