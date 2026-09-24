-- Add 'WEB_LOGBOOK_CSV' to the import_format enum.

ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'WEB_LOGBOOK_CSV';
