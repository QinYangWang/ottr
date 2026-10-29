package native

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/capture"
)

// The recording form offers a Screenshot button beside Start, and it reports
// the backend's failure instead of pretending a file was made.
func TestScreenshotButton(t *testing.T) {
	a, _ := newTestApp(t)
	a.sources = []capture.Source{{ID: "screen", Kind: "screen", Name: "Full screen"}}
	a.Shot = &capture.Snapshotter{Dir: t.TempDir(), Lib: a.Lib}
	a.pickSource()

	tt := ui.NewTester(a.view, CompactWidth, CompactHeight)
	tt.Frame()
	if !tt.HasText("Screenshot") {
		t.Fatalf("the form has no Screenshot button: %q", tt.Texts())
	}
	if err := tt.Click("Screenshot"); err != nil {
		t.Fatal(err)
	}
	// A backend-less snapshotter reports why instead of pretending.
	deadline := time.Now().Add(3 * time.Second)
	for a.busy && time.Now().Before(deadline) {
		tt.Frame()
		time.Sleep(10 * time.Millisecond)
	}
	tt.Frame()
	if a.err == "" {
		t.Error("a screenshot with no backend said nothing")
	}
}
