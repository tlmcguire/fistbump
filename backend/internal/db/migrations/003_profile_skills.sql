-- General skills that belong to the person rather than one role (for example a resume's skills list).
ALTER TABLE profile ADD COLUMN skills TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(skills));
