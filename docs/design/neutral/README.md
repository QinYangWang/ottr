# 中性 direction — an alternative look for Ottr

An exploration of Ottr's window in a **neutral, shadcn/ui "Nova" look on the Tailwind v4
neutral palette**: near-monochrome surfaces, hue only where it means something, one rounded
panel, 4px spacing, 32px controls. It is **not** the shipped design: `docs/design/README.md`
and `docs/design/pages` still describe the "ink and marks" system the app draws today. This
folder is the record of the alternative, so the next agent can carry on without the chat.

The frames live on the Doop canvas **`GqWuJqbn2T`** (canvas "Ottr"), page **`yu8EfmmV_N`**
("方案 · 中性"). Page 1 of that canvas is the shipped design; do not move frames between the
two pages and do not rename it to anything but 中性.

## Frames

| Snapshot | Frame id | Size | What it shows |
| --- | --- | --- | --- |
| `screens/01-recorder.png` | `BMfF2objk0` | 1280×800 | 录制 · 浅色 — sidebar, library list, New capture panel |
| `screens/02-recorder-dark.png` | `a4zaIZP-zl` | 1280×800 | 录制 · 深色 |
| `screens/03-playback.png` | `wpFrqTobTk` | 1280×800 | 播放 · 浅色 — stage, chapters, transcript panel |
| `screens/04-playback-dark.png` | `4ggD-pyLom` | 1280×800 | 播放 · 深色 |
| `screens/05-recording.png` | `cx6vbyZZvc` | 1280×800 | 录制中 · 浅色 — REC strip, the live picture, the four fields, live transcript |
| `screens/06-recording-dark.png` | `nGtt8pDKmL` | 1280×800 | 录制中 · 深色 |
| `screens/07-settings-appearance.png` | `Bq9YBXj-RZ` | 1280×800 | 外观 · 浅色 — language, theme tiles, display font, accent |
| `screens/08-settings-appearance-dark.png` | `Kk9ACEPEFc` | 1280×800 | 外观 · 深色 |
| `screens/09-settings-transcription.png` | `veWf7gLZXA` | 1280×800 | 转写 · 浅色 — the three models, In use, download, spoken language |
| `screens/10-settings-provider.png` | `eCOCENEaON` | 1280×800 | 服务商 · 浅色 — endpoints, the three fields, import chain, prompts |
| `screens/11-settings-speakers.png` | `3K4bIKo85t` | 1280×800 | 说话人 · 浅色 — the voice library with rename and forget |

`pages/<n>-<screen>.html` are those frames' own documents, exported from the canvas. They are
self-contained (inline CSS, Google Fonts links) and are the fastest way to read or edit a
frame without the canvas. **The canvas wins**: when a page and the canvas disagree, re-export
the page.

## Prototype wiring

The page's start frame is `BMfF2objk0` (录制 · 浅色). Every link resolves; keep it that way.

```
录制(浅) --row click--> 播放(浅) --Library/back--> 录制(浅) --Start recording--> 录制中(浅) --Stop--> 播放(浅)
录制(深) --row click--> 播放(深) --Library/back--> 录制(深) --Start recording--> 录制中(深) --Stop--> 播放(深)
录制中(浅/深) --Library--> 录制(浅/深)
every frame --Appearance--> 外观(浅);  every frame --Transcription/Provider/Speakers--> that 设置 page
外观(浅/深) --Theme Light/Dark tile--> 录制(浅/深)
```

Wiring is plain attributes in the frame HTML, as the doop-canvas skill describes:
`data-doop-on="click" data-doop-action="navigate:<frame id>"`. The frames are pushed with
`set_frame_html`, so **whatever rewrites a frame must preserve every `data-doop-*` attribute**.

## Tokens

The frames draw from these CSS variables (light / dark). They are the app's `internal/theme`
names, one for one, so a Go port is a rename, not a redesign.

