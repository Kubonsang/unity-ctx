//go:build windows

package durablefs

import (
	"fmt"
	"syscall"
)

// SyncDirectory opens a real directory handle rather than os.Open's search
// handle, then asks the volume to flush its metadata. FILE_FLAG_BACKUP_SEMANTICS
// is required for directory handles on Windows.
func SyncDirectory(path string) error {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(
		name,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return fmt.Errorf("open directory for durable sync: %w", err)
	}
	defer syscall.CloseHandle(handle)
	if err := syscall.FlushFileBuffers(handle); err != nil {
		return fmt.Errorf("flush directory metadata: %w", err)
	}
	return nil
}
