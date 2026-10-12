ALTER TABLE flights
    DROP COLUMN IF EXISTS release_height_m,
    DROP COLUMN IF EXISTS is_tow_flight,
    DROP COLUMN IF EXISTS is_outlanding,
    DROP COLUMN IF EXISTS launches_override,
    DROP COLUMN IF EXISTS launches;
