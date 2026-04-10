-- Revert migration 000031: Convert INTEGER minutes back to DECIMAL hours
-- Note: This reintroduces the rounding errors that prompted the migration.

ALTER TABLE flights ALTER COLUMN total_time TYPE DECIMAL(5,2) USING (total_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN pic_time TYPE DECIMAL(5,2) USING (pic_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN dual_time TYPE DECIMAL(5,2) USING (dual_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN night_time TYPE DECIMAL(5,2) USING (night_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN ifr_time TYPE DECIMAL(5,2) USING (ifr_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN solo_time TYPE DECIMAL(8,2) USING (solo_time / 60.0)::DECIMAL(8,2);
ALTER TABLE flights ALTER COLUMN cross_country_time TYPE DECIMAL(8,2) USING (cross_country_time / 60.0)::DECIMAL(8,2);
ALTER TABLE flights ALTER COLUMN sic_time TYPE DECIMAL(5,2) USING (sic_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN dual_given_time TYPE DECIMAL(5,2) USING (dual_given_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN simulated_flight_time TYPE DECIMAL(5,2) USING (simulated_flight_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN ground_training_time TYPE DECIMAL(5,2) USING (ground_training_time / 60.0)::DECIMAL(5,2);
ALTER TABLE flights ALTER COLUMN actual_instrument_time TYPE DOUBLE PRECISION USING (actual_instrument_time / 60.0);
ALTER TABLE flights ALTER COLUMN simulated_instrument_time TYPE DOUBLE PRECISION USING (simulated_instrument_time / 60.0);

ALTER TABLE users DROP COLUMN time_display_format;
