# AGENTS.md
<!-- repo-governance harness: v2 -->

Ottr is a local-first screen recorder with live and offline transcription, translation and
AI summaries, plus a searchable media library. The window is native [MyGo]
(https://github.com/egoist/mygo) UI written in Go — no webview, no HTML, no IPC. SQLite
stores everything; FFmpeg decodes and probes media; sherpa-onnx (cgo) does ASR.

Agents write and review changes here; the owner merges and settles what "Human gates"
lists. Read this file once, start to end: the route table names at most one more document
per task, so do not read the docs tree before your first edit.

## Project facts

| Fact | Value |
|---|---|
| Stack | Go 1.27; MyGo native UI; SQLite (`modernc.org/sqlite`, pure Go); FFmpeg/ffprobe; sherpa-onnx C API via cgo |
| Module / entry | `ottr` / `cmd/studio` |
| Fast check | `go vet ./...` |
| Full suite | `CGO_ENABLED=1 go test ./...`, local, ~50 s. CI runs `go test ./...` on `windows-latest` only (`.github/workflows/build.yml`) |
| Commit-header lint | `bash scripts/verify.sh`, section `commit convention` |
| Real use | `CGO_ENABLED=1 go run ./cmd/studio`; layout: `OTTR_PREVIEW=<dir> OTTR_PREVIEW_CJK=<cjk font> go test ./internal/native -run TestPreview` |
| Branches | `main` only. Every change is a short-lived branch off it; the owner merges |
| Release | `CGO_ENABLED=1 go tool mygo build -platform windows/amd64` (macOS: `darwin/arm64`); CI uploads `dist/<platform>/`. FFmpeg and the sherpa-onnx library are found at runtime, never committed |
| Platforms | Windows 10/11 is the supported capture platform; the app still builds without cgo, and then says native ASR is unavailable |
| Never touch without the owner | applied files in `migrations/`, `resources/<plat>/`, `.github/workflows/`, the model pins in `docs/models.md` |

FFmpeg/ffprobe come from `PATH` or `%APPDATA%\Ottr\bin`; the ASR library from
`resources/<platform>/bin` or `%APPDATA%\Ottr\bin`. Data lives in `%APPDATA%\Ottr`; the
download-and-verify path is in [docs/development.md](docs/development.md).

## The loop

1. **Branch off `main`.** One goal per branch; never a fix and a feature together.
2. **Reproduce** the case on the surface the user has — or write the failing test first.
3. **Map the whole path**: callers, sibling platform files, other providers, saved data,
   tests, docs, the Doop frame.
4. **Make the smallest complete change.** No drive-by refactor, reformat or dependency bump.
5. **Prove it**: red check, break check, full suite, real use, not-run list.6. **Record and merge**: the evidence block goes in the commit body; the owner merges the
   reviewed head.

## Rules that always apply

- The shipped app has **no web UI**: Go/MyGo only. `docs/design` HTML is reference.
- No visible change starts in Go: Doop frame first, then `docs/design`, then code.
- The UI layer never touches SQL, FFmpeg or WASAPI; it calls services.
- Every user-facing string goes through `a.tr` / `a.trf` with an `i18n.go` entry in both
  English and Chinese, same commit; model and provider names stay English.
- Only SenseVoice, FunASR Nano and Qwen3 ASR are accepted models.
- Speaker identities are never invented: `Speaker N`, renameable globally. No fake data.
- Migrations are append-only; never edit an applied file.
- Background work updates the view only through `a.update`.
- View state lives on `App`; a local inside a page function is reset every frame.
- Tests that need ffmpeg, a model, a device or the network call `t.Skip`, never fake it;
  a bug fix's test fails on the parent commit.
- A symbol read by shared code exists under every build tag; build the other platform.
- An FFmpeg option goes on the input or output that reads it, never as a global.
- A control a frame draws does something, or shows a disabled state saying why.

## Route by task

Read the row that matches, then stop.

| Task | Read |
|---|---|
| A rule you are unsure of | [code-standards.md](code-standards.md) — grep the rule |
| "This went wrong before" | [LESSONS.md](LESSONS.md) — search the subsystem or symptom |
| Gates, run them | [scripts/verify.sh](scripts/verify.sh) — run it, do not read it |
| Layer boundaries, data flow, threading | [docs/architecture.md](docs/architecture.md) |
| Prerequisites, run and test commands, adding a page or migration | [docs/development.md](docs/development.md) |
| Recording, sources, capture lifecycle | [docs/recording.md](docs/recording.md) |
| Native cgo capture, preview, encoding | [docs/native-capture.md](docs/native-capture.md) |
| In-window playback | [docs/native-playback.md](docs/native-playback.md) |
| ASR, segmentation, live subtitles | [docs/transcription.md](docs/transcription.md) |
| sherpa-onnx C shim | [docs/asr-cgo.md](docs/asr-cgo.md) |
| Translation providers, alignment, export | [docs/translation.md](docs/translation.md) |
| Summarization, structured output | [docs/summary.md](docs/summary.md) |
| Model discovery, validation, packaging | [docs/models.md](docs/models.md) |
| Schema, migrations, cascades | [docs/storage.md](docs/storage.md) |
| A screen, its tokens or its Go file | [docs/design/README.md](docs/design/README.md) |
| What is not implemented yet | [docs/ROADMAP.md](docs/ROADMAP.md) |
| Branch, commit and PR conventions | [CONTRIBUTING.md](CONTRIBUTING.md) |
| The whole docs tree | [docs/README.md](docs/README.md) |

## Repository map

`cmd/studio` is the composition root, `cmd/setup` the packaging helper, `internal/native`
the window (App state, pages, theme, i18n, player, `kit/`) and the other `internal/*`
packages the services; `migrations/` holds the SQL, `resources/<plat>/` the libraries.
The full tree is in [docs/development.md](docs/development.md).

## Doop is the source of truth: change the UI there first

Every screen lives in a shared Doop canvas. **A UI change starts in Doop, never in Go**:
edit the frame, check it with `get_frame_screenshot`, regenerate `docs/design/pages` and
`docs/design/screens` with
`DOOP_CLIENT="python3 <path-to>/doop.py" node .agents/skills/doop-canvas/scripts/build.js --shots`,
then mirror the frame in `internal/native`, lay the result beside it with the `TestPreview`
command above, and only then update prose docs. Client details, prototype wiring and the
frame-size table: [doop-canvas skill](.agents/skills/doop-canvas/SKILL.md); a full visual
overhaul: [redesign skill](.agents/skills/ottr-redesign/SKILL.md).

A screen change without its Doop frame and regenerated screenshot is incomplete.

## Branches and commits

- Header: `<type>(<scope>): <subject>` (`feat|fix|perf|docs|refactor|test|build|ci|chore`)
  or an imperative sentence; at most 72 characters, no trailing period.
- The body says *why*, names the measurement or the failing case, and keeps `Refs #N`.
- One goal per commit. A behavior change never hides under `chore` or a reformat.

## Evidence

Put this in the commit body and the PR; a merge at an unreviewed head starts over.

```
Ran at <pushed sha>:
- Red check: <test> failed before with <output>; passes after.
- Break check: <mutation> made <test> red.
- Full suite: go test ./... locally -> <summary>, or the CI run at <sha>.
- Real use: <what was opened, recorded, transcribed>.
- Not run: <check> (<why>).
Docs: <owning doc updated>.
```

Match the evidence to the change: a red test for logic, `OTTR_PREVIEW` before/after for
layout, the Doop frame for a screen, a real recording or model run for capture and ASR, the
request pair for a network path, a measurement for a performance claim. Full matrix in
[code-standards.md](code-standards.md).

## Agent safety

- Kill only a PID you captured at spawn, and stop what you started; leave the user's
  running Ottr alone. Never `pkill -f`, `pgrep | kill`, or a PID found by matching a name.
- Never write to, clean or start a server against `%APPDATA%\Ottr\`. Copy what you need
  into a temp directory, one way.
- Recordings, models and the database are user data: a test uses a temp DB and a temp
  recording directory, and never the live ones.
- Do not print tokens, API keys or personal paths, and do not commit them.

If a rule here fights the task in front of you, say so and get the owner's sign-off
before breaking it.

## Human gates

Stop and ask before: reversing a documented design decision or a rule in
`code-standards.md`; changing CI, the release path, permissions or secrets; changing the
branch model or the commit lint; accepting a new model family or a new API dependency;
touching real credentials or a live user database; an irreversible migration or deletion;
deleting or renaming a Doop frame; spending money, contacting users or making a public
claim.
