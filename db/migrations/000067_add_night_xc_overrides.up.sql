-- Adds the override flags for the two derived times a pilot most often
-- disagrees with. Night time is derived from civil twilight at the departure
-- location; cross-country from departure ≠ arrival. Neither matches every
-- regulator's definition (FAA 14 CFR 61.1(b) 50 NM landing rule, EASA FCL.010
-- pre-planned route), so a value the pilot enters is kept and re-derivation
-- skips the field, following the pattern of takeoffs_*_override and
-- sic_time_override.

ALTER TABLE flights ADD COLUMN night_time_override BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE flights ADD COLUMN cross_country_time_override BOOLEAN NOT NULL DEFAULT FALSE;
