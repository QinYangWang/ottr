# Roadmap

Items are grounded in the current code, not aspirations. Done work is
described in the other documents; item 1 is kept here for what it leaves open.

Status legend: `open` = no code exists; `partial` = some code exists but the
requirement is not met.

---

## 1. Live in-recording transcription — `done`

Recognized speech now shows while recording: the capture's PCM goes to a live
sink, chunks are decoded at quiet points, and `transcript:live` is emitted and
shown in the window. See
[Transcription](transcription.md#live-in-recording-recognition).

What it still does not do:

- Recognition is continuous and in process now (see
  [Transcription](transcription.md)), so nothing is lost at a chunk boundary;
  dropping live audio when inference falls behind is reported instead of hidden.
- Only the first audio stream is transcribed, so system audio plus microphone
  gives live text from one of them.
- The live pass is provisional by design: the post-recording pass replaces it.

---

## 2. Jobs module: persisted task state, progress, cancellation — `done`

`internal/jobs/` records every long task in the `job` table (kind, state,
progress, error) and keeps the context that ends it, so `Cancel(id)` stops the
work. `Transcribe`, `StartTranslate` and `StartGenerate` all return a job ID;
the window reads the newest job of the media shown each frame, draws its
progress, offers **Cancel**, and shows the error of the last failed one. A job
still `running` at startup is marked `interrupted` and its media is cleared from
`pending`. See [Storage](storage.md#job) and
[Architecture](architecture.md#concurrency-model).

What it still does not do:

- Translation and summarization report progress only where the work has stages
  worth reporting; translation is one batch, so its bar is indeterminate.
- Nothing runs two jobs over one media file on purpose, but nothing forbids it
  either: starting a second summary while one runs leaves two rows and the
  later one wins.
- `job:progress` is broadcast for web pages that do not exist yet.

---

## 3. Import → full pipeline — `done`

`Library.Import` probes and inserts, then the window runs one `import` job per
file with audio: `asr.TranscribeNow` (the offline pass a recording gets), and,
when the `Translate + summarize imports` switch in Settings ▸ Translation is
on, translation into the Translate tab's language and a summary, in that order.
A file already in that language skips translation rather than spending quota on
it. One job means one progress bar and one Cancel for the whole chain. See
[UI](ui.md#main-window-and-library).

What it still does not do:

- The checkbox is per session, not per file, and is not remembered across
  launches.
- The chain is all or nothing: no "translate but do not summarize".
- The target language is the one last chosen in the Translate tab, which is
  English until the user changes it.

---

## 4. Model management UI — `done`

The **Models** section of Settings (Transcription tab) lists `ListModels()`
with sizes and the reason a folder is unusable, marks the model transcription uses, and switches
with `Use`; `Open folder` and `Re-check` are there too. The choice is
`settings.asr_model_dir`, read by `Manager.Open` and consumed by
`FindASRModel`. See [Models](models.md#choosing-a-model).

What it still does not do:

- GPU selection: the recognizer is configured with `provider = "cpu"`, and no
  build with CUDA providers is shipped.
- No download, install, load/unload or disk management: the user puts the files
  there.
- Only ASR models are listed; `Info.Kind` has room for llm and translation
  models, which are still remote.

---

## 4b. Area and camera capture — `open`

Two disabled cards of the source grid explain the gap in their tooltips:
`Area`, which would record part of the screen, and `Camera only`, which would
record a camera with no screen.

`gdigrab` already takes `-offset_x`, `-offset_y` and `-video_size`, so an area
is a rectangle and a selection gesture; a camera is an FFmpeg `dshow` input and
an enumeration of devices. Both need work in `capture` and a selection or device
UI, so neither is implemented.

---

## 5. macOS and Linux capture — `open`

The replacement design is [Native capture and media redesign](native-capture.md).
It uses cgo/native capture and encoding on all three platforms, includes
screenshots, and removes FFmpeg/ffprobe rather than adding platform-specific
FFmpeg inputs. The notes below are the old implementation direction.

**Requirement:** ScreenCaptureKit on macOS; XDG portal + PipeWire on Wayland;
X11/XCB on Linux.

**Current state:** `capture/backend_other.go` and `audio_other.go` report
`capture.ErrUnsupported`. The code compiles on all platforms; only Windows
captures, through `capture/backend_windows.go`.

**Notes:**
- `capture.Backend` is the seam: `Sources`, `Open`, `Snapshot`, `Close`, with a
  `Stream` of BGRA frames. A platform is a backend, not FFmpeg arguments.
- macOS requires Screen Recording and Microphone TCC permissions; Linux Wayland
  requires the XDG Desktop Portal and a PipeWire session.
- Reuse the `audioSpec`/named-pipe mechanism or replace it with FFmpeg's native
  audio demuxers where they exist (`avfoundation`, `pulse`).

---

## 6. Permissions handling — `open`

Permission and portal-session ownership are part of the
[native capture design](native-capture.md#platform-backends).

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
`go build` has been exercised. The native `sherpa-onnx-c-api` library and the
models are not bundled or located relative to a packaged app.

**Notes:** `mygo build` produces `.app`/`.dmg`, `.exe` + NSIS installer, and
`.deb`. Shipping needs decisions on: whether to bundle FFmpeg, whether to bundle
the sherpa-onnx native library, code signing/notarization, and license compliance for
everything bundled.

---

## 8. Settings table as a migration — `done`

`migrations/0006_settings.sql` creates `settings (key, value)`; the services
that used to create it lazily only read and write keys now. The keys are the
provider settings for translation and summaries, plus `asr_model_dir` for the
model chosen in the Models page.

---

## 9. Orphaned recording files — `done`

The policy is to surface them and let the user decide; nothing is deleted
silently. `Library.Orphans(dir)` lists the media files in the recordings folder
that no record refers to, the window shows "N recording files are not in the
library" under the card grid, and `Add back` (`Library.AddFiles`) or
`Delete files…` (`Library.DeleteFiles`, behind a modal that names the count)
acts on them. See [UI](ui.md#main-window-and-library).

What it still does not do:

- The scan runs on each library reload, not on a schedule, and covers only the
  folder Ottr writes recordings to — an imported file whose record is deleted is
  out of reach.
- `Add back` probes with FFmpeg like any import: a file FFmpeg cannot read comes
  back as an error rather than a record.

---

## 10. Chunked summarization — `done`

`summary.generateSummary` cuts the transcript into groups under 6000
characters, summarizes each part (`map`), then rolls the parts up into one
overview, key points and action items (`reduce`); chapters and highlights are
merged from the parts in time order so their timestamps stay real. The job's
progress follows the parts. See [Summary](summary.md#long-recordings-parts-then-a-rollup).

What it still does not do:

- The rollup is a second full pass over what the parts said, not a token-budget
  calculation: a two-hour recording still makes many map requests.
- A part is bounded by characters, not tokens, and the model's window is not
  known to Ottr, so a model with a small context can still refuse a part.

---

## 11. Source language detection — `done`

`asr.DetectLanguage` reads the script of the recognized text (kana → `ja`,
hangul → `ko`, Han → `zh`, Latin → `en`, otherwise `und`), `saveTranscript`
stores that, and the Transcript tab's **Spoken language** select overrules it
through `asr.SetLanguage`. Translation uses it, sending `Autodetect` when it is
`und`. See [Transcription](transcription.md#language).

What it still does not do:

- It is script counting, not language identification: French, Spanish and
  English all say `en`, and Cantonese says `zh`.
- Switching the language does not retranslate stored translations.

---

## 12. Higher-fidelity Windows capture — `partial`

Windows.Graphics.Capture now feeds all capture paths
([Native capture and media redesign](native-capture.md)): displays and windows
are captured by Windows itself, covered windows included, with the pointer
optionally drawn in. What is still missing from the item below is Media
Foundation encoding and the removal of FFmpeg, which is phase 2 of that
document.

`gdigrab` is GDI-based: it cannot capture hardware-accelerated/exclusive
fullscreen content (some games, DRM surfaces), and its frame pacing is weaker
than Desktop Duplication.

Screen, display and window capture now go through FFmpeg's `ddagrab`, which is
Desktop Duplication, and `gdigrab` remains only the fallback for a desktop or
display when the session will not lend one out. What is still missing is
talking to DXGI directly: `Windows.Graphics.Capture` would give per-window
frames without cropping the desktop, so a covered window would record
uncovered. That needs WinRT interop or a small native bridge.

Related smaller gaps:
- A window is a crop of the desktop, so anything covering it is recorded too.
- No 5-minute end-to-end recording has been manually verified (automated tests
  record ~5–11 s).

---

## 13. In-window playback — `open`

The implementation design is [In-window native playback](native-playback.md).
It uses native demux/decode and audio output with `ui.Bitmap`, not an external
player, FFmpeg or a prerequisite custom video element.

The window currently shows the live preview of a recording, but not the media
of a finished file: `Play` hands the file to the system's player, and the
timestamps in the transcript and summary are therefore not clickable. Native
container/codec support will be explicit rather than delegated to whatever the
system player happens to handle.

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
