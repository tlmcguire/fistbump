CREATE TABLE IF NOT EXISTS saved_searches (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    name           TEXT NOT NULL,
    query_json     TEXT NOT NULL,
    last_run_at    TEXT,
    seen_keys_json TEXT NOT NULL DEFAULT '[]',
    created_at     TEXT NOT NULL
);
