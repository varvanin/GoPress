CREATE TABLE forms (
 id INTEGER PRIMARY KEY, version INTEGER NOT NULL DEFAULT 1,
 name TEXT NOT NULL, recipient TEXT NOT NULL, button TEXT NOT NULL, success TEXT NOT NULL,
 enabled INTEGER NOT NULL DEFAULT 1, fields TEXT NOT NULL CHECK(json_valid(fields))
);
CREATE TABLE form_entries (
 id INTEGER PRIMARY KEY, form_id INTEGER NOT NULL REFERENCES forms(id),
 created_at INTEGER NOT NULL DEFAULT(unixepoch()), values_json TEXT NOT NULL CHECK(json_valid(values_json)),
 mail_status TEXT NOT NULL DEFAULT 'pending'
);
CREATE INDEX form_entries_form ON form_entries(form_id,id DESC);
PRAGMA user_version=7;
