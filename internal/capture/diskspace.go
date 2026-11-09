package capture

import (
	"os"
	"path/filepath"
)

// Space reports how many bytes the volume recordings are written to has left
// for this user, and how large that volume is. The folder need not exist yet:
// the nearest existing parent is asked instead.
func (r *Recorder) Space() (uint64, uint64, error) {
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
	return volumeSpace(dir)
}
