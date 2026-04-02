-- Add proficiency check flag to flights table
ALTER TABLE flights ADD COLUMN is_proficiency_check BOOLEAN NOT NULL DEFAULT false;
