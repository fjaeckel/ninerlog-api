ALTER TABLE flights DROP CONSTRAINT IF EXISTS chk_flights_release_altitude_requires_launch;
ALTER TABLE flights DROP CONSTRAINT IF EXISTS chk_flights_release_altitude_ref;
ALTER TABLE flights DROP COLUMN IF EXISTS release_altitude_ref;
ALTER TABLE flights DROP COLUMN IF EXISTS release_altitude_m;
ALTER TABLE flights DROP COLUMN IF EXISTS launches;
