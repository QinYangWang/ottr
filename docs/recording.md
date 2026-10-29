# Recording

Windows-only in the current implementation. Other platforms report
`capture.ErrUnsupported` from the backend.

Capture is native now — Windows Graphics Capture through cgo, no `gdigrab`, no
`ddagrab`, no MJPEG preview socket. Encoding and muxing are still FFmpeg until
`internal/media` lands; see [Native capture and media redesign](native-capture.md)
for the target and its status.

## Why this design

- **Video** comes from Windows Graphics Capture, which records a display or a
  window as Windows composites it. A small C ABI over cgo hands Go tightly
  packed BGRA rows; no C++ or WinRT type crosses the boundary, and no helper
  program runs.
- **Audio** is captured in-process with WASAPI through pure-Go COM bindings
  (`moutend/go-wca`), then written as raw `f32le` PCM into a Windows named pipe
  that the encoder reads as an input.
- **Encoding, resampling, mixing, and muxing** are still done by FFmpeg, as
  phase 2 of [Native capture and media redesign](native-capture.md) replaces it
  with native encoders and a shared MP4 writer.

Windows loopback capture through FFmpeg's `dshow` would require a separate
virtual-audio device driver, which is not acceptable as a product dependency;
that is why audio has always been captured in Go. Video used to be `gdigrab`
and then `ddagrab` through FFmpeg, and no longer is.

## Pipeline

```
Windows.Graphics.Capture ──BGRA──┬─► preview (Go JPEG, 5 fps) ─► window bitmap
  (display or window)            └─► newest frame repeated at 30 fps
                                          │ named pipe (raw BGRA)
                                          ▼
WASAPI loopback ──pipe──► ffmpeg ─► .mp4 (h264_nvenc/qsv/amf, else libx264, + aac)
WASAPI microphone ─pipe──┘
WASAPI (any) ───────────► liveFeed ─► asr live sink ─► live subtitles
```

## Sources

`Recorder.Sources()` asks the capture backend. On Windows it enumerates three
kinds, each with the platform's own identity:

| Kind | ID format | How it is captured |
|---|---|---|
| `screen` | `screen` | WGC `CreateForMonitor` on the primary display |
| `display` | `display:<HMONITOR hex>` | WGC `CreateForMonitor` |
| `window` | `window:<HWND hex>` | WGC `CreateForWindow` |

A window is captured by Windows itself, not cropped out of the desktop, so a
covered window is still recorded correctly and resizing the window does not move
the capture. A window that closes ends its stream: the backend polls whether the
handle still names a visible window with a title.

The full screen is the bounding box of every display; per-display sources are
listed alongside it, not instead of it. `Detail` carries the size in physical
pixels. The pointer is drawn in recordings and screenshots, and left out of the
idle preview.

The backend releases every native object on the thread that made it, and
`Stream.Close` waits for a callback already in flight before it returns, so no
frame arrives after close and the cgo handle can be freed with it.

## Preview before recording

`capture.Previewer` opens the same source at `previewFPS` (5) and keeps only the
newest raw frame; a goroutine in the process scales it to 960 pixels wide and
encodes it as JPEG, at most five times a second. There is no socket, no MJPEG
parser and no second process: the first frame appears in about 180 ms, and a
slower window only causes frames to be skipped.

The idle preview asks for no pointer — the display's own pointer already sits
over the picture, and a second one would lag behind it. Recordings and
screenshots take the pointer.

Library's recording page shows the recorder's own preview the same way; that
stream only runs while a recording does.

Starting a recording stops the previewer — the recording draws its own frames —
and stopping it starts the previewer again. Switching sources restarts it.

A source that cannot be opened reports the backend's error in the panel, with
`Try again` to retry. `OTTR_LIVE=1 go test ./internal/capture -run
'TestPreviewerDrawsTheSource|TestScreenshotWritesAPNG'` uses the real desktop.

## Screenshots

`capture.Snapshotter.Screenshot(ctx, sourceID)` takes one frame of the current
source, converts BGRA to `image.RGBA`, and writes a PNG through a temporary file
and a rename, so a crash leaves either a whole image or none. The record is
inserted with `source = "screenshot"`, its native size, and no duration; nothing
transcribes it. A screenshot is taken from the WGC stream's first frame, since
Windows Graphics Capture has no one-shot read.

## Where FFmpeg and ffprobe come from

`internal/tools` finds both, in this order:

1. the folder or program chosen in Settings ▸ Transcription (`settings.ffmpeg_path`),
   which is how a user fixes an install Ottr cannot find;
2. the folder the app keeps its binaries in (`<userData>/bin`, the one
   sherpa-onnx lives in), then `%APPDATA%\Ottr\bin`;
3. the folders `winget` installs into — `%LOCALAPPDATA%\Microsoft\WinGet\Links`
   and `Packages\*\*\bin`, where a portable FFmpeg lands;
4. next to the executable: its `bin` folder, then its own;
5. the `PATH`.

Looking in the folders before the `PATH` matters: a program installed while Ottr
runs cannot appear in the `PATH` of a process already started, so `winget
install Gyan.FFmpeg` followed by `Re-check` works without a restart. A missing
program names all of this in its error, which the preview panel, the error
banner and the settings row show whole.

## Audio

`Recorder.AudioDevices()` enumerates active render and capture endpoints,
putting each default first. Two full-row dropdowns choose a speaker and
microphone by endpoint ID, or `Off`; choosing one does not change the system
default. Enumeration runs off the UI thread, and an unreadable endpoint is
skipped rather than hiding the working devices.
`startAudioStream(loopback, deviceID, sink)` opens that endpoint in shared mode:

