# Recording

Windows-only in the current implementation. Other platforms return an explicit
"not implemented yet (Phase 6)" error from `capture/listSources`.

## Why this design

Verified on the development machine (FFmpeg 9.0.1 Windows build):

- `ffmpeg -devices` lists **only** `gdigrab` and `dshow`. There is no `ddagrab`
  and no `wasapi` demuxer.
- Windows loopback capture through `dshow` requires installing a separate
  virtual-audio device driver, which is not acceptable as a product dependency.

Therefore:

- **Video** comes from FFmpeg's `gdigrab` (GDI screen capture), which is a
  native Windows API and needs no extra drivers.
- **Audio** is captured in-process with WASAPI through pure-Go COM bindings
  (`moutend/go-wca`), then written as raw `f32le` PCM into a Windows named pipe
  that FFmpeg reads as an input.
- **Encoding, resampling, mixing, and muxing** are all done by FFmpeg.

This keeps native code out of the Go process except for the COM calls WASAPI
requires, and keeps FFmpeg from needing a build with Windows audio demuxers.

## Pipeline

```
gdigrab ──────────────┐
                      ├─► ffmpeg ─► .mp4 (libx264 + aac)
WASAPI loopback ──pipe┘         └► mpjpeg over TCP ─► preview hub ─► window bitmap
WASAPI microphone ─pipe┘
WASAPI (any) ─────────┴─► liveFeed ─► asr live sink ─► live subtitles
```

## Sources

`Recorder.Sources()` enumerates three kinds:

| Kind | ID format | How it is captured |
|---|---|---|
| `screen` | `screen` | `gdigrab -i desktop` (whole virtual desktop) |
| `display` | `display:N` | `gdigrab` with `-offset_x/-offset_y/-video_size` from `EnumDisplayMonitors` |
| `window` | `window:<title>` | `gdigrab -i title=<title>` |

Monitor rectangles come from `EnumDisplayMonitors`; window titles from
`EnumWindows`, filtered to visible windows with a non-empty title, excluding the
Ottr process itself. Display sources are only offered when more than one monitor
exists.

Window capture matches by title. If two windows share a title, or the title
contains characters FFmpeg's `title=` parser mishandles, capture fails — this is
a known limitation, see ROADMAP.

## Audio

`startAudioStream(loopback bool)` opens the default render (loopback) or capture
(microphone) endpoint in shared mode:

1. `CoInitializeEx` on a locked OS thread.
2. `GetDefaultAudioEndpoint(ERender|ECapture, EConsole)`.
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
2. Resolve the source to FFmpeg input arguments; fail if the display or window
   no longer exists.
3. Start audio streams (fail the whole start if a requested device is
   unavailable, releasing anything already started).
4. Create the preview hub (a localhost TCP listener).
5. Spawn FFmpeg with `StdinPipe`, capture stderr into a buffer.
6. Publish `recording:state{recording:true, startedAt}`.

A watchdog goroutine waits on the FFmpeg process. If FFmpeg exits while
`r.current` still points at this session (i.e. not via `Stop`), the session is
treated as **failed**: audio and preview are closed, the partial file is
deleted, and `recording:state{error:...}` is broadcast. A failed recording can
never be reported as successful.

`Stop()`:

1. Clear `r.current` so the watchdog treats the exit as expected.
2. Write `q` to FFmpeg stdin and close it, so the MP4 container is finalized.
3. Stop audio and close the preview.
4. Wait for exit with a 15 s deadline; on timeout, kill the process and delete
   the file.
5. Verify the file exists and is non-empty; otherwise delete and error.
6. Probe and insert into the library with `source = "recording"`.
7. If the media has audio and an ASR service is configured, start background
   transcription. This pass replaces the live text with the accurate one.

## Live preview

`previewHub` listens on `127.0.0.1:0` and receives FFmpeg's second output
(`-f mpjpeg tcp://...`). It scans the byte stream for JPEG start/end markers and
keeps **only the most recent frame**. Memory is therefore bounded by one frame
regardless of how slow the window is.

`Recorder.Preview()` returns that frame, and the window decodes it into a
bitmap at most five times a second, so the view never touches the JPEG bytes
directly.

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
with `libx264 -preset veryfast -pix_fmt yuv420p` and `aac 160k` when audio is
present. `yuv420p` keeps the file playable everywhere, the system's own player
included.

## Testing

`recorder_windows_test.go` performs real recordings (no mocks):

- sources enumeration,
- screen + system audio: asserts a valid JPEG preview frame arrives, a second
  `Start` is rejected, and the output has both video and audio,
- video only,
- microphone,
- bad source.

A separate check verified with ffprobe that video and audio `start_time` are
both `0.000000` on a mixed system-audio + microphone recording.
