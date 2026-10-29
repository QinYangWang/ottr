# In-window native playback implementation plan

Status: **proposal, not implemented**. This defines the replacement for opening
finished media in the system player. It depends on native demux/decode from
[Native capture and media redesign](native-capture.md), not on FFmpeg, a webview,
an embedded browser or a third-party player executable.

The interaction target is the
[three-window HTML prototype](prototypes/native-ui.html): compact recorder,
draggable recording strip, a card-grid Library with an item page, and separate
Settings. The player below belongs on that Library item page, not in a
separate recording-details window.

## Scope

First release supports play, pause, seek, volume, mute, elapsed/duration display
and transcript/chapter timestamp seeking inside the existing Library window.
Ottr's own H.264/AAC fragmented MP4 recordings are the required format. PNG
screenshots use a static image viewer, not a fake one-frame video player.

Imports work only when their container/codec is in the published native decoder
matrix. Unsupported media retains an explicit error and “Show file”; it must not
fall back to FFmpeg. MKV/WebM and arbitrary codec compatibility are separate
demux/decoder work, not a promise of the player UI.

No playlists, playback-speed control, editing timeline, filters, subtitles burn-
in, casting, streaming, DRM or fullscreen mode in the first implementation.

## Current integration points

- `sessionHead` currently exposes `Play` through `mygo.Shell.OpenPath`.
- `sessionPage` consists of the header and Transcript/Translate/Summary tabs;
  there is no finished-media surface.
- `ui.Bitmap` already displays and updates decoded preview frames in
  `grabPreview`, so a custom MyGo video element is not a prerequisite.
- The Library card grid needs one decoded frame per card as a thumbnail. Until
  a thumbnail cache exists, cards draw a source glyph, and the first `Ready`
  frame of a player opened for that media fills its card.
- `App.update` marshals state changes to the UI thread. Decoder/audio threads
  must never touch `ui.Bitmap` or view state directly.
- Selecting, closing or deleting media currently changes only Library state;
  these actions must also own player shutdown so Windows file handles are not
  left open.

## Boundaries

```text
file path
   │
   ▼
media.Decoder ─── encoded/demux/decode, native per OS
   │ bounded VideoFrame queue       bounded PCM queue
   ▼                                ▼
player.Session                  audio.Output
   │ latest display frame             │ actual render clock
   │                                  │
   └──────────── audio/video clock ───┘
                   │
                   ▼ (UI thread only)
          ui/native player pane + ui.Bitmap
```

Three packages have separate responsibilities:

- `internal/media`: native demux/decode and metadata. No player or UI state.
- `internal/player`: transport, queues, seeking, A/V sync and audio output.
- `ui/native`: controls, bitmap presentation and mapping the selected media to
  one player session.

Do not import `internal/capture` from the player. If capture and playback share
a raw-frame struct, move that neutral type to `internal/media` rather than
creating a package cycle.

## Contracts

Keep the public surface small and make every session independently closable:

```go
type Session struct { /* one decoder, one output, one clock */ }

func Open(ctx context.Context, path string) (*Session, error)
func (s *Session) Play() error
func (s *Session) Pause() error
func (s *Session) Seek(ctx context.Context, position time.Duration) error
func (s *Session) SetVolume(float32) // 0..1
func (s *Session) SetMuted(bool)
func (s *Session) State() State
func (s *Session) LatestFrame() *media.VideoFrame // ownership defined once
func (s *Session) Close() error                  // idempotent
```

`State` contains loading/ready/playing/paused/ended/error, position, duration,
volume/mute and the terminal error. Playback is not a persisted `jobs.Job`: it
has no durable result and dies with its Library selection/window.

`media.Decoder` owns native reader state and produces Go-owned BGRA video frames
with PTS plus PCM buffers with PTS. `Seek` jumps to a preceding keyframe and the
decoder discards samples before the requested position. Native APIs handle H.264
decode order and codec priming; player code does not parse bitstreams.

Audio output is native and in-process:

