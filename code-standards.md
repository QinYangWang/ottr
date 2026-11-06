# Code standards

These are the rules a change in Ottr is held to. They describe the code that already
exists; when in doubt, match the surrounding file.

## Go

- **Formatting:** `gofmt` output only. `go vet ./...` is clean. No generated code without
  a matching generator in the repo.
- **Dependencies:** the standard library first, then MyGo, then what is already in
  `go.mod`. A new dependency needs a reason in the PR; never add one for a few lines.
- **Errors:** return them, wrap them (`fmt.Errorf("describe: %w", err)`), keep messages
  lowercase and without trailing punctuation. No `panic` in service or UI code. A user
  sees an error through `App.setError`, never a stack trace.
- **Packages:** one job per package. `internal/native` draws and holds view state; it must
  not open SQL, call FFmpeg or touch WASAPI. Services must not import `internal/native`.
- **Coupling:** communicate through small interfaces (`capture.Backend`,
  `asr.LiveWriter`) and plain structs. Do not reach across packages for unexported state.
- **Concurrency:** services return values; the view mutates only on its own frame, via
  `App.update`. Goroutines get a `context` when they do I/O. Shared maps/fields need a
  mutex or ownership by one goroutine — do not mix.
- **Platform code:** split with build tags (`*_windows.go` / `*_other.go`). Keep cgo in
  `internal/asr/native` and `internal/capture`; no Go pointers retained by C.
- **SQL:** embedded migrations, append-only, parameterized queries. Never format SQL with
  values. Timestamps are UTC RFC 3339 strings.
- **Comments:** explain *why*, one or two lines, on the declaration that needs it. No
  commented-out code, no change logs in comments, no banner comments around trivial code.
- **Naming:** short, domain words (`seg`, `trLang`, `liveWarn`). Exported identifiers get
  a doc comment that starts with the name.
- **Logging:** the window surfaces state; do not add ad-hoc prints to shipped paths.

## UI (`internal/native`)

- **Theme tokens only:** colors, radii and shadows come from `ui.Theme` or `theme.go`
  helpers (`popoverShadow`, `cardShadow`, `mediaBackdrop`, `onMedia`). No raw hex in pages.
- **Rhythm:** spacing is `t.Space(n)` (4 px units); controls use the 8 px radius, media
  frames 16 px, popovers the extra-large shadow. Reuse `kit/` before writing a new control.
- **State:** fields live on `App`; pages are `func (a *App) xPage(c *ui.Context, t *ui.Theme)`.
  Long-running work goes through `jobs` + `a.update`.
- **Strings:** every label, tooltip and empty state goes through `a.tr` / `a.trf`, with an
  entry in `i18n.go` for English and Chinese. Model/provider names stay untranslated.
- **Icons:** Lucide paths via `icons.go`, 18 px default, `currentColor`; do not inline a
  one-off SVG path in a page.
- **States:** a control that can fail shows loading, empty and error states. Anything
  clickable must do something; a disabled control says why in its tooltip.
- **Accessibility:** icon-only buttons have a label and tooltip; hit areas stay at least
  32 px; text contrast follows the design tokens.
- **Layout:** check 1280×800, 900×620 and 420×800. The library panel appears at ≥ 840 DIP;
  below that the panes stack.

## Design

- **The Doop canvas is the source of truth.** Change the frame there, confirm it with
  `get_frame_screenshot`, regenerate `docs/design` with
  `node .agents/skills/doop-canvas/scripts/build.js --shots`, and only then write the Go
  that mirrors it. A screen change that starts in Go is rejected.
- Snapshots, tokens and component examples live in `docs/design`; keep the mapping table
  there accurate when a screen moves between files.
- Do not invent identities or fake data that looks real: speakers are `Speaker N`,
  sample names are clearly placeholders.
- Any change to a screen updates `docs/design/pages` and `docs/design/screens`.

## Tests

- Tests assert behavior, not implementation details. Prefer table tests for parsing and
  formatting, real databases for services, `ui.NewTester` for the window.
- External dependencies (ffmpeg, model files, capture devices, network) call `t.Skip` when
  missing. Fakes are allowed only for the *client* side of an HTTP call, never for the
  thing under test.
- Opt-in suites use environment variables (`OTTR_PREVIEW`, `OTTR_TEST_VIDEO`,
  `OTTR_TEST_OFFLINE_MODEL`) and skip silently without them.
- A bug fix gets a regression test that fails before the fix.

## Acceptance criteria for every PR

1. `CGO_ENABLED=1 go build ./...`, `go vet ./...`, `go test ./...` pass.
2. The change is exercised the way a user would: a screenshot for UI, a real recording or
   a real file for capture/ASR, a real request for network paths.
3. New branches, loops, parsers or state machines have a test; the PR states which.
4. New user-facing text exists in both locales.
5. Documentation is updated when paths, commands, schema or behavior change.
6. No webview, HTML or JavaScript enters the shipped app.
7. No secrets, machine-specific paths or credentials in code, tests or fixtures.
8. Migrations are new files; existing ones are untouched.
9. The diff is scoped to the issue: no unrelated refactors, reformatting or dependency
   churn.
10. Design and code agree, and a screen change carries its Doop frame plus the regenerated
    `docs/design` screenshot — if they do not agree, the PR says which one will follow.
