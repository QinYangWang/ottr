---
name: doop-canvas
description: Edit the Ottr design canvas in Doop — frames, prototype wiring, screenshots. Use when changing a screen's look, adding a frame, fixing prototype links, or keeping docs/design in sync with the canvas.
---

# Ottr design canvas (Doop)

Doop is the shared design surface for Ottr's window. Frames render real HTML, humans
watch them live, and the canvas records prototype wiring. The shipped app is Go/MyGo in
`internal/native` and never renders this HTML — the canvas is a reference, not a runtime.

The canvas id is in `.doop/urls.txt` (gitignored). If that file is missing, ask the user
for the canvas URL; do not create a new canvas.

## Read first

1. `mcp__doop__get_guide` once per session (topic `doop-instructions`).
2. `mcp__doop__get_canvas` for frame ids, sizes, pages and approval state.
3. `docs/design/README.md` for the screen inventory, tokens and the Go mapping.

## Where the HTML comes from

`.doop/` holds the generator (gitignored, local only):

| File | Role |
| --- | --- |
| `gen2.js` | every page template, shared CSS, the `srow()` row helper |
| `ui.js` | interaction layer: tabs, switches, selects, seek, menus, toasts |
| `icons/*.svg` | Lucide icons used by `ic(name, size)` |
| `scripts/build.js` | regenerates pages; `--shots` also refreshes `docs/design` |

Edit `gen2.js` — never patch generated HTML by hand, or the next build loses the change.
Two scripts keep the loop honest:

```sh
node .agents/skills/doop-canvas/scripts/build.js          # regenerate .doop/*.html
node .agents/skills/doop-canvas/scripts/build.js --shots  # + docs/design pages/screens
```

The second form copies the generated pages into `docs/design/pages` (with the
settings-height and 420px overrides) and renders every screenshot into
`docs/design/screens`. Set `DOOP_BROWSER` to a Chromium/Edge executable if it is not
found automatically.

For a single page, render it directly:

```sh
msedge --headless=old --hide-scrollbars --virtual-time-budget=5000 \
  --window-size=1280,800 --screenshot=.doop/shot.png \
  "file:///<repo>/.doop/session.html"
```

Sizes: recorder/playback/library/recording 1280×800, settings 920×660 (snapshots use
920×1050 so the full page is captured), compact 900×620, narrow 420×800 (set
`html,body{width:420px}` — Chromium will not lay out below ~500px).

## Pushing a frame

- Prefer `append_frame_html` (streamed, watchable) for new or heavily reworked frames;
  `set_frame_html` for a whole-document replace; `edit_frame_html` for one string.
- **Keep every `data-doop-*` attribute** on elements you carry over. Comments, links,
  prototype navigations and locks ride on them. A wholesale replace that drops
  `data-doop-on` / `data-doop-action` silently breaks the prototype.
- `get_frame` returns a JSON envelope, not raw HTML: `JSON.parse(content[0].text).html`.
  String replacement against the envelope fails; parse it first.
- After pushing, call `get_frame_screenshot` and look at it. Fix spacing, overflow and
  contrast until the frame actually looks right, not just compiles.
- Use `set_status` while working and clear it when done; `save_decision` records style
  feedback the user gave in chat.

## Prototype rules

- Every visible control must respond: navigation, a state change, or a toast.
- Targets are frame ids: `data-doop-on="click" data-doop-action="navigate:<id>"`,
  `data-doop-transition="dissolve"`.
- Check `get_prototype` after structural edits: 0 problems, no dangling targets.

## Sync order

1. Change the design here and verify screenshots.
2. Mirror it in Go (`internal/native`) — the user expects design first, code second.
3. Refresh the committed snapshots with `build.js --shots`, and update the table in
   `docs/design/README.md` if a screen was added or resized.

## Do not

- Do not fabricate speaker identities. Speakers stay `Speaker 1…N`, renameable.
- Do not add web UI to the shipped app. No webview, no HTML, no JS in `internal/`.
- Do not commit `.doop/` — it is local tooling and gitignored.
