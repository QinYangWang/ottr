# UI

Native windows drawn by MyGo's `ui` package: no HTML, no JavaScript, no
webview and no frontend build step. Each view is a Go function of the app's
state (`ui/native.App.view` and `App.settingsView`), rebuilt by MyGo after
input and whenever the app asks for a frame.

## Design constraints

- Light and dark follow the desktop; the accent color is the system's.
- The theme's spacing unit sizes everything, so controls stay in step.
- Short English labels, and a caption above each group of controls, as a
  settings window has them.
- Capture sources are cards with an icon and name; speakers and microphones
  are dropdowns, including `Off`. Binary preferences use switches.
- Icons are Lucide's ([lucide.dev](https://lucide.dev), ISC), embedded as SVG
  files under [ui/native/icons](../ui/native/icons) and drawn through `ui.Icon`
  in the color of the text around them: no icon font, no PNG. A button whose
  meaning is clear shows the icon alone and carries its name in its label and
  tooltip — the import button of the library header.
- No controls without behavior. Anything unfinished explains why rather than
  acting as a dead button: the disabled `Area` and `Camera only` cards carry
  the reason in their tooltips.
- The window opens as the recording form alone, and grows when something is
  opened beside it.
- No timeline/editor UI.

## The window

MyGo creates the window with a hidden title bar (`TitleBarHidden`, 46 DIPs
high) and keeps its controls — minimize, maximize, close — in that strip, at
the top-right. The view draws the bar itself, so the app starts at the very top
of the window.

The main window opens at `CompactWidth` × `CompactHeight` (420 × 800) with
the recording form. Library (1180 × 800) and Settings (920 × 660) are separate
native windows and do not resize or cover the main one.

```
Main: Ottr                         Library: Ottr
Choose a capture source            ────────────────┬─────────────────────────
┌──────────┐┌──────────┐          Library [Import] │ Recorded screen 2026-10-24
│Full screen││ Display 1│         [Search]         │ Transcript Translate Summary
└──────────┘└──────────┘          Recorded screen │ 0:00  Alright, let's start…
┌──────────┐┌──────────┐          Bug triage      │ 0:04  We ship the editor…
│ Display 2││  Window  │                           │
└──────────┘└──────────┘                           │
[🔈 System audio      Speakers ▾]
[🎤 Microphone               Off ▾]
[        Start recording        ]
                         [Theme] [Library] [Settings]  (icons)
```

The main form's upper part — the cards and picture — scrolls in a short
window; audio, `Start recording`, Library and Settings stay in reach. Starting
a recording opens or focuses Library.

## Title bar

`ui/native.titleBar` draws the strip the window controls sit in, so nothing sits
above the recording form:

- `Ottr`, bold, in every window — it drags the window, as a title bar does
  (`.DragWindow()`), including a double click that maximizes.
- A red dot and the elapsed time while a recording runs, so the window says what
  it is doing on every page.
- Everything else is drag room: the strip up to the window controls moves the
  window.

The bar reads `ui.TitleBar()` each frame: the height the controls need, and the
room they take at the left (macOS traffic lights) and right (Windows and Linux
buttons), so that the name and the button keep clear of them. An error banner
hangs below the bar rather than over it.

## Main window and Library

- The sources are cards, two to a row and all the same height: `Full screen`
  and one `Display N` per monitor choose themselves. The chosen card carries
  the accent border and its icon in the accent color.
- Under the cards, a `Window` dropdown lists the open windows so one can be
  recorded without leaving the form. The list is read again each time it opens,
  since windows come and go. The dropdown shows the window chosen, or the
  first one when none is yet.
- `Area` and `Camera only` are disabled: recording part of the screen and
  recording a camera are not implemented (see ROADMAP), and each card's
  tooltip says so.
- Under them, a 192-DIP-high, 960-pixel-wide preview shows the source before
  recording, refreshing five times a second. Before the first frame it says
  which source it is and its size, or why there is no picture.
- `Start recording` uses the source chosen above it. System audio and the
  microphone are dropdowns of active WASAPI endpoints, with the default
  speaker selected and the microphone initially `Off`; `Off` leaves that
  sound out. The button is disabled while a start or stop is in flight, and
  a spinner says what is happening.
- Starting a recording opens Library, whose right side shows the elapsed time,
  live picture and transcript, with `Delete…` and `Stop and transcribe` above.
- In Library, `Search` filters through `Library.List(search)` as it is typed, and Escape
  clears it. An empty library says so, and so does a search with no hits.
- A row shows the name, when it was made, how long it runs, and what has been
  made of it (`recorded`, `transcribed`, `summary`), with a spinner while a
  transcript is pending.