| Variable | Light | Dark | Use |
| --- | --- | --- | --- |
| `--window` | `#EDEDED` | `#1E1E1E` | the strip behind the panel (title bar area) |
| `--sidebar` | `#FAFAFA` | `#171717` | sidebar, small inset fields, chips' neutral fill |
| `--bg` | `#FFFFFF` | `#0A0A0A` | content, panel, dialogs |
| `--fg` | `#0A0A0A` | `#FAFAFA` | body text, the selected tab's underline, the playhead |
| `--muted` | `#737373` | `#A1A1A1` | secondary text, icons at rest, meta |
| `--border` | `#E5E5E5` | `#FFFFFF1A` | panel edge, dividers, chips' edge |
| `--input` | `#E5E5E5` | `#FFFFFF26` | outlines of selects, inputs, outline buttons |
| `--ring` | `#A1A1A1` at 50% | `#737373` at 60% | keyboard focus ring, hovered divider |
| `--hover` / `--selected` | `#F5F5F5` | `#262626` | hovered row, ghost button, segmented track, selected nav |
| `--primary` / `--primary-text` | `#171717` / `#FFFFFF` | `#E5E5E5` / `#171717` | Start recording, play button, "All" pill, switch on |
| `--danger` | `#E7000B` | `#E7000B` | Stop, REC dot, destructive |
| `--success` | `#00A63E` | `#00A63E` | level meters, the "Synced" dot |
| `--unread` | `#2B7FFF` | `#2B7FFF` | the selected speaker tile's ring, unread dot |

Status tints (chips and toasts) come in three steps each — light / dark:

| Family | Fill | Edge | Text |
| --- | --- | --- | --- |
| green (Transcribed) | `#F0FDF4` / `#052E16` | `#B9F8CF` / `#166534` | `#008236` / `#86EFAC` |
| amber (Transcribing) | `#FFFBEB` / `#451A03` | `#FFD230` / `#92400E` | `#973C00` / `#FCD34D` |
| red (Failed, Recording) | `#FEF2F2` / `#450A0A` | `#FFC9C9` / `#991B1B` | `#C10007` / `#FCA5A5` |
| blue (中文) | `#DBEAFE` / `#172554` | `#BEDBFF` / `#1E40AF` | `#1447E6` / `#93C5FD` |

**Rules that make it read as one system**

- Hue is a state, never decoration: green = done, amber = working, red = recording or failed,
  blue = a language mark. Everything else is neutral.
- Borders, not shadows, between peers; shadow only lifts floating things (`--shadow-xs` on
  controls, a dialog gets the big one). One panel, inset 8px from the window's edges.
- Spacing on the 4px grid; controls 32px high (28 small, 40 large); rows 32px.
- Radii: 4 chips/kbd, 6 icon buttons, 8 buttons/rows/selects, 12 the panel and the stage.
- Type: Geist 400/500/600, Geist Mono for every number. 11/12/13/14/15/16/18px steps; body
  14/1.3, rows 13/1.3, transcript 13/1.5, translations 12.5 muted, section labels 11 muted.

## Layout

```
window 1280×800
└ top bar 44          lights · ottr · Recordings   …  338.2 GiB free · Search Ctrl K
└ panel (radius 12, inset 8 left/right/bottom, border 1)
  ├ sidebar 248      Library · Appearance · Transcription · Provider · Speakers
  ├ list (flex 1)    title + Import media, filters (segmented), day groups, rows
  └ right column     capture 360  (recorder)   |   transcript 440 (playback, recording)
```

The list is the flexible column: the right column is pinned to the panel's edge, so 1280
gives 656px of list. Sidebar and right column never grow. Settings uses the same shell — the
sidebar stays, the page takes the rest; there is no separate settings window in the app.

`New capture` is a 360px column with 14px padding: source tiles (2×2, the checked one gets an
inset 1px ink ring), a 16:9 preview on `#171717` with a white `Display 1 · 2560×1440` badge,
a display select, then AUDIO (System audio, Microphone) and WHILE RECORDING (Live transcript,
Translate) with the labels `capture.go` really draws, then a one-line summary of the choice
above the primary `Start recording` button with `Ctrl Shift R` kbd chips inside it.

