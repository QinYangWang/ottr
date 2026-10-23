# Roadmap

Everything below is **not implemented**. Items are grounded in the current code,
not aspirations. Done work is described in the other documents.

Status legend: `open` = no code exists; `partial` = some code exists but the
requirement is not met.

---

## 1. Live in-recording transcription — `partial`

**Requirement:** recognize speech and show subtitles *while* recording.

**Current state:** `asr.LiveEvent` (`transcript:live`) is declared but never
emitted. `capture.Recorder.Stop` transcribes the finished file only. The
`#/record` page has a `#live-subtitle` element that always reads
"Listening for audio…".

**Notes for implementation:**
- The audio that FFmpeg receives is already PCM in `capture/audio_windows.go`.
  A tee to a streaming recognizer is the natural insertion point.
- sherpa-onnx ships an online/streaming mode (`sherpa-onnx-microphone`,
  `SherpaOnnxOnlineRecognizer*` in the C API). Driving it as a subprocess needs
  a pipe-based interface, or a second binary that reads PCM from stdin.
- Must not block recording. Use a bounded queue and drop frames on overflow
  rather than applying backpressure to the capture thread.
- Final segments must reconcile with the post-recording pass, which is more
  accurate; the live pass should be treated as provisional.

---

## 2. Jobs module: persisted task state, progress, cancellation — `open`

**Requirement:** background tasks expose status, progress, error, and
cancellation; long tasks are cancellable.

**Current state:** `internal/jobs/` does not exist. There is no `job` table.
Progress is a spinner only. `Transcribe` runs in the background but cannot be
cancelled. `Translate` and `Generate` block the caller until finished.

**Notes:**
- Add `migrations/0005_jobs.sql` with `job(id, media_id, kind, state, progress,
  error, created_at, updated_at)`.
- Services return a job ID; the page subscribes to a `job:progress` event.
- Cancellation needs a registry of `context.CancelFunc` keyed by job ID and a
  bound `Jobs.Cancel(id)` method.
- The two blocking calls (`Translate`, `Generate`) should move to the same
  fire-and-forget + event pattern that `Transcribe` already uses, for
  consistency.

---

## 3. Import → full pipeline — `partial`

**Requirement:** importing a file runs probe → audio extract → normalize → ASR →
translate → summarize → save.

**Current state:** `Library.Import` probes and inserts. It does not transcribe,
translate, or summarize. Recording does auto-transcribe (see
`Recorder.Stop`), but import does not.

**Notes:** reuse the same background path as recording, ideally expressed as
jobs (item 2). Consider making the chain opt-in per import so a user with a paid
API key is not billed unexpectedly.

---

## 4. Model management UI — `partial`

**Requirement:** detect, validate, show size, load, unload, switch models, CPU/GPU.

**Current state:** `models.Manager` detects, validates, and reports size and
errors. There is **no UI**: no Settings/Models page exists, `Models` is bound but
never called from the frontend, and `FindASRModel` always takes the first valid
directory, so switching is impossible.

**Notes:**
- Add a Models page listing `ListModels()` results with sizes and validation
  errors, plus an "open folder" action.
- Model selection needs a persisted `asr_model_dir` setting consumed by
  `FindASRModel`.
- GPU selection is not wired anywhere; sherpa-onnx accepts `--provider`, but no
  build with CUDA providers is shipped.

---

## 5. macOS and Linux capture — `open`

**Requirement:** ScreenCaptureKit on macOS; XDG portal + PipeWire on Wayland;
X11/XCB or `ffmpeg x11grab` on Linux.

**Current state:** `capture/sources_other.go` and `audio_other.go` return
`"screen recording is not implemented on <os> yet (Phase 6)"`. The code compiles
on all platforms; only Windows records.

**Notes:**
- `resolveSource` is the seam: return platform-appropriate FFmpeg input args
  (`avfoundation` / `x11grab` / `kmsgrab`).
