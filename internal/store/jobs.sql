ALTER TABLE form_entries ADD COLUMN submission_key TEXT;
ALTER TABLE form_entries ADD COLUMN mail_attempts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE form_entries ADD COLUMN mail_next_at INTEGER NOT NULL DEFAULT 0;
ALTER TABLE form_entries ADD COLUMN mail_claim_until INTEGER NOT NULL DEFAULT 0;
CREATE UNIQUE INDEX form_entry_submission ON form_entries(submission_key) WHERE submission_key IS NOT NULL;
CREATE INDEX form_entry_queue ON form_entries(mail_status,mail_next_at,mail_claim_until);
UPDATE form_entries SET mail_status='retry' WHERE mail_status='failed';
UPDATE settings SET value=json_set(value,'$.form_retention_days',90,'$.audit_retention_days',365);
PRAGMA user_version=12;
