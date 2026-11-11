#!/usr/bin/env bash
# Local gates for Ottr. Run it from anywhere; it finds the repo root itself.
# The full suite also runs in CI (.github/workflows/build.yml, windows-latest):
# if you rely on that run, put its sha in the commit body instead of this script.
# Do not fake a line here. A gate that could not run prints NOT RUN and is
# repeated in the commit body.
set -euo pipefail
cd "$(dirname "$0")/.."

section() { printf '\n== %s\n' "$*"; }
not_run() { printf 'NOT RUN: %s\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

# --- commit convention -------------------------------------------------------
# Header: <type>(<scope>): <subject> or an imperative sentence; <=72 chars, no
# trailing period. Merge commits are skipped.
section "commit convention"
range="${RANGE:-}"
if [ -z "$range" ]; then
  if git rev-parse --verify -q origin/main >/dev/null; then range="origin/main..HEAD"
  else range="HEAD~1..HEAD"; fi
fi
printf 'range: %s\n' "$range"
bad=0
while IFS= read -r subj; do
  case "$subj" in
    Merge\ *|"") continue ;;
  esac
  if [ "${#subj}" -gt 72 ]; then printf 'long header (%d): %s\n' "${#subj}" "$subj"; bad=1; fi
  case "$subj" in
    *.) printf 'trailing period: %s\n' "$subj"; bad=1 ;;
  esac
  if ! printf '%s' "$subj" | grep -Eq \
      '^(feat|fix|perf|docs|refactor|test|build|ci|chore|style|revert)(\([a-z0-9._-]+\))?!?: [A-Za-z]|^[A-Z]'; then
    printf 'header: %s\n' "$subj"; bad=1
  fi
done < <(git log --format=%s "$range")
[ "$bad" = 0 ] || fail "commit headers do not follow the convention"

# --- format ------------------------------------------------------------------
# The checkout is CRLF (core.autocrlf=true), so gofmt is compared against the
# file with \r stripped, not against the file on disk.
section "format (gofmt)"
tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT
unformatted=0
while IFS= read -r f; do
  sed 's/\r$//' "$f" > "$tmp"
  if [ -n "$(gofmt -d "$tmp")" ]; then
    printf 'unformatted: %s\n' "$f"; unformatted=1
  fi
done < <(git ls-files '*.go')
[ "$unformatted" = 0 ] || fail "gofmt"

# --- fast check --------------------------------------------------------------
section "vet"
go vet ./...

# --- tests beside the change -------------------------------------------------
section "tests beside the change"
pkgs=$(git diff --name-only "$range" -- '*.go' 2>/dev/null | sed -e 's|/[^/]*$||' -e 's|^|./|' | sort -u)
if [ -n "$pkgs" ]; then
  printf '%s\n' "$pkgs"
  CGO_ENABLED=1 go test $pkgs
else
  not_run "no changed Go package in $range"
fi

# --- full suite --------------------------------------------------------------
section "full suite"
CGO_ENABLED=1 go test ./...

# --- real use ----------------------------------------------------------------
# The window opens on a display; this machine may have none. The preview test
# writes the real layouts to PNGs, which is the closest automated real-use check.
section "real use (window layouts)"
if [ -n "${OTTR_PREVIEW:-}" ]; then
  OTTR_PREVIEW="$OTTR_PREVIEW" go test ./internal/native -run TestPreview
else
  not_run "window layouts (set OTTR_PREVIEW=<dir> to render them; a human opens the app for capture and ASR)"
fi

# --- not run -----------------------------------------------------------------
section "not run by this script"
not_run "recording and ASR on real hardware (needs a Windows desktop, a device and a model; run the app)"
not_run "the Doop canvas sync (needs a Doop MCP client: node .agents/skills/doop-canvas/scripts/build.js --shots)"
not_run "CI (runs go test ./... on windows-latest at the pushed head)"

section "summary"
printf 'Gates passed. Record where the full suite ran in the commit body.\n'
