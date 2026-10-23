# UI

A single window, two pages, hash routing. The frontend is plain HTML/CSS/JS
embedded with `go:embed` and served through MyGo's `SetFrontend`; there is no
Bun/Vite build step. Pages call Go through the runtime injected by MyGo
(`window.mygo.call` / `window.mygo.on`).

## Design constraints

- Dark theme only, `color-scheme: dark`.
- Accent color inherits `--mygo-accent` from the OS.
- Short English labels.
- No controls without behavior. Anything unfinished renders an explicit reason
  rather than a dead button.
- No timeline/editor UI.

## Library (`#/`)

Top bar: title `Ottr`, search field, `Record`, `Import`.

Table columns: Name, Source, Created, Duration, Transcript status, Summary
status, Delete.

- Search filters by name through `Library.List(search)`, debounced by re-render
  on input; `Esc` clears it.
- Clicking a row navigates to `#/session/<id>`.
- `Delete` asks for confirmation in Go and only removes the file when the user
  checks "Also delete the file from disk".
- Empty state is genuine: "No videos yet" when the table is truly empty, "No
  matches" when a search has no hits. No demo rows are ever inserted.
- Status values render as color-coded badges (`done` green, `pending` amber,
  `error` red).

`library:changed` re-renders the list, so imports and status transitions appear
without manual refresh.

## Record (`#/record`)

- Source `<select>` grouped into Screen / Displays / Windows, populated by
  `Recorder.Sources()`.
- Two audio checkboxes: System audio (default on), Microphone.
- `Start` calls `Recorder.Start(cfg)`; disabled while the call is in flight.
- While recording the page shows: a pulsing red dot, an elapsed timer, the live
  MJPEG preview at `media://localhost/preview`, and `Stop`.
- `Stop` becomes "Finalizing…" while FFmpeg finalizes; on success the page
  navigates to the new session, on failure it returns to the Library and shows
  the error.
- `recording:state` events drive all transitions, including unexpected failure.

## Session (`#/session/<id>`)

Left column: `<video>` (or `<audio>` when the file has no video stream) sourced
from `media://localhost/<id>`, plus name, duration, source, creation time, and
resolution.

Right column: three tabs.

### Transcript

- If the media has no transcript: a `Transcribe` button (hidden entirely and
  replaced by an explanation when the file has no audio stream).
- `pending`: spinner.
- `done`: a searchable list of `[timestamp] text` rows.
  - Clicking a row seeks the player to that cue and starts playback.
  - The row under the playhead is highlighted and kept in view (`ontimeupdate`).
  - Export buttons: SRT, VTT, TXT — each opens a native save dialog.

### Translate

- Target language select: English, Chinese, Japanese.
- Same seeking/highlight behavior as Transcript, but each row shows the
  translation above the original text.
- Empty state offers `Translate`; while running it shows "Translating online…".
- Export: SRT, Bilingual SRT, VTT, TXT.
- `⚙ Config` opens an inline drawer to choose the provider (free MyMemory vs
  custom OpenAI-compatible endpoint) and store base URL / key / model.

### Summary

- Transcription required, otherwise a link back to Transcript.
- Empty state offers `Generate`; `pending` shows a spinner.
- Result renders Overview, Key Points, Action Items (checkbox glyph), Chapters,
  and Highlights. Chapters and highlights seek the player on click.
- `Copy` copies the Markdown to the clipboard; `Export` writes a `.md` file.
- `⚙ Config` edits base URL / key / model.

## Error reporting

Every failed `call` is displayed in a `.error-banner` pinned to the top of the
window with the Go error message verbatim. Errors are never swallowed and never
replaced with placeholder content.

## Media URLs

`mediaURL(path)` prefixes `http://media.localhost/` on Windows and
`media://localhost/` elsewhere, because WebView2 exposes custom schemes under an
`http` origin. Both media playback and the live preview use the same helper.
