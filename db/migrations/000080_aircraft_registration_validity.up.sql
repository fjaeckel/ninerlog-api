-- A registration can be reissued to a different airframe (N123AB as a C172,
-- later as a PA-16). Each fleet entry carries the date range during which its
-- registration identified it; NULL bounds are open. Ranges for the same
-- (user, registration) may not overlap, so every flight date resolves to at
-- most one fleet entry.
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE aircraft
    ADD COLUMN valid_from DATE,
    ADD COLUMN valid_to   DATE;

ALTER TABLE aircraft
    ADD CONSTRAINT aircraft_validity_order
    CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from <= valid_to);

DROP INDEX IF EXISTS idx_aircraft_user_registration;

ALTER TABLE aircraft
    ADD CONSTRAINT aircraft_registration_validity_excl
    EXCLUDE USING gist (
        user_id WITH =,
        registration WITH =,
        daterange(valid_from, valid_to, '[]') WITH &&
    );

CREATE INDEX idx_aircraft_user_registration_lookup ON aircraft(user_id, registration);

COMMENT ON COLUMN aircraft.valid_from IS 'First day the registration identified this aircraft (NULL = open)';
COMMENT ON COLUMN aircraft.valid_to IS 'Last day the registration identified this aircraft (NULL = open)';
