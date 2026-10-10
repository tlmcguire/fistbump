-- A professional summary on the master resume. Imports and the resume generator fill it.
ALTER TABLE profile ADD COLUMN summary TEXT;

-- Greenhouse is on by default. It still makes network requests only when the user searches or imports.
UPDATE settings SET value = 'true' WHERE key = 'connectors.greenhouse.enabled';
INSERT OR IGNORE INTO settings (key, value) VALUES ('connectors.greenhouse.enabled', 'true');
