-- Archived jobs are hidden from lists and pickers but keep their revisions, drafts and applications.
ALTER TABLE jobs ADD COLUMN archived_at TEXT;
CREATE INDEX idx_jobs_archived ON jobs(archived_at);
