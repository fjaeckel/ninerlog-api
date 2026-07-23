-- Revert airport fields to a strict 4-char ICAO code. Any value longer than 4
-- characters is truncated to fit the narrower column.
UPDATE flights SET departure_icao = LEFT(departure_icao, 4) WHERE LENGTH(departure_icao) > 4;
UPDATE flights SET arrival_icao = LEFT(arrival_icao, 4) WHERE LENGTH(arrival_icao) > 4;
UPDATE aircraft SET default_departure_icao = LEFT(default_departure_icao, 4) WHERE LENGTH(default_departure_icao) > 4;
UPDATE aircraft SET default_arrival_icao = LEFT(default_arrival_icao, 4) WHERE LENGTH(default_arrival_icao) > 4;

ALTER TABLE flights
    ALTER COLUMN departure_icao TYPE VARCHAR(4),
    ALTER COLUMN arrival_icao TYPE VARCHAR(4);

COMMENT ON COLUMN flights.departure_icao IS NULL;
COMMENT ON COLUMN flights.arrival_icao IS NULL;

ALTER TABLE aircraft
    ALTER COLUMN default_departure_icao TYPE VARCHAR(4),
    ALTER COLUMN default_arrival_icao TYPE VARCHAR(4);

COMMENT ON COLUMN aircraft.default_departure_icao IS 'Default departure airfield (ICAO) prefilled when logging a flight with this aircraft';
COMMENT ON COLUMN aircraft.default_arrival_icao IS 'Default arrival airfield (ICAO) prefilled when logging a flight with this aircraft';
