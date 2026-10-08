-- Widen flight session airports from a 4-char ICAO code to an airport
-- identifier of up to 10 characters, so a session can record a field without
-- an ICAO code by its OurAirports local identifier (e.g. DE-0249).
ALTER TABLE flight_sessions
    ALTER COLUMN departure_icao TYPE VARCHAR(10),
    ALTER COLUMN arrival_icao TYPE VARCHAR(10);
