-- Add launch_method column to flights for SPL/glider tracking
ALTER TABLE flights ADD COLUMN launch_method VARCHAR(20);
