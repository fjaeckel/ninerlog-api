-- Adds the two facts that decide whether co-pilot time may be logged.
--
-- aircraft.is_multi_pilot records that the type is certificated for a minimum
-- crew of two pilots (EASA FCL.010; FAA type certificate). Co-pilot time is
-- loggable only on such an aircraft, when the user is a required safety pilot
-- (14 CFR 91.109(b)), or when the user declares the co-pilot seat themselves.
--
-- flights.is_passenger marks a row the user was carried on rather than crewed.
-- Such a row keeps its record of the trip and contributes no flight time.

ALTER TABLE aircraft ADD COLUMN is_multi_pilot BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE flights ADD COLUMN is_passenger BOOLEAN NOT NULL DEFAULT FALSE;

-- Separate a co-pilot or multi-pilot time the pilot declared from one
-- derivation produced, so re-running derivation cannot mistake its own output
-- for a declaration.
ALTER TABLE flights ADD COLUMN sic_time_override BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE flights ADD COLUMN multi_pilot_time_override BOOLEAN NOT NULL DEFAULT FALSE;

-- Grandfather the values derivation could not have produced. Before this
-- migration a co-pilot or multi-pilot time on a row with no crew list was kept
-- as entered, so it came from the pilot; one on a row with a crew list was
-- written by derivation. Marking only the former as declared protects imported
-- and hand-entered logbooks while leaving derived values free to be corrected.
UPDATE flights f
SET sic_time_override = TRUE
WHERE f.sic_time > 0
  AND NOT EXISTS (SELECT 1 FROM flight_crew_members m WHERE m.flight_id = f.id);

UPDATE flights f
SET multi_pilot_time_override = TRUE
WHERE f.multi_pilot_time > 0
  AND NOT EXISTS (SELECT 1 FROM flight_crew_members m WHERE m.flight_id = f.id);

-- No other row is touched; POST /flights/recalculate re-derives the rest once
-- the fleet carries its multi-pilot designations.

DROP INDEX IF EXISTS idx_flights_user_date_not_simulator;

CREATE INDEX idx_flights_user_date_loggable
    ON flights (user_id, date DESC)
    WHERE NOT is_simulator AND NOT is_passenger;

CREATE INDEX idx_flights_user_passenger
    ON flights (user_id)
    WHERE is_passenger;
