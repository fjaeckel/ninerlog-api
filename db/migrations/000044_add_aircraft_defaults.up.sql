-- Per-aircraft logging defaults: prefill departure/arrival airfields when
-- logging a flight with this aircraft.
ALTER TABLE aircraft
    ADD COLUMN default_departure_icao VARCHAR(4),
    ADD COLUMN default_arrival_icao VARCHAR(4);

COMMENT ON COLUMN aircraft.default_departure_icao IS 'Default departure airfield (ICAO) prefilled when logging a flight with this aircraft';
COMMENT ON COLUMN aircraft.default_arrival_icao IS 'Default arrival airfield (ICAO) prefilled when logging a flight with this aircraft';
