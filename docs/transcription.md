# Transcription

Local, offline speech recognition using sherpa-onnx streaming Zipformer models,
called **in process** through its C API. The library is loaded at run time, so
Ottr starts and works without it; only transcription needs it.

## Decision: the C API through cgo, not a process

`sherpa-onnx` used to be spawned as a process per transcription, with its stdout
scraped for token JSON, and live audio cut into 3–5.5 s chunks, each with a fresh
recognizer. That is gone:

- `internal/asr/native` wraps the C API through cgo. `native/sherpa_shim.c`
  loads `sherpa-onnx-c-api` with `LoadLibraryExW` / `dlopen` and resolves the
  entry points once; the configuration structs come from the vendored header
  `native/sherpa_c_api_v2.0.1.h`, so their layout is sherpa-onnx's own, never
  guessed.
- Batch recognition and live text both use the same streaming recognizer. One
  recording is one utterance chain: an endpoint ends an utterance and starts the
  next, so words are not cut where a chunk used to end.
- A `CGO_ENABLED=0` build compiles, does everything except ASR, and answers
  “native ASR is not available in this build”. There is no executable fallback.
- A packaged app carries the library in its own resources, so an install needs
  no separate ASR setup; see [Models](models.md#packaging).

Nothing above `internal/asr` sees a C type, and no ASR file calls
`exec.Command`.

## Pipeline

```
media file ─► ffmpeg ─► 16 kHz mono s16le WAV (temp) ─► Converter ─► recognizer ─► tokens ─► GroupTokens ─► segment rows
```

1. `ExtractWav` runs `ffmpeg -vn -acodec pcm_s16le -ar 16000 -ac 1` into the
   system temp directory.
2. `readWav` reads it into float samples; `Converter` turns interleaved PCM at
   any rate or channel count into mono 16 kHz float32, keeping its fractional
   position across calls.
3. `recognizeWav` feeds the recognizer one second at a time. Between pieces it
   checks the job's context, so a cancelled job stops at a piece boundary rather
   than inside a native decode.
4. `GroupTokens` turns the token/timestamp arrays into subtitle lines.
5. Segments are written in one transaction, and `media.transcript_status`
   becomes `done`.

Progress is still coarse — extraction done, then recognition done — because
FFmpeg reports no position. `internal/media`'s native decoder will replace
step 1 and with it the progress reporting.

## Segmentation rules

`GroupTokens(tokens, timestamps)` groups tokens into readable lines:

- Break when the gap since the previous token exceeds **750 ms** (a pause).
- Break after a token containing `。！？!?`.
- Hard cap at 25 tokens per line.
- Each segment's `StartMs` is the timestamp of its first token; `EndMs` is the
  last token's time plus a small tail so subtitles stay legible.
- Empty results are dropped; a text-less group never reaches the database.

This is deliberately simple. It is not sentence-level punctuation restoration —
see ROADMAP.

## Data model

`transcript` is one row per media (`language`, `created_at`), `segment` holds
ordered rows (`idx`, `start_ms`, `end_ms`, `text`). Deleting a media row cascades
to transcripts and segments through foreign keys.

Re-running transcription replaces the previous transcript for that media inside
a single transaction, so a partial failure never leaves a half-written result.

## Language

`transcript.language` is what translation takes as its source, so it is
`DetectLanguage(segments)`, not a constant: it counts the scripts the recognizer
produced — kana means Japanese, hangul Korean, Han Chinese (unless Latin
outnumbers it), and Latin letters English — and answers `und` when there is
nothing to judge by. The limits are real: an English word inside Chinese does
not change the answer, but French, Spanish and English are all Latin letters and
all say `en`.

That guess is therefore an answer the user can overrule: the Transcript tab has
a **Spoken language** select (Detected / English / Chinese / Japanese) writing
through `asr.SetLanguage`, which is what the next translation reads. Switching
it does not retranslate what is already stored; translate again for that.

## Status transitions

`media.transcript_status`: `none → pending → done | error`.

`Transcribe` marks the media `pending`, records a job (`jobs.KindTranscribe`)
and returns its ID at once; the work runs under the job's context, which
carries a 30-minute limit and can be cancelled. Progress is reported at the
stages that exist — extraction done, then recognition done — so the window's
bar is coarse and honest rather than invented.

An import runs the same work through `TranscribeNow`, inside its own
`jobs.KindImport` job, so one progress bar and one Cancel cover the whole import
chain. See [UI](ui.md#main-window-and-library).

Errors mark the row `error` and surface the real reason (missing native runtime,
missing model, FFmpeg failure, sherpa-onnx refusing the model) in the tab that
offered the button. Cancelling marks it back to `none`: nothing was produced, so
nothing failed. Nothing is fabricated.

## Search and export

- `Search(mediaID, query)` performs a `LIKE %query%` over that media's segments;
  an empty query returns all segments.
- `Export` writes SRT, VTT, or TXT through a native save dialog. Formats are
  produced by `FormatSRT/FormatVTT/FormatTXT` in `formats.go`.

Time formatting is `HH:MM:SS,mmm` for SRT and `HH:MM:SS.mmm` for WebVTT, with
negative values clamped to zero.

## Testing

- `TestGroupTokens` — pause split and punctuation split produce exactly two
  segments with correct boundaries.
- `TestConverter*` — 16 kHz mono passes through unchanged, channels are
  averaged, 48 kHz becomes 16 kHz, ragged splits convert exactly like one call,
  empty input resets nothing, output is clipped, and a flush invents no audio.
- `TestDetectLanguage` — Chinese, Japanese, Korean, English, a Chinese line with
  an English word, and nothing to judge by.
- `TestTranscriptLanguageIsDetectedAndCanBeSet` — a saved transcript says what
  its text is written in, and `SetLanguage` overrules it (and refuses a media
  file with no transcript).
- `TestFormatSRT/VTT/TXT` — timestamp text and header.
- `TestTranscriptSaveAndGet` — write, read back, `transcript_status = done`,
  substring search.
- `TestTranscribeRealModel` — **real** recognition of the installed model's
  bundled `test_wavs/0.wav` through the native library; asserts non-empty
  segments with ordered, sane timestamps. Skips (not fakes) when the runtime or
  model is absent.

## Live, in-recording recognition

The recorder hands every PCM buffer the capture produces to a live sink
(`capture.LiveStarter`), on a separate goroutine. `asr.StartLive` builds that
sink.

1. `Write` converts the interleaved f32le buffer to mono 16 kHz and queues it to
   the sink's single owner goroutine. The queue is bounded by audio duration
   (`liveQueueAudio`); capture never waits.
2. The owner feeds the samples to the recognizer and runs every available decode
   step, then reads the current result as a **partial revision**.
3. On an endpoint it publishes the final revision, resets the decoder and
   remembers where the utterance began, so native timestamps — which restart at
   zero after a reset — land back on the recording's own timeline.
4. `Close` flushes the converter, publishes the last final revision and waits for
   the stream and recognizer to be destroyed.

If the recognizer falls behind, live audio is dropped (never the audio written
to the media file) and, past `liveWarnAudio`, a warning revision says the live
text is behind. The post-recording pass remains the transcript of record.

### Revisions in the window

`LiveSubtitle` carries an `Utterance` number:

- the same number again: replace that line's text, in place;
- `Final: true`: freeze the line; the next utterance is a new line;
- the pane keeps the newest six finalized lines plus the one being spoken.

`Recorder.State.Live` carries the same revision, so a window that polls State
cannot append the same line twice. Provisional text is never written to SQLite:
the recorded file's transcript replaces it after Stop.

## Not implemented

- Native audio decoding (FFmpeg extracts the WAV for now), which is
  [Native capture and media redesign](native-capture.md).
- Real language identification (sherpa-onnx LID): the language is guessed from
  the script the transcript is written in, and the user can overrule it.
