package kit

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestInlineSelectChoosesAnOption(t *testing.T) {
	chosen := "short"
	options := []SelectOption[string]{{Value: "short", Label: "Short"}, {Value: "long", Label: "A much longer option name"}}
	tt := ui.NewTester(func(c *ui.Context) {
		theme := ui.LightTheme()
		ui.Row(c).Children(func() {
			Select(c, theme, &chosen, options, "Choice", "Choose an option", nil, func(string) {})
		})
	}, 320, 480)
	tt.Frame()
	if err := tt.Click("Choose an option"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	header, _ := tt.Find("Choose an option")
	option, ok := tt.Find("A much longer option name")
	if !ok || option.Y < header.Y+header.H {
		t.Fatalf("options are not inline below the header: header=%+v option=%+v", header, option)
	}
	if err := tt.Click("A much longer option name"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if chosen != "long" {
		t.Fatalf("selected %q", chosen)
	}
}

func TestInlineSelectKeyboardAndOutsideDismiss(t *testing.T) {
	chosen := "short"
	options := []SelectOption[string]{{Value: "short", Label: "Short"}, {Value: "long", Label: "Long"}}
	tt := ui.NewTester(func(c *ui.Context) {
		Select(c, ui.LightTheme(), &chosen, options, "Choice", "Choose an option", nil, func(string) {})
	}, 320, 480)
	tt.Frame()
	if err := tt.Click("Choose an option"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyDown)
	tt.Frame()
	if chosen != "long" || !tt.HasText("Short") {
		t.Fatalf("keyboard did not choose while open: %q %q", chosen, tt.Texts())
	}
	tt.ClickAt(310, 450)
	tt.Frame()
	if tt.HasText("Short") {
		t.Fatal("clicking outside did not collapse the select")
	}
}

func TestSelectBoxChoosesAnOption(t *testing.T) {
	chosen := "A"
	options := []SelectOption[string]{{Value: "A", Label: "Alpha"}, {Value: "B", Label: "Beta"}}
	tt := ui.NewTester(func(c *ui.Context) {
		SelectBox(c, ui.LightTheme(), &chosen, options, "Pick a value", 200, func(string) {})
	}, 320, 240)
	tt.Frame()
	if !tt.HasText("Alpha") {
		t.Fatalf("the trigger does not show the value: %q", tt.Texts())
	}
	if err := tt.Click("Pick a value"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Beta"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if chosen != "B" {
		t.Fatalf("selected %q", chosen)
	}
}

// The palette is the only place colour is chosen, so its readable pairs are
// checked here: a new value that makes a chip or a button unreadable fails
// before it ships.
func TestPaletteContrast(t *testing.T) {
	type pair struct {
		name   string
		fg, bg ui.Color
		min    float64
	}
	white := ui.RGB(255, 255, 255)
	for _, look := range []struct {
		name string
		p    Palette
	}{{"light", light}, {"dark", dark}} {
		p := look.p
		pairs := []pair{
			{"ink on sheet", p.Ink, p.Sheet, 7},
			{"ink on paper", p.Ink, p.Paper, 7},
			{"ink2 on sheet", p.Ink2, p.Sheet, 4.5},
			{"accent ink on accent soft", p.AccentInk, p.AccentSoft, 4.5},
			{"accent on sheet", p.Accent, p.Sheet, 3},
			{"green ink on green soft", p.Green, p.GreenSoft, 4.5},
			{"red ink on red soft", p.RedInk, p.RedSoft, 4.5},
			{"red on sheet", p.Red, p.Sheet, 3},
			{"button text on button", p.BtnInk, p.Btn, 4.5},
		}
		for _, s := range SpeakerColors {
			pairs = append(pairs, pair{fmt.Sprintf("initials on #%02X%02X%02X", s.R, s.G, s.B), white, s, 4.5})
		}
		for _, pr := range pairs {
			if got := contrastRatio(pr.fg, pr.bg); got < pr.min {
				t.Errorf("%s: %s is %.2f:1, want at least %.1f:1", look.name, pr.name, got, pr.min)
			}
		}
	}
}

// contrastRatio is WCAG 2.1 relative luminance contrast.
func contrastRatio(a, b ui.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(c ui.Color) float64 {
	channel := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}
