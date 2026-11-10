-- Fails when a user holds more than one aircraft with the same registration;
-- merge or delete those entries first.
DROP INDEX IF EXISTS idx_aircraft_user_registration_lookup;
ALTER TABLE aircraft DROP CONSTRAINT IF EXISTS aircraft_registration_validity_excl;
ALTER TABLE aircraft DROP CONSTRAINT IF EXISTS aircraft_validity_order;
CREATE UNIQUE INDEX idx_aircraft_user_registration ON aircraft(user_id, registration);
ALTER TABLE aircraft
    DROP COLUMN IF EXISTS valid_to,
    DROP COLUMN IF EXISTS valid_from;
