CREATE TABLE modules_next (id TEXT PRIMARY KEY CHECK(id IN ('faq','gallery','testimonials')), enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)));
INSERT INTO modules_next SELECT id,enabled FROM modules;
INSERT INTO modules_next(id) VALUES('gallery'),('testimonials');
DROP TABLE modules;
ALTER TABLE modules_next RENAME TO modules;
CREATE TABLE module_collections (
 id INTEGER PRIMARY KEY,kind TEXT NOT NULL CHECK(kind IN ('gallery','testimonials')),
 version INTEGER NOT NULL DEFAULT 1,name TEXT NOT NULL,layout TEXT NOT NULL CHECK(layout IN ('grid','list')),
 items TEXT NOT NULL CHECK(json_valid(items))
);
CREATE TABLE module_collection_media (
 collection_id INTEGER NOT NULL REFERENCES module_collections(id) ON DELETE CASCADE,
 media_name TEXT NOT NULL REFERENCES media(name),PRIMARY KEY(collection_id,media_name)
);
CREATE INDEX module_collection_media_name ON module_collection_media(media_name);
PRAGMA user_version=15;
