package durablefs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// SecurePublicationSupported reports whether this build has a handle-based
// implementation that can keep path topology stable during publication.
func SecurePublicationSupported() bool {
	return securePublicationSupported()
}

// EnsureDirectoryTree creates one component at a time beneath a guarded
// existing parent. The existing parent becomes the trusted root and remains
// guarded while each missing descendant is created, so directory setup cannot
// be redirected outside that boundary.
func EnsureDirectoryTree(path string, mode os.FileMode, syncDirectory func(string) error) error {
	if !SecurePublicationSupported() {
		return errors.New("secure directory publication is unsupported on this operating system")
	}
	if syncDirectory == nil {
		return errors.New("secure directory publication requires directory sync")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := validateExistingDirectoryComponents(absolute); err != nil {
		return err
	}
	missing := make([]string, 0, 4)
	current := filepath.Clean(absolute)
	for {
		info, statErr := os.Lstat(current)
		if statErr == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("secure directory path contains a non-directory or reparse point")
			}
			break
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return errors.New("secure directory path has no existing filesystem root")
		}
		current = parent
	}
	trustedRoot := current
	guard, err := GuardDirectoryTree(trustedRoot, current)
	if err != nil {
		return err
	}
	defer func() {
		if guard != nil {
			_ = guard.Close()
		}
	}()
	for index := len(missing) - 1; index >= 0; index-- {
		next := missing[index]
		if err := os.Mkdir(guard.Path(filepath.Base(next)), mode); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		// Sync through the retained handle first. The logical callback is kept as
		// an injectable durability boundary for callers and tests, but cannot be
		// the only sync because a Linux pathname may be renamed concurrently.
		if err := guard.Sync(); err != nil {
			return err
		}
		if err := syncDirectory(current); err != nil {
			return err
		}
		if err := guard.Close(); err != nil {
			return err
		}
		current = next
		guard, err = GuardDirectoryTree(trustedRoot, current)
		if err != nil {
			return err
		}
	}
	return guard.VerifyPath(absolute)
}

func validateExistingDirectoryComponents(path string) error {
	root := filesystemRoot(path)
	current := root
	relative := strings.TrimPrefix(filepath.Clean(path), root)
	components := append([]string{root}, strings.Split(relative, string(filepath.Separator))...)
	for index, component := range components {
		if index > 0 {
			if component == "" {
				continue
			}
			current = filepath.Join(current, component)
		}
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		reparse, err := pathHasReparsePoint(current, info)
		if err != nil {
			return err
		}
		if !info.IsDir() || reparse {
			return errors.New("secure directory path contains a non-directory or reparse point")
		}
	}
	return nil
}

func filesystemRoot(path string) string {
	volume := filepath.VolumeName(path)
	if volume != "" {
		return volume + string(filepath.Separator)
	}
	return string(filepath.Separator)
}