| Platform | Decode | Audio output |
|---|---|---|
| macOS | AVAssetReader / native codec APIs | AudioUnit or AVAudioEngine |
| Windows | Media Foundation Source Reader | WASAPI shared mode |
| Linux | Native/shared demuxers and codec libraries | PipeWire or PulseAudio |

The same media decoder supplies ASR PCM, avoiding a second decode stack. A build
without cgo may display metadata/transcripts but reports playback unavailable.

## Threading, ownership and backpressure

Each session has one decoder owner, one audio-output owner and one clock owner.
Their queues are bounded:

- at most three full-size decoded video frames;
- roughly 200 ms of decoded audio;
- one pending UI frame, latest-frame-wins.

Never decode the entire file into memory. When queues are full, decoding pauses.
When paused, the player may keep one poster/next frame but does not continue
unbounded read-ahead. When the UI is slow, video frames are dropped before the
UI queue, not on the audio thread.

Native frame/sample memory is released or copied before the native read buffer
is reused. UI receives Go-owned pixels only. `Close` stops transport, waits for
native reads/callbacks, closes output, drains references and closes the decoder;
it is safe after partial `Open` and safe to call twice.

`App.update` must not be called once per decoded frame without coalescing. Keep
one scheduled UI notification and replace its pending frame with the newest.
Bitmap creation/update and all transport state reads used by the view happen on
the MyGo UI thread.

## Audio/video clock

Use **audio render time as the master clock** when an audio stream exists.
Samples written to an output buffer are not necessarily audible yet, so position
comes from the backend's rendered-frame/padding/delay information, not merely
from bytes queued.

- Display the newest video frame whose PTS is at or before the master clock.
- Keep the current frame when the next one is early; drop frames that are late.
- Start video and audio from the same timeline origin, including nonzero track
  start and encoder priming offsets.
- On audio underrun, output silence and re-anchor using the backend clock; do
  not let video free-run indefinitely away from audio.
- Video-only media uses a monotonic clock started at the first frame PTS.
- Pause freezes the timeline and audio render position. Resume does not include
  time spent paused.
- Ended is reached only after the final decoded audio has actually rendered, not
  merely when the demuxer returns EOF.

Target no accumulating A/V drift and at most 50 ms discrepancy in the ten-minute
acceptance recording. Video cadence follows source PTS; do not assume every file
is 30 fps.

## Seek semantics

Seek is one serialized operation on the session:

1. Pause transport and invalidate pending UI/audio frames with a new generation
   ID.
2. Stop/flush audio output and both decode queues.
3. Seek native decode to the keyframe before the target.
4. Decode forward, discarding audio/video before the exact target.
5. Reset the master clock and decoder/audio generation, show the target frame,
   then resume only if the session was playing before the seek.

Late buffers from an older generation are discarded. Repeated slider input may
coalesce to the newest target, but Close/Seek/Play must never race native
destruction. Clamp position to `[0, duration]`; an unknown duration disables
free seeking rather than displaying a false range.

## NativeUI layout

Extend `sessionPage`, between `sessionHead` and the existing tabs:

```text
name / metadata / Show file / Delete / Close
┌──────────────────────────────────────┐
│ paused poster or current video frame │
└──────────────────────────────────────┘
[Play/Pause]  0:12 / 3:41  [seek]  [volume] [Mute]
Transcript (original + translation) | Summary
```

- The player surface is hidden for audio-only media and replaced by the static
  image for screenshots. It does not consume the whole transcript area.
- `ui.Image(...).Fit(ui.Contain)` letterboxes without a Go resize on every frame.
  Reuse `ui.Bitmap` while dimensions are unchanged.
- Open loads metadata and a paused poster frame; it does not autoplay.
- Play/Pause and seek are accessible named controls. Position text updates about
  four times a second, independently of video-frame cadence.
- Transcript and translation share one list: each timestamped row shows the
  original with its translation directly below. Language selection lives in
  the Transcript toolbar; there is no separate Translate tab.
