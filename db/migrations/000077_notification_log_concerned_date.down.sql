-- Restores nullable dedup columns; backfilled values and deleted duplicates
-- are not restored.
DROP INDEX IF EXISTS idx_notification_log_dedup;

ALTER TABLE notification_log
    DROP COLUMN IF EXISTS skipped,
    ALTER COLUMN reference_id DROP NOT NULL,
    ALTER COLUMN days_before_expiry DROP NOT NULL,
    ALTER COLUMN expiry_reference_date DROP NOT NULL;

CREATE UNIQUE INDEX idx_notification_log_dedup
    ON notification_log(user_id, notification_type, reference_id, days_before_expiry, expiry_reference_date);
