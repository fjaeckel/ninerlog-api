-- Postgres cannot drop a value from an enum, so reversing migration 61 means
-- rebuilding import_format without the per-source formats and re-pointing the
-- column at it.
--
-- Rolling back must not lose a pilot's import history, and the history is only
-- a record of what happened -- nothing keys off the exact format. So rows
-- recorded against a removed format are folded back to 'CSV', which is what
-- they would have been recorded as before this migration, rather than failing
-- the rollback the way migration 58's down does for credentials.

ALTER TABLE flight_imports
    ALTER COLUMN import_format TYPE TEXT USING import_format::text;

UPDATE flight_imports
SET import_format = 'CSV'
WHERE import_format NOT IN ('CSV', 'FOREFLIGHT_CSV', 'XLS', 'XLSX');

ALTER TYPE import_format RENAME TO import_format_old;

CREATE TYPE import_format AS ENUM ('CSV', 'FOREFLIGHT_CSV', 'XLS', 'XLSX');

ALTER TABLE flight_imports
    ALTER COLUMN import_format TYPE import_format
    USING import_format::import_format;

DROP TYPE import_format_old;
