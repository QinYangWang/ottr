# Development

## Requirements

| Tool | Version | Why |
|---|---|---|
| Go | 1.27+ | MyGo requires 1.27 |
| FFmpeg + ffprobe | any recent build | probing, recording, the live preview, audio extraction |
| sherpa-onnx | v2.0.1 C API library | offline ASR (optional; transcription tests skip without it) |
| C toolchain | gcc/clang | cgo: the ASR wrapper is C |
| Windows 10/11 | — | recording is Windows-only today |

No Node, no Bun. The window is native UI written in Go, so there is no frontend
to build. cgo is used only by `internal/asr/native`; `CGO_ENABLED=0` still
builds the whole app, with ASR unavailable and saying so.

FFmpeg and ffprobe are searched for in `<userData>/bin`, `%APPDATA%\Ottr\bin`,
the folders `winget` installs into (`%LOCALAPPDATA%\Microsoft\WinGet`), next to
the executable, and the `PATH` (`internal/tools`). Dropping the two binaries in
`<userData>/bin` is enough — no system `PATH` change — and so is
`winget install Gyan.FFmpeg` followed by `Re-check` in Settings, even in a
running app: the folders are read at the moment the program is looked for.
`Choose…` in Settings ▸ Transcription points Ottr at any `ffmpeg.exe` and
remembers it.

The sherpa-onnx library is looked for in two places: the app's own resources
(`resources/bin`, merged per platform at build time) and then `<userData>/bin`.
`PATH` is never used, because a library from there could be any build with any
dependencies. See [Resources](models.md#packaging) for shipping it with the app — that
is why `CGO_ENABLED=1 go tool mygo build` is the command, not plain
`go tool mygo build`.

## Run

```sh
CGO_ENABLED=1 go run ./cmd/studio
```

`CGO_ENABLED=1` is what makes ASR work; without it the app builds and runs, and
transcription says native ASR is unavailable. Likewise `CGO_ENABLED=1 go tool
mygo dev` and `CGO_ENABLED=1 go tool mygo build`: the MyGo tool defaults to
`CGO_ENABLED=0`, and a build without cgo has no ASR.

The window has no webview and so no devtools. `ui.NewTester` runs the view
without a window, for tests (see below).

## Layout on disk

```
cmd/studio/main.go      entry point
internal/               services (see architecture.md)
migrations/*.sql        embedded migrations
ui/native/              the window's state and its view
docs/                   this documentation
```

## Data locations

| Path | Contents |
|---|---|
| `%APPDATA%\Ottr\ottr.db` | SQLite database |
| `%APPDATA%\Ottr\recordings\` | recorded `.mp4` files |
| `%APPDATA%\Ottr\models\` | ASR model directories |
| `%APPDATA%\Ottr\bin\` | `sherpa-onnx-c-api.dll` + onnxruntime DLLs |

## Prepare ASR

1. Download a streaming Zipformer model, e.g.
   `sherpa-onnx-streaming-zipformer-zh-14M-2023-02-23.tar.bz2` from the
   sherpa-onnx `asr-models` release.
2. Extract it under `%APPDATA%\Ottr\models\`.
3. Copy the C API library — `sherpa-onnx-c-api.dll` on Windows,
   `libsherpa-onnx-c-api.dylib` on macOS, `libsherpa-onnx-c-api.so` on Linux —
   with its `onnxruntime` libraries into `%APPDATA%\Ottr\bin\`, from the
   release matching the pinned version in
   [Models](models.md#native-runtime). No `sherpa-onnx` executable is used.

Transcription then appears automatically in the Session page. Without these
files the UI shows the real error and no transcript is produced.

## Tests

```sh
go vet ./...
go test ./...
```

Test suites and what they touch:

| Package | Kind |
|---|---|
| `internal/storage` | real SQLite temp DB |
| `internal/library` | real SQLite, real SQL |
| `internal/media` | real ffmpeg generates a clip, real ffprobe probes it |
| `internal/capture` | real screen/audio recording (Windows) |
| `internal/asr` | real sherpa-onnx library run on a real WAV (skips without it) |
| `internal/translate` | real HTTP call to MyMemory |
| `internal/summary` | local `httptest` server exercising the production HTTP path |
| `ui/native` | the view in `ui.NewTester`, over a real SQLite database |

Tests that need an external dependency call `t.Skip` (never `t.Fatal` or a
fake) when it is missing: no ffprobe, no recorder device, no ASR model, no
network.

Run a single suite:

```sh
go test ./internal/capture -run TestRecordScreenWithSystemAudio -v
```

The capture suite records real screen content for several seconds per test, so
the full run takes ~25 s.

## Adding a migration

1. Create `migrations/000N_name.sql`.
2. It is embedded automatically by `migrations/migrations.go`.
3. Never edit an applied file — add a new one.

## Adding a page

1. Add the state it shows to `ui/native.App`, and a `func (a *App) page(c
   *ui.Context)` that draws it.
2. Wire it into `App.view`, beside the recording and session pages.
3. If it runs a job, record it with `internal/jobs` (`jobs.Run` returns its
   ID and gives the work a context and a progress function). Change the state
   through `a.update`. `App.pending` asks the job table whether something is
   still running, so the view keeps drawing frames until it settles and can
   offer `Cancel` (`Jobs.Cancel`).
4. Test it with `ui.NewTester(a.view, w, h)`: click by label, then `tt.Frame()`
   until the job's result shows.
5. To look at a page while working on it, run
   `OTTR_PREVIEW=<folder> go test ./ui/native -run TestPreview`: it writes PNGs
   of the main form, Library, the sources page and the settings window into
   that folder. See `ui/native/preview_test.go`.
