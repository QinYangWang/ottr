# sherpa-onnx cgo implementation plan

Status: **implemented** (phases 0–2; phase 3, native PCM decode, waits on
[native capture and media redesign](native-capture.md)). What was built is
described in [Transcription](transcription.md); this document is kept as the
record of what the migration set out to do. The differences worth knowing:

- The ABI is **v2.0.1**, and the shim compiles sherpa-onnx's own vendored
  `c-api.h` rather than declaring the structs by hand — a layout copied by eye
  was already wrong twice before the header was vendored. Releases are recorded
  by archive digest, not by tag, and `cmd/setup` installs the newest recorded
  one; v1.13.8 stays installable by name.
- Batch and live audio share **one** path: both drive a streaming recognizer
  (the model is a streaming zipformer) instead of using the offline API for
  files, so one configuration and one set of results.
- `ExtractWav` (FFmpeg) is still how audio is obtained; only the recognizer
  moved in process.

## Goals

- Call sherpa-onnx in-process through its C API and cgo.
- Remove `sherpa-onnx(.exe)`, `FindSherpaBin`, process creation, stdout/stderr
  JSON scraping and live-transcription temporary WAV files.
- Preserve offline transcription, model selection, jobs/cancellation, live
  subtitles and post-recording transcription.
- Replace chunk-restart live recognition with one continuous online recognizer.
- Keep recording and the rest of the application usable when the optional ASR
  native runtime or model is absent.
- Use no third-party executable. Model files and native libraries are still
  required and are distributed explicitly.

Audio extraction from arbitrary media is owned by
[Native capture and media redesign](native-capture.md). The cgo migration can
start before that work: the first batch implementation may read the existing
16 kHz mono WAV, then switch to native PCM decode without changing recognizer
code.

## Current constraints in the code

- `RunSherpa` starts a process and scans mixed human/JSON output.
- `Service.recognize` extracts a temporary WAV before recognition.
- `StartLive` and `prepare` both require `FindSherpaBin`.
- `liveChunker` cuts 3–5.5 second chunks, writes one WAV per chunk and starts a
  new recognizer for each, losing model state at every boundary.
- `LiveSubtitle.Final` exists but the UI currently treats every changed string
  as a new line, so native partial results cannot be wired directly without
  flooding the six-line live view.

## Target structure

```text
capture / native media decoder
              │ interleaved PCM
              ▼
     streaming 16 kHz mono f32 converter
              │
       ┌──────┴────────┐
       ▼               ▼
 offline recognizer  online recognizer
 (one batch job)     (one live recording)
       │               │ partial + final results
       ▼               ▼
 GroupTokens       live subtitle revision
       │               │
       ▼               ▼
 transcript DB     Recorder.State / LiveEvent
```

Keep the cgo wrapper package-private inside `internal/asr`. Do not expose C
types to `capture`, `jobs`, `library` or `ui/native`.

```text
internal/asr/
    engine.go              PCM-to-segments orchestration; no os/exec
    native_config.go       model paths -> pinned C configuration
    native_batch_cgo.go    offline recognizer owner
    native_online_cgo.go   online recognizer/stream owner
    native_loader_cgo.go   dlopen/LoadLibrary and function-table loading
    native_stub.go         !cgo: explicit ASR-unavailable error
    native/sherpa_shim.c   small C ABI loader/memory shim
    pcm16k.go              stateful mono conversion and resampling
    live.go                continuous stream, endpoint and revision handling
```

Pin the sherpa-onnx C API version (start with the currently documented
v1.12.17, upgrading only after a compatibility test). Generate configuration
against that exact header; do not copy function signatures from another release.

## Runtime loading and packaging

ASR is optional today. Hard-linking an optional `libsherpa-onnx-c-api` can make
the OS refuse to start Ottr before Go can display an error. Use a small own
loader shim instead:

- Windows: `LoadLibraryW` + `GetProcAddress`.
- macOS/Linux: `dlopen` + `dlsym`.
- Resolve the required C functions once into a native function table. Fail with
  one clear error naming the missing/incompatible library.
