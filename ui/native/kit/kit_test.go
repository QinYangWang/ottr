package kit

import (
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
