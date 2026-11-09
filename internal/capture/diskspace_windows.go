package capture

import (
	"fmt"
	"syscall"
	"unsafe"
)

var procGetDiskFreeSpaceExW = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")

func volumeSpace(dir string) (uint64, uint64, error) {
	path, err := syscall.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0, fmt.Errorf("space of %s: %w", dir, err)
	}
	var free, total uint64
	ok, _, callErr := procGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(path)),
		uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)), 0)
	if ok == 0 {
		return 0, 0, fmt.Errorf("space of %s: %w", dir, callErr)
	}
	return free, total, nil
}
