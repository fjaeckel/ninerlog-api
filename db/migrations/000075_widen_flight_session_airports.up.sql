-- Widen flight session airports from CHAR(4) to VARCHAR(10): a 4-char ICAO
-- code or an OurAirports local identifier such as DE-0249.
ALTER TABLE flight_sessions
    ALTER COLUMN departure_icao TYPE VARCHAR(10),
    ALTER COLUMN arrival_icao TYPE VARCHAR(10);
