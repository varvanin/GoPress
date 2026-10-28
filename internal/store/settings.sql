CREATE TABLE settings(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL DEFAULT 0,value TEXT NOT NULL CHECK(json_valid(value)));
INSERT INTO settings VALUES(1,0,'{"language":"ru","home_mode":"posts","posts_per_page":12}');
PRAGMA user_version=8;
