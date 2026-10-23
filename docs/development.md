# Development

## Requirements

| Tool | Version | Why |
|---|---|---|
| Go | 1.27+ | MyGo requires 1.27 |
| FFmpeg + ffprobe | any recent build | probing, recording, audio extraction |
| sherpa-onnx | v1.12.17 binary | offline ASR (optional; transcription tests skip without it) |
| Windows 10/11 | — | recording is Windows-only today |

No cgo, no Node, no Bun. The window is native UI written in Go, so there is no
frontend to build.

## Run

```sh
go run ./cmd/studio
```

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
| `%APPDATA%\Ottr\bin\` | `sherpa-onnx.exe` + onnxruntime DLLs |

## Prepare ASR

1. Download a streaming Zipformer model, e.g.
   `sherpa-onnx-streaming-zipformer-zh-14M-2023-02-23.tar.bz2` from the
   sherpa-onnx `asr-models` release.
2. Extract it under `%APPDATA%\Ottr\models\`.
3. Copy `sherpa-onnx.exe`, `onnxruntime.dll`, and
   `onnxruntime_providers_shared.dll` into `%APPDATA%\Ottr\bin\`.

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
| `internal/asr` | real sherpa-onnx run on a real WAV |
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
3. If it runs a job, start it on a goroutine and change the state through
   `a.update`. Report it in `App.pending` so the view keeps drawing frames
   until it settles.
4. Test it with `ui.NewTester(a.view, w, h)`: click by label, then `tt.Frame()`
   until the job's result shows.
