DROP TRIGGER IF EXISTS record_license_deletion ON licenses;
DROP TRIGGER IF EXISTS record_credential_deletion ON credentials;
DROP TRIGGER IF EXISTS record_contact_deletion ON contacts;
DROP TRIGGER IF EXISTS record_aircraft_deletion ON aircraft;
DROP TRIGGER IF EXISTS record_flight_deletion ON flights;

DROP FUNCTION IF EXISTS record_deletion_tombstone();

DROP INDEX IF EXISTS idx_deletion_tombstones_entity;
DROP INDEX IF EXISTS idx_deletion_tombstones_deleted_at;
DROP INDEX IF EXISTS idx_deletion_tombstones_user_deleted_at;

DROP TABLE IF EXISTS deletion_tombstones;
