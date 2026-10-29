# UI

Native windows drawn by MyGo's `ui` package: no HTML, no JavaScript, no
webview and no frontend build step. Each view is a Go function of the app's
state (`ui/native.App.view` and `App.settingsView`), rebuilt by MyGo after
input and whenever the app asks for a frame.

## Proposed three-window redesign

[Open the interactive HTML prototype](prototypes/native-ui.html). It is a
standalone, dependency-free interaction prototype, not a proposal to put HTML
in the native application.

1. **Compact recorder:** the current small window keeps only source, audio and
   Start. Full screen and Window are the only source choices. Full screen has
   a display picker; both display and window selection update the same visible
   preview. Start remains in a fixed footer when the compact content scrolls.
2. **Recording strip:** Start replaces the compact window with a small draggable
   strip: Stop/save + timer, microphone, Pause, Restart and Discard. Restart and
   Discard require confirmation. Stop saves and opens Library.
3. **Library:** a grid of media cards with a toolbar search, not a permanent
   sidebar list. Opening a card switches Library to that item's page: the
   in-window player on the left, Transcript/Summary on the right. `Back`
   returns to the grid. Transcript and its translation share one timestamped
   list: original text above translated text, with language controls in that
   pane's toolbar. `New recording` opens the recorder in place. There is no
   separate large recording-details page.
4. **Settings:** remains a separate window with the current section/sidebar
   structure. The HTML prototype includes working theme, model and provider
   interactions.

Every visible control in the prototype changes state or performs its described
action; no disabled Area/Camera cards or duplicate theme/Library controls.

## Design constraints

- Light and dark follow the desktop; the accent color is the system's.
- The theme's spacing unit sizes everything, so controls stay in step.
- Three surface levels carry the hierarchy: `bg` for a window, `surface` for
  raised rows, cards and the title bar, `surface-sunken` for navigation,
  search fields and hover fills. Hairline borders and a two-step shadow do the
  rest — no heavy outlines, no second accent. The Go theme is
  [ui/native/theme.go](../ui/native/theme.go), which keeps `ui.LightTheme` /
  `ui.DarkTheme`'s structure and substitutes this palette.
- Type is small and tight: 13–13.5px body, 640-weight headings with negative
  tracking, and 10.5px uppercase labels above groups and inside the item page's
  fact strip.
- Thumbnails, the player stage and the recording preview share one dark
  treatment so media always sits on the same ground in both themes; text drawn
  over them is light in both looks.
- The grid is one wrapping row of fixed-width cards, so the column count
  follows the window instead of a fixed number: four at 1180 wide, two at 900,
  one below 620.
- Cards and the item page have no heading of their own. The window is named
  `Ottr` and the media's name is its title, so no `Library` heading or item
  count is drawn and no date/source line repeats what the fact strip says.
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
  opened beside it. Library and its item page replace one another inside the
  same window; no second window opens for details.
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
Main: Ottr
Choose a capture source
┌──────────┐┌──────────┐
│Full screen││ Display 1│
└──────────┘└──────────┘
┌──────────┐┌──────────┐
│ Display 2││  Window  │
└──────────┘└──────────┘
[🔈 System audio      Speakers ▾]
[🎤 Microphone               Off ▾]
[        Start recording        ]
                         [Theme] [Library] [Settings]  (icons)
```

```
Library: Ottr              [Search recordings] [New recording] [Import]
────────────────────────────────────────────────────────────────────────
┌────────────┐  ┌────────────┐  ┌────────────┐
│  ▷ preview │  │  ▷ preview │  │  ▣ shot    │   one card per media:
└────────────┘  └────────────┘  └────────────┘   thumbnail, status chips,
TXT AI    4:31  TXT       9:08  PNG   Oct 27     duration, name, source
Recorded screen  Bug triage      Design ref
```

```
Library ▸ item (the arrow beside the name returns to the grid)
‹ Recorded screen 2026-10-24                 [📁] [🗑 Delete]
┌──────────────────────────────┬────────────────────────────────┐
│                              │ Transcript | Summary     [⤓]   │
│          ▷  player           │ 0:00  Alright, let's start…    │
│                              │ 0:04  We ship the editor…      │
└──────────────────────────────┴────────────────────────────────┘
┌──────────┬──────────┬──────────────────┬─────────────────────┐
│ SOURCE   │ DURATION │ CREATED          │ STATUS              │
│ Recorded │ 3:43     │ Oct 29, 14:32    │ Transcribed · …     │
└──────────┴──────────┴──────────────────┴─────────────────────┘
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
- Under them, a preview panel shows the source before recording, refreshing
  five times a second. Before the first frame it says
  which source it is and its size, or why there is no picture.
