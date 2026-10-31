CREATE TABLE IF NOT EXISTS localized_menus(position TEXT NOT NULL CHECK(position IN ('header','footer')),language TEXT NOT NULL,version INTEGER NOT NULL,items TEXT NOT NULL CHECK(json_valid(items)),PRIMARY KEY(position,language));
DROP TRIGGER IF EXISTS translations_search_insert;
DROP TRIGGER IF EXISTS translations_search_update;
DROP TRIGGER IF EXISTS translations_search_delete;
DROP TABLE IF EXISTS translation_search;
CREATE VIRTUAL TABLE translation_search USING fts5(page_id UNINDEXED,language UNINDEXED,title,body,tokenize='unicode61 remove_diacritics 2');
CREATE TRIGGER translations_search_insert AFTER INSERT ON pages WHEN NEW.published_slug IS NOT NULL AND NEW.trashed_at=0 BEGIN
 INSERT INTO translation_search(page_id,language,title,body) SELECT NEW.id,key,json_extract(value,'$.title'),(SELECT coalesce(group_concat(value,' '),'') FROM json_tree(json_extract(t.value,'$.document')) WHERE key='text' AND type='text') FROM json_each(NEW.published_translations) t;
END;
CREATE TRIGGER translations_search_update AFTER UPDATE OF published_translations,published_slug,trashed_at ON pages BEGIN
 DELETE FROM translation_search WHERE page_id=OLD.id;
 INSERT INTO translation_search(page_id,language,title,body) SELECT NEW.id,key,json_extract(value,'$.title'),(SELECT coalesce(group_concat(value,' '),'') FROM json_tree(json_extract(t.value,'$.document')) WHERE key='text' AND type='text') FROM json_each(NEW.published_translations) t WHERE NEW.published_slug IS NOT NULL AND NEW.trashed_at=0;
END;
CREATE TRIGGER translations_search_delete AFTER DELETE ON pages BEGIN DELETE FROM translation_search WHERE page_id=OLD.id; END;
INSERT INTO translation_search(page_id,language,title,body) SELECT p.id,t.key,json_extract(t.value,'$.title'),(SELECT coalesce(group_concat(value,' '),'') FROM json_tree(json_extract(t.value,'$.document')) WHERE key='text' AND type='text') FROM pages p,json_each(p.published_translations) t WHERE p.published_slug IS NOT NULL AND p.trashed_at=0;
CREATE TABLE IF NOT EXISTS media_images(name TEXT PRIMARY KEY REFERENCES media(name) ON DELETE CASCADE,width INTEGER NOT NULL CHECK(width>0),height INTEGER NOT NULL CHECK(height>0),variants TEXT NOT NULL CHECK(json_valid(variants)));
CREATE TABLE IF NOT EXISTS backup_schedule(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL DEFAULT 0,enabled INTEGER NOT NULL DEFAULT 0,hour INTEGER NOT NULL DEFAULT 3 CHECK(hour BETWEEN 0 AND 23),keep INTEGER NOT NULL DEFAULT 7 CHECK(keep BETWEEN 1 AND 30),attempts INTEGER NOT NULL DEFAULT 0,day TEXT NOT NULL DEFAULT '',last_attempt INTEGER NOT NULL DEFAULT 0,last_success INTEGER NOT NULL DEFAULT 0,last_id TEXT NOT NULL DEFAULT '',failed INTEGER NOT NULL DEFAULT 0);
INSERT OR IGNORE INTO backup_schedule(id) VALUES(1);
CREATE TABLE IF NOT EXISTS scheduled_backups(id TEXT PRIMARY KEY,created_at INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS audit_action ON audit_events(action);
CREATE TABLE IF NOT EXISTS user_security(user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,identity TEXT NOT NULL UNIQUE CHECK(length(identity)=32 AND identity NOT GLOB '*[^a-f0-9]*'),version INTEGER NOT NULL DEFAULT 0,enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN(0,1)),last_counter INTEGER NOT NULL DEFAULT -1);
CREATE TABLE IF NOT EXISTS user_recovery_codes(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,hash TEXT NOT NULL CHECK(length(hash)=64 AND hash NOT GLOB '*[^a-f0-9]*'),PRIMARY KEY(user_id,hash));
INSERT OR IGNORE INTO user_security(user_id,identity) SELECT id,lower(hex(randomblob(16))) FROM users;
CREATE TRIGGER IF NOT EXISTS users_security_insert AFTER INSERT ON users BEGIN INSERT INTO user_security(user_id,identity) VALUES(NEW.id,lower(hex(randomblob(16)))); END;
CREATE TABLE IF NOT EXISTS session_details(hash TEXT PRIMARY KEY REFERENCES sessions(hash) ON DELETE CASCADE,created_at INTEGER NOT NULL,user_agent TEXT NOT NULL DEFAULT '');
PRAGMA user_version=17;

CREATE TABLE manual_redirects (
 slug TEXT PRIMARY KEY,
 page_id INTEGER NOT NULL REFERENCES pages(id) ON DELETE CASCADE
);
CREATE TABLE missing_paths (
 slug TEXT PRIMARY KEY,
 hits INTEGER NOT NULL DEFAULT 1,
 last_seen INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE TABLE content_reviews (
 page_id INTEGER PRIMARY KEY REFERENCES pages(id) ON DELETE CASCADE,
 version INTEGER NOT NULL,
 requested_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 requested_at INTEGER NOT NULL DEFAULT (unixepoch()),
 comment TEXT NOT NULL DEFAULT ''
);
