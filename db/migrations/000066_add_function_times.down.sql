ALTER TABLE flight_baselines
    DROP COLUMN IF EXISTS relief_minutes,
    DROP COLUMN IF EXISTS examiner_minutes,
    DROP COLUMN IF EXISTS spic_minutes,
    DROP COLUMN IF EXISTS picus_minutes;

ALTER TABLE flights
    DROP COLUMN IF EXISTS relief_time,
    DROP COLUMN IF EXISTS examiner_time,
    DROP COLUMN IF EXISTS spic_time,
    DROP COLUMN IF EXISTS picus_time;
