-- A sample master resume for manual testing and later phases:
--   sqlite3 "$DATA_DIR/fistbump.db" < backend/testdata/seed.sql
-- Run it on a migrated database with no profile.
INSERT INTO profile (full_name, email, phone, home_location, target_positions, preferred_mode, summary, skills, links)
VALUES ('Ada Lovelace', 'ada@example.com', '555-0100', 'Charleston, SC', '["Data Analyst"]', 'Hybrid',
        'Analyst with five years of experience turning messy data into decisions.',
        '["sql","excel","tableau"]', '["github.com/ada"]');

INSERT INTO experiences (profile_id, company_name, job_title, start_date, end_date, location, description, impact, skills, sort_order) VALUES
  (1, 'Acme Analytics', 'Senior Data Analyst', '2022-06-01', NULL, 'Charleston, SC',
   'Own the weekly finance reporting pipeline.', 'Cut reporting time by 40%.', '["sql","python","tableau"]', 1),
  (1, 'Beta Health', 'Data Analyst', '2020-01-01', '2022-05-01', 'Remote',
   'Built dashboards for clinic operations.', 'Found $120k of annual savings in supply orders.', '["sql","excel"]', 2),
  (1, 'Coastal University', 'Research Assistant', '2018-09-01', '2019-12-01', 'Charleston, SC',
   'Cleaned and analyzed survey data for a public health study.', NULL, '["r","statistics"]', 3);

INSERT INTO education (profile_id, institution, degree_level, discipline, graduation_date, gpa, honors, sort_order) VALUES
  (1, 'College of Charleston', 'BS', 'Computer Science', '2019-05-01', 3.8, 'Magna cum laude', 1),
  (1, 'Trident Technical College', 'AS', 'General Studies', '2017-05-01', NULL, NULL, 2);
