DROP TABLE IF EXISTS email_suppressions;
DROP TABLE IF EXISTS email_delivery_events;

DROP INDEX IF EXISTS idx_users_unverified_cleanup;

ALTER TABLE users
    DROP COLUMN IF EXISTS verification_reminder_sent_at;
