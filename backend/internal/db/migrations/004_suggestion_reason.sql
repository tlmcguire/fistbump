-- A short plain-language reason for each suggestion, written by the engine in the user's language.
ALTER TABLE suggestions ADD COLUMN reason TEXT NOT NULL DEFAULT '';
