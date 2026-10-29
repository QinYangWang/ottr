# Architecture

## Layer boundaries

```
cmd/studio/main.go          composition root: opens the database, builds the view, opens the window
        │
        ├── ui/native/       the window, drawn by MyGo's toolkit: pages are Go functions of state
        │
        └── internal/
              library/       media records, import, remove
              capture/       capture.Backend + platform backends (cgo), recording session, preview, screenshots
              media/         ffprobe probing
              asr/           sherpa-onnx through cgo (native/), PCM conversion, segmentation, export
              translate/     online translation clients, bilingual alignment, export
              summary/       remote LLM call, structured summary, Markdown
              models/        model + binary discovery and validation
              jobs/          background work: state, progress, error, cancellation
              storage/       SQLite open + migration runner
```

Capture is native: [Native capture and media redesign](native-capture.md)
describes it and what is still to come. Its phase 1 (common frame pipeline,
in-process preview, screenshots) is implemented for Windows; encoding and
muxing are still FFmpeg until phase 2. The sections below describe the code as
it exists today.

Rules the code follows:

- There is no web frontend and no IPC: `ui/native` calls the services directly,
  and the services call back into the window with `Window.Update`.
- The UI layer never touches FFmpeg, WASAPI, or SQL.
- `capture` does not import `asr`; live recognition arrives through the
  one-value `LiveStarter` type and the `LiveWriter` interface it returns.
- Platform-specific code is isolated by build tags: `*_windows.go` and
  `*_other.go` pairs in `capture`, `media`, and `asr`. The one cgo boundary is
  `capture/backend_windows.go` + `capture/wgc_windows.c`, a small C ABI over
  Windows Graphics Capture; Go passes a callback and a `runtime/cgo.Handle`, and
  no Go pointer is retained by native code.
- `capture.Backend` is constructed once in `cmd/studio/main.go` and shared by
  `Recorder`, `Previewer` and `Snapshotter`.

## The window

`ui/native.App` holds the state the window shows and the `view` function that
draws it. MyGo calls the view on the main thread after input and whenever the
app asks for a frame; the view is a pure function of the state, so there is no
diffing to get wrong and no JavaScript.

State changes from other goroutines go through `App.update`, which runs the
change on the main thread and redraws:

```go
go func() {
	imported, err := a.Lib.Import(ctx)
	a.update(func() { /* change state */ })
}()
```

Long jobs are started from the view and report their result the same way. Each
one is a row in `job` (see [Storage](storage.md#job)) with a state, a progress
and an error, so the view can show how far it has come and stop it; the view
reads the newest job of the media shown each frame while something runs
(`App.pending`), so a transcript or summary that finishes shows up without an
event stream. `jobs.Changed` (`job:progress`) describes the same work for the
roadmapped web views.

## Layout of the windows

The main window configures recording; Library and Settings are separate native
windows with the same hidden, app-drawn title bar:

| Part | Shows |
|---|---|
| Main window | source cards, a window dropdown, the preview, full-row speaker/microphone dropdowns, Start recording, and theme/Library/Settings icon buttons |
| Library title strip | `Ottr` and, on the grid, `Search`, `New recording` and the import icon button; an inner page carries a back arrow beside its own title instead |
| Library grid (`libraryGrid`) | one card per media: a 16:9 dark thumbnail with status chips and duration in its lower corners, then the name and the source with the date, with a right-click menu of the tabs, Show the file and Delete |
| Library new recording (`recordPage`) | the recording form in place of the grid: a back arrow and `New recording`, the source preview beside the cards, window and audio dropdowns, and Start |
| Library recording page (`recorderPage`) | the preview frame in the same dark ground, the live transcript in a panel beside it, and Delete/Stop |
| Library session page (`sessionPage`) | the name with `Show file` and `Delete`, the dark player stage, a four-cell fact strip, and the Transcript/Translate/Summary tabs in a right-hand pane, opened by a card and left by the arrow |
| Settings window | which model transcribes, and which provider translates and summarizes |

The main window is always 420 × 800. Library is 1180 × 800 and Settings is
920 × 660. Starting a recording opens or focuses Library, which shows the live
capture in place of the grid and then the finished recording's page.

`ui/native/theme.go` holds the palette all three windows share: it starts from
`ui.LightTheme` / `ui.DarkTheme` and substitutes the prototype's surfaces,
accent, radii and tight type, so widgets the app does not draw itself — buttons,
fields, tabs, scroll bars — land on the same look.

## Events (Go → app)

`mygo.Event` broadcasts to pages, and a window of native UI has none. The
window therefore reads what it shows rather than waiting to be told: it mirrors
`Recorder.State()` each frame, and redraws while a job it shows is running.
`recording:state` and `transcript:live` still describe the recorder for the
roadmapped web views.

`asr.LiveEvent` is now emitted: each chunk of recorded audio that is
recognized broadcasts its subtitle, and the recorder keeps the newest one in
`State.LiveText` for the view.

## Concurrency model

- `Recorder` guards its session with a mutex; only one recording at a time.
- `Transcribe`, `StartTranslate` and `StartGenerate` record a job, mark what
  they work on, and return its ID; the work runs on a goroutine under a
  30-minute context that `jobs.Cancel` can end. Cancelling leaves the record as
  it was instead of marking it failed.
- WASAPI capture runs on a `LockOSThread` goroutine with COM initialized on
  that thread, and streams PCM into a named pipe consumed by FFmpeg. The same
  buffers are offered to the live sink, which drops them rather than ever
  blocking the capture thread.
- Live recognition runs on a goroutine of its own, owning one continuous
  streaming recognizer: capture only queues converted samples (a bounded queue,
  audio dropped rather than blocking), and revisions go back to the window as
  `asr.LiveSubtitle`.

## Process model

FFmpeg and ffprobe are spawned per job with `CREATE_NO_WINDOW` on Windows
(`proc_windows.go`). No long-running daemon: FFmpeg lives exactly as long as a
recording.

sherpa-onnx is **not** a process: its library is loaded once per process and
stays loaded, while a recognizer is created and destroyed per job — one
recognizer per transcription, and one per recording for live text.
