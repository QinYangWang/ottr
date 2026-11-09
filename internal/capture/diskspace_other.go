//go:build !windows

package capture

import (
	"fmt"
	"syscall"
)

func volumeSpace(dir string) (uint64, uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, 0, fmt.Errorf("space of %s: %w", dir, err)
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize), nil
}
