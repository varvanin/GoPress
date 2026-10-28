CREATE TABLE IF NOT EXISTS media_details (
 name TEXT PRIMARY KEY REFERENCES media(name) ON DELETE CASCADE,
 title TEXT NOT NULL DEFAULT '',
 alt TEXT NOT NULL DEFAULT '',
 caption TEXT NOT NULL DEFAULT '',
 created_at INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE TABLE IF NOT EXISTS menus (
 position TEXT PRIMARY KEY CHECK(position IN ('header','footer')),
 version INTEGER NOT NULL DEFAULT 0,
 items TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(items))
);
INSERT OR IGNORE INTO menus(position) VALUES('header'),('footer');
PRAGMA user_version=5;
