-- Add is_pic and is_dual boolean columns to flights table
-- These replace manual entry of pic_time and dual_time with a simple toggle.
-- The server computes pic_time/dual_time from these booleans and total_time.

ALTER TABLE flights
    ADD COLUMN is_pic BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN is_dual BOOLEAN NOT NULL DEFAULT false;

-- Backfill: set is_pic = true where pic_time > 0, is_dual = true where dual_time > 0
UPDATE flights SET is_pic = true WHERE pic_time > 0;
UPDATE flights SET is_dual = true WHERE dual_time > 0;

-- Add constraint: is_pic and is_dual cannot both be true
ALTER TABLE flights
    ADD CONSTRAINT pic_dual_exclusive CHECK (NOT (is_pic AND is_dual));

COMMENT ON COLUMN flights.is_pic IS 'Whether this flight was logged as pilot-in-command';
COMMENT ON COLUMN flights.is_dual IS 'Whether this flight was logged as dual instruction received';
