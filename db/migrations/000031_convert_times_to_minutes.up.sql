-- Migration 000031: Convert all flight time columns from DECIMAL/DOUBLE to INTEGER (minutes)
-- and add time_display_format user preference.
--
-- Rationale: DECIMAL(5,2) causes rounding errors (e.g., 1h23m = 1.3833... stored as 1.38,
-- losing 0.2 minutes per flight). INTEGER minutes is lossless.
-- See Phase 6b½ in roadmap for full regulatory justification.

-- Step 1: Convert flight time columns from decimal hours to integer minutes
-- Formula: ROUND(column_value * 60) converts decimal hours to nearest minute

-- total_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN total_time TYPE INTEGER USING ROUND(total_time * 60)::INTEGER;

-- pic_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN pic_time TYPE INTEGER USING ROUND(pic_time * 60)::INTEGER;

-- dual_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN dual_time TYPE INTEGER USING ROUND(dual_time * 60)::INTEGER;

-- night_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN night_time TYPE INTEGER USING ROUND(night_time * 60)::INTEGER;

-- ifr_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN ifr_time TYPE INTEGER USING ROUND(ifr_time * 60)::INTEGER;

-- solo_time (DECIMAL 8,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN solo_time TYPE INTEGER USING ROUND(solo_time * 60)::INTEGER;

-- cross_country_time (DECIMAL 8,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN cross_country_time TYPE INTEGER USING ROUND(cross_country_time * 60)::INTEGER;

-- sic_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN sic_time TYPE INTEGER USING ROUND(sic_time * 60)::INTEGER;

-- dual_given_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN dual_given_time TYPE INTEGER USING ROUND(dual_given_time * 60)::INTEGER;

-- simulated_flight_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN simulated_flight_time TYPE INTEGER USING ROUND(simulated_flight_time * 60)::INTEGER;

-- ground_training_time (DECIMAL 5,2 → INTEGER)
ALTER TABLE flights ALTER COLUMN ground_training_time TYPE INTEGER USING ROUND(ground_training_time * 60)::INTEGER;

-- actual_instrument_time (DOUBLE PRECISION → INTEGER)
ALTER TABLE flights ALTER COLUMN actual_instrument_time TYPE INTEGER USING ROUND(actual_instrument_time * 60)::INTEGER;

-- simulated_instrument_time (DOUBLE PRECISION → INTEGER)
ALTER TABLE flights ALTER COLUMN simulated_instrument_time TYPE INTEGER USING ROUND(simulated_instrument_time * 60)::INTEGER;

-- Step 2: Add time_display_format to users table
-- "hm" = hours:minutes (EASA default), "decimal" = decimal hours (FAA convention)
ALTER TABLE users ADD COLUMN time_display_format VARCHAR(10) NOT NULL DEFAULT 'hm';
