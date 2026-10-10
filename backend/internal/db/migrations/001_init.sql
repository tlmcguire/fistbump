CREATE TABLE profile (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  full_name        TEXT NOT NULL,
  email            TEXT,
  phone            TEXT,
  home_location    TEXT,
  target_positions TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(target_positions)),
  preferred_mode   TEXT CHECK (preferred_mode IN ('Remote','Hybrid','On-site','Any')),
  min_desired_pay  INTEGER,
  clearance_certs  TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(clearance_certs)),
  summary          TEXT,
  skills           TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(skills)),
  links            TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(links)),
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  updated_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE experiences (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  profile_id   INTEGER NOT NULL REFERENCES profile(id) ON DELETE CASCADE,
  company_name TEXT NOT NULL,
  job_title    TEXT NOT NULL,
  start_date   TEXT,
  end_date     TEXT,
  location     TEXT,
  description  TEXT,
  impact       TEXT,
  skills       TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(skills)),
  sort_order   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE education (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  profile_id      INTEGER NOT NULL REFERENCES profile(id) ON DELETE CASCADE,
  institution     TEXT NOT NULL,
  degree_level    TEXT,
  discipline      TEXT,
  graduation_date TEXT,
  gpa             REAL,
  honors          TEXT,
  details         TEXT,
  sort_order      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE resumes (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  profile_id      INTEGER NOT NULL REFERENCES profile(id) ON DELETE CASCADE,
  version_label   TEXT NOT NULL,
  target_position TEXT,
  content         TEXT NOT NULL,
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE jobs (
  id               INTEGER PRIMARY KEY AUTOINCREMENT,
  company_name     TEXT NOT NULL,
  position_title   TEXT NOT NULL,
  listing_url      TEXT,
  source           TEXT NOT NULL DEFAULT 'pasted' CHECK (source IN ('pasted','manual','greenhouse')),
  external_id      TEXT,
  work_mode        TEXT,
  employment_type  TEXT,
  location         TEXT,
  pay_min          INTEGER,
  pay_max          INTEGER,
  description      TEXT,
  raw_text         TEXT,
  req_tech_skills  TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(req_tech_skills)),
  pref_tech_skills TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(pref_tech_skills)),
  req_soft_skills  TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(req_soft_skills)),
  pref_soft_skills TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(pref_soft_skills)),
  requirements     TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(requirements)),
  close_date       TEXT,
  created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  searched_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  archived_at      TEXT,
  trashed_at       TEXT
);
CREATE UNIQUE INDEX idx_jobs_source_external
  ON jobs(source, external_id) WHERE external_id IS NOT NULL;
CREATE INDEX idx_jobs_searched_at ON jobs(searched_at);
CREATE INDEX idx_jobs_archived    ON jobs(archived_at);

CREATE TABLE revisions (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id         INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  base_resume_id INTEGER REFERENCES resumes(id) ON DELETE SET NULL,
  engine         TEXT NOT NULL CHECK (engine IN ('local','remote','rules')),
  status         TEXT NOT NULL DEFAULT 'queued'
                 CHECK (status IN ('queued','running','done','failed','canceled')),
  error          TEXT,
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  completed_at   TEXT
);

CREATE TABLE suggestions (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  revision_id   INTEGER NOT NULL REFERENCES revisions(id) ON DELETE CASCADE,
  section       TEXT NOT NULL CHECK (section IN ('summary','experience','education','skills')),
  target_id     INTEGER,
  original_text TEXT NOT NULL,
  proposed_text TEXT NOT NULL,
  state         TEXT NOT NULL DEFAULT 'pending'
                CHECK (state IN ('pending','accepted','rejected','edited')),
  edited_text   TEXT
);
CREATE INDEX idx_suggestions_revision ON suggestions(revision_id);

CREATE TABLE tailored_resumes (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id         INTEGER NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
  revision_id    INTEGER REFERENCES revisions(id) ON DELETE SET NULL,
  base_resume_id INTEGER REFERENCES resumes(id) ON DELETE SET NULL,
  content        TEXT NOT NULL,
  base_content   TEXT,
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE applications (
  id                  INTEGER PRIMARY KEY AUTOINCREMENT,
  job_id              INTEGER NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
  profile_id          INTEGER NOT NULL REFERENCES profile(id) ON DELETE CASCADE,
  base_resume_id      INTEGER REFERENCES resumes(id) ON DELETE SET NULL,
  tailored_resume_id  INTEGER REFERENCES tailored_resumes(id) ON DELETE SET NULL,
  status              TEXT NOT NULL DEFAULT 'Saved'
                      CHECK (status IN ('Saved','Applied','Interviewing','Offered','Rejected')),
  date_applied        TEXT,
  next_step_date      TEXT,
  exported_pdf_path   TEXT,
  notes               TEXT,
  archived_at         TEXT,
  created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
  updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX idx_applications_status ON applications(status);
CREATE UNIQUE INDEX idx_applications_job ON applications(job_id);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

INSERT OR IGNORE INTO settings (key, value) VALUES
  ('ai.mode', 'auto'),
  ('ai.selected_model', ''),
  ('ai.idle_minutes', '5'),
  ('jobs.retention_days', '30'),
  ('jobs.trash_days', '7'),
  ('connectors.greenhouse.boards', '[]');
