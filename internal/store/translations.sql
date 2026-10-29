ALTER TABLE pages ADD COLUMN translations TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(translations));
ALTER TABLE pages ADD COLUMN published_translations TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(published_translations));
ALTER TABLE revisions ADD COLUMN translations TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(translations));
PRAGMA user_version=16;
