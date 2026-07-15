//go:build !linux && !windows

package durablefs

import "errors"

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
