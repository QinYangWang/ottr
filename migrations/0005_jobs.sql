CREATE TABLE job (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    kind TEXT NOT NULL, -- transcribe | translate | summarize
    state TEXT NOT NULL, -- running | done | failed | cancelled
    progress REAL NOT NULL DEFAULT 0, -- 0..1
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_job_media ON job(media_id, id DESC);
