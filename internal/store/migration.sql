CREATE TABLE installation (
 id INTEGER PRIMARY KEY CHECK(id=1), title TEXT NOT NULL, base_url TEXT NOT NULL,
 timezone TEXT NOT NULL, installed_at INTEGER NOT NULL
);
CREATE TABLE users (id INTEGER PRIMARY KEY CHECK(id=1), email TEXT NOT NULL UNIQUE, password_hash BLOB NOT NULL);
CREATE TABLE sessions (hash TEXT PRIMARY KEY, expires_at INTEGER NOT NULL);
CREATE TABLE media (name TEXT PRIMARY KEY, mime TEXT NOT NULL, size INTEGER NOT NULL CHECK(size>0));
CREATE TABLE pages (
 id INTEGER PRIMARY KEY, slug TEXT NOT NULL UNIQUE, version INTEGER NOT NULL CHECK(version>0),
 title TEXT NOT NULL, document TEXT NOT NULL CHECK(json_valid(document)), cover TEXT REFERENCES media(name),
 published_slug TEXT UNIQUE, published_title TEXT, published_document TEXT CHECK(published_document IS NULL OR json_valid(published_document)),
 published_cover TEXT REFERENCES media(name)
);
CREATE TABLE routes (slug TEXT PRIMARY KEY, page_id INTEGER NOT NULL REFERENCES pages(id));
PRAGMA user_version=1;
