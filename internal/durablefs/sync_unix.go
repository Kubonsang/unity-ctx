//go:build !windows

package durablefs

import "os"

// SyncDirectory persists directory-entry changes after an atomic publish.
func SyncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
