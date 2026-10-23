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

Long jobs are started from the view and report their result the same way. The
view polls while something it shows is still running (`App.pending`), so a
transcript or summary that finishes shows up without an event stream.

## Layout of the window

One window, one page at a time, with the library always on the left:

| Part | Shows |
|---|---|
| Sidebar | the recording form, the search field, and the library list |
| Library row | name, when it was made, how long it runs, what has been made of it; a right-click menu of the tabs and Delete |
| Recording page | the preview frame, the provisional live transcript, and Delete/Stop |
| Session page | name and details, and the Transcript, Translate and Summary tabs |

Recording lives in the sidebar rather than on a page of its own, so a finished
recording lands in the list it was started from, right above the form.

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
- `Transcribe` marks the media `pending` and returns; the work runs on a
  goroutine with a 30-minute context. `Translate` and `Generate` block the
  goroutine that calls them, which the view starts for the job.
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