- `Start recording` uses the source chosen above it. System audio and the
  microphone are dropdowns of active WASAPI endpoints, with the default
  speaker selected and the microphone initially `Off`; `Off` leaves that
  sound out. The button is disabled while a start or stop is in flight, and
  a spinner says what is happening.
- Starting a recording opens Library, whose item page shows the elapsed time,
  live picture and transcript, with `Delete…` and `Stop and transcribe` above.
- Library is a page of cards, one per media, four to a row at 1180 wide and
  fewer as the window narrows: a 16:9 thumbnail carrying the status chips and
  the duration in its lower corners, the name on two lines at most, and the
  source with the date. A screenshot shows its picture instead of a preview.
- The `Library` header holds `Search` — which filters through
  `Library.List(search)` as it is typed, and Escape clears it — plus
  `New recording` and the import icon button. The grid has no title of its own:
  the window name already says `Ottr`, so the cards start at the top and no
  item count is drawn. An empty library says so, and so does a search with no
  hits.
- A card's chips say what has been made of it: `TXT` for a transcript, `AI` for
  a summary, `PNG` for a screenshot. A card whose transcript is pending says
  `Working…` in place of its duration.
- Clicking a card opens its page in place: the back arrow beside the name
  returns to the grid, and the card that was open keeps the accent ring. `New
  recording` in the header opens the recorder page in place, whose `Library`
  button returns to the grid.
- The `Library` icon in the main window's status bar always opens the grid, not
  the last open item.
- A right-click on a card opens a menu of the tabs, "Show the file" and
  `Delete…`.
- `Delete…` asks in a modal, with "Keep the file on disk" checked by default:
  the card goes, the recording stays.
- The item page keeps the visual language of the grid: the name with an icon
  button for `Show file` and a `Delete` in the danger tint, a dark player stage
  with the picture centred and the file's time in a strip below, and a
  four-cell fact strip (source, duration, created, status) divided by
  hairlines. A screenshot shows the still instead of a play button.
- To add imports to the automatic chain, the `Translate + summarize imports`
  switch sits in Settings ▸ Translation: the grid has no room for it now, and
  it is a preference rather than a per-import choice.
- Below the grid, a notice says how many files in the recordings folder no
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
| Transcription | the models found, with size or the reason a folder is unusable, `In use` beside the one transcription uses and `Use` on the others; `Open folder`, `Re-check`, and which sherpa-onnx version the native runtime reports (or why there is none); then `FFmpeg and ffprobe` with the path they were found at, or why not, and `Choose…` to point at `ffmpeg.exe` when Ottr cannot find it itself |
| Translation | the provider as a segmented control — the free MyMemory, or a custom OpenAI-compatible endpoint with base URL, key and model |
| Summary | base URL, key and model of the model that writes summaries |

Each row is drawn as a settings window draws them: the name and what it does
on the left, the control on the right. `Save` stores a section through
`translate.SaveSettings` / `summary.SaveSettings`; the model choice is stored
by `models.Manager.Select` as soon as it is clicked. The native window's close
button closes Settings alone; the main window keeps its size and page. See
[models.md](models.md).

The visual language is one system across all three windows: a sunken
`surface-sunken` for navigation and inputs, raised rows and cards on
`surface` with a hairline border and a soft shadow, one accent used only for
selection, primary actions and focus rings, small uppercase section labels,
and 12–13.5px body text with tighter heading leading.

## Recording page

Shown in place of the card grid while recording.

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

Opened by clicking a card; `Back` returns to the grid. The stage on the left
carries the name, a `Show file` icon button (folder glyph, name in its label and
tooltip), a `Delete` button in the danger tint, the player, and a four-cell fact
strip below it; the pane on the right carries the Transcript and Summary tabs
and `Export…`. The name is the only text in the header — the date, source and
duration live in the fact strip instead of repeating under the title.

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
transcript and summary are not clickable for that reason. The proposed
implementation is [In-window native playback](native-playback.md).