- macOS requires Screen Recording and Microphone TCC permissions; Linux Wayland
  requires the XDG Desktop Portal and a PipeWire session.
- Reuse the `audioSpec`/named-pipe mechanism or replace it with FFmpeg's native
  audio demuxers where they exist (`avfoundation`, `pulse`).

---

## 6. Permissions handling — `open`

**Requirement:** explain correct authorization; never bypass it.

**Current state:** no permission checks or guidance anywhere. On Windows GDI
capture currently works without prompting; the macOS/Linux paths that would need
permissions are not implemented yet.

**Notes:** detect denial, show OS-specific instructions (System Settings →
Privacy → Screen Recording on macOS; portal approval on Wayland), and avoid
retry loops that look like bypass attempts.

---

## 7. Packaging and signing — `open`

**Requirement:** installers for the three platforms.

**Current state:** there is no `mygo.json`, no `mygo.config.ts`, and only
`go build` has been exercised. `sherpa-onnx.exe` and the models are not bundled
or located relative to a packaged app.

**Notes:** `mygo build` produces `.app`/`.dmg`, `.exe` + NSIS installer, and
`.deb`. Shipping needs decisions on: whether to bundle FFmpeg, whether to bundle
sherpa-onnx binaries, code signing/notarization, and license compliance for
everything bundled.

---

## 8. Settings table as a migration — `open`

`settings` is created lazily by `translate` and `summary` with
`CREATE TABLE IF NOT EXISTS`. Two services doing DDL at runtime is fragile: a
schema change must be edited in two places and races on first launch. Move it
into `migrations/0005_settings.sql` (or fold it into the jobs migration).

---

## 9. Orphaned recording files — `open`

`Library.Remove` deletes the media row and, only when the user opts in, the
media file. Files under `%APPDATA%\Ottr\recordings\` whose row was removed
without the checkbox are left on disk forever. There is no cleanup view or
startup reconciliation. Decide the policy (keep silently, surface a "N orphaned
files" notice, or always delete managed recordings) and implement it.

---

## 10. Chunked summarization — `open`

`summary.Generate` sends the entire transcript in a single prompt. Transcripts
longer than the model's context window will be truncated by the provider or
fail. A map-reduce (per-chapter summaries, then a rollup) or sliding-window
approach is needed for long recordings.

---

## 11. Source language detection — `open`

`transcript.language` is written as `'zh'` unconditionally by
`asr.saveTranscript`, and translation uses it as the source language. An English
recording is therefore declared Chinese. Either detect the language (sherpa-onnx
offers language identification) or let the user state it.

---

## 12. Higher-fidelity Windows capture — `open`

`gdigrab` is GDI-based: it cannot capture hardware-accelerated/exclusive
fullscreen content (some games, DRM surfaces), and its frame pacing is weaker
than Desktop Duplication. `Windows.Graphics.Capture` (WGC) + DXGI Desktop
Duplication is the stated target. This needs WinRT interop or a small native
bridge; it is intentionally deferred because `gdigrab` gives a working, driver-free
baseline today.

Related smaller gaps:
- Window capture matches by title; duplicate titles or unusual characters fail.
- No 5-minute end-to-end recording has been manually verified (automated tests
  record ~5–11 s).

---

## 13. Container playback support — `open`

MKV and MOV playback depends on WebView2's codecs. Unsupported containers
currently surface as a player that does not start, with no explanation. Options:
detect support and show a clear message, or remux/transcode on import.

---

## 14. Offline translation and offline summaries — `open` (deferred by decision)

Both were explicitly deferred in favor of online providers. Offline translation
would use a local MT model; offline summarization would use llama.cpp with a
GGUF model. The remote paths in `translate/client.go` and `summary/client.go`
are behind interfaces/thin wrappers, so adding local implementations does not
require restructuring.

---

## Non-goals

Explicitly out of scope, per the product definition: timeline editing, effects,
transitions, filters, and any video editing capability.
