-- Glider/SPL operations: make launches a first-class count and record
-- release (cable-break) altitude for winch/aerotow launches.
--
-- Background: until now SPL recency (EASA FCL.140.S) used the landings count as
-- a proxy for launches. A club training row routinely chains several winch
-- launches in a single logical entry, so launches must be tracked separately.

ALTER TABLE flights
    ADD COLUMN launches INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN release_altitude_m INTEGER,
    ADD COLUMN release_altitude_ref VARCHAR(4);

-- Constrain the reference datum.
ALTER TABLE flights
    ADD CONSTRAINT chk_flights_release_altitude_ref
    CHECK (release_altitude_ref IS NULL OR release_altitude_ref IN ('AGL', 'AMSL'));

-- Release altitude only makes sense alongside a launch method.
ALTER TABLE flights
    ADD CONSTRAINT chk_flights_release_altitude_requires_launch
    CHECK (release_altitude_m IS NULL OR launch_method IS NOT NULL);

-- Backfill: existing glider flights (those with a launch_method) inherit their
-- landings count as the launch count so SPL currency totals are unchanged.
-- A glider flight always has at least one launch. Powered flights keep 0.
UPDATE flights
SET launches = GREATEST(COALESCE(all_landings, landings_day + landings_night, 1), 1)
WHERE launch_method IS NOT NULL;
