ALTER TABLE users RENAME TO initial_user;
CREATE TABLE users (
 id INTEGER PRIMARY KEY, email TEXT NOT NULL COLLATE NOCASE UNIQUE, password_hash BLOB NOT NULL,
 name TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT 'admin' CHECK(role IN ('admin','editor','author')),
 blocked INTEGER NOT NULL DEFAULT 0 CHECK(blocked IN (0,1)), version INTEGER NOT NULL DEFAULT 1,
 created_at INTEGER NOT NULL DEFAULT(unixepoch())
);
INSERT INTO users(id,email,password_hash) SELECT id,email,password_hash FROM initial_user;
DROP TABLE initial_user;
ALTER TABLE sessions ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;
CREATE INDEX sessions_user ON sessions(user_id);
CREATE TABLE password_resets(hash TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id),expires_at INTEGER NOT NULL);
ALTER TABLE pages ADD COLUMN owner_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE media ADD COLUMN owner_id INTEGER NOT NULL DEFAULT 1;
CREATE INDEX pages_owner ON pages(owner_id,kind,trashed_at,id DESC);
CREATE TABLE audit_events(id INTEGER PRIMARY KEY,at INTEGER NOT NULL DEFAULT(unixepoch()),user_id INTEGER NOT NULL DEFAULT 0,action TEXT NOT NULL,object TEXT NOT NULL DEFAULT '');
CREATE INDEX audit_at ON audit_events(at,id);
PRAGMA user_version=9;