The recording page drops the level meters the service cannot report; a mono line under the
stage carries what it does know (`H.264 · 1080p · 412 MB`).

The playback and recording screens share `stage` (16:9, `#171717`, radius 12), `facts`
(4 columns, hairline above) or `recfields` (2×2 label + select), and the 440px `doc` column:
underline tabs (Transcript / Summary), a sub-bar (`Spoken` select, `中文 under lines` switch),
rows (24px avatar tile, name 12/600, mono time 11, text 13/1.5, translation 12.5 muted), and
a 40px footer (`Transcribed on this computer · SenseVoice · N speakers … lines`).

The active transcript row is `--hover` with a 2px `--fg` bar on the left; the search hit is
the only coloured text background (`#FFF085` with `#0A0A0A`).

## Things the frames already settled

- Settings opens in the main window: 1280×800, the same 184px sidebar, one page of bordered
  rows. It is why there are no 920×660 frames.
- Sidebar labels are shown on the library and settings screens and collapse to a 64px rail
  on the item pages, where the stage and the 440px document panel need the width.
- The capture panel shows the controls `capture.go` has: two audio selects and two live
  selects. Its 360px column is sized to fit 800px without scrolling.
- The play button sits at the stage's centre, so speaker tiles put their name at the foot,
  not the top left.
- Transcript panels scroll; the rows stop above the footer instead of running under it.

- The disk figure moved to the top bar (`338.2 GiB free` with a hard-drive icon); the
  sidebar has no disk card.
- `Ctrl Shift R` appears **once**, inside the primary button as kbd chips — not in the
  panel header, not as a hint under the button.
- `New capture` is flush with the panel's right edge (the list flexes).
- The live transcript dims the provisional line and ends it with a 2px caret, no avatar
  (speakers are unknown before the full pass).
- Stop is a filled `--danger` button; Pause is an outline button.
- Chinese text is rendered by a per-frame Google Fonts subset: the build adds
  `&text=<the frame's non-ASCII characters>` for Noto Sans SC. Without it the canvas
  renderer draws tofu, because it has no system CJK font. Keep that when adding frames.

## Next

1. **More screens**, same page, same 2×2 rhythm: the transcribing pipeline card (job state,
   progress, Cancel), the ⌘K search palette, the three dialogs (delete / export / transcription
   failed), first-run, empty library, and the download-in-progress variant of Transcription.
   Then 900×620 and 420×800 variants of recorder and playback. The settings pages are done:
   light for all four, dark only for Appearance until the others earn one.
2. **A theme you can see in the prototype**: the Appearance tiles switch the recorder's theme;
   the settings pages themselves only exist in light.
3. **Decide the remaining questions** with the owner before going wide:
   - icons: the frames use Lucide line icons (what the app ships). The style's native set is
     Remix (line at rest, fill when selected). Switching is a one-pass change but touches
     every frame.
   - the wordmark: currently lowercase `ottr` + ring glyph, like the app today.
4. **Then the Go port**: `internal/native/kit/palette.go` gains the neutral tokens beside the
   current ones, `theme.go` picks one, and the recorder page is rebuilt first. Do not start in
   Go before the canvas frames for that screen exist and are approved.

## Working on it

- Canvas: `GqWuJqbn2T`, page `yu8EfmmV_N`. Push with `set_frame_html` (frames are whole
  documents), then `get_frame_screenshot` and judge the render (fit, spacing, hierarchy,
  contrast, alignment, realism) before moving on.
- Keep every `data-doop-*` attribute; check `get_prototype` reports 0 problems after edits.
- Re-export `pages/` and `screens/` in this folder after a frame changes (the same
  `get_frame` / `get_frame_screenshot` pair), or the record here goes stale.
- Do not mention any third-party app or repo in the frames or in this folder: the direction
  is called **中性 (neutral)**.
