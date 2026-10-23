# Storage

SQLite through `modernc.org/sqlite` (pure Go, no cgo), one writer connection
(`SetMaxOpenConns(1)`) to keep writes serialized.

Location: `<userData>/ottr.db`, where `<userData>` is
`mygo.App.Path(mygo.PathUserData)` — `%APPDATA%\Ottr` on Windows,
`~/Library/Application Support/Ottr` on macOS, `~/.local/share/ottr` on Linux.

PRAGMAs: `journal_mode=wal`, `foreign_keys=on`. Foreign keys are enforced, which
matters because every child table cascades from `media`.

## Migrations

Files in `migrations/*.sql` are embedded into the binary and applied in filename
order. Applied versions are recorded in `schema_migrations(version)` inside the
same transaction as the migration body, so a failed migration leaves no partial
state and is retried on next start.

| File | Creates |
|---|---|
| `0001_init.sql` | `media` |
| `0002_transcript.sql` | `transcript`, `segment` |
| `0003_translation.sql` | `translation`, `translation_segment` |
| `0004_summary.sql` | `summary` |

`TestAllTablesExist` asserts all seven tables (including
`schema_migrations`) exist after a fresh `Open`.

There is also a small `settings` table created lazily by the translation and
summary services (`key TEXT PRIMARY KEY, value TEXT`) holding provider
configuration. It is not part of the migration chain yet — see ROADMAP.

## Entities

### media

One row per file in the knowledge base.

| Column | Notes |
|---|---|
| `id` | autoincrement |
| `name` | display name, filename without extension |
| `source` | `import` or `recording` |
| `path` | absolute path to the media file |
| `duration_ms`, `width`, `height`, `size` | from ffprobe |
| `has_audio`, `has_video` | from ffprobe stream types |
| `created_at` | RFC 3339 UTC |
| `transcript_status` | `none`, `pending`, `done`, `error` |
| `summary_status` | same vocabulary |

Media bytes are **never** stored in SQLite; only the path.

### transcript / segment

`transcript(media_id, language, created_at)`;
`segment(transcript_id, idx, start_ms, end_ms, text)`.
Indexed by `transcript_id`; cascade delete from `media`.

### translation / translation_segment

`translation(media_id, target_lang, source_lang, created_at)` with
`UNIQUE(media_id, target_lang)`;
`translation_segment(translation_id, segment_id, idx, start_ms, end_ms,
original_text, translated_text)`. Originals and translations live in separate
rows/tables, joined by `segment_id` and by identical timestamps.

### summary

One row per media with `UNIQUE(media_id)`, holding both structured JSON columns
and the rendered `content_markdown`.

## Deletion semantics

`Library.Remove` deliberately distinguishes two operations:

1. Always: delete the `media` row (all dependent rows cascade).
2. Only if the user checks **"Also delete the file from disk"**: `os.Remove` the
   media file.

The checkbox is unchecked by default; a plain removal never destroys data
irrecoverably. The dialog is a native confirmation, so the destructive path
always requires an explicit second action.

## Testing

- `TestOpenMigrates` — migrations apply, `media` is usable, reopening the same
  file does not re-apply.
- `TestAllTablesExist` — schema completeness.
- Per-service tests exercise real SQL against real temporary databases.
