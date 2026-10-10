-- Links and handles from the user's resume header (LinkedIn, GitHub, portfolio). Kept as written.
ALTER TABLE profile ADD COLUMN links TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(links));
