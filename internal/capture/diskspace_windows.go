package capture

import (
	"fmt"
	"syscall"
	"unsafe"
)

var procGetDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func freeBytes(dir string) (uint64, error) {
	path, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, fmt.Errorf("free space of %s: %w", dir, err)
	}
	var available uint64
	ok, _, callErr := procGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&available)), 0, 0)
	if ok == 0 {
		return 0, fmt.Errorf("free space of %s: %w", dir, callErr)
	}
	return available, nil
}
