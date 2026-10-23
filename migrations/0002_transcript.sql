CREATE TABLE transcript (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    language TEXT NOT NULL DEFAULT 'zh',
    created_at TEXT NOT NULL
);
CREATE INDEX idx_transcript_media ON transcript(media_id);

CREATE TABLE segment (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    transcript_id INTEGER NOT NULL REFERENCES transcript(id) ON DELETE CASCADE,
    idx INTEGER NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms INTEGER NOT NULL,
    text TEXT NOT NULL
);
CREATE INDEX idx_segment_transcript ON segment(transcript_id);
