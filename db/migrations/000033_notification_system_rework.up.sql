-- Phase 6d: Notification System Rework
-- Replace coarse currency_warnings/credential_warnings booleans with granular per-category control.
-- Add expiry-cycle-aware deduplication to prevent re-sending after renewal.
-- Add check_hour for daily scheduling instead of every-hour processing.

-- Step 1: Add new columns to notification_preferences
ALTER TABLE notification_preferences
    ADD COLUMN enabled_categories TEXT[] NOT NULL DEFAULT '{credential_medical,credential_language,credential_security,credential_other,rating_expiry,currency_passenger,currency_night,currency_instrument,currency_flight_review,currency_revalidation}',
    ADD COLUMN check_hour INTEGER NOT NULL DEFAULT 8;

-- Step 2: Migrate existing data — map old booleans to new categories
-- Users who had currency_warnings=false: remove all currency_* categories
UPDATE notification_preferences
SET enabled_categories = ARRAY_REMOVE(
    ARRAY_REMOVE(
        ARRAY_REMOVE(
            ARRAY_REMOVE(
                ARRAY_REMOVE(
                    ARRAY_REMOVE(enabled_categories, 'rating_expiry'),
                    'currency_passenger'),
                'currency_night'),
            'currency_instrument'),
        'currency_flight_review'),
    'currency_revalidation')
WHERE currency_warnings = false;

-- Users who had credential_warnings=false: remove all credential_* categories
UPDATE notification_preferences
SET enabled_categories = ARRAY_REMOVE(
    ARRAY_REMOVE(
        ARRAY_REMOVE(
            ARRAY_REMOVE(enabled_categories, 'credential_medical'),
            'credential_language'),
        'credential_security'),
    'credential_other')
WHERE credential_warnings = false;

-- Step 3: Drop old boolean columns
ALTER TABLE notification_preferences
    DROP COLUMN currency_warnings,
    DROP COLUMN credential_warnings;

-- Step 4: Add expiry_reference_date to notification_log for cycle-aware dedup
ALTER TABLE notification_log
    ADD COLUMN expiry_reference_date DATE,
    ADD COLUMN subject VARCHAR(500);

-- Step 5: Drop old dedup index and create new one with expiry_reference_date
DROP INDEX IF EXISTS idx_notification_log_dedup;
CREATE UNIQUE INDEX idx_notification_log_dedup
    ON notification_log(user_id, notification_type, reference_id, days_before_expiry, expiry_reference_date);
