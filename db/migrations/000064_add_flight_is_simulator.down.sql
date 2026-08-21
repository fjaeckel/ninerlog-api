-- Reverses 000063. The flight-time columns cleared on FSTD sessions are not
-- restored: their pre-migration values were block times invented to satisfy
-- the required off-block/on-block fields.

DROP INDEX IF EXISTS idx_flights_user_simulator;
DROP INDEX IF EXISTS idx_flights_user_date_not_simulator;

ALTER TABLE flights DROP COLUMN IF EXISTS is_simulator;
