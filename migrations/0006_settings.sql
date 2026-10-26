-- Application settings, one row per key: provider configuration for
-- translation and summaries, and which ASR model to use. Created here rather
-- than lazily by each service that reads a key.
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
