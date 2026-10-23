CREATE TABLE media (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    source TEXT NOT NULL, -- import | recording
    path TEXT NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    size INTEGER NOT NULL DEFAULT 0,
    has_audio INTEGER NOT NULL DEFAULT 0,
    has_video INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    transcript_status TEXT NOT NULL DEFAULT 'none', -- none | pending | done | error
    summary_status TEXT NOT NULL DEFAULT 'none'
);
CREATE INDEX idx_media_created ON media(created_at DESC);
