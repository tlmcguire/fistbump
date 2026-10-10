-- Adds 'Rejected' to application statuses, for tracking where the user was not offered a role.
-- SQLite cannot alter a CHECK constraint, so the table is rebuilt with the same columns, keys and
-- indexes, and every row is copied.
CREATE TABLE applications_new (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id              INTEGER NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
  profile_id          INTEGER NOT NULL REFERENCES profile(id) ON DELETE CASCADE,
  base_resume_id      INTEGER REFERENCES resumes(id) ON DELETE SET NULL,
  tailored_resume_id  INTEGER REFERENCES tailored_resumes(id) ON DELETE SET NULL,
  status              TEXT NOT NULL DEFAULT 'Saved'
                      CHECK (status IN ('Saved','Applied','Interviewing','Offered','Rejected','Archived')),
  date_applied        TEXT,
  next_step_date      TEXT,
  exported_pdf_path   TEXT,
  notes               TEXT,
  created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
INSERT INTO applications_new (id, job_id, profile_id, base_resume_id, tailored_resume_id, status, date_applied, next_step_date, exported_pdf_path, notes, created_at, updated_at)
  SELECT id, job_id, profile_id, base_resume_id, tailored_resume_id, status, date_applied, next_step_date, exported_pdf_path, notes, created_at, updated_at FROM applications;
DROP TABLE applications;
ALTER TABLE applications_new RENAME TO applications;
CREATE INDEX idx_applications_status ON applications(status);
CREATE INDEX idx_applications_job    ON applications(job_id);
