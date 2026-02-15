-- Remove instrument tracking columns from flights
ALTER TABLE flights
    DROP COLUMN IF EXISTS actual_instrument_time,
    DROP COLUMN IF EXISTS simulated_instrument_time,
    DROP COLUMN IF EXISTS holds,
    DROP COLUMN IF EXISTS approaches_count,
    DROP COLUMN IF EXISTS is_ipc,
    DROP COLUMN IF EXISTS is_flight_review;
