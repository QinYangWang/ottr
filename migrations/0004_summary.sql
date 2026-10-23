CREATE TABLE summary (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    content_markdown TEXT NOT NULL,
    overview TEXT NOT NULL,
    key_points_json TEXT NOT NULL DEFAULT '[]',
    action_items_json TEXT NOT NULL DEFAULT '[]',
    chapters_json TEXT NOT NULL DEFAULT '[]',
    highlights_json TEXT NOT NULL DEFAULT '[]',
    model TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(media_id)
);
CREATE INDEX idx_summary_media ON summary(media_id);
