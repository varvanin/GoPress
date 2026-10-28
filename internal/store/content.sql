ALTER TABLE pages ADD COLUMN created_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pages ADD COLUMN updated_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pages ADD COLUMN published_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE pages ADD COLUMN trashed_at INTEGER NOT NULL DEFAULT 0;
UPDATE pages SET created_at=unixepoch(),updated_at=unixepoch(),published_at=CASE WHEN published_slug IS NOT NULL THEN unixepoch() ELSE 0 END;
CREATE TABLE revisions (
 page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
 version INTEGER NOT NULL, slug TEXT NOT NULL, title TEXT NOT NULL,
 document TEXT NOT NULL CHECK(json_valid(document)), cover TEXT REFERENCES media(name),
 created_at INTEGER NOT NULL, PRIMARY KEY(page_id,version)
);
INSERT INTO revisions SELECT id,version,slug,title,document,cover,updated_at FROM pages;
CREATE INDEX pages_content ON pages(kind,trashed_at,id DESC);
PRAGMA user_version=3;
