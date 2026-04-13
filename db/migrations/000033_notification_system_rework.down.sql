-- Revert Phase 6d: Notification System Rework

-- Step 1: Drop new dedup index
DROP INDEX IF EXISTS idx_notification_log_dedup;

-- Step 2: Remove new columns from notification_log
ALTER TABLE notification_log
    DROP COLUMN IF EXISTS subject,
    DROP COLUMN IF EXISTS expiry_reference_date;

-- Step 3: Recreate old dedup index
CREATE INDEX idx_notification_log_dedup ON notification_log(user_id, notification_type, reference_id, days_before_expiry);

-- Step 4: Add back old boolean columns with defaults
ALTER TABLE notification_preferences
    ADD COLUMN currency_warnings BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN credential_warnings BOOLEAN NOT NULL DEFAULT true;

-- Step 5: Migrate data back — if all currency categories are present, set true
UPDATE notification_preferences
SET currency_warnings = (
    enabled_categories @> ARRAY['currency_passenger', 'rating_expiry']
);

UPDATE notification_preferences
SET credential_warnings = (
    enabled_categories @> ARRAY['credential_medical']
);

-- Step 6: Drop new columns
ALTER TABLE notification_preferences
    DROP COLUMN IF EXISTS check_hour,
    DROP COLUMN IF EXISTS enabled_categories;
