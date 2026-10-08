-- Revert flight session airports to CHAR(4); longer identifiers are cleared.
UPDATE flight_sessions SET departure_icao = NULL WHERE LENGTH(departure_icao) > 4;
UPDATE flight_sessions SET arrival_icao = NULL WHERE LENGTH(arrival_icao) > 4;

ALTER TABLE flight_sessions
    ALTER COLUMN departure_icao TYPE CHAR(4),
    ALTER COLUMN arrival_icao TYPE CHAR(4);
