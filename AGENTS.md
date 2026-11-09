# AGENTS.md

Ottr is a local-first screen recorder with live and offline transcription, translation
and AI summaries, plus a searchable media library. The window is native [MyGo]
(https://github.com/egoist/mygo) UI written in Go — no webview, no HTML, no IPC.
SQLite stores everything; FFmpeg decodes and probes media; sherpa-onnx (cgo) does ASR.

- Go 1.27, Windows 10/11 is the supported capture platform
- Module `ottr`, entry point `cmd/studio`
- UI code: `internal/native`; services: the other `internal/*` packages

## Documentation index

| Document | Read it for |
|---|---|
| [docs/README.md](docs/README.md) | Documentation contents and current status |
| [docs/architecture.md](docs/architecture.md) | Layer boundaries, data flow, threading rules |
| [docs/development.md](docs/development.md) | Prerequisites, run commands, tests, adding a page or migration |
| [docs/design/README.md](docs/design/README.md) | Screens, breakpoints, "studio console" tokens, Go file mapping |
| [docs/recording.md](docs/recording.md) | Capture pipeline, sources, lifecycle |
| [docs/native-capture.md](docs/native-capture.md) | Native cgo capture, preview, encoding plan |
| [docs/native-playback.md](docs/native-playback.md) | In-window video/audio playback |
| [docs/transcription.md](docs/transcription.md) | ASR integration, segmentation, live subtitles |
| [docs/asr-cgo.md](docs/asr-cgo.md) | sherpa-onnx C API shim |
| [docs/translation.md](docs/translation.md) | Translation providers, alignment, export |
| [docs/summary.md](docs/summary.md) | Remote summarization and structured output |
| [docs/models.md](docs/models.md) | ASR model discovery, validation, packaging |
| [docs/storage.md](docs/storage.md) | SQLite schema, migrations, cascades |
| [docs/ROADMAP.md](docs/ROADMAP.md) | What is not implemented yet |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Branch, commit and review conventions |
| [code-standards.md](code-standards.md) | Code style and the bar every PR must clear |

## Quick start

```sh
CGO_ENABLED=1 go run ./cmd/studio   # the app
go vet ./... && go test ./...       # checks before any PR
```

`CGO_ENABLED=1` is required for ASR; without cgo the app still builds and reports that
native ASR is unavailable. FFmpeg/ffprobe must be on `PATH` or in `%APPDATA%\Ottr\bin`.
Details: [docs/development.md](docs/development.md).

## Repository map

```
cmd/studio/        composition root: DB, services, view, window
cmd/setup/         installer/packaging helper
internal/native/   the window: App state, pages, theme, i18n, player, kit/
internal/{capture,asr,media,library,translate,summary,models,jobs,storage,player,tools,settings}/
migrations/        embedded SQL migrations (append only)
docs/              documentation; docs/design holds the UI reference
.agents/skills/        project skills (Doop canvas workflow, Ottr redesign)
resources/<plat>/  third-party native libraries shipped with the app
```

## Invariants

- The shipped app has **no web UI**: Go/MyGo only. `docs/design` HTML is reference.
- No visible change starts in Go: Doop frame first, then `docs/design`, then code.
- The UI layer never touches SQL, FFmpeg or WASAPI; it calls services.
- Only SenseVoice, FunASR Nano and Qwen3 ASR are accepted models.
- Every user-facing string goes through `a.tr` / `a.trf` and has an `i18n.go` entry.
- Speaker identities are never invented: `Speaker N`, renameable globally.
- Migrations are append-only.
- Background work updates the view only through `a.update`.
- Tests that need ffmpeg, a model, a device or the network call `t.Skip`, never fake it.

## Doop is the source of truth: change the UI there first

Every screen lives in the Doop canvas. **A UI change starts in Doop, never in Go.**
The order is fixed:

1. Change the frame on the canvas and check it with `get_frame_screenshot`.
2. Regenerate `docs/design/pages` and `docs/design/screens` from the canvas:

   ```sh
   DOOP_CLIENT="python3 <path-to>/doop.py" node .agents/skills/doop-canvas/scripts/build.js --shots
   ```

   `DOOP_CLIENT` is a Doop MCP client taking `call <tool> -` with JSON on stdin;
   without it `build.js` reads the gitignored `.doop/frames/` cache.
3. Mirror the frame in `internal/native`; tests and the mapping table follow.
   Lay the Go window beside the frame with
   `OTTR_PREVIEW=<dir> OTTR_PREVIEW_CJK=<cjk font> go test ./internal/native -run TestPreview`.
4. Only then update prose docs.

A PR that changes a screen without the matching Doop frame and regenerated
screenshot is incomplete, even if the Go code looks right. The full workflow,
including prototype wiring and the CSV of frame sizes, is the
[doop-canvas skill](.agents/skills/doop-canvas/SKILL.md).
