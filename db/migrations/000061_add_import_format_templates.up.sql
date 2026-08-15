-- Add the per-source logbook formats to the import_format enum.
--
-- import_format is a Postgres ENUM (migration 13), so the importer cannot
-- record which logbook a file came from until the value exists here: the INSERT
-- into flight_imports is rejected by the type itself. The set mirrors the
-- ImportFormat enum in api-spec/openapi.yaml and the template catalogue in
-- internal/service/importtemplate/sources.go — all three must be changed
-- together when another logbook is added.
--
-- Recording the source (rather than collapsing everything to 'CSV') is what
-- lets the admin dashboard show which logbooks pilots are migrating from, and
-- what makes a past import auditable once a template's column mapping changes.
--
-- ADD VALUE inside a transaction is fine on Postgres 12+ as long as the new
-- value is not *used* in the same transaction, which nothing here does.

ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'NINERLOG_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'LOGTEN_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'MYFLIGHTBOOK_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'CAPZLOG_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'FLYLOG_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'WADER_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'VEREINSFLIEGER_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'MCC_PILOTLOG_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'SKYDEMON_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'EASA_CSV';
ALTER TYPE import_format ADD VALUE IF NOT EXISTS 'FAA_CSV';
