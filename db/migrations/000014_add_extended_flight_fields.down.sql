-- Revert extended flight fields
ALTER TABLE flights DROP COLUMN IF EXISTS route;
ALTER TABLE flights DROP COLUMN IF EXISTS takeoffs_day;
ALTER TABLE flights DROP COLUMN IF EXISTS takeoffs_night;
ALTER TABLE flights DROP COLUMN IF EXISTS solo_time;
ALTER TABLE flights DROP COLUMN IF EXISTS cross_country_time;
ALTER TABLE flights DROP COLUMN IF EXISTS distance;
ALTER TABLE flights DROP COLUMN IF EXISTS all_landings;
ALTER TABLE flights DROP COLUMN IF EXISTS takeoffs_day_override;
ALTER TABLE flights DROP COLUMN IF EXISTS takeoffs_night_override;
ALTER TABLE flights DROP COLUMN IF EXISTS landings_day_override;
ALTER TABLE flights DROP COLUMN IF EXISTS landings_night_override;
