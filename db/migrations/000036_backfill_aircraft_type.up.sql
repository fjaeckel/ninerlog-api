-- Backfill flights.aircraft_type for rows that were imported without a type
-- (the previous importer fell back to using the registration as the type, which
-- broke the "Hours by Aircraft Type" / "Aircraft Type Breakdown" reports).
-- For each user, look up the matching aircraft in their fleet by registration
-- and replace the (incorrect) type with the actual aircraft type.
UPDATE flights f
SET aircraft_type = a.type
FROM aircraft a
WHERE a.user_id = f.user_id
  AND UPPER(a.registration) = UPPER(f.aircraft_reg)
  AND UPPER(f.aircraft_type) = UPPER(f.aircraft_reg)
  AND a.type <> ''
  AND a.type IS NOT NULL;