- Search only the application native-library directory and the configured
  `<userData>/bin`; do not search `PATH` for executables.
- Keep the loader loaded for the process lifetime. Recognizers and streams are
  still destroyed per owner.

Expected artifacts are the sherpa-onnx C API library, its required
sherpa/onnxruntime libraries and the model directory. Set macOS `@rpath` and
Linux `$ORIGIN` deliberately; on Windows place dependent DLLs beside the loaded
library or use an explicit private dependency directory. Record the tested
library list in `docs/models.md`.

`models.Manager` continues to find and validate models. Replace `FindSherpaBin`
with a native-runtime status check used by Settings and ASR startup. A missing
runtime is an honest ASR error, not an application startup failure.

## Native ownership and cgo rules

Each offline job owns one recognizer. Each live recording owns one online
recognizer and one stream. Do not add a global recognizer pool initially: model
loading is already measured as cheap, and a pool introduces model-switch and
concurrency ownership problems without a demonstrated need.

- All calls on one recognizer/stream are serialized by its single Go owner
  goroutine. No recognizer method runs concurrently with `Destroy`.
- Convert model paths to native strings with the pinned API's expected encoding
  and free them after create returns.
- C receives sample pointers only for the duration of a call documented to copy
  them. Otherwise allocate the sample buffer in C and copy PCM into it.
- Copy result text, tokens and timestamps into Go before destroying the native
  result. Never retain a C result pointer in `Segment` or `LiveSubtitle`.
- Native errors include the operation and model path, but not audio contents.
- `Close` is idempotent. It stops input, drains the owner, finishes pending
  native calls, then destroys stream before recognizer.
- A failed create cleans every object created before the failure.

A `CGO_ENABLED=0` build keeps non-ASR features and returns “native ASR is not
available in this build”; it never silently falls back to an executable.

## PCM input

Use one streaming converter for batch and live input:

- input: interleaved `float32` PCM with a declared rate/channel count;
- output: mono `float32` at 16 kHz, clipped to `[-1, 1]`;
- channel averaging and resampling retain fractional position across calls;
- empty and partial input buffers do not reset the resampler;
- `Finish` flushes only complete output samples and does not invent audio.

Batch decoders and capture may already provide 16 kHz mono. The converter passes
that through without another allocation where practical. Remove the live
float32-to-int16 WAV path after the online path works; the C API consumes float
samples directly.

## Batch transcription

The offline wrapper uses the pinned C offline recognizer/transducer
configuration for tokens, encoder, decoder, joiner, CPU provider and two threads.
Keep `GroupTokens` unchanged so token/timestamp segmentation and database output
remain comparable.

`Service.recognize` becomes:

1. Validate media, native runtime and model; mark `pending` as today.
2. Open native audio decode (or read the transitional 16 kHz WAV in Go).
3. Create the offline recognizer and feed PCM through the streaming converter.
4. Finish input, decode and copy text/tokens/timestamps into Go.
5. Group and save segments in the existing transaction; publish the same events.

There is no temporary WAV once native audio decode lands. Progress can advance
from actual compressed/decoded audio position instead of jumping from 15% to
90%; if the native decoder cannot report position, retain the existing coarse
progress rather than fabricating it.

Cancellation is cooperative at PCM/read/decode boundaries. If the pinned C call
cannot be interrupted, cancellation waits for that call to return before
destroying native state; the job must not free memory under an active decode.
Document the maximum observed cancellation latency in tests. Do not reintroduce
a process merely to make cancellation forceful.

## Continuous live recognition

Replace `liveChunker` with one online recognizer for the recording:

1. Create recognizer and stream when capture starts.
2. `Write` converts incoming PCM and queues it to the single native owner. The
   queue is bounded; capture never waits for inference.
3. Feed samples with their actual input rate after conversion, then run every
   available decode step.
4. Read the current result as a **partial revision**. On an endpoint, emit the
   final result and reset recognition state without losing the recording's
   absolute timeline.
