//go:build !windows

package reviewgrant

import "os"

func openLeaseFile(path string) (*os.File, error) {
	return os.Open(path)
}
