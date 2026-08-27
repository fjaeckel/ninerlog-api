-- Pilot function times beyond PIC/SIC/dual: PICUS (PIC under supervision,
-- EASA FCL.030(2)(e)), SPIC (student PIC on an integrated course), examiner
-- time (conducting a check, overlays function time like dual_given_time) and
-- cruise relief co-pilot time. Integer minutes, declared by the pilot, never
-- auto-derived. The same four are added to the carried-forward baseline.
ALTER TABLE flights
    ADD COLUMN picus_time INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN spic_time INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN examiner_time INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN relief_time INTEGER NOT NULL DEFAULT 0;

ALTER TABLE flight_baselines
    ADD COLUMN picus_minutes INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN spic_minutes INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN examiner_minutes INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN relief_minutes INTEGER NOT NULL DEFAULT 0;
