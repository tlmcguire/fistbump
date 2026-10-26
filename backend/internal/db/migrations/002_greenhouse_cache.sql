-- Greenhouse search cache. Rows are disposable: the backend deletes rows older than a day at startup,
-- and storage cleanup can empty both tables. Times are UTC RFC 3339, so text comparison orders them.
CREATE TABLE IF NOT EXISTS greenhouse_listing_cache (
  board      TEXT PRIMARY KEY,
  etag       TEXT NOT NULL DEFAULT '',
  not_found  INTEGER NOT NULL DEFAULT 0 CHECK (not_found IN (0,1)),
  jobs       TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(jobs)),
  fetched_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_greenhouse_listing_cache_fetched ON greenhouse_listing_cache(fetched_at);

CREATE TABLE IF NOT EXISTS greenhouse_description_cache (
  board      TEXT NOT NULL,
  job_id     TEXT NOT NULL,
  posting    TEXT NOT NULL CHECK (json_valid(posting)),
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (board, job_id)
);

CREATE INDEX IF NOT EXISTS idx_greenhouse_description_cache_fetched ON greenhouse_description_cache(fetched_at);
