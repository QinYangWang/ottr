# Ottr UI design reference

Static snapshots of the Ottr desktop UI, exported from the Doop canvas, plus the tokens they
are drawn with. These pages are **design references only**: the shipped app is native Go/MyGo
in [`internal/native`](../../internal/native) and never renders HTML. Use this folder to check
spacing, colour and state behaviour, then mirror the change in Go.

The system is **"studio console + highlighter"**: graphite ink on cool paper-grey, one amber
accent for what is current (the playing line, the playhead, a search hit, a focused field),
red only for recording and destruction, green only for "done" and level meters.

## Screens

Open any file in `pages/` in a browser. Each one is the frame's own HTML, self-contained.

| Screenshot / page | Size | What it shows |
| --- | --- | --- |
| `01-recorder` | 1280×800 | Sidebar, library list and the New capture panel |
| `02-recorder-dark` | 1280×800 | Same, dark |
| `03-recording` | 1280×800 | Recording, in the item page's shell: the REC strip and the file being written on one line, Pause/Stop, the live picture in the stage, the source, sounds and model as rows with selects, and the live transcript on the right |
| `04-recording-dark` | 1280×800 | Same, dark |
| `05-transcribing` | 1280×800 | Item page after Stop: the pipeline card (transcribe → translate → summarize) |
| `06-playback` | 1280×800 | Item page: way back, stage, chapter-segmented seek bar, facts strip, document panel |
| `07-playback-dark` | 1280×800 | Same, dark |
| `08-summary` | 1280×800 | Document panel on the Summary tab |
| `09-search` | 1280×800 | Transcript search with hit ticks on the seek bar |
| `10-library-grid` | 1280×800 | Library as a grid |
| `11-library-empty` | 1280×800 | First run: record or import, three steps |
| `12-settings-appearance` | 1280×800 | Settings in the main window: language, font, theme, highlighter |
| `13-settings-transcription` | 1280×800 | Speech recognition models and the spoken language |
| `14-settings-provider` | 1280×800 | Provider tiles, bordered fields, import chain, prompts |
| `15-settings-speakers` | 1280×800 | Speaker library with search and rename |
| `23-settings-transcription-download` | 1280×800 | The Qwen3 download in progress: model row with a progress bar |
| `24-settings-transcription-first-run` | 1280×800 | First run without a model: the download callout and three Download buttons |
| `25-first-run` | 1280×800 | The first-run guide: set up transcription, the three models with their download, Later |
| `16-dialog-delete` | 440×252 | Delete recording |
| `17-dialog-export` | 460×452 | Export: transcript / translation / summary, formats |
| `18-dialog-failed` | 460×256 | Transcription failed |
| `19-recorder-900` | 900×620 | Rail + recorder, last recording under the audio rows |
| `20-recorder-420` | 420×800 | Narrow recorder, library button in the header |
| `21-playback-900` | 900×620 | Rail + item page, 344px document panel |
| `22-playback-420` | 420×800 | Narrow item page, document under the player |

### Breakpoints

The main window opens at **1280×800** with a **40px** title bar the view draws itself.

| Width | Layout (`layoutOf` in `app.go`) |
| --- | --- |
| ≥ 1100 | `wide`: 232px sidebar, library, 360px capture panel; item pages use a 56px rail and a 440px document panel |
| 560 – 1099 | `mid`: 56px rail and one page (recorder or library); item pages use a 344px document panel |
| < 560 | `narrow`: one column; the recorder header has a Library button, item pages stack the document under the player |

## Tokens

[`tokens.css`](tokens.css) carries the palette for light and dark; the Go mirror is
[`kit/palette.go`](../../internal/native/kit/palette.go).

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| paper | `#F3F4F5` | `#0E0F11` | Window chrome, sidebar, rail |
| sheet | `#FFFFFF` | `#16181B` | Content, cards, fields |
| sunk / hover | `#F7F8F9` / `#EEF0F2` | `#1B1E22` / `#22262B` | Inset wells / hover fills |
| ink · ink2 · ink3 · ink4 | `#121417` `#4A5058` `#7C838C` `#A6ACB3` | `#EDEEF0` `#A9AFB7` `#737A83` `#555B62` | Text: primary, secondary, meta (≥11px only), disabled |
| line / line2 | `#E4E6E9` / `#CDD1D6` | `#2A2E33` / `#3A3F46` | Dividers and card borders / control borders |
| amber | `#FFB020` | `#FFB020` | The highlighter: current line, playhead, focus |
| amber-soft / amber-ink | `#FFF4DB` / `#8A5600` | `#33270F` / `#FFC453` | Active-line fill / text on it (amber itself is never text) |
| red / red-soft | `#E5383B` / `#FDECEC` | `#E5383B` / `#3A1516` | Recording and destructive only |
| green / green-soft | `#1F9D61` / `#E4F5EC` | `#3DD68C` / `#12301F` | Done chips, level meters |
| btn | `#121417` | `#EDEEF0` | Primary button fill |
| media | `#1B1E22` | `#0A0B0D` | Stage and thumbnails |

