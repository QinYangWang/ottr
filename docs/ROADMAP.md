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
Finalized lines are translated into the target language as they arrive, but a line
still being spoken is not sent until it freezes, and live translations are never
saved — the stored transcript is translated again after the recording stops.

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
when the `Translate + summarize imports` switch in Settings ▸ Provider is
on, translation into the Translate tab's language and a summary, in that order.
A file already in that language skips translation rather than spending quota on
it. One job means one progress bar and one Cancel for the whole chain. See
[UI](design/README.md).

What it still does not do:

- The checkbox is per session, not per file, and is not remembered across
  launches.
- The chain is all or nothing: no "translate but do not summarize".
- The target language is the one last chosen in the Translate tab, which is
  English until the user changes it.

---

## 4. Model management UI — `done`

The model list lives in the **Transcription** section of Settings: `ListModels()`
with sizes and the reason a folder is unusable, the model transcription uses marked, and `Use`
to switch. The choice is
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

Two disabled tiles of the capture panel, marked SOON, explain the gap in their
tooltips: `Area`, which would record part of the screen, and `Camera`, which
would record a camera with no screen.

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

**Current state:** `mygo.json` names the bundle and `cmd/setup` fetches the pinned
`sherpa-onnx` runtime for each target into `resources/<os>-<arch>/bin`, which
`CGO_ENABLED=1 go tool mygo build` merges in; a hand-built `setup.exe` exists but is not
part of a release pipeline. There is still no signed installer, no notarization, and no
decision on bundling models or FFmpeg.

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
acts on them. See [UI](design/README.md).

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

Windows.Graphics.Capture now feeds every capture path
([Native capture and media redesign](native-capture.md)): displays and windows are
captured by Windows itself, covered windows included, with the pointer optionally drawn
in. `ddagrab` and `gdigrab` are gone from the Windows backend.

What is still missing is phase 2 of that document: Media Foundation encoding and the
removal of FFmpeg, so capture still hands raw frames to FFmpeg for muxing.

Related smaller gaps:
- No 5-minute end-to-end recording has been manually verified (automated tests record
  ~5–11 s).
- The Windows backend has no automated coverage for a covered window or a display that
  changes resolution mid-recording.

---

## 13. In-window playback — `done`

Playback happens inside the Library item page: `internal/player` decodes audio and video
through FFmpeg pipes into a `ui.VideoPlayer` and `audio.Player` — never the system player
— with play/pause, seeking from the transport bar, mute, fullscreen, and transcript rows
that seek the media. See [In-window native playback](native-playback.md).

What it still does not do:
- Volume is mute-only (`SetVolume(0|1)`); there is no level control.
- The decoder is FFmpeg in pipes, not native demux/decode; an A/V clock is FFmpeg's, not
  Ottr's, so long-term drift is not controlled.
- Only H.264/AAC MP4 from Ottr's own recorder is expected; other containers are untested.

---

## 14. Offline translation and offline summaries — `open` (deferred by decision)

Both were explicitly deferred in favor of online providers. Offline translation
would use a local MT model; offline summarization would use llama.cpp with a
GGUF model. The remote paths in `translate/client.go` and `summary/client.go`
are behind interfaces/thin wrappers, so adding local implementations does not
require restructuring.

---

## 15. Speaker library — `done`

Diarization names the voices in a recording
([Transcription](transcription.md#speakers)), and Settings ▸ Speakers is the library they
accumulate in: name, avatar, how many recordings they were heard on, for how
long, and when last. **Rename** applies to every transcript; **Remove** forgets
the voice. `asr.Speakers()` counts once per recording, from `speaker` joined
through `segment` and `transcript` (migrations `0007_speaker.sql`,
`0008_global_speakers.sql`).

What it still does not do:

- No **Merge duplicates** and no search, though the design shows both: two rows
  for one voice can only be fixed by removing one.
- Removing a speaker leaves its id on the segments, which then fall back to a
  numbered label; the audio is not reassigned.
- A row has no embedding, sample or delete-the-audio action, and nothing limits
  how many voices the library collects.

---

## 16. Canvas items the code does not do yet — `open`

The Doop frames of 2026-11-06 show two things with no code behind them:

- **A global recording shortcut.** Ctrl + Shift + R starts and stops a
  recording while the Ottr window has focus, and the capture panel shows it;
  Ottr registers no system-wide hotkey, so it does nothing from other apps.
- **Retry from the timestamp a transcription stopped at.** Recognition keeps no
  partial transcript, so the failure dialog offers a full retry.

---

## 17. Redesign gaps between the canvas and the window — `partial`

The window follows the "studio console + highlighter" canvas of 2026-11-06
(tokens, fonts, layout at 1280 / 900 / 420, every screen in
[docs/design](design/README.md)). These parts of the frames have no code
behind them yet, or are drawn differently:

- **Live level meters.** The live panel draws green level bars beside
  System audio; the capture service reports no audio level, so Go shows the
  device name alone.
- **Recording size and format.** The live panel says "1.4 GB · 1080p30"; Go
  shows the resolution only.
- **Monitor names.** The display select reads "Display 1 — DELL U2723QE";
  the capture backend names displays by number only.
- **中文 flag and "Has 中文" filter.** Rows of translated media carry an amber
  中文 chip and the library has a "Has 中文" chip; translations are not
  counted per media in the library query, so neither exists.
- **Speed chip.** The transport's "1×" chip: the player has no playback rate.
- **Automatic translate and summarize after a recording.** The pipeline card
  runs translation and summary on their own after transcription; Go runs
  them on request (and for imports only when the Provider switch is on).
- **Provider tiles.** The canvas offers Free online / OpenAI-compatible /
  Custom AI model; Go offers the DeepSeek, OpenRouter, Ollama and Custom
  presets. The key "Works" check and **Re-check** button are not built.
- **Row hover actions.** The canvas shows a trash icon on the hovered row;
  Go keeps delete in the row's ⋯ menu.

When one of these is built, change the frame first if the code differs from
it, then remove the line here.

---

## Non-goals

Explicitly out of scope, per the product definition: timeline editing, effects,
transitions, filters, and any video editing capability.
