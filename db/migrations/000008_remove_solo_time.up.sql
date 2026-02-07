-- Remove solo_time column from flights table
-- Solo time is no longer tracked as a separate field

ALTER TABLE flights DROP COLUMN IF EXISTS solo_time;

-- Drop the old time_breakdown_valid constraint that referenced solo_time
ALTER TABLE flights DROP CONSTRAINT IF EXISTS valid_time_distribution;
