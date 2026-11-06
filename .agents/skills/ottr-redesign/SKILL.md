---
name: ottr-redesign
description: Redesign Ottr's UI end to end — audit the Doop canvas, choose a direction, rewrite the frames, then carry the result into the native MyGo window in internal/native. Use for a visual overhaul or a new design system, not for a one-frame tweak (that is the doop-canvas skill).
---

# Redesigning Ottr (Doop canvas → native MyGo)

A redesign has two halves: the canvas, which is the source of truth, and the Go window,
which must be able to draw everything the canvas shows. Do the canvas first, finish it,
then port. The doop-canvas skill covers frame mechanics (pushing HTML, prototype wiring,
`build.js`); this skill is the order of work and the decisions around it.

The current system is **"studio console + highlighter"** (docs/design/README.md). Read it
before proposing another; a redesign must earn its departure from it.

## 1. Audit (read-only)

1. `get_guide`, `get_canvas`, `get_guidelines` for the canvas style doc; `set_status`
   ("Auditing…") so watchers know what is happening.
2. `get_frame_screenshot` every main frame and open each image. Note what the eye does on
   each screen: what it lands on first, what competes, where space is dead.
3. Read `docs/design/README.md` (screens, tokens, breakpoints), `docs/ROADMAP.md` (what the
   app really does) and `internal/native/i18n.go` (real labels). A frame must not show a
   feature the ROADMAP says is missing unless it is clearly marked (disabled, SOON).
4. Write findings as problems, not taste: "one accent is used for selection, buttons, tabs
   and links, so nothing outranks anything", "the transcript, the product's output, sits
   in a narrow column".

Feature truths the canvas must respect: live text is provisional and replaced after Stop;
ASR is offline with SenseVoice / FunASR Nano / Qwen3 ASR only; translation and summary are
online and on request; speakers are `Speaker N` until renamed and never invented; volume
is mute-only; no editing features.

## 2. Direction

1. Try `search_inspiration` for an exemplar; if it is not configured, say so and derive the
   brief from the audit.
2. List four or five mood candidates as physical metaphors (maritime chart, darkroom,
   newsroom, field recorder, studio console + highlighter). Reject the first instinct if
   it is the generic one ("blue tech").
3. Pick one and justify it by mapping every colour to an object. For Ottr: paper = desk
   paper, ink = graphite pen, amber = the highlighter you mark a transcript with, red =
   the tally light of a recording camera, green = the level meter.
4. Name the hero device (for Ottr: the active transcript line — amber-soft fill, 3px amber
   bar, amber playhead and search ticks) and a one-line direction.
5. Decide the structural changes, not only the skin: what is persistent (sidebar), what
   docks (capture panel that becomes the live panel), what the library row says (first
   transcript line, pipeline chips), how the item page reads (stage + document panel).

## 3. Brief and tokens

Write the brief once, as a guideline (`set_guidelines`), with: palette light/dark as hex,
type (family, weights, scale, which text is mono), radii, control height, the single
shadow, the hero device and the structural rules. `save_decision` caps at 500 characters;
record the short decision there and keep the full brief in the guideline.

Mirror the tokens in `docs/design/tokens.css` and in Go (`kit/palette.go`) with the same
names, so a reviewer can diff them.

## 4. Doop tool sequence

1. Reuse frame ids when overwriting: `set_frame_html` on the existing frame, `update_frame`
   to rename or resize. Comments and prototype links survive; `create_frame` only for
   screens that had no frame (e.g. dark variants).
2. Generate frames from a small script (tokens + helpers + one function per screen) and
   push whole documents with `set_frame_html`. A review fix then lands on every frame.
3. After every push: `get_frame_screenshot`, open the image, review (section 5), fix,
   push again. Never batch-push without looking.
4. Lay out the board in rows with `update_frame` x/y: main + dark + recording, playback
   family, library states, settings + dialogs, responsive variants.
5. Wire the prototype (`data-*` attributes, see doop-canvas), `set_prototype_start`, then
   `get_prototype` and check every target exists.
6. `set_guidelines` with the final system; `get_canvas` for a last check (no duplicates);
   `set_status ""`.
7. Export: `DOOP_CLIENT=… node .agents/skills/doop-canvas/scripts/build.js --shots`.

## 5. Screenshot review checklist

For every frame, at its real size:

- **Fit**: nothing clipped, truncated mid-word or overflowing; tiles in a row share a height.
- **Dead zones**: a tall empty area means the layout is wrong — fill it with real content
  (a facts strip, last recording) or shrink the frame.
- **Hierarchy**: one thing first; the accent appears only on what is current.
- **Consistency of story**: the active line, the playhead time, the highlighted speaker
  tile and the search count ("3 of 4", tick count) agree with each other.
