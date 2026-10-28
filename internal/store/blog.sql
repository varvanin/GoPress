CREATE TABLE terms (
 id INTEGER PRIMARY KEY, kind TEXT NOT NULL CHECK(kind IN ('category','tag')),
 name TEXT NOT NULL, name_key TEXT NOT NULL, version INTEGER NOT NULL DEFAULT 1, deleted_at INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX terms_name ON terms(kind,name_key) WHERE deleted_at=0;
ALTER TABLE pages ADD COLUMN term_ids TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(term_ids));
ALTER TABLE pages ADD COLUMN published_term_ids TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(published_term_ids));
ALTER TABLE pages ADD COLUMN scheduled_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pages ADD COLUMN scheduled_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE revisions ADD COLUMN term_ids TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(term_ids));
CREATE INDEX pages_schedule ON pages(scheduled_at) WHERE scheduled_at>0;
CREATE INDEX pages_blog ON pages(published_at DESC,id DESC) WHERE kind='post' AND trashed_at=0 AND published_slug IS NOT NULL;
PRAGMA user_version=4;
