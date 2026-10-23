CREATE TABLE translation (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    target_lang TEXT NOT NULL,
    source_lang TEXT NOT NULL DEFAULT 'zh',
    created_at TEXT NOT NULL,
    UNIQUE(media_id, target_lang)
);
CREATE INDEX idx_translation_media ON translation(media_id);

CREATE TABLE translation_segment (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    translation_id INTEGER NOT NULL REFERENCES translation(id) ON DELETE CASCADE,
    segment_id INTEGER NOT NULL REFERENCES segment(id) ON DELETE CASCADE,
    idx INTEGER NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms INTEGER NOT NULL,
    original_text TEXT NOT NULL,
    translated_text TEXT NOT NULL
);
CREATE INDEX idx_translation_segment ON translation_segment(translation_id);
