CREATE TABLE module_packages (
 id TEXT PRIMARY KEY, version INTEGER NOT NULL DEFAULT 1, enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled IN (0,1)),
 manifest TEXT NOT NULL CHECK(json_valid(manifest)), template TEXT NOT NULL, css TEXT NOT NULL DEFAULT '',
 settings TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(settings)), screenshot BLOB, screenshot_mime TEXT NOT NULL DEFAULT ''
);
PRAGMA user_version=14;
