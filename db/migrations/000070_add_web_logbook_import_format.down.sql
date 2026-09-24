-- Rebuild import_format without 'WEB_LOGBOOK_CSV', folding its rows to 'CSV'.

ALTER TABLE flight_imports
    ALTER COLUMN import_format TYPE TEXT USING import_format::text;

UPDATE flight_imports
SET import_format = 'CSV'
WHERE import_format = 'WEB_LOGBOOK_CSV';

ALTER TYPE import_format RENAME TO import_format_old;

CREATE TYPE import_format AS ENUM (
    'CSV', 'FOREFLIGHT_CSV', 'XLS', 'XLSX',
    'NINERLOG_CSV', 'LOGTEN_CSV', 'MYFLIGHTBOOK_CSV', 'CAPZLOG_CSV',
    'FLYLOG_CSV', 'WADER_CSV', 'VEREINSFLIEGER_CSV',
    'VEREINSFLIEGER_EXTENDED_CSV', 'SKYDEMON_CSV', 'EASA_CSV', 'FAA_CSV'
);

ALTER TABLE flight_imports
    ALTER COLUMN import_format TYPE import_format
    USING import_format::import_format;

DROP TYPE import_format_old;
