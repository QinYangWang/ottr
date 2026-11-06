package native

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"ottr/internal/asr"
	"ottr/internal/translate"
)

// Settings offers the design's four pages, and the provider page writes the
// one endpoint into both services that talk to it.
func TestProviderSettingsSaveBothServices(t *testing.T) {
	a, _ := newTestApp(t)
	tt := ui.NewTester(a.settingsView, settingsWidth, 1100)
	tt.Frame()
	for _, section := range []string{"Appearance", "Transcription", "Provider", "Speakers"} {
		if !tt.HasText(section) {
			t.Fatalf("Settings has no %q page: %q", section, tt.Texts())
		}
	}

	a.settingsTab = 2
	for i := 0; i < 100 && !tt.HasText("Prompt templates"); i++ {
		time.Sleep(5 * time.Millisecond)
		tt.Frame()
	}
	if !tt.HasText("Prompt templates") {
		t.Fatalf("the provider page did not load: %q", tt.Texts())
	}
	if err := tt.Click("OpenRouter"); err != nil {
		t.Fatal(err)
	}
	if a.provider.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("the preset filled in %q", a.provider.BaseURL)
	}
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for a.Sum.GetSettings().BaseURL != "https://openrouter.ai/api/v1" {
		if time.Now().After(deadline) {
			t.Fatalf("summary settings = %+v", a.Sum.GetSettings())
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := a.Trans.GetSettings().BaseURL; got != "https://openrouter.ai/api/v1" {
		t.Fatalf("translation settings = %q", got)
	}
	if got := a.Trans.GetSettings().Prompt; got != translate.DefaultPrompt {
		t.Errorf("translation prompt = %q", got)
	}
}

// The speaker library lists what the app knows and renames a speaker in place.
func TestSpeakerLibraryPage(t *testing.T) {
	a, _ := newTestApp(t)
	a.speakerLib = []asr.Speaker{{
		ID: 1, Name: "Ana Ruiz", Recordings: 2, DurationMs: 41 * 60000,
		LastHeard: "2026-11-05T13:55:00Z",
	}}
	a.loadedSpeakers = true
	a.settingsTab = 3
	tt := ui.NewTester(a.settingsView, settingsWidth, 1100)
	tt.Frame()
	for _, text := range []string{"Speakers", "Ana Ruiz", "2 recordings · 41m · last heard Nov 5"} {
		if !tt.HasText(text) {
			t.Fatalf("the speaker page does not show %q: %q", text, tt.Texts())
		}
	}

	if err := tt.Click("Rename"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.speakerEditText = "Ana R."
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for a.speakerLib[0].Name != "Ana R." {
		tt.Frame()
		if time.Now().After(deadline) {
			t.Fatalf("the renamed speaker is %q", a.speakerLib[0].Name)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
