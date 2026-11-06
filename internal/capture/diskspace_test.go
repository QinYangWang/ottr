package capture

import (
	"path/filepath"
	"testing"
)

// A recordings folder that does not exist yet still has a volume under it.
func TestFreeSpace(t *testing.T) {
	r := &Recorder{Dir: filepath.Join(t.TempDir(), "not", "yet")}
	free, err := r.FreeSpace()
	if err != nil {
		t.Fatal(err)
	}
	if free == 0 {
		t.Error("the temp volume reports no free space")
	}
}
