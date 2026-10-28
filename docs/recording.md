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
| `screen` | `screen` | `ddagrab` (Desktop Duplication), or `gdigrab -i desktop` where that is refused |
| `display` | `display:N` | the same, offset and sized from `EnumDisplayMonitors` |
| `window` | `window:<title>` | `gdigrab -i title=<title>`; duplication has no window mode |

Desktop Duplication is preferred because it reads what the compositor already
drew instead of forcing a fresh GDI copy. Windows refuses it while the session
sits on a secure or disconnected desktop (a locked screen, fast user switching,
a dropped remote session), so `ddaAvailable` probes it once per run with a
single frame and falls back to `gdigrab` when the answer is no. `gdigrab` also
remains the only way to capture a single window.

Monitor rectangles come from `EnumDisplayMonitors`; window titles and sizes from
`EnumWindows` and `GetWindowRect`, filtered to visible windows with a non-empty
title, excluding the Ottr process itself. Display sources are only offered when
more than one monitor exists. Both carry a `Detail` — the rectangle's size, as
`1920x1080` — which the window dropdown shows; a window that reports no sane
rectangle (minimized, or in another session) carries none.

Window capture matches by title. If two windows share a title, or the title
contains characters FFmpeg's `title=` parser mishandles, capture fails — this is
a known limitation, see ROADMAP.

## Preview before recording

`capture.Previewer` runs the same input `resolveSourceFPS` resolves for a
recording, at `previewFPS` (5) instead of 30, and writes an `mpjpeg` stream over
TCP into a `previewHub` that keeps only the newest frame. The window decodes it
into a bitmap and draws it in the main window. Asking the source for five frames
a second rather than grabbing thirty and dropping most of them is what keeps the
cost down: 36% of a core instead of 112%.

The preview passes `-draw_mouse 0` — the desktop's own pointer already sits over
the picture, and a second one lags behind it as it moves.

Library's recording page shows the recorder's own `mpjpeg` preview the same way,
which only runs while a recording does.

Starting a recording stops the previewer — the recording draws its own frames —
and stopping it starts the previewer again. Switching sources restarts it.

When FFmpeg stops on its own — bad arguments, a window that closed, no FFmpeg at
all — the panel shows the first line of what FFmpeg said, with `Try again` to
start it afresh. `OTTR_LIVE=1 go test ./internal/capture -run
TestPreviewerShowsTheScreen` grabs the real screen and checks that what comes
back is a JPEG.

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
