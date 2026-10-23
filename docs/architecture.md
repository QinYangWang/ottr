# Architecture

## Layer boundaries

```
cmd/studio/main.go          composition root: binds services, registers protocol, opens window
        │
        ├── ui/web/          embedded HTML/CSS/JS; talks to Go through window.mygo.call
        │
        └── internal/
              library/       media records, import, remove, media:// file serving
              capture/       recording session, platform capture adapters, preview relay
              media/         ffprobe probing
              asr/           sherpa-onnx invocation, segmentation, subtitle export
              translate/     online translation clients, bilingual alignment, export
              summary/       remote LLM call, structured summary, Markdown
              models/        model + binary discovery and validation
              storage/       SQLite open + migration runner
```

Rules the code follows:

- The UI layer never touches FFmpeg, WASAPI, or SQL. It only calls bound services.
- `capture` does not import `asr`; it depends on a one-method `Transcriber`
  interface (`capture.Transcriber`), which `asr.Service` satisfies.
- `asr`, `translate`, and `summary` do not import MyGo window APIs except for
  `mygo.Dialog` and event broadcasting.
- Platform-specific code is isolated by build tags:
  `*_windows.go` and `*_other.go` pairs in `capture`, `media`, and `asr`.

## Bound services

`mygo.BindAs` exposes these to the frontend. Each exported method becomes a
promise-returning function in the page.

| Name | Type | Key methods |
|---|---|---|
| `Library` | `library.Service` | `List`, `Get`, `Import`, `Remove` |
| `Recorder` | `capture.Recorder` | `Sources`, `State`, `Start`, `Stop` |
| `Transcription` | `asr.Service` | `Get`, `Search`, `Transcribe`, `Export` |
| `Translation` | `translate.Service` | `Get`, `Translate`, `Export`, `GetSettings`, `SaveSettings` |
| `Summary` | `summary.Service` | `Get`, `Generate`, `Export`, `GetSettings`, `SaveSettings` |
| `Models` | `models.Manager` | `FindSherpaBin`, `FindASRModel`, `ListModels` |

Methods that must not be callable from the page (`Open`, `AddRecording`,
`MediaHandler`) are either unexported or defined as package-level functions,
because `mygo.Bind` rejects methods whose signatures cannot be JSON-encoded.

## Custom protocol

A single `media://` scheme serves two things, routed by path in `main.go`:

- `media://localhost/<id>` → `library.MediaHandler` streams the media file with
  range support so the player can seek.
- `media://localhost/preview` → `capture.PreviewHandler` streams the live MJPEG
  preview of an active recording.

On Windows, WebView2 exposes custom schemes at `http://<scheme>.localhost`, so
the frontend rewrites the URL when `mygo.platform === "win32"`.

## Events (Go → page)

| Event | Payload | Emitted by |
|---|---|---|
| `library:changed` | `{}` | import, remove, status transitions |
| `recording:state` | `{recording, startedAt, error}` | recorder start/stop/failure |
| `transcript:ready` | `mediaID` | ASR completion |
| `translation:ready` | `Translation` | translation completion |
| `summary:ready` | `Summary` | summary completion |
| `transcript:live` | `LiveSubtitle` | declared; **not emitted yet** (see ROADMAP) |

## Concurrency model

- Each bound method runs on its own goroutine, so any method may block.
- `Recorder` guards its session with a mutex; only one recording at a time.
- Long jobs (`Transcribe`, `Translate`, `Generate`) run with a `context`
  timeout. `Transcribe` returns immediately and finishes in the background;
  `Translate` and `Generate` currently block the caller until done.
- WASAPI capture runs on an `LockOSThread` goroutine with COM initialized on
  that thread, and streams PCM into a named pipe consumed by FFmpeg.

## Process model

FFmpeg and ffprobe are spawned per job with `CREATE_NO_WINDOW` on Windows
(`proc_windows.go`). No long-running daemon: FFmpeg lives exactly as long as a
recording, sherpa-onnx lives exactly as long as one transcription.
