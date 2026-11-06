package capture

import (
	"os"
	"path/filepath"
)

// FreeSpace reports how many bytes the volume recordings are written to has
// left for this user. The folder need not exist yet: the nearest existing
// parent is asked instead.
func (r *Recorder) FreeSpace() (uint64, error) {
	dir := r.Dir
	for dir != "" {
		if _, err := os.Stat(dir); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return freeBytes(dir)
}
