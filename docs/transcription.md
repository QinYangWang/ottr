# Transcription

Local, offline speech recognition using sherpa-onnx streaming Zipformer models.

## Decision: sherpa-onnx binary, not a Go binding

The `sherpa-onnx-go` module requires cgo and platform-specific prebuilt
archives. Ottr is a pure-Go build (MyGo needs no cgo), so instead the
`sherpa-onnx` **executable** is invoked as a subprocess.

Trade-off: one process spawn per transcription (~0.2 s model load for the 14M
int8 model), in exchange for keeping the build cgo-free and the dependency
optional. On the development machine a 5.6 s clip transcribes in **0.16 s**
(RTF 0.028), so the spawn cost is not the bottleneck.

## Pipeline

```
media file ─► ffmpeg ─► 16 kHz mono s16le WAV (temp) ─► sherpa-onnx ─► token JSON ─► GroupTokens ─► segment rows
```

1. `ExtractWav` runs `ffmpeg -vn -acodec pcm_s16le -ar 16000 -ac 1` into the
   system temp directory.
2. `RunSherpa` invokes the binary with `--tokens/--encoder/--decoder/--joiner`,
   `--provider=cpu`, `--num-threads=2`, and the WAV path.
3. sherpa-onnx prints human-readable progress to stdout **and** one JSON object
   per utterance line. `RunSherpa` scans lines starting with `{` and decodes
   them; all other lines are ignored. Combined output is captured so failures
   carry the real stderr text.
4. `GroupTokens` turns the flat token/timestamp arrays into subtitle lines.
5. Segments are written in one transaction, and `media.transcript_status`
   becomes `done`.

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

## Status transitions

`media.transcript_status`: `none → pending → done | error`.

`Transcribe` sets `pending` synchronously, then does the work on a background
goroutine with a 30-minute context timeout. The page reacts to
`library:changed` (spinner while `pending`) and `transcript:ready` (reload).

Errors mark the row `error` and surface the real reason (missing binary, missing
model, FFmpeg failure, sherpa failure). Nothing is fabricated.

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
- `TestFormatSRT/VTT/TXT` — timestamp text and header.
- `TestTranscriptSaveAndGet` — write, read back, `transcript_status = done`,
  substring search.
- `TestTranscribeRealModel` — **real** invocation of `sherpa-onnx.exe` against the
  installed 14M Chinese Zipformer model and its bundled `test_wavs/0.wav`;
  asserts non-empty segments with sane timestamps. Skips (not fakes) when the
  binary or model is absent.

## Not implemented

Live, in-recording streaming recognition. `asr.LiveEvent` is declared but never
emitted; `capture.Recorder` only transcribes the finished file. See ROADMAP.