Radii: controls **6**, cards **10**, panels and groups **12**, pills full. Controls are **32px**
tall. One shadow (`--shadow`) for dialogs and popovers; cards use 1px borders, never shadows.
A selected tile is a 2px ink stroke.

Type: **Geist** 400/500/600 for the UI and **Geist Mono** (tabular figures) for every number,
timecode, size, path and key, and for the 10.5px capital section labels. Both are embedded
from `internal/native/fonts` (SIL OFL 1.1, `fonts/OFL.txt`); CJK falls back to the system
(Microsoft YaHei UI, Noto Sans SC). Scale: 20 / 18 / 16 titles, 13 body, 12.5 中文 and
secondary, 11.5–12 meta, 10.5 mono labels.

## Native constraints

The canvas is drawn only with what MyGo can paint: flat fills, 1px borders, rounded rects,
one shadow, Lucide icons at stroke 1.75. No gradients, blur, backdrop filters or images as
chrome. Hover, on and focus states are fills and borders only.

## Regenerating pages and screenshots

```bash
DOOP_CLIENT="python3 <path-to>/doop.py" node .agents/skills/doop-canvas/scripts/build.js --shots
```

`DOOP_CLIENT` is a Doop MCP client that takes `call <tool> -` with JSON arguments on stdin.
Without it the pages are rebuilt from the gitignored `.doop/frames/` cache. Screenshots come
from a local Chromium/Edge (`DOOP_BROWSER`), or from Doop's `get_frame_screenshot` when none
is found. The frame → page table is at the top of `build.js`.

To compare the Go window with these screenshots, render it headless:

```bash
OTTR_PREVIEW=<dir> OTTR_PREVIEW_CJK=<a CJK .otf/.ttf> go test ./internal/native -run TestPreview
```

`TestPreview` writes PNGs under the same names as `screens/`. `OTTR_PREVIEW_CJK` is needed on
machines without a CJK font (Linux containers), where 中文 would otherwise draw as nothing.

## Mapping to the Go implementation

| Design | Go |
| --- | --- |
| Palette, radii, heights, fonts | [`kit/palette.go`](../../internal/native/kit/palette.go) |
| Buttons, chips, switches, groups, selects, bordered `Field`, avatars | [`kit/kit.go`](../../internal/native/kit/kit.go) |
| ui.Theme mapping, font registration, the one shadow | [`theme.go`](../../internal/native/theme.go), [`fonts/`](../../internal/native/fonts) |
| Window frame, breakpoints, title bar and REC pill, rail, back button | [`app.go`](../../internal/native/app.go) |
| Sidebar: Library, Settings sections, disk card | [`shell.go`](../../internal/native/shell.go) |
| New capture panel: source tiles, preview, audio rows, last recording, Ctrl+Shift+R | [`capture.go`](../../internal/native/capture.go) |
| Recording page while it runs (stage with the live picture, clock, Pause/Stop, facts, live transcript) | [`recorder.go`](../../internal/native/recorder.go) |
| Library page: title, search (Ctrl+K), filter chips, list/grid, day groups | [`library.go`](../../internal/native/library.go), [`details.go`](../../internal/native/details.go) |
| A library row's first line and speaker count | [`rowfacts.go`](../../internal/native/rowfacts.go) |
| Thumbnails | [`thumbnail.go`](../../internal/native/thumbnail.go) |
| Item page: stage, facts strip, document panel and its two tabs, summary | [`session.go`](../../internal/native/session.go) |
| Transport and seek bar | [`player.go`](../../internal/native/player.go) |
| Pipeline card after Stop | [`transcribing.go`](../../internal/native/transcribing.go) |
| Dialogs: delete, export, transcription failed | [`dialogs.go`](../../internal/native/dialogs.go) |
| Settings pages in the sidebar | [`settings.go`](../../internal/native/settings.go) |
| First-run guide before a model is installed | [`onboarding.go`](../../internal/native/onboarding.go) |
| Subtitle settings menu (document panel) | [`settings_menu.go`](../../internal/native/settings_menu.go) |
| Icons (Lucide) | [`icons.go`](../../internal/native/icons.go), [`icons/`](../../internal/native/icons) |
| Strings, English and 中文 | [`i18n.go`](../../internal/native/i18n.go) |
| Headless screenshots of every screen | [`preview_test.go`](../../internal/native/preview_test.go) |

Where the Go window still differs from a frame, the gap is listed in
[ROADMAP §17](../ROADMAP.md#17-redesign-gaps-between-the-canvas-and-the-window--partial).
