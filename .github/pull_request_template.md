## Change kind

<!-- One of: fix / feature / refactor / perf / docs / design / dependency / release PR.
     A fix and a feature do not share a PR. -->

## Base branch

`main` (the only line; every change is a short-lived branch off it).

## Goal

<!-- The problem, in one paragraph. Link the issue: Refs #N / close #N. -->

## Semantic change

<!-- What behavior, interface, state or failure behavior changes. Name the owning doc you
     updated. "No semantic change" is a valid answer for a refactor, docs or a pin. -->

## Cost

<!-- What it makes slower, larger, more expensive or harder to run: new dependency, a
     network path, disk, CI minutes, another model download. "None" is an answer. -->

## Evidence

```
Ran at <pushed sha>:
- Red check: <test> failed before with <output>; passes after.
- Break check: <mutation> made <test> red.
- Full suite: go test ./... locally -> <summary>, or the CI run at <sha>.
- Real use: <what was opened, recorded, transcribed>.
- Not run: <check> (<why>).
Docs: <owning doc updated>.
```

Screenshots for a layout change (`OTTR_PREVIEW` before/after at 1280×800, 900×620,
420×800); the Doop frame screenshot for a screen change.

## Risk and rollback

<!-- What breaks if this is wrong, who sees it, and how it is undone. Migrations are
     append-only: say what an old binary does with the new schema. -->

## Checklist

- [ ] `bash scripts/verify.sh` passed, or the not-run lines are in the body
- [ ] New behavior has a test that fails on the parent commit, or the body says why not
- [ ] User-facing strings exist in `i18n.go` in English and Chinese
- [ ] A visible change started in the Doop canvas and `docs/design` is regenerated
- [ ] Schema changes are a new migration file; applied files are untouched
- [ ] The owning doc under `docs/` (or `code-standards.md`) is updated
- [ ] No secrets, tokens, API keys or personal paths in the diff

Full rules: [CONTRIBUTING.md](../CONTRIBUTING.md) and
[code-standards.md](../code-standards.md).
