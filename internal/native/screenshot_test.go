package native

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"ottr/internal/capture"
)

func TestStartRecordingShowsSpinnerInButton(t *testing.T) {
	a, _ := newTestApp(t)
	a.sources = []capture.Source{{ID: "screen", Kind: "screen", Name: "Full screen"}}
	a.pickSource()
	a.busy = true
	tt := ui.NewTester(a.view, CompactWidth, CompactHeight)
	tt.Frame()
	if !tt.HasText("Starting…") || tt.HasText("Working…") {
		t.Fatalf("start loading is not inside the button: %q", tt.Texts())
	}
}

// The recording form follows the design: Start, then round Library and
// Settings buttons — not a Screenshot button.
func TestRecorderButtonsFollowTheDesign(t *testing.T) {
	a, _ := newTestApp(t)
	a.sources = []capture.Source{{ID: "screen", Kind: "screen", Name: "Full screen"}}
	a.pickSource()

	tt := ui.NewTester(a.view, CompactWidth, CompactHeight)
	tt.Frame()
	if tt.HasText("Screenshot") {
		t.Fatalf("the form still offers Screenshot: %q", tt.Texts())
	}
	for _, label := range []string{"Library", "Settings"} {
		if _, ok := tt.Find(label); !ok {
			t.Errorf("the start bar has no %s button: %q", label, tt.Texts())
		}
	}
	if !tt.HasText("Start recording") {
		t.Errorf("the start bar lost its primary button: %q", tt.Texts())
	}
}
