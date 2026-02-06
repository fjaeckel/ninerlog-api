-- Add off-block and on-block time columns to flights table
-- Per EASA FCL.010 and FAA 14 CFR 1.1:
--   Block time = off-block (chocks off / engine start) to on-block (chocks on / engine shutdown)
--   Flight time (airborne) = takeoff (departure_time) to landing (arrival_time)

ALTER TABLE flights
    ADD COLUMN off_block_time TIME,
    ADD COLUMN on_block_time TIME;

-- Update column comments for clarity
COMMENT ON COLUMN flights.off_block_time IS 'Off-block time (chocks off / engine start) in UTC';
COMMENT ON COLUMN flights.on_block_time IS 'On-block time (chocks on / engine shutdown) in UTC';
COMMENT ON COLUMN flights.departure_time IS 'Takeoff time in UTC (airborne start)';
COMMENT ON COLUMN flights.arrival_time IS 'Landing time in UTC (airborne end)';
