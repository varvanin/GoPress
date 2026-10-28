CREATE TABLE modules (id TEXT PRIMARY KEY CHECK(id='faq'), enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)));
INSERT INTO modules(id) VALUES('faq');
CREATE TABLE faq_groups (id INTEGER PRIMARY KEY, version INTEGER NOT NULL DEFAULT 1, name TEXT NOT NULL, items TEXT NOT NULL CHECK(json_valid(items)));
PRAGMA user_version=13;