1. `CoInitializeEx` on a locked OS thread.
2. Find the selected endpoint with `EnumAudioEndpoints`, or call
   `GetDefaultAudioEndpoint(ERender|ECapture, EConsole)` when no ID is given.
3. `IAudioClient.Initialize` in `AUDCLNT_SHAREMODE_SHARED`, with
   `AUDCLNT_STREAMFLAGS_LOOPBACK` for system audio.
4. `GetMixFormat` determines the exact sample rate and channel count that FFmpeg
   is told about, so no conversion happens in Go.
5. Packets are read with `GetBuffer`/`ReleaseBuffer` and written to the pipe.
   Silent buffers are written as zeros; error flags do not stop the stream.
6. If the loopback endpoint produces no packets (Windows suspends the audio
   engine when nothing is playing), 20 ms of silence is written per iteration to
   keep FFmpeg's timestamps advancing. Without this, a silent desktop produces a
   recording with a video stream but no audio stream.

The stream ends when the session's `done` channel closes (on `Stop`) or when a
WASAPI call fails, which covers device unplug.

## Session lifecycle

`Start(cfg)`:

1. Reject if a session is active.
2. Open the native capture stream and wait for its first frame; fail atomically
   on denial, cancellation or an empty initial frame. The first frame fixes the
   recording canvas.
3. Start the raw-frame pipe the encoder reads, and start audio streams (fail
   the whole start if a requested device is unavailable, releasing anything
   already started).
4. Start the recording's own preview stream.
5. Spawn FFmpeg with the pipe as its video input, capture stderr into a buffer.
7. Publish `recording:state{recording:true, startedAt}`.

A watchdog goroutine waits on the FFmpeg process. If FFmpeg exits while
`r.current` still points at this session (i.e. not via `Stop`), the session is
treated as **failed**: audio and preview are closed, the partial file is
deleted, and `recording:state{error:...}` is broadcast. A failed recording can
never be reported as successful.

`Stop()`:

1. Clear `r.current` so the watchdog treats the exit as expected.
2. Write `q` to FFmpeg stdin and close it, so the MP4 container is finalized.
3. Close the capture stream, stop audio and close the preview.
4. Wait for exit with a 15 s deadline; on timeout, kill the process and delete
   the file.
5. Verify the file exists and is non-empty; otherwise delete and error.
6. Probe and insert into the library with `source = "recording"`.
7. If the media has audio and an ASR service is configured, start background
   transcription. This pass replaces the live text with the accurate one.

## Live preview

`Recorder.Preview()` returns the newest JPEG from the recording's own preview
stream, and the window decodes it into a bitmap at most five times a second, so
the view never touches the raw frames. See
[Preview before recording](#preview-before-recording) for how they are made.

## Live transcription

The same WASAPI buffers go to `audioSink`, which hands one buffer at a time to
the `LiveStarter` sink and drops the rest when the sink is behind: the capture
thread never waits. The sink is `asr.StartLive`, and the text it recognizes
appears in the window while the recording runs; see
[Transcription](transcription.md#live-in-recording-recognition). Only the first
audio stream is fed to it, so a recording of system audio and the microphone
gets live text from one of them.

## Output

Files are written to `<userData>/recordings/Recording YYYY-MM-DD HH.MM.SS.mp4`
with `libx264 -preset veryfast -crf 23 -pix_fmt yuv420p` and `aac 160k` when
audio is present. `yuv420p` keeps the file playable everywhere, the system's
own player included.

The recording canvas is fixed by the first frame: its size, rounded down to even
sides, is what the encoder is told. A window that resizes afterwards is scaled
and letterboxed into that canvas, so the codec's configuration never changes
mid-file. Frames are BGRA straight from the capture, and the encoder converts
them once.

### Encoder

The encoder is probed once per run (`encoder_windows.go`): `h264_nvenc`,
then `h264_qsv`, then `h264_amf`, each opened on a tiny test frame, and
`libx264` when none opens. A build offering a hardware encoder is no promise it
works — `h264_nvenc` fails without `nvcuda.dll`.

Measured on a 4K desktop, 8 seconds of 30 fps capture: `libx264` used 14.08 CPU
seconds, `h264_qsv` 9.38 — 33% less, and a slightly smaller file.

The file is a **fragmented mp4** (`-g 30 -movflags
+frag_keyframe+empty_moov+default_base_moof`): each keyframe's fragment is
written as it finishes rather than holding the index until the end. A recording
that dies without stopping cleanly — a killed process, a lost power — is still
playable up to its last keyframe. Measured on a killed 8-second recording: the
fragmented file played 5.77 seconds, while the plain mp4 of the same recording
was unreadable and remuxing recovered nothing.

The fragmentation changes nothing for the rest of the pipeline: ffprobe reads
the duration and streams, and `asr.ExtractWav` extracts the audio as before.

## Testing

`backend_test.go` needs no desktop and no media program: it uses a fake backend
of synthetic frames and covers the bounded queue (newest two, oldest dropped,
cancellation), BGRA→RGBA channel order, stride removal, aspect-fit letterboxing,
the JPEG preview rate, the previewer's error reporting, and PNG screenshots.

`recorder_windows_test.go` performs real recordings, all behind `OTTR_LIVE=1`:

- sources enumeration,
- one stream: tightly packed BGRA at physical pixels, a monotonic PTS, and a
  second `Close` that is safe,
- preview: a JPEG arrives from the real desktop,
- screen + system audio: asserts a valid JPEG preview frame arrives, a second
  `Start` is rejected, and the output has both video and audio,
- video only,
- microphone,
- a closed window is refused rather than recorded,
- a screenshot is a PNG at the source's own size and lands as `screenshot`.