- Transcript segment timestamps and Summary chapter timestamps call
  `Seek(position)` and show that media's current frame; they do not open an
  external player.
- Starting a recording pauses/closes playback so capture and playback do not
  compete for audio/render resources.
- Switching media, closing details, deleting media, closing Library and app
  shutdown all call `Close` before releasing/deleting the file.

Keep “Show file”. An “Open externally” action can remain as an explicit user
choice, but in-window playback must not invoke it implicitly.

Suggested additions:

```text
internal/player/
    session.go            transport owner and state
    clock.go              audio/video clock and late-frame policy
    seek.go               generation-based flush/seek
    audio_windows.go      WASAPI render client
    audio_darwin.go       AudioUnit/AVAudioEngine output
    audio_linux.go        PipeWire/PulseAudio output

ui/native/
    player.go             surface, transport and bitmap presentation
```

## Implementation phases

### 0. Decoder and raw-bitmap prerequisite

- Define the neutral media frame/PCM and decoder contracts.
- Produce one paused BGRA poster frame from a native recording on Windows.
- Display it through `ui.Bitmap` without JPEG or a subprocess; measure frame
  upload/update cost before committing to 30/60 fps UI playback.

Exit: correct colors, orientation, physical dimensions and clean decoder Close.

### 1. Video-only vertical slice

- Implement session lifecycle, bounded video queue and monotonic clock.
- Add the player surface, Play/Pause, seek and media-switch cleanup.
- Use fake decoders for transport tests and native H.264 video for integration.

Exit: 1080p30 video-only recording plays, pauses and seeks in Library with
bounded memory and no external process.

### 2. Audio and A/V synchronization

- Add WASAPI render output, PCM queue and the audio master clock.
- Handle mute/volume, underrun, EOF rendering and seek flush.
- Connect Ottr's H.264/AAC recordings and audio-only files.

Exit: ten-minute recording has no accumulating drift, and Pause/Seek/Ended do
not strand queued audio or restart old generations.

### 3. macOS and Linux

- Implement AVAssetReader/AudioUnit and Linux decoder/audio-output adapters.
- Reuse the same session, clock and UI; only native decoder/output differs.
- Publish and enforce the actual container/codec support matrix.

Exit: the native acceptance suite passes on all three desktop platforms.

### 4. Transcript integration and removal of implicit external playback

- Make transcript and chapter timestamps seek the session.
- Complete screenshot/audio-only layouts and unsupported-codec errors.
- Remove or demote the external Play button only after in-window behavior is at
  parity for supported media.

Exit: recording selection, playback, timestamp navigation, deletion and window
close work without a helper media process.

## Tests and acceptance

- Unit/fake decoder: Play/Pause/Resume, early/late frames, EOF, audio-only,
  video-only, seek clamp, seek while paused and repeated seek generation races.
- PCM/clock: output delay, underrun, nonzero start PTS, codec priming and pause
  intervals never move the displayed timeline incorrectly.
- UI: poster appears paused, controls have accessible names, selecting another
  row replaces the player and timestamp clicks seek the intended media.
- Lifecycle: 100 open/play/seek/close cycles, repeated Library close/reopen and
  media deletion with no goroutine/native-memory growth or locked file.
- Backpressure: a blocked UI drops video frames but does not block audio; paused
  playback does not grow decoded queues.
- Real native playback: 1080p30 ten-minute H.264/AAC file, bounded memory and no
  accumulating drift; 4K support is reported from measurement, not promised.
- Crash/error paths: missing file, malformed MP4, unsupported codec, audio-device
  unplug and decode failure leave the rest of Library usable.
- Production player contains no `os/exec`, browser/webview, FFmpeg or external
  player call. Real audio/desktop tests remain opt-in where CI has no device.

## Deliberately deferred

A custom MyGo GPU video element, zero-copy textures, arbitrary-format playback,
playback speed, fullscreen, frame stepping, captions over video and advanced
audio effects. Add a raw texture path only if profiling shows `ui.Bitmap`
cannot meet the 1080p30 target; do not make it a prerequisite.
