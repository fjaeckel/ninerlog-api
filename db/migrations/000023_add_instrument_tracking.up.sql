-- Add instrument tracking columns to flights
ALTER TABLE flights
    ADD COLUMN actual_instrument_time DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN simulated_instrument_time DOUBLE PRECISION NOT NULL DEFAULT 0,
    ADD COLUMN holds INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN approaches_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN is_ipc BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN is_flight_review BOOLEAN NOT NULL DEFAULT false;
