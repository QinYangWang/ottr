# Ottr — Documentation

Offline-first screen recorder with live transcription, translation, AI summaries,
and a local video knowledge base.

- **Language:** Go 1.27
- **UI:** [MyGo](https://github.com/egoist/mygo) v0.6.2, web frontend (embedded, no build step)
- **Database:** SQLite via `modernc.org/sqlite` (pure Go, no cgo)
- **Media:** FFmpeg / ffprobe (system binaries)
- **Capture:** GDI screen capture + WASAPI audio (Windows)

## Contents

| Document | Covers |
|---|---|
| [architecture.md](architecture.md) | Module layout, boundaries, data flow, threading |
| [recording.md](recording.md) | Screen/audio capture pipeline, sources, lifecycle |
| [transcription.md](transcription.md) | sherpa-onnx integration, segmentation, subtitle handling |
| [translation.md](translation.md) | Online translation providers, bilingual alignment, export |
| [summary.md](summary.md) | Remote AI summarization, structured output, Markdown |
| [storage.md](storage.md) | SQLite schema, migrations, cascade behavior |
| [models.md](models.md) | ASR model discovery and validation |
| [ui.md](ui.md) | Library and Session pages, interaction contracts |
| [development.md](development.md) | Running, testing, environment requirements |
| [ROADMAP.md](ROADMAP.md) | What is not implemented yet |

## Status at a glance

Phases 1–5 are implemented and tested on Windows. Cross-platform capture,
live in-recording subtitles, and packaging remain open — see
[ROADMAP.md](ROADMAP.md).
