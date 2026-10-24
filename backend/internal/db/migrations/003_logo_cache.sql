-- Company logos fetched from each company's own website. found = 0 remembers that none was found.
-- Rows are disposable: the backend deletes rows older than a day at startup and every hour.
CREATE TABLE logo_cache (
  host         TEXT PRIMARY KEY,
  found        INTEGER NOT NULL CHECK (found IN (0,1)),
  content_type TEXT NOT NULL DEFAULT '',
  data         BLOB,
  fetched_at   TEXT NOT NULL
);

CREATE INDEX idx_logo_cache_fetched ON logo_cache(fetched_at);
