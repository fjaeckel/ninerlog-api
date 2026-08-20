-- Restore the NOT NULL on flight_imports.license_id.
--
-- Rows with a NULL license_id are deleted first. They are exactly the rows this
-- migration made possible -- before it, the constraint rejected every one of
-- them -- so removing them restores the state that preceded the migration
-- rather than discarding anything that could have existed without it.
--
-- Only import history is affected. The imported flights, aircraft and contacts
-- are separate tables and are untouched.

DELETE FROM flight_imports WHERE license_id IS NULL;

ALTER TABLE flight_imports ALTER COLUMN license_id SET NOT NULL;
