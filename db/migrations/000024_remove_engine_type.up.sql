-- Remove engine_type column and change aircraft_class to VARCHAR for free-form values
ALTER TABLE aircraft DROP COLUMN IF EXISTS engine_type;
ALTER TABLE aircraft ALTER COLUMN aircraft_class TYPE VARCHAR(50);
