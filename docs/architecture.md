# Architecture

## Layer boundaries

```
cmd/studio/main.go          composition root: opens the database, builds the view, opens the window
        │
        ├── ui/native/       the window, drawn by MyGo's toolkit: pages are Go functions of state
        │
        └── internal/
              library/       media records, import, remove
              capture/       recording session, platform capture adapters, preview frames
              media/         ffprobe probing
              asr/           sherpa-onnx invocation, live chunks, segmentation, subtitle export
              translate/     online translation clients, bilingual alignment, export
              summary/       remote LLM call, structured summary, Markdown
              models/        model + binary discovery and validation
              jobs/          background work: state, progress, error, cancellation
              storage/       SQLite open + migration runner
```

Rules the code follows:

- There is no web frontend and no IPC: `ui/native` calls the services directly,
  and the services call back into the window with `Window.Update`.
- The UI layer never touches FFmpeg, WASAPI, or SQL.
- `capture` does not import `asr`; live recognition arrives through the
  one-value `LiveStarter` type and the `LiveWriter` interface it returns.
- Platform-specific code is isolated by build tags: `*_windows.go` and
  `*_other.go` pairs in `capture`, `media`, and `asr`.

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
| Main window | source cards, the preview, full-row speaker/microphone dropdowns, Start recording, and theme/Library/Settings icon buttons |
| Library sidebar | search, import, and media rows with a right-click menu of the tabs and Delete |
| Library recording page | the preview frame, provisional live transcript, and Delete/Stop |
| Library session page | name and details, and the Transcript, Translate and Summary tabs |
| Sources page | the windows to record, after a search, beside the main recording form |
| Settings window | which model transcribes, and which provider translates and summarizes |

The main window is 420 × 800 and grows only for the source picker
(`App.openPane`, `App.closePane`). Library is 1180 × 800 and Settings is
920 × 660. Starting a recording opens or focuses Library, whose right side
shows the live capture and then the finished recording.

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
- Live recognition runs on a worker goroutine of its own, decoding 3-second
  chunks cut at the quietest point near their end.

## Process model

FFmpeg, ffprobe and sherpa-onnx are spawned per job with `CREATE_NO_WINDOW` on
Windows (`proc_windows.go`). No long-running daemon: FFmpeg lives exactly as
long as a recording; sherpa-onnx lives exactly as long as one transcription,
and a live recognition spawns one per chunk.