- A right-click on a row opens the tabs, "Show the file" and `Delete…`.
- `Delete…` asks in a modal, with "Keep the file on disk" checked by default:
  the row goes, the recording stays.
- The import button, an icon alone at the right of the `Library` header, adds
  media files through a native dialog. Each one is transcribed on
  this computer, as a recording is; the `Translate + summarize imports` switch
  (off by default, because it spends the provider's quota) also translates them
  into the Translate tab's language and summarizes them, as one `import` job
  the page shows and can cancel.
- Below the list, a notice says how many files in the recordings folder no
  record holds — left there by a removal that kept the file — with `Add back`
  (re-add them as recordings) and `Delete files…`, which asks in a modal that
  names the count and cannot be undone.
- At the bottom-right of the main window, borderless icon buttons toggle the
  global light/dark theme and open or focus Library and Settings. Their names
  are exposed as labels and tooltips rather than visible text.

## Settings window

`Settings` at the bottom of the main window opens a separate 920 × 660 native
window, or focuses it if it is already open. Its left sidebar has one entry
per thing that can be configured; the chosen section fills the scrollable
area on the right:

| Section | Holds |
|---|---|
| Appearance | three cards — `System`, `Light`, `Dark`, each drawn as a miniature window in that look — that pick the appearance at once and remember it (`settings.ui_theme`) |
| Transcription | the models found, with size or the reason a folder is unusable, `In use` beside the one transcription uses and `Use` on the others; `Open folder`, `Re-check`, and where the sherpa-onnx binary was found; then `FFmpeg and ffprobe` with the path they were found at, or why not, and `Choose…` to point at `ffmpeg.exe` when Ottr cannot find it itself |
| Translation | the provider as a segmented control — the free MyMemory, or a custom OpenAI-compatible endpoint with base URL, key and model |
| Summary | base URL, key and model of the model that writes summaries |

Each row is drawn as a settings window draws them: the name and what it does
on the left, the control on the right. `Save` stores a section through
`translate.SaveSettings` / `summary.SaveSettings`; the model choice is stored
by `models.Manager.Select` as soon as it is clicked. The native window's close
button closes Settings alone; the main window keeps its size and page. See
[models.md](models.md).

## Recording page

Shown on the right of Library while recording.

- The latest preview frame from `Recorder.Preview()`, decoded into a bitmap
  that is updated in place as frames come.
- The live transcript: the last six lines, the newest in full color and the
  older ones muted, each with the time it was recognized at.
- It says the text is provisional, because it is: the finished file is
  transcribed again, which is the accurate pass.
- `Stop and transcribe` finalizes the file, adds it to the library, selects
  it, and starts the offline transcription.
- `Delete…` asks, then stops the recorder and deletes the file it made.

## Session page

The header shows the name, the source, the duration, the creation time and the
resolution, with `Play` (the system's player), `Show file`, `Delete…` and
`Close`, which closes the details and leaves the media list open.

### Transcript

- Not transcribed: an explanation and `Transcribe`, or a note that the file
  has no audio when it has none.
- `pending` or a running job: a spinner, a progress bar, and `Cancel`.
- Under the toolbar, `Spoken language` (Detected, English, Chinese, Japanese)
  overrules what the transcript says it is in, which is what translation reads.
- `done`: a searchable list of `timestamp  text` rows, with SRT, VTT and TXT
  exports (each opening a native save dialog) and `Transcribe again`.
- The text is selectable, so a line can be copied.

### Translate

- Transcript required, otherwise a link back to Transcript.
- Target language: English, Chinese (中文), Japanese (日本語).
- Empty state offers `Translate`; while running it says what is happening,
  with a progress bar and `Cancel`.
- Each row shows the translation above the original text.
- Exports: SRT, bilingual SRT, VTT, TXT.
- The provider is configured in Settings ▸ Translation, not here.

### Summary

- Transcript required.
- Empty state offers `Generate`; while running it shows a progress bar and
  `Cancel`.
- Result renders Overview, Key points, Action items (check-box glyph),
  Chapters and Highlights.
- `Copy` puts the Markdown on the clipboard; `Export…` writes a `.md` file.
- The model is configured in Settings ▸ Summary.

## Errors

Every failure lands in `App.err` and shows as a banner pinned to the top of the
window, with the Go error verbatim. Errors are never swallowed and never
replaced with placeholder content. Dismissing the banner clears it.

## Media playback

The window shows a preview of a recording in progress, but not the media of a
finished file: `Play` hands the file to the system's player. `ui` has no video
element yet, so a player inside the window is roadmapped; the timestamps in the
transcript and summary are not clickable for that reason.
