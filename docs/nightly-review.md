# Nightly review

Once a day, after the day's work is merged: review what had to be fixed again, what
was verified too thinly and what merged that should have waited, then propose a diff to
[LESSONS.md](../LESSONS.md) — and to [code-standards.md](../code-standards.md) only when a
rule has been seen on three separate days.

Run it by hand for now. The range is the `reviewed-through` SHA at the top of
`LESSONS.md` (currently `25801e5`), rewritten at the end of a run. CI does not schedule
this: adding a workflow is an owner decision, so a scheduled session or a local cron is
the trigger.

## Inputs

- `git log --oneline <reviewed-through>..HEAD`, with each commit body;
- the merge commits, the PR descriptions and their review comments;
- the CI runs for that range, including cancelled and re-run attempts (`.github/workflows/build.yml`);
- the current `LESSONS.md` and `code-standards.md`, for deduplication.

## The three questions

1. **Did this have to be fixed again?** A later commit, revert or reopened report touching
   the same behavior. The lesson is the rule that would have caught it — reproduce the
   reporter's case, fix every sibling, re-derive the tests — never "be more careful".
2. **Should this have merged?** A mixed goal, a reversed owner decision, an unreviewed
   head, a documented contract changed without its doc, a known problem merged past.
3. **Was the verification too thin?** A test that passes on the parent commit, no break
   check, only chosen packages run, a faked ffmpeg or model, a platform claim only
   compiled, a green rerun hiding a red first attempt.

## Output

A PR, never a push to `main`. It carries:

- new `LESSONS.md` entries in the file's own format (rule, evidence, fix, `Seen N×` with
  the dates), or an added sighting on the rule that already covers it;
- promotions when a rule has three sightings on separate days: the shorter entry is
  removed from `LESSONS.md`, a reviewer-checkable row appears in `code-standards.md`, and
  the rule is listed under "Moved to the code standards" with the dates;
- `reviewed-through` advanced to the last commit reviewed;
- a PR body with the range, what was merged/closed as duplicate, what could not be
  verified, and the checks that were not run.

Invent nothing: every entry cites a commit, PR, CI run or owner decision. If the day had
no rework, say so and advance the SHA.

## Checking the loop

Monthly: take a change that had to be fixed twice and check whether the review caught it.
If it did not, the lesson was too vague or the gate is missing — fix that before adding
more rules. `LESSONS.md` stays under 800 lines, `code-standards.md` under 250; pruning a
rule happens in a commit that says why.
