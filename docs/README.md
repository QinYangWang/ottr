# Ottr — Documentation

Offline-first screen recorder with live transcription, translation, AI summaries,
and a local video knowledge base.

- **Language:** Go 1.27
- **UI:** [MyGo](https://github.com/egoist/mygo) v0.6.2, native UI written in Go (no webview, no HTML)
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
| [ui.md](ui.md) | The window: library, recording and session pages |
| [development.md](development.md) | Running, testing, environment requirements |
| [ROADMAP.md](ROADMAP.md) | What is not implemented yet |

## Status at a glance

Recording, offline transcription with live in-recording subtitles, online
translation and AI summaries are implemented and tested on Windows, in a
native window. Cross-platform capture, in-window playback and packaging remain
open — see [ROADMAP.md](ROADMAP.md).
