BEGIN;

-- Explicitly remove the audit-log feature and its persisted data as requested.
DROP TABLE IF EXISTS audit_events;

COMMIT;
