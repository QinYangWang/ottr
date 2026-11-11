# Contributing to Ottr

## Before you start

Read [AGENTS.md](AGENTS.md) for the map and [docs/development.md](docs/development.md) for
prerequisites. Install Go 1.27, FFmpeg, a C toolchain, and the sherpa-onnx C library if
you want transcription to work. Build and test with cgo:

```sh
CGO_ENABLED=1 go run ./cmd/studio
go vet ./...
go test ./...
bash scripts/verify.sh   # the same gates plus the commit-header lint
```

## Workflow

1. **Open an issue or comment on one** before larger work: capture backends, schema
   changes, new providers and UI restructuring all need agreement first.
2. **Branch** from `main`: `feat/live-translation`, `fix/seek-drift`, `docs/design-sync`.
3. **Keep the diff scoped.** No drive-by refactors, reformatting or dependency bumps in a
   feature PR.
4. **Design first, in Doop.** A visible change starts on the canvas (skill:
   `.agents/skills/doop-canvas/SKILL.md`), is verified with a frame screenshot, then synced
   with `build.js --shots`, then mirrored in `internal/native`. Go code is never the place a
   screen change starts.
5. **Test what you touched.** New branches, loops and parsers need a test at the level the
   codebase uses: services against a real temp database, the view through `ui.NewTester`,
   layout through the opt-in preview test. External dependencies are skipped, not faked.
6. **Update docs** when behavior, commands, paths or the schema change.

## Commits

- Subject in the imperative, ≤ 72 characters: `Add speaker tint to transcript rows`.
- One logical change per commit; the body explains *why*, not *what*.
- Reference the issue (`Refs #12`) when there is one.
- Never commit generated artifacts: `.doop/`, `.preview/`, `docs/design/screens` are
  committed on purpose, but `ottr.exe`, `dist/`, `bin/` and model folders are not.

## What a PR must include

The pull request template asks for the change kind, the base branch, the semantic change,
the cost, the evidence block and the rollback; its checklist is the short form of
[code-standards.md](code-standards.md):

- [ ] `go vet ./...` and `go test ./...` pass with `CGO_ENABLED=1`
- [ ] New behavior has a test, or the PR says why none is possible
- [ ] User-facing strings are in `internal/native/i18n.go` for both English and Chinese
- [ ] Visible UI changes started in the Doop canvas; `docs/design` is regenerated and the
      frame screenshot is in the description
- [ ] Dataset/schema changes come with a new migration file, never an edited one
- [ ] Documentation touched: `docs/`, `AGENTS.md` or `code-standards.md` as appropriate
- [ ] No secrets, tokens, API keys or personal paths in the diff

Reviewers check [code-standards.md](code-standards.md) — a PR that ignores it will be sent
back before a functional review.

## Reporting bugs

Include the OS build, whether ASR models and FFmpeg were found, the exact steps, and the
error text the window showed. Recording/ffmpeg failures are much easier to fix with the
command line Ottr logged (`%APPDATA%\Ottr\logs` when present) and the media probe result.

## Localization

Chinese is a first-class locale, not a translation afterthought. Add both strings in the
same commit. Model and provider names stay in English; interface labels are translated.

## Design content

The Doop canvas is shared and visible to others while you edit. Keep working frames on the
canvas, announce what you are doing with `set_status`, and leave prototype links intact.
Do not delete frames without asking.
