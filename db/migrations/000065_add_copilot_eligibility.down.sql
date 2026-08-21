DROP INDEX IF EXISTS idx_flights_user_passenger;
DROP INDEX IF EXISTS idx_flights_user_date_loggable;

CREATE INDEX IF NOT EXISTS idx_flights_user_date_not_simulator
    ON flights (user_id, date DESC)
    WHERE NOT is_simulator;

ALTER TABLE flights DROP COLUMN IF EXISTS multi_pilot_time_override;

ALTER TABLE flights DROP COLUMN IF EXISTS sic_time_override;

ALTER TABLE flights DROP COLUMN IF EXISTS is_passenger;

ALTER TABLE aircraft DROP COLUMN IF EXISTS is_multi_pilot;
