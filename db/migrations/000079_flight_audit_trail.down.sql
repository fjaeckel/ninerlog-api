DROP TRIGGER IF EXISTS flight_revisions_append_only ON flight_revisions;
DROP TRIGGER IF EXISTS flight_revision_finalize ON flight_revision_pending;
DROP TRIGGER IF EXISTS flight_revision_capture_crew ON flight_crew_members;
DROP TRIGGER IF EXISTS flight_revision_capture ON flights;

DROP FUNCTION IF EXISTS flight_revisions_append_only();
DROP FUNCTION IF EXISTS flight_revision_finalize();
DROP FUNCTION IF EXISTS flight_revision_capture_crew();
DROP FUNCTION IF EXISTS flight_revision_capture_flight();
DROP FUNCTION IF EXISTS flight_revision_capture(UUID);
DROP FUNCTION IF EXISTS flight_revision_crew_key(JSONB);
DROP FUNCTION IF EXISTS flight_revision_crew(UUID);

DROP TABLE IF EXISTS flight_revision_pending;
DROP TABLE IF EXISTS flight_revisions;
DROP TABLE IF EXISTS logbook_certifications;
DROP TABLE IF EXISTS logbook_mode_events;

ALTER TABLE flight_signatures DROP COLUMN IF EXISTS signed_fields_hash;
ALTER TABLE licenses DROP COLUMN IF EXISTS issuing_country;
ALTER TABLE users
    DROP COLUMN IF EXISTS strict_logbook_since,
    DROP COLUMN IF EXISTS strict_logbook;
