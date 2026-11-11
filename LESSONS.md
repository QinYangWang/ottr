<!-- reviewed-through: 25801e5 (2026-11-10) -->
# Lessons

**Search this file, do not read it end to end.** Grep the subsystem or the symptom
(`capture`, `ffmpeg`, `layout`, `i18n`, `page state`, `design sync`). One rule per
entry, each with the commit that proves it.

Ottr is maintained by one owner working with agents; the owner merges. An entry
enters this file only with a commit, issue or owner decision behind it. When the
same rule shows up on **three separate days**, it moves to
[code-standards.md](code-standards.md) as a reviewer-checkable row, gets a check
where one is possible, and is listed under "Moved to the code standards".

## Platform splits

**A symbol shared code reads must exist under every platform build tag.**
- `6c950d9`: `recordFPS` was declared only in `internal/capture/*_windows.go`;
  the shared recorder used it, so the package did not build — and nothing above
  it was tested — on macOS or Linux.
- Fix: declare shared values in the untagged file, or give each tag its own
  definition, and build the other platform in the same change.
- Seen 1× (2026-11-06).

## Capture and FFmpeg

**An FFmpeg option goes on the input or output that reads it, never as a global.**
- `ef7e7d3`: `draw_mouse` was passed before `-i`, which `gdigrab` tolerated and
  `ddagrab` rejected with "Option draw_mouse not found", so the live preview broke
  on every machine where Desktop Duplication works.
- Fix: `resolveSourceFPS` takes the flag and writes it into that input's own
  argument set; the preview omits it, the recording keeps it.
- Seen 1× (2026-10-28).

**A recording must stay playable if the process or the machine dies.**
- `900956f`: a plain mp4 keeps its index in the `moov` atom written at a clean
  stop; a killed 8-second recording left 786 KB that ffprobe refused to open
  ("moov atom not found") and remuxing recovered nothing.
- Fix: fragmented mp4, fragments cut at each keyframe, and `crf 23` pinned —
  libx264 had no rate control, so the same content produced 584 KB and 995 KB in
  two three-second runs. Same killed file now plays 5.77 s.
- Seen 1× (2026-10-28).

**The idle preview captures nothing; a preview that grabs continuously starves
the desktop compositor.**
- `ce74dc0`: at 30 fps the preview held a core and the system pointer stuttered;
  at 5 fps it still stuttered. 3M samples of `GetCursorInfo`/`GetCursor` showed
  no change in pointer shape, position or visibility — gdigrab's `BitBlt` was
  competing with the compositor.
- Fix: one frame per `Show`, refreshed every three seconds; a recording's own
  preview runs only while recording.
- Seen 1× (2026-10-28).

## View state

**Page state lives on `App`; a value held in a local inside a page function is
reset every frame.**
- `ac9c105`: the delete dialog's "keep the file" checkbox kept its value in a
  local, so it was `false` on every redraw and the file was never deleted;
  the confirmation it drew was not the answer the user gave.
- Fix: the choice is an `App` field, read when the job runs.
- Seen 1× (2026-11-06).

**Every control a frame draws does something, including the one that only opens
a path.**
- `2b42ad4`: the Custom provider tile is where a custom endpoint starts, but its
  click was ignored unless a URL was already typed, so Settings could never leave
  a preset provider.
- Fix: clicking it clears the field for entry and keeps a URL the user typed.
  A dead or conditional click gets a disabled state whose tooltip says why.
- Seen 1× (2026-11-10).

**Clip a custom child inside a rounded bordered root, or it paints over the
corners.**
- `25801e5`: the header's hover fill was a square child of the bordered root, so
  it covered the border's rounded corners on hover.
- Fix: clip the root before drawing children, as `Group` does, and let the border
  draw over them.
- Seen 1× (2026-11-10).

## Design sync

**Regenerated artifacts ship in the same change as the frame they come from.**
- `e759d87` said the committed `docs/design/pages` and `screens` were not
  regenerated while the canvas was being redesigned; `0028232` then had to
  regenerate them.
- Fix: run `node .agents/skills/doop-canvas/scripts/build.js --shots` before the
  commit, and say in the body which frames changed.
- Seen 1× (2026-11-06).

## Done right

- `8f9eddb` reads the samples once, decodes 30-second chunks and diarizes them in
  parallel, and a diarizer that fails leaves the transcript without speaker
  labels instead of failing the job — a measurement and a fallback in one change.
- `6d9a0d2` adds `TestPreview`, so a layout claim can be checked and the screenshot
  kept as evidence instead of described in prose.

## Moved to the code standards

Nothing yet. A rule that is enforced by `scripts/verify.sh` or a test is listed
here with the date it moved.
