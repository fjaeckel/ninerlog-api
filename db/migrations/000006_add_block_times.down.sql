-- Remove off-block and on-block time columns from flights table
ALTER TABLE flights
    DROP COLUMN IF EXISTS off_block_time,
    DROP COLUMN IF EXISTS on_block_time;
