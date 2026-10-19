-- notification_log: every row is keyed by the date it concerns, and the
-- dedup unique index holds for every row.
--
-- expiry_reference_date is the concerned date: the expiry date (last valid
-- day) for warnings, credentials, ratings and revalidation notices; the first
-- non-current day for currency and custom-rule lapse notices; 0001-01-01 for
-- an item that was never current.
-- skipped marks a warning threshold recorded as satisfied without a mail.
--
-- Backfill of rows written before this migration:
--   reference_id NULL       -> the nil UUID
--   days_before_expiry NULL -> 0
--   currency_revalidation with a NULL date -> the rating's current expiry date
--   any other NULL date     -> the UTC date the row was sent
-- Rows that collide after the backfill keep the earliest.

DROP INDEX IF EXISTS idx_notification_log_dedup;

UPDATE notification_log
SET reference_id = '00000000-0000-0000-0000-000000000000'
WHERE reference_id IS NULL;

UPDATE notification_log
SET days_before_expiry = 0
WHERE days_before_expiry IS NULL;

UPDATE notification_log nl
SET expiry_reference_date = cr.expiry_date
FROM class_ratings cr
WHERE nl.expiry_reference_date IS NULL
  AND nl.notification_type = 'currency_revalidation'
  AND nl.reference_id = cr.id
  AND cr.expiry_date IS NOT NULL;

UPDATE notification_log
SET expiry_reference_date = (sent_at AT TIME ZONE 'UTC')::date
WHERE expiry_reference_date IS NULL;

DELETE FROM notification_log a
USING notification_log b
WHERE a.user_id = b.user_id
  AND a.notification_type = b.notification_type
  AND a.reference_id = b.reference_id
  AND a.days_before_expiry = b.days_before_expiry
  AND a.expiry_reference_date = b.expiry_reference_date
  AND (a.sent_at, a.id) > (b.sent_at, b.id);

ALTER TABLE notification_log
    ALTER COLUMN reference_id SET NOT NULL,
    ALTER COLUMN days_before_expiry SET NOT NULL,
    ALTER COLUMN expiry_reference_date SET NOT NULL,
    ADD COLUMN skipped BOOLEAN NOT NULL DEFAULT false;

CREATE UNIQUE INDEX idx_notification_log_dedup
    ON notification_log(user_id, notification_type, reference_id, days_before_expiry, expiry_reference_date);
