# Native capture and media redesign

Status: **in progress**. Phase 0 (native ABI proof) and the capture half of
phase 1 (common frame pipeline, preview, screenshots) are implemented on
Windows. This supersedes the capture direction in `ROADMAP.md` items 5, 6 and
12.

What exists now, and what each remaining phase still needs, is at the end of
this document under [Status](#status).

## Goal and hard constraints

Record displays/windows and take screenshots on macOS, Windows and Linux using
native capture APIs through cgo.

The target has **no FFmpeg or ffprobe dependency**: no executable is searched,
configured, spawned or bundled, and no `libav*` library is used. Capture,
preview, encoding, MP4 muxing, screenshots, media metadata and ASR audio decode
must all work in a clean environment without FFmpeg.

Keep the current product behavior: one recording at a time, live preview,
optional system/microphone audio, live and post-recording transcription,
fragmented MP4 recovery, Library integration and explicit permission errors.
Area capture, camera capture, editing and DRM/protected-content capture are not
part of this redesign.

## Current seams and what must change

Today `resolveSource` returns FFmpeg input arguments. `Recorder` and `Previewer`
then each start FFmpeg; WASAPI writes audio into named pipes consumed by it.
That makes FFmpeg the capture, conversion, encoding, preview and muxing layer,
so changing `resolveSource` alone cannot remove it.

The replacement has three independent boundaries:

1. **Capture backend**: sources, permissions, native streams and one-shot frames.
2. **Media writer**: audio conversion/mixing, native encoding and shared MP4
   muxing. It knows nothing about windows, displays or the UI.
3. **Products**: `Recorder`, `Previewer` and `Screenshotter`, which keep the UI
   contract and orchestrate capture, media and Library services.

`Library.AddRecording` currently probes the finished file. Native recording
already knows its metadata, so it will use a metadata-based Library method
instead. Imports and post-recording ASR still need their ffprobe/FFmpeg paths
replaced before the dependency can be removed repository-wide.

## Target architecture

```text
ui/native
   ├── Recorder ───────┐
   ├── Previewer ──────┼── one capture.Backend per process
   └── Screenshotter ──┘             │
                                     ▼
                    ┌─────────────────────────────────┐
                    │ macOS: ScreenCaptureKit         │
                    │ Windows: Windows.Graphics.Capture│
                    │ Wayland: XDG portal + PipeWire  │
                    │ X11: XCB + XShm                 │
                    └─────────────────────────────────┘
                                     │ Go-owned BGRA frames + monotonic PTS
                       ┌─────────────┴──────────────┐
                       ▼                            ▼
                preview / PNG                 media.Writer
                (Go image packages)                 │
                                     ┌──────────────┼──────────────┐
                                     ▼              ▼              ▼
                               native H.264    native AAC     shared fragmented
                                  encoder         encoder         MP4 writer
                                     └──────────────┴──────────────┘
                                                    │
                                                    ▼
                                             recording.mp4
```

FFmpeg's MJPEG-over-TCP preview disappears. The capture session keeps the
newest raw frame; a 5 fps preview worker scales it and encodes JPEG with Go's
`image` packages. Recording and idle preview use the same mechanism, and the UI
continues to receive a JPEG bitmap. No helper process or localhost socket is
needed.

One backend instance is constructed in `cmd/studio/main.go` and shared by the
three products. This is required on Wayland, where source selection, consent,
the portal session and its restore token belong together rather than to an
individual recording.

## Go contracts

The exact fields can follow existing naming, but the boundaries should remain
this small:

```go
type Backend interface {
    Sources(context.Context) ([]Source, error)
    Open(context.Context, StreamConfig) (Stream, error)
    Snapshot(context.Context, SnapshotConfig) (Frame, error)
    Close() error
}

type Stream interface {
    Next(context.Context) (Frame, error) // io.EOF on a normal end
    Close() error                       // idempotent; callbacks quiesced on return
}

type Frame struct {
    Data          []byte       // tightly packed, Go-owned BGRA8888
    Width, Height int          // physical pixels
    PTS           time.Duration // native monotonic timestamp
}
```

`StreamConfig` carries the source ID, requested fps, cursor policy and recording
canvas. `Next` hides each platform's callback thread behind one bounded queue.
A video queue holds at most two frames and drops the oldest when the consumer is
behind; native callback threads never wait for encoding or the UI. Sustained
audio overflow is an error instead of silently punching holes in the soundtrack.

Errors retain their cause and classify at least permission denied, user
cancelled, source disappeared, unsupported and encoder unavailable. The UI maps
those to OS-specific instructions; it never retries a denial in a loop.

### cgo boundary

Every platform exports a small C ABI, not C++/Objective-C types:

- Go registers callbacks with a `runtime/cgo.Handle`; no Go pointer is retained
  by native code.
- Native code supplies data, dimensions, stride and timestamp. The bridge
  removes row padding and the Go callback makes one Go-owned copy.
- Callbacks only enqueue data. They never call UI, encoder or Library code.
- Native objects and pixel buffers are released on the originating thread.
- `Close` stops the native stream, waits for callbacks to finish, and only then
  releases the cgo handle. It is safe after partial startup and safe to call
  twice.
- The common contract is tightly packed BGRA. Format conversion, readback and
  timestamp extraction stay in the platform bridge.

This costs one CPU copy per delivered frame but avoids unsafe Go/C ownership
rules and keeps all native encoders behind the same testable contract.

## Platform backends

Proposed baselines are macOS 14+, Windows 10 1903+, and current Wayland or X11
Linux desktops. macOS 14 provides both `SCStream` and `SCScreenshotManager`; do
not add a second legacy screenshot path for older systems unless supporting
them is an explicit requirement.

| Platform | Source and stream | Screenshot | H.264 / AAC |
|---|---|---|---|
| macOS | `SCShareableContent`, `SCContentFilter`, `SCStream` | `SCScreenshotManager`, falling back to the first stream frame only where needed | VideoToolbox / AudioToolbox |
| Windows | `GraphicsCaptureItem`, `Direct3D11CaptureFramePool`, D3D11 readback | Same WGC path, one frame | Media Foundation `IMFTransform` encoders |
| Wayland | XDG ScreenCast portal + PipeWire | First frame from the authorized PipeWire stream | OpenH264 / FDK-AAC |
| X11 | XCB + XShm | Same X11 stream, one frame | OpenH264 / FDK-AAC |

### macOS

An Objective-C cgo bridge links ScreenCaptureKit, CoreMedia, CoreVideo,
VideoToolbox and AudioToolbox. Display/window IDs come from
`SCShareableContent`, not window titles. Pixel buffers use physical pixels and
BGRA; sample-buffer presentation timestamps become monotonic nanoseconds.

Preflight Screen Recording and Microphone consent where the API permits, then
let the OS make the request. A denial explains the relevant System Settings
page. ScreenCaptureKit's system audio/microphone support can feed the common
PCM path on supported OS versions; microphone capture otherwise remains a
separate AVFoundation audio backend rather than blocking video-only recording.

### Windows

A C++ bridge uses WGC `CreateForMonitor`/`CreateForWindow`, so window capture no
longer crops the desktop and an occluded window can be captured directly. IDs
contain native monitor/window identity, not a title or display index. The old
DDA/GDI implementation remains until WGC reaches recording and preview parity,
then is deleted rather than kept as another permanent backend.

WASAPI remains the audio source, but its destination changes from a named pipe
to the common PCM queue. Media Foundation encoder MFTs replace FFmpeg encoder
probing; startup selects an encoder that actually accepts a test frame and
falls back to the software MFT.

C++/WinRT should be built with MSVC as a small C-ABI DLL, with Go's cgo toolchain
linking only that ABI. This avoids assuming MinGW can compile Microsoft's C++
WinRT projections. The DLL and import library are build/package artifacts, not
external capture programs.

### Linux

Wayland capture must use `org.freedesktop.portal.ScreenCast` and PipeWire;
there is no supported direct Wayland desktop-read API. The portal owns source
selection and consent, so `Sources` exposes a logical “Screen or window…”
picker entry rather than pretending to enumerate windows. `Open`/`Snapshot`
performs selection when necessary and reuses the authorized session. A portal
restore token is treated as a credential: never log it, and invalidate it when
the portal rejects it.

PipeWire negotiates BGRA where possible and copies mapped buffers while
respecting stride. dmabuf is an optimization only after the memfd path works.
X11 uses XCB/XShm and is selected at runtime from the actual session, not by
displaying a Wayland error on every Linux machine.

Linux has no system equivalent of AVAssetWriter or Media Foundation Sink
Writer. Use OpenH264 and FDK-AAC through the same cgo encoder boundary, with
libyuv for BGRA-to-I420 conversion if profiling shows the Go conversion is not
fast enough. These are linked libraries; none introduces a media executable.

## Encoding, timestamps and MP4

Do not use `AVAssetWriter` or `IMFSinkWriter` as the common abstraction: they
hide encoded samples and make the existing fragmented-MP4 recovery policy hard
to preserve. Native APIs produce encoded H.264 video and AAC audio access units;
a shared Go muxer writes the container.

Use a pure-Go MP4 library (`mp4ff` after an initial feasibility test) for box
parsing/writing rather than hand-maintaining a second container implementation.
The muxer owns:

- H.264 SPS/PPS to `avcC`, AAC config to the audio sample entry;
- DTS/PTS ordering, keyframe flags and track timescales;
- an initialization segment and one `moof`/`mdat` fragment per keyframe group;
- final duration and clean finalization, with the last complete fragment still
  playable after a crash.

Recording canvas dimensions are fixed after the first valid frame. A resized
window is aspect-fit into that canvas; raw screenshots keep their current native
size. This prevents mid-file codec/configuration changes. Odd dimensions are
handled by the canvas/conversion path rather than passed to H.264 accidentally.

All video and audio use a common monotonic epoch captured at session start.
The recorder passes native PTS through to the encoders/muxer; it does not infer
timing from a ticker. PCM is normalized to 48 kHz interleaved `float32`, mixed
before AAC encoding, and also offered to the existing non-blocking live-ASR
sink. Initial audio/video offsets and encoder priming are explicitly included
in track edit/timestamp calculations.

`media.Writer` exposes only `WriteVideo`, `WritePCM`, `Close` and `Abort`, and
returns `media.Info` on success. `Recorder` inserts that metadata directly into
Library. It does not probe its own output.

## Lifecycle

### Start

1. Lock the single-recording slot and resolve the native source.
2. Open capture and wait for the first valid frame; fail atomically on denial,
   cancellation or a black/invalid initial configuration.
3. Create the native writer and start requested audio streams. Any failure
   closes everything already opened.
4. Start video/audio pump goroutines and publish `recording:state`.
5. A stream, encoder, queue or file error stops the session through one owner,
   aborts the writer and preserves the existing failed-recording semantics.

### Stop

1. Prevent new frames, close native capture, then drain writer input queues.
2. Close/finalize video and audio encoders and the MP4 writer with a deadline.
3. On success, verify non-empty output and add the returned metadata to Library.
4. On failure, call `Abort`; do not report a partial file as successful.
5. Restart idle preview and run post-recording ASR as today.

There is no subprocess watchdog and no `q` command. The session owner, not an
`exec.Cmd.Wait` goroutine, owns cancellation and finalization.

### Screenshot

`Screenshotter` uses the currently selected source, obtains one native frame,
converts BGRA to `image.RGBA`, and writes PNG through a temporary file plus
atomic rename. Add `png` and `screenshot` to Library support; screenshots do not
start ASR. Clipboard export, annotations and region selection are out of scope.

## File layout after migration

```text
internal/capture/
    backend.go             common Source/Frame/Backend contracts and errors
    recorder.go            session ownership; no platform API or exec calls
    livepreview.go         raw-frame preview, no TCP or subprocess
    screenshot.go          one-shot PNG and Library integration
    backend_darwin.go      cgo entry points
    backend_windows.go
    backend_linux.go       runtime Wayland/X11 selection
    native/darwin/         Objective-C bridge
    native/windows/        C++/WinRT + Media Foundation bridge
    native/linux/          portal/PipeWire and XCB/XShm bridges

internal/media/
    writer.go              common writer contract and encoded-sample model
    writer_darwin.go       VideoToolbox / AudioToolbox
    writer_windows.go      Media Foundation MFTs
    writer_linux.go        OpenH264 / FDK-AAC
    mp4.go                 shared fragmented MP4 muxer
    pcm.go                 resampling, mixing and clock tests
    probe.go               native metadata, no ffprobe
    decode_*.go            PCM decode for ASR imports/recordings
```

Remove `args.go`, `dda_windows.go`, `encoder_*.go`, TCP `preview.go` and the
capture-specific process wrappers once no caller remains. Do not leave a second
FFmpeg recording path behind a build tag.

## Implementation sequence

Each phase ends with a usable vertical slice; no phase requires FFmpeg.

### 0. Lock behavior and prove the native ABI

- Add fake-backend tests around current Recorder/Previewer state transitions.
- Add characterization tests for source disappearance, repeated Start/Stop,
  cancellation and Library insertion.
- Build tiny cgo probes for WGC/D3D11 readback, one ScreenCaptureKit frame and
  one PipeWire frame. In particular, prove the MSVC-DLL/cgo Windows build and
  packaging before writing the full bridge.

Exit: one BGRA frame with correct pixels, dimensions and PTS on each OS/toolchain.

### 1. Common frame pipeline, preview and screenshots

- Introduce `Backend`, `Stream`, `Frame`, typed errors and the shared backend in
  the composition root.
- Replace MJPEG/TCP with bounded raw frames and Go JPEG preview.
- Implement one-frame capture and atomic PNG output, then add the Screenshot UI
  and Library metadata path.
- Test color conversion, stride, Retina/HiDPI, odd sizes and latest-frame-only
  backpressure.

Exit: native preview and screenshots without FFmpeg; old recording remains
temporarily available only until the next phase replaces it.

### 2. Encoders, PCM and fragmented MP4

- Implement the shared encoded-sample/muxer contract and native encoder
  adapters.
- Move WASAPI off named pipes into the PCM queue; add 48 kHz conversion/mixing.
- Feed generated BGRA frames and PCM through each writer. Validate MP4 boxes
  with the Go parser and playback with each OS's native player; no FFmpeg test
  helpers.
- Test keyframe recovery by terminating a synthetic recording and parsing every
  complete fragment.

Exit: repeatable H.264/AAC fragmented MP4 on all platforms, including video-only.

### 3. Windows recording vertical slice

- Connect WGC, WASAPI, MF encoders and the muxer through `Recorder`.
- Replace old capture tests with real native display/window recording and
  screenshots. Include covered windows, resizing, minimizing, device unplug,
  secure-desktop/session loss and unexpected source closure.
- Remove the FFmpeg/DDA/GDI recording implementation after parity.

Exit: five-minute 1080p30 recording with system audio + microphone, bounded
memory, no accumulating A/V drift, and normal Stop/failure/Library behavior.

### 4. macOS backend

- Implement shareable-content enumeration, permissions, display/window streams,
  native audio and VideoToolbox/AudioToolbox adapters.
- Verify Retina dimensions, display changes, window resize/close, Screen
  Recording denial, microphone denial and app sleep/wake.

Exit: the same recording and screenshot acceptance suite passes on macOS.

### 5. Linux Wayland, then X11

- Implement portal consent/session/restore-token handling and PipeWire streams.
- Add XCB/XShm only after Wayland works; share encoders, PCM and muxing.
- Test portal cancel/deny, token expiry, stream reconnect, compositor restart,
  multi-monitor selection and X11 source close.

Exit: recording and screenshot work on Wayland and X11 with clear unsupported /
permission errors rather than fallback executables.

### 6. Remove the remaining ffprobe/FFmpeg users

- Read native recording metadata directly; parse PNG/WAV and MP4/MOV metadata in
  `internal/media`.
- Decode recorded AAC to 16 kHz mono PCM using platform readers or the shared
  MP4/FDK-AAC path for ASR. Keep temporary PCM only if profiling shows a second
  decode is measurably costly.
- Replace import decoding with native readers. Publish an explicit supported
  format matrix; do not silently claim existing MKV/WebM/codec support without
  new demuxers/decoders.
- Delete FFmpeg Settings controls, `ffmpeg_path`, `internal/tools` media lookup,
  old docs/tests and all production `exec` media calls once unused.

Exit: `rg -n 'ffmpeg|ffprobe' --glob '!docs/**' .` has no production callers,
and a clean machine/container with no FFmpeg can build, record, screenshot,
import supported media and transcribe.

## Acceptance and non-negotiable tests

- Correct BGRA/RGB channel order, alpha handling, stride and physical-pixel
  dimensions on every backend.
- Repeated 100× open/close and preview/recording switches with no callback after
  close, goroutine growth or native memory growth.
- 4K frame queues remain bounded; preview latency does not affect recording.
- Audio/video drift stays below 50 ms over a five-minute recording, including a
  silent system-audio stream and a 44.1 kHz microphone.
- Source close, permission denial, encoder failure and disk-full all finalize or
  abort according to one tested policy; none reports a false success.
- Fragmented output remains readable after process termination.
- All live desktop tests remain behind `OTTR_LIVE=1`; unit tests use synthetic
  frames and fake backends and require no desktop or media executable.

## Status

| Phase | State |
|---|---|
| 0. Lock behavior, prove the native ABI | **done on Windows.** WGC + D3D11 staging readback delivers BGRA physical pixels with monotonic PTS; built by mingw cgo, no MSVC DLL needed. |
| 1. Common frame pipeline, preview, screenshots | **done on Windows.** `capture.Backend`/`Stream`/`Frame`, typed errors, bounded queue, native JPEG preview (no TCP, no subprocess), PNG screenshots with atomic rename, `Library.AddScreenshot`, a Screenshot button in the recording form. |
| 2. Encoders, PCM, fragmented MP4 | **not started.** Recording still hands raw BGRA to FFmpeg over a named pipe; no `internal/media` yet. |

### What phase 1 changed

- `internal/capture/backend.go` holds the contracts: `Backend`, `Stream`,
  `Frame`, `StreamConfig`, `SnapshotConfig`, the typed errors, the two-frame
  queue and the BGRA helpers.
- `backend_windows.go` + `wgc_windows.c`/`wgc.h` are the Windows backend. The C
  bridge keeps every C++/WinRT object on its own side and only passes
  `(data, width, height, stride, pts)` to one Go callback, which copies. The
  pool is free-threaded, so there is no dispatcher queue or message loop. The
  capture border is turned off where Windows allows it.
- Sources carry platform identity: `screen`, `display:<HMONITOR>` and
  `window:<HWND>`, both hex. A window is the window itself, so it can be
  covered and still be recorded correctly.
- `wgc.c` polls `ottr_wgc_alive` so that a closed window ends its stream instead
  of hanging the recorder.
- `livepreview.go` replaces the MJPEG/TCP preview: the newest raw frame is
  encoded to a 960-wide JPEG in process, at most five times a second.
- `screenshot.go` writes PNG through a temporary file plus rename.
- `Library.AddScreenshot`/`AddNativeRecording` insert metadata instead of
  probing.
- The old DDA/GDI `resolveSource`, `mpjpeg` `previewHub` and their tests are
  deleted rather than kept behind a build tag.

Still FFmpeg, until phase 2: the encoder. `Recorder` opens a native stream,
takes the first frame to fix the recording canvas (even dimensions; a later
resize is letterboxed into it), then repeats the newest frame at 30 fps into a
named pipe FFmpeg reads as raw BGRA. That pacing is by ticker, not by native
PTS, which is the drift the native muxer removes.

## Deliberately excluded

No FFmpeg fallback, native UI video player, area selection, camera, streaming,
DRM bypass, custom shader compositor, GPU zero-copy requirement, or handwritten
general-purpose demuxer in the first implementation. Zero-copy dmabuf/D3D11
sharing is worthwhile only after measurements show the single CPU readback/copy
is the bottleneck.
