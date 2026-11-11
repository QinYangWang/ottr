# Architecture

## Layer boundaries

```
cmd/studio/main.go         composition root: opens the database, builds the view, opens the window
        │
        └── internal/
            native/        the window, drawn by MyGo's toolkit: pages are Go functions of state
            library/       media records, import, remove
            capture/       capture.Backend + platform backends (cgo), recording session, preview
            media/         ffprobe probing
            asr/           sherpa-onnx through cgo (native/), PCM conversion, segmentation, export
            translate/     online translation clients, bilingual alignment, export
            summary/       remote LLM call, structured summary, Markdown
            models/        model + binary discovery and validation
            jobs/          background work: state, progress, error, cancellation
            storage/       SQLite open + migration runner
```

Capture is native: [Native capture and media redesign](native-capture.md)
describes it and what is still to come. Its phase 1 (common frame pipeline and
in-process preview) is implemented for Windows; encoding and
muxing are still FFmpeg until phase 2. The sections below describe the code as
it exists today.

Rules the code follows:

- There is no web frontend and no IPC: `internal/native` calls the services directly,
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
  `Recorder` and `Previewer`.

## The window

`internal/native.App` holds the state the window shows and the `view` function that
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

## Layout of the window

One native window holds everything; it changes shape with its width (1280 wide,
900, 420 narrow) and never opens a second window of its own:

| Part | Shows |
|---|---|
| Title strip | `ottr`, what the window is when it is not the library, and the page's own tools |
| Sidebar (`navSidebar`, from 1100 px) | Library, the four Settings sections, and the disk-usage card |
| Rail (`rail`, from 900 px) | New recording, Library, summaries, Speakers and Settings, as 36 px icon buttons |
| Library (`libraryPage`) | search, the filter chips and the media list or grid: a 16:9 thumbnail, status chips, the name, the source and the date, with a menu of the actions |
| Recording form (`sourceSection`, `audioSection`) | source cards, the preview, the display select, and the audio, live-transcript and translate selects |
| Recording page (`recordingPage`, while a recording runs) | the whole window in the item page's shell: the REC strip and the file being written on one line, Pause/Stop, the live picture in the stage, the choices (source, the two sounds, the model) as rows with their selects, and every line of the live transcript with its translation on the right |
| Session page (`sessionPage`) | the way back, the name with `Show file` and `Delete`, the player stage, the fact strip, and the Transcript/Summary tabs — on a fresh recording the transcript is the live text, with `Transcribe again` for the full pass |
| Settings (`settingsPage`) | the section chosen in the sidebar: the model, the AI endpoint and prompts, the speaker library, or the look |
| First run (`onboardingPage`) | before a model is installed: the three models with their download, Later, and the models folder |

The wide window lays the sidebar, the library and the 360 px capture panel side by
to side; from 900 px the sidebar becomes the rail and one page shows at a time; at
420 px the page fills the window. A recording running takes the whole window over —
the rail and the library come back when it stops — and stopping it opens the
finished recording's page.

`internal/native/theme.go` holds the palette: it starts from
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
