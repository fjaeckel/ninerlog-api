-- Widen departure/arrival airport fields from a strict 4-char ICAO code to a
-- free-text location name. Helicopter and glider pilots routinely operate from
-- fields, meadows, and off-airport sites that have no ICAO code but are still
-- legitimate logbook entries. Values that happen to match a real ICAO code are
-- still resolved to coordinates for distance/map/night calculations; anything
-- else is stored verbatim and simply omitted from the map.
ALTER TABLE flights
    ALTER COLUMN departure_icao TYPE VARCHAR(100),
    ALTER COLUMN arrival_icao TYPE VARCHAR(100);

COMMENT ON COLUMN flights.departure_icao IS 'Departure location: an ICAO code (resolved to coordinates) or a free-text place name (e.g. an off-airport glider/helicopter site)';
COMMENT ON COLUMN flights.arrival_icao IS 'Arrival location: an ICAO code (resolved to coordinates) or a free-text place name (e.g. an off-airport glider/helicopter site)';

ALTER TABLE aircraft
    ALTER COLUMN default_departure_icao TYPE VARCHAR(100),
    ALTER COLUMN default_arrival_icao TYPE VARCHAR(100);

COMMENT ON COLUMN aircraft.default_departure_icao IS 'Default departure location (ICAO code or free-text place name) prefilled when logging a flight with this aircraft';
COMMENT ON COLUMN aircraft.default_arrival_icao IS 'Default arrival location (ICAO code or free-text place name) prefilled when logging a flight with this aircraft';