5. `Close` marks input finished, performs the final decode, emits the last final
   result and waits for native cleanup.

If inference falls behind, drop only provisional live audio at a documented
threshold and report a live-quality warning; never drop audio sent to the media
writer. The post-recording pass remains authoritative.

### Partial/final UI protocol

Use the existing `Final` field and add a stable utterance/revision identity to
the in-memory live event. Do not write provisional results to SQLite.

- `Final=false`: update the current live line **in place**, including its text
  and end time.
- `Final=true`: freeze that line and make the next partial a new line.
- Keep the newest six finalized lines plus at most one current partial.
- Native timestamps are relative to the continuous stream; after an endpoint
  reset, add the stored stream-base offset rather than a chunk offset.
- `Recorder.State` carries the same current-line identity/finality so polling
  the native window cannot append duplicate revisions.

The existing final transcript still replaces provisional live text after Stop.

## Settings and cleanup

After no caller remains:

- Delete `RunSherpa`, `FindSherpaBin`, `proc_*.go`, WAV-only live helpers and
  process-based tests.
- Replace Settings' “sherpa-onnx binary found at …” with native runtime version
  and load status. Model selection and folders remain unchanged.
- Update `docs/transcription.md`, `docs/models.md` and `docs/development.md` to
  describe native build/library requirements.
- Do not retain an opt-in executable fallback; that would preserve the process,
  packaging and test matrix this migration is intended to remove.

## Implementation phases

### 0. Pin and prove the ABI

- Build a minimal loader against the pinned C header on all three OSes.
- Load one known model, recognize its bundled test WAV in-process and destroy
  all objects.
- Verify Windows Unicode model paths and packaged dependent-library lookup.

Exit: the same non-empty text/timestamps on macOS, Windows and Linux, with no
sherpa process created.

### 1. Batch cgo migration

- Implement native ownership, offline recognition and transitional Go WAV read.
- Route `Service.recognize` through it; keep GroupTokens, DB and jobs unchanged.
- Replace Settings binary discovery with runtime status.

Exit: real-model transcription and cancellation pass without `sherpa-onnx.exe`.

### 2. Continuous online recognition

- Add the stateful PCM converter and online recognizer.
- Implement partial revisions, endpoints, final flush and absolute timestamps.
- Update Recorder/UI polling to revise rather than append partials.

Exit: a recording containing speech across the old 3/5.5-second boundaries keeps
its words, produces no temporary WAV and starts no per-chunk process.

### 3. Native PCM decode and cleanup

- Connect `internal/media` native decode directly to batch ASR.
- Remove temporary WAV extraction and all executable lookup/process code.
- Complete packaging, documentation and clean-machine tests.

Exit: recording → native decode → in-process ASR → transcript works on a machine
with neither FFmpeg nor a sherpa executable.

## Tests and acceptance

- Unit: mono conversion, arbitrary buffer splits, resampler continuity, clipping,
  partial/final revision handling and endpoint timestamp offsets.
- Native real-model: non-empty result, ordered non-negative timestamps, empty
  audio, very short audio and a long model test WAV.
- Lifecycle: 100 create/feed/close cycles and repeated model switches without
  goroutine or native-memory growth; no callback/result use after Close.
- Cancellation: during input and native decode; no use-after-free and correct
  `transcript_status` restoration.
- Concurrency: two media jobs cannot share or destroy each other's recognizer.
- UI: partial updates do not add lines; final lines remain stable at six plus
  one partial; post-recording text replaces live text as before.
- Packaging: missing native runtime leaves recording/import/UI usable and shows
  an actionable ASR error; missing model behavior is unchanged.
- Final scan has no `FindSherpaBin`, `exec.Command` or `sherpa-onnx.exe` caller in
  production ASR code. Tests use real models/libraries, never a fake transcript.

## Out of scope

A custom ONNX runtime, model format conversion, GPU provider selection, model
downloads, language identification and recognition-quality/model changes. Those
are independent of replacing the process boundary.
