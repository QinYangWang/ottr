# UI

A single native window, drawn by MyGo's `ui` package: no HTML, no JavaScript,
no webview and no frontend build step. The view is a Go function of the app's
state (`ui/native.App.view`), rebuilt by MyGo after input and whenever the app
asks for a frame.

## Design constraints

- Light and dark follow the desktop; the accent color is the system's.
- The theme's spacing unit sizes everything, so controls stay in step.
- Short English labels.
- No controls without behavior. Anything unfinished renders an explicit reason
  rather than a dead button.
- No timeline/editor UI.

## The window

```
┌──────────────────────┬──────────────────────────────────────────────┐
│ Ottr        [Recording]  │ Recording 0:07          [Delete…][Stop]   │
│ [Screen · Full screen ▾] │ ┌──────────────────┐  Live transcript     │
│ ☑ System audio ☐ Mic     │ │  preview frame    │  Provisional: …      │
│ [Start recording]        │ └──────────────────┘  0:08 军             │
├──────────────────────┤                                              │
│ Library      [Import…]   │                                              │
│ [Search]                 │                                              │
│ Recording 15:04 …        │                                              │
│ Recording 14:02 …        │                                              │
└──────────────────────┴──────────────────────────────────────────────┘
```

The library is always on the left, so recording and the recordings it makes
share one place. The right side shows the recording in progress, or the media
chosen.

## Library (sidebar)

- `Start recording` uses the source picked above it, with system audio and the
  microphone as checkboxes. It is disabled while a start or stop is in flight,
  and a spinner says what is happening.
- While recording, the form gives way to the elapsed time; `Delete…` and
  `Stop and transcribe` are in the page header. Choosing another file is off
  until it stops, but the library list is still there.
- `Search` filters through `Library.List(search)` as it is typed, and Escape
  clears it. An empty library says so, and so does a search with no hits.
- A row shows the name, when it was made, how long it runs, and what has been
  made of it (`recorded`, `transcribed`, `summary`), with a spinner while a
  transcript is pending.
- A right-click on a row opens the tabs, "Show the file" and `Delete…`.
- `Delete…` asks in a modal, with "Keep the file on disk" checked by default:
  the row goes, the recording stays.

## Recording page

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
resolution, with `Play` (the system's player), `Show file` and `Delete…`.

### Transcript

- Not transcribed: an explanation and `Transcribe`, or a note that the file
  has no audio when it has none.
- `pending`: a spinner and what is running.
- `done`: a searchable list of `timestamp  text` rows, with SRT, VTT and TXT
  exports (each opening a native save dialog) and `Transcribe again`.
- The text is selectable, so a line can be copied.

### Translate

- Transcript required, otherwise a link back to Transcript.
- Target language: English, Chinese (中文), Japanese (日本語).
- Empty state offers `Translate`; while running it says what is happening.
- Each row shows the translation above the original text.
- Exports: SRT, bilingual SRT, VTT, TXT.
- `Settings` opens an inline panel: the free MyMemory provider, or a custom
  OpenAI-compatible endpoint with base URL, key and model.

### Summary

- Transcript required.
- Empty state offers `Generate`; `pending` shows a spinner.
- Result renders Overview, Key points, Action items (check-box glyph),
  Chapters and Highlights.
- `Copy` puts the Markdown on the clipboard; `Export…` writes a `.md` file.
- `Settings` edits base URL, key and model.

## Errors

Every failure lands in `App.err` and shows as a banner pinned to the top of the
window, with the Go error verbatim. Errors are never swallowed and never
replaced with placeholder content. Dismissing the banner clears it.

## Media playback

The window shows a preview of a recording in progress, but not the media of a
finished file: `Play` hands the file to the system's player. `ui` has no video
element yet, so a player inside the window is roadmapped; the timestamps in the
transcript and summary are not clickable for that reason.