- **State truth**: a chip never claims work that has not run ("Waiting", not "Translated").
- **CJK**: load a CJK web font explicitly, or 中文 renders in a mismatched fallback.
- **CSS leaks**: scope nested selectors (`.tiles>div`), avoid generic class names that
  collide between components.
- **Contrast**: meta grey only at ≥11px; amber is never text on white (use amber-ink on
  amber-soft).
- **Realism**: real labels from i18n.go, real model names, specific content; no lorem.
- **Dark**: check at least the recorder, recording and playback in dark.

## 6. Native MyGo constraints (design within them)

The canvas may only use what `internal/native` can paint:

- Flat fills only. No gradients, blur, backdrop filters or images as chrome.
- 1px solid borders; a selected tile is a 2px stroke (border + same-colour inset).
- One shadow token, for dialogs and popovers. Cards are bordered, not shadowed.
- Rounded rects only, radii 6 / 10 / 12 / full; controls 32px tall.
- Icons from Lucide at stroke 1.75 (copied into `internal/native/icons/*.svg`).
- Fonts that can be embedded: Geist / Geist Mono (OFL, ship `OFL.txt`); CJK from the system.
- Alpha only as a single premultiplied fill (the duration badge over a thumbnail).
- States are fills and borders; nothing needs animation to be understood.
- Window sizes: main 1280×800, settings 920×660, responsive 900×620 and 420×800.

## 7. Mapping the design to internal/native

| Design part | Go |
| --- | --- |
| Tokens, radii, fonts | `kit/palette.go`, `theme.go` (`themeFor` maps the palette onto `ui.Theme`), `fonts/` |
| Reusable controls (button, chip, switch, group row, select, `Field`, avatar) | `kit/kit.go` |
| Frame, breakpoints (`layoutOf`: wide ≥1100, mid ≥560, narrow), title bar, rail | `app.go` |
| Sidebar and status card | `shell.go` |
| Capture panel, source tiles, audio rows, shortcuts | `capture.go` |
| Live panel while recording | `recorder.go` |
| Library page, rows, day groups, row facts | `library.go`, `details.go`, `rowfacts.go` |
| Item page, document panel, summary | `session.go`, `player.go`, `transcribing.go` |
| Dialogs / Settings / subtitle menu | `dialogs.go` / `settings.go` / `settings_menu.go` |
| Strings (both languages) | `i18n.go` |

Order of the port: tokens and kit → shell and library → item page → settings and dialogs
→ strings → tests. Commit in those pieces.

Check the port with the headless renderer, which writes PNGs named like
`docs/design/screens`:

```sh
OTTR_PREVIEW=<dir> OTTR_PREVIEW_CJK=<cjk .otf> go test ./internal/native -run TestPreview
```

Lay each PNG beside its screen (a few lines of PIL) and fix layout breaks at 1280, 900 and
420. List what you deliberately leave undone in `docs/ROADMAP.md` as known gaps.

## 8. Lessons from the native port

- **`ui.ButtonBase` centres its children.** A row-shaped button (list row, nav item,
  chapter row) needs `.Justify(ui.Start)` or its content floats to the middle.
- **Set derived maps before `reload()`.** Anything `reload` reads to draw (the thumbnail
  map, row facts) must be assigned first, or the first frame draws without it.
- **The preview test calls `frame()` directly.** `view()` also polls services and drains
  the update queue; a static screenshot fixture must call `a.frame(c, a.applyTheme(c))`
  so the fixture state is not overwritten.
- **Linux headless rendering has no CJK font.** Register one in the test
  (`OTTR_PREVIEW_CJK`), or every 中文 string draws as nothing and layouts look emptier
  than they are.
- **Accessible labels must be unique.** Tests click by label; two buttons named
  "Settings" (rail and subtitle menu) make the test click the one that opens a native
  window, which blocks a headless run. Give each a distinct label.
- **Narrow panels overflow quietly.** At 900 the 344px document panel cannot hold tabs,
  a status chip and three tools; drop the chip there rather than clip the tools. Give
  mono time columns a fixed width with `Shrink(0)` so "11:31" never wraps.
- **Section labels are uppercased by the kit.** Tests must look for "CHAPTERS", not
  "Chapters"; `upper` leaves CJK untouched.
- **Every new string gets a zh entry in the same commit.** Grep `a.tr(`/`a.trf(` and the
  string tables passed into them (labels in struct literals) for keys missing from `zhCN`.
- **Rewrite stale tests, do not delete them.** Keep what each test protects (the timer
  is not duplicated, back returns to the library) and update only the copy and layout it
  asserts.
- **Show the true state, not the canvas's.** Where a frame shows something the service
  cannot report (level meters, monitor names), leave it out and record the gap; never
  draw fake data in the shipped window.
